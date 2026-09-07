package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/adapters/source"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
	"github.com/Jh123x/prompiler/internal/token"
)

// modelSource drives the full interactive flow: browse -> select -> form
// (drill into a class, edit scalars, add/remove array and map entries) ->
// choose a file target -> render and write.
const modelSource = `
class Person {
  name: string
  age: int
}

template Demo {
  variables {
    name: string
    count: int = 5
    person: Person
    scores: int[]
    tags: map<string, string>
  }
  prompt {{{ name }}|{{ person.name }}|{{ person.age }}|{{ count }}.}
}
`

const divZeroSource = `
template DivisionByZero {
  variables {
    n: int
    d: int
  }
  prompt {{{ n / d }}}
}
`

const enumSource = `
enum Difficulty { Easy, Medium, Hard }

template EnumTest {
  variables {
    level: Difficulty
  }
  prompt {
    Level: {{ level }}
  }
}
`

const optionalSource = `
template OptTest {
  variables {
    nickname: Optional<string>
  }
  prompt {
    {{ nickname.value() }}
  }
}
`

// --- key helpers ---

func keyRune(r rune) tea.Msg   { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }
func keyText(s string) tea.Msg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

var (
	keyEnterMsg = tea.KeyMsg{Type: tea.KeyEnter}
	keyEscMsg   = tea.KeyMsg{Type: tea.KeyEsc}
	keyUpMsg    = tea.KeyMsg{Type: tea.KeyUp}
	keyDownMsg  = tea.KeyMsg{Type: tea.KeyDown}
	keyLeftMsg  = tea.KeyMsg{Type: tea.KeyLeft}
	keyRightMsg = tea.KeyMsg{Type: tea.KeyRight}
)

func tap(m *Model, msg tea.Msg) { m.Update(msg) }

func modelForSource(t *testing.T, files map[string]string) *Model {
	t.Helper()
	app := domain.NewApplication(&source.MemSource{Files: files}, builtin.NewRegistry())
	m := newModel(app, ".")
	require.Empty(t, m.browseErr, "browse must load templates")
	require.NotEmpty(t, m.templates, "expected at least one template")
	return m
}

func TestModelFlowEndToEnd(t *testing.T) {
	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})

	// --- browse: list and select the Demo template ---
	require.Equal(t, stageBrowse, m.stage)
	require.Equal(t, []domain.TemplateInfo{{Path: "demo.ppl", Name: "Demo", Description: ""}}, m.templates)
	assert.Contains(t, m.View(), "demo.ppl")

	tap(m, keyEnterMsg) // open Demo
	require.Equal(t, stageForm, m.stage)
	require.NotNil(t, m.form)
	assert.NotEmpty(t, m.View())

	root := func() *formView { return m.view }
	require.Len(t, root().rows, 5)

	// --- edit a scalar at the root (name) ---
	tap(m, keyEnterMsg) // row 0: name -> text editor
	require.Equal(t, editText, m.editKind)
	tap(m, keyText("Alice"))
	tap(m, keyEnterMsg) // commit
	require.Equal(t, editNone, m.editKind)
	require.Equal(t, "Alice", mustField(t, m.form, "name").text)

	// --- drill into the Person class and edit nested scalars ---
	tap(m, keyDownMsg) // idx1 count
	tap(m, keyDownMsg) // idx2 person
	tap(m, keyEnterMsg)
	require.NotNil(t, root().container)
	require.Equal(t, "person", root().container.label)
	require.Len(t, root().rows, 2)

	person := mustField(t, m.form, "person")
	tap(m, keyEnterMsg) // person.name
	tap(m, keyText("Bob"))
	tap(m, keyEnterMsg)
	tap(m, keyDownMsg) // person.age
	tap(m, keyEnterMsg)
	tap(m, keyText("30"))
	tap(m, keyEnterMsg)
	require.Equal(t, "Bob", person.fields[0].text)
	require.Equal(t, "30", person.fields[1].text)

	tap(m, keyEscMsg) // back to the root view
	require.Equal(t, m.view.parent, (*formView)(nil))
	require.Equal(t, 2, m.view.idx)

	// --- array: add (from the array's own row) and remove an element ---
	tap(m, keyDownMsg)   // idx3 scores
	tap(m, keyRune('a')) // adding descends into the array
	require.Equal(t, "scores", m.view.container.label)
	scores := mustField(t, m.form, "scores")
	require.Len(t, scores.elems, 1)
	require.Len(t, m.view.rows, 1)
	tap(m, keyRune('d')) // remove the element
	require.Empty(t, scores.elems)
	require.Empty(t, m.view.rows)
	tap(m, keyEscMsg) // back to the root list
	require.Equal(t, 3, m.view.idx)

	// --- map: add an entry, set its key and value, then remove it ---
	tap(m, keyDownMsg)   // idx4 tags
	tap(m, keyRune('a')) // adding descends into the map
	require.Equal(t, "tags", m.view.container.label)
	tags := mustField(t, m.form, "tags")
	require.Len(t, tags.entries, 1)
	require.Len(t, m.view.rows, 1)

	tap(m, keyRune('k')) // edit the entry key
	require.Equal(t, editKey, m.editKind)
	tap(m, keyText("lang"))
	tap(m, keyEnterMsg)
	require.Equal(t, "lang", tags.entries[0].key)

	tap(m, keyEnterMsg) // drill into the entry value
	require.Equal(t, editText, m.editKind)
	tap(m, keyText("Go"))
	tap(m, keyEnterMsg)
	require.Equal(t, "Go", tags.entries[0].value.text)

	tap(m, keyRune('d')) // remove the entry again
	require.Empty(t, tags.entries)
	tap(m, keyEscMsg) // back to the root list
	require.Equal(t, stageForm, m.stage)

	// --- run: validate and advance to the output stage ---
	tap(m, keyRune('r'))
	require.Equal(t, stageOutput, m.stage)
	require.Empty(t, m.formErr)
	assert.NotEmpty(t, m.View())

	// --- choose the file target and type a destination path ---
	tap(m, keyDownMsg) // outSel 0 (stdout) -> 1 (file)
	require.Equal(t, 1, m.outSel)
	tap(m, keyEnterMsg)
	require.Equal(t, outFile, m.outKind)
	require.Equal(t, outputPath, m.outMode)

	path := filepath.Join(t.TempDir(), "out.txt")
	tap(m, keyText(path))
	tap(m, keyEnterMsg)

	require.Equal(t, stageResult, m.stage)
	require.True(t, m.resultOK)
	require.NoError(t, m.resultErr)
	require.Equal(t, "Alice|Bob|30|5.\n", m.rendered)
	assert.NotEmpty(t, m.View())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, m.rendered, string(data))
}

func TestModelAddToEmptyCollection(t *testing.T) {
	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})
	tap(m, keyEnterMsg) // open Demo
	require.Equal(t, stageForm, m.stage)

	// Descend into the empty scores array via Enter, then add inside it.
	tap(m, keyDownMsg) // 1 count
	tap(m, keyDownMsg) // 2 person
	tap(m, keyDownMsg) // 3 scores
	tap(m, keyEnterMsg)
	require.Equal(t, "scores", m.view.container.label)
	require.Empty(t, m.view.rows)

	tap(m, keyRune('a')) // regression: adding inside an empty array used to be a no-op
	require.Len(t, m.view.rows, 1)
	require.Len(t, mustField(t, m.form, "scores").elems, 1)

	// Same for the empty map.
	tap(m, keyEscMsg)  // back to the root list
	tap(m, keyDownMsg) // idx4 tags
	tap(m, keyEnterMsg)
	require.Equal(t, "tags", m.view.container.label)
	require.Empty(t, m.view.rows)

	tap(m, keyRune('a'))
	require.Len(t, m.view.rows, 1)
	require.Len(t, mustField(t, m.form, "tags").entries, 1)
}

func TestModelEnumChooser(t *testing.T) {
	m := modelForSource(t, map[string]string{"enum.ppl": enumSource})
	tap(m, keyEnterMsg) // open EnumTest
	require.Equal(t, stageForm, m.stage)

	// Entering the enum opens a chooser listing its members.
	tap(m, keyEnterMsg)
	require.Len(t, m.view.rows, 3)
	require.Equal(t, "Easy", m.view.rows[0].option)
	require.Equal(t, "Medium", m.view.rows[1].option)
	require.Equal(t, "Hard", m.view.rows[2].option)

	// Select "Hard" with the arrow keys.
	tap(m, keyDownMsg)
	tap(m, keyDownMsg)
	tap(m, keyEnterMsg)
	require.Equal(t, "Hard", mustField(t, m.form, "level").text)
}

func TestModelOptionalEdit(t *testing.T) {
	m := modelForSource(t, map[string]string{"opt.ppl": optionalSource})
	tap(m, keyEnterMsg) // open OptTest
	require.Equal(t, stageForm, m.stage)

	nick := mustField(t, m.form, "nickname")
	require.False(t, nick.present) // starts as "none"

	// Enter switches to "some" and begins editing the scalar value.
	tap(m, keyEnterMsg)
	require.True(t, nick.present)
	require.Equal(t, editText, m.editKind)
	require.Equal(t, nick.child, m.editField)

	tap(m, keyText("Ada"))
	tap(m, keyEnterMsg)
	require.Equal(t, "Ada", nick.child.text)

	// Enter again to edit, then the down arrow selects "none".
	tap(m, keyEnterMsg)
	require.Equal(t, editText, m.editKind)
	tap(m, keyDownMsg)
	require.False(t, nick.present)
	require.Equal(t, editNone, m.editKind)
}

func TestModelMapEntryKeyThenValue(t *testing.T) {
	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})
	tap(m, keyEnterMsg) // open Demo
	require.Equal(t, stageForm, m.stage)

	// Navigate to tags (row 4) and add an entry.
	tap(m, keyDownMsg) // 1 count
	tap(m, keyDownMsg) // 2 person
	tap(m, keyDownMsg) // 3 scores
	tap(m, keyDownMsg) // 4 tags
	tap(m, keyRune('a'))
	tags := mustField(t, m.form, "tags")
	require.Len(t, tags.entries, 1)

	// Entering a new entry prompts for the key, then chains into the value.
	tap(m, keyEnterMsg)
	require.Equal(t, editKey, m.editKind)
	tap(m, keyText("lang"))
	tap(m, keyEnterMsg) // commit key -> prompt value
	require.Equal(t, "lang", tags.entries[0].key)
	require.Equal(t, editText, m.editKind)

	tap(m, keyText("Go"))
	tap(m, keyEnterMsg)
	require.Equal(t, "Go", tags.entries[0].value.text)
}

func TestModelBackFromForm(t *testing.T) {
	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})
	tap(m, keyEnterMsg) // open Demo
	require.Equal(t, stageForm, m.stage)

	// Drill into the Person class; left arrow backs up within the form.
	tap(m, keyDownMsg) // count
	tap(m, keyDownMsg) // person
	tap(m, keyEnterMsg)
	require.NotNil(t, m.view.container)

	tap(m, keyLeftMsg)
	require.Equal(t, stageForm, m.stage)
	require.Nil(t, m.view.container) // back at the root

	// Left arrow at the form root returns to the browse stage.
	tap(m, keyLeftMsg)
	require.Equal(t, stageBrowse, m.stage)
}

func TestModelScalarValidation(t *testing.T) {
	m := modelForSource(t, map[string]string{"dbz.ppl": divZeroSource})
	tap(m, keyEnterMsg) // open DivisionByZero
	require.Equal(t, stageForm, m.stage)

	// n (int): typing an invalid value is rejected with an error.
	tap(m, keyEnterMsg) // edit n
	tap(m, keyText("abc"))
	tap(m, keyEnterMsg)
	require.Equal(t, editText, m.editKind) // still editing
	require.NotEmpty(t, m.editErr)

	// Cancel reverts the field to its previous (empty) value.
	tap(m, keyEscMsg)
	require.Equal(t, editNone, m.editKind)
	require.Empty(t, mustField(t, m.form, "n").text)
}

func TestModelStdoutDefersOutput(t *testing.T) {
	m := modelForSource(t, map[string]string{
		"s.ppl": "template Simple {\n  variables { name: string }\n  prompt { Hello {{ name }} }\n}\n",
	})
	tap(m, keyEnterMsg) // open Simple
	require.Equal(t, stageForm, m.stage)

	tap(m, keyEnterMsg) // edit name
	tap(m, keyText("Ada"))
	tap(m, keyEnterMsg)
	tap(m, keyRune('r')) // run -> output
	tap(m, keyEnterMsg)  // outSel 0 -> stdout

	require.Equal(t, stageResult, m.stage)
	require.True(t, m.resultOK)
	require.NotEmpty(t, m.rendered)
	require.Equal(t, m.rendered, m.stdoutOutput)
}

func TestModelDivisionByZeroReturnsToForm(t *testing.T) {
	m := modelForSource(t, map[string]string{"dbz.ppl": divZeroSource})

	tap(m, keyEnterMsg) // select DivisionByZero
	require.Equal(t, stageForm, m.stage)

	// fill the required ints
	tap(m, keyEnterMsg) // n
	tap(m, keyText("10"))
	tap(m, keyEnterMsg)
	tap(m, keyDownMsg) // d
	tap(m, keyEnterMsg)
	tap(m, keyText("0"))
	tap(m, keyEnterMsg)
	require.Equal(t, "10", mustField(t, m.form, "n").text)
	require.Equal(t, "0", mustField(t, m.form, "d").text)

	// run to the output stage and pick stdout
	tap(m, keyRune('r'))
	require.Equal(t, stageOutput, m.stage)
	tap(m, keyEnterMsg) // outSel 0 -> stdout -> render

	require.Equal(t, stageResult, m.stage)
	require.False(t, m.resultOK)
	require.Error(t, m.resultErr)

	var rerr *eval.RuntimeError
	require.ErrorAs(t, m.resultErr, &rerr)
	require.NotNil(t, rerr)
	assert.Equal(t, token.CatDivisionByZero, rerr.Category)
	assert.Contains(t, m.View(), "division by zero")

	// 'e' returns to the form with the entered values intact
	tap(m, keyRune('e'))
	require.Equal(t, stageForm, m.stage)
	require.Nil(t, m.resultErr)
	require.NotNil(t, m.form)
	require.Equal(t, "10", mustField(t, m.form, "n").text)
	require.Equal(t, "0", mustField(t, m.form, "d").text)
	assert.NotEmpty(t, m.View())
}

func TestModelEscBacksOutOfNestedViews(t *testing.T) {
	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})
	tap(m, keyEnterMsg) // open Demo
	require.Equal(t, stageForm, m.stage)

	// descend to the Person class then back out
	tap(m, keyDownMsg) // count
	tap(m, keyDownMsg) // person
	tap(m, keyEnterMsg)
	require.NotNil(t, m.view.container)

	// esc while editing aborts the edit without losing prior text
	tap(m, keyEnterMsg) // edit person.name
	tap(m, keyText("X"))
	tap(m, keyEscMsg) // cancel the edit
	require.Equal(t, editNone, m.editKind)
	person := mustField(t, m.form, "person")
	require.Equal(t, "", person.fields[0].text) // discarded

	tap(m, keyEscMsg) // back to root
	require.Nil(t, m.view.parent)
	require.Equal(t, stageForm, m.stage)
}

func TestModelQuit(t *testing.T) {
	// The produced command must be a real tea.Cmd that yields a QuitMsg.
	isQuit := func(t *testing.T, cmd tea.Cmd) {
		t.Helper()
		require.NotNil(t, cmd)
		msg := cmd()
		require.IsType(t, tea.QuitMsg{}, msg)
	}

	m := modelForSource(t, map[string]string{"demo.ppl": modelSource})

	// ctrl+c quits from any stage.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	isQuit(t, cmd)

	// 'q' quits from browse.
	m = modelForSource(t, map[string]string{"demo.ppl": modelSource})
	_, cmd = m.Update(keyRune('q'))
	isQuit(t, cmd)
}

func TestModelBrowseInvalidSourceStaysInBrowse(t *testing.T) {
	// An unclosed template body is a parse error: discovery lists the file as a
	// syntax-error entry (not skipped), and the model stays on the browse stage.
	src := `
template Broken {
  prompt {hello
`
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{"b.ppl": src}}, builtin.NewRegistry())
	m := newModel(app, ".")
	require.Len(t, m.templates, 1)
	require.NotEmpty(t, m.templates[0].Diagnostics)
	require.Empty(t, m.browseErr)
	require.Equal(t, stageBrowse, m.stage)
	assert.NotEmpty(t, m.View())
}

func TestModelBrowseSyntaxErrorDetail(t *testing.T) {
	// Selecting a syntax-error entry shows the full error; esc returns to the list.
	src := `
template Broken {
  prompt {hello
`
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{"b.ppl": src}}, builtin.NewRegistry())
	m := newModel(app, ".")

	tap(m, keyEnterMsg)
	require.NotEmpty(t, m.browseDetail)
	require.Equal(t, stageBrowse, m.stage)

	tap(m, keyEscMsg)
	require.Empty(t, m.browseDetail)
	require.Equal(t, stageBrowse, m.stage)
}

func TestModelBrowseSelectsPerDirectory(t *testing.T) {
	// Two independent directories each declare a class "P"; whole-root analysis
	// would reject the duplicate, but the TUI discovers both templates and
	// analyzes only the selected template's own directory.
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "a"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "b"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a", "one.ppl"), []byte("class P { x: string }\ntemplate One { variables { x: string } prompt { {{ x }} } }\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b", "two.ppl"), []byte("class P { y: string }\ntemplate Two { variables { y: string } prompt { {{ y }} } }\n"), 0o644))

	app := domain.NewApplication(&source.FSSource{}, builtin.NewRegistry())
	m := newModel(app, dir)

	require.Len(t, m.templates, 2)
	require.Empty(t, m.browseErr)

	tap(m, keyEnterMsg) // open the first template (in a/)
	require.Equal(t, stageForm, m.stage)
	require.Equal(t, "One", m.templateName)
}
