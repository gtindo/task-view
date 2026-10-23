package engine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gtindo/changes-view/internal/fixture"
	"github.com/gtindo/changes-view/internal/langs"
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
	a := files[0]
	if a.Path != "src/a.ts" || len(a.Hunks) != 3 || a.Added() != 3 || a.Removed() != 3 {
		t.Errorf("a.ts parsed wrong: %+v", a)
	}
	if h := a.Hunks[2]; !h.PureDeletion() || h.OldStart != 12 || h.OldCount != 2 || h.Old[0] != "-- removed line that starts with dashes" {
		t.Errorf("pure deletion parsed wrong: %+v", h)
	}
	if r := files[1]; r.Status != DiffRenamed || r.OldPath != "old name.ts" || r.Path != "new name.ts" {
		t.Errorf("rename parsed wrong: %+v", r)
	}
	if u := files[2]; u.Status != DiffAdded || u.Path != "café.ts" {
		t.Errorf("quoted path parsed wrong: %+v", u)
	}
	if b := files[3]; !b.Binary || b.Path != "img.png" {
		t.Errorf("binary parsed wrong: %+v", b)
	}
	if d := files[4]; d.Status != DiffDeleted || d.Key() != "gone.ts" || d.Removed() != 2 {
		t.Errorf("deletion parsed wrong: %+v", d)
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
	for _, c := range cases {
		if got := langs.MatchGlob(c.pattern, c.path); got != c.want {
			t.Errorf("MatchGlob(%q, %q) = %v", c.pattern, c.path, got)
		}
	}
}

func TestWorktreeChanged(t *testing.T) {
	s := fixture.Build(t)
	data, _ := json.Marshal(s.Review())
	path := filepath.Join(s.Web, ".review", "change.json")
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, data, 0o644)

	beforeWorktreeCheck = func() {
		os.WriteFile(filepath.Join(s.Web, "src", "a.ts"), []byte("changed while checking, longer content\n"), 0o644)
	}
	defer func() { beforeWorktreeCheck = nil }()

	e := Run(path, Options{})
	found := false
	for _, f := range e.Findings {
		if f.Code == "worktree_changed" && f.File == "src/a.ts" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no worktree_changed finding: %+v", e.Findings)
	}
}
