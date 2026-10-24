// Package replay prepares the Git state each run works in: a cached clone
// per project, checks that every case still matches its repo, and a fresh
// work dir holding the task as an agent leaves it when it finishes.
package replay

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gtindo/task-view/bench/internal/config"
)

// Size class limits on changed lines (added plus removed) for a whole
// task. They match the classes the suite's cases claim.
const (
	smallMaxLines  = 80
	mediumMaxLines = 500
	bigMaxLines    = 2000
)

// SizeOf is the size class a task of this many changed lines belongs to.
func SizeOf(lines int) string {
	switch {
	case lines <= smallMaxLines:
		return "small"
	case lines <= mediumMaxLines:
		return "medium"
	case lines <= bigMaxLines:
		return "big"
	}

	return "agent-scale"
}

func git(dir string, stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.String(), nil
}

// EnsureClone returns the project's bare clone under cacheDir, cloning it
// on first use and fetching again only when one of shas is missing.
func EnsureClone(cacheDir string, project config.Project, shas []string) (string, error) {
	repo := filepath.Join(cacheDir, project.ID+".git")
	if _, err := os.Stat(repo); err != nil {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return "", err
		}

		if _, err := git(cacheDir, nil, "clone", "--bare", "--quiet", project.Repo, repo); err != nil {
			return "", err
		}
	}

	missing := slices.ContainsFunc(shas, func(sha string) bool {
		_, err := git(repo, nil, "cat-file", "-e", sha+"^{commit}")

		return err != nil
	})
	if missing {
		if _, err := git(repo, nil, "fetch", "--quiet", "origin", "+refs/heads/*:refs/heads/*"); err != nil {
			return "", err
		}
	}

	return repo, nil
}

// CaseCheck is what prepare found about one case.
type CaseCheck struct {
	Case     string
	Files    int
	Lines    int
	Problems []string // the case cannot run as written
	Warnings []string // it runs, but does not match its description
}

// Check verifies a case against its clone: every SHA exists, the commits
// are exactly the first-parent chain after the base, and the change's size
// fits the class the case claims.
func Check(repo string, task config.Case) CaseCheck {
	check := CaseCheck{Case: task.ID}
	chain, err := git(repo, nil, "rev-list", "--first-parent", task.Base+".."+task.Tip())
	if err != nil {
		check.Problems = append(check.Problems, err.Error())

		return check
	}

	if got := strings.Fields(chain); !slices.Equal(got, task.Commits) {
		check.Problems = append(check.Problems,
			fmt.Sprintf("commits are not the first-parent chain after the base: git lists %v", got))
	}

	numstat, err := git(repo, nil, "diff", "--numstat", task.Base, task.Tip())
	if err != nil {
		check.Problems = append(check.Problems, err.Error())

		return check
	}

	check.Files, check.Lines = sumNumstat(numstat)
	if size := SizeOf(check.Lines); size != task.Size {
		check.Warnings = append(check.Warnings,
			fmt.Sprintf("%d changed lines read as %s, but the case says %s", check.Lines, size, task.Size))
	}

	return check
}

// sumNumstat counts files and changed lines in `git diff --numstat`
// output; binary files count as files with no lines.
func sumNumstat(numstat string) (int, int) {
	files, lines := 0, 0
	for _, row := range strings.Split(strings.TrimSpace(numstat), "\n") {
		fields := strings.Fields(row)
		if len(fields) < 3 {
			continue
		}

		files++
		added, _ := strconv.Atoi(fields[0])
		removed, _ := strconv.Atoi(fields[1])
		lines += added + removed
	}

	return files, lines
}

// Workdir builds the task's end state in dir from the project's clone: a
// branch `task` holding the commits, and, with UncommittedTip, the newest
// commit's change applied to the working tree instead (new files left
// untracked). Remotes, other branches and tags are removed so the work dir
// holds nothing but the task, and `.review/` is ignored as the instructions ask.
func Workdir(repo, dir string, task config.Case) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}

	if _, err := git(filepath.Dir(repo), nil, "clone", "--quiet", "--no-checkout", repo, dir); err != nil {
		return err
	}

	start := task.Tip()
	if task.UncommittedTip {
		start = task.Base
		if len(task.Commits) > 1 {
			start = task.Commits[1]
		}
	}

	steps := [][]string{
		{"checkout", "--quiet", "-B", "task", start},
		{"remote", "remove", "origin"},
		{"config", "user.name", "Benchmark agent"},
		{"config", "user.email", "agent@bench.invalid"},
	}
	for _, step := range steps {
		if _, err := git(dir, nil, step...); err != nil {
			return err
		}
	}

	if err := dropOtherRefs(dir); err != nil {
		return err
	}

	if task.UncommittedTip {
		patch, err := git(dir, nil, "diff", "--binary", start, task.Tip())
		if err != nil {
			return err
		}

		if _, err := git(dir, []byte(patch), "apply", "--whitespace=nowarn"); err != nil {
			return err
		}
	}

	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}

	return os.WriteFile(exclude, []byte(".review/\n"), 0o644)
}

// dropOtherRefs deletes every branch but `task` and every tag, since tags
// point at commits made after the task and would show its future.
func dropOtherRefs(dir string) error {
	refs, err := git(dir, nil, "for-each-ref", "--format=%(refname)", "refs/heads", "refs/tags")
	if err != nil {
		return err
	}

	for _, ref := range strings.Fields(refs) {
		if ref == "refs/heads/task" {
			continue
		}

		if _, err := git(dir, nil, "update-ref", "-d", ref); err != nil {
			return err
		}
	}

	return nil
}

// WorkState fingerprints the work dir's code, the review folder left out,
// so the runner can tell whether the agent changed the task's change.
func WorkState(dir string) (string, error) {
	status, err := git(dir, nil, "status", "--porcelain=v1", "-uall")
	if err != nil {
		return "", err
	}

	diff, err := git(dir, nil, "diff", "HEAD")
	if err != nil {
		return "", err
	}

	head, err := git(dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}

	var kept []string
	for _, line := range strings.Split(status, "\n") {
		if !strings.Contains(line, ".review/") {
			kept = append(kept, line)
		}
	}

	sum := sha256.Sum256([]byte(head + strings.Join(kept, "\n") + diff))

	return hex.EncodeToString(sum[:]), nil
}

// BuildTaskView builds the task-view binary from the repo at root into
// out and returns the version it was stamped with: `git describe` of the
// checkout, `-dirty` when it has uncommitted changes.
func BuildTaskView(root, out string) (string, error) {
	described, err := git(root, nil, "describe", "--always", "--dirty", "--tags")
	if err != nil {
		return "", err
	}

	version := strings.TrimSpace(described)
	cmd := exec.Command("go", "build", "-ldflags", "-X main.version="+version, "-o", out, "./cmd/task-view")
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build: %w: %s", err, output)
	}

	return version, nil
}
