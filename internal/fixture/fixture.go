// Package fixture builds deterministic Git repos for tests: commits, a
// worktree change, a rename, a deleted file and a binary file.
package fixture

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gtindo/task-view/internal/review"
)

// Set is a built fixture: three repos side by side plus an unrelated dirty
// sibling. The paths are absolute; the SHAs are stable across runs, so golden
// output may contain them.
type Set struct {
	Root string
	Web  string
	Mock string
	API  string

	WebBase, WebChange   string // web commits: base and the change
	MockBase, MockChange string
	APIHead              string
}

// gitConfig pins everything that would otherwise come from the user's Git
// setup, so commits get the same SHAs on every machine.
var gitConfig = []string{
	"-c", "commit.gpgsign=false",
	"-c", "core.hooksPath=/dev/null",
	"-c", "init.defaultBranch=main",
	"-c", "user.name=Fixture",
	"-c", "user.email=fixture@example.com",
}

// gitEnv fixes the commit dates and keeps system config out, for the same
// reason as gitConfig; git also points HOME at the repo to skip global config.
var gitEnv = []string{
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
	"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	"GIT_CONFIG_NOSYSTEM=1",
}

func git(t testing.TB, dir string, args ...string) string {
	t.Helper()

	command := exec.Command("git", slices.Concat(gitConfig, args)...)
	command.Dir = dir
	command.Env = slices.Concat(os.Environ(), gitEnv, []string{"HOME=" + dir})

	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}

	return strings.TrimSpace(string(output))
}

func initRepo(t testing.TB, dir string) {
	t.Helper()

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	git(t, dir, "init", "-q")
}

func writeFile(t testing.TB, dir, path, content string) {
	t.Helper()

	fullPath := filepath.Join(dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t testing.TB, dir, message string) string {
	t.Helper()

	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", message)

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

const renameBase = `export const r1 = 1;
export const r2 = 2;
export const r3 = 3;
export const r4 = 4;
export const r5 = 5;
`

// Build creates the fixture under a fresh temp dir.
func Build(t testing.TB) *Set {
	t.Helper()

	// Resolve symlinks (macOS temp dirs live under /private) so paths match
	// what Git reports.
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	set := &Set{
		Root: root,
		Web:  filepath.Join(root, "web"),
		Mock: filepath.Join(root, "mock"),
		API:  filepath.Join(root, "api"),
	}

	buildWeb(t, set)

	// mock: one committed change.
	initRepo(t, set.Mock)
	writeFile(t, set.Mock, "handlers.ts", "export const h = () => 1;\n// GET /x\n")
	set.MockBase = commit(t, set.Mock, "mock base")
	writeFile(t, set.Mock, "handlers.ts", "export const h = () => 2;\n// GET /x\n")
	set.MockChange = commit(t, set.Mock, "mock change")

	// api: read-only context.
	initRepo(t, set.API)
	writeFile(t, set.API, "routes.ts", "// GET /x\nexport const route = 1;\n")
	set.APIHead = commit(t, set.API, "api")

	// other: a dirty sibling repo the review does not list.
	other := filepath.Join(root, "other")
	initRepo(t, other)
	writeFile(t, other, "dirty.txt", "x\n")

	return set
}

// buildWeb makes the web repo: a base, then a commit with a modify, add,
// delete, rename and a binary change, then uncommitted work and an untracked
// file.
func buildWeb(t testing.TB, set *Set) {
	t.Helper()

	web := set.Web
	initRepo(t, web)
	writeFile(t, web, "src/a.ts", aBase)
	writeFile(t, web, "src/a.test.ts", testBase)
	writeFile(t, web, "src/old.ts", "export const old = 1;\n")
	writeFile(t, web, "src/ren.ts", renameBase)
	writeFile(t, web, "img.png", "PNG\x00\x01\x02base")
	set.WebBase = commit(t, web, "web base")

	aChanged := strings.Replace(aBase, "return 1;", "return 10;", 1)
	aChanged = strings.Replace(aChanged, "return 2;", "return 20;", 1)
	testChanged := strings.Replace(testBase, "toBe(1);\n});\ntest", "toBe(10);\n});\ntest", 1)
	testChanged = strings.Replace(testChanged, "expect(1).toBe(1)", "expect(1).toBe(2)", 1)
	writeFile(t, web, "src/a.ts", aChanged)
	writeFile(t, web, "src/a.test.ts", testChanged)
	writeFile(t, web, "src/new.ts", "export const n = 1;\nexport const m = a();\n")

	if err := os.Remove(filepath.Join(web, "src/old.ts")); err != nil {
		t.Fatal(err)
	}

	if err := os.Rename(filepath.Join(web, "src/ren.ts"), filepath.Join(web, "src/renamed.ts")); err != nil {
		t.Fatal(err)
	}

	writeFile(t, web, "img.png", "PNG\x00\x01\x02changed")
	set.WebChange = commit(t, web, "web change")

	writeFile(t, web, "src/a.ts", strings.Replace(aChanged, "c = 3", "c = 30", 1))
	writeFile(t, web, "src/untracked.ts", "export const u = 1;\n")
	// Keep the review file itself out of the worktree change.
	writeFile(t, web, ".git/info/exclude", ".review/\n")
}

// Commit writes content to path in the repo at dir and commits it with msg,
// returning the new SHA. Tests use it to move a repo's HEAD past the commits
// a review lists (head_ahead cases).
func (s *Set) Commit(t testing.TB, dir, path, content, msg string) string {
	t.Helper()

	writeFile(t, dir, path, content)

	return commit(t, dir, msg)
}

func rangeRef(repo, file string, first, last int) *review.Ref {
	return &review.Ref{Type: review.RefRange, Repo: repo, File: file, Lines: []int{first, last}}
}

func siteRef(repo, file string, line int, contains string) *review.Ref {
	return &review.Ref{Type: review.RefSite, Repo: repo, File: file, Line: line, Contains: contains}
}

func fileRef(repo, path string) *review.Ref {
	return &review.Ref{Type: review.RefFile, Repo: repo, File: path}
}

// Review returns an accurate review of the fixture: it validates with exit
// code 0 and no unmentioned hunks. Each call returns a fresh value, so tests
// may mutate it to break one thing.
func (s *Set) Review() *review.Review {
	languages := review.OrderedMap{
		{Key: "**/*.ts", Value: "typescript"},
		{Key: "**/*.png", Value: "plaintext"},
	}

	return &review.Review{
		Version: 1,
		Summary: "Fixture change: values bumped, a new module, a removed module, a rename and a binary change.",
		Repos: []*review.Repo{
			{ID: "web", Path: ".", Commits: []string{review.Worktree, s.WebChange}, Languages: languages},
			{ID: "mock", Path: "../mock", Commits: []string{s.MockChange}, Languages: languages},
			{ID: "api", Path: "../api", Role: review.RoleContext, Commits: []string{s.APIHead}, Languages: languages},
		},
		Nodes: []*review.Node{
			{ID: "fn-a", Kind: "code", Label: "a()", Change: "modified", Ref: rangeRef("web", "src/a.ts", 1, 3), Why: "returns 10"},
			{ID: "fn-b", Kind: "code", Label: "b()", Change: "modified", Ref: rangeRef("web", "src/a.ts", 5, 7), Why: "returns 20"},
			{ID: "const-c", Kind: "code", Label: "c", Change: "modified", Ref: rangeRef("web", "src/a.ts", 9, 9), Why: "now 30"},
			{ID: "new", Kind: "code", Label: "new module", Change: "added", Ref: fileRef("web", "src/new.ts"), Why: "adds n and m"},
			{ID: "old", Kind: "code", Label: "old module", Change: "deleted", Ref: fileRef("web", "src/old.ts"), Why: "unused"},
			{ID: "ren", Kind: "code", Label: "renamed module", Change: "modified", Ref: fileRef("web", "src/renamed.ts"), Why: "clearer name"},
			{ID: "img", Kind: "asset", Label: "image", Change: "modified", Ref: fileRef("web", "img.png"), Why: "new art"},
			{ID: "untracked", Kind: "code", Label: "u", Change: "added", Ref: fileRef("web", "src/untracked.ts"), Why: "adds u"},
			{
				ID:     "t-a",
				Kind:   "test",
				Label:  "test a",
				Change: "modified",
				Ref:    rangeRef("web", "src/a.test.ts", 2, 4),
				Why:    "expects 10",
				Meta: review.OrderedMap{
					{Key: "command", Value: "vitest"},
					{Key: "observed", Value: "not run"},
				},
			},
			{ID: "t-b", Kind: "test", Label: "test b", Change: "modified", Ref: rangeRef("web", "src/a.test.ts", 5, 7), Why: "expects 2"},
			{ID: "mock-h", Kind: "code", Label: "mock handler", Change: "modified", Ref: rangeRef("mock", "handlers.ts", 1, 2), Why: "returns 2"},
			{ID: "api-route", Kind: "code", Label: "api route", Change: "unchanged", Ref: rangeRef("api", "routes.ts", 1, 2)},
			{ID: "ep", Kind: "endpoint", Label: "GET /x", Change: "modified", Why: "response changes"},
		},
		Edges: []*review.Edge{
			{From: "t-a", To: "fn-a", Kind: "tests", Ref: siteRef("web", "src/a.test.ts", 3, "expect(a())")},
			{From: "t-b", To: "fn-b", Kind: "tests", Ref: siteRef("web", "src/a.test.ts", 5, `test("b"`)},
			{From: "new", To: "fn-a", Kind: "calls", Ref: siteRef("web", "src/new.ts", 2, "a()")},
			{From: "mock-h", To: "ep", Kind: "mocks", Ref: siteRef("mock", "handlers.ts", 2, "GET /x")},
			{From: "api-route", To: "ep", Kind: "serves", Ref: siteRef("api", "routes.ts", 1, "GET /x")},
		},
		Flows: []*review.Flow{
			{ID: "call-a", Title: "new calls a", Path: []string{"new", "fn-a"}, Tests: []string{"t-a"}},
		},
		Claims: []*review.Claim{
			{
				Text:  "a returns 10",
				About: []string{"fn-a"},
				Evidence: []*review.Ref{
					{Type: review.RefHunk, Repo: "web", File: "src/a.ts", Lines: []int{2, 2}},
					{Type: review.RefCommit, Repo: "web", SHA: s.WebChange},
				},
			},
		},
		Notes: []string{"Fixture."},
	}
}
