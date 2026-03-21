package config

import (
	"os"
	"path/filepath"
)

// DefaultConfig returns a Config with all spec-defined default values.
func DefaultConfig() Config {
	return Config{
		Backend: "gemini",
		Tracker: TrackerConfig{
			Endpoint:       "https://api.linear.app/graphql",
			ActiveStates:   []string{"Todo", "In Progress"},
			TerminalStates: []string{"Closed", "Cancelled", "Canceled", "Duplicate", "Done"},
		},
		Polling: PollingConfig{
			IntervalMs: 30000,
		},
		Workspace: WorkspaceConfig{
			Root: filepath.Join(os.TempDir(), "symphony_workspaces"),
		},
		Hooks: HooksConfig{
			TimeoutMs: 60000,
		},
		Agent: AgentConfig{
			MaxConcurrentAgents:        10,
			MaxTurns:                   20,
			MaxRetryBackoffMs:          300000,
			MaxConcurrentAgentsByState: map[string]int{},
		},
		Gemini: GeminiConfig{
			Command:        "gemini --acp",
			Model:          "gemini-3.1-pro-preview",
			TurnTimeoutMs:  3600000,
			ReadTimeoutMs:  5000,
			StallTimeoutMs: 300000,
		},
		Claude: ClaudeConfig{
			Command:        "claude",
			Model:          "claude-sonnet-4-6",
			PermissionMode: "bypassPermissions",
			AllowedTools:   []string{"Read", "Write", "Edit", "Bash"},
			MaxTurns:       25,
			TurnTimeoutMs:  600000,
			StallTimeoutMs: 300000,
		},
		MiniAgent: MiniAgentConfig{
			Command:        "mini-agent-acp",
			Model:          "MiniMax-M2.5",
			APIBase:        "https://api.minimax.io",
			TurnTimeoutMs:  3600000,
			ReadTimeoutMs:  5000,
			StallTimeoutMs: 300000,
		},
		Cmux: CmuxConfig{
			Enabled:       false,
			WorkspaceName: "Symphony",
			CloseDelayMs:  30000,
		},
	}
}
