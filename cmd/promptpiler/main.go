// Command promptpiler is the Prompiler CLI: list, check, and run templates.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/Jh123x/prompiler/internal/adapters/jsonvalue"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	app, err := initializeApplication()
	if err != nil {
		fmt.Fprintln(os.Stderr, "init:", err)
		os.Exit(1)
	}
	root := "."
	switch os.Args[1] {
	case "list":
		templates, err := app.ListTemplates(root)
		if err != nil {
			fatal(err)
		}
		for _, t := range templates {
			fmt.Println(t.Name)
		}
	case "check":
		diags, err := app.CheckTemplates(root)
		if err != nil {
			fatal(err)
		}
		for _, d := range diags {
			fmt.Fprintf(os.Stderr, "%s: %s: %s\n", d.Stage, d.Category, d.Message)
		}
		if len(diags) > 0 {
			os.Exit(1)
		}
	case "run":
		if len(os.Args) < 3 {
			usage()
			os.Exit(2)
		}
		if err := run(app, root, os.Args[2]); err != nil {
			fatal(err)
		}
	default:
		usage()
		os.Exit(2)
	}
}

func run(app *domain.Application, root, name string) error {
	prog, diags := app.AnalyzeRoot(root)
	if len(diags) > 0 {
		return fmt.Errorf("%s: %s: %s", diags[0].Stage, diags[0].Category, diags[0].Message)
	}
	varData, err := os.ReadFile(filepath.Join(root, "variables.json"))
	if err != nil {
		return fmt.Errorf("read variables.json: %w", err)
	}
	vs, err := jsonvalue.New(varData, prog.Sem.Env)
	if err != nil {
		return err
	}
	var vars []domain.Variable
	for n, t := range prog.Sem.TemplateVars[name] {
		vars = append(vars, domain.Variable{Name: n, Type: t})
	}
	sort.Slice(vars, func(i, j int) bool { return vars[i].Name < vars[j].Name })
	inputs, err := vs.Provide(vars)
	if err != nil {
		return err
	}
	out, rerr := eval.NewComposer(prog.Files, prog.Sem).Render(name, inputs)
	if rerr != nil {
		return fmt.Errorf("%s: %s", rerr.Category, rerr.Message)
	}
	fmt.Print(out)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: promptpiler <list|check|run <template>>")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
