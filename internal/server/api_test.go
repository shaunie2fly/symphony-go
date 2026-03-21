package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/symphony-go/symphony/internal/agent"
	"github.com/symphony-go/symphony/internal/cmux"
	"github.com/symphony-go/symphony/internal/config"
	"github.com/symphony-go/symphony/internal/orchestrator"
	"github.com/symphony-go/symphony/internal/tracker"
	"github.com/symphony-go/symphony/internal/workflow"
	"github.com/symphony-go/symphony/internal/workspace"
)

func setupTestServer(t *testing.T) (*Server, string) {
	t.Helper()

	cfg := config.DefaultConfig()
	cfg.Tracker.Kind = "linear"
	cfg.Tracker.APIKey = "test"
	cfg.Tracker.ProjectSlug = "proj"

	wf := &workflow.WorkflowDefinition{
		Config:         map[string]any{},
		PromptTemplate: "test",
	}

	root := t.TempDir()
	hooks := &cfg.Hooks
	workspaceMgr := workspace.NewManager(root, hooks)
	mockTracker := &testMockTracker{}
	mockLauncher := &testMockLauncher{}

	cmuxMgr := cmux.New(nil)
	orch := orchestrator.New(&cfg, wf, mockTracker, mockLauncher, workspaceMgr, cmuxMgr)

	srv := New(0, orch) // ephemeral port
	if err := srv.Start(); err != nil {
		t.Fatalf("failed to start test server: %v", err)
	}
	t.Cleanup(func() { srv.Shutdown(context.Background()) })

	baseURL := "http://127.0.0.1:" + itoa(srv.Port())
	return srv, baseURL
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}

// Minimal mocks for server tests
type testMockTracker struct{}

func (m *testMockTracker) FetchCandidateIssues(string, []string) ([]tracker.Issue, error) {
	return nil, nil
}
func (m *testMockTracker) FetchIssueStatesByIDs([]string) ([]tracker.Issue, error) {
	return nil, nil
}
func (m *testMockTracker) FetchIssuesByStates(string, []string) ([]tracker.Issue, error) {
	return nil, nil
}

type testMockLauncher struct{}

func (m *testMockLauncher) Launch(_ context.Context, _ agent.RunParams, _ chan<- agent.OrchestratorEvent) error {
	return nil
}

func TestGetState_ReturnsJSON(t *testing.T) {
	_, baseURL := setupTestServer(t)

	resp, err := http.Get(baseURL + "/api/v1/state")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got %q", ct)
	}

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if _, ok := result["generated_at"]; !ok {
		t.Error("expected generated_at field")
	}
	if _, ok := result["counts"]; !ok {
		t.Error("expected counts field")
	}
	if _, ok := result["running"]; !ok {
		t.Error("expected running field")
	}
}

func TestGetIssue_NotFound(t *testing.T) {
	_, baseURL := setupTestServer(t)

	resp, err := http.Get(baseURL + "/api/v1/UNKNOWN-123")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if _, ok := result["error"]; !ok {
		t.Error("expected error envelope")
	}
}

func TestPostRefresh_Returns202(t *testing.T) {
	_, baseURL := setupTestServer(t)

	resp, err := http.Post(baseURL+"/api/v1/refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		t.Errorf("expected 202, got %d", resp.StatusCode)
	}

	var result map[string]any
	json.NewDecoder(resp.Body).Decode(&result)
	if result["queued"] != true {
		t.Error("expected queued=true")
	}
}

func TestDashboard_ReturnsHTML(t *testing.T) {
	_, baseURL := setupTestServer(t)

	resp, err := http.Get(baseURL + "/")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "text/html; charset=utf-8" {
		t.Errorf("expected text/html, got %q", ct)
	}
}

func TestAgentCallback_Returns200(t *testing.T) {
_, baseURL := setupTestServer(t)

body := strings.NewReader(`{"status":"completed","message":"Finished the task"}`)
resp, err := http.Post(baseURL+"/api/internal/agent/callback", "application/json", body)
if err != nil {
t.Fatalf("request failed: %v", err)
}
defer resp.Body.Close()

if resp.StatusCode != http.StatusOK {
t.Errorf("expected 200, got %d", resp.StatusCode)
}

var result map[string]any
json.NewDecoder(resp.Body).Decode(&result)
if result["acknowledged"] != true {
t.Error("expected acknowledged=true")
}
if result["status"] != "completed" {
t.Errorf("expected status=completed, got %v", result["status"])
}
}

func TestAgentCallback_MissingStatus_Returns400(t *testing.T) {
_, baseURL := setupTestServer(t)

body := strings.NewReader(`{"message":"no status"}`)
resp, err := http.Post(baseURL+"/api/internal/agent/callback", "application/json", body)
if err != nil {
t.Fatalf("request failed: %v", err)
}
defer resp.Body.Close()

if resp.StatusCode != http.StatusBadRequest {
t.Errorf("expected 400, got %d", resp.StatusCode)
}
}

func TestAgentCallback_InvalidJSON_Returns400(t *testing.T) {
_, baseURL := setupTestServer(t)

body := strings.NewReader(`not-json`)
resp, err := http.Post(baseURL+"/api/internal/agent/callback", "application/json", body)
if err != nil {
t.Fatalf("request failed: %v", err)
}
defer resp.Body.Close()

if resp.StatusCode != http.StatusBadRequest {
t.Errorf("expected 400, got %d", resp.StatusCode)
}
}
