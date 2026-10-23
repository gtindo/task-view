// Command change-view turns a coding agent's account of a change into review
// views, after checking that the account matches the repos exactly.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gtindo/changes-view/internal/engine"
	"github.com/gtindo/changes-view/internal/instructions"
	"github.com/gtindo/changes-view/internal/langs"
	"github.com/gtindo/changes-view/internal/render"
	"github.com/gtindo/changes-view/internal/review"
)

// version is set at release time with -ldflags.
var version = "0.1.0-dev"

const usage = `change-view — review views for a coding agent's change

Usage:
  change-view validate <file> [--format text|json]
  change-view open <file> [--out <path>] [--no-browser]
  change-view schema
  change-view languages
  change-view instructions
  change-view version

Exit codes (validate, open):
  0  schema valid, no contradicted or unresolved references
  1  at least one contradicted or unresolved reference
  2  schema invalid or file unreadable
  3  environment error (repo missing, not a Git repo, git not found)
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "validate":
		return cmdValidate(rest, stdout, stderr)
	case "open":
		return cmdOpen(rest, stdout, stderr)
	case "schema":
		stdout.Write(review.SchemaJSON)
		return 0
	case "languages":
		for _, l := range langs.All() {
			names := append([]string{l.Name}, l.Aliases...)
			line := strings.Join(names, ", ")
			if len(l.Extensions) > 0 {
				line += "\t" + strings.Join(l.Extensions, " ")
			}
			fmt.Fprintln(stdout, line)
		}
		return 0
	case "instructions":
		fmt.Fprint(stdout, instructions.Text)
		return 0
	case "version", "--version", "-v":
		fmt.Fprintf(stdout, "change-view %s\nschema versions: %s\n", version, joinInts(review.SupportedVersions))
		return 0
	case "help", "--help", "-h":
		fmt.Fprint(stdout, usage)
		return 0
	}
	fmt.Fprintf(stderr, "unknown command %q\n\n%s", cmd, usage)
	return 2
}

// parse splits positional args from flags so flags may follow the file.
func parse(fs *flag.FlagSet, args []string) ([]string, error) {
	var pos []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		args = fs.Args()
		if len(args) > 0 {
			pos = append(pos, args[0])
			args = args[1:]
		}
	}
	return pos, nil
}

func cmdValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	format := fs.String("format", "text", "output format: text or json")
	skipSiblings := fs.Bool("no-sibling-scan", false, "skip the unlisted sibling repo scan")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		fmt.Fprint(stderr, "usage: change-view validate <file> [--format text|json]\n")
		return 2
	}
	e := engine.Run(pos[0], engine.Options{SkipSiblings: *skipSiblings})
	switch *format {
	case "json":
		writeJSON(stdout, e)
	case "text":
		writeText(stdout, pos[0], e)
	default:
		fmt.Fprintf(stderr, "unknown format %q\n", *format)
		return 2
	}
	return e.ExitCode
}

type validateOutput struct {
	Valid         bool             `json:"valid"`
	ExitCode      int              `json:"exitCode"`
	SchemaVersion int              `json:"schemaVersion"`
	Errors        []review.Finding `json:"errors"`
	Warnings      []review.Finding `json:"warnings"`
	Summary       engine.Summary   `json:"summary"`
}

func writeJSON(w io.Writer, e *engine.Engine) {
	errs, warns := review.Split(e.Findings)
	out := validateOutput{Valid: e.ExitCode == 0, ExitCode: e.ExitCode, SchemaVersion: review.SchemaVersion,
		Errors: errs, Warnings: warns, Summary: e.Summary}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	enc.Encode(out)
}

func writeText(w io.Writer, path string, e *engine.Engine) {
	errs, warns := review.Split(e.Findings)
	mark := "✓"
	if e.ExitCode != 0 {
		mark = "✗"
	}
	fmt.Fprintf(w, "%s %s — exit %d: %d %s, %d %s\n", mark, path, e.ExitCode,
		len(errs), pl(len(errs), "error", "errors"), len(warns), pl(len(warns), "warning", "warnings"))
	s := e.Summary
	fmt.Fprintf(w, "  verified %d · contradicted %d · unresolved %d · agent statements %d\n",
		s.Verified, s.Contradicted, s.Unresolved, s.AgentStatements)
	section := func(title string, list []review.Finding) {
		if len(list) == 0 {
			return
		}
		fmt.Fprintf(w, "\n%s\n", title)
		for _, f := range list {
			loc := []string{}
			if f.Path != "" {
				loc = append(loc, f.Path)
			}
			where := f.Repo
			if f.File != "" {
				where += " " + f.File
				if len(f.Lines) == 2 {
					if f.Lines[0] == f.Lines[1] {
						where += fmt.Sprintf(":%d", f.Lines[0])
					} else {
						where += fmt.Sprintf(":%d-%d", f.Lines[0], f.Lines[1])
					}
				}
			}
			if strings.TrimSpace(where) != "" {
				loc = append(loc, strings.TrimSpace(where))
			}
			fmt.Fprintf(w, "  %-22s %s\n", f.Code, strings.Join(loc, "  "))
			fmt.Fprintf(w, "    %s\n", f.Message)
			if f.Hint != "" {
				fmt.Fprintf(w, "    hint: %s\n", f.Hint)
			}
		}
	}
	section("errors", errs)
	section("warnings", warns)
}

func cmdOpen(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("open", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "where to write the HTML file (default: a temp folder)")
	noBrowser := fs.Bool("no-browser", false, "write the page without opening it")
	skipSiblings := fs.Bool("no-sibling-scan", false, "skip the unlisted sibling repo scan")
	pos, err := parse(fs, args)
	if err != nil || len(pos) != 1 {
		fmt.Fprint(stderr, "usage: change-view open <file> [--out <path>] [--no-browser]\n")
		return 2
	}

	e := engine.Run(pos[0], engine.Options{SkipSiblings: *skipSiblings})
	if e.ExitCode >= review.ExitSchema {
		writeText(stderr, pos[0], e)
		fmt.Fprintln(stderr, "\nno page generated: fix the errors above, then run open again")
		return e.ExitCode
	}

	page, err := render.HTML(e, version)
	if err != nil {
		fmt.Fprintf(stderr, "cannot render page: %v\n", err)
		return 3
	}
	target := *out
	if target == "" {
		dir, err := os.MkdirTemp("", "change-view-")
		if err != nil {
			fmt.Fprintf(stderr, "cannot create temp folder: %v\n", err)
			return 3
		}
		target = filepath.Join(dir, "review.html")
	}
	if err := os.WriteFile(target, page, 0o644); err != nil {
		fmt.Fprintf(stderr, "cannot write %s: %v\n", target, err)
		return 3
	}
	abs, _ := filepath.Abs(target)
	if e.ExitCode != 0 {
		writeText(stderr, pos[0], e)
		fmt.Fprintln(stderr)
	}
	fmt.Fprintf(stdout, "wrote %s\n", abs)
	if !*noBrowser {
		if err := openBrowser(abs); err != nil {
			fmt.Fprintf(stderr, "cannot open browser: %v\n", err)
		}
	}
	return e.ExitCode
}

func openBrowser(path string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", path)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", path)
	default:
		cmd = exec.Command("xdg-open", path)
	}
	return cmd.Start()
}

func pl(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ", ")
}
