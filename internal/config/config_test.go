package config

import (
	"strings"
	"testing"
)

func TestParseConfig_EmptyMap(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaults := DefaultConfig()

	if cfg.Tracker.Endpoint != defaults.Tracker.Endpoint {
		t.Errorf("expected default endpoint %q, got %q", defaults.Tracker.Endpoint, cfg.Tracker.Endpoint)
	}
	if cfg.Polling.IntervalMs != defaults.Polling.IntervalMs {
		t.Errorf("expected default poll interval %d, got %d", defaults.Polling.IntervalMs, cfg.Polling.IntervalMs)
	}
	if cfg.Agent.MaxConcurrentAgents != defaults.Agent.MaxConcurrentAgents {
		t.Errorf("expected default max_concurrent %d, got %d", defaults.Agent.MaxConcurrentAgents, cfg.Agent.MaxConcurrentAgents)
	}
	if cfg.Gemini.Command != defaults.Gemini.Command {
		t.Errorf("expected default gemini command %q, got %q", defaults.Gemini.Command, cfg.Gemini.Command)
	}
	if cfg.Gemini.Model != defaults.Gemini.Model {
		t.Errorf("expected default gemini model %q, got %q", defaults.Gemini.Model, cfg.Gemini.Model)
	}
	if cfg.Hooks.TimeoutMs != defaults.Hooks.TimeoutMs {
		t.Errorf("expected default hook timeout %d, got %d", defaults.Hooks.TimeoutMs, cfg.Hooks.TimeoutMs)
	}
}

func TestParseConfig_NilMap(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.TurnTimeoutMs != 3600000 {
		t.Errorf("expected default turn_timeout_ms, got %d", cfg.Gemini.TurnTimeoutMs)
	}
}

func TestParseConfig_OverrideValues(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":         "linear",
			"api_key":      "test-key",
			"project_slug": "my-proj",
		},
		"polling": map[string]any{
			"interval_ms": 60000,
		},
		"gemini": map[string]any{
			"command": "custom-gemini --acp",
			"model":   "gemini-2.0-flash",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Tracker.Kind != "linear" {
		t.Errorf("expected kind=linear, got %q", cfg.Tracker.Kind)
	}
	if cfg.Tracker.APIKey != "test-key" {
		t.Errorf("expected api_key=test-key, got %q", cfg.Tracker.APIKey)
	}
	if cfg.Polling.IntervalMs != 60000 {
		t.Errorf("expected interval_ms=60000, got %d", cfg.Polling.IntervalMs)
	}
	if cfg.Gemini.Command != "custom-gemini --acp" {
		t.Errorf("expected custom command, got %q", cfg.Gemini.Command)
	}
	if cfg.Gemini.Model != "gemini-2.0-flash" {
		t.Errorf("expected model override, got %q", cfg.Gemini.Model)
	}

	// Defaults preserved for unset fields
	if cfg.Tracker.Endpoint != "https://api.linear.app/graphql" {
		t.Errorf("expected default endpoint, got %q", cfg.Tracker.Endpoint)
	}
	if cfg.Agent.MaxConcurrentAgents != 10 {
		t.Errorf("expected default max_concurrent=10, got %d", cfg.Agent.MaxConcurrentAgents)
	}
}

func TestParseConfig_CodexAliasToGemini(t *testing.T) {
	raw := map[string]any{
		"codex": map[string]any{
			"command":         "my-codex-command",
			"turn_timeout_ms": 500000,
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.Command != "my-codex-command" {
		t.Errorf("expected codex alias to gemini.command, got %q", cfg.Gemini.Command)
	}
	if cfg.Gemini.TurnTimeoutMs != 500000 {
		t.Errorf("expected turn_timeout from codex alias, got %d", cfg.Gemini.TurnTimeoutMs)
	}
}

func TestParseConfig_GeminiTakesPrecedenceOverCodex(t *testing.T) {
	raw := map[string]any{
		"codex": map[string]any{
			"command": "codex-command",
		},
		"gemini": map[string]any{
			"command": "gemini-command",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Gemini.Command != "gemini-command" {
		t.Errorf("expected gemini to take precedence, got %q", cfg.Gemini.Command)
	}
}

func TestParseConfig_StringIntegerCoercion(t *testing.T) {
	// yaml.v3 should handle this, but verify with raw map
	raw := map[string]any{
		"polling": map[string]any{
			"interval_ms": 45000, // int in YAML
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Polling.IntervalMs != 45000 {
		t.Errorf("expected 45000, got %d", cfg.Polling.IntervalMs)
	}
}

func TestParseConfig_GeminiCommandPreservedAsShellString(t *testing.T) {
	raw := map[string]any{
		"gemini": map[string]any{
			"command": "gemini --experimental-acp --model gemini-3.1-pro-preview",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "gemini --experimental-acp --model gemini-3.1-pro-preview"
	if cfg.Gemini.Command != expected {
		t.Errorf("expected command preserved as shell string %q, got %q", expected, cfg.Gemini.Command)
	}
}

func TestParseConfig_BackendDefault(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend != "gemini" {
		t.Errorf("expected default backend %q, got %q", "gemini", cfg.Backend)
	}
}

func TestParseConfig_BackendClaude(t *testing.T) {
	raw := map[string]any{
		"backend": "claude",
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend != "claude" {
		t.Errorf("expected backend %q, got %q", "claude", cfg.Backend)
	}
}

func TestParseConfig_ClaudeDefaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defaults := DefaultConfig()

	if cfg.Claude.Command != defaults.Claude.Command {
		t.Errorf("expected default claude command %q, got %q", defaults.Claude.Command, cfg.Claude.Command)
	}
	if cfg.Claude.Model != defaults.Claude.Model {
		t.Errorf("expected default claude model %q, got %q", defaults.Claude.Model, cfg.Claude.Model)
	}
	if cfg.Claude.PermissionMode != defaults.Claude.PermissionMode {
		t.Errorf("expected default permission_mode %q, got %q", defaults.Claude.PermissionMode, cfg.Claude.PermissionMode)
	}
	if len(cfg.Claude.AllowedTools) != len(defaults.Claude.AllowedTools) {
		t.Errorf("expected %d allowed_tools, got %d", len(defaults.Claude.AllowedTools), len(cfg.Claude.AllowedTools))
	}
	if cfg.Claude.MaxTurns != defaults.Claude.MaxTurns {
		t.Errorf("expected default max_turns %d, got %d", defaults.Claude.MaxTurns, cfg.Claude.MaxTurns)
	}
	if cfg.Claude.TurnTimeoutMs != defaults.Claude.TurnTimeoutMs {
		t.Errorf("expected default turn_timeout_ms %d, got %d", defaults.Claude.TurnTimeoutMs, cfg.Claude.TurnTimeoutMs)
	}
	if cfg.Claude.StallTimeoutMs != defaults.Claude.StallTimeoutMs {
		t.Errorf("expected default stall_timeout_ms %d, got %d", defaults.Claude.StallTimeoutMs, cfg.Claude.StallTimeoutMs)
	}
}

func TestParseConfig_ClaudeOverrides(t *testing.T) {
	raw := map[string]any{
		"claude": map[string]any{
			"command": "claude --custom",
			"model":   "claude-opus-4-6",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.Command != "claude --custom" {
		t.Errorf("expected overridden command %q, got %q", "claude --custom", cfg.Claude.Command)
	}
	if cfg.Claude.Model != "claude-opus-4-6" {
		t.Errorf("expected overridden model %q, got %q", "claude-opus-4-6", cfg.Claude.Model)
	}
	// Defaults preserved for unset fields
	defaults := DefaultConfig()
	if cfg.Claude.PermissionMode != defaults.Claude.PermissionMode {
		t.Errorf("expected default permission_mode preserved, got %q", cfg.Claude.PermissionMode)
	}
	if cfg.Claude.MaxTurns != defaults.Claude.MaxTurns {
		t.Errorf("expected default max_turns preserved, got %d", cfg.Claude.MaxTurns)
	}
}

func TestParseConfig_ClaudeCodeAlias(t *testing.T) {
	raw := map[string]any{
		"claude_code": map[string]any{
			"command": "claude-code-bin",
			"model":   "claude-sonnet-4-6",
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Claude.Command != "claude-code-bin" {
		t.Errorf("expected claude_code alias to claude.command, got %q", cfg.Claude.Command)
	}
	if cfg.Claude.Model != "claude-sonnet-4-6" {
		t.Errorf("expected claude_code alias to claude.model, got %q", cfg.Claude.Model)
	}
}

func TestParseConfig_TrackerEmail(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":         "jira",
			"endpoint":     "https://mycompany.atlassian.net",
			"api_key":      "jira-token",
			"project_slug": "PROJ",
			"email":        "user@example.com",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Tracker.Email != "user@example.com" {
		t.Errorf("expected email %q, got %q", "user@example.com", cfg.Tracker.Email)
	}
	if cfg.Tracker.Kind != "jira" {
		t.Errorf("expected kind %q, got %q", "jira", cfg.Tracker.Kind)
	}
	if cfg.Tracker.Endpoint != "https://mycompany.atlassian.net" {
		t.Errorf("expected endpoint %q, got %q", "https://mycompany.atlassian.net", cfg.Tracker.Endpoint)
	}
}

func TestParseCmuxConfig(t *testing.T) {
	raw := map[string]any{
		"cmux": map[string]any{
			"enabled":        true,
			"workspace_name": "MyWorkspace",
			"close_delay_ms": 5000,
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Cmux.Enabled {
		t.Error("expected cmux.enabled=true")
	}
	if cfg.Cmux.WorkspaceName != "MyWorkspace" {
		t.Errorf("expected workspace_name %q, got %q", "MyWorkspace", cfg.Cmux.WorkspaceName)
	}
	if cfg.Cmux.CloseDelayMs != 5000 {
		t.Errorf("expected close_delay_ms=5000, got %d", cfg.Cmux.CloseDelayMs)
	}
}

func TestCmuxDefaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Cmux.Enabled {
		t.Error("expected cmux.enabled=false by default")
	}
	if cfg.Cmux.WorkspaceName != "Symphony" {
		t.Errorf("expected default workspace_name %q, got %q", "Symphony", cfg.Cmux.WorkspaceName)
	}
	if cfg.Cmux.CloseDelayMs != 30000 {
		t.Errorf("expected default close_delay_ms=30000, got %d", cfg.Cmux.CloseDelayMs)
	}
}

func TestCmuxDefaultsPartial(t *testing.T) {
	raw := map[string]any{
		"cmux": map[string]any{
			"enabled": true,
		},
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.Cmux.Enabled {
		t.Error("expected cmux.enabled=true")
	}
	if cfg.Cmux.WorkspaceName != "Symphony" {
		t.Errorf("expected default workspace_name preserved, got %q", cfg.Cmux.WorkspaceName)
	}
	if cfg.Cmux.CloseDelayMs != 30000 {
		t.Errorf("expected default close_delay_ms preserved, got %d", cfg.Cmux.CloseDelayMs)
	}
}

func TestValidateDispatchConfig_InvalidBackend(t *testing.T) {
	cfg := validConfig()
	cfg.Backend = "unknown"

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid backend")
	}
	if !strings.Contains(err.Error(), "unsupported backend") {
		t.Errorf("expected 'unsupported backend' error, got: %v", err)
	}
}

func TestValidateDispatchConfig_ClaudeEmptyCommand(t *testing.T) {
	cfg := validConfig()
	cfg.Backend = "claude"
	cfg.Claude.Command = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for empty claude.command")
	}
	if !strings.Contains(err.Error(), "claude.command") {
		t.Errorf("expected error about claude.command, got: %v", err)
	}
}

// TestParseConfig_GitHubDefaultStates verifies that when tracker.kind is
// "github" and active_states/terminal_states are not provided, the defaults
// are "open" and "closed" rather than the Linear-specific defaults.
func TestParseConfig_GitHubDefaultStates(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":         "github",
			"api_key":      "ghp_token",
			"project_slug": "owner/repo",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Tracker.ActiveStates) != 1 || cfg.Tracker.ActiveStates[0] != "open" {
		t.Errorf("expected GitHub default active_states=[open], got %v", cfg.Tracker.ActiveStates)
	}
	if len(cfg.Tracker.TerminalStates) != 1 || cfg.Tracker.TerminalStates[0] != "closed" {
		t.Errorf("expected GitHub default terminal_states=[closed], got %v", cfg.Tracker.TerminalStates)
	}
}

// TestParseConfig_GitHubCustomStates verifies that explicitly configured
// active_states/terminal_states are preserved for the github tracker.
func TestParseConfig_GitHubCustomStates(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":            "github",
			"api_key":         "ghp_token",
			"project_slug":    "owner/repo",
			"active_states":   []any{"in-progress"},
			"terminal_states": []any{"closed", "done"},
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(cfg.Tracker.ActiveStates) != 1 || cfg.Tracker.ActiveStates[0] != "in-progress" {
		t.Errorf("expected active_states=[in-progress], got %v", cfg.Tracker.ActiveStates)
	}
	if len(cfg.Tracker.TerminalStates) != 2 {
		t.Errorf("expected 2 terminal states, got %v", cfg.Tracker.TerminalStates)
	}
}

// TestParseConfig_LinearDefaultStates verifies that when tracker.kind is
// "linear", the Linear-specific defaults are applied.
func TestParseConfig_LinearDefaultStates(t *testing.T) {
	raw := map[string]any{
		"tracker": map[string]any{
			"kind":         "linear",
			"api_key":      "lin_token",
			"project_slug": "my-proj",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	defaults := DefaultConfig()
	if len(cfg.Tracker.ActiveStates) == 0 {
		t.Error("expected non-empty default active_states for linear")
	}
	// Linear defaults should match the global defaults
	if len(cfg.Tracker.ActiveStates) != len(defaults.Tracker.ActiveStates) {
		t.Errorf("expected %v, got %v", defaults.Tracker.ActiveStates, cfg.Tracker.ActiveStates)
	}
}

func TestParseConfig_MiniAgentDefaults(t *testing.T) {
	cfg, err := ParseConfig(map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defaults := DefaultConfig()

	if cfg.MiniAgent.Command != defaults.MiniAgent.Command {
		t.Errorf("expected default mini_agent command %q, got %q", defaults.MiniAgent.Command, cfg.MiniAgent.Command)
	}
	if cfg.MiniAgent.Model != defaults.MiniAgent.Model {
		t.Errorf("expected default mini_agent model %q, got %q", defaults.MiniAgent.Model, cfg.MiniAgent.Model)
	}
	if cfg.MiniAgent.APIBase != defaults.MiniAgent.APIBase {
		t.Errorf("expected default mini_agent api_base %q, got %q", defaults.MiniAgent.APIBase, cfg.MiniAgent.APIBase)
	}
	if cfg.MiniAgent.TurnTimeoutMs != defaults.MiniAgent.TurnTimeoutMs {
		t.Errorf("expected default turn_timeout_ms %d, got %d", defaults.MiniAgent.TurnTimeoutMs, cfg.MiniAgent.TurnTimeoutMs)
	}
	if cfg.MiniAgent.ReadTimeoutMs != defaults.MiniAgent.ReadTimeoutMs {
		t.Errorf("expected default read_timeout_ms %d, got %d", defaults.MiniAgent.ReadTimeoutMs, cfg.MiniAgent.ReadTimeoutMs)
	}
	if cfg.MiniAgent.StallTimeoutMs != defaults.MiniAgent.StallTimeoutMs {
		t.Errorf("expected default stall_timeout_ms %d, got %d", defaults.MiniAgent.StallTimeoutMs, cfg.MiniAgent.StallTimeoutMs)
	}
}

func TestParseConfig_MiniAgentOverrides(t *testing.T) {
	raw := map[string]any{
		"backend": "mini_agent",
		"mini_agent": map[string]any{
			"command":         "mini-agent-acp",
			"model":           "MiniMax-M3",
			"api_key":         "$MINIMAX_API_KEY",
			"api_base":        "https://api.minimaxi.com",
			"turn_timeout_ms": 1800000,
			"read_timeout_ms": 10000,
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Backend != "mini_agent" {
		t.Errorf("expected backend=mini_agent, got %q", cfg.Backend)
	}
	if cfg.MiniAgent.Command != "mini-agent-acp" {
		t.Errorf("expected command %q, got %q", "mini-agent-acp", cfg.MiniAgent.Command)
	}
	if cfg.MiniAgent.Model != "MiniMax-M3" {
		t.Errorf("expected model %q, got %q", "MiniMax-M3", cfg.MiniAgent.Model)
	}
	if cfg.MiniAgent.APIKey != "$MINIMAX_API_KEY" {
		t.Errorf("expected api_key %q, got %q", "$MINIMAX_API_KEY", cfg.MiniAgent.APIKey)
	}
	if cfg.MiniAgent.APIBase != "https://api.minimaxi.com" {
		t.Errorf("expected api_base %q, got %q", "https://api.minimaxi.com", cfg.MiniAgent.APIBase)
	}
	if cfg.MiniAgent.TurnTimeoutMs != 1800000 {
		t.Errorf("expected turn_timeout_ms=1800000, got %d", cfg.MiniAgent.TurnTimeoutMs)
	}
	if cfg.MiniAgent.ReadTimeoutMs != 10000 {
		t.Errorf("expected read_timeout_ms=10000, got %d", cfg.MiniAgent.ReadTimeoutMs)
	}
	// Unset field should retain default
	defaults := DefaultConfig()
	if cfg.MiniAgent.StallTimeoutMs != defaults.MiniAgent.StallTimeoutMs {
		t.Errorf("expected default stall_timeout_ms preserved, got %d", cfg.MiniAgent.StallTimeoutMs)
	}
}

func TestParseConfig_MiniMaxAgentAlias(t *testing.T) {
	raw := map[string]any{
		"minimax_agent": map[string]any{
			"command": "minimax-agent-acp",
			"model":   "MiniMax-M2.5",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MiniAgent.Command != "minimax-agent-acp" {
		t.Errorf("expected minimax_agent alias to mini_agent.command, got %q", cfg.MiniAgent.Command)
	}
	if cfg.MiniAgent.Model != "MiniMax-M2.5" {
		t.Errorf("expected minimax_agent alias to mini_agent.model, got %q", cfg.MiniAgent.Model)
	}
}

func TestParseConfig_MiniAgentTakesPrecedenceOverMiniMaxAgent(t *testing.T) {
	raw := map[string]any{
		"minimax_agent": map[string]any{
			"command": "minimax-agent-acp",
		},
		"mini_agent": map[string]any{
			"command": "mini-agent-acp",
		},
	}

	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.MiniAgent.Command != "mini-agent-acp" {
		t.Errorf("expected mini_agent to take precedence over minimax_agent, got %q", cfg.MiniAgent.Command)
	}
}

func TestParseConfig_BackendMiniAgent(t *testing.T) {
	raw := map[string]any{
		"backend": "mini_agent",
	}
	cfg, err := ParseConfig(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backend != "mini_agent" {
		t.Errorf("expected backend %q, got %q", "mini_agent", cfg.Backend)
	}
}
