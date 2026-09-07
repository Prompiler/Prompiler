package domain_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Jh123x/prompiler/internal/adapters/source"
	"github.com/Jh123x/prompiler/internal/builtin"
	"github.com/Jh123x/prompiler/internal/domain"
)

func TestListTemplatesOrdersByPath(t *testing.T) {
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{
		"b/zed.ppl": `template Zed { variables { x: string } prompt { {{ x }} } }`,
		"a/two.ppl": `template Two { variables { x: string } prompt { {{ x }} } }`,
		"a/one.ppl": `template One { variables { x: string } prompt { {{ x }} } }`,
	}}, builtin.NewRegistry())

	got, err := app.ListTemplates(".")
	require.NoError(t, err)
	require.Equal(t, []domain.TemplateInfo{
		{Path: "a/one.ppl", Name: "One"},
		{Path: "a/two.ppl", Name: "Two"},
		{Path: "b/zed.ppl", Name: "Zed"},
	}, got)
}

func TestDiscoverTemplatesToleratesCrossFileDuplicates(t *testing.T) {
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{
		"a/one.ppl": "class P { x: string }\ntemplate One { variables { x: string } prompt { {{ x }} } }\n",
		"b/two.ppl": "class P { y: string }\ntemplate Two { variables { y: string } prompt { {{ y }} } }\n",
	}}, builtin.NewRegistry())

	// Whole-root analysis rejects the duplicate class P.
	if _, err := app.ListTemplates("."); err == nil {
		t.Fatal("expected ListTemplates to fail on the cross-file duplicate")
	}

	// DiscoverTemplates is tolerant and lists both templates with their paths.
	got, err := app.DiscoverTemplates(".")
	require.NoError(t, err)
	require.Equal(t, []domain.TemplateInfo{
		{Path: "a/one.ppl", Name: "One"},
		{Path: "b/two.ppl", Name: "Two"},
	}, got)
}

func TestDiscoverTemplatesMarksSyntaxErrors(t *testing.T) {
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{
		"broken.ppl": "template Broken {\n  prompt { hello\n",
	}}, builtin.NewRegistry())

	got, err := app.DiscoverTemplates(".")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "broken.ppl", got[0].Path)
	require.NotEmpty(t, got[0].Diagnostics)
}

func TestDiscoverTemplatesMarksTypeErrors(t *testing.T) {
	app := domain.NewApplication(&source.MemSource{Files: map[string]string{
		"err/broken.ppl": "enum Color { Red, Green, Red }\ntemplate Broken { variables { x: int } prompt { {{ x }} } }\n",
	}}, builtin.NewRegistry())

	got, err := app.DiscoverTemplates(".")
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "err/broken.ppl", got[0].Path)
	require.Equal(t, "Broken", got[0].Name)
	require.NotEmpty(t, got[0].Diagnostics)
}
