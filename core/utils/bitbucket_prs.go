package utils

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
)

// bitbucketAPIBaseURL can be overridden in tests to point at a httptest server.
var bitbucketAPIBaseURL string

const bitbucketDefaultBaseURL = "https://api.bitbucket.org/2.0"

// Bitbucket API response types.

type bitbucketPRList struct {
	Values []bitbucketPR `json:"values"`
	Next   string        `json:"next"`
}

type bitbucketPR struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	State       string `json:"state"`
	Description string `json:"description"`
	CreatedOn   string `json:"created_on"`
	UpdatedOn   string `json:"updated_on"`
	Author      struct {
		DisplayName string `json:"display_name"`
	} `json:"author"`
}

type bitbucketCommentList struct {
	Values []bitbucketComment `json:"values"`
	Next   string             `json:"next"`
}

type bitbucketComment struct {
	Content struct {
		Raw string `json:"raw"`
	} `json:"content"`
	User struct {
		DisplayName string `json:"display_name"`
	} `json:"user"`
}

// fetchBitbucketPRs fetches pull requests from the Bitbucket REST API,
// including comments and diffs, filtered by the given date range.
func fetchBitbucketPRs(ctx context.Context, workspace, repoSlug, token string, dateFilter *osdd.DatesFilter, summaryOnly bool) ([]pullRequest, error) {
	baseURL := bitbucketAPIBaseURL
	if baseURL == "" {
		baseURL = bitbucketDefaultBaseURL
	}

	sinceTime, untilTime := resolvePRDateRange(dateFilter)

	// Phase 1: collect PR metadata from paginated list.
	var allPRs []pullRequest
	apiURL := fmt.Sprintf("%s/repositories/%s/%s/pullrequests?state=OPEN&state=MERGED&state=DECLINED&state=SUPERSEDED", baseURL, workspace, repoSlug)

	for apiURL != "" {
		body, err := bitbucketGet(ctx, apiURL, token)
		if err != nil {
			return nil, fmt.Errorf("failed to list Bitbucket PRs: %w", err)
		}

		var prList bitbucketPRList
		if err := json.Unmarshal(body, &prList); err != nil {
			return nil, fmt.Errorf("failed to parse Bitbucket PRs response: %w", err)
		}

		for _, pr := range prList.Values {
			createdOn, _ := time.Parse(time.RFC3339, pr.CreatedOn)
			updatedOn, _ := time.Parse(time.RFC3339, pr.UpdatedOn)

			if !isInDateRange(createdOn, updatedOn, sinceTime, untilTime) {
				continue
			}

			allPRs = append(allPRs, pullRequest{
				Number:    pr.ID,
				Title:     pr.Title,
				Author:    pr.Author.DisplayName,
				State:     pr.State,
				CreatedAt: createdOn,
				UpdatedAt: updatedOn,
				Body:      pr.Description,
			})
		}

		apiURL = prList.Next
	}

	// Phase 2: fetch comments and diffs in parallel (max 5 concurrent).
	// When summaryOnly is true, skip fetching details since they won't be used.
	if !summaryOnly {
		fetchPRDetails(ctx, allPRs, func(ctx context.Context, pr *pullRequest) {
			comments, err := fetchBitbucketComments(ctx, baseURL, workspace, repoSlug, pr.Number, token)
			if err != nil {
				slog.Warn("Failed to fetch comments for Bitbucket PR", "id", pr.Number, "error", err)
			} else {
				pr.Reviews = comments
			}

			diff, err := fetchBitbucketDiff(ctx, baseURL, workspace, repoSlug, pr.Number, token)
			if err != nil {
				slog.Warn("Failed to fetch diff for Bitbucket PR", "id", pr.Number, "error", err)
			} else {
				pr.Diff = diff
			}
		})
	}

	slog.Debug("Bitbucket PRs fetched", "count", len(allPRs))
	return allPRs, nil
}

func fetchBitbucketComments(ctx context.Context, baseURL, workspace, repoSlug string, prID int, token string) ([]prReview, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/comments", baseURL, workspace, repoSlug, prID)
	body, err := bitbucketGet(ctx, url, token)
	if err != nil {
		return nil, err
	}

	var commentList bitbucketCommentList
	if err := json.Unmarshal(body, &commentList); err != nil {
		return nil, fmt.Errorf("failed to parse comments: %w", err)
	}

	var reviews []prReview
	for _, c := range commentList.Values {
		reviews = append(reviews, prReview{
			Author: c.User.DisplayName,
			State:  "COMMENTED",
			Body:   c.Content.Raw,
		})
	}
	return reviews, nil
}

func fetchBitbucketDiff(ctx context.Context, baseURL, workspace, repoSlug string, prID int, token string) (string, error) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/diff", baseURL, workspace, repoSlug, prID)
	body, err := bitbucketGet(ctx, url, token)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func bitbucketGet(ctx context.Context, url, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		if strings.Contains(token, ":") {
			// App password (user:password) → Basic Auth.
			req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(token)))
		} else {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bitbucket API returned status %d: %s", resp.StatusCode, truncateBody(body))
	}

	return body, nil
}
