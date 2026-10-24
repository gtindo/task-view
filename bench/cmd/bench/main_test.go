package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gtindo/task-view/bench/internal/config"
	"github.com/gtindo/task-view/bench/internal/replay"
)

// gitEnv pins identity and dates so the fixture repo builds the same way on
// every machine.
var gitEnv = []string{
	"GIT_AUTHOR_NAME=Fixture", "GIT_AUTHOR_EMAIL=fixture@example.com",
	"GIT_COMMITTER_NAME=Fixture", "GIT_COMMITTER_EMAIL=fixture@example.com",
	"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	"GIT_CONFIG_NOSYSTEM=1",
}

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := exec.Command("git", append([]string{"-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), gitEnv...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}

	return strings.TrimSpace(string(output))
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

// stubAdapter stands in for an agent: it writes an accurate review of the
// fixture change, validates it once through the wrapper on its PATH, and
// reports usage in the generic format.
const stubAdapter = `#!/bin/sh
set -eu
mkdir -p .review
cat > .review/change-demo-small.json <<'EOF'
{"version": 1, "summary": "Adds b.txt.",
 "repos": [{"id": "demo", "path": ".", "commits": ["WORKTREE"], "languages": {"**/*.txt": "plaintext"}}],
 "nodes": [{"id": "b", "kind": "config", "label": "b.txt", "change": "added",
            "ref": {"type": "file", "repo": "demo", "file": "b.txt"}, "why": "adds b"}]}
EOF
task-view validate .review/change-demo-small.json --format json > /dev/null
printf '{"input_tokens": 1000, "output_tokens": 2000, "turns": 3, "model": "stub-model"}' > "$BENCH_OUT/usage.json"
`

func TestPrepareRunReport(t *testing.T) {
	if testing.Short() {
		t.Skip("builds task-view and runs git")
	}

	tmp := t.TempDir()
	taskView := filepath.Join(tmp, "task-view")
	build := exec.Command("go", "build", "-o", taskView, "./cmd/task-view")
	build.Dir = filepath.Join("..", "..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build task-view: %v\n%s", err, output)
	}

	project := filepath.Join(tmp, "demo")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	gitIn(t, project, "init", "-q")
	writeFile(t, filepath.Join(project, "a.txt"), "a\n", 0o644)
	gitIn(t, project, "add", "-A")
	gitIn(t, project, "commit", "-q", "-m", "base")
	base := gitIn(t, project, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(project, "b.txt"), "b\n", 0o644)
	gitIn(t, project, "add", "-A")
	gitIn(t, project, "commit", "-q", "-m", "add b")
	tip := gitIn(t, project, "rev-parse", "HEAD")
	gitIn(t, project, "tag", "later")

	root := filepath.Join(tmp, "bench")
	writeFile(t, filepath.Join(root, "cases.json"), `{"projects": [{"id": "demo", "repo": "`+project+`", "language": "Text", "license": "MIT"}],
 "cases": [{"id": "demo-small", "project": "demo", "size": "small", "title": "Add b", "base": "`+base+`", "commits": ["`+tip+`"], "uncommitted_tip": true}]}`, 0o644)
	writeFile(t, filepath.Join(root, "briefs", "demo-small.md"), "## What was asked\n\nAdd b.txt.\n", 0o644)
	writeFile(t, filepath.Join(root, "prompt.md"), "{{brief}}\nUse {{task_view}}; write {{review_file}}. {{state}}\n", 0o644)
	writeFile(t, filepath.Join(root, "matrix.json"), `{"harness": "harness/stub", "models": [{"id": "stub", "label": "Stub", "price": "stub"}],
 "efforts": ["medium"], "repeats": 2, "parallel": 2, "cases": ["*"], "timeout": "2m"}`, 0o644)
	writeFile(t, filepath.Join(root, "prices.json"), `{"as_of": "test", "models": {"stub": {"input": 1, "output": 10, "cache_read": 0, "cache_write": 0}}}`, 0o644)
	writeFile(t, filepath.Join(root, "harness", "stub", "harness.json"), `{"name": "stub", "command": "run.sh", "usage_format": "usage-json", "usage_file": "usage.json", "version_command": "echo stub 1.2"}`, 0o644)
	writeFile(t, filepath.Join(root, "harness", "stub", "run.sh"), stubAdapter, 0o755)

	if err := cmdPrepare([]string{"-root", root, "-task-view", taskView}); err != nil {
		t.Fatalf("prepare: %v", err)
	}

	out := filepath.Join(root, ".cache", "results", "test")
	runArgs := []string{"-root", root, "-out", out, "-task-view", taskView}
	if err := cmdRun(runArgs); err != nil {
		t.Fatalf("run: %v", err)
	}

	results, err := readResults(filepath.Join(out, "runs.jsonl"))
	if err != nil || len(results) != 2 {
		t.Fatalf("got %d results, %v", len(results), err)
	}

	result := results[0]
	if result.HarnessError != "" || result.WorkTouched {
		t.Fatalf("run failed: %+v", result)
	}

	if !result.Final.Accurate() || result.Calls.Validate != 1 || result.Change.Added != 1 || result.Review.Nodes != 1 {
		t.Errorf("result: final %+v, calls %+v, change %+v, review %+v", result.Final, result.Calls, result.Change, result.Review)
	}

	if result.CostSource != "prices" || result.CostUSD <= 0 {
		t.Errorf("cost %v from %q, want a computed cost", result.CostUSD, result.CostSource)
	}

	var prompt []byte
	prompt, err = os.ReadFile(filepath.Join(out, "runs", result.RunID, "prompt.md"))
	if err != nil || !strings.Contains(string(prompt), "None of the work is committed yet") || !strings.Contains(string(prompt), "Add b.txt.") {
		t.Errorf("prompt: %s, %v", prompt, err)
	}

	// A second run of the same command finds everything done.
	if err := cmdRun(runArgs); err != nil {
		t.Fatalf("resume: %v", err)
	}

	if again, _ := readResults(filepath.Join(out, "runs.jsonl")); len(again) != 2 {
		t.Errorf("resume added runs: %d results", len(again))
	}

	if err := cmdReport([]string{out}); err != nil {
		t.Fatalf("report: %v", err)
	}

	markdown, err := os.ReadFile(filepath.Join(out, "report.md"))
	if err != nil || !strings.Contains(string(markdown), "| demo-small | small | Stub | medium | 2 | 2/2 | 2/2 |") {
		t.Errorf("report:\n%s", markdown)
	}

	var meta map[string]any
	data, _ := os.ReadFile(filepath.Join(out, "meta.json"))
	if json.Unmarshal(data, &meta) != nil || meta["harness"] != "stub" || meta["harness_version"] != "stub 1.2" {
		t.Errorf("meta: %s", data)
	}

	if _, err := os.Stat(filepath.Join(out, "runs", result.RunID, "review.html")); err != nil {
		t.Errorf("no page rendered: %v", err)
	}

	if err := cmdPublish([]string{"-root", root, out}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	bench, err := os.ReadFile(filepath.Join(tmp, "BENCH.md"))
	if err != nil || !strings.Contains(string(bench), "## Choosing a model") || !strings.Contains(string(bench), "bench/results/reviews.tar.xz") {
		t.Errorf("BENCH.md:\n%s", bench)
	}

	listing, err := exec.Command("tar", "-tJf", filepath.Join(root, "results", archiveName)).Output()
	wantEntry := "reviews/" + result.RunID + "/review.html"
	if err != nil || !strings.Contains(string(listing), wantEntry) || !strings.Contains(string(listing), "change-demo-small.json") {
		t.Errorf("archive lacks %s: %s, %v", wantEntry, listing, err)
	}
}

// A run whose latest result is a harness failure is retried on resume; one
// that failed and then succeeded is done.
func TestDoneRunsRetriesFailures(t *testing.T) {
	file := filepath.Join(t.TempDir(), "runs.jsonl")
	writeFile(t, file, `{"run_id": "a", "harness_error": "usage limit"}
{"run_id": "b", "harness_error": "usage limit"}
{"run_id": "b"}
{"run_id": "c"}
`, 0o644)

	done, err := doneRuns(file)
	if err != nil {
		t.Fatal(err)
	}

	if done["a"] || !done["b"] || !done["c"] {
		t.Errorf("done = %v, want b and c only", done)
	}
}

// The work dir holds the task and nothing after it: the tip's change is
// uncommitted, and later tags and branches are gone.
func TestWorkdirHidesTheFuture(t *testing.T) {
	tmp := t.TempDir()
	project := filepath.Join(tmp, "p")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}

	gitIn(t, project, "init", "-q")
	writeFile(t, filepath.Join(project, "a.txt"), "a\n", 0o644)
	gitIn(t, project, "add", "-A")
	gitIn(t, project, "commit", "-q", "-m", "base")
	writeFile(t, filepath.Join(project, "a.txt"), "a2\n", 0o644)
	gitIn(t, project, "commit", "-q", "-am", "later")
	gitIn(t, project, "tag", "v-later")

	work := filepath.Join(tmp, "work")
	bare := filepath.Join(tmp, "p.git")
	gitIn(t, tmp, "clone", "--bare", "-q", project, bare)
	base := gitIn(t, project, "rev-parse", "HEAD~1")
	tip := gitIn(t, project, "rev-parse", "HEAD")

	task := config.Case{Base: base, Commits: []string{tip}, UncommittedTip: true}
	if err := replay.Workdir(bare, work, task); err != nil {
		t.Fatal(err)
	}

	if refs := gitIn(t, work, "for-each-ref", "--format=%(refname)"); refs != "refs/heads/task" {
		t.Errorf("refs left in the work dir: %q", refs)
	}

	if status := gitIn(t, work, "status", "--porcelain"); status != "M a.txt" {
		t.Errorf("status %q, want the tip's change uncommitted", status)
	}
}
