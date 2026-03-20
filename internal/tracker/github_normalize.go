package tracker

import (
	"fmt"
	"strconv"
	"strings"
)

// normalizeGitHubIssue converts a raw GitHub issue to the domain Issue model.
func normalizeGitHubIssue(raw githubIssue, owner, repo string) Issue {
	id := strconv.Itoa(raw.Number)
	identifier := fmt.Sprintf("%s/%s#%d", owner, repo, raw.Number)

	issue := Issue{
		ID:         id,
		Identifier: identifier,
		Title:      raw.Title,
		State:      raw.State,
		Labels:     []string{},
		BlockedBy:  []Blocker{},
	}

	// Description
	if raw.Body != nil && *raw.Body != "" {
		issue.Description = raw.Body
	}

	// URL
	if raw.HTMLURL != "" {
		url := raw.HTMLURL
		issue.URL = &url
	}

	// Labels: normalized to lowercase
	for _, l := range raw.Labels {
		if l.Name != "" {
			issue.Labels = append(issue.Labels, strings.ToLower(l.Name))
		}
	}

	// Priority: GitHub does not have a native priority field
	issue.Priority = nil

	// BranchName: GitHub does not provide a branch name on issues
	issue.BranchName = nil

	// Timestamps
	if raw.CreatedAt != "" {
		issue.CreatedAt = parseTimestamp(&raw.CreatedAt)
	}
	if raw.UpdatedAt != "" {
		issue.UpdatedAt = parseTimestamp(&raw.UpdatedAt)
	}

	return issue
}
