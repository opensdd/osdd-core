package utils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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

	prs, err := fetchGitHubPRs(t.Context(), "owner", "repo", "test-token", df)
	require.NoError(t, err)
	require.Len(t, prs, 1)

	assert.Equal(t, 1, prs[0].Number)
	assert.Equal(t, "First PR", prs[0].Title)
	assert.Equal(t, "alice", prs[0].Author)
	assert.Equal(t, "closed", prs[0].State)
	assert.Equal(t, "Description of first PR", prs[0].Body)
	assert.Contains(t, prs[0].Diff, "diff content for PR")

	require.Len(t, prs[0].Reviews, 1)
	assert.Equal(t, "bob", prs[0].Reviews[0].Author)
	assert.Equal(t, "APPROVED", prs[0].Reviews[0].State)
	assert.Equal(t, "LGTM", prs[0].Reviews[0].Body)
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

	prs, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", df)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, "In range", prs[0].Title)
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

	prs, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Len(t, prs, 101)
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

	_, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil)
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

	_, err := fetchGitHubPRs(t.Context(), "owner", "repo", "token", nil)
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

	prs, err := fetchGitHubPRs(t.Context(), "owner", "repo", "", nil)
	require.NoError(t, err)
	assert.Empty(t, prs)
}
