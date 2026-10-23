package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/gtindo/changes-view/internal/fixture"
	"github.com/gtindo/changes-view/internal/langs"
	"github.com/gtindo/changes-view/internal/review"
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

	path := filepath.Join(set.Web, ".review", "change.json")
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
