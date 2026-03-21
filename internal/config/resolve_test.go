package config

import (
	"os"
	"strings"
	"testing"
)

func TestResolveConfig_EnvVarResolution(t *testing.T) {
	t.Setenv("TEST_LINEAR_KEY", "my-secret-key")

	cfg := DefaultConfig()
	cfg.Tracker.APIKey = "$TEST_LINEAR_KEY"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Tracker.APIKey != "my-secret-key" {
		t.Errorf("expected api_key=my-secret-key, got %q", resolved.Tracker.APIKey)
	}
}

func TestResolveConfig_EmptyEnvVarTreatedAsMissing(t *testing.T) {
	t.Setenv("TEST_EMPTY_KEY", "")

	cfg := DefaultConfig()
	cfg.Tracker.APIKey = "$TEST_EMPTY_KEY"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Tracker.APIKey != "" {
		t.Errorf("expected empty api_key, got %q", resolved.Tracker.APIKey)
	}
}

func TestResolveConfig_TildeExpansion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Workspace.Root = "~/my_workspaces"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	home, _ := os.UserHomeDir()
	if !strings.HasPrefix(resolved.Workspace.Root, home) {
		t.Errorf("expected path to start with home dir %q, got %q", home, resolved.Workspace.Root)
	}
	if !strings.HasSuffix(resolved.Workspace.Root, "my_workspaces") {
		t.Errorf("expected path to end with my_workspaces, got %q", resolved.Workspace.Root)
	}
}

func TestResolveConfig_EnvVarInWorkspaceRoot(t *testing.T) {
	t.Setenv("TEST_WS_ROOT", "/custom/workspace/root")

	cfg := DefaultConfig()
	cfg.Workspace.Root = "$TEST_WS_ROOT"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Workspace.Root != "/custom/workspace/root" {
		t.Errorf("expected /custom/workspace/root, got %q", resolved.Workspace.Root)
	}
}

func TestResolveConfig_PerStateConcurrencyNormalization(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Agent.MaxConcurrentAgentsByState = map[string]int{
		"Todo":        3,
		"In Progress": 5,
		"Invalid":     -1,
		"Zero":        0,
	}

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	m := resolved.Agent.MaxConcurrentAgentsByState
	if m["todo"] != 3 {
		t.Errorf("expected todo=3, got %d", m["todo"])
	}
	if m["in progress"] != 5 {
		t.Errorf("expected 'in progress'=5, got %d", m["in progress"])
	}
	if _, exists := m["invalid"]; exists {
		t.Error("expected 'invalid' to be dropped (non-positive)")
	}
	if _, exists := m["zero"]; exists {
		t.Error("expected 'zero' to be dropped (non-positive)")
	}
}

func TestResolveConfig_LiteralValueNotResolved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Tracker.APIKey = "lin_api_literal_token"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.Tracker.APIKey != "lin_api_literal_token" {
		t.Errorf("expected literal token, got %q", resolved.Tracker.APIKey)
	}
}

func TestResolveConfig_MiniAgentAPIKeyEnvVar(t *testing.T) {
	t.Setenv("TEST_MINIMAX_KEY", "minimax-secret-key")

	cfg := DefaultConfig()
	cfg.MiniAgent.APIKey = "$TEST_MINIMAX_KEY"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.MiniAgent.APIKey != "minimax-secret-key" {
		t.Errorf("expected mini_agent.api_key=minimax-secret-key, got %q", resolved.MiniAgent.APIKey)
	}
}

func TestResolveConfig_MiniAgentAPIKeyFallbackToEnv(t *testing.T) {
	t.Setenv("MINIMAX_API_KEY", "fallback-minimax-key")

	cfg := DefaultConfig()
	// api_key is empty — should fall back to MINIMAX_API_KEY env var
	cfg.MiniAgent.APIKey = ""

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.MiniAgent.APIKey != "fallback-minimax-key" {
		t.Errorf("expected fallback to MINIMAX_API_KEY, got %q", resolved.MiniAgent.APIKey)
	}
}

func TestResolveConfig_MiniAgentAPIKeyLiteralNotResolved(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MiniAgent.APIKey = "literal-api-key"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resolved.MiniAgent.APIKey != "literal-api-key" {
		t.Errorf("expected literal api key unchanged, got %q", resolved.MiniAgent.APIKey)
	}
}

func TestResolveConfig_MiniAgentAPIKeyEnvVarEmpty(t *testing.T) {
	os.Unsetenv("MINIMAX_API_KEY")

	cfg := DefaultConfig()
	cfg.MiniAgent.APIKey = "$NONEXISTENT_MINIMAX_VAR"

	resolved, err := ResolveConfig(&cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unset env var resolves to empty string
	if resolved.MiniAgent.APIKey != "" {
		t.Errorf("expected empty api_key for missing env var, got %q", resolved.MiniAgent.APIKey)
	}
}
