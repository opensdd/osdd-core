package providers

import (
	"context"

	"github.com/opensdd/osdd-api/clients/go/osdd"
	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
)

type IDEProvider interface {
	Materialize(ctx context.Context, ide *recipes.Ide) (*osdd.MaterializedResult, error)
}
