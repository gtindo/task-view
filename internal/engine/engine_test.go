package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gtindo/task-view/internal/fixture"
	"github.com/gtindo/task-view/internal/langs"
	"github.com/gtindo/task-view/internal/review"
)

func TestParseDiff(t *testing.T) {
	out := `diff --git a/src/a.ts b/src/a.ts
index 1..2 100644
--- a/src/a.ts
+++ b/src/a.ts
@@ -2 +2 @@ export function a() {
-  return 1;
+  return 10;
@@ -9,0 +10,2 @@
+x
+y
@@ -12,2 +13,0 @@
--- removed line that starts with dashes
-z
diff --git a/old name.ts b/new name.ts
similarity index 100%
rename from old name.ts
rename to new name.ts
diff --git "a/caf\303\251.ts" "b/caf\303\251.ts"
new file mode 100644
--- /dev/null
+++ "b/caf\303\251.ts"
@@ -0,0 +1 @@
+hi
diff --git a/img.png b/img.png
index 1..2 100644
Binary files a/img.png and b/img.png differ
diff --git a/gone.ts b/gone.ts
deleted file mode 100644
--- a/gone.ts
+++ /dev/null
@@ -1,2 +0,0 @@
-a
-b
diff --git a/mode.sh b/mode.sh
old mode 100644
new mode 100755
`
	files := ParseDiff(out)
	if len(files) != 5 {
		t.Fatalf("got %d files, want 5 (mode-only change dropped)", len(files))
	}

	modified := files[0]
	if modified.Path != "src/a.ts" || len(modified.Hunks) != 3 || modified.Added() != 3 || modified.Removed() != 3 {
		t.Errorf("a.ts parsed wrong: %+v", modified)
	}

	if deletion := modified.Hunks[2]; !deletion.PureDeletion() || deletion.OldStart != 12 || deletion.OldCount != 2 || deletion.Old[0] != "-- removed line that starts with dashes" {
		t.Errorf("pure deletion parsed wrong: %+v", deletion)
	}

	if renamed := files[1]; renamed.Status != DiffRenamed || renamed.OldPath != "old name.ts" || renamed.Path != "new name.ts" {
		t.Errorf("rename parsed wrong: %+v", renamed)
	}

	if quoted := files[2]; quoted.Status != DiffAdded || quoted.Path != "café.ts" {
		t.Errorf("quoted path parsed wrong: %+v", quoted)
	}

	if binary := files[3]; !binary.Binary || binary.Path != "img.png" {
		t.Errorf("binary parsed wrong: %+v", binary)
	}

	if deleted := files[4]; deleted.Status != DiffDeleted || deleted.Key() != "gone.ts" || deleted.Removed() != 2 {
		t.Errorf("deletion parsed wrong: %+v", deleted)
	}
}

func TestSplitLines(t *testing.T) {
	got := SplitLines([]byte("a\r\nb\nc\n"))
	if len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("got %q", got)
	}

	if len(SplitLines([]byte("a\nb"))) != 2 || SplitLines(nil) != nil {
		t.Error("final line without break")
	}
}

func TestGlobs(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**/*.ts", "a.ts", true},
		{"**/*.ts", "src/deep/a.ts", true},
		{"**/*.ts", "src/a.tsx", false},
		{"src/*.ts", "src/a.ts", true},
		{"src/*.ts", "src/x/a.ts", false},
		{"**/*.{ts,tsx}", "x/y.tsx", true},
		{"Makefile", "Makefile", true},
		{"docs/**", "docs/a/b.md", true},
	}
	for _, testCase := range cases {
		if got := langs.MatchGlob(testCase.pattern, testCase.path); got != testCase.want {
			t.Errorf("MatchGlob(%q, %q) = %v", testCase.pattern, testCase.path, got)
		}
	}
}

func TestWorktreeChanged(t *testing.T) {
	set := fixture.Build(t)
	data, err := json.Marshal(set.Review())
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(set.Web, ".review", "change-fixture.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	beforeWorktreeCheck = func() {
		edited := []byte("changed while checking, longer content\n")
		if err := os.WriteFile(filepath.Join(set.Web, "src", "a.ts"), edited, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { beforeWorktreeCheck = nil }()

	run := Run(path, Options{})
	found := slices.ContainsFunc(run.Findings, func(finding review.Finding) bool {
		return finding.Code == "worktree_changed" && finding.File == "src/a.ts"
	})
	if !found {
		t.Fatalf("no worktree_changed finding: %+v", run.Findings)
	}
}

func TestRepoIDs(t *testing.T) {
	got := repoIDs([]string{"/a/web", "/b/web", "/c/My_App.v2", "/d/---", "/e/web-2"})
	want := []string{"web", "web-2", "my-app-v2", "repo", "web-2-2"}
	if !slices.Equal(got, want) {
		t.Errorf("repoIDs = %v, want %v", got, want)
	}
}

func TestLanguageGlobs(t *testing.T) {
	languageOf := func(file string) string {
		switch {
		case filepath.Base(file) == "CMakeLists.txt":
			return "cmake"
		case filepath.Base(file) == "Makefile":
			return "makefile"
		case filepath.Ext(file) == ".txt":
			return "plaintext"
		case filepath.Ext(file) == ".go":
			return "go"
		}

		return ""
	}

	got, err := json.Marshal(languageGlobs([]string{"a/main.go", "b.go", "CMakeLists.txt", "notes.txt", "build/Makefile", "unknown.xyz"}, languageOf))
	if err != nil {
		t.Fatal(err)
	}

	want := `{"**/CMakeLists.txt":"cmake","**/Makefile":"makefile","**/*.go":"go","**/*.txt":"plaintext"}`
	if string(got) != want {
		t.Errorf("languageGlobs = %s, want %s", got, want)
	}
}

func TestContextWindow(t *testing.T) {
	lines := []string{"1", "2", "3", "4", "5"}
	cases := []struct {
		first, last, size int
		wantStart         int
		wantLines         []string
	}{
		{3, 3, 1, 2, []string{"2", "3", "4"}},
		{1, 2, 3, 1, []string{"1", "2", "3", "4", "5"}},
		// A pure deletion after line 2: the empty span 3-2.
		{3, 2, 1, 2, []string{"2", "3"}},
	}
	for _, testCase := range cases {
		got := contextWindow(lines, testCase.first, testCase.last, testCase.size)
		if got == nil || got.Start != testCase.wantStart || !slices.Equal(got.Lines, testCase.wantLines) {
			t.Errorf("contextWindow(%d, %d, %d) = %+v", testCase.first, testCase.last, testCase.size, got)
		}
	}

	if got := contextWindow(nil, 1, 0, 3); got != nil {
		t.Errorf("empty file gave %+v", got)
	}
}

func TestHunkEvidenceLines(t *testing.T) {
	limits := excerptLimits{contextLines: 0, maxHunkLines: 10}
	finalLines := []string{"x", "y", "z"}

	topDeletion := &Hunk{OldStart: 1, OldCount: 2, NewStart: 0, Old: []string{"a", "b"}}
	got := newHunkEvidence(DiffModified, topDeletion, finalLines, limits)
	if !slices.Equal(got.Lines, []int{1, 1}) || got.Context != nil {
		t.Errorf("deletion at the top: lines %v, context %+v", got.Lines, got.Context)
	}

	if emptied := newHunkEvidence(DiffModified, topDeletion, nil, limits); emptied.Lines != nil {
		t.Errorf("emptied file has lines %v; its file ref covers it", emptied.Lines)
	}

	deleted := newHunkEvidence(DiffDeleted, topDeletion, nil, limits)
	if deleted.Lines != nil || !slices.Equal(deleted.OldLines, []int{1, 2}) {
		t.Errorf("deleted file: lines %v, old lines %v", deleted.Lines, deleted.OldLines)
	}

	replaced := &Hunk{OldStart: 2, OldCount: 1, NewStart: 2, NewCount: 1, Old: []string{"b"}, New: []string{"y"}}
	withContext := newHunkEvidence(DiffModified, replaced, finalLines, excerptLimits{contextLines: 1, maxHunkLines: 10})
	if withContext.Context == nil || withContext.Context.Start != 1 || len(withContext.Context.Lines) != 3 {
		t.Errorf("context asked for: %+v", withContext.Context)
	}
}

func TestAttributeBlankLines(t *testing.T) {
	text := map[int]string{10: "", 11: "  \t", 12: "x", 20: "", 30: ""}
	textAt := func(line int) string { return text[line] }
	spans := []span{{start: 1, end: 9, node: "above"}, {start: 31, end: 40, node: "below"}}

	cases := []struct {
		name      string
		lines     []int
		remaining []int
		owners    []string
	}{
		{"blank run after a node", []int{10, 11}, nil, []string{"above"}},
		{"blank line before a node", []int{30}, nil, []string{"below"}},
		{"blank line with no node beside it", []int{20}, []int{20}, nil},
		{"run with a non-blank line", []int{10, 11, 12}, []int{10, 11, 12}, nil},
	}
	for _, testCase := range cases {
		remaining, owners := attributeBlankLines(testCase.lines, textAt, spans)
		ownerNodes := make([]string, 0, len(owners))
		for _, owner := range owners {
			ownerNodes = append(ownerNodes, owner.node)
		}

		if !slices.Equal(remaining, testCase.remaining) || !slices.Equal(ownerNodes, testCase.owners) {
			t.Errorf("%s: remaining %v owners %v, want %v %v", testCase.name, remaining, ownerNodes, testCase.remaining, testCase.owners)
		}
	}
}
