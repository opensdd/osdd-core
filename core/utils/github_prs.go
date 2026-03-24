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
	Number        int
	Title         string
	URL           string
	Author        string
	AuthorEmail   string
	MergedBy      string
	MergedByEmail string
	State         string
	BaseBranch    string
	HeadBranch    string
	Labels        []string
	Additions     int
	Deletions     int
	ChangedFiles  int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	MergedAt      time.Time
	Body          string
	Reviews       []prReview
	Diff          string
}

// prReview represents a single review on a pull request.
type prReview struct {
	Author      string
	AuthorEmail string
	State       string
	Body        string
}

// prFetchResult bundles the pull requests fetched from an API along with
// identity metadata discovered during fetching (e.g. login→email mappings
// resolved from PR commit metadata).
type prFetchResult struct {
	PRs []pullRequest
	// LoginEmails maps GitHub/Bitbucket login → commit-author email,
	// built from PR commit metadata during fetching.
	LoginEmails map[string]string
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
func fetchGitHubPRs(ctx context.Context, owner, repo, token string, dateFilter *osdd.DatesFilter, summaryOnly bool) (prFetchResult, error) {
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
			return prFetchResult{}, fmt.Errorf("failed to list GitHub PRs: %w", err)
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

			// List endpoint has merged_at but not merged/additions/deletions.
			state := pr.GetState()
			var mergedAt time.Time
			if pr.MergedAt != nil {
				if t := pr.MergedAt.GetTime(); t != nil {
					mergedAt = *t
				}
				state = "merged"
			}

			var labels []string
			for _, l := range pr.Labels {
				labels = append(labels, l.GetName())
			}

			p := pullRequest{
				Number:     pr.GetNumber(),
				Title:      pr.GetTitle(),
				URL:        pr.GetHTMLURL(),
				Author:     pr.GetUser().GetLogin(),
				State:      state,
				BaseBranch: pr.GetBase().GetRef(),
				HeadBranch: pr.GetHead().GetRef(),
				Labels:     labels,
				CreatedAt:  createdAt,
				UpdatedAt:  updatedAt,
				Body:       pr.GetBody(),
			}
			if email := pr.GetUser().GetEmail(); email != "" {
				p.AuthorEmail = email
			}
			if !mergedAt.IsZero() {
				p.MergedAt = mergedAt
			}
			allPRs = append(allPRs, p)
		}

		if pastRange || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	// Phase 2: fetch individual PR details, reviews, diffs, and emails in parallel.
	var emailStore sync.Map // login → email
	fetchPRDetails(ctx, allPRs, func(ctx context.Context, pr *pullRequest) {
		// Fetch individual PR for fields not in list response (merged_by, stats).
		fullPR, _, err := client.PullRequests.Get(ctx, owner, repo, pr.Number)
		if err != nil {
			slog.Debug("Failed to fetch full PR", "number", pr.Number, "error", err)
		} else {
			if mb := fullPR.GetMergedBy(); mb != nil {
				pr.MergedBy = mb.GetLogin()
			}
			pr.Additions = fullPR.GetAdditions()
			pr.Deletions = fullPR.GetDeletions()
			pr.ChangedFiles = fullPR.GetChangedFiles()
		}

		// Resolve emails from PR commits.
		emails, err := resolveEmailsFromPRCommits(ctx, client, owner, repo, pr.Number)
		if err != nil {
			slog.Debug("Failed to resolve emails from PR commits", "number", pr.Number, "error", err)
		}
		for login, email := range emails {
			emailStore.LoadOrStore(login, email)
		}
		if pr.AuthorEmail == "" {
			pr.AuthorEmail = emails[pr.Author]
		}

		if summaryOnly {
			return
		}

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

	// Build plain map from sync.Map for applying to merged-by / reviewers.
	emailMap := make(map[string]string)
	emailStore.Range(func(key, value any) bool {
		emailMap[key.(string)] = value.(string)
		return true
	})
	for i := range allPRs {
		if allPRs[i].MergedBy != "" && allPRs[i].MergedByEmail == "" {
			allPRs[i].MergedByEmail = emailMap[allPRs[i].MergedBy]
		}
		for j := range allPRs[i].Reviews {
			if allPRs[i].Reviews[j].AuthorEmail == "" {
				allPRs[i].Reviews[j].AuthorEmail = emailMap[allPRs[i].Reviews[j].Author]
			}
		}
	}

	slog.Debug("GitHub PRs fetched", "count", len(allPRs))
	return prFetchResult{PRs: allPRs, LoginEmails: emailMap}, nil
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

// resolveEmailsFromPRCommits fetches commits for a PR and builds a login→email
// map from the git commit metadata. This captures emails for all committers on
// the PR (author, co-authors), not just the PR opener.
func resolveEmailsFromPRCommits(ctx context.Context, client *github.Client, owner, repo string, number int) (map[string]string, error) {
	opts := &github.ListOptions{PerPage: 100}
	emails := make(map[string]string)

	for {
		commits, resp, err := client.PullRequests.ListCommits(ctx, owner, repo, number, opts)
		if err != nil {
			return emails, err
		}
		for _, c := range commits {
			login := c.GetAuthor().GetLogin()
			email := c.GetCommit().GetAuthor().GetEmail()
			if login != "" && email != "" && !isNoReplyEmail(email) {
				if _, ok := emails[login]; !ok {
					emails[login] = email
				}
			}
			// Also check committer (may differ from author).
			cLogin := c.GetCommitter().GetLogin()
			cEmail := c.GetCommit().GetCommitter().GetEmail()
			if cLogin != "" && cEmail != "" && !isNoReplyEmail(cEmail) {
				if _, ok := emails[cLogin]; !ok {
					emails[cLogin] = cEmail
				}
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return emails, nil
}

// isNoReplyEmail returns true for GitHub's noreply placeholder emails.
func isNoReplyEmail(email string) bool {
	return strings.HasSuffix(email, "@users.noreply.github.com")
}

func truncateBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
