package tracker

import (
	"fmt"
	"strconv"
	"strings"
)

// resolveGitHubState returns the effective state for a GitHub issue.
// When labelStates is non-empty, it checks the issue's labels against the list
// (in order) and returns the first matching label name as the state.
// Terminal label states are expected to appear before active label states in
// labelStates so that terminal always takes priority.
// If no label matches, the native GitHub state ("open"/"closed") is returned.
func resolveGitHubState(nativeState string, labels []githubLabel, labelStates []string) string {
	if len(labelStates) == 0 {
		return nativeState
	}
	labelSet := make(map[string]bool, len(labels))
	for _, l := range labels {
		if l.Name != "" {
			labelSet[strings.ToLower(l.Name)] = true
		}
	}
	for _, ls := range labelStates {
		if labelSet[strings.ToLower(ls)] {
			return strings.ToLower(ls)
		}
	}
	return nativeState
}

// normalizeGitHubIssue converts a raw GitHub issue to the domain Issue model.
// labelStates is an ordered list of non-native state names (labels) to check
// when resolving the effective state; terminal states should come before active
// states so they take priority.
func normalizeGitHubIssue(raw githubIssue, owner, repo string, labelStates []string) Issue {
	id := strconv.Itoa(raw.Number)
	identifier := fmt.Sprintf("%s/%s#%d", owner, repo, raw.Number)

	issue := Issue{
		ID:         id,
		Identifier: identifier,
		Title:      raw.Title,
		State:      resolveGitHubState(raw.State, raw.Labels, labelStates),
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
