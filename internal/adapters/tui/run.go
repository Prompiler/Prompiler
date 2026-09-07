// run.go holds the pure render helper shared by the interactive stages.
package tui

import (
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
)

// renderTemplate renders the named template of prog against inputs.
func renderTemplate(prog *domain.Program, name string, inputs eval.InputValues) (string, *eval.RuntimeError) {
	return eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs)
}
