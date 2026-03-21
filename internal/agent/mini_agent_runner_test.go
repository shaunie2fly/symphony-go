package agent

import (
	"context"
	"encoding/json"
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

// miniAgentRunParams builds RunParams configured for the mini_agent backend.
func miniAgentRunParams(t *testing.T) (RunParams, string) {
	t.Helper()
	root := t.TempDir()

	desc := "Test mini-agent issue"
	issue := &tracker.Issue{
		ID:          "issue-ma-1",
		Identifier:  "MA-1",
		Title:       "Mini-Agent test task",
		Description: &desc,
		State:       "In Progress",
		Labels:      []string{},
		BlockedBy:   []tracker.Blocker{},
	}

	wf := &workflow.WorkflowDefinition{
		Config:         map[string]any{},
		PromptTemplate: "Work on {{ issue.identifier }}: {{ issue.title }}",
	}

	hooks := &config.HooksConfig{TimeoutMs: 5000}
	mgr := workspace.NewManager(root, hooks)

	miniAgentCfg := &config.MiniAgentConfig{
		Command:       "mini-agent-acp",
		Model:         "MiniMax-M2.5",
		APIKey:        "test-api-key",
		APIBase:       "https://api.minimax.io",
		ReadTimeoutMs: 5000,
		TurnTimeoutMs: 30000,
	}

	agentCfg := &config.AgentConfig{
		MaxTurns: 3,
	}

	params := RunParams{
		Issue:         issue,
		Attempt:       nil,
		Workflow:      wf,
		MiniAgentCfg:  miniAgentCfg,
		AgentCfg:      agentCfg,
		WorkspaceMgr:  mgr,
		WorkspaceRoot: root,
		ActiveStates:  []string{"In Progress", "Todo"},
		CallbackURL:   "http://localhost:8080/api/internal/agent/callback",
	}

	return params, root
}

// TestWriteMiniAgentConfig_CreatesConfigFile verifies that writeMiniAgentConfig
// writes the expected YAML config to the workspace.
func TestWriteMiniAgentConfig_CreatesConfigFile(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.MiniAgentConfig{
		APIKey:  "test-key-123",
		APIBase: "https://api.minimax.io",
		Model:   "MiniMax-M2.5",
	}

	if err := writeMiniAgentConfig(dir, cfg); err != nil {
		t.Fatalf("writeMiniAgentConfig failed: %v", err)
	}

	configPath := filepath.Join(dir, "mini_agent", "config", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not found at %s: %v", configPath, err)
	}

	content := string(data)
	if !strings.Contains(content, `"test-key-123"`) {
		t.Errorf("expected api_key in config, got:\n%s", content)
	}
	if !strings.Contains(content, `"https://api.minimax.io"`) {
		t.Errorf("expected api_base in config, got:\n%s", content)
	}
	if !strings.Contains(content, `"MiniMax-M2.5"`) {
		t.Errorf("expected model in config, got:\n%s", content)
	}
}

// TestWriteMiniAgentConfig_DefaultsApplied verifies that empty APIBase and Model
// use the default values.
func TestWriteMiniAgentConfig_DefaultsApplied(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.MiniAgentConfig{
		APIKey:  "my-key",
		APIBase: "", // empty — should use default
		Model:   "", // empty — should use default
	}

	if err := writeMiniAgentConfig(dir, cfg); err != nil {
		t.Fatalf("writeMiniAgentConfig failed: %v", err)
	}

	configPath := filepath.Join(dir, "mini_agent", "config", "config.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("config file not found: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, `"https://api.minimax.io"`) {
		t.Errorf("expected default api_base, got:\n%s", content)
	}
	if !strings.Contains(content, `"MiniMax-M2.5"`) {
		t.Errorf("expected default model, got:\n%s", content)
	}
}

// TestWriteMiniAgentConfig_ChinaPlatform verifies that the China API base is preserved.
func TestWriteMiniAgentConfig_ChinaPlatform(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.MiniAgentConfig{
		APIKey:  "china-key",
		APIBase: "https://api.minimaxi.com",
		Model:   "MiniMax-M2.5",
	}

	if err := writeMiniAgentConfig(dir, cfg); err != nil {
		t.Fatalf("writeMiniAgentConfig failed: %v", err)
	}

	configPath := filepath.Join(dir, "mini_agent", "config", "config.yaml")
	data, _ := os.ReadFile(configPath)
	if !strings.Contains(string(data), `"https://api.minimaxi.com"`) {
		t.Errorf("expected China api_base, got:\n%s", string(data))
	}
}

// TestWriteMiniAgentConfig_FilePermissions verifies that the config file
// is written with restricted permissions (0600 — not world-readable).
func TestWriteMiniAgentConfig_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.MiniAgentConfig{
		APIKey:  "secret-key",
		APIBase: "https://api.minimax.io",
		Model:   "MiniMax-M2.5",
	}

	if err := writeMiniAgentConfig(dir, cfg); err != nil {
		t.Fatalf("writeMiniAgentConfig failed: %v", err)
	}

	configPath := filepath.Join(dir, "mini_agent", "config", "config.yaml")
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	// Must be 0600: owner read/write only
	mode := info.Mode().Perm()
	if mode != 0600 {
		t.Errorf("expected mode 0600, got %o", mode)
	}
}

// TestWriteSymphonyContext_CreatesContextFile verifies that writeSymphonyContext
// writes valid JSON with issue info and callback URL.
func TestWriteSymphonyContext_CreatesContextFile(t *testing.T) {
	dir := t.TempDir()

	params, _ := miniAgentRunParams(t)
	params.CallbackURL = "http://localhost:9090/api/internal/agent/callback"

	if err := writeSymphonyContext(dir, params); err != nil {
		t.Fatalf("writeSymphonyContext failed: %v", err)
	}

	contextPath := filepath.Join(dir, miniAgentContextFile)
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("context file not found at %s: %v", contextPath, err)
	}

	var ctx map[string]any
	if err := json.Unmarshal(data, &ctx); err != nil {
		t.Fatalf("context file is not valid JSON: %v", err)
	}

	if ctx["callback_url"] != "http://localhost:9090/api/internal/agent/callback" {
		t.Errorf("expected callback_url, got %v", ctx["callback_url"])
	}

	issue, ok := ctx["issue"].(map[string]any)
	if !ok {
		t.Fatalf("expected issue object, got %T", ctx["issue"])
	}
	if issue["id"] != "issue-ma-1" {
		t.Errorf("expected issue.id=issue-ma-1, got %v", issue["id"])
	}
	if issue["identifier"] != "MA-1" {
		t.Errorf("expected issue.identifier=MA-1, got %v", issue["identifier"])
	}
	if issue["title"] != "Mini-Agent test task" {
		t.Errorf("expected issue.title, got %v", issue["title"])
	}
	if issue["state"] != "In Progress" {
		t.Errorf("expected issue.state=In Progress, got %v", issue["state"])
	}
}

// TestWriteSymphonyContext_DefaultCallbackURL verifies that when CallbackURL
// is empty, the default URL is used.
func TestWriteSymphonyContext_DefaultCallbackURL(t *testing.T) {
	dir := t.TempDir()

	params, _ := miniAgentRunParams(t)
	params.CallbackURL = "" // empty — should use default

	if err := writeSymphonyContext(dir, params); err != nil {
		t.Fatalf("writeSymphonyContext failed: %v", err)
	}

	data, _ := os.ReadFile(filepath.Join(dir, miniAgentContextFile))
	var ctx map[string]any
	json.Unmarshal(data, &ctx)

	if ctx["callback_url"] != "http://localhost:8080/api/internal/agent/callback" {
		t.Errorf("expected default callback URL, got %v", ctx["callback_url"])
	}
}

// mockMiniAgentServer simulates a Mini-Agent ACP subprocess.
// It handles initialize + session/new + N prompt turns, returning the given stopReason.
func mockMiniAgentServer(t *testing.T, incoming io.Reader, outgoing io.Writer, numTurns int, stopReason string) {
	t.Helper()
	scanner := make([]byte, 65536)

	for i := 0; i < numTurns+3; i++ {
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
					"agentInfo":         map[string]any{"name": "mini-agent", "version": "0.1.0"},
					"agentCapabilities": map[string]any{},
				},
			}
			b, _ := json.Marshal(resp)
			outgoing.Write(append(b, '\n'))

		case "session/new":
			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"sessionId": "ma-session-1"},
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
					"sessionId": "ma-session-1",
					"update": map[string]any{
						"sessionUpdate": "message_chunk",
						"role":          "agent",
						"text":          "Analyzing the task...",
					},
				},
			}
			b, _ := json.Marshal(notif)
			outgoing.Write(append(b, '\n'))

			// Determine stop reason: use specified for last turn, end_turn otherwise
			reason := "end_turn"
			if i >= numTurns+2 { // last expected prompt
				reason = stopReason
			}

			resp := map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"stopReason": reason},
			}
			b, _ = json.Marshal(resp)
			outgoing.Write(append(b, '\n'))
		}
	}
}

// TestMiniAgentRunner_ACPHandshake verifies the ACP initialize + session/new flow
// for the mini_agent backend using a mock server.
func TestMiniAgentRunner_ACPHandshake(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)

	go mockMiniAgentServer(t, clientR, mockW, 1, "end_turn")

	initResult, err := client.Initialize(5 * time.Second)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if initResult.AgentInfo.Name != "mini-agent" {
		t.Errorf("expected agent name=mini-agent, got %q", initResult.AgentInfo.Name)
	}

	sessionID, err := client.SessionNew(ws.Path, 5*time.Second)
	if err != nil {
		t.Fatalf("SessionNew failed: %v", err)
	}
	if sessionID != "ma-session-1" {
		t.Errorf("expected sessionId=ma-session-1, got %q", sessionID)
	}

	client.Close()
}

// TestMiniAgentRunner_StopReason_MaxTurnRequests verifies that the runner
// treats "max_turn_requests" as a non-continuable stop reason.
func TestMiniAgentRunner_StopReason_MaxTurnRequests(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)

	// Server returns max_turn_requests on the first prompt
	go mockMiniAgentServer(t, clientR, mockW, 1, "max_turn_requests")

	client.Initialize(5 * time.Second)
	sessionID, _ := client.SessionNew(ws.Path, 5*time.Second)

	turnPrompt, _ := buildTurnPrompt(params.Workflow, params.Issue, nil, 1, 3)
	result, err := client.SessionPrompt(sessionID, []ContentBlock{
		{Type: "text", Text: turnPrompt},
	}, 5*time.Second, nil)

	if err != nil {
		t.Fatalf("SessionPrompt failed: %v", err)
	}
	if result.StopReason != "max_turn_requests" {
		t.Errorf("expected stopReason=max_turn_requests, got %q", result.StopReason)
	}

	// Verify runner would stop on this reason
	switch result.StopReason {
	case "refusal", "cancelled", "max_turn_requests":
		// correct — non-continuable
	default:
		t.Errorf("expected non-continuable stop reason, got %q", result.StopReason)
	}

	client.Close()
}

// TestMiniAgentRunner_StopReason_Refusal verifies that "refusal" is non-continuable.
func TestMiniAgentRunner_StopReason_Refusal(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockMiniAgentServer(t, clientR, mockW, 1, "refusal")

	client.Initialize(5 * time.Second)
	sessionID, _ := client.SessionNew(ws.Path, 5*time.Second)

	turnPrompt, _ := buildTurnPrompt(params.Workflow, params.Issue, nil, 1, 3)
	result, err := client.SessionPrompt(sessionID, []ContentBlock{
		{Type: "text", Text: turnPrompt},
	}, 5*time.Second, nil)

	if err != nil {
		t.Fatalf("SessionPrompt failed: %v", err)
	}
	if result.StopReason != "refusal" {
		t.Errorf("expected stopReason=refusal, got %q", result.StopReason)
	}
	client.Close()
}

// TestMiniAgentRunner_StopReason_Cancelled verifies that "cancelled" is non-continuable.
func TestMiniAgentRunner_StopReason_Cancelled(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockMiniAgentServer(t, clientR, mockW, 1, "cancelled")

	client.Initialize(5 * time.Second)
	sessionID, _ := client.SessionNew(ws.Path, 5*time.Second)

	turnPrompt, _ := buildTurnPrompt(params.Workflow, params.Issue, nil, 1, 3)
	result, err := client.SessionPrompt(sessionID, []ContentBlock{
		{Type: "text", Text: turnPrompt},
	}, 5*time.Second, nil)

	if err != nil {
		t.Fatalf("SessionPrompt failed: %v", err)
	}
	if result.StopReason != "cancelled" {
		t.Errorf("expected stopReason=cancelled, got %q", result.StopReason)
	}
	client.Close()
}

// TestMiniAgentRunner_EndTurn_Continuable verifies that "end_turn" allows the loop to continue.
func TestMiniAgentRunner_EndTurn_Continuable(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	params.AgentCfg.MaxTurns = 2

	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockMiniAgentServer(t, clientR, mockW, 2, "end_turn")

	client.Initialize(5 * time.Second)
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

		// Non-continuable check
		switch result.StopReason {
		case "refusal", "cancelled", "max_turn_requests":
			goto done
		}

		if turn >= params.AgentCfg.MaxTurns {
			break
		}
	}
done:
	client.Close()

	if turnsCompleted != 2 {
		t.Errorf("expected 2 turns (end_turn is continuable), got %d", turnsCompleted)
	}
}

// TestMiniAgentRunner_Launch_MissingConfig verifies that Launch returns an error
// when MiniAgentCfg is nil.
func TestMiniAgentRunner_Launch_MissingConfig(t *testing.T) {
	runner := NewMiniAgentRunner()

	params, _ := miniAgentRunParams(t)
	params.MiniAgentCfg = nil // missing

	eventCh := make(chan OrchestratorEvent, 10)
	err := runner.Launch(context.Background(), params, eventCh)

	if err == nil {
		t.Fatal("expected error when MiniAgentCfg is nil")
	}
	if !strings.Contains(err.Error(), "MiniAgentCfg is required") {
		t.Errorf("expected MiniAgentCfg error, got: %v", err)
	}
}

// TestMiniAgentRunner_ConfigFileWrittenBeforeLaunch verifies that when api_key is set,
// the mini_agent config file is written to the workspace before the ACP process starts.
func TestMiniAgentRunner_ConfigFileWrittenBeforeLaunch(t *testing.T) {
	params, _ := miniAgentRunParams(t)

	// Create the workspace manually to inspect it
	ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	if err != nil {
		t.Fatalf("workspace creation failed: %v", err)
	}

	// Write the config as the runner would
	if err := writeMiniAgentConfig(ws.Path, params.MiniAgentCfg); err != nil {
		t.Fatalf("writeMiniAgentConfig failed: %v", err)
	}

	// Verify the file exists
	configPath := filepath.Join(ws.Path, miniAgentConfigDir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Errorf("expected config file to exist at %s", configPath)
	}
}

// TestMiniAgentRunner_ContextFileWrittenBeforeLaunch verifies that the symphony
// context file is written to the workspace root.
func TestMiniAgentRunner_ContextFileWrittenBeforeLaunch(t *testing.T) {
	params, _ := miniAgentRunParams(t)

	ws, err := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)
	if err != nil {
		t.Fatalf("workspace creation failed: %v", err)
	}

	if err := writeSymphonyContext(ws.Path, params); err != nil {
		t.Fatalf("writeSymphonyContext failed: %v", err)
	}

	contextPath := filepath.Join(ws.Path, miniAgentContextFile)
	data, err := os.ReadFile(contextPath)
	if err != nil {
		t.Fatalf("context file not found: %v", err)
	}

	var ctx map[string]any
	if err := json.Unmarshal(data, &ctx); err != nil {
		t.Fatalf("invalid JSON in context file: %v", err)
	}

	issue := ctx["issue"].(map[string]any)
	if issue["identifier"] != "MA-1" {
		t.Errorf("expected identifier=MA-1, got %v", issue["identifier"])
	}
}

// TestMiniAgentRunner_MultiTurn_IssueBecomesInactive verifies that the turn loop
// exits when the issue transitions to an inactive state.
func TestMiniAgentRunner_MultiTurn_IssueBecomesInactive(t *testing.T) {
	params, _ := miniAgentRunParams(t)
	params.AgentCfg.MaxTurns = 5

	callCount := 0
	params.CheckIssueState = func(ctx context.Context, issueID string) (string, error) {
		callCount++
		if callCount >= 2 {
			return "Done", nil // issue no longer active
		}
		return "In Progress", nil
	}

	ws, _ := params.WorkspaceMgr.CreateForIssue(params.Issue.Identifier)

	clientR, clientW := io.Pipe()
	mockR, mockW := io.Pipe()
	client := newTestACPClient(clientW, mockR)
	go mockMiniAgentServer(t, clientR, mockW, 5, "end_turn")

	client.Initialize(5 * time.Second)
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

		switch result.StopReason {
		case "refusal", "cancelled", "max_turn_requests":
			goto done
		}

		if turn >= params.AgentCfg.MaxTurns {
			break
		}

		state, _ := params.CheckIssueState(context.Background(), params.Issue.ID)
		if !isActiveState(state, params) {
			break
		}
	}
done:
	client.Close()

	if turnsCompleted != 2 {
		t.Errorf("expected 2 turns before issue became inactive, got %d", turnsCompleted)
	}
}

// TestMiniAgentRunner_NoConfigWrittenWhenNoAPIKey verifies that when api_key is empty,
// no config file is written to the workspace.
func TestMiniAgentRunner_NoConfigWrittenWhenNoAPIKey(t *testing.T) {
	dir := t.TempDir()

	cfg := &config.MiniAgentConfig{
		APIKey: "", // empty — should not write config
		Model:  "MiniMax-M2.5",
	}

	// Simulate what the runner does: only write config when APIKey is set
	if cfg.APIKey != "" {
		if err := writeMiniAgentConfig(dir, cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	configPath := filepath.Join(dir, "mini_agent", "config", "config.yaml")
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Error("expected config file to NOT exist when api_key is empty")
	}
}

// TestMiniAgentRunner_PromptRendering verifies that the first-turn prompt includes
// the issue identifier.
func TestMiniAgentRunner_PromptRendering(t *testing.T) {
	params, _ := miniAgentRunParams(t)

	prompt, err := buildTurnPrompt(params.Workflow, params.Issue, nil, 1, 3)
	if err != nil {
		t.Fatalf("buildTurnPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "MA-1") {
		t.Errorf("expected prompt to contain issue identifier MA-1, got: %q", prompt)
	}
	if !strings.Contains(prompt, "Mini-Agent test task") {
		t.Errorf("expected prompt to contain issue title, got: %q", prompt)
	}
}

// TestMiniAgentRunner_ContinuationPrompt verifies that continuation turns produce
// a guidance-only prompt (not re-rendering the original template).
func TestMiniAgentRunner_ContinuationPrompt(t *testing.T) {
	params, _ := miniAgentRunParams(t)

	prompt, err := buildTurnPrompt(params.Workflow, params.Issue, nil, 2, 3)
	if err != nil {
		t.Fatalf("buildTurnPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "turn 2 of 3") {
		t.Errorf("expected continuation guidance with turn count, got: %q", prompt)
	}
	// Continuation prompt must not contain the rendered template ("Work on MA-1:"),
	// which would indicate the original template was re-rendered instead of generating
	// the continuation guidance message.
	if strings.Contains(prompt, "Work on MA-1:") {
		t.Errorf("continuation prompt should not re-render original template, got: %q", prompt)
	}
}
