package utils

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// jiraBaseURL can be overridden in tests to point at a httptest server.
var jiraBaseURL string

const jiraMaxResults = 50
const jiraMaxIssues = 1000

// jiraSearchRequest is the POST body for the Jira search/jql endpoint.
type jiraSearchRequest struct {
	JQL           string   `json:"jql"`
	MaxResults    int      `json:"maxResults"`
	Fields        []string `json:"fields"`
	NextPageToken string   `json:"nextPageToken,omitempty"`
}

// jiraSearchResponse is the relevant subset of the Jira search response.
type jiraSearchResponse struct {
	Issues        []jiraIssue `json:"issues"`
	NextPageToken string      `json:"nextPageToken"`
}

type jiraIssue struct {
	Key    string          `json:"key"`
	Fields jiraIssueFields `json:"fields"`
}

type jiraIssueFields struct {
	Summary     string          `json:"summary"`
	Description json.RawMessage `json:"description"`
	Status      jiraName        `json:"status"`
	Assignee    *jiraAssignee   `json:"assignee"`
	IssueType   jiraName        `json:"issuetype"`
	Priority    jiraName        `json:"priority"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
}

type jiraName struct {
	Name string `json:"name"`
}

type jiraAssignee struct {
	DisplayName string `json:"displayName"`
}

// FetchJiraIssues fetches issues from Jira Cloud using the REST API and returns
// a structured IssuesResult containing a summary list and per-issue JSON.
// The token parameter is the raw auth credential (email:token for PAT, or bare API key).
func FetchJiraIssues(ctx context.Context, src *recipes.JiraIssuesSource, token string) (*IssuesResult, error) {
	if src == nil {
		return nil, fmt.Errorf("jira issues source cannot be nil")
	}

	org := strings.TrimSpace(src.GetOrganization())
	if org == "" {
		return nil, fmt.Errorf("jira organization cannot be empty")
	}

	projects := src.GetProjects()
	slog.Debug("Fetching Jira issues", "organization", org, "projects", projects)

	baseURL := jiraBaseURL
	if baseURL == "" {
		baseURL = fmt.Sprintf("https://%s.atlassian.net", org)
	}

	jql := buildJQL(projects, src.GetFilter())

	var allIssues []jiraIssue
	nextPageToken := ""

	for {
		reqBody := jiraSearchRequest{
			JQL:           jql,
			MaxResults:    jiraMaxResults,
			Fields:        []string{"summary", "description", "status", "assignee", "created", "updated", "issuetype", "priority"},
			NextPageToken: nextPageToken,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jira request: %w", err)
		}

		url := baseURL + "/rest/api/3/search/jql"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create jira request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		if token != "" {
			if strings.Contains(token, ":") {
				// email:token format (personal access token) → Basic Auth
				req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(token)))
			} else {
				// API key → Basic Auth with empty username
				req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(":"+token)))
			}
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from jira: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read jira response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("jira API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var searchResp jiraSearchResponse
		if err := json.Unmarshal(respBody, &searchResp); err != nil {
			return nil, fmt.Errorf("failed to parse jira response: %w", err)
		}

		allIssues = append(allIssues, searchResp.Issues...)
		slog.Debug("Jira pagination", "issuesSoFar", len(allIssues))

		if searchResp.NextPageToken == "" || len(allIssues) >= jiraMaxIssues {
			break
		}
		nextPageToken = searchResp.NextPageToken
	}

	slog.Debug("Jira issues fetched", "count", len(allIssues))

	result := &IssuesResult{
		Summary: make([]IssueSummary, 0, len(allIssues)),
		Issues:  make(map[string]string, len(allIssues)),
	}
	for _, issue := range allIssues {
		result.Summary = append(result.Summary, IssueSummary{
			ID:    issue.Key,
			Title: issue.Fields.Summary,
		})
		raw, err := json.MarshalIndent(issue, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal jira issue %s: %w", issue.Key, err)
		}
		result.Issues[issue.Key] = string(raw)
	}
	return result, nil
}

// buildJQL constructs a JQL query string from projects and filters.
func buildJQL(projects []string, filter *recipes.IssuesFilter) string {
	var clauses []string

	if len(projects) > 0 {
		quoted := make([]string, len(projects))
		for i, p := range projects {
			quoted[i] = `"` + p + `"`
		}
		clauses = append(clauses, fmt.Sprintf("project IN (%s)", strings.Join(quoted, ", ")))
	}

	if filter != nil {
		if filter.HasCreatedAtFilter() {
			cf := filter.GetCreatedAtFilter()
			if cf.HasFrom() {
				clauses = append(clauses, fmt.Sprintf("created >= %q", formatTimestampForJQL(cf.GetFrom())))
			}
			if cf.HasTo() {
				clauses = append(clauses, fmt.Sprintf("created <= %q", formatTimestampForJQL(cf.GetTo())))
			}
		}
		if filter.HasUpdatedAtFilter() {
			uf := filter.GetUpdatedAtFilter()
			if uf.HasFrom() {
				clauses = append(clauses, fmt.Sprintf("updated >= %q", formatTimestampForJQL(uf.GetFrom())))
			}
			if uf.HasTo() {
				clauses = append(clauses, fmt.Sprintf("updated <= %q", formatTimestampForJQL(uf.GetTo())))
			}
		}
	}

	if len(clauses) == 0 {
		// Jira Cloud rejects unbounded JQL queries; default to recent issues.
		return "created >= -30d ORDER BY created DESC"
	}
	return strings.Join(clauses, " AND ") + " ORDER BY created DESC"
}

// formatTimestampForJQL formats a protobuf timestamp for JQL date queries.
func formatTimestampForJQL(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format("2006-01-02")
}
