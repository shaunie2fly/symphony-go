package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/workspace"
)

// MiniAgentRunner implements AgentLauncher using Mini-Agent in ACP mode.
// Mini-Agent (https://github.com/MiniMax-AI/Mini-Agent) is an ACP-compatible
// agent powered by the MiniMax M2.5 model. It speaks the same JSON-RPC 2.0
// over stdio protocol as Gemini CLI.
//
// Setup:
//
//	uv tool install git+https://github.com/MiniMax-AI/Mini-Agent.git
//
// This installs the "mini-agent-acp" entry point used by this runner.
type MiniAgentRunner struct{}

// NewMiniAgentRunner creates a new MiniAgentRunner.
func NewMiniAgentRunner() *MiniAgentRunner {
	return &MiniAgentRunner{}
}

// miniAgentContextFile is the filename written to the workspace root so that
// Mini-Agent's SymphonyStatusUpdateTool can discover the callback URL.
const miniAgentContextFile = ".symphony-context.json"

// miniAgentConfigDir is the relative path Mini-Agent searches first for its
// config file when the process working directory is the workspace.
const miniAgentConfigDir = "mini_agent/config"

// Launch runs a full Mini-Agent attempt: workspace → optional config setup →
// ACP session → turn loop.
func (r *MiniAgentRunner) Launch(ctx context.Context, params RunParams, eventCh chan<- OrchestratorEvent) error {
	if params.MiniAgentCfg == nil {
		return fmt.Errorf("MiniAgentCfg is required for mini_agent backend")
	}

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

	// 4. Write Mini-Agent config to the workspace when api_key is configured.
	//    Mini-Agent searches for mini_agent/config/config.yaml relative to
	//    its working directory (the workspace) before checking ~/.mini-agent/.
	//    Writing the file here lets Symphony manage credentials per-workflow.
	if params.MiniAgentCfg.APIKey != "" {
		if err := writeMiniAgentConfig(ws.Path, params.MiniAgentCfg); err != nil {
			params.WorkspaceMgr.RunAfterRun(ws.Path)
			return fmt.Errorf("failed to write mini-agent config: %w", err)
		}
	}

	// 5. Write Symphony context file so Mini-Agent's SymphonyStatusUpdateTool
	//    can POST completion callbacks and the system prompt picks up the issue.
	if err := writeSymphonyContext(ws.Path, params); err != nil {
		logger.Warn("failed to write symphony context file", "error", err)
		// Non-fatal: context file is optional; Mini-Agent still works without it.
	}

	// 6. Launch ACP client
	logger.Info("launching Mini-Agent ACP server", "command", params.MiniAgentCfg.Command)
	client, err := NewACPClient(params.MiniAgentCfg.Command, ws.Path, params.ExtraEnv)
	if err != nil {
		params.WorkspaceMgr.RunAfterRun(ws.Path)
		return fmt.Errorf("failed to launch ACP subprocess: %w", err)
	}
	defer func() {
		client.Close()
		params.WorkspaceMgr.RunAfterRun(ws.Path)
	}()

	readTimeout := time.Duration(params.MiniAgentCfg.ReadTimeoutMs) * time.Millisecond
	turnTimeout := time.Duration(params.MiniAgentCfg.TurnTimeoutMs) * time.Millisecond

	// 7. ACP handshake
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

	// 8. Turn loop
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

		// Non-continuable stop reasons:
		//   "refusal"           — model refused to continue
		//   "cancelled"         — session was cancelled by the orchestrator
		//   "max_turn_requests" — Mini-Agent hit its internal max_steps limit
		switch result.StopReason {
		case "refusal", "cancelled", "max_turn_requests":
			logger.Warn("turn ended with non-continuable reason", "stop_reason", result.StopReason)
			return nil
		}

		// Last turn — don't check state
		if turnNumber >= maxTurns {
			logger.Info("reached max turns")
			break
		}

		// Re-check issue state before continuing to the next turn
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

// writeMiniAgentConfig writes a minimal Mini-Agent config.yaml into the
// workspace at <workspace>/mini_agent/config/config.yaml — the highest-priority
// search location when mini-agent-acp is launched with the workspace as cwd.
func writeMiniAgentConfig(workspacePath string, cfg *config.MiniAgentConfig) error {
	configDir := filepath.Join(workspacePath, miniAgentConfigDir)
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create mini-agent config dir: %w", err)
	}

	apiBase := cfg.APIBase
	if apiBase == "" {
		apiBase = "https://api.minimax.io"
	}
	model := cfg.Model
	if model == "" {
		model = "MiniMax-M2.5"
	}

	// Mini-Agent's Config.from_yaml() reads top-level api_key, api_base, model.
	content := fmt.Sprintf("api_key: %q\napi_base: %q\nmodel: %q\n", cfg.APIKey, apiBase, model)
	configPath := filepath.Join(configDir, "config.yaml")
	return os.WriteFile(configPath, []byte(content), 0600)
}

// writeSymphonyContext writes a .symphony-context.json to the workspace root.
// Mini-Agent's SymphonyStatusUpdateTool reads this file (via load_symphony_context)
// to find the callback URL and to inject the issue identifier into its prompt.
func writeSymphonyContext(workspacePath string, params RunParams) error {
	callbackURL := params.CallbackURL
	if callbackURL == "" {
		callbackURL = "http://localhost:8080/api/internal/agent/callback"
	}
	ctx := map[string]any{
		"issue": map[string]any{
			"id":         params.Issue.ID,
			"identifier": params.Issue.Identifier,
			"title":      params.Issue.Title,
			"state":      params.Issue.State,
		},
		"callback_url": callbackURL,
	}
	data, err := json.Marshal(ctx)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workspacePath, miniAgentContextFile), data, 0644)
}
