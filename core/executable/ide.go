package executable

import (
	"fmt"
	"strings"

	"github.com/opensdd/osdd-core/core/plugins/claude"
	"github.com/opensdd/osdd-core/core/plugins/codex"
	"github.com/opensdd/osdd-core/core/plugins/cursorcli"
	"github.com/opensdd/osdd-core/core/providers"
)

func getIDE(ideType string) (providers.IDE, error) {
	if ideType == "" {
		return nil, fmt.Errorf("ide type not provided")
	}
	switch strings.ToLower(ideType) {
	case "claude":
		return claude.NewIDEProvider(), nil
	case "cursor-cli":
		return cursorcli.NewIDEProvider(), nil
	case "codex":
		return codex.NewIDEProvider(), nil
	}
	return nil, fmt.Errorf("unsupported IDE type: [%v]", ideType)
}
