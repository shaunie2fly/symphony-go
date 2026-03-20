package tracker

import "fmt"

// Error kind constants for tracker operations.
const (
	ErrUnsupportedTrackerKind  = "unsupported_tracker_kind"
	ErrMissingAPIKey           = "missing_tracker_api_key"
	ErrMissingProjectSlug      = "missing_tracker_project_slug"
	ErrLinearAPIRequest        = "linear_api_request"
	ErrLinearAPIStatus         = "linear_api_status"
	ErrLinearGraphQLErrors     = "linear_graphql_errors"
	ErrLinearUnknownPayload    = "linear_unknown_payload"
	ErrLinearMissingEndCursor  = "linear_missing_end_cursor"

	ErrJiraAuthFailed     = "jira_auth_failed"
	ErrJiraForbidden      = "jira_forbidden"
	ErrJiraNotFound       = "jira_not_found"
	ErrJiraBadRequest     = "jira_bad_request"
	ErrJiraAPIStatus      = "jira_api_status"
	ErrJiraAPIRequest     = "jira_api_request"
	ErrJiraUnknownPayload = "jira_unknown_payload"

	ErrGitHubInvalidSlug    = "github_invalid_slug"
	ErrGitHubAPIRequest     = "github_api_request"
	ErrGitHubAPIStatus      = "github_api_status"
	ErrGitHubAuthFailed     = "github_auth_failed"
	ErrGitHubForbidden      = "github_forbidden"
	ErrGitHubNotFound       = "github_not_found"
	ErrGitHubRateLimited    = "github_rate_limited"
	ErrGitHubUnknownPayload = "github_unknown_payload"
)

// TrackerError is a typed error for tracker operations.
type TrackerError struct {
	Kind    string
	Message string
	Cause   error
}

func (e *TrackerError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

func (e *TrackerError) Unwrap() error {
	return e.Cause
}

func newTrackerError(kind, message string, cause error) *TrackerError {
	return &TrackerError{Kind: kind, Message: message, Cause: cause}
}
