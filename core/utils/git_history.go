package utils

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"text/template"
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
	Hash        string
	Author      string
	Date        string
	Message     string
	Diff        string
	GitHubLogin string
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

	skipCommits := src.GetSkipCommits()
	skipPRs := src.GetSkipPrs()
	summaryOnly := src.GetCommitSummaryOnly()

	// Determine date range.
	sinceDate, untilDate := resolveDateRange(src)

	// Run commit fetch (clone+gitlog) and PR fetch concurrently.
	type commitResult struct {
		commits []parsedCommit
		err     error
	}
	type prResult struct {
		result prFetchResult
		err    error
	}

	commitCh := make(chan commitResult, 1)
	prCh := make(chan prResult, 1)

	// Goroutine 1: clone + git log (skipped entirely when skipCommits is set).
	go func() {
		if skipCommits {
			commitCh <- commitResult{}
			return
		}
		tmpDir, err := os.MkdirTemp("", "git-history-*")
		if err != nil {
			commitCh <- commitResult{err: fmt.Errorf("failed to create temp dir: %w", err)}
			return
		}
		defer func() { _ = os.RemoveAll(tmpDir) }()

		slog.Debug("Cloning repo for git history", "repo", fullName, "dest", tmpDir)
		if err := CloneGitRepo(ctx, repo, tmpDir, token); err != nil {
			commitCh <- commitResult{err: fmt.Errorf("failed to clone repo: %w", err)}
			return
		}
		logOutput, err := runGitLog(ctx, tmpDir, sinceDate, untilDate, summaryOnly)
		if err != nil {
			commitCh <- commitResult{err: fmt.Errorf("failed to run git log: %w", err)}
			return
		}
		commits := parseGitLog(logOutput)
		slog.Debug("Parsed commits", "count", len(commits))
		commitCh <- commitResult{commits: commits}
	}()

	// Goroutine 2: fetch PRs via API (skipped when skipPRs is set).
	go func() {
		if skipPRs {
			prCh <- prResult{}
			return
		}
		provider := strings.ToLower(strings.TrimSpace(repo.GetProvider()))
		var dateFilter *osdd.DatesFilter
		if src.HasDateFilter() {
			dateFilter = src.GetDateFilter()
		}
		fetched, err := fetchPRs(ctx, provider, fullName, token, dateFilter, summaryOnly)
		if err != nil {
			slog.Warn("Failed to fetch PRs, continuing with commits only", "error", err)
			prCh <- prResult{}
			return
		}
		slog.Debug("Fetched PRs", "count", len(fetched.PRs))
		prCh <- prResult{result: fetched}
	}()

	// Collect results.
	cr := <-commitCh
	if cr.err != nil {
		return nil, cr.err
	}
	commits := cr.commits

	pr := <-prCh
	prs := pr.result.PRs

	// Enrich commits with GitHub login via reverse email→login map from PRs.
	if loginEmails := pr.result.LoginEmails; len(loginEmails) > 0 {
		reverseMap := make(map[string]string, len(loginEmails))
		for login, email := range loginEmails {
			reverseMap[email] = login
		}
		for i := range commits {
			if email := parseEmailFromAuthor(commits[i].Author); email != "" {
				if login, ok := reverseMap[email]; ok {
					commits[i].GitHubLogin = login
				}
			}
		}
	}

	// Commits: batched by token limit.
	var files []GitHistoryFile
	if !skipCommits {
		commitItems := formatCommits(commits, summaryOnly)
		files = append(files, splitByTokenLimit(commitItems, maxTokens, "commits")...)
	}

	// PRs: one file per PR.
	if !skipPRs {
		for _, pr := range prs {
			files = append(files, GitHistoryFile{
				Name:    fmt.Sprintf("prs/PR-%d.md", pr.Number),
				Content: formatOnePR(pr, summaryOnly),
			})
		}
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

func runGitLog(ctx context.Context, repoDir, since, until string, summaryOnly bool) (string, error) {
	args := []string{"log", "--format=" + gitLogFormat, "--since=" + since}
	if !summaryOnly {
		args = append(args, "-p")
	}
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

func fetchPRs(ctx context.Context, provider, fullName, token string, dateFilter *osdd.DatesFilter, summaryOnly bool) (prFetchResult, error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return prFetchResult{}, fmt.Errorf("invalid full_name %q: expected owner/repo", fullName)
	}
	owner, repo := parts[0], parts[1]

	switch provider {
	case "", "github":
		return fetchGitHubPRs(ctx, owner, repo, token, dateFilter, summaryOnly)
	case "bitbucket":
		return fetchBitbucketPRs(ctx, owner, repo, token, dateFilter, summaryOnly)
	default:
		return prFetchResult{}, fmt.Errorf("unsupported provider for PR fetching: %s", provider)
	}
}

var commitTmpl = template.Must(template.New("commit").Parse(
	`## Commit {{.Hash}}

**Author:** {{.Author}}
{{- if .GitHubLogin}}
**GitHub:** {{.GitHubLogin}}
{{- end}}
**Date:** {{.Date}}
{{if .Message}}
### Message

{{.Message}}
{{end}}{{if .Diff}}
### Diff

` + "```diff" + `
{{.Diff}}
` + "```" + `
{{end}}`))

func formatCommits(commits []parsedCommit, summaryOnly bool) []formattedItem {
	items := make([]formattedItem, 0, len(commits))
	for _, c := range commits {
		data := c
		if summaryOnly {
			data.Diff = ""
		}
		var buf bytes.Buffer
		if err := commitTmpl.Execute(&buf, data); err != nil {
			slog.Warn("Failed to execute commit template", "hash", c.Hash, "err", err)
			continue
		}
		items = append(items, formattedItem{
			Label:   c.Hash,
			Content: buf.String(),
		})
	}
	return items
}

// prTmplData wraps a pullRequest with a SummaryOnly flag for the template.
type prTmplData struct {
	pullRequest
	SummaryOnly bool
	CreatedFmt  string
	UpdatedFmt  string
	MergedAtFmt string
}

var prFuncs = template.FuncMap{
	"join": strings.Join,
}

var prTmpl = template.Must(template.New("pr").Funcs(prFuncs).Parse(
	`## PR #{{.Number}}: {{.Title}}
{{- if .URL}}
**URL:** {{.URL}}
{{- end}}
**Author:** {{.Author}}
{{- if .AuthorEmail}}
**Author Email:** {{.AuthorEmail}}
{{- end}}
**State:** {{.State}}
{{- if .MergedBy}}
**Merged By:** {{.MergedBy}}{{if .MergedByEmail}} ({{.MergedByEmail}}){{end}}
{{- end}}
{{- if .MergedAtFmt}}
**Merged At:** {{.MergedAtFmt}}
{{- end}}
{{- if .BaseBranch}}
**Base Branch:** {{.BaseBranch}}
{{- end}}
{{- if .HeadBranch}}
**Head Branch:** {{.HeadBranch}}
{{- end}}
{{- if .Labels}}
**Labels:** {{join .Labels ", "}}
{{- end}}
{{- if or .Additions .Deletions .ChangedFiles}}
**Changes:** +{{.Additions}} -{{.Deletions}} ({{.ChangedFiles}} files)
{{- end}}
**Created:** {{.CreatedFmt}}
**Updated:** {{.UpdatedFmt}}
{{if .Body}}
### Description

{{.Body}}
{{end}}{{if not .SummaryOnly}}{{if .Reviews}}
### Reviews

{{range .Reviews}}- **{{.Author}}**{{if .AuthorEmail}} ({{.AuthorEmail}}){{end}} ({{.State}}){{if .Body}}: {{.Body}}{{end}}
{{end}}{{end}}{{if .Diff}}
### Diff

` + "```diff" + `
{{.Diff}}
` + "```" + `
{{end}}{{end}}`))

// formatOnePR formats a single pull request as a standalone markdown document.
// When summaryOnly is true, diffs and reviews are omitted.
func formatOnePR(pr pullRequest, summaryOnly bool) string {
	data := prTmplData{
		pullRequest: pr,
		SummaryOnly: summaryOnly,
		CreatedFmt:  pr.CreatedAt.Format(time.RFC3339),
		UpdatedFmt:  pr.UpdatedAt.Format(time.RFC3339),
		MergedAtFmt: formatTimeIfSet(pr.MergedAt),
	}
	var buf bytes.Buffer
	if err := prTmpl.Execute(&buf, data); err != nil {
		slog.Warn("Failed to execute PR template", "number", pr.Number, "err", err)
		return ""
	}
	return buf.String()
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

// parseEmailFromAuthor extracts the email from a git author string like "Name <email>".
func parseEmailFromAuthor(author string) string {
	start := strings.LastIndex(author, "<")
	end := strings.LastIndex(author, ">")
	if start >= 0 && end > start+1 {
		return author[start+1 : end]
	}
	return ""
}

func formatTimeIfSet(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
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
