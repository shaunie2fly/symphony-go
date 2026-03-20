package config

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
)

// Config is the typed service configuration derived from WORKFLOW.md front matter.
type Config struct {
	Backend   string          `yaml:"backend"   json:"backend"`
	Tracker   TrackerConfig   `yaml:"tracker"   json:"tracker"`
	Polling   PollingConfig   `yaml:"polling"   json:"polling"`
	Workspace WorkspaceConfig `yaml:"workspace" json:"workspace"`
	Hooks     HooksConfig     `yaml:"hooks"     json:"hooks"`
	Agent     AgentConfig     `yaml:"agent"     json:"agent"`
	Gemini    GeminiConfig    `yaml:"gemini"    json:"gemini"`
	Claude    ClaudeConfig    `yaml:"claude"    json:"claude"`
	Server    ServerConfig    `yaml:"server"    json:"server"`
	Cmux      CmuxConfig      `yaml:"cmux"      json:"cmux"`
}

type TrackerConfig struct {
	Kind           string   `yaml:"kind"            json:"kind"`
	Endpoint       string   `yaml:"endpoint"        json:"endpoint"`
	APIKey         string   `yaml:"api_key"         json:"api_key"`
	ProjectSlug    string   `yaml:"project_slug"    json:"project_slug"`
	Email          string   `yaml:"email"           json:"email"`
	ActiveStates   []string `yaml:"active_states"   json:"active_states"`
	TerminalStates []string `yaml:"terminal_states" json:"terminal_states"`
}

type PollingConfig struct {
	IntervalMs int `yaml:"interval_ms" json:"interval_ms"`
}

type WorkspaceConfig struct {
	Root string `yaml:"root" json:"root"`
}

type HooksConfig struct {
	AfterCreate  *string `yaml:"after_create"  json:"after_create"`
	BeforeRun    *string `yaml:"before_run"    json:"before_run"`
	AfterRun     *string `yaml:"after_run"     json:"after_run"`
	BeforeRemove *string `yaml:"before_remove" json:"before_remove"`
	TimeoutMs    int     `yaml:"timeout_ms"    json:"timeout_ms"`
}

type AgentConfig struct {
	MaxConcurrentAgents        int            `yaml:"max_concurrent_agents"          json:"max_concurrent_agents"`
	MaxTurns                   int            `yaml:"max_turns"                      json:"max_turns"`
	MaxRetryBackoffMs          int            `yaml:"max_retry_backoff_ms"           json:"max_retry_backoff_ms"`
	MaxConcurrentAgentsByState map[string]int `yaml:"max_concurrent_agents_by_state" json:"max_concurrent_agents_by_state"`
}

type GeminiConfig struct {
	Command        string `yaml:"command"          json:"command"`
	Model          string `yaml:"model"            json:"model"`
	TurnTimeoutMs  int    `yaml:"turn_timeout_ms"  json:"turn_timeout_ms"`
	ReadTimeoutMs  int    `yaml:"read_timeout_ms"  json:"read_timeout_ms"`
	StallTimeoutMs int    `yaml:"stall_timeout_ms" json:"stall_timeout_ms"`
}

type ClaudeConfig struct {
	Command        string   `yaml:"command"          json:"command"`
	Model          string   `yaml:"model"            json:"model"`
	PermissionMode string   `yaml:"permission_mode"  json:"permission_mode"`
	AllowedTools   []string `yaml:"allowed_tools"    json:"allowed_tools"`
	MaxTurns       int      `yaml:"max_turns"        json:"max_turns"`
	TurnTimeoutMs  int      `yaml:"turn_timeout_ms"  json:"turn_timeout_ms"`
	StallTimeoutMs int      `yaml:"stall_timeout_ms" json:"stall_timeout_ms"`
}

type ServerConfig struct {
	Port *int `yaml:"port" json:"port"`
}

type CmuxConfig struct {
	Enabled       bool   `yaml:"enabled"        json:"enabled"`
	WorkspaceName string `yaml:"workspace_name" json:"workspace_name"`
	CloseDelayMs  int    `yaml:"close_delay_ms" json:"close_delay_ms"`
}

// ParseConfig takes the raw map from WORKFLOW.md front matter and produces a typed Config.
// It starts from defaults and overlays the raw values.
func ParseConfig(raw map[string]any) (*Config, error) {
	cfg := DefaultConfig()

	if raw == nil || len(raw) == 0 {
		return &cfg, nil
	}

	// Alias: if raw has "codex" but not "gemini", rename it
	if _, hasGemini := raw["gemini"]; !hasGemini {
		if codexVal, hasCodex := raw["codex"]; hasCodex {
			raw["gemini"] = codexVal
			delete(raw, "codex")
		}
	}

	// Alias: if raw has "claude_code" but not "claude", rename it
	if _, hasClaude := raw["claude"]; !hasClaude {
		if claudeCodeVal, hasClaudeCode := raw["claude_code"]; hasClaudeCode {
			raw["claude"] = claudeCodeVal
			delete(raw, "claude_code")
		}
	}

	// Marshal raw map to YAML, then unmarshal onto the config struct.
	// This lets yaml.v3 handle the field mapping and type coercion.
	yamlBytes, err := yaml.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("config marshal error: %w", err)
	}

	if err := yaml.Unmarshal(yamlBytes, &cfg); err != nil {
		return nil, fmt.Errorf("config parse error: %w", err)
	}

	// Preserve defaults for zero-value fields that weren't in the raw map.
	// yaml.Unmarshal sets zero values for missing fields, so we need to
	// re-apply defaults selectively.
	defaults := DefaultConfig()
	applyDefaults(&cfg, &defaults, raw)

	return &cfg, nil
}

// applyDefaults re-applies default values for fields that were not present in the raw config.
func applyDefaults(cfg *Config, defaults *Config, raw map[string]any) {
	trackerRaw, _ := raw["tracker"].(map[string]any)
	pollingRaw, _ := raw["polling"].(map[string]any)
	workspaceRaw, _ := raw["workspace"].(map[string]any)
	hooksRaw, _ := raw["hooks"].(map[string]any)
	agentRaw, _ := raw["agent"].(map[string]any)
	geminiRaw, _ := raw["gemini"].(map[string]any)
	claudeRaw, _ := raw["claude"].(map[string]any)

	if trackerRaw == nil {
		cfg.Tracker = defaults.Tracker
	} else {
		if _, ok := trackerRaw["endpoint"]; !ok {
			cfg.Tracker.Endpoint = defaults.Tracker.Endpoint
		}
		if _, ok := trackerRaw["active_states"]; !ok {
			if cfg.Tracker.Kind == "github" {
				cfg.Tracker.ActiveStates = []string{"open"}
			} else {
				cfg.Tracker.ActiveStates = defaults.Tracker.ActiveStates
			}
		}
		if _, ok := trackerRaw["terminal_states"]; !ok {
			if cfg.Tracker.Kind == "github" {
				cfg.Tracker.TerminalStates = []string{"closed"}
			} else {
				cfg.Tracker.TerminalStates = defaults.Tracker.TerminalStates
			}
		}
	}

	if pollingRaw == nil {
		cfg.Polling = defaults.Polling
	} else {
		if _, ok := pollingRaw["interval_ms"]; !ok {
			cfg.Polling.IntervalMs = defaults.Polling.IntervalMs
		}
	}

	if workspaceRaw == nil {
		cfg.Workspace = defaults.Workspace
	} else {
		if _, ok := workspaceRaw["root"]; !ok {
			cfg.Workspace.Root = defaults.Workspace.Root
		}
	}

	if hooksRaw == nil {
		cfg.Hooks = defaults.Hooks
	} else {
		if _, ok := hooksRaw["timeout_ms"]; !ok {
			cfg.Hooks.TimeoutMs = defaults.Hooks.TimeoutMs
		}
	}

	if agentRaw == nil {
		cfg.Agent = defaults.Agent
	} else {
		if _, ok := agentRaw["max_concurrent_agents"]; !ok {
			cfg.Agent.MaxConcurrentAgents = defaults.Agent.MaxConcurrentAgents
		}
		if _, ok := agentRaw["max_turns"]; !ok {
			cfg.Agent.MaxTurns = defaults.Agent.MaxTurns
		}
		if _, ok := agentRaw["max_retry_backoff_ms"]; !ok {
			cfg.Agent.MaxRetryBackoffMs = defaults.Agent.MaxRetryBackoffMs
		}
		if cfg.Agent.MaxConcurrentAgentsByState == nil {
			cfg.Agent.MaxConcurrentAgentsByState = defaults.Agent.MaxConcurrentAgentsByState
		}
	}

	if geminiRaw == nil {
		cfg.Gemini = defaults.Gemini
	} else {
		if _, ok := geminiRaw["command"]; !ok {
			cfg.Gemini.Command = defaults.Gemini.Command
		}
		if _, ok := geminiRaw["model"]; !ok {
			cfg.Gemini.Model = defaults.Gemini.Model
		}
		if _, ok := geminiRaw["turn_timeout_ms"]; !ok {
			cfg.Gemini.TurnTimeoutMs = defaults.Gemini.TurnTimeoutMs
		}
		if _, ok := geminiRaw["read_timeout_ms"]; !ok {
			cfg.Gemini.ReadTimeoutMs = defaults.Gemini.ReadTimeoutMs
		}
		if _, ok := geminiRaw["stall_timeout_ms"]; !ok {
			cfg.Gemini.StallTimeoutMs = defaults.Gemini.StallTimeoutMs
		}
	}

	if claudeRaw == nil {
		cfg.Claude = defaults.Claude
	} else {
		if _, ok := claudeRaw["command"]; !ok {
			cfg.Claude.Command = defaults.Claude.Command
		}
		if _, ok := claudeRaw["model"]; !ok {
			cfg.Claude.Model = defaults.Claude.Model
		}
		if _, ok := claudeRaw["permission_mode"]; !ok {
			cfg.Claude.PermissionMode = defaults.Claude.PermissionMode
		}
		if _, ok := claudeRaw["allowed_tools"]; !ok {
			cfg.Claude.AllowedTools = defaults.Claude.AllowedTools
		}
		if _, ok := claudeRaw["max_turns"]; !ok {
			cfg.Claude.MaxTurns = defaults.Claude.MaxTurns
		}
		if _, ok := claudeRaw["turn_timeout_ms"]; !ok {
			cfg.Claude.TurnTimeoutMs = defaults.Claude.TurnTimeoutMs
		}
		if _, ok := claudeRaw["stall_timeout_ms"]; !ok {
			cfg.Claude.StallTimeoutMs = defaults.Claude.StallTimeoutMs
		}
	}

	cmuxRaw, _ := raw["cmux"].(map[string]any)
	if cmuxRaw == nil {
		cfg.Cmux = defaults.Cmux
	} else {
		if _, ok := cmuxRaw["workspace_name"]; !ok {
			cfg.Cmux.WorkspaceName = defaults.Cmux.WorkspaceName
		}
		if _, ok := cmuxRaw["close_delay_ms"]; !ok {
			cfg.Cmux.CloseDelayMs = defaults.Cmux.CloseDelayMs
		}
	}

	if _, ok := raw["backend"]; !ok {
		cfg.Backend = defaults.Backend
	}
}

// Clone returns a deep copy of the Config.
func (c *Config) Clone() *Config {
	// Use JSON round-trip for deep copy
	data, _ := json.Marshal(c)
	var clone Config
	_ = json.Unmarshal(data, &clone)
	if clone.Agent.MaxConcurrentAgentsByState == nil {
		clone.Agent.MaxConcurrentAgentsByState = map[string]int{}
	}
	return &clone
}
