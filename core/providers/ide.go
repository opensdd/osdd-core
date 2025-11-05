package providers

import (
	"context"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
)

type IDE interface {
	Materialize(ctx context.Context, genCtx *core.GenerationContext, ide *recipes.Ide) (*osdd.MaterializedResult, error)
	PrepareStart(ctx context.Context, genCtx *core.GenerationContext) (core.ExecProps, error)
}
