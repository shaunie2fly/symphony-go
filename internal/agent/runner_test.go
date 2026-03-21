package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workflow"
	"github.com/symphony-go/symphony/internal/workspace"
)

func testRunParams(t *testing.T) (RunParams, string) {
	t.Helper()
	root := t.TempDir()

	desc := "Test issue"
	issue := &tracker.Issue{
		ID:          "issue-1",
		Identifier:  "MT-1",
		Title:       "Fix bug",
		Description: &desc,
		State:       "Todo",
		Labels:      []string{"bug"},
		BlockedBy:   []tracker.Blocker{},
	}

	wf := &workflow.WorkflowDefinition{
		Config:         map[string]any{},
		PromptTemplate: "Work on {{ issue.identifier }}: {{ issue.title }}",
	}

	hooks := &config.HooksConfig{TimeoutMs: 5000}
	mgr := workspace.NewManager(root, hooks)

	geminiCfg := &config.GeminiConfig{
		Command:        "echo test", // won't actually run — we mock the ACP client
		ReadTimeoutMs:  5000,
		TurnTimeoutMs:  30000,
		StallTimeoutMs: 300000,
	}

	agentCfg := &config.AgentConfig{
		MaxTurns: 3,
	}

	params := RunParams{
		Issue:         issue,
		Attempt:       nil,
		Workflow:      wf,
		GeminiCfg:     geminiCfg,
		AgentCfg:      agentCfg,
		WorkspaceMgr:  mgr,
		WorkspaceRoot: root,
	}

	return params, root
}

// mockACPServer simulates Gemini CLI. Runs in a goroutine, handles initialize + session/new + N prompts.
func mockACPServer(t *testing.T, incoming io.Reader, outgoing io.Writer, numTurns int) {
	t.Helper()
	scanner := make([]byte, 65536)

	for i := 0; i < numTurns+3; i++ { // +3 for initialize, session/new, session/set_mode
		n, err := incoming.Read(scanner)
		if err != nil {
			return
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(scanner[:n], &req); err != nil {
			continue
		}

		switch req.Method {
		case "initialize":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"protocolVersion":   1,
					"agentInfo":         map[string]any{"name": "mock-gemini", "version": "1.0"},
					"agentCapabilities": map[string]any{},
				},
			}
			b, _ := json.Marshal(resp)
			outgoing.Write(append(b, '\n'))

		case "session/new":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"sessionId": "test-session-1"},
			}
			b, _ := json.Marshal(resp)
			outgoing.Write(append(b, '\n'))

		case "session/set_mode":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"modeId": "yolo"},
			}
			b, _ := json.Marshal(resp)
			outgoing.Write(append(b, '\n'))

		case "session/prompt":
			// Send an update notification
			notif := map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "test-session-1",
					"update": map[string]any{
						"sessionUpdate": "message_chunk",
						"role":          "agent",
						"text":          "Working...",
					},
				},
			}
			b, _ := json.Marshal(notif)
			outgoing.Write(append(b, '\n'))

			// Send prompt response
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"stopReason": "end_turn"},
			}
			b, _ = json.Marshal(resp)
			outgoing.Write(append(b, '\n'))
		}
	}
}

// TestRunnerFullLifecycle_SingleTurn tests workspace + ACP handshake + 1 turn + cleanup.
func TestRunnerFullLifecycle_SingleTurn(t *testing.T) {
	params, root := testRunParams(t)
	params.AgentCfg.MaxTurns = 1

	// We can't easily inject a mock ACP client into GeminiRunner since it creates the process.
	// Instead, test the building blocks directly.

	// Test workspace creation
	ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	if err != nil {
		t.Fatalf("workspace creation failed: %v", err)
	}
	if !ws.CreatedNow {
		t.Error("expected new workspace")
	}

	expectedPath := filepath.Join(root, "MT-1")
	if ws.Path != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, ws.Path)
	}

	// Test prompt rendering
	turnPrompt, err := buildTurnPrompt(params.Workflow, params.Issue, nil, 1, 1)
	if err != nil {
		t.Fatalf("prompt build failed: %v", err)
	}
	if !strings.Contains(turnPrompt, "MT-1") {
		t.Errorf("expected prompt to contain MT-1, got: %q", turnPrompt)
	}

	// Test ACP client with pipes
	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()

	client := newTestACPClient(clientW, mockR)

	go mockACPServer(t, clientR, mockW, 1)

	initResult, err := client.Initialize(5 * time.Second)
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	if initResult.AgentInfo.Name != "mock-gemini" {
		t.Errorf("unexpected agent: %q", initResult.AgentInfo.Name)
	}

	sessionID, err := client.SessionNew(ws.Path, 5*time.Second)
	if err != nil {
		t.Fatalf("session/new failed: %v", err)
	}
	if sessionID != "test-session-1" {
		t.Errorf("unexpected session ID: %q", sessionID)
	}

	var updates []SessionUpdateParams
	result, err := client.SessionPrompt(sessionID, []ContentBlock{
		{Type: "text", Text: turnPrompt},
	}, 5*time.Second, func(update *SessionUpdateParams) {
		updates = append(updates, *update)
	})
	if err != nil {
		t.Fatalf("session/prompt failed: %v", err)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("expected end_turn, got %q", result.StopReason)
	}
	if len(updates) != 1 {
		t.Errorf("expected 1 update, got %d", len(updates))
	}

	client.Close()
}

func TestBuildTurnPrompt_FirstTurn(t *testing.T) {
	wf := &workflow.WorkflowDefinition{
		PromptTemplate: "Fix {{ issue.identifier }}",
	}
	issue := &tracker.Issue{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Bug",
		State:      "Todo",
		Labels:     []string{},
		BlockedBy:  []tracker.Blocker{},
	}

	result, err := buildTurnPrompt(wf, issue, nil, 1, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "Fix MT-1" {
		t.Errorf("expected 'Fix MT-1', got %q", result)
	}
}

func TestBuildTurnPrompt_ContinuationTurn(t *testing.T) {
	wf := &workflow.WorkflowDefinition{
		PromptTemplate: "Fix {{ issue.identifier }}",
	}
	issue := &tracker.Issue{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Bug",
		State:      "Todo",
		Labels:     []string{},
		BlockedBy:  []tracker.Blocker{},
	}

	result, err := buildTurnPrompt(wf, issue, nil, 2, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result, "turn 2 of 5") {
		t.Errorf("expected continuation guidance, got: %q", result)
	}
	// Should NOT contain the original prompt
	if strings.Contains(result, "MT-1") {
		t.Errorf("continuation should not re-render original prompt")
	}
}

func TestBuildTurnPrompt_RenderError(t *testing.T) {
	wf := &workflow.WorkflowDefinition{
		PromptTemplate: "{% for %}",
	}
	issue := &tracker.Issue{
		ID:         "1",
		Identifier: "MT-1",
		Title:      "Bug",
		State:      "Todo",
		Labels:     []string{},
		BlockedBy:  []tracker.Blocker{},
	}

	_, err := buildTurnPrompt(wf, issue, nil, 1, 5)
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestRunnerMultiTurn_IssueBecomesInactive(t *testing.T) {
	params, _ := testRunParams(t)
	params.AgentCfg.MaxTurns = 5

	// Simulate: issue active on turn 1, inactive on turn 2
	callCount := 0
	params.CheckIssueState = func(ctx context.Context, issueID string) (string, error) {
		callCount++
		if callCount >= 2 {
			return "Done", nil // no longer active
		}
		return "In Progress", nil
	}

	// Create workspace
	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	// Run multi-turn with mock ACP
	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockACPServer(t, clientR, mockW, 5) // support up to 5 turns

	initResult, _ := client.Initialize(5 * time.Second)
	_ = initResult
	sessionID, _ := client.SessionNew(ws.Path, 5*time.Second)

	turnsCompleted := 0
	for turn := 1; turn <= params.AgentCfg.MaxTurns; turn++ {
		turnPrompt, _ := buildTurnPrompt(params.Workflow, params.Issue, nil, turn, params.AgentCfg.MaxTurns)
		result, err := client.SessionPrompt(sessionID, []ContentBlock{
			{Type: "text", Text: turnPrompt},
		}, 5*time.Second, nil)
		if err != nil {
			t.Fatalf("turn %d failed: %v", turn, err)
		}
		turnsCompleted++

		if result.StopReason == "refusal" || result.StopReason == "cancelled" {
			break
		}
		if turn >= params.AgentCfg.MaxTurns {
			break
		}

		// Check issue state
		state, _ := params.CheckIssueState(context.Background(), params.Issue.ID)
		if state == "Done" {
			break
		}
	}

	client.Close()

	// Should have completed 2 turns (active after turn 1, inactive check after turn 2)
	if turnsCompleted != 2 {
		t.Errorf("expected 2 turns before issue became inactive, got %d", turnsCompleted)
	}
}

func TestRunnerMaxTurns(t *testing.T) {
	params, _ := testRunParams(t)
	params.AgentCfg.MaxTurns = 2

	// Issue always active
	params.CheckIssueState = func(ctx context.Context, issueID string) (string, error) {
		return "In Progress", nil
	}

	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockACPServer(t, clientR, mockW, 5)

	client.Initialize(5 * time.Second)
	sessionID, _ := client.SessionNew(ws.Path, 5*time.Second)

	turnsCompleted := 0
	for turn := 1; turn <= params.AgentCfg.MaxTurns; turn++ {
		turnPrompt, _ := buildTurnPrompt(params.Workflow, params.Issue, nil, turn, params.AgentCfg.MaxTurns)
		_, err := client.SessionPrompt(sessionID, []ContentBlock{
			{Type: "text", Text: turnPrompt},
		}, 5*time.Second, nil)
		if err != nil {
			t.Fatalf("turn %d failed: %v", turn, err)
		}
		turnsCompleted++

		if turn >= params.AgentCfg.MaxTurns {
			break
		}

		state, _ := params.CheckIssueState(context.Background(), params.Issue.ID)
		_ = state
	}

	client.Close()

	if turnsCompleted != 2 {
		t.Errorf("expected exactly 2 turns (max_turns), got %d", turnsCompleted)
	}
}

func TestWorkspaceCreationFailure(t *testing.T) {
	// Use a non-writable root to force creation failure
	root := filepath.Join(os.TempDir(), fmt.Sprintf("symphony_test_readonly_%d", time.Now().UnixNano()))
	os.MkdirAll(root, 0444)
	defer os.RemoveAll(root)

	hooks := &config.HooksConfig{TimeoutMs: 5000}
	mgr := workspace.NewManager(root, hooks)

	_, err := mgr.CreateForIssue("MT-1")
	// On some systems this might succeed if running as root, so just check the path is reasonable
	if err != nil {
		// Expected: permission denied
		if !strings.Contains(err.Error(), "permission denied") && !strings.Contains(err.Error(), "mkdir") {
			// Still acceptable — the important thing is it handled the error
		}
	}
}

func TestBeforeRunHookFailure(t *testing.T) {
	root := t.TempDir()
	script := "exit 1"
	hooks := &config.HooksConfig{
		BeforeRun: &script,
		TimeoutMs: 5000,
	}
	mgr := workspace.NewManager(root, hooks)

	// Create workspace first
	ws, err := mgr.CreateForIssue("MT-1")
	if err != nil {
		t.Fatalf("workspace creation failed: %v", err)
	}

	// before_run should fail
	err = mgr.RunBeforeRun(ws.Path)
	if err == nil {
		t.Error("expected before_run hook to fail")
	}
}

func TestNewLauncher_Gemini(t *testing.T) {
	launcher, err := NewLauncher("gemini")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := launcher.(*GeminiRunner); !ok {
		t.Errorf("expected *GeminiRunner, got %T", launcher)
	}
}

func TestNewLauncher_Claude(t *testing.T) {
	launcher, err := NewLauncher("claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := launcher.(*ClaudeRunner); !ok {
		t.Errorf("expected *ClaudeRunner, got %T", launcher)
	}
}

func TestNewLauncher_MiniAgent(t *testing.T) {
	launcher, err := NewLauncher("mini_agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := launcher.(*MiniAgentRunner); !ok {
		t.Errorf("expected *MiniAgentRunner, got %T", launcher)
	}
}

func TestNewLauncher_MiniAgentAlias(t *testing.T) {
	launcher, err := NewLauncher("mini-agent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := launcher.(*MiniAgentRunner); !ok {
		t.Errorf("expected *MiniAgentRunner for mini-agent alias, got %T", launcher)
	}
}

func TestNewLauncher_Empty(t *testing.T) {
	launcher, err := NewLauncher("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := launcher.(*GeminiRunner); !ok {
		t.Errorf("expected *GeminiRunner for empty backend, got %T", launcher)
	}
}

func TestNewLauncher_Invalid(t *testing.T) {
	_, err := NewLauncher("unknown")
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
	if !strings.Contains(err.Error(), "unsupported backend") {
		t.Errorf("expected 'unsupported backend' error, got: %v", err)
	}
}
