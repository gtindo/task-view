package engine

import (
	"os"
	"slices"

	"github.com/gtindo/changes-view/internal/review"
)

// lineOwners maps a final path to, per line, the state (commit or
// WORKTREE) that last wrote that line.
type lineOwners map[string]map[int]string

// attribute records, for every final-state line the change adds, the commit
// (or WORKTREE) that last wrote it, by replaying each step's diff in order.
// It only runs when the change spans more than one state.
func (s *RepoState) attribute() {
	steps := slices.Clone(s.Commits)
	slices.Reverse(steps)
	if s.Worktree {
		steps = append(steps, review.Worktree)
	}

	if len(steps) < 2 {
		return
	}

	owners := lineOwners{}
	from := s.Initial
	for _, step := range steps {
		files, err := s.stepDiff(from, step)
		if err != nil {
			return
		}

		owners.replay(files, step)
		if step != review.Worktree {
			from = step
		}
	}

	s.owners = owners
}

// stepDiff is the diff one step of the change made on top of from. A
// WORKTREE step diffs the working tree and includes untracked files.
func (s *RepoState) stepDiff(from, step string) ([]*FileDiff, error) {
	to := step
	if step == review.Worktree {
		to = ""
	}

	out, err := s.Git.Diff(from, to)
	if err != nil {
		return nil, err
	}

	files := ParseDiff(string(out))
	if step != review.Worktree {
		return files, nil
	}

	return append(files, s.untrackedAdditions()...), nil
}

// untrackedAdditions lists untracked text files as whole-file additions.
// Attribution is best effort, so files that cannot be listed or read are
// left out instead of failing the run.
func (s *RepoState) untrackedAdditions() []*FileDiff {
	untracked, err := s.Git.Untracked()
	if err != nil {
		return nil
	}

	var additions []*FileDiff
	for _, file := range untracked {
		data, err := os.ReadFile(s.diskPath(file))
		if err != nil {
			continue
		}

		addition := untrackedDiff(file, data)
		if len(addition.Hunks) == 0 {
			continue
		}

		additions = append(additions, addition)
	}

	return additions
}

// replay applies one step's file diffs: lines the step leaves alone keep
// their owner at their shifted position, and lines it adds belong to step.
func (o lineOwners) replay(files []*FileDiff, step string) {
	for _, fileDiff := range files {
		next := map[int]string{}
		for line, owner := range o[fileDiff.OldPath] {
			if shifted, ok := shiftLine(fileDiff.Hunks, line); ok {
				next[shifted] = owner
			}
		}

		for _, hunk := range fileDiff.Hunks {
			for line := hunk.NewStart; line <= hunk.NewEnd(); line++ {
				next[line] = step
			}
		}

		if fileDiff.OldPath != "" {
			delete(o, fileDiff.OldPath)
		}

		if fileDiff.Path != "" {
			o[fileDiff.Path] = next
		}
	}
}

// shiftLine maps an unchanged old line through a diff; ok is false when
// the diff replaces or removes it.
func shiftLine(hunks []*Hunk, line int) (int, bool) {
	shift := 0
	for _, hunk := range hunks {
		// A pure insertion sits after OldStart and removes nothing.
		if hunk.OldCount == 0 {
			if line > hunk.OldStart {
				shift += hunk.NewCount
			}

			continue
		}

		replaced := line >= hunk.OldStart && line <= hunk.OldEnd()
		if replaced {
			return 0, false
		}

		if line > hunk.OldEnd() {
			shift += hunk.NewCount - hunk.OldCount
		}
	}

	return line + shift, true
}

// HunkCommits lists the states that wrote a hunk's added lines, newest
// first; empty for single-state changes and pure deletions.
func (s *RepoState) HunkCommits(path string, hunk *Hunk) []string {
	fileOwners := s.owners[path]
	if fileOwners == nil {
		return nil
	}

	writers := map[string]bool{}
	for line := hunk.NewStart; line <= hunk.NewEnd(); line++ {
		if owner := fileOwners[line]; owner != "" {
			writers[owner] = true
		}
	}

	var commits []string
	if writers[review.Worktree] {
		commits = append(commits, review.Worktree)
	}

	for _, commit := range s.Commits {
		if writers[commit] {
			commits = append(commits, commit)
		}
	}

	return commits
}
