// Package fixture builds deterministic Git repos for tests: commits, a
// worktree change, a rename, a deleted file and a binary file.
package fixture

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtindo/changes-view/internal/review"
)

// Set is a built fixture: three repos side by side plus an unrelated dirty
// sibling.
type Set struct {
	Root string
	Web  string
	Mock string
	API  string

	WebBase, WebChange   string // web commits: base and the change
	MockBase, MockChange string
	APIHead              string
}

func git(t testing.TB, dir string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "init.defaultBranch=main",
		"-c", "user.name=Fixture", "-c", "user.email=fixture@example.com"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
		"GIT_CONFIG_NOSYSTEM=1", "HOME="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t testing.TB, dir, path, content string) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t testing.TB, dir, msg string) string {
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", msg)
	return git(t, dir, "rev-parse", "HEAD")
}

const aBase = `export function a() {
  return 1;
}

export function b() {
  return 2;
}

export const c = 3;
// end
`

const testBase = `import { a } from "./a";
test("a", () => {
  expect(a()).toBe(1);
});
test("b", () => {
  expect(1).toBe(1);
});
`

// Build creates the fixture under a fresh temp dir.
func Build(t testing.TB) *Set {
	t.Helper()
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	s := &Set{Root: root, Web: filepath.Join(root, "web"), Mock: filepath.Join(root, "mock"), API: filepath.Join(root, "api")}

	// web: base, then a commit with a modify, add, delete, rename and a
	// binary change, then uncommitted work and an untracked file.
	os.MkdirAll(s.Web, 0o755)
	git(t, s.Web, "init", "-q")
	write(t, s.Web, "src/a.ts", aBase)
	write(t, s.Web, "src/a.test.ts", testBase)
	write(t, s.Web, "src/old.ts", "export const old = 1;\n")
	write(t, s.Web, "src/ren.ts", "export const r1 = 1;\nexport const r2 = 2;\nexport const r3 = 3;\nexport const r4 = 4;\nexport const r5 = 5;\n")
	write(t, s.Web, "img.png", "PNG\x00\x01\x02base")
	s.WebBase = commit(t, s.Web, "web base")

	write(t, s.Web, "src/a.ts", strings.Replace(strings.Replace(aBase, "return 1;", "return 10;", 1), "return 2;", "return 20;", 1))
	write(t, s.Web, "src/a.test.ts", strings.Replace(strings.Replace(testBase, "toBe(1);\n});\ntest", "toBe(10);\n});\ntest", 1), "expect(1).toBe(1)", "expect(1).toBe(2)", 1))
	write(t, s.Web, "src/new.ts", "export const n = 1;\nexport const m = a();\n")
	os.Remove(filepath.Join(s.Web, "src/old.ts"))
	os.Rename(filepath.Join(s.Web, "src/ren.ts"), filepath.Join(s.Web, "src/renamed.ts"))
	write(t, s.Web, "img.png", "PNG\x00\x01\x02changed")
	s.WebChange = commit(t, s.Web, "web change")

	write(t, s.Web, "src/a.ts", strings.Replace(strings.Replace(strings.Replace(aBase, "return 1;", "return 10;", 1), "return 2;", "return 20;", 1), "c = 3", "c = 30", 1))
	write(t, s.Web, "src/untracked.ts", "export const u = 1;\n")
	write(t, s.Web, ".git/info/exclude", ".review/\n")

	// mock: one committed change.
	os.MkdirAll(s.Mock, 0o755)
	git(t, s.Mock, "init", "-q")
	write(t, s.Mock, "handlers.ts", "export const h = () => 1;\n// GET /x\n")
	s.MockBase = commit(t, s.Mock, "mock base")
	write(t, s.Mock, "handlers.ts", "export const h = () => 2;\n// GET /x\n")
	s.MockChange = commit(t, s.Mock, "mock change")

	// api: read-only context.
	os.MkdirAll(s.API, 0o755)
	git(t, s.API, "init", "-q")
	write(t, s.API, "routes.ts", "// GET /x\nexport const route = 1;\n")
	s.APIHead = commit(t, s.API, "api")

	// other: a dirty sibling repo the review does not list.
	other := filepath.Join(root, "other")
	os.MkdirAll(other, 0o755)
	git(t, other, "init", "-q")
	write(t, other, "dirty.txt", "x\n")
	return s
}

// Commit makes a new commit in a repo (for head_ahead cases).
func (s *Set) Commit(t testing.TB, dir, path, content, msg string) string {
	write(t, dir, path, content)
	return commit(t, dir, msg)
}

func rng(repo, file string, a, b int) *review.Ref {
	return &review.Ref{Type: review.RefRange, Repo: repo, File: file, Lines: []int{a, b}}
}

func site(repo, file string, line int, contains string) *review.Ref {
	return &review.Ref{Type: review.RefSite, Repo: repo, File: file, Line: line, Contains: contains}
}

func file(repo, path string) *review.Ref {
	return &review.Ref{Type: review.RefFile, Repo: repo, File: path}
}

// Review returns an accurate review of the fixture: it validates with exit
// code 0 and no unmentioned hunks.
func (s *Set) Review() *review.Review {
	ts := review.OrderedMap{{Key: "**/*.ts", Value: "typescript"}, {Key: "**/*.png", Value: "plaintext"}}
	return &review.Review{
		Version: 1,
		Summary: "Fixture change: values bumped, a new module, a removed module, a rename and a binary change.",
		Repos: []*review.Repo{
			{ID: "web", Path: ".", Commits: []string{review.Worktree, s.WebChange}, Languages: ts},
			{ID: "mock", Path: "../mock", Commits: []string{s.MockChange}, Languages: ts},
			{ID: "api", Path: "../api", Role: review.RoleContext, Commits: []string{s.APIHead}, Languages: ts},
		},
		Nodes: []*review.Node{
			{ID: "fn-a", Kind: "code", Label: "a()", Change: "modified", Ref: rng("web", "src/a.ts", 1, 3), Why: "returns 10"},
			{ID: "fn-b", Kind: "code", Label: "b()", Change: "modified", Ref: rng("web", "src/a.ts", 5, 7), Why: "returns 20"},
			{ID: "const-c", Kind: "code", Label: "c", Change: "modified", Ref: rng("web", "src/a.ts", 9, 9), Why: "now 30"},
			{ID: "new", Kind: "code", Label: "new module", Change: "added", Ref: file("web", "src/new.ts"), Why: "adds n and m"},
			{ID: "old", Kind: "code", Label: "old module", Change: "deleted", Ref: file("web", "src/old.ts"), Why: "unused"},
			{ID: "ren", Kind: "code", Label: "renamed module", Change: "modified", Ref: file("web", "src/renamed.ts"), Why: "clearer name"},
			{ID: "img", Kind: "asset", Label: "image", Change: "modified", Ref: file("web", "img.png"), Why: "new art"},
			{ID: "untracked", Kind: "code", Label: "u", Change: "added", Ref: file("web", "src/untracked.ts"), Why: "adds u"},
			{ID: "t-a", Kind: "test", Label: "test a", Change: "modified", Ref: rng("web", "src/a.test.ts", 2, 4), Why: "expects 10",
				Meta: review.OrderedMap{{Key: "command", Value: "vitest"}, {Key: "observed", Value: "not run"}}},
			{ID: "t-b", Kind: "test", Label: "test b", Change: "modified", Ref: rng("web", "src/a.test.ts", 5, 7), Why: "expects 2"},
			{ID: "mock-h", Kind: "code", Label: "mock handler", Change: "modified", Ref: rng("mock", "handlers.ts", 1, 2), Why: "returns 2"},
			{ID: "api-route", Kind: "code", Label: "api route", Change: "unchanged", Ref: rng("api", "routes.ts", 1, 2)},
			{ID: "ep", Kind: "endpoint", Label: "GET /x", Change: "modified", Why: "response changes"},
		},
		Edges: []*review.Edge{
			{From: "t-a", To: "fn-a", Kind: "tests", Ref: site("web", "src/a.test.ts", 3, "expect(a())")},
			{From: "t-b", To: "fn-b", Kind: "tests", Ref: site("web", "src/a.test.ts", 5, `test("b"`)},
			{From: "new", To: "fn-a", Kind: "calls", Ref: site("web", "src/new.ts", 2, "a()")},
			{From: "mock-h", To: "ep", Kind: "mocks", Ref: site("mock", "handlers.ts", 2, "GET /x")},
			{From: "api-route", To: "ep", Kind: "serves", Ref: site("api", "routes.ts", 1, "GET /x")},
		},
		Flows: []*review.Flow{{ID: "call-a", Title: "new calls a", Path: []string{"new", "fn-a"}, Tests: []string{"t-a"}}},
		Claims: []*review.Claim{{Text: "a returns 10", About: []string{"fn-a"}, Evidence: []*review.Ref{
			{Type: review.RefHunk, Repo: "web", File: "src/a.ts", Lines: []int{2, 2}},
			{Type: review.RefCommit, Repo: "web", SHA: s.WebChange},
		}}},
		Notes: []string{"Fixture."},
	}
}
