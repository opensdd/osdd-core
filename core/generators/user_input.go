package generators

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/opensdd/osdd-api/clients/go/osdd/recipes"
	"github.com/opensdd/osdd-core/core"
)

// userInputTemplate is a Go text/template used to render the user input markdown.
// It is intentionally kept as a constant for clarity and reuse.
const userInputTemplate = `# User Input

{{range .Params}}## {{.Name}}

**Description**: {{.Description}}

**Value**: {{.Value}}

{{if not .IsLast}}---

{{end}}{{end}}`

// renderUserInput produces a Markdown document describing requested user inputs and their values.
func renderUserInput(spec *recipes.UserInputContextSource, genCtx *core.GenerationContext) (string, error) {
	if spec == nil {
		return "", fmt.Errorf("user input specification cannot be nil")
	}
	params := spec.GetEntries()
	if len(params) == 0 {
		return "", nil
	}
	userVals := map[string]string{}
	if genCtx != nil && genCtx.GetUserInput() != nil {
		userVals = genCtx.GetUserInput()
	}

	type paramVM struct {
		Name        string
		Description string
		Value       string
		IsLast      bool
	}

	var (
		missing []string
		vm      []paramVM
	)

	for _, p := range params {
		name := p.GetName()
		if name == "" {
			// Ignore unnamed parameters silently to avoid surprising errors
			continue
		}
		desc := p.GetDescription()
		val, ok := userVals[name]
		if !ok && !p.GetOptional() {
			missing = append(missing, name)
		}
		if !ok {
			val = ""
		}
		vm = append(vm, paramVM{Name: name, Description: desc, Value: val})
	}

	if len(missing) > 0 {
		return "", fmt.Errorf("missing required user input parameters: %s", strings.Join(missing, ", "))
	}

	// Mark the last element to suppress the trailing separator
	if len(vm) > 0 {
		vm[len(vm)-1].IsLast = true
	}

	// Render using Go template to ensure consistent formatting
	tpl, err := template.New("userInput").Parse(userInputTemplate)
	if err != nil {
		return "", fmt.Errorf("failed to parse user input template: %w", err)
	}
	var out bytes.Buffer
	data := struct{ Params []paramVM }{Params: vm}
	if err := tpl.Execute(&out, data); err != nil {
		return "", fmt.Errorf("failed to execute user input template: %w", err)
	}
	return out.String(), nil
}
