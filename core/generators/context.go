package generators

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
	"github.com/opensdd/osdd-core/core/utils"
)

type Context struct{}

func (c *Context) Materialize(ctx context.Context, contextMsg *recipes.Context, genCtx *core.GenerationContext) (*osdd.MaterializedResult, error) {
	if contextMsg == nil {
		return nil, fmt.Errorf("context cannot be nil")
	}

	entries := contextMsg.GetEntries()
	if entries == nil {
		return osdd.MaterializedResult_builder{}.Build(), nil
	}

	// Filter entries by IDE before parallelizing.
	var filtered []*recipes.ContextEntry
	for _, entry := range entries {
		ideFilter := entry.GetFilter().GetIde()
		if len(ideFilter) > 0 && genCtx.IDE != "" && !slices.Contains(ideFilter, genCtx.IDE) {
			continue
		}
		filtered = append(filtered, entry)
	}

	// Materialize entries in parallel with bounded concurrency.
	const maxConcurrency = 5
	type indexedResult struct {
		index   int
		entries []*osdd.MaterializedResult_Entry
		err     error
	}

	results := make([]indexedResult, len(filtered))
	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	for i, entry := range filtered {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, entry *recipes.ContextEntry) {
			defer wg.Done()
			defer func() { <-sem }()
			materialized, err := c.materializeEntry(ctx, entry, genCtx)
			results[i] = indexedResult{index: i, entries: materialized, err: err}
		}(i, entry)
	}
	wg.Wait()

	// Collect results in order; fail on first error.
	var resultEntries []*osdd.MaterializedResult_Entry
	for _, r := range results {
		if r.err != nil {
			return nil, fmt.Errorf("failed to materialize entry for path %s: %w", filtered[r.index].GetPath(), r.err)
		}
		resultEntries = append(resultEntries, r.entries...)
	}

	return osdd.MaterializedResult_builder{
		Entries: resultEntries,
	}.Build(), nil
}

func (c *Context) materializeEntry(ctx context.Context, entry *recipes.ContextEntry, genCtx *core.GenerationContext) ([]*osdd.MaterializedResult_Entry, error) {
	path := entry.GetPath()
	if path == "" {
		return nil, fmt.Errorf("entry path cannot be empty")
	}

	if !entry.HasFrom() {
		return nil, fmt.Errorf("entry must have a 'from' source")
	}

	from := entry.GetFrom()

	// URL fetch entries download bytes directly to disk.
	if from.WhichType() == recipes.ContextFrom_UrlFetch_case {
		return c.materializeUrlFetch(ctx, entry, genCtx)
	}

	// GitRepo entries clone a full repository to disk and return a Directory entry.
	if from.WhichType() == recipes.ContextFrom_GitRepo_case {
		e, err := c.materializeGitRepo(ctx, entry, genCtx)
		if err != nil {
			return nil, err
		}
		return []*osdd.MaterializedResult_Entry{e}, nil
	}

	// Jira/Linear entries produce multiple files: a summary index + one file per issue.
	if from.WhichType() == recipes.ContextFrom_JiraIssues_case {
		src := from.GetJiraIssues()
		token := resolveAuthToken(src.GetAuthTokenEnvVar(), genCtx)
		return c.materializeIssues(path, func() (*utils.IssuesResult, error) {
			return utils.FetchJiraIssues(ctx, src, token)
		})
	}
	if from.WhichType() == recipes.ContextFrom_LinearIssues_case {
		src := from.GetLinearIssues()
		token := resolveAuthToken(src.GetAuthTokenEnvVar(), genCtx)
		return c.materializeIssues(path, func() (*utils.IssuesResult, error) {
			return utils.FetchLinearIssues(ctx, src, token)
		})
	}

	if from.WhichType() == recipes.ContextFrom_GitHistory_case {
		src := from.GetGitHistory()
		token := resolveAuthToken(src.GetRepo().GetAuthTokenEnvVar(), genCtx)
		return c.materializeGitHistory(path, func() (*utils.GitHistoryResult, error) {
			return utils.FetchGitHistory(ctx, src, token)
		})
	}

	content, err := c.fetchContent(ctx, from, genCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	return []*osdd.MaterializedResult_Entry{
		osdd.MaterializedResult_Entry_builder{
			File: osdd.FullFileContent_builder{
				Path:    path,
				Content: content,
			}.Build(),
		}.Build(),
	}, nil
}

// materializeIssues converts an IssuesResult into a summary file and per-issue files.
// Path is treated as a folder: the summary is written to <path>/all-issues.json;
// individual issues go to <path>/issues/<id>.json.
func (c *Context) materializeIssues(path string, fetch func() (*utils.IssuesResult, error)) ([]*osdd.MaterializedResult_Entry, error) {
	result, err := fetch()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch issues: %w", err)
	}

	summaryJSON, err := json.MarshalIndent(result.Summary, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal issues summary: %w", err)
	}

	entries := make([]*osdd.MaterializedResult_Entry, 0, 1+len(result.Issues))

	// Summary file at <path>/all-issues.json
	summaryPath := path + "/all-issues.json"
	entries = append(entries, osdd.MaterializedResult_Entry_builder{
		File: osdd.FullFileContent_builder{
			Path:    summaryPath,
			Content: string(summaryJSON),
		}.Build(),
	}.Build())

	// Per-issue files at <path>/issues/<id>.json
	for _, s := range result.Summary {
		issuePath := path + "/issues/" + s.ID + ".json"
		entries = append(entries, osdd.MaterializedResult_Entry_builder{
			File: osdd.FullFileContent_builder{
				Path:    issuePath,
				Content: result.Issues[s.ID],
			}.Build(),
		}.Build())
	}

	return entries, nil
}

// materializeGitHistory converts a GitHistoryResult into one MaterializedResult_Entry per file.
// Path is treated as a folder: files are written to <path>/<file.Name>.
func (c *Context) materializeGitHistory(path string, fetch func() (*utils.GitHistoryResult, error)) ([]*osdd.MaterializedResult_Entry, error) {
	result, err := fetch()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch git history: %w", err)
	}

	entries := make([]*osdd.MaterializedResult_Entry, 0, len(result.Files))
	for _, f := range result.Files {
		filePath := path + "/" + f.Name
		entries = append(entries, osdd.MaterializedResult_Entry_builder{
			File: osdd.FullFileContent_builder{
				Path:    filePath,
				Content: f.Content,
			}.Build(),
		}.Build())
	}

	return entries, nil
}

func (c *Context) materializeGitRepo(ctx context.Context, entry *recipes.ContextEntry, genCtx *core.GenerationContext) (*osdd.MaterializedResult_Entry, error) {
	path := entry.GetPath()
	slog.Debug("Materializing git repository context", "path", path)

	destPath := path
	if genCtx != nil && genCtx.WorkspacePath != "" {
		destPath = filepath.Join(genCtx.WorkspacePath, path)
	}

	repo := entry.GetFrom().GetGitRepo()
	token := resolveAuthToken(repo.GetAuthTokenEnvVar(), genCtx)
	if err := utils.CloneGitRepo(ctx, repo, destPath, token); err != nil {
		return nil, fmt.Errorf("failed to clone git repository: %w", err)
	}

	return osdd.MaterializedResult_Entry_builder{
		Directory: &path,
	}.Build(), nil
}

// urlFetchMaxAttempts is the maximum number of fetch attempts for URL context entries.
var urlFetchMaxAttempts = 3

// urlFetchBackoffBase is the base duration for exponential backoff between retry attempts.
var urlFetchBackoffBase = time.Second

// urlFetchBackoff returns the backoff duration for the given attempt (0-indexed).
func urlFetchBackoff(attempt int) time.Duration {
	return time.Duration(1<<uint(attempt)) * urlFetchBackoffBase // 1x, 2x, 4x base
}

func (c *Context) materializeUrlFetch(ctx context.Context, entry *recipes.ContextEntry, genCtx *core.GenerationContext) ([]*osdd.MaterializedResult_Entry, error) {
	src := entry.GetFrom().GetUrlFetch()
	rawURL := src.GetUrl()
	optional := src.GetOptional()
	path := entry.GetPath()

	slog.Debug("Fetching URL context", "url", rawURL, "path", path, "optional", optional)

	// Validate URL — validation errors always fail, regardless of optional.
	if err := utils.ValidateURL(rawURL); err != nil {
		return nil, err
	}

	// Validate destination path against workspace root.
	if genCtx == nil || genCtx.WorkspacePath == "" {
		return nil, fmt.Errorf("workspace path is required for url fetch")
	}
	destPath := filepath.Join(genCtx.WorkspacePath, filepath.Clean(path))
	if !core.IsPathWithinRoot(genCtx.WorkspacePath, destPath) {
		return nil, fmt.Errorf("destination path escapes workspace: %s", path)
	}

	// Retry loop.
	var data []byte
	var lastErr error
	for attempt := range urlFetchMaxAttempts {
		data, lastErr = utils.FetchURL(ctx, rawURL)
		if lastErr == nil {
			break
		}
		if ctx.Err() != nil {
			lastErr = fmt.Errorf("context cancelled while fetching url %s for path %s: %w", rawURL, path, ctx.Err())
			break
		}
		if attempt < urlFetchMaxAttempts-1 {
			slog.Debug("Retrying URL fetch", "url", rawURL, "attempt", attempt+1, "error", lastErr)
			select {
			case <-ctx.Done():
				return nil, fmt.Errorf("context cancelled while fetching url %s for path %s: %w", rawURL, path, ctx.Err())
			case <-time.After(urlFetchBackoff(attempt)):
			}
		}
	}

	if lastErr != nil {
		if optional {
			slog.Warn("URL fetch failed, skipping optional entry", "url", rawURL, "path", path, "error", lastErr)
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch url %s for path %s: %w", rawURL, path, lastErr)
	}

	// Atomic write: temp file → rename.
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directories for %s: %w", destPath, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".url-fetch-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(data); err != nil {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to write temp file %s: %w", tmpPath, err)
	}
	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to close temp file %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("failed to rename temp file to %s: %w", destPath, err)
	}

	slog.Debug("URL content written", "url", rawURL, "path", path, "bytes", len(data))

	// File is already on disk; return no entries to avoid PersistMaterializedResult overwriting.
	return nil, nil
}

func (c *Context) fetchContent(ctx context.Context, from *recipes.ContextFrom, genCtx *core.GenerationContext) (string, error) {
	if from == nil {
		return "", fmt.Errorf("from source cannot be nil")
	}

	switch from.WhichType() {
	case recipes.ContextFrom_Text_case:
		return from.GetText(), nil

	case recipes.ContextFrom_Cmd_case:
		return utils.ExecuteCommand(ctx, from.GetCmd())

	case recipes.ContextFrom_Github_case:
		return utils.FetchGithub(ctx, from.GetGithub())

	case recipes.ContextFrom_Combined_case:
		return c.fetchCombined(ctx, from.GetCombined(), genCtx)

	case recipes.ContextFrom_PrefetchId_case:
		data, ok := genCtx.GetPrefetched()[from.GetPrefetchId()]
		if !ok {
			return "", fmt.Errorf("prefetch id [%v] not found", from.GetPrefetchId())
		}
		return data.GetData(), nil

	case recipes.ContextFrom_UserInput_case:
		return renderUserInput(from.GetUserInput(), genCtx)

	case recipes.ContextFrom_LocalFile_case:
		p := strings.TrimSpace(from.GetLocalFile())
		if p == "" {
			return "", fmt.Errorf("local file path cannot be empty")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("failed to read local file %s: %w", p, err)
		}
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown or unset context source type [%+v]", from.WhichType())
	}
}

func (c *Context) fetchCombined(ctx context.Context, combined *recipes.CombinedContextSource, genCtx *core.GenerationContext) (string, error) {
	if combined == nil {
		return "", fmt.Errorf("combined source cannot be nil")
	}

	items := combined.GetItems()
	if len(items) == 0 {
		return "", nil
	}

	var builder strings.Builder
	for i, item := range items {
		content, err := c.fetchCombinedItem(ctx, item, genCtx)
		if err != nil {
			return "", fmt.Errorf("failed to fetch combined item %d: %w", i, err)
		}
		builder.WriteString(content)
	}

	return builder.String(), nil
}

func (c *Context) fetchCombinedItem(ctx context.Context, item *recipes.CombinedContextSource_Item, genCtx *core.GenerationContext) (string, error) {
	if item == nil {
		return "", fmt.Errorf("combined item cannot be nil")
	}

	switch item.WhichType() {
	case recipes.CombinedContextSource_Item_Text_case:
		return item.GetText(), nil

	case recipes.CombinedContextSource_Item_Cmd_case:
		return utils.ExecuteCommand(ctx, item.GetCmd())

	case recipes.CombinedContextSource_Item_Github_case:
		return utils.FetchGithub(ctx, item.GetGithub())

	case recipes.CombinedContextSource_Item_PrefetchId_case:
		data, ok := genCtx.GetPrefetched()[item.GetPrefetchId()]
		if !ok {
			return "", fmt.Errorf("prefetch id [%v] not found", item.GetPrefetchId())
		}
		return data.GetData(), nil

	case recipes.CombinedContextSource_Item_UserInput_case:
		return renderUserInput(item.GetUserInput(), genCtx)

	case recipes.CombinedContextSource_Item_LocalFile_case:
		p := strings.TrimSpace(item.GetLocalFile())
		if p == "" {
			return "", fmt.Errorf("local file path cannot be empty")
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", fmt.Errorf("failed to read local file %s: %w", p, err)
		}
		return string(b), nil

	default:
		return "", fmt.Errorf("unknown or unset combined item type")
	}
}

// resolveAuthToken resolves an auth token env var name through genCtx.ResolveEnv.
func resolveAuthToken(envVar string, genCtx *core.GenerationContext) string {
	if envVar == "" {
		return ""
	}
	return genCtx.ResolveEnv(envVar)
}
