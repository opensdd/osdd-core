package utils

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func withBitbucketServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	old := bitbucketAPIBaseURL
	bitbucketAPIBaseURL = server.URL
	t.Cleanup(func() { bitbucketAPIBaseURL = old })
	return server.URL
}

func TestFetchBitbucketPRs_Success(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		resp := bitbucketPRList{
			Values: []bitbucketPR{
				{
					ID:          1,
					Title:       "BB PR One",
					State:       "MERGED",
					Description: "First BB PR",
					CreatedOn:   time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					UpdatedOn:   time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Author: struct {
						DisplayName string `json:"display_name"`
						Nickname    string `json:"nickname"`
						AccountID   string `json:"account_id"`
					}{DisplayName: "Alice"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/repositories/ws/repo/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		resp := bitbucketCommentList{
			Values: []bitbucketComment{
				{
					Content: struct {
						Raw string `json:"raw"`
					}{Raw: "Nice work!"},
					User: struct {
						DisplayName string `json:"display_name"`
					}{DisplayName: "Bob"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/repositories/ws/repo/pullrequests/1/diff", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("diff --git a/file.go b/file.go\n+added line"))
	})

	withBitbucketServer(t, mux)

	df := osdd.DatesFilter_builder{
		From: timestamppb.New(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
		To:   timestamppb.New(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)),
	}.Build()

	prs, err := fetchBitbucketPRs(t.Context(), "ws", "repo", "test-token", df, false)
	require.NoError(t, err)
	require.Len(t, prs, 1)

	assert.Equal(t, 1, prs[0].Number)
	assert.Equal(t, "BB PR One", prs[0].Title)
	assert.Equal(t, "Alice", prs[0].Author)
	assert.Equal(t, "merged", prs[0].State)
	assert.Equal(t, "First BB PR", prs[0].Body)
	assert.Contains(t, prs[0].Diff, "+added line")

	require.Len(t, prs[0].Reviews, 1)
	assert.Equal(t, "Bob", prs[0].Reviews[0].Author)
	assert.Equal(t, "Nice work!", prs[0].Reviews[0].Body)
}

func TestFetchBitbucketPRs_DateFiltering(t *testing.T) {
	mux := http.NewServeMux()

	mux.HandleFunc("/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		resp := bitbucketPRList{
			Values: []bitbucketPR{
				{
					ID:        1,
					Title:     "In range",
					State:     "OPEN",
					CreatedOn: time.Date(2025, 6, 15, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					UpdatedOn: time.Date(2025, 6, 16, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Author: struct {
						DisplayName string `json:"display_name"`
						Nickname    string `json:"nickname"`
						AccountID   string `json:"account_id"`
					}{DisplayName: "Alice"},
				},
				{
					ID:        2,
					Title:     "Out of range",
					State:     "MERGED",
					CreatedOn: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					UpdatedOn: time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC).Format(time.RFC3339),
					Author: struct {
						DisplayName string `json:"display_name"`
						Nickname    string `json:"nickname"`
						AccountID   string `json:"account_id"`
					}{DisplayName: "Bob"},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/repositories/ws/repo/pullrequests/1/comments", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bitbucketCommentList{})
	})
	mux.HandleFunc("/repositories/ws/repo/pullrequests/1/diff", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(""))
	})

	withBitbucketServer(t, mux)

	df := osdd.DatesFilter_builder{
		From: timestamppb.New(time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)),
		To:   timestamppb.New(time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)),
	}.Build()

	prs, err := fetchBitbucketPRs(t.Context(), "ws", "repo", "", df, false)
	require.NoError(t, err)
	require.Len(t, prs, 1)
	assert.Equal(t, "In range", prs[0].Title)
}

func TestFetchBitbucketPRs_Pagination(t *testing.T) {
	callCount := 0
	var serverURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/repositories/ws/repo/pullrequests", func(w http.ResponseWriter, r *http.Request) {
		callCount++
		now := time.Now().UTC().Format(time.RFC3339)
		var resp bitbucketPRList
		if callCount == 1 {
			resp = bitbucketPRList{
				Values: []bitbucketPR{
					{ID: 1, Title: "PR 1", State: "OPEN", CreatedOn: now, UpdatedOn: now,
						Author: struct {
							DisplayName string `json:"display_name"`
							Nickname    string `json:"nickname"`
							AccountID   string `json:"account_id"`
						}{DisplayName: "user"}},
				},
				Next: serverURL + "/repositories/ws/repo/pullrequests?page=2",
			}
		} else {
			resp = bitbucketPRList{
				Values: []bitbucketPR{
					{ID: 2, Title: "PR 2", State: "MERGED", CreatedOn: now, UpdatedOn: now,
						Author: struct {
							DisplayName string `json:"display_name"`
							Nickname    string `json:"nickname"`
							AccountID   string `json:"account_id"`
						}{DisplayName: "user"}},
				},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(bitbucketCommentList{})
	})

	serverURL = withBitbucketServer(t, mux)

	prs, err := fetchBitbucketPRs(t.Context(), "ws", "repo", "", nil, false)
	require.NoError(t, err)
	assert.Equal(t, 2, callCount)
	assert.Len(t, prs, 2)
}

func TestFetchBitbucketPRs_HTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"unauthorized"}}`))
	})

	withBitbucketServer(t, mux)

	_, err := fetchBitbucketPRs(t.Context(), "ws", "repo", "bad-token", nil, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bitbucket API returned status 401")
}
