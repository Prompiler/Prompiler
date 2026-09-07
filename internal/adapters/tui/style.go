// style.go centralises the lipgloss styles used by the interactive views.
package tui

import "github.com/charmbracelet/lipgloss"

var (
	// titleStyle styles headings (browse root, form header, result title).
	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))

	// stageStyle styles the current-stage footer.
	stageStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	// rowStyle is the default body row.
	rowStyle = lipgloss.NewStyle()

	// selStyle highlights the selected row.
	selStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Bold(true)

	// hintStyle renders help / secondary text.
	hintStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	// errStyle renders diagnostics and validation errors.
	errStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))

	// okStyle renders success messages.
	okStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("76"))

	// errRowStyle renders a browse row whose source file has a syntax error.
	errRowStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
)
