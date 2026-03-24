package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/go-github/v83/github"
	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func withGitHubServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	old := githubAPIBaseURL
	githubAPIBaseURL = server.URL
	t.Cleanup(func() { githubAPIBaseURL = old })
	return server.URL
}

// ghTimestamp creates a *github.Timestamp from a time.Time.
func ghTimestamp(t time.Time) *github.Timestamp {
	return &github.Timestamp{Time: t}
}

func TestFetchGitHubPRs_Success(t *testing.T) {
	mux := http.NewServeMux()

	// PR listing
	mux.HandleFunc("GET /repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		prs := []*github.PullRequest{
			{
				Number:    github.Ptr(1),
				Title:     github.Ptr("First PR"),
				State:     github.Ptr("closed"),
				Body:      github.Ptr("Description of first PR"),
				CreatedAt: ghTimestamp(time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)),
				UpdatedAt: ghTimestamp(time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)),
				User:      &github.User{Login: github.Ptr("alice")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prs)
	})

	// Individual PR (for diff)
	mux.HandleFunc("GET /repos/owner/repo/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("diff content for PR"))
	})

	// Reviews
	mux.HandleFunc("GET /repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		reviews := []*github.PullRequestReview{
			{
				State: github.Ptr("APPROVED"),
				Body:  github.Ptr("LGTM"),
				User:  &github.User{Login: github.Ptr("bob")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(reviews)
	})

	withGitHubServer(t, mux)

	df := osdd.DatesFilter_builder{
		From: timestamppb.New(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
		To:   timestamppb.New(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)),
	}.Build()

	result, err := fetchGitHubPRs(t.Context(), "owner", "repo", "test-token", df, false)
	require.NoError(t, err)
	require.Len(t, result.PRs, 1)

	assert.Equal(t, 1, result.PRs[0].Number)
	assert.Equal(t, "First PR", result.PRs[0].Title)
	assert.Equal(t, "alice", result.PRs[0].Author)
	assert.Equal(t, "closed", result.PRs[0].State)
	assert.Equal(t, "Description of first PR", result.PRs[0].Body)
	assert.Contains(t, result.PRs[0].Diff, "diff content for PR")

	require.Len(t, result.PRs[0].Reviews, 1)
	assert.Equal(t, "bob", result.PRs[0].Reviews[0].Author)
	assert.Equal(t, "APPROVED", result.PRs[0].Reviews[0].State)
	assert.Equal(t, "LGTM", result.PRs[0].Reviews[0].Body)
}

func TestFetchGitHubPRs_DateFiltering(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		prs := []*github.PullRequest{
			{
				Number:    github.Ptr(1),
				Title:     github.Ptr("In range"),
				State:     github.Ptr("open"),
				CreatedAt: ghTimestamp(time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)),
				UpdatedAt: ghTimestamp(time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)),
				User:      &github.User{Login: github.Ptr("alice")},
			},
			{
				Number:    github.Ptr(2),
				Title:     github.Ptr("Out of range"),
				State:     github.Ptr("closed"),
				CreatedAt: ghTimestamp(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)),
				UpdatedAt: ghTimestamp(time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)),
				User:      &github.User{Login: github.Ptr("bob")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prs)
	})

	mux.HandleFunc("GET /repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("GET /repos/owner/repo/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(""))
	})

	withGitHubServer(t, mux)

	df := osdd.DatesFilter_builder{
		From: timestamppb.New(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
		To:   timestamppb.New(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)),
	}.Build()

	result, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", df, false)
	require.NoError(t, err)
	require.Len(t, result.PRs, 1)
	assert.Equal(t, "In range", result.PRs[0].Title)
}

func TestFetchGitHubPRs_Pagination(t *testing.T) {
	callCount := 0
	mux := http.NewServeMux()

	mux.HandleFunc("GET /repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		now := time.Now()
		var prs []*github.PullRequest
		if callCount == 1 {
			for i := range 100 {
				prs = append(prs, &github.PullRequest{
					Number:    github.Ptr(i + 1),
					Title:     github.Ptr("PR"),
					State:     github.Ptr("closed"),
					CreatedAt: ghTimestamp(now),
					UpdatedAt: ghTimestamp(now),
					User:      &github.User{Login: github.Ptr("user")},
				})
			}
			// Signal next page via Link header (go-github uses this).
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/owner/repo/pulls?page=2>; rel="next"`, r.Header.Get("Referer")))
		} else {
			prs = append(prs, &github.PullRequest{
				Number:    github.Ptr(101),
				Title:     github.Ptr("Last PR"),
				State:     github.Ptr("open"),
				CreatedAt: ghTimestamp(now),
				UpdatedAt: ghTimestamp(now),
				User:      &github.User{Login: github.Ptr("user")},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prs)
	})

	// Catch-all for reviews and diffs.
	mux.HandleFunc("GET /repos/owner/repo/pulls/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})

	withGitHubServer(t, mux)

	result, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil, false)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Len(t, result.PRs, 101)
}

func TestFetchGitHubPRs_EmptyToken(t *testing.T) {
	var receivedAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})

	withGitHubServer(t, mux)

	_, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil, false)
	require.NoError(t, err)
	assert.Empty(t, receivedAuth)
}

func TestFetchGitHubPRs_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limit exceeded"}`))
	})

	withGitHubServer(t, mux)

	_, err := fetchGitHubPRs(t.Context(), "owner", "repo", "token", nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "403")
}

func TestFetchGitHubPRs_EmptyResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})

	withGitHubServer(t, mux)

	result, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil, false)
	require.NoError(t, err)
	assert.Empty(t, result.PRs)
}

func TestFetchGitHubPRs_SummaryOnlySkipsReviewsAndDiff(t *testing.T) {
	reviewsCalled := false
	diffCalled := false
	mux := http.NewServeMux()

	mux.HandleFunc("GET /repos/owner/repo/pulls", func(w http.ResponseWriter, r *http.Request) {
		prs := []*github.PullRequest{
			{
				Number:    github.Ptr(1),
				Title:     github.Ptr("PR One"),
				State:     github.Ptr("closed"),
				CreatedAt: ghTimestamp(time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC)),
				UpdatedAt: ghTimestamp(time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC)),
				User:      &github.User{Login: github.Ptr("alice")},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(prs)
	})

	// Commits endpoint IS expected (for email resolution).
	mux.HandleFunc("GET /repos/owner/repo/pulls/1/commits", func(w http.ResponseWriter, r *http.Request) {
		commits := []map[string]any{
			{
				"sha":       "abc123",
				"author":    map[string]any{"login": "alice"},
				"committer": map[string]any{"login": "alice"},
				"commit": map[string]any{
					"author":    map[string]any{"name": "Alice", "email": "alice@example.com"},
					"committer": map[string]any{"name": "Alice", "email": "alice@example.com"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(commits)
	})

	// Individual PR endpoint (for Get call — returns PR JSON with stats).
	// Diff requests use the same path but with Accept: application/vnd.github.diff.
	mux.HandleFunc("GET /repos/owner/repo/pulls/1", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.Header.Get("Accept"), "diff") {
			diffCalled = true
			_, _ = w.Write([]byte(""))
			return
		}
		// Return individual PR JSON with merged_by, additions, etc.
		pr := map[string]any{
			"number": 1, "title": "PR One", "state": "closed",
			"additions": 10, "deletions": 5, "changed_files": 2,
			"user": map[string]any{"login": "alice"},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pr)
	})

	// Reviews should NOT be called.
	mux.HandleFunc("GET /repos/owner/repo/pulls/1/reviews", func(w http.ResponseWriter, r *http.Request) {
		reviewsCalled = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	})

	withGitHubServer(t, mux)

	df := osdd.DatesFilter_builder{
		From: timestamppb.New(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
		To:   timestamppb.New(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)),
	}.Build()

	result, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", df, true)
	require.NoError(t, err)
	require.Len(t, result.PRs, 1)
	assert.Equal(t, "PR One", result.PRs[0].Title)
	assert.Equal(t, "alice@example.com", result.PRs[0].AuthorEmail)
	assert.Empty(t, result.PRs[0].Reviews)
	assert.Empty(t, result.PRs[0].Diff)
	assert.False(t, reviewsCalled, "reviews endpoint should not be called when summaryOnly=true")
	assert.False(t, diffCalled, "diff endpoint should not be called when summaryOnly=true")
}

func TestIsInDateRange(t *testing.T) {
	since := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		created   time.Time
		updated   time.Time
		wantMatch bool
	}{
		{
			name:      "both in range",
			created:   time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			updated:   time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC),
			wantMatch: true,
		},
		{
			name:      "created before updated in range",
			created:   time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC),
			updated:   time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC),
			wantMatch: true,
		},
		{
			name:      "both before range",
			created:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			updated:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
			wantMatch: false,
		},
		{
			name:      "both after range",
			created:   time.Date(2025, 8, 1, 0, 0, 0, 0, time.UTC),
			updated:   time.Date(2025, 8, 2, 0, 0, 0, 0, time.UTC),
			wantMatch: false,
		},
		{
			name:      "exact since boundary",
			created:   since,
			updated:   since,
			wantMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isInDateRange(tt.created, tt.updated, since, until)
			assert.Equal(t, tt.wantMatch, got)
		})
	}
}
