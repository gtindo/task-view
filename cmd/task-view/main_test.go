package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gtindo/task-view/internal/fixture"
	"github.com/gtindo/task-view/internal/review"
)

var update = flag.Bool("update", false, "rewrite golden files")

// validation is what one `validate --format json` run produced.
type validation struct {
	exit int
	out  validateOutput
	raw  string
}

// writeReviewFile puts data where the fixture's review file lives and returns
// its path.
func writeReviewFile(t *testing.T, set *fixture.Set, data []byte) string {
	t.Helper()

	dir := filepath.Join(set.Web, ".review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "change-fixture.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

func marshalReview(t *testing.T, rev *review.Review) []byte {
	t.Helper()

	data, err := json.MarshalIndent(rev, "", "  ")
	if err != nil {
		t.Fatal(err)
	}

	return data
}

func validateRaw(t *testing.T, set *fixture.Set, data []byte, extraArgs ...string) validation {
	t.Helper()

	path := writeReviewFile(t, set, data)

	var stdout, stderr bytes.Buffer
	args := append([]string{"validate", path, "--format", "json"}, extraArgs...)
	exit := run(args, &stdout, &stderr)

	var out validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON output: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}

	return validation{exit: exit, out: out, raw: stdout.String()}
}

func validate(t *testing.T, set *fixture.Set, rev *review.Review, extraArgs ...string) validation {
	t.Helper()

	return validateRaw(t, set, marshalReview(t, rev), extraArgs...)
}

func countCodes(findings []review.Finding) map[string]int {
	counts := map[string]int{}
	for _, finding := range findings {
		counts[finding.Code]++
	}

	return counts
}

func TestValidReviewGolden(t *testing.T) {
	set := fixture.Build(t)

	res := validate(t, set, set.Review())
	if res.exit != review.ExitOK {
		t.Fatalf("exit %d, want 0:\n%s", res.exit, res.raw)
	}

	if n := countCodes(res.out.Warnings)["unmentioned_hunk"]; n != 0 {
		t.Fatalf("unexpected unmentioned hunks:\n%s", res.raw)
	}

	got := strings.ReplaceAll(res.raw, set.Root, "$ROOT")
	golden := filepath.Join("testdata", "valid.golden.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("missing golden file (run with -update): %v", err)
	}

	if got != string(want) {
		t.Errorf("output differs from %s (run with -update to accept):\n%s", golden, got)
	}
}

// Each case breaks the accurate review in one way and expects one code.
func TestCodes(t *testing.T) {
	shared := fixture.Build(t)

	cases := []struct {
		code string
		exit int
		// prepare changes the repos, so a case that sets it gets its own fixture.
		prepare func(t *testing.T, set *fixture.Set)
		mutate  func(rev *review.Review, set *fixture.Set)
		raw     []byte // sent instead of the marshalled review
		args    []string
	}{
		{
			code: "file_unreadable",
			exit: review.ExitSchema,
			raw:  []byte("{not json"),
		},
		{
			code: "unsupported_version",
			exit: review.ExitSchema,
			raw:  []byte(`{"version": 7, "summary": "x", "repos": [], "nodes": []}`),
		},
		{
			code:   "schema_invalid",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Change = "tweaked" },
		},
		{
			code:   "schema_invalid",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Ref.Lines = []int{9, 3} },
		},
		{
			code:   "duplicate_id",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[1].ID = rev.Nodes[0].ID },
		},
		{
			code:   "unknown_node",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Edges[0].To = "nope" },
		},
		{
			code:   "not_a_test",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Flows[0].Tests = []string{"fn-a"} },
		},
		{
			code:   "unknown_repo",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Ref.Repo = "nope" },
		},
		{
			code:   "bad_path",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Ref.File = "../web/src/a.ts" },
		},
		{
			code:   "unknown_language",
			exit:   review.ExitSchema,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Repos[0].Languages[0].Value = "typescrypt" },
		},
		{
			code:   "repo_not_found",
			exit:   review.ExitEnv,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Repos[1].Path = "../missing" },
		},
		{
			code:   "not_repo_root",
			exit:   review.ExitEnv,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Repos[0].Path = "src" },
		},
		{
			code: "commit_not_found",
			exit: review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) {
				rev.Repos[1].Commits = []string{strings.Repeat("ab", 20)}
			},
		},
		{
			code: "commit_chain_broken",
			exit: review.ExitReferences,
			mutate: func(rev *review.Review, set *fixture.Set) {
				rev.Repos[1].Commits = []string{set.MockBase, set.MockChange}
			},
		},
		{
			code: "commit_chain_broken",
			exit: review.ExitReferences,
			mutate: func(rev *review.Review, set *fixture.Set) {
				rev.Repos[0].Commits = []string{set.WebChange, review.Worktree}
			},
		},
		{
			code:   "context_commits",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Repos[2].Commits = []string{review.Worktree} },
		},
		{
			code:   "bad_at",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, set *fixture.Set) { rev.Claims[0].Evidence[0].At = set.WebBase },
		},
		{
			code:   "file_not_found",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Ref.File = "src/missing.ts" },
		},
		{
			code:   "range_out_of_bounds",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[2].Ref.Lines = []int{9, 40} },
		},
		{
			code: "binary_file",
			exit: review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) {
				rev.Nodes[6].Ref = &review.Ref{Type: review.RefRange, Repo: "web", File: "img.png", Lines: []int{1, 1}}
			},
		},
		{
			code:   "change_mismatch",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Ref.Lines = []int{4, 4} },
		},
		{
			code:   "change_mismatch",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[3].Change = "modified" },
		},
		{
			code: "change_mismatch",
			exit: review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) {
				rev.Nodes[11].Change = "modified"
				rev.Nodes[11].Why = "x"
			},
		},
		{
			code:   "site_mismatch",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Edges[2].Ref.Contains = "b()" },
		},
		{
			code:   "hunk_not_found",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Claims[0].Evidence[0].Lines = []int{4, 4} },
		},
		{
			code:   "commit_not_listed",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, set *fixture.Set) { rev.Claims[0].Evidence[1].SHA = set.WebBase },
		},
		{
			code:   "site_offset",
			exit:   review.ExitOK,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Edges[0].Ref.Line = 4 },
		},
		{
			code:   "unmentioned_hunk",
			exit:   review.ExitOK,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes = append(rev.Nodes[:2], rev.Nodes[3:]...) },
		},
		{
			code:   "unreferenced_test_hunk",
			exit:   review.ExitOK,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[9].Kind = "code" },
		},
		{
			code: "unlisted_repo",
			exit: review.ExitOK,
			args: []string{"--scan-siblings"},
		},
		{
			code:   "uncommitted_changes",
			exit:   review.ExitReferences,
			mutate: func(rev *review.Review, set *fixture.Set) { rev.Repos[0].Commits = []string{set.WebChange} },
		},
		{
			code: "head_ahead",
			exit: review.ExitOK,
			prepare: func(t *testing.T, set *fixture.Set) {
				set.Commit(t, set.Mock, "later.ts", "x\n", "later work")
			},
		},
		{
			code:   "no_language",
			exit:   review.ExitOK,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Repos[0].Languages = rev.Repos[0].Languages[:1] },
		},
		{
			code:   "missing_why",
			exit:   review.ExitOK,
			mutate: func(rev *review.Review, _ *fixture.Set) { rev.Nodes[0].Why = "" },
		},
	}

	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			set := shared
			if tc.prepare != nil {
				set = fixture.Build(t)
				tc.prepare(t, set)
			}

			data := tc.raw
			if data == nil {
				rev := set.Review()
				if tc.mutate != nil {
					tc.mutate(rev, set)
				}

				data = marshalReview(t, rev)
			}

			res := validateRaw(t, set, data, tc.args...)
			if res.exit != tc.exit {
				t.Errorf("exit %d, want %d\n%s", res.exit, tc.exit, res.raw)
			}

			findings := slices.Concat(res.out.Errors, res.out.Warnings)
			if countCodes(findings)[tc.code] == 0 {
				t.Errorf("no %s finding\n%s", tc.code, res.raw)
			}

			for _, finding := range findings {
				needsHint := finding.Code == tc.code && finding.Code != "file_unreadable"
				if needsHint && finding.Hint == "" {
					t.Errorf("%s has no hint", finding.Code)
				}
			}
		})
	}
}

func TestSiblingsNotScannedByDefault(t *testing.T) {
	set := fixture.Build(t)

	res := validate(t, set, set.Review())
	if n := countCodes(res.out.Warnings)["unlisted_repo"]; n != 0 {
		t.Fatalf("unlisted_repo without --scan-siblings:\n%s", res.raw)
	}
}

// Git reports repo roots with symlinks resolved, so a review file reached
// through a symlink (macOS temp dirs, a linked checkout) must still have its
// .review/ folder left out of the diff.
func TestReviewPathThroughSymlink(t *testing.T) {
	set := fixture.Build(t)
	path := writeReviewFile(t, set, marshalReview(t, set.Review()))

	// The fixture hides .review/ from Git; show it, so only task-view's own
	// exclusion keeps it out.
	if err := os.WriteFile(filepath.Join(set.Web, ".git", "info", "exclude"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(set.Root, link); err != nil {
		t.Fatal(err)
	}

	relPath, err := filepath.Rel(set.Root, path)
	if err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	exit := run([]string{"validate", filepath.Join(link, relPath), "--format", "json"}, &stdout, &stderr)
	if exit != review.ExitOK {
		t.Fatalf("exit %d, want 0:\n%s%s", exit, stdout.String(), stderr.String())
	}

	var out validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON output: %v\n%s", err, stdout.String())
	}

	warnings := countCodes(out.Warnings)
	reviewFolderCounted := warnings["unmentioned_hunk"] > 0 || warnings["uncommitted_changes"] > 0
	if reviewFolderCounted {
		t.Fatalf("the review folder counted as part of the change:\n%s", stdout.String())
	}
}

func TestGitNotFound(t *testing.T) {
	set := fixture.Build(t)
	path := writeReviewFile(t, set, marshalReview(t, set.Review()))
	t.Setenv("PATH", "")

	var out bytes.Buffer
	if exit := run([]string{"validate", path, "--format", "json"}, &out, &out); exit != review.ExitEnv {
		t.Fatalf("exit %d, want 3\n%s", exit, out.String())
	}

	if !strings.Contains(out.String(), "git_not_found") {
		t.Fatalf("no git_not_found:\n%s", out.String())
	}
}

func TestOpenWritesPage(t *testing.T) {
	set := fixture.Build(t)
	path := writeReviewFile(t, set, marshalReview(t, set.Review()))
	page := filepath.Join(t.TempDir(), "review.html")

	var out bytes.Buffer
	if exit := run([]string{"open", path, "--no-browser", "--out", page}, &out, &out); exit != review.ExitOK {
		t.Fatalf("exit %d\n%s", exit, out.String())
	}

	html, err := os.ReadFile(page)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{`id="review-data"`, "cytoscape", "fn-a", "return 10"} {
		if !bytes.Contains(html, []byte(want)) {
			t.Errorf("page has no %q", want)
		}
	}

	loadsExternalScript := bytes.Contains(html, []byte("https://")) && bytes.Contains(html, []byte("<script src="))
	if loadsExternalScript {
		t.Errorf("page loads an external script")
	}
}

func TestOpenDefaultsToGeneratedFolder(t *testing.T) {
	set := fixture.Build(t)
	path := writeReviewFile(t, set, marshalReview(t, set.Review()))

	var out bytes.Buffer
	if exit := run([]string{"open", path, "--no-browser"}, &out, &out); exit != review.ExitOK {
		t.Fatalf("exit %d\n%s", exit, out.String())
	}

	page := filepath.Join(filepath.Dir(path), "generated", "change-fixture.html")
	if _, err := os.Stat(page); err != nil {
		t.Fatalf("no page at %s: %v\n%s", page, err, out.String())
	}
}

func TestOpenRefusesSchemaErrors(t *testing.T) {
	set := fixture.Build(t)
	path := writeReviewFile(t, set, []byte("{}"))
	page := filepath.Join(t.TempDir(), "review.html")

	var out bytes.Buffer
	if exit := run([]string{"open", path, "--no-browser", "--out", page}, &out, &out); exit != review.ExitSchema {
		t.Fatalf("exit %d, want 2", exit)
	}

	if _, err := os.Stat(page); err == nil {
		t.Fatal("page written despite schema errors")
	}
}

func TestCommands(t *testing.T) {
	for _, command := range []string{"schema", "languages", "instructions", "version"} {
		var out bytes.Buffer
		if exit := run([]string{command}, &out, &out); exit != review.ExitOK || out.Len() == 0 {
			t.Errorf("%s: exit %d, %d bytes", command, exit, out.Len())
		}
	}

	var out bytes.Buffer
	run([]string{"schema"}, &out, &out)

	var schema map[string]any
	if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
}
