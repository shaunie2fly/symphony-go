package tracker

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	githubDefaultBaseURL  = "https://api.github.com"
	githubDefaultPageSize = 100
)

// GitHubClient implements TrackerClient for GitHub's REST Issues API.
type GitHubClient struct {
	baseURL    string
	token      string
	owner      string
	repo       string
	httpClient *http.Client
}

// NewGitHubClient creates a new GitHub REST API client.
// projectSlug must be in "owner/repo" format.
func NewGitHubClient(projectSlug, token string) (*GitHubClient, error) {
	parts := strings.SplitN(projectSlug, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, newTrackerError(ErrGitHubInvalidSlug,
			fmt.Sprintf("project_slug must be in owner/repo format, got: %q", projectSlug), nil)
	}
	return &GitHubClient{
		baseURL:    githubDefaultBaseURL,
		token:      token,
		owner:      parts[0],
		repo:       parts[1],
		httpClient: &http.Client{Timeout: defaultNetTimeout},
	}, nil
}

// githubIssue is a single issue from the GitHub REST API.
type githubIssue struct {
	Number    int            `json:"number"`
	Title     string         `json:"title"`
	Body      *string        `json:"body"`
	State     string         `json:"state"`
	HTMLURL   string         `json:"html_url"`
	Labels    []githubLabel  `json:"labels"`
	CreatedAt string         `json:"created_at"`
	UpdatedAt string         `json:"updated_at"`
	User      *githubUser    `json:"user"`
	PullRequest *struct{}    `json:"pull_request,omitempty"`
}

// githubLabel is a label on a GitHub issue.
type githubLabel struct {
	Name string `json:"name"`
}

// githubUser is a GitHub user reference.
type githubUser struct {
	Login string `json:"login"`
}

// FetchCandidateIssues fetches issues in active states for the configured repository.
// The activeStates parameter is translated to GitHub's native state filter:
// "open" and "closed" are treated as native states; if neither is present,
// "open" is used as the default.
func (c *GitHubClient) FetchCandidateIssues(_ string, activeStates []string) ([]Issue, error) {
	state := toGitHubState(activeStates)
	return c.fetchAllPages(state)
}

// FetchIssueStatesByIDs fetches current states for specific issue numbers.
// The ids parameter contains issue numbers as strings (e.g., "1", "42").
func (c *GitHubClient) FetchIssueStatesByIDs(ids []string) ([]Issue, error) {
	if len(ids) == 0 {
		return []Issue{}, nil
	}

	var issues []Issue
	for _, id := range ids {
		num, err := strconv.Atoi(id)
		if err != nil {
			continue
		}

		issue, err := c.fetchIssue(num)
		if err != nil {
			te, ok := err.(*TrackerError)
			if ok && te.Kind == ErrGitHubNotFound {
				continue
			}
			return nil, err
		}
		// Skip pull requests that appear in the issues endpoint
		if issue.PullRequest != nil {
			continue
		}
		issues = append(issues, normalizeGitHubIssue(*issue, c.owner, c.repo))
	}

	return issues, nil
}

// FetchIssuesByStates fetches issues in specific states for the configured repository.
func (c *GitHubClient) FetchIssuesByStates(_ string, states []string) ([]Issue, error) {
	if len(states) == 0 {
		return []Issue{}, nil
	}

	state := toGitHubState(states)
	return c.fetchAllPages(state)
}

// fetchAllPages fetches all pages of issues with the given GitHub state filter.
func (c *GitHubClient) fetchAllPages(state string) ([]Issue, error) {
	var allIssues []Issue
	page := 1

	for {
		url := fmt.Sprintf("%s/repos/%s/%s/issues?state=%s&per_page=%d&page=%d",
			c.baseURL, c.owner, c.repo, state, githubDefaultPageSize, page)

		raw, err := c.doRequest(url)
		if err != nil {
			return nil, err
		}

		var batch []githubIssue
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, newTrackerError(ErrGitHubUnknownPayload, "failed to parse issues response", err)
		}

		for _, gi := range batch {
			// Skip pull requests that appear in the issues endpoint
			if gi.PullRequest != nil {
				continue
			}
			allIssues = append(allIssues, normalizeGitHubIssue(gi, c.owner, c.repo))
		}

		if len(batch) < githubDefaultPageSize {
			break
		}
		page++
	}

	return allIssues, nil
}

// fetchIssue fetches a single issue by number.
func (c *GitHubClient) fetchIssue(number int) (*githubIssue, error) {
	url := fmt.Sprintf("%s/repos/%s/%s/issues/%d", c.baseURL, c.owner, c.repo, number)

	raw, err := c.doRequest(url)
	if err != nil {
		return nil, err
	}

	var issue githubIssue
	if err := json.Unmarshal(raw, &issue); err != nil {
		return nil, newTrackerError(ErrGitHubUnknownPayload, "failed to parse issue response", err)
	}

	return &issue, nil
}

// doRequest performs an authenticated GET request and returns the response body.
func (c *GitHubClient) doRequest(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, newTrackerError(ErrGitHubAPIRequest, "failed to create request", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, newTrackerError(ErrGitHubAPIRequest, "request failed", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, newTrackerError(ErrGitHubAPIRequest, "failed to read response", err)
	}

	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusUnauthorized:
		return nil, newTrackerError(ErrGitHubAuthFailed,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	case http.StatusForbidden:
		if strings.Contains(string(body), "rate limit") {
			return nil, newTrackerError(ErrGitHubRateLimited,
				fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
		}
		return nil, newTrackerError(ErrGitHubForbidden,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	case http.StatusNotFound:
		return nil, newTrackerError(ErrGitHubNotFound,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	case http.StatusTooManyRequests:
		return nil, newTrackerError(ErrGitHubRateLimited,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	default:
		return nil, newTrackerError(ErrGitHubAPIStatus,
			fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)), nil)
	}
}

// toGitHubState translates a list of state names to a GitHub state query parameter.
// GitHub supports "open", "closed", and "all".
func toGitHubState(states []string) string {
	var hasOpen, hasClosed bool
	for _, s := range states {
		switch strings.ToLower(s) {
		case "open":
			hasOpen = true
		case "closed":
			hasClosed = true
		}
	}
	switch {
	case hasOpen && hasClosed:
		return "all"
	case hasClosed:
		return "closed"
	default:
		return "open"
	}
}
