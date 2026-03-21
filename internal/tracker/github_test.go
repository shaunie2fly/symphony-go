package tracker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGitHubClient_ValidSlug(t *testing.T) {
	client, err := NewGitHubClient("owner/repo", "test-token", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if client.owner != "owner" {
		t.Errorf("expected owner=owner, got %q", client.owner)
	}
	if client.repo != "repo" {
		t.Errorf("expected repo=repo, got %q", client.repo)
	}
}

func TestNewGitHubClient_InvalidSlug(t *testing.T) {
	tests := []struct {
		slug string
	}{
		{"noslash"},
		{""},
		{"/repo"},
		{"owner/"},
	}
	for _, tc := range tests {
		_, err := NewGitHubClient(tc.slug, "token", nil, nil)
		if err == nil {
			t.Errorf("expected error for slug %q", tc.slug)
			continue
		}
		te, ok := err.(*TrackerError)
		if !ok {
			t.Errorf("expected *TrackerError for slug %q, got %T", tc.slug, err)
			continue
		}
		if te.Kind != ErrGitHubInvalidSlug {
			t.Errorf("expected %s, got %s", ErrGitHubInvalidSlug, te.Kind)
		}
	}
}

func TestGitHubFetchCandidateIssues_FetchesOpenByDefault(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedURL == "" {
		t.Fatal("expected a request to be made")
	}
	if got := receivedURL; !strings.Contains(got, "state=open") {
		t.Errorf("expected state=open in URL, got %q", got)
	}
}

func TestGitHubFetchCandidateIssues_ClosedState(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedURL, "state=closed") {
		t.Errorf("expected state=closed in URL, got %q", receivedURL)
	}
}

func TestGitHubFetchCandidateIssues_BothStates(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open", "closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(receivedURL, "state=all") {
		t.Errorf("expected state=all in URL, got %q", receivedURL)
	}
}

func TestGitHubFetchCandidateIssues_ReturnsIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]githubIssue{
			{
				Number:    42,
				Title:     "Fix the bug",
				State:     "open",
				HTMLURL:   "https://github.com/owner/repo/issues/42",
				Labels:    []githubLabel{{Name: "bug"}},
				CreatedAt: "2024-01-01T00:00:00Z",
				UpdatedAt: "2024-01-02T00:00:00Z",
			},
		})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	issue := issues[0]
	if issue.ID != "42" {
		t.Errorf("expected ID=42, got %q", issue.ID)
	}
	if issue.Title != "Fix the bug" {
		t.Errorf("expected title='Fix the bug', got %q", issue.Title)
	}
	if issue.State != "open" {
		t.Errorf("expected state=open, got %q", issue.State)
	}
	if len(issue.Labels) != 1 || issue.Labels[0] != "bug" {
		t.Errorf("expected labels=[bug], got %v", issue.Labels)
	}
}

func TestGitHubFetchCandidateIssues_SkipsPullRequests(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pr := &struct{}{}
		json.NewEncoder(w).Encode([]githubIssue{
			{Number: 1, Title: "PR", State: "open", PullRequest: pr},
			{Number: 2, Title: "Issue", State: "open"},
		})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue (PR skipped), got %d", len(issues))
	}
	if issues[0].ID != "2" {
		t.Errorf("expected ID=2, got %q", issues[0].ID)
	}
}

func TestGitHubFetchCandidateIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			// Return a full page (100 items) to trigger pagination
			issues := make([]githubIssue, githubDefaultPageSize)
			for i := range issues {
				issues[i] = githubIssue{
					Number: i + 1,
					Title:  "Issue",
					State:  "open",
				}
			}
			json.NewEncoder(w).Encode(issues)
		} else {
			// Return a partial page to signal end of results
			json.NewEncoder(w).Encode([]githubIssue{
				{Number: 101, Title: "Last Issue", State: "open"},
			})
		}
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 101 {
		t.Errorf("expected 101 issues across pages, got %d", len(issues))
	}
	if callCount != 2 {
		t.Errorf("expected 2 HTTP calls, got %d", callCount)
	}
}

func TestGitHubFetchIssueStatesByIDs_EmptyList(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchIssueStatesByIDs([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty, got %d issues", len(issues))
	}
	if callCount != 0 {
		t.Errorf("expected no HTTP calls, got %d", callCount)
	}
}

func TestGitHubFetchIssueStatesByIDs_FetchesByNumber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubIssue{
			Number:  42,
			Title:   "A bug",
			State:   "closed",
			HTMLURL: "https://github.com/owner/repo/issues/42",
		})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchIssueStatesByIDs([]string{"42"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].State != "closed" {
		t.Errorf("expected state=closed, got %q", issues[0].State)
	}
	if issues[0].ID != "42" {
		t.Errorf("expected ID=42, got %q", issues[0].ID)
	}
}

func TestGitHubFetchIssueStatesByIDs_SkipsInvalidIDs(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		json.NewEncoder(w).Encode(githubIssue{Number: 1, State: "open"})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	// "not-a-number" should be silently skipped; "1" should be fetched
	issues, err := client.FetchIssueStatesByIDs([]string{"not-a-number", "1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(issues))
	}
	if callCount != 1 {
		t.Errorf("expected 1 HTTP call (invalid ID skipped), got %d", callCount)
	}
}

func TestGitHubFetchIssuesByStates_EmptyStates(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	issues, err := client.FetchIssuesByStates("owner/repo", []string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected empty, got %d issues", len(issues))
	}
	if callCount != 0 {
		t.Errorf("expected no HTTP calls, got %d", callCount)
	}
}

func TestGitHubClient_HTTP401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Requires authentication"))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "bad-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubAuthFailed {
		t.Errorf("expected %s, got %s", ErrGitHubAuthFailed, te.Kind)
	}
}

func TestGitHubClient_HTTP404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubNotFound {
		t.Errorf("expected %s, got %s", ErrGitHubNotFound, te.Kind)
	}
}

func TestGitHubClient_HTTP429_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("rate limit exceeded"))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubRateLimited {
		t.Errorf("expected %s, got %s", ErrGitHubRateLimited, te.Kind)
	}
}

func TestGitHubClient_HTTP403_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubRateLimited {
		t.Errorf("expected %s, got %s", ErrGitHubRateLimited, te.Kind)
	}
}

func TestGitHubClient_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("{invalid json"))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubUnknownPayload {
		t.Errorf("expected %s, got %s", ErrGitHubUnknownPayload, te.Kind)
	}
}

func TestGitHubClient_ConnectionRefused(t *testing.T) {
	client := &GitHubClient{
		baseURL:    "http://127.0.0.1:1",
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err == nil {
		t.Fatal("expected error")
	}
	te, ok := err.(*TrackerError)
	if !ok {
		t.Fatalf("expected *TrackerError, got %T", err)
	}
	if te.Kind != ErrGitHubAPIRequest {
		t.Errorf("expected %s, got %s", ErrGitHubAPIRequest, te.Kind)
	}
}

func TestGitHubClient_BearerAuth(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "ghp_testtoken",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if receivedAuth != "Bearer ghp_testtoken" {
		t.Errorf("expected 'Bearer ghp_testtoken', got %q", receivedAuth)
	}
}

func TestGitHubFetchIssueStatesByIDs_NotFoundSkipped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	// 404 for a specific issue should be silently skipped
	issues, err := client.FetchIssueStatesByIDs([]string{"999"})
	if err != nil {
		t.Fatalf("expected no error for 404, got %v", err)
	}
	if len(issues) != 0 {
		t.Errorf("expected 0 issues, got %d", len(issues))
	}
}

// TestNewGitHubClient_LabelStates verifies that non-native active/terminal
// states are stored as label states with terminal states taking precedence.
func TestNewGitHubClient_LabelStates(t *testing.T) {
	active := []string{"in-progress", "review"}
	terminal := []string{"closed", "done"}

	client, err := NewGitHubClient("owner/repo", "token", active, terminal)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// "done" is a non-native terminal state; "in-progress" and "review" are
	// non-native active states. "closed" is native and should be excluded.
	// Terminal states come first in labelStates.
	if len(client.labelStates) != 3 {
		t.Fatalf("expected 3 label states, got %d: %v", len(client.labelStates), client.labelStates)
	}
	if client.labelStates[0] != "done" {
		t.Errorf("expected labelStates[0]=done (terminal first), got %q", client.labelStates[0])
	}
}

// TestNewGitHubClient_NoLabelStates verifies that when only native states are
// configured, labelStates is empty.
func TestNewGitHubClient_NoLabelStates(t *testing.T) {
	client, err := NewGitHubClient("owner/repo", "token", []string{"open"}, []string{"closed"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(client.labelStates) != 0 {
		t.Errorf("expected no label states, got %v", client.labelStates)
	}
}

// TestGitHubFetchCandidateIssues_LabelFilter verifies that when label-based
// active states are configured, the labels query parameter is included in the URL.
func TestGitHubFetchCandidateIssues_LabelFilter(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"in-progress", "review"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should default to state=open and filter by labels
	if !strings.Contains(receivedURL, "state=open") {
		t.Errorf("expected state=open in URL, got %q", receivedURL)
	}
	if !strings.Contains(receivedURL, "labels=") {
		t.Errorf("expected labels= parameter in URL, got %q", receivedURL)
	}
	if !strings.Contains(receivedURL, "in-progress") {
		t.Errorf("expected in-progress label in URL, got %q", receivedURL)
	}
	if !strings.Contains(receivedURL, "review") {
		t.Errorf("expected review label in URL, got %q", receivedURL)
	}
}

// TestGitHubFetchCandidateIssues_NativePlusLabelFilter verifies that mixing a
// native state ("open") with a label state results in a label-filtered request.
func TestGitHubFetchCandidateIssues_NativePlusLabelFilter(t *testing.T) {
	var receivedURL string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedURL = r.URL.String()
		json.NewEncoder(w).Encode([]githubIssue{})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:    server.URL,
		token:      "test-token",
		owner:      "owner",
		repo:       "repo",
		httpClient: &http.Client{},
	}

	_, err := client.FetchCandidateIssues("owner/repo", []string{"open", "in-progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(receivedURL, "state=open") {
		t.Errorf("expected state=open in URL, got %q", receivedURL)
	}
	if !strings.Contains(receivedURL, "labels=in-progress") {
		t.Errorf("expected labels=in-progress in URL, got %q", receivedURL)
	}
}

// TestGitHubFetchCandidateIssues_LabelStateResolution verifies that when an
// issue has a label matching a configured label state, its State field reflects
// the label name rather than the native GitHub state.
func TestGitHubFetchCandidateIssues_LabelStateResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]githubIssue{
			{
				Number: 1,
				Title:  "Work item",
				State:  "open",
				Labels: []githubLabel{{Name: "in-progress"}},
			},
		})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:     server.URL,
		token:       "test-token",
		owner:       "owner",
		repo:        "repo",
		httpClient:  &http.Client{},
		labelStates: []string{"in-progress"},
	}

	issues, err := client.FetchCandidateIssues("owner/repo", []string{"in-progress"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].State != "in-progress" {
		t.Errorf("expected state=in-progress (resolved from label), got %q", issues[0].State)
	}
}

// TestGitHubFetchIssueStatesByIDs_LabelStateResolution verifies that
// FetchIssueStatesByIDs resolves state from labels using the stored labelStates.
func TestGitHubFetchIssueStatesByIDs_LabelStateResolution(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(githubIssue{
			Number: 5,
			Title:  "In flight",
			State:  "open",
			Labels: []githubLabel{{Name: "in-progress"}},
		})
	}))
	defer server.Close()

	client := &GitHubClient{
		baseURL:     server.URL,
		token:       "test-token",
		owner:       "owner",
		repo:        "repo",
		httpClient:  &http.Client{},
		labelStates: []string{"in-progress"},
	}

	issues, err := client.FetchIssueStatesByIDs([]string{"5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].State != "in-progress" {
		t.Errorf("expected state=in-progress (resolved from label), got %q", issues[0].State)
	}
}
