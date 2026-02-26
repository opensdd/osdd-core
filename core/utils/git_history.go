package utils

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/pkoukk/tiktoken-go"
)

const (
	defaultMaxFileTokens = 50000
	defaultSinceDays     = 30
	commitBoundary       = "COMMIT_BOUNDARY"
	gitLogFormat         = commitBoundary + "%ncommit %H%nAuthor: %an <%ae>%nDate: %aI%n%n%w(0,4,4)%B" // %B = subject+body
)

// GitHistoryResult contains the output files produced by FetchGitHistory.
type GitHistoryResult struct {
	Files []GitHistoryFile
}

// GitHistoryFile is a single output file with a name and markdown content.
type GitHistoryFile struct {
	Name    string
	Content string
}

// parsedCommit holds a single parsed commit from git log output.
type parsedCommit struct {
	Hash    string
	Author  string
	Date    string
	Message string
	Diff    string
}

// formattedItem is a piece of formatted markdown ready for splitting.
type formattedItem struct {
	Label   string // human-readable label for logging
	Content string // markdown content
}

// FetchGitHistory clones the repository described by src, extracts commits
// and pull requests within the configured date range, and returns the results
// as token-limited markdown files.
func FetchGitHistory(ctx context.Context, src *recipes.GitHistorySource, token string) (*GitHistoryResult, error) {
	if src == nil {
		return nil, fmt.Errorf("git history source cannot be nil")
	}
	repo := src.GetRepo()
	if repo == nil {
		return nil, fmt.Errorf("git history source repo cannot be nil")
	}
	fullName := strings.TrimSpace(repo.GetFullName())
	if fullName == "" {
		return nil, fmt.Errorf("git history source repo full_name cannot be empty")
	}

	maxTokens := int(src.GetMaxFileTokens())
	if maxTokens <= 0 {
		maxTokens = defaultMaxFileTokens
	}

	// Determine date range.
	sinceDate, untilDate := resolveDateRange(src)

	// Clone into a temp dir.
	tmpDir, err := os.MkdirTemp("", "git-history-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	slog.Debug("Cloning repo for git history", "repo", fullName, "dest", tmpDir)
	if err := CloneGitRepo(ctx, repo, tmpDir, token); err != nil {
		return nil, fmt.Errorf("failed to clone repo: %w", err)
	}

	// Run git log.
	logOutput, err := runGitLog(ctx, tmpDir, sinceDate, untilDate)
	if err != nil {
		return nil, fmt.Errorf("failed to run git log: %w", err)
	}

	commits := parseGitLog(logOutput)
	slog.Debug("Parsed commits", "count", len(commits))

	// Fetch PRs.
	provider := strings.ToLower(strings.TrimSpace(repo.GetProvider()))
	var dateFilter *osdd.DatesFilter
	if src.HasDateFilter() {
		dateFilter = src.GetDateFilter()
	}
	prs, err := fetchPRs(ctx, provider, fullName, token, dateFilter)
	if err != nil {
		slog.Warn("Failed to fetch PRs, continuing with commits only", "er"+
			"ror", err)
		prs = nil
	}
	slog.Debug("Fetched PRs", "count", len(prs))

	// Commits: batched by token limit.
	commitItems := formatCommits(commits)
	var files []GitHistoryFile
	files = append(files, splitByTokenLimit(commitItems, maxTokens, "commits")...)

	// PRs: one file per PR.
	for _, pr := range prs {
		files = append(files, GitHistoryFile{
			Name:    fmt.Sprintf("prs/PR-%d.md", pr.Number),
			Content: formatOnePR(pr),
		})
	}

	return &GitHistoryResult{Files: files}, nil
}

func resolveDateRange(src *recipes.GitHistorySource) (since, until string) {
	df := src.GetDateFilter()
	if df != nil && df.HasFrom() {
		since = df.GetFrom().AsTime().UTC().Format("2006-01-02")
	} else {
		since = time.Now().AddDate(0, 0, -defaultSinceDays).UTC().Format("2006-01-02")
	}
	if df != nil && df.HasTo() {
		// Add one day to make "to" inclusive for git log --until.
		until = df.GetTo().AsTime().UTC().AddDate(0, 0, 1).Format("2006-01-02")
	}
	return since, until
}

func runGitLog(ctx context.Context, repoDir, since, until string) (string, error) {
	args := []string{"log", "--format=" + gitLogFormat, "-p", "--since=" + since}
	if until != "" {
		args = append(args, "--until="+until)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = repoDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git log failed: %w (output: %s)", err, string(output))
	}
	return string(output), nil
}

// parseGitLog splits raw git log output into individual commits.
func parseGitLog(output string) []parsedCommit {
	if strings.TrimSpace(output) == "" {
		return nil
	}

	// Split by boundary marker.
	parts := strings.Split(output, commitBoundary+"\n")
	var commits []parsedCommit

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		c := parseOneCommit(part)
		if c.Hash != "" {
			commits = append(commits, c)
		}
	}
	return commits
}

func parseOneCommit(raw string) parsedCommit {
	var c parsedCommit
	lines := strings.Split(raw, "\n")

	// Find the commit header, author, date.
	i := 0
	for i < len(lines) {
		line := lines[i]
		switch {
		case strings.HasPrefix(line, "commit "):
			c.Hash = strings.TrimPrefix(line, "commit ")
		case strings.HasPrefix(line, "Author: "):
			c.Author = strings.TrimPrefix(line, "Author: ")
		case strings.HasPrefix(line, "Date: "):
			c.Date = strings.TrimSpace(strings.TrimPrefix(line, "Date: "))
		}
		i++
		if c.Hash != "" && c.Author != "" && c.Date != "" {
			break
		}
	}

	// After the header, find the message and diff.
	var msgLines []string
	var diffLines []string
	inDiff := false

	for ; i < len(lines); i++ {
		line := lines[i]
		if !inDiff && strings.HasPrefix(line, "diff --git") {
			inDiff = true
		}
		if inDiff {
			diffLines = append(diffLines, line)
		} else {
			msgLines = append(msgLines, line)
		}
	}

	c.Message = strings.TrimSpace(strings.Join(msgLines, "\n"))
	c.Diff = strings.Join(diffLines, "\n")
	return c
}

func fetchPRs(ctx context.Context, provider, fullName, token string, dateFilter *osdd.DatesFilter) ([]pullRequest, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid full_name %q: expected owner/repo", fullName)
	}
	owner, repo := parts[0], parts[1]

	switch provider {
	case "", "github":
		return fetchGitHubPRs(ctx, owner, repo, token, dateFilter)
	case "bitbucket":
		return fetchBitbucketPRs(ctx, owner, repo, token, dateFilter)
	default:
		return nil, fmt.Errorf("unsupported provider for PR fetching: %s", provider)
	}
}

func formatCommits(commits []parsedCommit) []formattedItem {
	items := make([]formattedItem, 0, len(commits))
	for _, c := range commits {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("## Commit %s\n\n", c.Hash))
		sb.WriteString(fmt.Sprintf("**Author:** %s\n", c.Author))
		sb.WriteString(fmt.Sprintf("**Date:** %s\n\n", c.Date))
		if c.Message != "" {
			sb.WriteString(fmt.Sprintf("### Message\n\n%s\n\n", c.Message))
		}
		if c.Diff != "" {
			sb.WriteString("### Diff\n\n```diff\n")
			sb.WriteString(c.Diff)
			sb.WriteString("\n```\n\n")
		}
		items = append(items, formattedItem{
			Label:   c.Hash,
			Content: sb.String(),
		})
	}
	return items
}

// formatOnePR formats a single pull request as a standalone markdown document.
func formatOnePR(pr pullRequest) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("## PR #%d: %s\n\n", pr.Number, pr.Title))
	sb.WriteString(fmt.Sprintf("**Author:** %s\n", pr.Author))
	sb.WriteString(fmt.Sprintf("**State:** %s\n", pr.State))
	sb.WriteString(fmt.Sprintf("**Created:** %s\n", pr.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Updated:** %s\n\n", pr.UpdatedAt.Format(time.RFC3339)))
	if pr.Body != "" {
		sb.WriteString(fmt.Sprintf("### Description\n\n%s\n\n", pr.Body))
	}
	if len(pr.Reviews) > 0 {
		sb.WriteString("### Reviews\n\n")
		for _, r := range pr.Reviews {
			sb.WriteString(fmt.Sprintf("- **%s** (%s)", r.Author, r.State))
			if r.Body != "" {
				sb.WriteString(fmt.Sprintf(": %s", r.Body))
			}
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	if pr.Diff != "" {
		sb.WriteString("### Diff\n\n```diff\n")
		sb.WriteString(pr.Diff)
		sb.WriteString("\n```\n\n")
	}
	return sb.String()
}

// splitByTokenLimit groups formatted items into files that stay under maxTokens.
// Each file is named "{prefix}-001.md", "{prefix}-002.md", etc.
func splitByTokenLimit(items []formattedItem, maxTokens int, prefix string) []GitHistoryFile {
	if len(items) == 0 {
		return nil
	}

	var files []GitHistoryFile
	var current strings.Builder
	currentTokens := 0
	fileNum := 1

	for _, item := range items {
		itemTokens := countTokens(item.Content)

		// If adding this item would exceed the limit and we already have content, flush.
		if currentTokens > 0 && currentTokens+itemTokens > maxTokens {
			files = append(files, GitHistoryFile{
				Name:    fmt.Sprintf("%s-%03d.md", prefix, fileNum),
				Content: current.String(),
			})
			current.Reset()
			currentTokens = 0
			fileNum++
		}

		current.WriteString(item.Content)
		currentTokens += itemTokens
	}

	// Flush remaining.
	if current.Len() > 0 {
		files = append(files, GitHistoryFile{
			Name:    fmt.Sprintf("%s-%03d.md", prefix, fileNum),
			Content: current.String(),
		})
	}

	return files
}

// countTokens estimates the number of tokens in text using cl100k_base encoding.
func countTokens(text string) int {
	enc, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		// Fallback: rough estimate of 4 chars per token.
		return len(text) / 4
	}
	return len(enc.Encode(text, nil, nil))
}
