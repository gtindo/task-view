package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/gtindo/task-view/internal/engine"
	"github.com/gtindo/task-view/internal/review"
)

const gatherUsage = "usage: task-view gather [<path>[=<base>]...] [--base <rev>] [--tip <rev>] [--context <path>]...\n" +
	"                        [--review <file>] [--lines <n>] [--max-hunk-lines <n>] [--format text|json]\n"

// pathList is a repeatable string flag.
type pathList []string

func (p *pathList) String() string { return strings.Join(*p, ",") }

func (p *pathList) Set(value string) error {
	*p = append(*p, value)

	return nil
}

// gatherOutput is the JSON gather prints: the evidence between the exit
// code and the findings, in the order an agent reads it.
type gatherOutput struct {
	ExitCode      int `json:"exitCode"`
	SchemaVersion int `json:"schemaVersion"`
	*engine.Evidence
	Errors   []review.Finding `json:"errors"`
	Warnings []review.Finding `json:"warnings"`
}

func cmdGather(args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("gather", flag.ContinueOnError)
	flags.SetOutput(stderr)
	format := flags.String("format", "text", "output format: text or json")
	base := flags.String("base", "", "commit the change started from, for repos without their own =<base>")
	tip := flags.String("tip", "", "newest commit of the change (default: HEAD plus uncommitted work)")
	reviewPath := flags.String("review", "", "where the review file will live (default: .review/change.json in this Git root)")
	contextLines := flags.Int("lines", engine.DefaultContextLines, "context lines around each hunk (default none)")
	maxHunkLines := flags.Int("max-hunk-lines", engine.DefaultMaxHunkLines, "added and removed lines kept per hunk; 0 keeps all")

	var contextPaths pathList
	flags.Var(&contextPaths, "context", "a repo you only read (repeatable)")

	positional, err := parseArgs(flags, args)
	if err != nil {
		emit(stderr, "%s", gatherUsage)

		return exitUsage
	}

	specs, ok := repoSpecs(positional, contextPaths, *base)
	knownFormat := *format == "json" || *format == "text"
	if !ok || !knownFormat {
		emit(stderr, "%s", gatherUsage)

		return exitUsage
	}

	evidence := engine.Gather(engine.GatherOptions{
		ReviewPath:   *reviewPath,
		Repos:        specs,
		Tip:          *tip,
		ContextLines: *contextLines,
		MaxHunkLines: *maxHunkLines,
	})

	if *format == "text" {
		emit(stdout, "%s", gatherTextReport(evidence))

		return evidence.ExitCode
	}

	// Unlike validate's report, the evidence is long and read by agents
	// only, so it is printed compact: indentation would triple its size.
	errs, warnings := review.Split(evidence.Findings)
	encodeJSON(stdout, "", gatherOutput{
		ExitCode:      evidence.ExitCode,
		SchemaVersion: review.SchemaVersion,
		Evidence:      evidence,
		Errors:        errs,
		Warnings:      warnings,
	})

	return evidence.ExitCode
}

// repoSpecs turns `<path>[=<base>]` arguments and --context paths into repo
// specs; with no positional path the current directory is the changed repo.
// ok is false for an argument with no path.
func repoSpecs(positional, contextPaths []string, defaultBase string) ([]engine.RepoSpec, bool) {
	if len(positional) == 0 {
		positional = []string{"."}
	}

	var specs []engine.RepoSpec
	for _, argument := range positional {
		repoPath, repoBase, hasBase := strings.Cut(argument, "=")
		if repoPath == "" {
			return nil, false
		}

		if !hasBase {
			repoBase = defaultBase
		}

		specs = append(specs, engine.RepoSpec{Path: repoPath, Base: repoBase})
	}

	for _, contextPath := range contextPaths {
		specs = append(specs, engine.RepoSpec{Path: contextPath, Context: true})
	}

	return specs, true
}

// gatherTextReport renders the evidence for a reader: a status line, the
// repos block to paste, each file with its hunks, then the findings.
func gatherTextReport(evidence *engine.Evidence) string {
	errs, warnings := review.Split(evidence.Findings)
	mark := "✓"
	if evidence.ExitCode != review.ExitOK {
		mark = "✗"
	}

	var report strings.Builder
	fmt.Fprintf(&report, "%s gather — exit %d: %d %s, %d changed %s, %d %s, %d %s\n", mark, evidence.ExitCode,
		len(evidence.Repos), plural(len(evidence.Repos), "repo", "repos"),
		len(evidence.Files), plural(len(evidence.Files), "file", "files"),
		len(errs), plural(len(errs), "error", "errors"),
		len(warnings), plural(len(warnings), "warning", "warnings"))

	// The repos come straight from the review model, which always encodes.
	reposJSON, _ := json.MarshalIndent(evidence.Repos, "", "  ")
	fmt.Fprintf(&report, "\nrepos (paste into the review file):\n%s\n", reposJSON)

	for _, file := range evidence.Files {
		writeFileEvidence(&report, file)
	}

	writeFindings(&report, "errors", errs)
	writeFindings(&report, "warnings", warnings)

	return report.String()
}

// writeFileEvidence appends one changed file and its hunks, removed lines
// before added ones as in a diff.
func writeFileEvidence(report *strings.Builder, file *engine.FileEvidence) {
	fmt.Fprintf(report, "\n%s %s  %s", file.Repo, file.File, file.Change)
	if file.OldFile != "" {
		fmt.Fprintf(report, " (renamed from %s)", file.OldFile)
	}

	switch {
	case file.Binary:
		report.WriteString(" · binary")
	case file.Language != "":
		fmt.Fprintf(report, " · %s", file.Language)
	}

	fmt.Fprintf(report, " · +%d −%d\n", file.Added, file.Removed)

	for _, hunk := range file.Hunks {
		fmt.Fprintf(report, "  @@ %s", spanText("lines", hunk.Lines))
		if len(hunk.OldLines) > 0 {
			fmt.Fprintf(report, " %s", spanText("was", hunk.OldLines))
		}

		if len(hunk.Commits) > 0 {
			fmt.Fprintf(report, " · %s", strings.Join(shortStates(hunk.Commits), ", "))
		}

		report.WriteString("\n")
		for _, line := range hunk.Removed {
			fmt.Fprintf(report, "    - %s\n", line)
		}

		for _, line := range hunk.Added {
			fmt.Fprintf(report, "    + %s\n", line)
		}

		if hunk.Truncated {
			report.WriteString("    … truncated; read the file for the rest\n")
		}
	}
}

// spanText labels a [first, last] line pair, or returns "" without one.
func spanText(label string, lines []int) string {
	if len(lines) != 2 {
		return ""
	}

	if lines[0] == lines[1] {
		return fmt.Sprintf("%s %d", label, lines[0])
	}

	return fmt.Sprintf("%s %d-%d", label, lines[0], lines[1])
}

// shortSHALen is how many SHA characters the text report shows.
const shortSHALen = 10

// shortStates shortens commit SHAs for display and keeps WORKTREE as is.
func shortStates(states []string) []string {
	shortened := make([]string, len(states))
	for i, state := range states {
		shortened[i] = state[:min(len(state), shortSHALen)]
	}

	return shortened
}
