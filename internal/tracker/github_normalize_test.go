package tracker

import (
	"testing"
)

func TestNormalizeGitHubIssue_BasicFields(t *testing.T) {
	body := "This is the description."
	raw := githubIssue{
		Number:    7,
		Title:     "Add feature X",
		Body:      &body,
		State:     "open",
		HTMLURL:   "https://github.com/owner/repo/issues/7",
		Labels:    []githubLabel{{Name: "enhancement"}, {Name: "Help Wanted"}},
		CreatedAt: "2024-03-01T10:00:00Z",
		UpdatedAt: "2024-03-02T12:30:00Z",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.ID != "7" {
		t.Errorf("expected ID=7, got %q", issue.ID)
	}
	if issue.Identifier != "owner/repo#7" {
		t.Errorf("expected Identifier=owner/repo#7, got %q", issue.Identifier)
	}
	if issue.Title != "Add feature X" {
		t.Errorf("expected title='Add feature X', got %q", issue.Title)
	}
	if issue.State != "open" {
		t.Errorf("expected state=open, got %q", issue.State)
	}
	if issue.Description == nil || *issue.Description != body {
		t.Errorf("expected description=%q, got %v", body, issue.Description)
	}
	if issue.URL == nil || *issue.URL != "https://github.com/owner/repo/issues/7" {
		t.Errorf("unexpected URL: %v", issue.URL)
	}

	// Labels should be lowercased
	if len(issue.Labels) != 2 {
		t.Fatalf("expected 2 labels, got %d", len(issue.Labels))
	}
	if issue.Labels[0] != "enhancement" {
		t.Errorf("expected labels[0]=enhancement, got %q", issue.Labels[0])
	}
	if issue.Labels[1] != "help wanted" {
		t.Errorf("expected labels[1]=help wanted, got %q", issue.Labels[1])
	}

	if issue.Priority != nil {
		t.Errorf("expected Priority=nil, got %v", issue.Priority)
	}
	if issue.BranchName != nil {
		t.Errorf("expected BranchName=nil, got %v", issue.BranchName)
	}
	if issue.CreatedAt == nil {
		t.Error("expected CreatedAt to be set")
	}
	if issue.UpdatedAt == nil {
		t.Error("expected UpdatedAt to be set")
	}
}

func TestNormalizeGitHubIssue_NilBody(t *testing.T) {
	raw := githubIssue{
		Number: 1,
		Title:  "No body",
		State:  "open",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.Description != nil {
		t.Errorf("expected nil Description, got %v", issue.Description)
	}
}

func TestNormalizeGitHubIssue_EmptyBody(t *testing.T) {
	empty := ""
	raw := githubIssue{
		Number: 2,
		Title:  "Empty body",
		Body:   &empty,
		State:  "open",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.Description != nil {
		t.Errorf("expected nil Description for empty body, got %v", issue.Description)
	}
}

func TestNormalizeGitHubIssue_NoLabels(t *testing.T) {
	raw := githubIssue{
		Number: 3,
		Title:  "No labels",
		State:  "open",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.Labels == nil {
		t.Error("expected non-nil Labels slice")
	}
	if len(issue.Labels) != 0 {
		t.Errorf("expected 0 labels, got %d", len(issue.Labels))
	}
}

func TestNormalizeGitHubIssue_BlockedByEmpty(t *testing.T) {
	raw := githubIssue{
		Number: 4,
		Title:  "Issue",
		State:  "open",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.BlockedBy == nil {
		t.Error("expected non-nil BlockedBy slice")
	}
	if len(issue.BlockedBy) != 0 {
		t.Errorf("expected 0 blockers, got %d", len(issue.BlockedBy))
	}
}

func TestNormalizeGitHubIssue_ClosedState(t *testing.T) {
	raw := githubIssue{
		Number: 5,
		Title:  "Closed issue",
		State:  "closed",
	}

	issue := normalizeGitHubIssue(raw, "owner", "repo")

	if issue.State != "closed" {
		t.Errorf("expected state=closed, got %q", issue.State)
	}
}

func TestNormalizeGitHubIssue_Identifier(t *testing.T) {
	raw := githubIssue{
		Number: 123,
		Title:  "Test",
		State:  "open",
	}

	issue := normalizeGitHubIssue(raw, "myorg", "myrepo")

	if issue.Identifier != "myorg/myrepo#123" {
		t.Errorf("expected Identifier=myorg/myrepo#123, got %q", issue.Identifier)
	}
}
