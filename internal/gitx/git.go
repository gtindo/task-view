// Package gitx runs the fixed set of read-only Git subcommands change-view
// is allowed to use. Every call sets GIT_OPTIONAL_LOCKS=0 so nothing is
// written to the repositories.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// EmptyTree is the SHA-1 of the empty tree, the initial state of a change
// that starts at a root commit.
const EmptyTree = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// minStatusRecordLen is the shortest porcelain v1 record that names a path:
// two status letters, a space and at least one path byte.
const minStatusRecordLen = len("XY p")

// commitLogFields is the number of fields Log asks `git log` to print per
// commit: full SHA, short SHA, subject, author and author date.
const commitLogFields = 5

// Available reports whether a git executable is on PATH.
func Available() bool {
	_, err := exec.LookPath("git")

	return err == nil
}

// Repo runs Git in one repository directory.
type Repo struct {
	Dir string
}

func (r Repo) run(args ...string) ([]byte, error) {
	gitArgs := append([]string{"-c", "core.quotepath=off", "--no-pager"}, args...)
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return stdout.Bytes(), nil
}

// Toplevel returns the root of the repository containing dir.
func Toplevel(dir string) (string, error) {
	out, err := Repo{Dir: dir}.run("rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}

	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

// ResolveCommit returns the full SHA of a commit, or ok=false.
func (r Repo) ResolveCommit(rev string) (string, bool) {
	out, err := r.run("rev-parse", "--verify", "--quiet", rev+"^{commit}")
	if err != nil {
		return "", false
	}

	return strings.TrimSpace(string(out)), true
}

// FirstParent returns the first parent of a commit, or ok=false for a root.
func (r Repo) FirstParent(sha string) (string, bool) {
	out, err := r.run("rev-parse", "--verify", "--quiet", sha+"^1")
	if err != nil {
		return "", false
	}

	return strings.TrimSpace(string(out)), true
}

// Head returns the commit HEAD points at, or ok=false on an unborn branch.
func (r Repo) Head() (string, bool) {
	return r.ResolveCommit("HEAD")
}

// FirstParentList lists the commits of rangeSpec along first parents,
// newest first. A positive maxCount caps the list; zero lists them all.
func (r Repo) FirstParentList(rangeSpec string, maxCount int) ([]string, error) {
	args := []string{"rev-list", "--first-parent"}
	if maxCount > 0 {
		args = append(args, "--max-count="+strconv.Itoa(maxCount))
	}

	out, err := r.run(append(args, rangeSpec)...)
	if err != nil {
		return nil, err
	}

	return strings.Fields(string(out)), nil
}

// Exists reports whether path exists in a commit.
func (r Repo) Exists(sha, path string) bool {
	_, err := r.run("cat-file", "-e", sha+":"+path)

	return err == nil
}

// Show returns the content of path at a commit.
func (r Repo) Show(sha, path string) ([]byte, error) {
	return r.run("show", sha+":"+path)
}

// Diff returns the zero-context diff from one state to another. An empty
// `to` diffs against the working tree.
func (r Repo) Diff(from, to string) ([]byte, error) {
	args := []string{"diff", "--unified=0", "-M", "--no-color", "--no-ext-diff", from}
	if to != "" {
		args = append(args, to)
	}

	return r.run(append(args, "--")...)
}

// StatusEntry is one line of `git status --porcelain`.
type StatusEntry struct {
	Code string
	Path string
}

// Status lists uncommitted changes, untracked files included.
func (r Repo) Status() ([]StatusEntry, error) {
	out, err := r.run("status", "--porcelain=v1", "-z")
	if err != nil {
		return nil, err
	}

	var entries []StatusEntry

	// With -z, a rename or copy record is followed by a separate record
	// holding its source path, which is not an entry of its own.
	skipSourcePath := false

	for _, record := range strings.Split(string(out), "\x00") {
		if skipSourcePath {
			skipSourcePath = false

			continue
		}

		if len(record) < minStatusRecordLen {
			continue
		}

		entry := StatusEntry{Code: record[:2], Path: record[3:]}
		skipSourcePath = entry.Code[0] == 'R' || entry.Code[0] == 'C'
		entries = append(entries, entry)
	}

	return entries, nil
}

// Untracked lists untracked files that are not ignored.
func (r Repo) Untracked() ([]string, error) {
	out, err := r.run("ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}

	var files []string

	for _, path := range strings.Split(string(out), "\x00") {
		if path != "" {
			files = append(files, path)
		}
	}

	return files, nil
}

// CommitInfo describes one commit for the header.
type CommitInfo struct {
	SHA     string `json:"sha"`
	Short   string `json:"short"`
	Subject string `json:"subject"`
	Author  string `json:"author"`
	Date    string `json:"date"`
}

// Log returns commit details for the given SHAs, keyed by full SHA.
func (r Repo) Log(shas []string) (map[string]CommitInfo, error) {
	if len(shas) == 0 {
		return map[string]CommitInfo{}, nil
	}

	// Unit and record separators cannot appear in a subject or author name,
	// so they split the output safely.
	args := append([]string{"log", "--no-walk=unsorted", "--format=%H%x1f%h%x1f%s%x1f%an%x1f%aI%x1e"}, shas...)

	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}

	infos := map[string]CommitInfo{}

	for _, record := range strings.Split(string(out), "\x1e") {
		record = strings.TrimSpace(record)
		if record == "" {
			continue
		}

		fields := strings.Split(record, "\x1f")
		if len(fields) < commitLogFields {
			continue
		}

		infos[fields[0]] = CommitInfo{
			SHA:     fields[0],
			Short:   fields[1],
			Subject: fields[2],
			Author:  fields[3],
			Date:    fields[4],
		}
	}

	return infos, nil
}

// IsRepoRoot reports whether dir is the root of a Git work tree.
func IsRepoRoot(dir string) (bool, error) {
	top, err := Toplevel(dir)
	if err != nil {
		return false, err
	}

	// Git reports the physical path while dir may go through a symlink
	// (/tmp on macOS, for example), so compare resolved paths.
	resolvedTop, topErr := filepath.EvalSymlinks(top)
	resolvedDir, dirErr := filepath.EvalSymlinks(dir)

	if err := errors.Join(topErr, dirErr); err != nil {
		return false, err
	}

	return filepath.Clean(resolvedTop) == filepath.Clean(resolvedDir), nil
}
