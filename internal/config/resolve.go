package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ResolveConfig applies environment variable resolution, path expansion,
// and normalization to a parsed Config.
func ResolveConfig(cfg *Config) (*Config, error) {
	resolved := cfg.Clone()

	// Resolve $VAR in tracker.api_key
	resolved.Tracker.APIKey = resolveEnvVar(resolved.Tracker.APIKey)

	// Resolve $VAR in tracker.email
	resolved.Tracker.Email = resolveEnvVar(resolved.Tracker.Email)

	// Fallback to LINEAR_API_KEY env var if still empty
	if resolved.Tracker.APIKey == "" {
		resolved.Tracker.APIKey = os.Getenv("LINEAR_API_KEY")
	}

	// Jira-specific env fallbacks
	if resolved.Tracker.Kind == "jira" {
		if resolved.Tracker.Email == "" {
			resolved.Tracker.Email = os.Getenv("JIRA_EMAIL")
		}
		if resolved.Tracker.APIKey == "" {
			resolved.Tracker.APIKey = os.Getenv("JIRA_API_TOKEN")
		}
	}

	// Resolve $VAR and ~ in workspace.root
	resolved.Workspace.Root = resolveEnvVar(resolved.Workspace.Root)
	resolved.Workspace.Root = expandHome(resolved.Workspace.Root)
	resolved.Workspace.Root = expandPath(resolved.Workspace.Root)

	// Resolve $VAR in mini_agent.api_key
	resolved.MiniAgent.APIKey = resolveEnvVar(resolved.MiniAgent.APIKey)

	// Fallback to MINIMAX_API_KEY env var if still empty
	if resolved.MiniAgent.APIKey == "" {
		resolved.MiniAgent.APIKey = os.Getenv("MINIMAX_API_KEY")
	}

	// Normalize per-state concurrency: lowercase keys, drop invalid values
	resolved.Agent.MaxConcurrentAgentsByState = normalizePerStateConcurrency(
		resolved.Agent.MaxConcurrentAgentsByState,
	)

	return resolved, nil
}

// resolveEnvVar resolves a $VAR_NAME reference from the environment.
// If the value starts with $, look up the rest as an env var name.
// If the env var is empty, return empty string (caller treats as missing).
func resolveEnvVar(value string) string {
	if !strings.HasPrefix(value, "$") {
		return value
	}
	envName := value[1:]
	if envName == "" {
		return value
	}
	return os.Getenv(envName)
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[1:])
	}
	return path
}

// expandPath cleans and resolves a path.
// Paths containing separators are cleaned; bare strings are preserved.
func expandPath(path string) string {
	if path == "" {
		return path
	}
	if strings.Contains(path, string(filepath.Separator)) || strings.Contains(path, "/") {
		return filepath.Clean(path)
	}
	return path
}

// normalizePerStateConcurrency lowercases keys and drops non-positive values.
func normalizePerStateConcurrency(m map[string]int) map[string]int {
	result := make(map[string]int)
	for k, v := range m {
		if v > 0 {
			result[strings.ToLower(k)] = v
		}
	}
	return result
}

// CoerceStringInt attempts to parse a string as an integer.
// Returns the parsed int if successful, or the original value and an error.
func CoerceStringInt(value any) (int, error) {
	switch v := value.(type) {
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("cannot coerce %q to int: %w", v, err)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("unsupported type %T for int coercion", value)
	}
}
