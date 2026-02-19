package utils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func jiraSource(org string, projects []string, authEnvVar *string, filter *recipes.IssuesFilter) *recipes.JiraIssuesSource {
	b := recipes.JiraIssuesSource_builder{
		Organization:    org,
		Projects:        projects,
		Filter:          filter,
		AuthTokenEnvVar: authEnvVar,
	}
	return b.Build()
}

func jiraServerResponse(issues []jiraIssue, nextPageToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := jiraSearchResponse{
			Issues:        issues,
			NextPageToken: nextPageToken,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}
}

func TestFetchJiraIssues_NilSource(t *testing.T) {
	t.Parallel()
	_, err := FetchJiraIssues(context.Background(), nil, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jira issues source cannot be nil")
}

func TestFetchJiraIssues_EmptyOrganization(t *testing.T) {
	t.Parallel()
	src := jiraSource("", nil, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jira organization cannot be empty")
}

func TestFetchJiraIssues_Success(t *testing.T) {
	issues := []jiraIssue{
		{
			Key: "PROJ-1",
			Fields: jiraIssueFields{
				Summary:   "First issue",
				Status:    jiraName{Name: "Open"},
				IssueType: jiraName{Name: "Bug"},
				Priority:  jiraName{Name: "High"},
				Assignee:  &jiraAssignee{DisplayName: "Alice"},
				Created:   "2025-01-15T10:00:00.000+0000",
				Updated:   "2025-02-01T14:30:00.000+0000",
			},
		},
		{
			Key: "PROJ-2",
			Fields: jiraIssueFields{
				Summary:   "Second issue",
				Status:    jiraName{Name: "Done"},
				IssueType: jiraName{Name: "Task"},
				Priority:  jiraName{Name: "Low"},
				Created:   "2025-01-20T08:00:00.000+0000",
				Updated:   "2025-02-05T09:00:00.000+0000",
			},
		},
	}

	server := httptest.NewServer(jiraServerResponse(issues, ""))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", []string{"PROJ"}, nil, nil)
	result, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)

	require.Len(t, result.Summary, 2)
	assert.Equal(t, "PROJ-1", result.Summary[0].ID)
	assert.Equal(t, "First issue", result.Summary[0].Title)
	assert.Equal(t, "PROJ-2", result.Summary[1].ID)
	assert.Equal(t, "Second issue", result.Summary[1].Title)

	require.Len(t, result.Issues, 2)
	assert.Contains(t, result.Issues["PROJ-1"], `"key": "PROJ-1"`)
	assert.Contains(t, result.Issues["PROJ-1"], `"displayName": "Alice"`)
	assert.Contains(t, result.Issues["PROJ-2"], `"key": "PROJ-2"`)
}

func TestFetchJiraIssues_EmptyResponse(t *testing.T) {
	server := httptest.NewServer(jiraServerResponse(nil, ""))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	result, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Empty(t, result.Summary)
	assert.Empty(t, result.Issues)
}

func TestFetchJiraIssues_APIKey_BasicAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jiraSearchResponse{})
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "my-api-key")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(receivedAuth, "Basic "), "expected Basic auth prefix")
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(receivedAuth, "Basic "))
	require.NoError(t, err)
	assert.Equal(t, ":my-api-key", string(decoded))
}

func TestFetchJiraIssues_PersonalAccessToken_BasicAuth(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jiraSearchResponse{})
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "user@example.com:api-token-123")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(receivedAuth, "Basic "), "expected Basic auth prefix")
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(receivedAuth, "Basic "))
	require.NoError(t, err)
	assert.Equal(t, "user@example.com:api-token-123", string(decoded))
}

func TestFetchJiraIssues_NoAuthToken(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jiraSearchResponse{})
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Empty(t, receivedAuth)
}

func TestFetchJiraIssues_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"errorMessages":["not authorized"]}`))
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "jira API returned status 401")
}

func TestFetchJiraIssues_Pagination(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var resp jiraSearchResponse
		if callCount == 1 {
			resp = jiraSearchResponse{
				Issues: []jiraIssue{
					{Key: "P-1", Fields: jiraIssueFields{Summary: "Issue 1", Status: jiraName{Name: "Open"}, IssueType: jiraName{Name: "Task"}, Priority: jiraName{Name: "Medium"}}},
				},
				NextPageToken: "page2",
			}
		} else {
			resp = jiraSearchResponse{
				Issues: []jiraIssue{
					{Key: "P-2", Fields: jiraIssueFields{Summary: "Issue 2", Status: jiraName{Name: "Done"}, IssueType: jiraName{Name: "Bug"}, Priority: jiraName{Name: "Low"}}},
				},
				NextPageToken: "",
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	result, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	require.Len(t, result.Summary, 2)
	assert.Equal(t, "P-1", result.Summary[0].ID)
	assert.Equal(t, "P-2", result.Summary[1].ID)
	assert.Contains(t, result.Issues["P-1"], `"key": "P-1"`)
	assert.Contains(t, result.Issues["P-2"], `"key": "P-2"`)
}

func TestFetchJiraIssues_ADFDescription(t *testing.T) {
	descJSON := json.RawMessage(`{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello world"}]}]}`)
	issues := []jiraIssue{
		{
			Key: "PROJ-1",
			Fields: jiraIssueFields{
				Summary:     "ADF test",
				Description: descJSON,
				Status:      jiraName{Name: "Open"},
				IssueType:   jiraName{Name: "Bug"},
				Priority:    jiraName{Name: "High"},
			},
		},
	}

	server := httptest.NewServer(jiraServerResponse(issues, ""))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	result, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Contains(t, result.Issues["PROJ-1"], "Hello world")
}

func TestFetchJiraIssues_PlainStringDescription(t *testing.T) {
	descJSON := json.RawMessage(`"This is a plain description"`)
	issues := []jiraIssue{
		{
			Key: "PROJ-1",
			Fields: jiraIssueFields{
				Summary:     "Plain desc test",
				Description: descJSON,
				Status:      jiraName{Name: "Open"},
				IssueType:   jiraName{Name: "Task"},
				Priority:    jiraName{Name: "Low"},
			},
		},
	}

	server := httptest.NewServer(jiraServerResponse(issues, ""))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", nil, nil, nil)
	result, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Contains(t, result.Issues["PROJ-1"], "This is a plain description")
}

func TestFetchJiraIssues_RequestContainsJQL(t *testing.T) {
	var receivedBody jiraSearchRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&receivedBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(jiraSearchResponse{})
	}))
	defer server.Close()

	old := jiraBaseURL
	jiraBaseURL = server.URL
	defer func() { jiraBaseURL = old }()

	src := jiraSource("test-org", []string{"ALPHA", "BETA"}, nil, nil)
	_, err := FetchJiraIssues(context.Background(), src, "")
	require.NoError(t, err)
	assert.Contains(t, receivedBody.JQL, "ALPHA")
	assert.Contains(t, receivedBody.JQL, "BETA")
	assert.Contains(t, receivedBody.JQL, "project IN")
}

func TestBuildJQL_ProjectsOnly(t *testing.T) {
	t.Parallel()
	jql := buildJQL([]string{"PROJ1", "PROJ2"}, nil)
	assert.Contains(t, jql, "project IN")
	assert.Contains(t, jql, "PROJ1")
	assert.Contains(t, jql, "PROJ2")
}

func TestBuildJQL_NoFilters(t *testing.T) {
	t.Parallel()
	jql := buildJQL(nil, nil)
	assert.Contains(t, jql, "created >= -30d")
	assert.Contains(t, jql, "ORDER BY created DESC")
}

func TestBuildJQL_WithDates(t *testing.T) {
	t.Parallel()
	ts := timestamppb.New(time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC))
	filter := recipes.IssuesFilter_builder{
		CreatedAtFilter: osdd.DatesFilter_builder{From: ts}.Build(),
	}.Build()

	jql := buildJQL(nil, filter)
	assert.Contains(t, jql, `created >= "2025-06-15"`)
}

func TestBuildJQL_ProjectsAndDates(t *testing.T) {
	t.Parallel()
	from := timestamppb.New(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	to := timestamppb.New(time.Date(2025, 12, 31, 0, 0, 0, 0, time.UTC))
	filter := recipes.IssuesFilter_builder{
		CreatedAtFilter: osdd.DatesFilter_builder{From: from, To: to}.Build(),
		UpdatedAtFilter: osdd.DatesFilter_builder{From: from}.Build(),
	}.Build()

	jql := buildJQL([]string{"PROJ"}, filter)
	assert.Contains(t, jql, `project IN ("PROJ")`)
	assert.Contains(t, jql, `created >= "2025-01-01"`)
	assert.Contains(t, jql, `created <= "2025-12-31"`)
	assert.Contains(t, jql, `updated >= "2025-01-01"`)
	assert.Contains(t, jql, " AND ")
}
