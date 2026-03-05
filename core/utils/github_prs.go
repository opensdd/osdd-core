package utils

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/google/go-github/v83/github"
	"github.com/opensdd/osdd-api/clients/go/osdd"
)

const maxPRFetchConcurrency = 5

// githubAPIBaseURL can be overridden in tests to point at a httptest server.
// When empty, the default GitHub API URL is used.
var githubAPIBaseURL string

// pullRequest is the shared PR type used by both GitHub and Bitbucket fetchers.
type pullRequest struct {
	Number    int
	Title     string
	Author    string
	State     string
	CreatedAt time.Time
	UpdatedAt time.Time
	Body      string
	Reviews   []prReview
	Diff      string
}

// prReview represents a single review on a pull request.
type prReview struct {
	Author string
	State  string
	Body   string
}

// isInDateRange returns true if the PR's created or updated time falls within
// [sinceTime, untilTime). If untilTime is zero, only the lower bound is checked.
func isInDateRange(createdAt, updatedAt, sinceTime, untilTime time.Time) bool {
	// Both timestamps entirely after the range — skip.
	if !untilTime.IsZero() && createdAt.After(untilTime) && updatedAt.After(untilTime) {
		return false
	}

	if (createdAt.Equal(sinceTime) || createdAt.After(sinceTime)) &&
		(untilTime.IsZero() || createdAt.Before(untilTime)) {
		return true
	}
	if (updatedAt.Equal(sinceTime) || updatedAt.After(sinceTime)) &&
		(untilTime.IsZero() || updatedAt.Before(untilTime)) {
		return true
	}
	return false
}

// resolvePRDateRange computes sinceTime and untilTime from a DatesFilter,
// applying defaults when fields are absent.
func resolvePRDateRange(dateFilter *osdd.DatesFilter) (sinceTime, untilTime time.Time) {
	if dateFilter != nil {
		if dateFilter.HasFrom() {
			sinceTime = dateFilter.GetFrom().AsTime().UTC()
		}
		if dateFilter.HasTo() {
			// Make "to" inclusive by adding one day.
			untilTime = dateFilter.GetTo().AsTime().UTC().AddDate(0, 0, 1)
		}
	}
	if sinceTime.IsZero() {
		sinceTime = time.Now().AddDate(0, 0, -defaultSinceDays).UTC()
	}
	return sinceTime, untilTime
}

// newGitHubClient creates a go-github Client, optionally authenticated
// and optionally pointed at a custom base URL (for tests).
func newGitHubClient(token string) *github.Client {
	client := github.NewClient(nil)
	if token != "" {
		client = client.WithAuthToken(token)
	}
	if githubAPIBaseURL != "" {
		// Must end with "/" for go-github URL resolution.
		base := githubAPIBaseURL
		if !strings.HasSuffix(base, "/") {
			base += "/"
		}
		client.BaseURL, _ = url.Parse(base)
	}
	return client
}

// fetchGitHubPRs fetches pull requests from the GitHub REST API using go-github,
// including reviews and diffs, filtered by the given date range.
func fetchGitHubPRs(ctx context.Context, owner, repo, token string, dateFilter *osdd.DatesFilter, summaryOnly bool) ([]pullRequest, error) {
	sinceTime, untilTime := resolvePRDateRange(dateFilter)
	client := newGitHubClient(token)

	opts := &github.PullRequestListOptions{
		State:     "all",
		Sort:      "updated",
		Direction: "desc",
		ListOptions: github.ListOptions{
			PerPage: 100,
		},
	}

	slog.Debug("Fetching GitHub PRs", "repo", owner+"/"+repo, "since", sinceTime.Format("2006-01-02"), "until", untilTime.Format("2006-01-02"))

	// Phase 1: collect PR metadata from paginated list.
	var allPRs []pullRequest

	for {
		ghPRs, resp, err := client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to list GitHub PRs: %w", err)
		}

		if len(ghPRs) == 0 {
			break
		}

		pastRange := false
		for _, pr := range ghPRs {
			createdAt := pr.GetCreatedAt().Time
			updatedAt := pr.GetUpdatedAt().Time

			if updatedAt.Before(sinceTime) {
				pastRange = true
				break
			}

			if !isInDateRange(createdAt, updatedAt, sinceTime, untilTime) {
				continue
			}

			allPRs = append(allPRs, pullRequest{
				Number:    pr.GetNumber(),
				Title:     pr.GetTitle(),
				Author:    pr.GetUser().GetLogin(),
				State:     pr.GetState(),
				CreatedAt: createdAt,
				UpdatedAt: updatedAt,
				Body:      pr.GetBody(),
			})
		}

		if pastRange || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Phase 2: fetch reviews and diffs in parallel (max 5 concurrent).
	// When summaryOnly is true, skip fetching details since they won't be used.
	if !summaryOnly {
		fetchPRDetails(ctx, allPRs, func(ctx context.Context, pr *pullRequest) {
			slog.Debug("Fetching PR details", "repo", owner+"/"+repo, "pr", pr.Number, "title", pr.Title)

			reviews, err := fetchGitHubReviews(ctx, client, owner, repo, pr.Number)
			if err != nil {
				slog.Warn("Failed to fetch reviews for PR", "number", pr.Number, "error", err)
			} else {
				pr.Reviews = reviews
			}

			diff, err := fetchGitHubDiff(ctx, client, owner, repo, pr.Number)
			if err != nil {
				slog.Warn("Failed to fetch diff for PR", "number", pr.Number, "error", err)
			} else {
				pr.Diff = diff
			}
		})
	}

	slog.Debug("GitHub PRs fetched", "count", len(allPRs))
	return allPRs, nil
}

// fetchPRDetails runs fn for each PR in parallel with bounded concurrency.
func fetchPRDetails(ctx context.Context, prs []pullRequest, fn func(context.Context, *pullRequest)) {
	if len(prs) == 0 {
		return
	}
	sem := make(chan struct{}, maxPRFetchConcurrency)
	var wg sync.WaitGroup
	for i := range prs {
		wg.Add(1)
		sem <- struct{}{}
		go func(pr *pullRequest) {
			defer wg.Done()
			defer func() { <-sem }()
			fn(ctx, pr)
		}(&prs[i])
	}
	wg.Wait()
}

func fetchGitHubReviews(ctx context.Context, client *github.Client, owner, repo string, number int) ([]prReview, error) {
	ghReviews, _, err := client.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	if err != nil {
		return nil, err
	}

	result := make([]prReview, 0, len(ghReviews))
	for _, r := range ghReviews {
		result = append(result, prReview{
			Author: r.GetUser().GetLogin(),
			State:  r.GetState(),
			Body:   r.GetBody(),
		})
	}
	return result, nil
}

func fetchGitHubDiff(ctx context.Context, client *github.Client, owner, repo string, number int) (string, error) {
	diff, _, err := client.PullRequests.GetRaw(ctx, owner, repo, number, github.RawOptions{Type: github.Diff})
	if err != nil {
		return "", err
	}
	return diff, nil
}

func truncateBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
