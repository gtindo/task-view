package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtindo/changes-view/internal/fixture"
	"github.com/gtindo/changes-view/internal/review"
)

var update = flag.Bool("update", false, "rewrite golden files")

type result struct {
	exit int
	out  validateOutput
	raw  string
}

func validateRaw(t *testing.T, s *fixture.Set, data []byte, extra ...string) result {
	t.Helper()
	dir := filepath.Join(s.Web, ".review")
	os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "change.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	args := append([]string{"validate", path, "--format", "json"}, extra...)
	exit := run(args, &stdout, &stderr)
	var out validateOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("bad JSON output: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	return result{exit, out, stdout.String()}
}

func validate(t *testing.T, s *fixture.Set, r *review.Review, extra ...string) result {
	t.Helper()
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return validateRaw(t, s, data, append([]string{"--no-sibling-scan"}, extra...)...)
}

func codes(fs []review.Finding) map[string]int {
	m := map[string]int{}
	for _, f := range fs {
		m[f.Code]++
	}
	return m
}

func TestValidReviewGolden(t *testing.T) {
	s := fixture.Build(t)
	res := validate(t, s, s.Review())
	if res.exit != 0 {
		t.Fatalf("exit %d, want 0:\n%s", res.exit, res.raw)
	}
	if n := codes(res.out.Warnings)["unmentioned_hunk"]; n != 0 {
		t.Fatalf("unexpected unmentioned hunks:\n%s", res.raw)
	}

	got := strings.ReplaceAll(res.raw, s.Root, "$ROOT")
	golden := filepath.Join("testdata", "valid.golden.json")
	if *update {
		os.MkdirAll("testdata", 0o755)
		os.WriteFile(golden, []byte(got), 0o644)
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
		code   string
		exit   int
		fresh  bool // needs its own fixture because it changes the repos
		mutate func(r *review.Review, s *fixture.Set)
		raw    func(s *fixture.Set) []byte
		args   []string
	}{
		{code: "file_unreadable", exit: 2, raw: func(*fixture.Set) []byte { return []byte("{not json") }},
		{code: "unsupported_version", exit: 2, raw: func(*fixture.Set) []byte { return []byte(`{"version": 7, "summary": "x", "repos": [], "nodes": []}`) }},
		{code: "schema_invalid", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Change = "tweaked" }},
		{code: "schema_invalid", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Ref.Lines = []int{9, 3} }},
		{code: "duplicate_id", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[1].ID = r.Nodes[0].ID }},
		{code: "unknown_node", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Edges[0].To = "nope" }},
		{code: "not_a_test", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Flows[0].Tests = []string{"fn-a"} }},
		{code: "unknown_repo", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Ref.Repo = "nope" }},
		{code: "bad_path", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Ref.File = "../web/src/a.ts" }},
		{code: "unknown_language", exit: 2, mutate: func(r *review.Review, _ *fixture.Set) { r.Repos[0].Languages[0].Value = "typescrypt" }},
		{code: "repo_not_found", exit: 3, mutate: func(r *review.Review, _ *fixture.Set) { r.Repos[1].Path = "../missing" }},
		{code: "not_repo_root", exit: 3, mutate: func(r *review.Review, _ *fixture.Set) { r.Repos[0].Path = "src" }},
		{code: "commit_not_found", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Repos[1].Commits = []string{strings.Repeat("ab", 20)} }},
		{code: "commit_chain_broken", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Repos[1].Commits = []string{s.MockBase, s.MockChange} }},
		{code: "commit_chain_broken", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Repos[0].Commits = []string{s.WebChange, review.Worktree} }},
		{code: "context_commits", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Repos[2].Commits = []string{review.Worktree} }},
		{code: "bad_at", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Claims[0].Evidence[0].At = s.WebBase }},
		{code: "file_not_found", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Ref.File = "src/missing.ts" }},
		{code: "range_out_of_bounds", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[2].Ref.Lines = []int{9, 40} }},
		{code: "binary_file", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) {
			r.Nodes[6].Ref = &review.Ref{Type: review.RefRange, Repo: "web", File: "img.png", Lines: []int{1, 1}}
		}},
		{code: "change_mismatch", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Ref.Lines = []int{4, 4} }},
		{code: "change_mismatch", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[3].Change = "modified" }},
		{code: "change_mismatch", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[11].Change = "modified"; r.Nodes[11].Why = "x" }},
		{code: "site_mismatch", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Edges[2].Ref.Contains = "b()" }},
		{code: "hunk_not_found", exit: 1, mutate: func(r *review.Review, _ *fixture.Set) { r.Claims[0].Evidence[0].Lines = []int{4, 4} }},
		{code: "commit_not_listed", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Claims[0].Evidence[1].SHA = s.WebBase }},
		{code: "site_offset", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) { r.Edges[0].Ref.Line = 4 }},
		{code: "unmentioned_hunk", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes = append(r.Nodes[:2], r.Nodes[3:]...) }},
		{code: "unreferenced_test_hunk", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[9].Kind = "code" }},
		{code: "untested_change", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) {}},
		{code: "unlisted_repo", exit: 0, raw: func(s *fixture.Set) []byte { b, _ := json.Marshal(s.Review()); return b }},
		{code: "uncommitted_changes", exit: 1, mutate: func(r *review.Review, s *fixture.Set) { r.Repos[0].Commits = []string{s.WebChange} }},
		{code: "head_ahead", exit: 0, fresh: true, mutate: func(r *review.Review, s *fixture.Set) {
			s.Commit(t, s.Mock, "later.ts", "x\n", "later work")
		}},
		{code: "no_language", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) { r.Repos[0].Languages = r.Repos[0].Languages[:1] }},
		{code: "missing_why", exit: 0, mutate: func(r *review.Review, _ *fixture.Set) { r.Nodes[0].Why = "" }},
	}

	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			s := shared
			if c.fresh {
				s = fixture.Build(t)
			}
			var res result
			switch {
			case c.raw != nil && c.code == "unlisted_repo":
				res = validateRaw(t, s, c.raw(s))
			case c.raw != nil:
				res = validateRaw(t, s, c.raw(s), "--no-sibling-scan")
			default:
				r := s.Review()
				c.mutate(r, s)
				res = validate(t, s, r, c.args...)
			}
			if res.exit != c.exit {
				t.Errorf("exit %d, want %d\n%s", res.exit, c.exit, res.raw)
			}
			all := codes(append(append([]review.Finding{}, res.out.Errors...), res.out.Warnings...))
			if all[c.code] == 0 {
				t.Errorf("no %s finding\n%s", c.code, res.raw)
			}
			for _, f := range append(res.out.Errors, res.out.Warnings...) {
				if f.Code == c.code && f.Hint == "" && f.Code != "file_unreadable" {
					t.Errorf("%s has no hint", f.Code)
				}
			}
		})
	}
}

func TestGitNotFound(t *testing.T) {
	s := fixture.Build(t)
	data, _ := json.Marshal(s.Review())
	path := filepath.Join(s.Web, ".review", "change.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
	t.Setenv("PATH", "")
	var out bytes.Buffer
	if exit := run([]string{"validate", path, "--format", "json"}, &out, &out); exit != 3 {
		t.Fatalf("exit %d, want 3\n%s", exit, out.String())
	}
	if !strings.Contains(out.String(), "git_not_found") {
		t.Fatalf("no git_not_found:\n%s", out.String())
	}
}

func TestOpenWritesPage(t *testing.T) {
	s := fixture.Build(t)
	data, _ := json.Marshal(s.Review())
	path := filepath.Join(s.Web, ".review", "change.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)
	page := filepath.Join(t.TempDir(), "review.html")
	var out bytes.Buffer
	if exit := run([]string{"open", path, "--no-browser", "--no-sibling-scan", "--out", page}, &out, &out); exit != 0 {
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
	if bytes.Contains(html, []byte("https://")) && bytes.Contains(html, []byte("<script src=")) {
		t.Errorf("page loads an external script")
	}
}

func TestOpenRefusesSchemaErrors(t *testing.T) {
	s := fixture.Build(t)
	path := filepath.Join(s.Web, ".review", "change.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("{}"), 0o644)
	page := filepath.Join(t.TempDir(), "review.html")
	var out bytes.Buffer
	if exit := run([]string{"open", path, "--no-browser", "--out", page}, &out, &out); exit != 2 {
		t.Fatalf("exit %d, want 2", exit)
	}
	if _, err := os.Stat(page); err == nil {
		t.Fatal("page written despite schema errors")
	}
}

func TestCommands(t *testing.T) {
	for _, cmd := range []string{"schema", "languages", "instructions", "version"} {
		var out bytes.Buffer
		if exit := run([]string{cmd}, &out, &out); exit != 0 || out.Len() == 0 {
			t.Errorf("%s: exit %d, %d bytes", cmd, exit, out.Len())
		}
	}
	var out bytes.Buffer
	run([]string{"schema"}, &out, &out)
	var schema map[string]any
	if err := json.Unmarshal(out.Bytes(), &schema); err != nil {
		t.Fatalf("schema is not JSON: %v", err)
	}
}
