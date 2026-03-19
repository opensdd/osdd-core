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
	ClosedOn    string `json:"closed_on"`
	MergeCommit *struct {
		Hash string `json:"hash"`
	} `json:"merge_commit"`
	Author struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
		AccountID   string `json:"account_id"`
	} `json:"author"`
	ClosedBy *struct {
		DisplayName string `json:"display_name"`
		Nickname    string `json:"nickname"`
	} `json:"closed_by"`
	Source struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"source"`
	Destination struct {
		Branch struct {
			Name string `json:"name"`
		} `json:"branch"`
	} `json:"destination"`
	Links struct {
		HTML struct {
			Href string `json:"href"`
		} `json:"html"`
	} `json:"links"`
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

type bitbucketCommitList struct {
	Values []bitbucketCommit `json:"values"`
	Next   string            `json:"next"`
}

type bitbucketCommit struct {
	Author struct {
		Raw  string `json:"raw"` // "Name <email>"
		User struct {
			DisplayName string `json:"display_name"`
			Nickname    string `json:"nickname"`
			AccountID   string `json:"account_id"`
		} `json:"user"`
	} `json:"author"`
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

			author := pr.Author.DisplayName
			if pr.Author.Nickname != "" && pr.Author.Nickname != author {
				author += " (" + pr.Author.Nickname + ")"
			}

			p := pullRequest{
				Number:     pr.ID,
				Title:      pr.Title,
				URL:        pr.Links.HTML.Href,
				Author:     author,
				State:      strings.ToLower(pr.State),
				BaseBranch: pr.Destination.Branch.Name,
				HeadBranch: pr.Source.Branch.Name,
				CreatedAt:  createdOn,
				UpdatedAt:  updatedOn,
				Body:       pr.Description,
			}
			if pr.ClosedOn != "" {
				if t, err := time.Parse(time.RFC3339, pr.ClosedOn); err == nil && strings.EqualFold(pr.State, "MERGED") {
					p.MergedAt = t
				}
			}
			if pr.ClosedBy != nil {
				name := pr.ClosedBy.DisplayName
				if pr.ClosedBy.Nickname != "" && pr.ClosedBy.Nickname != name {
					name += " (" + pr.ClosedBy.Nickname + ")"
				}
				if strings.EqualFold(pr.State, "MERGED") {
					p.MergedBy = name
				}
			}
			allPRs = append(allPRs, p)
		}

		apiURL = prList.Next
	}

	// Phase 2: fetch stats, comments, diffs, and resolve author emails from commits.
	fetchPRDetails(ctx, allPRs, func(ctx context.Context, pr *pullRequest) {
		// Resolve author email from PR commits.
		if pr.AuthorEmail == "" {
			email := resolveEmailFromBitbucketCommits(ctx, baseURL, workspace, repoSlug, pr.Number, token)
			if email != "" {
				pr.AuthorEmail = email
			}
		}

		// Fetch diffstat for additions/deletions/changed_files.
		adds, dels, files := fetchBitbucketDiffstat(ctx, baseURL, workspace, repoSlug, pr.Number, token)
		pr.Additions = adds
		pr.Deletions = dels
		pr.ChangedFiles = files

		if summaryOnly {
			return
		}

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

type bitbucketDiffstatList struct {
	Values []bitbucketDiffstatEntry `json:"values"`
}

type bitbucketDiffstatEntry struct {
	LinesAdded   int `json:"lines_added"`
	LinesRemoved int `json:"lines_removed"`
}

// fetchBitbucketDiffstat fetches the diffstat for a PR and returns
// total additions, deletions, and number of changed files.
func fetchBitbucketDiffstat(ctx context.Context, baseURL, workspace, repoSlug string, prID int, token string) (additions, deletions, changedFiles int) {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/diffstat", baseURL, workspace, repoSlug, prID)
	body, err := bitbucketGet(ctx, url, token)
	if err != nil {
		slog.Debug("Failed to fetch Bitbucket diffstat", "id", prID, "error", err)
		return 0, 0, 0
	}

	var diffstat bitbucketDiffstatList
	if err := json.Unmarshal(body, &diffstat); err != nil {
		slog.Debug("Failed to parse Bitbucket diffstat", "id", prID, "error", err)
		return 0, 0, 0
	}

	for _, entry := range diffstat.Values {
		additions += entry.LinesAdded
		deletions += entry.LinesRemoved
	}
	return additions, deletions, len(diffstat.Values)
}

// resolveEmailFromBitbucketCommits fetches the first page of commits for a PR
// and extracts the author email from the commit's "raw" field (format: "Name <email>").
func resolveEmailFromBitbucketCommits(ctx context.Context, baseURL, workspace, repoSlug string, prID int, token string) string {
	url := fmt.Sprintf("%s/repositories/%s/%s/pullrequests/%d/commits?pagelen=1", baseURL, workspace, repoSlug, prID)
	body, err := bitbucketGet(ctx, url, token)
	if err != nil {
		slog.Debug("Failed to fetch Bitbucket PR commits", "id", prID, "error", err)
		return ""
	}

	var commitList bitbucketCommitList
	if err := json.Unmarshal(body, &commitList); err != nil {
		slog.Debug("Failed to parse Bitbucket PR commits", "id", prID, "error", err)
		return ""
	}

	for _, c := range commitList.Values {
		if email := parseEmailFromRaw(c.Author.Raw); email != "" {
			return email
		}
	}
	return ""
}

// parseEmailFromRaw extracts an email from a git author "raw" string like "Name <email>".
func parseEmailFromRaw(raw string) string {
	start := strings.LastIndex(raw, "<")
	end := strings.LastIndex(raw, ">")
	if start >= 0 && end > start+1 {
		return raw[start+1 : end]
	}
	return ""
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
