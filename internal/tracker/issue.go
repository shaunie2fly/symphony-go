package tracker

import (
	"fmt"
	"time"

	"github.com/symphony-go/symphony/internal/config"
)

// Issue is the normalized issue record used by orchestration and prompt rendering.
type Issue struct {
	ID          string     `json:"id"`
	Identifier  string     `json:"identifier"`
	Title       string     `json:"title"`
	Description *string    `json:"description"`
	Priority    *int       `json:"priority"`
	State       string     `json:"state"`
	BranchName  *string    `json:"branch_name"`
	URL         *string    `json:"url"`
	Labels      []string   `json:"labels"`
	BlockedBy   []Blocker  `json:"blocked_by"`
	CreatedAt   *time.Time `json:"created_at"`
	UpdatedAt   *time.Time `json:"updated_at"`
}

// Blocker represents a blocking issue reference.
type Blocker struct {
	ID         *string `json:"id"`
	Identifier *string `json:"identifier"`
	State      *string `json:"state"`
}

// TrackerClient is the interface for issue tracker operations.
type TrackerClient interface {
	FetchCandidateIssues(projectSlug string, activeStates []string) ([]Issue, error)
	FetchIssueStatesByIDs(ids []string) ([]Issue, error)
	FetchIssuesByStates(projectSlug string, states []string) ([]Issue, error)
}

// NewTrackerClient creates the appropriate TrackerClient based on config.
func NewTrackerClient(cfg *config.TrackerConfig) (TrackerClient, error) {
	switch cfg.Kind {
	case "linear":
		return NewLinearClient(cfg.Endpoint, cfg.APIKey), nil
	case "jira":
		return NewJiraClient(cfg.Endpoint, cfg.Email, cfg.APIKey), nil
	case "github":
		return NewGitHubClient(cfg.ProjectSlug, cfg.APIKey)
	default:
		return nil, &TrackerError{
			Kind:    ErrUnsupportedTrackerKind,
			Message: fmt.Sprintf("unsupported tracker kind: %q", cfg.Kind),
		}
	}
}
