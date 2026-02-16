package generators

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

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

	var resultEntries []*osdd.MaterializedResult_Entry

	for _, entry := range entries {
		ideFilter := entry.GetFilter().GetIde()
		if len(ideFilter) > 0 && genCtx.IDE != "" && !slices.Contains(ideFilter, genCtx.IDE) {
			continue
		}
		materializedEntry, err := c.materializeEntry(ctx, entry, genCtx)
		if err != nil {
			return nil, fmt.Errorf("failed to materialize entry for path %s: %w", entry.GetPath(), err)
		}
		resultEntries = append(resultEntries, materializedEntry)
	}

	return osdd.MaterializedResult_builder{
		Entries: resultEntries,
	}.Build(), nil
}

func (c *Context) materializeEntry(ctx context.Context, entry *recipes.ContextEntry, genCtx *core.GenerationContext) (*osdd.MaterializedResult_Entry, error) {
	path := entry.GetPath()
	if path == "" {
		return nil, fmt.Errorf("entry path cannot be empty")
	}

	if !entry.HasFrom() {
		return nil, fmt.Errorf("entry must have a 'from' source")
	}

	// GitRepo entries clone a full repository to disk and return a Directory entry.
	if entry.GetFrom().WhichType() == recipes.ContextFrom_GitRepo_case {
		return c.materializeGitRepo(ctx, entry, genCtx)
	}

	content, err := c.fetchContent(ctx, entry.GetFrom(), genCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch content: %w", err)
	}

	return osdd.MaterializedResult_Entry_builder{
		File: osdd.FullFileContent_builder{
			Path:    path,
			Content: content,
		}.Build(),
	}.Build(), nil
}

func (c *Context) materializeGitRepo(ctx context.Context, entry *recipes.ContextEntry, genCtx *core.GenerationContext) (*osdd.MaterializedResult_Entry, error) {
	path := entry.GetPath()
	slog.Debug("Materializing git repository context", "path", path)

	destPath := path
	if genCtx != nil && genCtx.WorkspacePath != "" {
		destPath = filepath.Join(genCtx.WorkspacePath, path)
	}

	if err := utils.CloneGitRepo(ctx, entry.GetFrom().GetGitRepo(), destPath); err != nil {
		return nil, fmt.Errorf("failed to clone git repository: %w", err)
	}

	return osdd.MaterializedResult_Entry_builder{
		Directory: &path,
	}.Build(), nil
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
		return "", fmt.Errorf("unknown or unset context source type")
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
