package config

import (
	"errors"
	"fmt"
	"strings"
)

// ValidationError holds one or more config validation failures.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("config validation failed: %s", strings.Join(e.Errors, "; "))
}

// ValidateDispatchConfig checks that the config has all required fields
// for the orchestrator to poll and dispatch work.
func ValidateDispatchConfig(cfg *Config) error {
	var errs []string

	// tracker.kind must be present and supported
	if cfg.Tracker.Kind == "" {
		errs = append(errs, "tracker.kind is required")
	} else {
		switch cfg.Tracker.Kind {
		case "linear":
			if cfg.Tracker.ProjectSlug == "" {
				errs = append(errs, `tracker.project_slug is required when tracker.kind is "linear"`)
			}
		case "jira":
			if cfg.Tracker.Endpoint == "" {
				errs = append(errs, `tracker.endpoint is required when tracker.kind is "jira"`)
			}
			if cfg.Tracker.ProjectSlug == "" {
				errs = append(errs, `tracker.project_slug is required when tracker.kind is "jira"`)
			}
			if cfg.Tracker.Email == "" {
				errs = append(errs, `tracker.email is required when tracker.kind is "jira"`)
			}
		case "github":
			if cfg.Tracker.ProjectSlug == "" {
				errs = append(errs, `tracker.project_slug is required when tracker.kind is "github" (must be in owner/repo format)`)
			}
		default:
			errs = append(errs, fmt.Sprintf("tracker.kind %q is not supported", cfg.Tracker.Kind))
		}
	}

	// api_key is required for all tracker kinds
	if cfg.Tracker.APIKey == "" {
		errs = append(errs, "tracker.api_key is required")
	}

	// backend-specific validation
	switch cfg.Backend {
	case "", "gemini":
		if cfg.Gemini.Command == "" {
			errs = append(errs, "gemini.command is required (must be non-empty)")
		}
	case "claude":
		if cfg.Claude.Command == "" {
			errs = append(errs, "claude.command is required when backend is \"claude\"")
		}
	case "mini_agent", "mini-agent":
		if cfg.MiniAgent.Command == "" {
			errs = append(errs, "mini_agent.command is required when backend is \"mini_agent\"")
		}
	default:
		errs = append(errs, fmt.Sprintf("unsupported backend: %q", cfg.Backend))
	}

	if len(errs) > 0 {
		return &ValidationError{Errors: errs}
	}
	return nil
}

// IsValidationError checks if an error is a ValidationError.
func IsValidationError(err error) bool {
	var ve *ValidationError
	return errors.As(err, &ve)
}
