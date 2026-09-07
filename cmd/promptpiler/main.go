// Command promptpiler is the Prompiler CLI: list, check, and run templates.
// With no arguments it starts the interactive TUI.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/Jh123x/prompiler/internal/adapters/jsonvalue"
	"github.com/Jh123x/prompiler/internal/domain"
	"github.com/Jh123x/prompiler/internal/eval"
)

func main() {
	os.Exit(dispatch(os.Args[1:], os.Stdout, os.Stderr, startTUI))
}

// dispatch runs the CLI, writing to the provided streams so it is testable. It
// returns the process exit code.
func dispatch(args []string, stdout, stderr io.Writer, startTUI func(root string) error) int {
	if len(args) == 0 {
		if err := startTUI("."); err != nil {
			fmt.Fprintf(stderr, "tui: %v\n", err)
			return 1
		}
		return 0
	}

	switch args[0] {
	case "list":
		fs := newFlagSet("list", stderr)
		root := fs.String("root", ".", "root directory to scan")
		if code := parse(fs, args[1:], stderr); code != 0 {
			return code
		}
		app, err := initializeApplication()
		if err != nil {
			fmt.Fprintf(stderr, "init: %v\n", err)
			return 1
		}
		templates, err := app.ListTemplates(*root)
		if err != nil {
			return fail(stderr, err)
		}
		for _, t := range templates {
			fmt.Fprintln(stdout, t.Name)
		}
		return 0

	case "check":
		fs := newFlagSet("check", stderr)
		root := fs.String("root", ".", "root directory to scan")
		if code := parse(fs, args[1:], stderr); code != 0 {
			return code
		}
		app, err := initializeApplication()
		if err != nil {
			fmt.Fprintf(stderr, "init: %v\n", err)
			return 1
		}
		diags, err := app.CheckTemplates(*root)
		if err != nil {
			return fail(stderr, err)
		}
		for _, d := range diags {
			fmt.Fprintf(stderr, "%s: %s: %s\n", d.Stage, d.Category, d.Message)
		}
		if len(diags) > 0 {
			return 1
		}
		return 0

	case "run":
		fs := newFlagSet("run", stderr)
		root := fs.String("root", ".", "root directory to scan")
		variables := fs.String("variables", "", "path to a JSON file of input values (defaults to <root>/variables.json)")
		if code := parse(fs, args[1:], stderr); code != 0 {
			return code
		}
		name := fs.Arg(0)
		if name == "" {
			usage(stderr)
			return 2
		}
		app, err := initializeApplication()
		if err != nil {
			fmt.Fprintf(stderr, "init: %v\n", err)
			return 1
		}
		if err := run(stdout, app, *root, name, *variables); err != nil {
			return fail(stderr, err)
		}
		return 0

	default:
		usage(stderr)
		return 2
	}
}

// newFlagSet returns a per-subcommand flag set that reports usage to stderr.
func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	return fs
}

// parse consumes the subcommand flags, returning a non-zero exit code on error.
func parse(fs *flag.FlagSet, args []string, stderr io.Writer) int {
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return 0
}

// startTUI wires and launches the interactive TUI rooted at root.
func startTUI(root string) error {
	app, err := initializeTUI()
	if err != nil {
		return err
	}
	return app.Run(root)
}

func run(stdout io.Writer, app *domain.Application, root, name, variables string) error {
	prog, diags := app.AnalyzeRoot(root)
	if len(diags) > 0 {
		return fmt.Errorf("%s: %s: %s", diags[0].Stage, diags[0].Category, diags[0].Message)
	}
	varsPath := variables
	if varsPath == "" {
		varsPath = filepath.Join(root, "variables.json")
	}
	varData, err := os.ReadFile(varsPath)
	if err != nil {
		return fmt.Errorf("read variables from %q: %w", varsPath, err)
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
	fmt.Fprint(stdout, out)
	return nil
}

func usage(stderr io.Writer) {
	fmt.Fprintln(stderr, "usage: promptpiler [<list|check|run <template>> [-root <dir>] [-variables <path>]]")
}

// fail reports an application error to stderr and returns exit code 1.
func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "error:", err)
	return 1
}
