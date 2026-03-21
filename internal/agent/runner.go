package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/prompt"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workspace"
	"github.com/symphony-go/symphony/internal/workflow"
)

// OrchestratorEvent is sent from a worker back to the orchestrator.
type OrchestratorEvent struct {
	Type    EventType
	IssueID string
	Payload any
}

// EventType identifies the kind of orchestrator event.
type EventType int

const (
	EventWorkerDone   EventType = iota
	EventWorkerFailed
	EventAgentUpdate
)

// RunParams holds all inputs for an agent run attempt.
type RunParams struct {
	Issue           *tracker.Issue
	Attempt         *int
	Workflow        *workflow.WorkflowDefinition
	GeminiCfg       *config.GeminiConfig
	ClaudeCfg       *config.ClaudeConfig
	MiniAgentCfg    *config.MiniAgentConfig
	AgentCfg        *config.AgentConfig
	ActiveStates    []string
	WorkspaceMgr    *workspace.Manager
	WorkspaceRoot   string
	ExtraEnv        []string  // additional env vars for the agent subprocess
	EventLogWriter  io.Writer // if non-nil, raw protocol events are written here
	CallbackURL     string    // base URL of the Symphony HTTP server for agent callbacks
	CheckIssueState func(ctx context.Context, issueID string) (string, error)
}

// AgentLauncher is the interface for launching agent runs (supports mock injection).
type AgentLauncher interface {
	Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error
}

// GeminiRunner implements AgentLauncher using Gemini CLI in ACP mode.
type GeminiRunner struct{}

// NewGeminiRunner creates a new GeminiRunner.
func NewGeminiRunner() *GeminiRunner {
	return &GeminiRunner{}
}

// NewLauncher returns an AgentLauncher for the given backend name.
func NewLauncher(backend string) (AgentLauncher, error) {
	switch backend {
	case "", "gemini":
		return NewGeminiRunner(), nil
	case "claude":
		return NewClaudeRunner(), nil
	case "mini_agent", "mini-agent":
		return NewMiniAgentRunner(), nil
	default:
		return nil, fmt.Errorf("unsupported backend: %q", backend)
	}
}

// Launch runs a full agent attempt: workspace → prompt → ACP session → turn loop.
func (r *GeminiRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error {
	logger := slog.With("issue_id", params.Issue.ID, "issue_identifier", params.Issue.Identifier)

	// 1. Create/reuse workspace
	ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	if err != nil {
		return fmt.Errorf("workspace creation failed: %w", err)
	}
	logger = logger.With("workspace", ws.Path)

	// 2. Validate workspace path
	if err := workspace.ValidateWorkspacePath(ws.Path, params.WorkspaceRoot); err != nil {
		return fmt.Errorf("workspace safety check failed: %w", err)
	}

	// 3. Run before_run hook
	if err := params.WorkspaceMgr.RunBeforeRun(ws.Path); err != nil {
		return fmt.Errorf("before_run hook failed: %w", err)
	}

	// 4. Launch ACP client
	logger.Info("launching Gemini CLI", "command", params.GeminiCfg.Command)
	client, err := NewACPClient(params.GeminiCfg.Command, ws.Path, params.ExtraEnv)
	if err != nil {
		params.WorkspaceMgr.RunAfterRun(ws.Path)
		return fmt.Errorf("failed to launch ACP subprocess: %w", err)
	}
	defer func() {
		client.Close()
		params.WorkspaceMgr.RunAfterRun(ws.Path)
	}()

	readTimeout := time.Duration(params.GeminiCfg.ReadTimeoutMs) * time.Millisecond
	turnTimeout := time.Duration(params.GeminiCfg.TurnTimeoutMs) * time.Millisecond

	// 5. ACP handshake
	initResult, err := client.Initialize(readTimeout)
	if err != nil {
		return fmt.Errorf("ACP initialize failed: %w", err)
	}
	logger.Info("ACP initialized", "agent", initResult.AgentInfo.Name, "protocol_version", initResult.ProtocolVersion)
	if params.EventLogWriter != nil {
		fmt.Fprintf(params.EventLogWriter, " %s%s%s  %s%s── ACP initialized — agent: %s, protocol: %d ──%s\n",
			cGray, time.Now().Format("15:04:05"), cReset, cBold, cBlue, initResult.AgentInfo.Name, initResult.ProtocolVersion, cReset)
	}

	sessionID, err := client.SessionNew(ws.Path, readTimeout)
	if err != nil {
		return fmt.Errorf("ACP session/new failed: %w", err)
	}
	logger = logger.With("session_id", sessionID)
	logger.Info("ACP session created")
	if params.EventLogWriter != nil {
		fmt.Fprintf(params.EventLogWriter, " %s%s%s  %s%s── Session created: %s ──%s\n",
			cGray, time.Now().Format("15:04:05"), cReset, cBold, cBlue, sessionID, cReset)
	}

	// Emit session_started
	eventCh <- OrchestratorEvent{
		Type:    EventAgentUpdate,
		IssueID: params.Issue.ID,
		Payload: AgentEvent{
			Type:      EventSessionStarted,
			Timestamp: time.Now().UTC(),
			SessionID: sessionID,
		},
	}

	// 6. Turn loop
	maxTurns := params.AgentCfg.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 20
	}

	for turnNumber := 1; turnNumber <= maxTurns; turnNumber++ {
		select {
		case <-ctx.Done():
			client.SessionCancel(sessionID)
			return ctx.Err()
		default:
		}

		// Build prompt
		turnPrompt, err := buildTurnPrompt(params.Workflow, params.Issue, params.Attempt, turnNumber, maxTurns)
		if err != nil {
			return fmt.Errorf("prompt rendering failed: %w", err)
		}

		logger.Info("starting turn", "turn", turnNumber, "max_turns", maxTurns)

		// Update handler forwards events to orchestrator
		updateHandler := func(update *SessionUpdateParams) {
			logEvent(params.EventLogWriter, formatAcpUpdate(update))

			evt := AgentEvent{
				Type:      classifyUpdate(update),
				Timestamp: time.Now().UTC(),
				SessionID: sessionID,
				Message:   summarizeUpdate(update),
				Usage:     update.Usage,
			}
			eventCh <- OrchestratorEvent{
				Type:    EventAgentUpdate,
				IssueID: params.Issue.ID,
				Payload: evt,
			}
		}

		// Run turn
		if params.EventLogWriter != nil {
			logAnnotation(params.EventLogWriter, fmt.Sprintf("Starting turn %d of %d", turnNumber, maxTurns))
		}

		result, err := client.SessionPrompt(sessionID, []ContentBlock{
			{Type: "text", Text: turnPrompt},
		}, turnTimeout, updateHandler)
		if err != nil {
			if params.EventLogWriter != nil {
				logAnnotation(params.EventLogWriter, fmt.Sprintf("%sTurn %d failed%s — %s", cRed, turnNumber, cReset, err.Error()))
			}
			// Emit turn_failed
			eventCh <- OrchestratorEvent{
				Type:    EventAgentUpdate,
				IssueID: params.Issue.ID,
				Payload: AgentEvent{
					Type:      EventTurnFailed,
					Timestamp: time.Now().UTC(),
					SessionID: sessionID,
					Message:   err.Error(),
				},
			}
			return fmt.Errorf("turn %d failed: %w", turnNumber, err)
		}

		logger.Info("turn completed", "turn", turnNumber, "stop_reason", result.StopReason)
		if params.EventLogWriter != nil {
			logAnnotation(params.EventLogWriter, fmt.Sprintf("%sTurn %d completed%s — %s", cGreen, turnNumber, cReset, result.StopReason))
		}

		// Emit turn_completed
		eventCh <- OrchestratorEvent{
			Type:    EventAgentUpdate,
			IssueID: params.Issue.ID,
			Payload: AgentEvent{
				Type:      EventTurnCompleted,
				Timestamp: time.Now().UTC(),
				SessionID: sessionID,
				Message:   result.StopReason,
			},
		}

		// Check stop reasons that mean we shouldn't continue
		if result.StopReason == "refusal" || result.StopReason == "cancelled" {
			logger.Warn("turn ended with non-continuable reason", "stop_reason", result.StopReason)
			break
		}

		// Last turn — don't check state
		if turnNumber >= maxTurns {
			logger.Info("reached max turns")
			break
		}

		// Re-check issue state before continuing
		if params.CheckIssueState != nil {
			state, err := params.CheckIssueState(ctx, params.Issue.ID)
			if err != nil {
				return fmt.Errorf("issue state check failed: %w", err)
			}
			if !isActiveState(state, params) {
				logger.Info("issue no longer active, ending turn loop", "state", state)
				break
			}
		}
	}

	return nil
}

// buildTurnPrompt creates the prompt for a specific turn.
func buildTurnPrompt(wf *workflow.WorkflowDefinition, issue *tracker.Issue, attempt *int, turnNumber int, maxTurns int) (string, error) {
	if turnNumber == 1 {
		return prompt.RenderPrompt(wf.PromptTemplate, issue, attempt)
	}

	// Continuation turns
	return fmt.Sprintf(
		"Continue working on this issue. You are on turn %d of %d.\n"+
			"Check the current state of your work and continue from where you left off.\n"+
			"The issue is still in an active state in the tracker.",
		turnNumber, maxTurns,
	), nil
}

func isActiveState(state string, params RunParams) bool {
	if state == "" {
		return false
	}
	lowerState := strings.ToLower(state)
	for _, s := range params.ActiveStates {
		if strings.ToLower(s) == lowerState {
			return true
		}
	}
	return false
}

func classifyUpdate(update *SessionUpdateParams) string {
	switch update.Update.SessionUpdate {
	case "tool_call", "tool_call_update":
		return EventToolCall
	case "message_chunk":
		return EventNotification
	default:
		return EventNotification
	}
}

func summarizeUpdate(update *SessionUpdateParams) string {
	switch update.Update.SessionUpdate {
	case "tool_call":
		return fmt.Sprintf("tool_call: %s (%s)", update.Update.Title, update.Update.Status)
	case "tool_call_update":
		return fmt.Sprintf("tool_call_update: %s", update.Update.Status)
	case "message_chunk":
		text := update.Update.Text
		if len(text) > 100 {
			text = text[:100] + "..."
		}
		return text
	default:
		return update.Update.SessionUpdate
	}
}

// ANSI color codes for terminal output.
const (
	cReset  = "\033[0m"
	cDim    = "\033[2m"
	cBold   = "\033[1m"
	cRed    = "\033[31m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cBlue   = "\033[34m"
	cCyan   = "\033[36m"
	cGray   = "\033[90m"
)

// formatAcpUpdate returns a human-readable one-line summary of an ACP session update.
func formatAcpUpdate(update *SessionUpdateParams) string {
	u := update.Update
	switch u.SessionUpdate {
	case "tool_call":
		status := colorStatus(u.Status)
		return fmt.Sprintf("%sTOOL%s   %s  %s", cYellow, cReset, u.Title, status)
	case "tool_call_update":
		text := extractContentText(u.Content)
		if u.Status == "failed" {
			msg := u.Title
			if text != "" {
				msg = text
			}
			return fmt.Sprintf("%sFAIL%s   %s", cRed, cReset, truncate(msg, 120))
		}
		if text != "" {
			return fmt.Sprintf("       %s%s%s", cDim, truncate(text, 120), cReset)
		}
		return fmt.Sprintf("       %s  %s", u.Title, colorStatus(u.Status))
	case "message_chunk":
		if u.Role == "thought" {
			return fmt.Sprintf("%sTHINK%s  %s%s%s", cGray, cReset, cDim, truncate(u.Text, 150), cReset)
		}
		return fmt.Sprintf("%sAGENT%s  %s", cCyan, cReset, truncate(u.Text, 150))
	case "agent_thought_chunk":
		return fmt.Sprintf("%sTHINK%s  %s%s%s", cGray, cReset, cDim, truncate(u.Text, 150), cReset)
	case "agent_message_chunk":
		return fmt.Sprintf("%sAGENT%s  %s", cCyan, cReset, truncate(u.Text, 150))
	default:
		return fmt.Sprintf("%sEVENT%s  %s", cGray, cReset, u.SessionUpdate)
	}
}

// logAnnotation writes a colored annotation line to the event log writer.
func logAnnotation(w io.Writer, message string) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, " %s%s%s  %s%s── %s ──%s\n",
		cGray, time.Now().Format("15:04:05"), cReset, cBold, cBlue, message, cReset)
}

// logEvent writes a colored event line to the event log writer.
func logEvent(w io.Writer, formatted string) {
	if w == nil {
		return
	}
	fmt.Fprintf(w, " %s%s%s  %s\n", cGray, time.Now().Format("15:04:05"), cReset, formatted)
}

func colorStatus(status string) string {
	switch status {
	case "completed":
		return fmt.Sprintf("%s%s%s", cGreen, status, cReset)
	case "failed":
		return fmt.Sprintf("%s%s%s", cRed, status, cReset)
	case "in_progress":
		return fmt.Sprintf("%s%s%s", cYellow, status, cReset)
	default:
		return status
	}
}

func truncate(s string, max int) string {
	// Remove newlines for single-line display
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max] + "..."
	}
	return s
}

func extractContentText(content []byte) string {
	if len(content) == 0 {
		return ""
	}
	// Content is a JSON array of content blocks; extract text simply
	var blocks []struct {
		Type    string `json:"type"`
		Content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	for _, b := range blocks {
		if b.Content.Text != "" {
			return b.Content.Text
		}
	}
	return ""
}
