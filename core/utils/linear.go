package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// linearBaseURL can be overridden in tests to point at a httptest server.
var linearBaseURL string

const linearMaxIssues = 1000

const linearQuery = `query($filter: IssueFilter, $after: String) {
  issues(filter: $filter, after: $after, first: 50) {
    nodes {
      identifier
      title
      description
      state { name }
      assignee { name }
      createdAt
      updatedAt
      priority
      priorityLabel
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}`

type linearGraphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

type linearGraphQLResponse struct {
	Data   *linearData    `json:"data"`
	Errors []linearGQLErr `json:"errors"`
}

type linearData struct {
	Issues linearIssuesData `json:"issues"`
}

type linearIssuesData struct {
	Nodes    []linearIssue  `json:"nodes"`
	PageInfo linearPageInfo `json:"pageInfo"`
}

type linearIssue struct {
	Identifier    string      `json:"identifier"`
	Title         string      `json:"title"`
	Description   string      `json:"description"`
	State         *linearName `json:"state"`
	Assignee      *linearName `json:"assignee"`
	CreatedAt     string      `json:"createdAt"`
	UpdatedAt     string      `json:"updatedAt"`
	Priority      int         `json:"priority"`
	PriorityLabel string      `json:"priorityLabel"`
}

type linearName struct {
	Name string `json:"name"`
}

type linearPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type linearGQLErr struct {
	Message string `json:"message"`
}

// FetchLinearIssues fetches issues from the Linear GraphQL API and returns
// a structured IssuesResult containing a summary list and per-issue JSON.
// The token parameter is the Linear API key used in the Authorization header.
func FetchLinearIssues(ctx context.Context, src *recipes.LinearIssuesSource, token string) (*IssuesResult, error) {
	if src == nil {
		return nil, fmt.Errorf("linear issues source cannot be nil")
	}

	teams := src.GetTeams()
	slog.Debug("Fetching Linear issues", "teams", teams)

	if token == "" {
		return nil, fmt.Errorf("linear API requires authentication: set the auth token env var")
	}

	baseURL := linearBaseURL
	if baseURL == "" {
		baseURL = "https://api.linear.app/graphql"
	}

	filter := buildLinearFilter(teams, src.GetFilter())

	var allIssues []linearIssue
	var cursor string

	for {
		variables := map[string]any{}
		if filter != nil {
			variables["filter"] = filter
		}
		if cursor != "" {
			variables["after"] = cursor
		}

		reqBody := linearGraphQLRequest{
			Query:     linearQuery,
			Variables: variables,
		}

		body, err := json.Marshal(reqBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal linear request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("failed to create linear request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", token)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch from linear: %w", err)
		}

		respBody, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read linear response: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("linear API returned status %d: %s", resp.StatusCode, string(respBody))
		}

		var gqlResp linearGraphQLResponse
		if err := json.Unmarshal(respBody, &gqlResp); err != nil {
			return nil, fmt.Errorf("failed to parse linear response: %w", err)
		}

		if len(gqlResp.Errors) > 0 {
			msgs := make([]string, len(gqlResp.Errors))
			for i, e := range gqlResp.Errors {
				msgs[i] = e.Message
			}
			return nil, fmt.Errorf("linear API returned errors: %s", strings.Join(msgs, "; "))
		}

		if gqlResp.Data == nil {
			break
		}

		allIssues = append(allIssues, gqlResp.Data.Issues.Nodes...)
		slog.Debug("Linear pagination", "cursor", cursor, "issuesSoFar", len(allIssues))

		if !gqlResp.Data.Issues.PageInfo.HasNextPage || len(allIssues) >= linearMaxIssues {
			break
		}
		cursor = gqlResp.Data.Issues.PageInfo.EndCursor
	}

	slog.Debug("Linear issues fetched", "count", len(allIssues))

	result := &IssuesResult{
		Summary: make([]IssueSummary, 0, len(allIssues)),
		Issues:  make(map[string]string, len(allIssues)),
	}
	for _, issue := range allIssues {
		result.Summary = append(result.Summary, IssueSummary{
			ID:    issue.Identifier,
			Title: issue.Title,
		})
		raw, err := json.MarshalIndent(issue, "", "  ")
		if err != nil {
			return nil, fmt.Errorf("failed to marshal linear issue %s: %w", issue.Identifier, err)
		}
		result.Issues[issue.Identifier] = string(raw)
	}
	return result, nil
}

// buildLinearFilter constructs a GraphQL filter object from teams and IssuesFilter.
func buildLinearFilter(teams []string, filter *recipes.IssuesFilter) map[string]any {
	f := map[string]any{}

	if len(teams) > 0 {
		f["team"] = map[string]any{
			"key": map[string]any{
				"in": teams,
			},
		}
	}

	if filter != nil {
		if filter.HasCreatedAtFilter() {
			cf := filter.GetCreatedAtFilter()
			dateFilter := map[string]any{}
			if cf.HasFrom() {
				dateFilter["gte"] = formatTimestampISO(cf.GetFrom())
			}
			if cf.HasTo() {
				dateFilter["lte"] = formatTimestampISO(cf.GetTo())
			}
			if len(dateFilter) > 0 {
				f["createdAt"] = dateFilter
			}
		}
		if filter.HasUpdatedAtFilter() {
			uf := filter.GetUpdatedAtFilter()
			dateFilter := map[string]any{}
			if uf.HasFrom() {
				dateFilter["gte"] = formatTimestampISO(uf.GetFrom())
			}
			if uf.HasTo() {
				dateFilter["lte"] = formatTimestampISO(uf.GetTo())
			}
			if len(dateFilter) > 0 {
				f["updatedAt"] = dateFilter
			}
		}
	}

	if len(f) == 0 {
		return nil
	}
	return f
}

// formatTimestampISO formats a protobuf timestamp as an ISO 8601 string.
func formatTimestampISO(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format("2006-01-02T15:04:05.000Z")
}
