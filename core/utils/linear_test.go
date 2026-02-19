package utils

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func linearSource(workspace string, teams []string, authEnvVar *string, filter *recipes.IssuesFilter) *recipes.LinearIssuesSource {
	b := recipes.LinearIssuesSource_builder{
		Workspace:       workspace,
		Teams:           teams,
		Filter:          filter,
		AuthTokenEnvVar: authEnvVar,
	}
	return b.Build()
}

func linearGQLResponse(issues []linearIssue, hasNextPage bool, endCursor string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := linearGraphQLResponse{
			Data: &linearData{
				Issues: linearIssuesData{
					Nodes: issues,
					PageInfo: linearPageInfo{
						HasNextPage: hasNextPage,
						EndCursor:   endCursor,
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestFetchLinearIssues_NilSource(t *testing.T) {
	t.Parallel()
	_, err := FetchLinearIssues(context.Background(), nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear issues source cannot be nil")
}

func TestFetchLinearIssues_MissingAuthToken(t *testing.T) {
	t.Parallel()
	src := linearSource("my-workspace", nil, nil, nil)
	_, err := FetchLinearIssues(context.Background(), src, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear API requires authentication")
}

func TestFetchLinearIssues_Success(t *testing.T) {
	issues := []linearIssue{
		{
			Identifier:    "TEAM-1",
			Title:         "First issue",
			Description:   "Description of first issue",
			State:         &linearName{Name: "In Progress"},
			Assignee:      &linearName{Name: "Bob"},
			CreatedAt:     "2025-01-15T10:00:00.000Z",
			UpdatedAt:     "2025-02-01T14:30:00.000Z",
			Priority:      1,
			PriorityLabel: "Urgent",
		},
		{
			Identifier:    "TEAM-2",
			Title:         "Second issue",
			Description:   "Description of second issue",
			State:         &linearName{Name: "Done"},
			CreatedAt:     "2025-01-20T08:00:00.000Z",
			UpdatedAt:     "2025-02-05T09:00:00.000Z",
			Priority:      3,
			PriorityLabel: "Medium",
		},
	}

	server := httptest.NewServer(linearGQLResponse(issues, false, ""))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("my-workspace", nil, nil, nil)
	result, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.NoError(t, err)

	require.Len(t, result.Summary, 2)
	assert.Equal(t, "TEAM-1", result.Summary[0].ID)
	assert.Equal(t, "First issue", result.Summary[0].Title)
	assert.Equal(t, "TEAM-2", result.Summary[1].ID)
	assert.Equal(t, "Second issue", result.Summary[1].Title)

	require.Len(t, result.Issues, 2)
	assert.Contains(t, result.Issues["TEAM-1"], `"identifier": "TEAM-1"`)
	assert.Contains(t, result.Issues["TEAM-1"], `"description": "Description of first issue"`)
	assert.Contains(t, result.Issues["TEAM-2"], `"identifier": "TEAM-2"`)
}

func TestFetchLinearIssues_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(linearGQLResponse(nil, false, ""))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("my-workspace", nil, nil, nil)
	result, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.NoError(t, err)
	assert.Empty(t, result.Summary)
	assert.Empty(t, result.Issues)
}

func TestFetchLinearIssues_AuthHeader(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(linearGraphQLResponse{Data: &linearData{}})
	}))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("ws", nil, nil, nil)
	_, err := FetchLinearIssues(context.Background(), src, "my-linear-secret")
	require.NoError(t, err)
	assert.Equal(t, "my-linear-secret", receivedAuth)
}

func TestFetchLinearIssues_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("ws", nil, nil, nil)
	_, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear API returned status 500")
}

func TestFetchLinearIssues_GraphQLErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := linearGraphQLResponse{
			Errors: []linearGQLErr{
				{Message: "unauthorized"},
				{Message: "invalid query"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("ws", nil, nil, nil)
	_, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "linear API returned errors")
	assert.Contains(t, err.Error(), "unauthorized")
	assert.Contains(t, err.Error(), "invalid query")
}

func TestFetchLinearIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp linearGraphQLResponse
		if callCount == 1 {
			resp = linearGraphQLResponse{
				Data: &linearData{
					Issues: linearIssuesData{
						Nodes: []linearIssue{
							{Identifier: "T-1", Title: "Issue 1", State: &linearName{Name: "Open"}, PriorityLabel: "High"},
						},
						PageInfo: linearPageInfo{HasNextPage: true, EndCursor: "cursor1"},
					},
				},
			}
		} else {
			resp = linearGraphQLResponse{
				Data: &linearData{
					Issues: linearIssuesData{
						Nodes: []linearIssue{
							{Identifier: "T-2", Title: "Issue 2", State: &linearName{Name: "Done"}, PriorityLabel: "Low"},
						},
						PageInfo: linearPageInfo{HasNextPage: false},
					},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("ws", nil, nil, nil)
	result, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	require.Len(t, result.Summary, 2)
	assert.Equal(t, "T-1", result.Summary[0].ID)
	assert.Equal(t, "T-2", result.Summary[1].ID)
	assert.Contains(t, result.Issues["T-1"], `"identifier": "T-1"`)
	assert.Contains(t, result.Issues["T-2"], `"identifier": "T-2"`)
}

func TestFetchLinearIssues_FilterInRequest(t *testing.T) {
	var receivedVars map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req linearGraphQLRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		receivedVars = req.Variables
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(linearGraphQLResponse{Data: &linearData{}})
	}))
	defer server.Close()

	old := linearBaseURL
	linearBaseURL = server.URL
	defer func() { linearBaseURL = old }()

	src := linearSource("ws", []string{"ENG", "DESIGN"}, nil, nil)
	_, err := FetchLinearIssues(context.Background(), src, "test-token")
	require.NoError(t, err)

	filterVal, ok := receivedVars["filter"].(map[string]any)
	require.True(t, ok, "expected filter in variables")
	teamVal, ok := filterVal["team"].(map[string]any)
	require.True(t, ok, "expected team in filter")
	keyVal, ok := teamVal["key"].(map[string]any)
	require.True(t, ok, "expected key in team")
	inVal, ok := keyVal["in"].([]any)
	require.True(t, ok, "expected in array in key")
	assert.Len(t, inVal, 2)
}

func TestBuildLinearFilter_TeamsOnly(t *testing.T) {
	t.Parallel()
	f := buildLinearFilter([]string{"ENG"}, nil)
	require.NotNil(t, f)
	team, ok := f["team"].(map[string]any)
	require.True(t, ok)
	key, ok := team["key"].(map[string]any)
	require.True(t, ok)
	in, ok := key["in"].([]string)
	require.True(t, ok)
	assert.Equal(t, []string{"ENG"}, in)
}

func TestBuildLinearFilter_Empty(t *testing.T) {
	t.Parallel()
	f := buildLinearFilter(nil, nil)
	assert.Nil(t, f)
}

func TestBuildLinearFilter_WithDates(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC))
	filter := recipes.IssuesFilter_builder{
		CreatedAtFilter: osdd.DatesFilter_builder{From: ts}.Build(),
	}.Build()

	f := buildLinearFilter(nil, filter)
	require.NotNil(t, f)
	created, ok := f["createdAt"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "2025-06-15T00:00:00.000Z", created["gte"])
}

func TestBuildLinearFilter_TeamsAndDates(t *testing.T) {
	t.Parallel()
	from := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))
	filter := recipes.IssuesFilter_builder{
		CreatedAtFilter: osdd.DatesFilter_builder{From: from, To: to}.Build(),
		UpdatedAtFilter: osdd.DatesFilter_builder{From: from}.Build(),
	}.Build()

	f := buildLinearFilter([]string{"TEAM"}, filter)
	require.NotNil(t, f)
	assert.Contains(t, f, "team")
	assert.Contains(t, f, "createdAt")
	assert.Contains(t, f, "updatedAt")
}

func TestFormatTimestampISO(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(time.Date(2025, 6, 15, 12, 30, 0, 0, time.UTC))
	assert.Equal(t, "2025-06-15T12:30:00.000Z", formatTimestampISO(ts))
	assert.Equal(t, "", formatTimestampISO(nil))
}
