package engine

import (
	"os"
	"path/filepath"

	"github.com/gtindo/changes-view/internal/review"
)

// attribute records, for every final-state line the change adds, the commit
// (or WORKTREE) that last wrote it, by replaying each step's diff in order.
// It only runs when the change spans more than one state.
func (e *Engine) attribute(s *RepoState) {
	steps := make([]string, 0, len(s.Commits)+1)
	for i := len(s.Commits) - 1; i >= 0; i-- {
		steps = append(steps, s.Commits[i])
	}
	if s.Worktree {
		steps = append(steps, review.Worktree)
	}
	if len(steps) < 2 {
		return
	}

	owners := map[string]map[int]string{}
	from := s.Initial
	for _, step := range steps {
		to := step
		if step == review.Worktree {
			to = ""
		}
		out, err := s.Git.Diff(from, to)
		if err != nil {
			return
		}
		files := ParseDiff(string(out))
		if step == review.Worktree {
			if untracked, err := s.Git.Untracked(); err == nil {
				for _, p := range untracked {
					data, err := os.ReadFile(filepath.Join(s.Abs, filepath.FromSlash(p)))
					if err != nil || IsBinary(data) {
						continue
					}
					if n := len(SplitLines(data)); n > 0 {
						files = append(files, &FileDiff{Path: p, Status: DiffAdded, Hunks: []*Hunk{{NewStart: 1, NewCount: n}}})
					}
				}
			}
		}
		for _, fd := range files {
			prev := owners[fd.OldPath]
			if fd.OldPath == "" {
				prev = nil
			}
			next := map[int]string{}
			for line, owner := range prev {
				if n, ok := shiftLine(fd.Hunks, line); ok {
					next[n] = owner
				}
			}
			for _, h := range fd.Hunks {
				for l := h.NewStart; l <= h.NewEnd(); l++ {
					next[l] = step
				}
			}
			if fd.OldPath != "" {
				delete(owners, fd.OldPath)
			}
			if fd.Path != "" {
				owners[fd.Path] = next
			}
		}
		if step != review.Worktree {
			from = step
		}
	}
	s.owners = owners
}

// shiftLine maps an unchanged old line through a diff; ok is false when
// the diff replaces or removes it.
func shiftLine(hunks []*Hunk, line int) (int, bool) {
	shift := 0
	for _, h := range hunks {
		if h.OldCount > 0 {
			if line >= h.OldStart && line <= h.OldEnd() {
				return 0, false
			}
			if line > h.OldEnd() {
				shift += h.NewCount - h.OldCount
			}
		} else if line > h.OldStart {
			shift += h.NewCount
		}
	}
	return line + shift, true
}

// HunkCommits lists the states that wrote a hunk's added lines, newest
// first; empty for single-state changes and pure deletions.
func (s *RepoState) HunkCommits(path string, h *Hunk) []string {
	m := s.owners[path]
	if m == nil {
		return nil
	}
	seen := map[string]bool{}
	for l := h.NewStart; l <= h.NewEnd(); l++ {
		if o := m[l]; o != "" {
			seen[o] = true
		}
	}
	var out []string
	if seen[review.Worktree] {
		out = append(out, review.Worktree)
	}
	for _, c := range s.Commits {
		if seen[c] {
			out = append(out, c)
		}
	}
	return out
}
