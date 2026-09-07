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
