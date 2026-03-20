package tracker

import (
	"testing"

	"github.com/symphony-go/symphony/internal/config"
)

func TestNewTrackerClient_Linear(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind:     "linear",
		Endpoint: "https://api.linear.app/graphql",
		APIKey:   "test-key",
	}
	client, err := NewTrackerClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*LinearClient); !ok {
		t.Errorf("expected *LinearClient, got %T", client)
	}
}

func TestNewTrackerClient_Jira(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind:     "jira",
		Endpoint: "https://test.atlassian.net",
		Email:    "test@example.com",
		APIKey:   "test-token",
	}
	client, err := NewTrackerClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*JiraClient); !ok {
		t.Errorf("expected *JiraClient, got %T", client)
	}
}

func TestNewTrackerClient_GitHub(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind:        "github",
		ProjectSlug: "owner/repo",
		APIKey:      "ghp_testtoken",
	}
	client, err := NewTrackerClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := client.(*GitHubClient); !ok {
		t.Errorf("expected *GitHubClient, got %T", client)
	}
}

func TestNewTrackerClient_GitHub_InvalidSlug(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind:        "github",
		ProjectSlug: "noslash",
		APIKey:      "ghp_testtoken",
	}
	_, err := NewTrackerClient(cfg)
	if err == nil {
		t.Fatal("expected error for invalid project slug")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubInvalidSlug {
		t.Errorf("expected %s, got %s", ErrGitHubInvalidSlug, te.Kind)
	}
}

func TestNewTrackerClient_Unknown(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind: "trello",
	}
	_, err := NewTrackerClient(cfg)
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrUnsupportedTrackerKind {
		t.Errorf("expected kind %q, got %q", ErrUnsupportedTrackerKind, te.Kind)
	}
}

func TestNewTrackerClient_Empty(t *testing.T) {
	cfg := &config.TrackerConfig{
		Kind: "",
	}
	_, err := NewTrackerClient(cfg)
	if err == nil {
		t.Fatal("expected error for empty kind")
	}
}
