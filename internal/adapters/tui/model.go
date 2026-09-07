// model.go holds the Bubble Tea Model: a thin adapter over the pure form state
// that turns key presses into edits of the same InputForm the pure tests drive.
package tui

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/Jh123x/prompiler/internal/adapters/output"
	"github.com/Jh123x/prompiler/internal/ast"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/token"
	"github.com/Jh123x/prompiler/internal/types"
)

// stage is the interactive state machine.
type stage int

const (
	stageBrowse stage = iota
	stageForm
	stageOutput
	stageResult
)

// outKind identifies the chosen output sink.
type outKind int

const (
	outStdout outKind = iota
	outFile
	outClipboard
)

// outputMode narrows stageOutput.
type outputMode int

const (
	outputPick outputMode = iota
	outputPath
	outputConfirm
)

// editKind distinguishes what the shared text editor is editing.
type editKind int

const (
	editNone editKind = iota
	editText
	editKey
)

const defaultRoot = "."

// formRow is one editable row inside a form view.
type formRow struct {
	field  *FormField
	entry  *mapEntry    // non-nil when the row is a map entry (key text lives here)
	choice *types.Class // non-nil in an interface class chooser
}

// formView is the visible container of rows being navigated.
type formView struct {
	title     string
	rows      []formRow
	idx       int
	container *FormField // field whose children these rows are (nil at the root)
	parent    *formView
}

// Model is the Bubble Tea model. It mutates shared state in place so tests can
// drive it by constructing tea.KeyMsg and calling Update directly.
type Model struct {
	app   *domain.Application
	stage stage

	// browse
	rootPath  string
	templates []domain.TemplateInfo
	browseSel int
	browseErr string
	pathEdit  bool
	pathInput textinput.Model

	// form
	templateName string
	prog         *domain.Program
	form         *InputForm
	view         *formView
	formErr      string

	// text editor (scalar values and map keys)
	ti        textinput.Model
	editKind  editKind
	editField *FormField
	editEntry *mapEntry

	// output
	outKind     outKind
	outSel      int
	outMode     outputMode
	fileInput   textinput.Model
	filePathVal string

	// result
	rendered   string
	targetDesc string
	resultErr  error
	resultOK   bool
}

// newModel builds a fresh model rooted at root (defaulting to ".").
func newModel(app *domain.Application, root string) *Model {
	if root == "" {
		root = defaultRoot
	}
	m := &Model{app: app, stage: stageBrowse, rootPath: root}
	m.pathInput = textinput.New()
	m.pathInput.Prompt = ""
	m.pathInput.Placeholder = "path to .ppl files"
	m.ti = textinput.New()
	m.ti.Prompt = ""
	m.ti.Placeholder = "type and press enter"
	m.fileInput = textinput.New()
	m.fileInput.Prompt = ""
	m.fileInput.Placeholder = "output file path"
	m.reloadBrowse()
	return m
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	if key.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	switch m.stage {
	case stageBrowse:
		return m, m.updateBrowse(key)
	case stageForm:
		return m, m.updateForm(key)
	case stageOutput:
		return m, m.updateOutput(key)
	case stageResult:
		return m, m.updateResult(key)
	}
	return m, nil
}

// --- browse ---

func (m *Model) reloadBrowse() {
	m.browseSel = 0
	m.browseErr = ""
	templates, err := m.app.ListTemplates(m.rootPath)
	if err != nil {
		m.templates = nil
		m.browseErr = err.Error()
		return
	}
	m.templates = templates
}

func (m *Model) updateBrowse(key tea.KeyMsg) tea.Cmd {
	if m.pathEdit {
		switch key.Type {
		case tea.KeyEnter:
			p := strings.TrimSpace(m.pathInput.Value())
			if p != "" {
				m.rootPath = p
			}
			m.pathEdit = false
			m.pathInput.Blur()
			m.reloadBrowse()
		case tea.KeyEsc:
			m.pathEdit = false
			m.pathInput.Blur()
		default:
			m.pathInput, _ = m.pathInput.Update(key)
		}
		return nil
	}
	switch {
	case key.Type == tea.KeyEnter:
		if len(m.templates) > 0 {
			m.openTemplate(m.templates[m.browseSel].Name)
		}
	case key.Type == tea.KeyUp:
		if m.browseSel > 0 {
			m.browseSel--
		}
	case key.Type == tea.KeyDown:
		if m.browseSel < len(m.templates)-1 {
			m.browseSel++
		}
	case key.Type == tea.KeyRunes && len(key.Runes) == 1:
		switch key.Runes[0] {
		case 'p':
			m.pathEdit = true
			m.pathInput.SetValue(m.rootPath)
			m.pathInput.CursorEnd()
			m.pathInput.Focus()
		case 'q':
			return tea.Quit
		}
	}
	return nil
}

func (m *Model) openTemplate(name string) {
	prog, diags := m.app.AnalyzeRoot(m.rootPath)
	if len(diags) > 0 {
		m.browseErr = formatDiags(diags)
		return
	}
	var td *ast.TemplateDecl
	for _, f := range prog.Files {
		for _, d := range f.Decls {
			if t, ok := d.(*ast.TemplateDecl); ok && t.Name == name {
				td = t
			}
		}
	}
	if td == nil {
		m.browseErr = fmt.Sprintf("template %q not found", name)
		return
	}
	m.prog = prog
	m.templateName = name
	m.form = BuildForm(td, prog.Sem.TemplateVars[name], prog.Sem.Env)
	m.view = rootView(m.form, name)
	m.formErr = ""
	m.stage = stageForm
}

// --- form ---

func (m *Model) updateForm(key tea.KeyMsg) tea.Cmd {
	if m.editKind != editNone {
		switch key.Type {
		case tea.KeyEnter:
			m.commitEdit()
		case tea.KeyEsc:
			m.endEdit()
		default:
			m.ti, _ = m.ti.Update(key)
		}
		return nil
	}
	switch {
	case key.Type == tea.KeyUp:
		m.moveCursor(-1)
	case key.Type == tea.KeyDown:
		m.moveCursor(1)
	case key.Type == tea.KeyEnter:
		m.enterRow()
	case key.Type == tea.KeyEsc:
		m.backOut()
	case key.Type == tea.KeyLeft:
		m.cycleSelectedEnum(-1)
	case key.Type == tea.KeyRight:
		m.cycleSelectedEnum(1)
	case key.Type == tea.KeyRunes && len(key.Runes) == 1:
		switch key.Runes[0] {
		case 'a':
			m.addRow()
		case 'd':
			m.removeRow()
		case 't':
			m.toggleOptional()
		case 'k':
			m.editSelectedKey()
		case 'r':
			return m.runForm()
		case 'q':
			return tea.Quit
		}
	}
	return nil
}

func (m *Model) moveCursor(d int) {
	if m.view == nil || len(m.view.rows) == 0 {
		return
	}
	m.view.idx += d
	if m.view.idx < 0 {
		m.view.idx = 0
	}
	if m.view.idx >= len(m.view.rows) {
		m.view.idx = len(m.view.rows) - 1
	}
}

// currentRow returns the selected row, if any.
func (m *Model) currentRow() *formRow {
	if m.view == nil || len(m.view.rows) == 0 {
		return nil
	}
	m.view.idx = min(max(m.view.idx, 0), len(m.view.rows)-1)
	return &m.view.rows[m.view.idx]
}

func (m *Model) backOut() {
	if m.editKind != editNone {
		m.endEdit()
		return
	}
	if m.view != nil && m.view.parent != nil {
		m.view = m.view.parent
	}
}

func (m *Model) enterRow() {
	r := m.currentRow()
	if r == nil {
		return
	}
	if r.choice != nil {
		// choosing a satisfying class for the selected interface
		f := r.field
		for i, c := range f.choices {
			if c == r.choice {
				f.chooseClass(i)
				break
			}
		}
		if m.view.parent != nil {
			m.view = m.view.parent
		}
		return
	}
	if r.entry != nil {
		// enter edits the entry value (a compound drills deeper)
		m.enterField(r.entry.value)
		return
	}
	m.enterField(r.field)
}

func (m *Model) enterField(f *FormField) {
	if f == nil {
		return
	}
	switch f.typ.(type) {
	case types.Primitive:
		m.beginTextEdit(f)
	case *types.Enum:
		// members are cycled with left/right
	case *types.Class, *types.Array, *types.Map:
		m.descend(f)
	case *types.Interface:
		if f.chosen < 0 {
			m.openChooser(f)
		} else {
			m.descend(f)
		}
	case *types.Optional:
		if f.present && f.child != nil {
			m.enterField(f.child)
		}
	}
}

// descend opens the child rows of a compound field.
func (m *Model) descend(f *FormField) {
	title := f.label
	if title == "" {
		title = f.typ.String()
	}
	m.view = &formView{
		title:     title,
		rows:      rowsOf(f),
		container: f,
		parent:    m.view,
	}
}

func (m *Model) openChooser(f *FormField) {
	title := f.label
	if title == "" {
		title = f.typ.String()
	}
	rows := make([]formRow, 0, len(f.choices))
	for _, c := range f.choices {
		rows = append(rows, formRow{field: f, choice: c})
	}
	m.view = &formView{title: title, rows: rows, parent: m.view}
}

func (m *Model) cycleSelectedEnum(d int) {
	r := m.currentRow()
	if r == nil || r.field == nil {
		return
	}
	f := r.field
	if _, ok := f.typ.(*types.Enum); !ok {
		return
	}
	if len(f.members) == 0 {
		return
	}
	start := 0
	if i := slices.Index(f.members, f.text); i >= 0 {
		start = i
	}
	if f.text == "" && d < 0 {
		start = len(f.members) - 1
	}
	n := (start + d) % len(f.members)
	if n < 0 {
		n += len(f.members)
	}
	f.text = f.members[n]
}

func (m *Model) addRow() {
	r := m.currentRow()
	if r == nil {
		return
	}
	var target *FormField
	inside := false
	if r.entry == nil && r.field != nil {
		switch r.field.typ.(type) {
		case *types.Array, *types.Map:
			target = r.field
			inside = m.view != nil && m.view.container == target
		}
	}
	if target == nil && m.view != nil && m.view.container != nil {
		switch m.view.container.typ.(type) {
		case *types.Array, *types.Map:
			target = m.view.container
			inside = true
		}
	}
	if target == nil {
		return
	}
	switch tt := target.typ.(type) {
	case *types.Array:
		target.elems = append(target.elems, buildField(tt.Elem, target.env))
	case *types.Map:
		val := buildField(tt.Value, target.env)
		target.entries = append(target.entries, mapEntry{value: val})
	}
	if !inside {
		m.descend(target)
	}
	m.refreshView()
	if len(m.view.rows) > 0 {
		m.view.idx = len(m.view.rows) - 1
	}
}

func (m *Model) removeRow() {
	if m.view == nil || m.view.container == nil {
		return
	}
	i := m.view.idx
	switch m.view.container.typ.(type) {
	case *types.Array:
		if i < 0 || i >= len(m.view.container.elems) {
			return
		}
		m.view.container.elems = slices.Delete(m.view.container.elems, i, i+1)
	case *types.Map:
		if i < 0 || i >= len(m.view.container.entries) {
			return
		}
		m.view.container.entries = slices.Delete(m.view.container.entries, i, i+1)
	default:
		return
	}
	m.refreshView()
}

func (m *Model) toggleOptional() {
	r := m.currentRow()
	if r == nil || r.field == nil {
		return
	}
	f := r.field
	if opt, ok := f.typ.(*types.Optional); ok {
		f.present = !f.present
		if !f.present {
			f.child = buildField(opt.Elem, f.env)
		}
	}
}

func (m *Model) editSelectedKey() {
	r := m.currentRow()
	if r == nil || r.entry == nil {
		return
	}
	m.beginKeyEdit(r.entry)
}

func (m *Model) beginTextEdit(f *FormField) {
	m.editKind = editText
	m.editField = f
	m.editEntry = nil
	m.ti.SetValue(f.text)
	m.ti.CursorEnd()
	m.ti.Focus()
}

func (m *Model) beginKeyEdit(e *mapEntry) {
	m.editKind = editKey
	m.editField = nil
	m.editEntry = e
	m.ti.SetValue(e.key)
	m.ti.CursorEnd()
	m.ti.Focus()
}

func (m *Model) commitEdit() {
	switch m.editKind {
	case editText:
		if m.editField != nil {
			m.editField.text = m.ti.Value()
		}
	case editKey:
		if m.editEntry != nil {
			m.editEntry.key = m.ti.Value()
		}
	}
	m.endEdit()
}

func (m *Model) endEdit() {
	m.editKind = editNone
	m.editField = nil
	m.editEntry = nil
	m.ti.Blur()
}

func (m *Model) runForm() tea.Cmd {
	errs := m.form.Validate()
	if len(errs) > 0 {
		m.formErr = formatFieldErrors(errs)
		return nil
	}
	m.formErr = ""
	m.stage = stageOutput
	m.outSel = 0
	m.outKind = outStdout
	m.outMode = outputPick
	m.resultErr = nil
	m.resultOK = false
	return nil
}

// refreshView rebuilds the current view's rows after structural edits.
func (m *Model) refreshView() {
	if m.view == nil {
		return
	}
	if m.view.container != nil {
		m.view.rows = rowsOf(m.view.container)
	} else if m.form != nil {
		m.view.rows = rootRows(m.form)
	}
	if m.view.idx >= len(m.view.rows) {
		m.view.idx = len(m.view.rows) - 1
	}
	if m.view.idx < 0 {
		m.view.idx = 0
	}
}

// --- output ---

func (m *Model) updateOutput(key tea.KeyMsg) tea.Cmd {
	switch m.outMode {
	case outputPath:
		switch key.Type {
		case tea.KeyEnter:
			p := strings.TrimSpace(m.fileInput.Value())
			if p == "" {
				return nil
			}
			m.filePathVal = p
			m.finishFile()
		case tea.KeyEsc:
			m.outMode = outputPick
			m.fileInput.Blur()
		default:
			m.fileInput, _ = m.fileInput.Update(key)
		}
		return nil
	case outputConfirm:
		if key.Type == tea.KeyEsc {
			m.outMode = outputPick
			return nil
		}
		if key.Type == tea.KeyRunes && len(key.Runes) == 1 {
			switch key.Runes[0] {
			case 'y':
				m.outMode = outputPick
				m.fileInput.Blur()
				m.deliverCurrent()
			}
		}
		return nil
	default:
		switch {
		case key.Type == tea.KeyUp:
			if m.outSel > 0 {
				m.outSel--
			}
		case key.Type == tea.KeyDown:
			if m.outSel < 2 {
				m.outSel++
			}
		case key.Type == tea.KeyEnter:
			m.chooseTarget(m.outSel)
		case key.Type == tea.KeyRunes && len(key.Runes) == 1:
			if key.Runes[0] == 'q' {
				return tea.Quit
			}
		}
		return nil
	}
}

func (m *Model) chooseTarget(i int) {
	switch i {
	case 0:
		m.outKind = outStdout
		m.deliverCurrent()
	case 1:
		m.outKind = outFile
		if m.filePathVal != "" {
			m.finishFile()
		} else {
			m.outMode = outputPath
			m.fileInput.SetValue("")
			m.fileInput.Focus()
		}
	case 2:
		m.outKind = outClipboard
		m.deliverCurrent()
	}
}

func (m *Model) finishFile() {
	if _, err := os.Stat(m.filePathVal); err == nil {
		m.outMode = outputConfirm
		return
	}
	m.outMode = outputPick
	m.fileInput.Blur()
	m.deliverCurrent()
}

func (m *Model) deliverCurrent() {
	inputs, err := m.form.ToInputValues()
	if err != nil {
		m.setResultErr(err)
		return
	}
	out, rerr := renderTemplate(m.prog, m.templateName, inputs)
	if rerr != nil {
		m.setResultErr(rerr)
		return
	}
	var sink domain.OutputSink
	var target string
	switch m.outKind {
	case outStdout:
		sink = output.StdoutSink{}
		target = "stdout"
	case outFile:
		sink = output.NewFileSink(m.filePathVal)
		target = m.filePathVal
	case outClipboard:
		sink = output.NewClipboardSink(output.RealClipboard{})
		target = "clipboard"
	}
	if sink == nil {
		m.setResultErr(fmt.Errorf("no output target chosen"))
		return
	}
	if err := sink.Write(out); err != nil {
		m.setResultErr(err)
		return
	}
	m.resultOK = true
	m.resultErr = nil
	m.rendered = out
	m.targetDesc = target
	m.stage = stageResult
}

func (m *Model) setResultErr(err error) {
	m.resultOK = false
	m.resultErr = err
	m.rendered = ""
	m.stage = stageResult
}

// --- result ---

func (m *Model) updateResult(key tea.KeyMsg) tea.Cmd {
	if key.Type == tea.KeyEsc {
		// return to the form to edit variables
		m.stage = stageForm
		m.resultErr = nil
		return nil
	}
	if key.Type == tea.KeyEnter {
		// retry the same target
		if !m.resultOK && m.outKind != 0 {
			m.deliverCurrent()
		}
		return nil
	}
	if key.Type == tea.KeyRunes && len(key.Runes) == 1 {
		switch key.Runes[0] {
		case 'e':
			m.stage = stageForm
			m.resultErr = nil
			return nil
		case 'r':
			if !m.resultOK {
				m.deliverCurrent()
			}
			return nil
		case 'q':
			return tea.Quit
		}
	}
	return nil
}

// --- helpers ---

func rootRows(form *InputForm) []formRow {
	rows := make([]formRow, 0, len(form.Fields))
	for _, f := range form.Fields {
		rows = append(rows, formRow{field: f})
	}
	return rows
}

func rootView(form *InputForm, name string) *formView {
	return &formView{title: name, rows: rootRows(form)}
}

// rowsOf lists the child rows a compound field currently owns.
func rowsOf(f *FormField) []formRow {
	var rows []formRow
	switch f.typ.(type) {
	case *types.Class:
		for _, sub := range f.fields {
			rows = append(rows, formRow{field: sub})
		}
	case *types.Interface:
		if f.chosen >= 0 {
			for _, sub := range f.fields {
				rows = append(rows, formRow{field: sub})
			}
		}
	case *types.Array:
		for _, el := range f.elems {
			rows = append(rows, formRow{field: el})
		}
	case *types.Map:
		for i := range f.entries {
			rows = append(rows, formRow{field: f.entries[i].value, entry: &f.entries[i]})
		}
	case *types.Optional:
		if f.present && f.child != nil {
			rows = append(rows, formRow{field: f.child})
		}
	}
	return rows
}

func formatDiags(diags []token.Diagnostic) string {
	var sb strings.Builder
	for _, d := range diags {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s (%s): %s", d.Category, d.Stage, d.Message)
	}
	return sb.String()
}

func formatFieldErrors(errs []fieldError) string {
	var sb strings.Builder
	for _, e := range errs {
		if sb.Len() > 0 {
			sb.WriteString("\n")
		}
		fmt.Fprintf(&sb, "%s: %s", e.path, e.msg)
	}
	return sb.String()
}
