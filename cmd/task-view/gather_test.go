package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gtindo/task-view/internal/fixture"
	"github.com/gtindo/task-view/internal/review"
)

// gathering is what one `gather --format json` run produced.
type gathering struct {
	exit int
	out  gatherOutput
	raw  string
}

// gather runs gather for a review file placed where the fixture keeps it.
func gather(t *testing.T, set *fixture.Set, args ...string) gathering {
	t.Helper()

	reviewPath := filepath.Join(set.Web, ".review", "change-fixture.json")
	var stdout, stderr bytes.Buffer
	exit := run(slices.Concat([]string{"gather", "--review", reviewPath, "--format", "json"}, args), &stdout, &stderr)

	var out gatherOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON output: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}

	return gathering{exit: exit, out: out, raw: stdout.String()}
}

// fixtureGather gathers the whole fixture change: web from its base with
// the worktree, mock from its base, api as context.
func fixtureGather(t *testing.T, set *fixture.Set, args ...string) gathering {
	t.Helper()

	return gather(t, set, slices.Concat([]string{
		set.Web + "=" + set.WebBase,
		set.Mock + "=" + set.MockBase,
		"--context", set.API,
	}, args)...)
}

func TestGatherGolden(t *testing.T) {
	set := fixture.Build(t)

	res := fixtureGather(t, set)
	if res.exit != review.ExitOK || len(res.out.Errors)+len(res.out.Warnings) > 0 {
		t.Fatalf("exit %d with findings, want a clean run:\n%s", res.exit, res.raw)
	}

	// gather prints one line; the golden file is indented so its diffs read.
	var indented bytes.Buffer
	if err := json.Indent(&indented, []byte(res.raw), "", "  "); err != nil {
		t.Fatal(err)
	}

	compareGolden(t, "gather.golden.json", strings.ReplaceAll(indented.String(), set.Root, "$ROOT"))
}

// A review with one node per ref gather proposes must cover the diff
// exactly: gather and validate read the repos the same way.
func TestGatherCoversDiff(t *testing.T) {
	set := fixture.Build(t)

	res := fixtureGather(t, set)
	rev := &review.Review{
		Version: review.SchemaVersion,
		Summary: "Every ref gather proposes, as a node.",
		Repos:   res.out.Repos,
	}

	for _, file := range res.out.Files {
		// The instructions' rule: a range over each hunk's lines, and the
		// file's own ref for a file without hunks or hunks without lines.
		var refs []*review.Ref
		needsFileRef := len(file.Hunks) == 0
		for _, hunk := range file.Hunks {
			if hunk.Lines == nil {
				needsFileRef = true

				continue
			}

			refs = append(refs, &review.Ref{Type: review.RefRange, Repo: file.Repo, File: file.File, Lines: hunk.Lines})
		}

		if needsFileRef {
			refs = append(refs, file.Ref)
		}

		for _, ref := range refs {
			rev.Nodes = append(rev.Nodes, &review.Node{
				ID:     fmt.Sprintf("n-%d", len(rev.Nodes)+1),
				Kind:   "code",
				Label:  file.File,
				Change: file.Change,
				Ref:    ref,
				Why:    "part of the change",
			})
		}
	}

	checked := validate(t, set, rev)
	if checked.exit != review.ExitOK {
		t.Fatalf("exit %d, want 0:\n%s", checked.exit, checked.raw)
	}

	if count := countCodes(checked.out.Warnings)["unmentioned_hunk"]; count != 0 {
		t.Fatalf("gather's refs leave hunks uncovered:\n%s", checked.raw)
	}
}

func TestGatherCases(t *testing.T) {
	shared := fixture.Build(t)

	cases := []struct {
		name    string
		prepare func(t *testing.T, set *fixture.Set)
		args    func(set *fixture.Set) []string
		exit    int
		code    string
		check   func(t *testing.T, set *fixture.Set, out gatherOutput)
	}{
		{
			name: "unknown base",
			args: func(set *fixture.Set) []string { return []string{set.Web + "=nope"} },
			exit: review.ExitEnv,
			code: "bad_base",
		},
		{
			name: "tip before base",
			args: func(set *fixture.Set) []string {
				return []string{set.Web + "=" + set.WebChange, "--tip", set.WebBase}
			},
			exit: review.ExitEnv,
			code: "bad_base",
		},
		{
			name: "clean repo without base",
			args: func(set *fixture.Set) []string { return []string{set.API} },
			code: "empty_change",
			check: func(t *testing.T, _ *fixture.Set, out gatherOutput) {
				if len(out.Repos) != 0 {
					t.Errorf("empty repo listed: %+v", out.Repos)
				}
			},
		},
		{
			name: "explicit tip leaves the worktree out",
			args: func(set *fixture.Set) []string {
				return []string{set.Web + "=" + set.WebBase, "--tip", set.WebChange}
			},
			code: "uncommitted_changes",
			check: func(t *testing.T, set *fixture.Set, out gatherOutput) {
				if got := out.Repos[0].Commits; !slices.Equal(got, []string{set.WebChange}) {
					t.Errorf("commits %v, want only the tip", got)
				}
			},
		},
		{
			name: "worktree only by default",
			args: func(set *fixture.Set) []string { return []string{set.Web} },
			check: func(t *testing.T, _ *fixture.Set, out gatherOutput) {
				if got := out.Repos[0].Commits; !slices.Equal(got, []string{review.Worktree}) {
					t.Errorf("commits %v, want [WORKTREE]", got)
				}
			},
		},
		{
			name: "review folder is not part of the change",
			prepare: func(t *testing.T, set *fixture.Set) {
				writeTestFile(t, filepath.Join(set.Web, ".git", "info", "exclude"), "")
				writeTestFile(t, filepath.Join(set.Web, ".review", "change-other.json"), "{}\n")
			},
			args:  func(set *fixture.Set) []string { return []string{set.Web} },
			check: checkNoReviewFolder,
		},
		{
			name: "context only when asked for",
			args: func(set *fixture.Set) []string {
				return []string{set.Web + "=" + set.WebBase, "--lines", "1"}
			},
			check: checkContext,
		},
		{
			name: "long hunks are truncated",
			args: func(set *fixture.Set) []string {
				return []string{set.Web + "=" + set.WebBase, "--max-hunk-lines", "1"}
			},
			check: checkTruncated,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			set := shared
			if tc.prepare != nil {
				set = fixture.Build(t)
				tc.prepare(t, set)
			}

			res := gather(t, set, tc.args(set)...)
			if res.exit != tc.exit {
				t.Errorf("exit %d, want %d\n%s", res.exit, tc.exit, res.raw)
			}

			findings := slices.Concat(res.out.Errors, res.out.Warnings)
			if tc.code != "" && countCodes(findings)[tc.code] == 0 {
				t.Errorf("no %s finding\n%s", tc.code, res.raw)
			}

			for _, finding := range findings {
				if finding.Hint == "" {
					t.Errorf("%s has no hint", finding.Code)
				}
			}

			if tc.check != nil {
				tc.check(t, set, res.out)
			}
		})
	}
}

func checkNoReviewFolder(t *testing.T, _ *fixture.Set, out gatherOutput) {
	for _, file := range out.Files {
		if strings.HasPrefix(file.File, ".review/") {
			t.Errorf("review folder gathered: %s", file.File)
		}
	}
}

func checkTruncated(t *testing.T, _ *fixture.Set, out gatherOutput) {
	truncated := false
	for _, file := range out.Files {
		for _, hunk := range file.Hunks {
			if len(hunk.Added) > 1 || len(hunk.Removed) > 1 {
				t.Errorf("%s: hunk keeps more than one line", file.File)
			}

			if hunk.Truncated && hunk.Context != nil {
				t.Errorf("%s: truncated hunk has context", file.File)
			}

			truncated = truncated || hunk.Truncated
		}
	}

	if !truncated {
		t.Error("no hunk marked truncated")
	}
}

func checkContext(t *testing.T, _ *fixture.Set, out gatherOutput) {
	withContext := 0
	for _, file := range out.Files {
		for _, hunk := range file.Hunks {
			if hunk.Context != nil {
				withContext++
			}
		}
	}

	if withContext == 0 {
		t.Error("--lines 1 gave no hunk any context")
	}
}

func TestGatherUsage(t *testing.T) {
	for _, args := range [][]string{{"gather", "=HEAD"}, {"gather", "--format", "yaml"}, {"gather", "--lines", "x"}} {
		var out bytes.Buffer
		if exit := run(args, &out, &out); exit != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, exit, exitUsage)
		}
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
