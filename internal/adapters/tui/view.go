// view.go renders each stage of the interactive model to a string.
package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/types"
)

// View implements tea.Model.
func (m *Model) View() string {
	switch m.stage {
	case stageBrowse:
		return m.viewBrowse()
	case stageForm:
		return m.viewForm()
	case stageOutput:
		return m.viewOutput()
	case stageResult:
		return m.viewResult()
	}
	return ""
}

func (m *Model) viewBrowse() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Prompiler — interactive run") + "\n")
	b.WriteString(hintStyle.Render("root: "+m.rootPath) + "\n\n")
	if m.browseDetail != "" {
		b.WriteString(errStyle.Render("Error in "+m.templates[m.browseSel].Path+":") + "\n")
		b.WriteString(errStyle.Render(m.browseDetail) + "\n")
		b.WriteString(hintStyle.Render("\n[esc/b] back · [q] quit") + "\n")
		return b.String()
	}
	if m.browseErr != "" {
		b.WriteString(errStyle.Render(m.browseErr) + "\n")
		return b.String()
	}
	if len(m.templates) == 0 {
		b.WriteString(rowStyle.Render("(no templates found under "+m.rootPath+")") + "\n")
		return b.String()
	}
	for i, t := range m.templates {
		marker := "  "
		style := rowStyle
		if i == m.browseSel {
			marker = "> "
			style = selStyle
		}
		if len(t.Diagnostics) > 0 {
			style = errRowStyle
		}
		line := t.Path + " — " + t.Name
		if t.Name == "" {
			line = t.Path + " — (syntax error)"
		}
		if t.Description != "" {
			line += " — " + t.Description
		}
		b.WriteString(style.Render(marker+line) + "\n")
	}
	if m.pathEdit {
		b.WriteString("\n" + rowStyle.Render("root path: "+m.pathInput.Value()+"█  (→/enter confirm, ←/esc cancel)") + "\n")
	} else {
		b.WriteString(hintStyle.Render("\n↑/↓ choose · → select · ← back · p edit root · q quit") + "\n")
	}
	return b.String()
}

func (m *Model) viewForm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Edit: "+m.templateName) + "\n")
	if m.formErr != "" {
		b.WriteString(errStyle.Render("Validation errors:\n"+m.formErr) + "\n")
	}
	b.WriteString("\n")
	if m.view != nil {
		b.WriteString(hintStyle.Render("path: "+m.breadcrumb()) + "\n\n")
		for i := range m.view.rows {
			marker := "  "
			style := rowStyle
			if i == m.view.idx {
				marker = "> "
				style = selStyle
			}
			b.WriteString(style.Render(marker+m.describeRow(&m.view.rows[i], i)) + "\n")
		}
	}
	if m.editKind != editNone {
		target := "field"
		if m.editField != nil {
			target = m.editField.label
		}
		if m.editEntry != nil {
			target = "map key"
		}
		b.WriteString("\n" + rowStyle.Render("editing "+target+": "+m.ti.Value()+"█") + "\n")
	} else {
		b.WriteString(hintStyle.Render("\n↑/↓ move · → select · ← back · a add · d remove · r run · q quit") + "\n")
	}
	return b.String()
}

func (m *Model) viewOutput() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("Choose output target") + "\n\n")
	targets := []string{"stdout", "file", "clipboard"}
	for i, n := range targets {
		marker := "  "
		style := rowStyle
		if i == m.outSel {
			marker = "> "
			style = selStyle
		}
		b.WriteString(style.Render(marker+n) + "\n")
	}
	switch m.outMode {
	case outputPath:
		b.WriteString("\n" + rowStyle.Render("file path: "+m.fileInput.Value()+"█") + "\n")
		b.WriteString(hintStyle.Render("(→/enter confirms, ←/esc cancels)") + "\n")
	case outputConfirm:
		b.WriteString("\n" + errStyle.Render("overwrite existing file "+m.filePathVal+"? (y/n)") + "\n")
	default:
		b.WriteString(hintStyle.Render("\n↑/↓ choose · → select · ← back · q quit") + "\n")
	}
	if m.outKind == outFile && m.outMode == outputPick && m.filePathVal != "" {
		b.WriteString(hintStyle.Render("current target file: "+m.filePathVal) + "\n")
	}
	return b.String()
}

func (m *Model) viewResult() string {
	var b strings.Builder
	if m.resultOK {
		b.WriteString(okStyle.Render("Rendered to "+m.targetDesc) + "\n\n")
		switch m.outKind {
		case outStdout:
			b.WriteString(m.rendered + "\n")
		case outFile:
			b.WriteString(hintStyle.Render("wrote "+strconv.Itoa(len(m.rendered))+" chars to "+m.targetDesc) + "\n")
		case outClipboard:
			b.WriteString(hintStyle.Render("copied "+strconv.Itoa(len(m.rendered))+" chars to the clipboard") + "\n")
		}
		b.WriteString(hintStyle.Render("\n← back · q quit") + "\n")
	} else {
		b.WriteString(errStyle.Render("Error: "+resultErrText(m.resultErr)) + "\n")
		b.WriteString(hintStyle.Render("\n← back · → retry · q quit") + "\n")
	}
	return b.String()
}

func resultErrText(err error) string {
	if rerr, ok := err.(*eval.RuntimeError); ok {
		return fmt.Sprintf("%s: %s", rerr.Category, rerr.Message)
	}
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}

func (m *Model) breadcrumb() string {
	var titles []string
	for v := m.view; v != nil; v = v.parent {
		titles = append([]string{v.title}, titles...)
	}
	return strings.Join(titles, " / ")
}

// describeRow renders a single editable row with its label, type and summary.
func (m *Model) describeRow(r *formRow, idx int) string {
	if r.choice != nil {
		return r.choice.Name + "  (satisfies " + typeString(r.field.typ) + ")"
	}
	if r.option != "" {
		return r.option
	}
	f := r.field
	if f == nil {
		return "(empty)"
	}
	name := f.label
	if name == "" {
		switch {
		case r.entry != nil:
			if r.entry.key == "" {
				name = "«new entry»"
			} else {
				name = "key " + strconv.Quote(r.entry.key)
			}
		default:
			name = fmt.Sprintf("#%d", idx)
		}
	}
	line := name + "  " + typeString(f.typ) + "  = " + valueSummary(f)
	if f.optional {
		line += "  (optional, default: " + f.default_ + ")"
	}
	return line
}

func typeString(t types.Type) string {
	if t == nil {
		return "?"
	}
	return t.String()
}

func valueSummary(f *FormField) string {
	switch f.typ.(type) {
	case types.Primitive:
		if f.text == "" {
			return "(empty)"
		}
		return f.text
	case *types.Enum:
		if f.text == "" {
			return "(choose a member)"
		}
		return f.text
	case *types.Class:
		return plural(len(f.fields), "field")
	case *types.Interface:
		if f.chosen < 0 {
			return "(choose a class)"
		}
		if f.chosen >= 0 && f.chosen < len(f.choices) {
			return f.choices[f.chosen].Name + ", " + plural(len(f.fields), "field")
		}
		return "(choose a class)"
	case *types.Array:
		return plural(len(f.elems), "element")
	case *types.Map:
		return plural(len(f.entries), "entry")
	case *types.Optional:
		if !f.present {
			return "none"
		}
		if f.child == nil {
			return "some(?)"
		}
		return "some(" + valueSummary(f.child) + ")"
	}
	return typeString(f.typ)
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}
