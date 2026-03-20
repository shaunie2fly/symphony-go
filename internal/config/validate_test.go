package config

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	cfg := DefaultConfig()
	cfg.Tracker.Kind = "linear"
	cfg.Tracker.APIKey = "lin_test_key"
	cfg.Tracker.ProjectSlug = "my-project"
	return &cfg
}

func TestValidateDispatchConfig_ValidConfig(t *testing.T) {
	if err := ValidateDispatchConfig(validConfig()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateDispatchConfig_MissingTrackerKind(t *testing.T) {
	cfg := validConfig()
	cfg.Tracker.Kind = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.kind")
	}
	if !strings.Contains(err.Error(), "tracker.kind") {
		t.Errorf("expected error about tracker.kind, got: %v", err)
	}
}

func TestValidateDispatchConfig_UnsupportedTrackerKind(t *testing.T) {
	cfg := validConfig()
	cfg.Tracker.Kind = "trello"

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for unsupported tracker.kind")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}

func TestValidateDispatchConfig_MissingAPIKey(t *testing.T) {
	cfg := validConfig()
	cfg.Tracker.APIKey = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("expected error about api_key, got: %v", err)
	}
}

func TestValidateDispatchConfig_MissingProjectSlug(t *testing.T) {
	cfg := validConfig()
	cfg.Tracker.ProjectSlug = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing project_slug")
	}
	if !strings.Contains(err.Error(), "project_slug") {
		t.Errorf("expected error about project_slug, got: %v", err)
	}
}

func TestValidateDispatchConfig_MissingGeminiCommand(t *testing.T) {
	cfg := validConfig()
	cfg.Gemini.Command = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing gemini.command")
	}
	if !strings.Contains(err.Error(), "gemini.command") {
		t.Errorf("expected error about gemini.command, got: %v", err)
	}
}

func TestValidateDispatchConfig_MultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	// kind, api_key, project_slug all missing

	err := ValidateDispatchConfig(&cfg)
	if err == nil {
		t.Fatal("expected error")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected *ValidationError, got %T", err)
	}
	if len(ve.Errors) < 2 {
		t.Errorf("expected multiple validation errors, got %d: %v", len(ve.Errors), ve.Errors)
	}
}

func validJiraConfig() *Config {
	cfg := DefaultConfig()
	cfg.Tracker.Kind = "jira"
	cfg.Tracker.Endpoint = "https://mycompany.atlassian.net"
	cfg.Tracker.APIKey = "jira_test_token"
	cfg.Tracker.ProjectSlug = "PROJ"
	cfg.Tracker.Email = "user@example.com"
	return &cfg
}

func TestValidateDispatchConfig_JiraValid(t *testing.T) {
	if err := ValidateDispatchConfig(validJiraConfig()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateDispatchConfig_JiraMissingEmail(t *testing.T) {
	cfg := validJiraConfig()
	cfg.Tracker.Email = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.email")
	}
	if !strings.Contains(err.Error(), "tracker.email") {
		t.Errorf("expected error about tracker.email, got: %v", err)
	}
}

func TestValidateDispatchConfig_JiraMissingEndpoint(t *testing.T) {
	cfg := validJiraConfig()
	cfg.Tracker.Endpoint = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.endpoint")
	}
	if !strings.Contains(err.Error(), "tracker.endpoint") {
		t.Errorf("expected error about tracker.endpoint, got: %v", err)
	}
}

func TestValidateDispatchConfig_JiraMissingProjectSlug(t *testing.T) {
	cfg := validJiraConfig()
	cfg.Tracker.ProjectSlug = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.project_slug")
	}
	if !strings.Contains(err.Error(), "tracker.project_slug") {
		t.Errorf("expected error about tracker.project_slug, got: %v", err)
	}
}

func validGitHubConfig() *Config {
	cfg := DefaultConfig()
	cfg.Tracker.Kind = "github"
	cfg.Tracker.APIKey = "ghp_testtoken"
	cfg.Tracker.ProjectSlug = "owner/repo"
	return &cfg
}

func TestValidateDispatchConfig_GitHubValid(t *testing.T) {
	if err := ValidateDispatchConfig(validGitHubConfig()); err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestValidateDispatchConfig_GitHubMissingProjectSlug(t *testing.T) {
	cfg := validGitHubConfig()
	cfg.Tracker.ProjectSlug = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.project_slug")
	}
	if !strings.Contains(err.Error(), "tracker.project_slug") {
		t.Errorf("expected error about tracker.project_slug, got: %v", err)
	}
}

func TestValidateDispatchConfig_GitHubMissingAPIKey(t *testing.T) {
	cfg := validGitHubConfig()
	cfg.Tracker.APIKey = ""

	err := ValidateDispatchConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing tracker.api_key")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("expected error about api_key, got: %v", err)
	}
}
