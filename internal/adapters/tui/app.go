// app.go is the TUI composition root: the public surface the CLI depends on.
package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Jh123x/prompiler/internal/domain"
)

// App is an interactive front end over a domain.Application.
type App struct {
	app *domain.Application
}

// NewApp wires the TUI to the injected application.
func NewApp(app *domain.Application) *App {
	return &App{app: app}
}

// Run starts the interactive session rooted at root (defaulting to "."). It
// returns nil once the user quits.
func (a *App) Run(root string) error {
	m := newModel(a.app, root)
	p := tea.NewProgram(m, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return err
	}
	// Print any pending stdout output after the alternate screen is restored.
	if fm, ok := final.(*Model); ok && fm.stdoutOutput != "" {
		fmt.Print(fm.stdoutOutput)
	}
	return nil
}
