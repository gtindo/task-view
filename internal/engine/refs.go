package engine

import (
	"fmt"
	"strings"

	"github.com/gtindo/changes-view/internal/review"
)

// RefResult is the outcome of checking one ref.
type RefResult struct {
	Status     string   `json:"status"`
	Codes      []string `json:"codes,omitempty"`
	Reason     string   `json:"reason,omitempty"`
	Hint       string   `json:"hint,omitempty"`
	State      string   `json:"state,omitempty"`     // SHA or WORKTREE that was read
	StateKind  string   `json:"stateKind,omitempty"` // final, initial or at
	ActualLine int      `json:"actualLine,omitempty"`
	Language   string   `json:"language,omitempty"`
}

type refCheck struct {
	e    *Engine
	ref  *review.Ref
	path string
	node *review.Node
	res  *RefResult
	repo *RepoState
}

func (c *refCheck) fail(code, msg, hint string) {
	f := review.NewFinding(code, c.path, msg, hint)
	f.Repo = c.ref.Repo
	f.File = c.ref.File
	if c.ref.Start() > 0 {
		f.Lines = []int{c.ref.Start(), c.ref.End()}
	}
	c.e.Findings = append(c.e.Findings, f)
	c.res.Codes = append(c.res.Codes, code)
	worse := f.Status
	if c.res.Status == review.StatusVerified || (c.res.Status == review.StatusContradicted && worse == review.StatusUnresolved) {
		c.res.Status = worse
		c.res.Reason = msg
		c.res.Hint = hint
	}
}

func (c *refCheck) warn(code, msg, hint string) {
	f := review.NewFinding(code, c.path, msg, hint)
	f.Repo = c.ref.Repo
	f.File = c.ref.File
	if c.ref.Start() > 0 {
		f.Lines = []int{c.ref.Start(), c.ref.End()}
	}
	c.e.Findings = append(c.e.Findings, f)
	c.res.Codes = append(c.res.Codes, code)
}

// checkRef resolves one ref; node is set for node refs, whose change
// state is checked against the diff.
func (e *Engine) checkRef(ref *review.Ref, path string, node *review.Node) *RefResult {
	res := &RefResult{Status: review.StatusVerified}
	repo := e.repoByID[ref.Repo]
	c := &refCheck{e: e, ref: ref, path: path, node: node, res: res, repo: repo}

	if repo == nil || !repo.OK {
		res.Status = review.StatusUnresolved
		res.Reason = fmt.Sprintf("repo %q could not be resolved; fix its commits first", ref.Repo)
		return res
	}
	if ref.File != "" {
		res.Language = repo.Language(ref.File, ref.Language)
	}

	state := StateFinal
	res.StateKind = "final"
	if ref.At != "" {
		found := false
		for _, sha := range repo.Commits {
			if sha == ref.At {
				found = true
			}
		}
		if !found {
			c.fail("bad_at", fmt.Sprintf("at %s is not one of repo %q's listed commits", short(ref.At), ref.Repo),
				"set `at` to a full SHA from the repo's commits (not WORKTREE), or remove it")
			return res
		}
		state = ref.At
		res.StateKind = "at"
	} else if node != nil && node.Change == review.ChangeDeleted {
		state = StateInitial
		res.StateKind = "initial"
	}
	res.State = repo.StateSHA(state)

	if ref.Type == review.RefCommit {
		for _, sha := range repo.Commits {
			if strings.HasPrefix(sha, ref.SHA) {
				res.State = sha
				return res
			}
		}
		c.fail("commit_not_listed", fmt.Sprintf("commit %s is not one of repo %q's listed commits", ref.SHA, ref.Repo),
			"point at a commit from the repo's `commits`, or add it there if it is part of the change")
		return res
	}

	content := repo.Read(state, ref.File)
	if !content.Exists {
		where := map[string]string{"final": "final state", "initial": "initial state", "at": "state at " + short(ref.At)}[res.StateKind]
		hint := "check the path; it is repo-relative"
		if fd := repo.DiffByOld(ref.File); fd != nil && fd.Status == DiffRenamed {
			hint = fmt.Sprintf("the file was renamed to %s; reference it by its final path", fd.Path)
		} else if fd := repo.Diff(ref.File); fd != nil && fd.Status == DiffDeleted && node != nil && node.Change != review.ChangeDeleted {
			hint = "the file is deleted by the change; mark the node `deleted` to read it in the initial state"
		} else if node != nil && node.Change == review.ChangeDeleted {
			hint = "deleted nodes are read in the initial state; the file did not exist there"
		}
		c.fail("file_not_found", fmt.Sprintf("%s not found in the %s of %s", ref.File, where, ref.Repo), hint)
		return res
	}

	if ref.IsLineBased() {
		if content.Binary {
			c.fail("binary_file", fmt.Sprintf("%s is a binary file; it can only be referenced with a `file` ref", ref.File),
				"use { \"type\": \"file\", ... } for binary files")
			return res
		}
		last := len(content.Lines)
		if ref.Type != review.RefSite && ref.End() > last {
			c.fail("range_out_of_bounds", fmt.Sprintf("lines %d-%d end past the last line (%d) of %s", ref.Start(), ref.End(), last, ref.File),
				fmt.Sprintf("the file has %d lines in this state; adjust the range", last))
			return res
		}
	}

	switch ref.Type {
	case review.RefFile:
		if node != nil && ref.At == "" {
			c.checkFileChange()
		}
	case review.RefRange:
		if node != nil && ref.At == "" {
			c.checkRangeChange()
		}
	case review.RefSite:
		c.checkSite(content)
	case review.RefHunk:
		c.checkHunk()
		if node != nil && node.Change == review.ChangeUnchanged && res.Status == review.StatusVerified {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked unchanged but references a hunk of the diff", node.ID),
				"mark the node with the change it makes, or use a `range` ref")
		}
	}
	return res
}

func (c *refCheck) checkFileChange() {
	n, repo, file := c.node, c.repo, c.ref.File
	fd := repo.Diff(file)
	if repo.Repo.IsContext() {
		if n.IsChanged() {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but repo %q is a context repo with no diff", n.ID, n.Change, repo.Repo.ID),
				"refs into context repos never count as changed; mark the node unchanged or list the repo as changed")
		}
		return
	}
	if n.Change == review.ChangeUnchanged {
		if fd != nil {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked unchanged but %s is %s in the diff", n.ID, file, fd.Status),
				fmt.Sprintf("mark the node %s, or point it at an unchanged file", fd.ChangeState()))
		}
		return
	}
	if fd == nil {
		c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but %s is not in the diff", n.ID, n.Change, file),
			"mark the node unchanged, or check that the change is in the listed commits/WORKTREE")
		return
	}
	if fd.ChangeState() != n.Change {
		c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but %s is %s in the diff", n.ID, n.Change, file, fd.Status),
			fmt.Sprintf("mark the node %s", fd.ChangeState()))
	}
}

func (c *refCheck) checkRangeChange() {
	n, repo, file := c.node, c.repo, c.ref.File
	s, e := c.ref.Lines[0], c.ref.Lines[1]
	fd := repo.Diff(file)
	if repo.Repo.IsContext() {
		if n.IsChanged() {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but repo %q is a context repo with no diff", n.ID, n.Change, repo.Repo.ID),
				"refs into context repos never count as changed; mark the node unchanged or list the repo as changed")
		}
		return
	}

	switch n.Change {
	case review.ChangeUnchanged:
		if fd == nil {
			return
		}
		if lines := changedInRange(fd, s, e, true); len(lines) > 0 {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked unchanged but lines %s inside %d-%d changed", n.ID, runsText(lines), s, e),
				"mark the node modified, or narrow the range to unchanged lines")
		}
	case review.ChangeAdded, review.ChangeModified:
		if fd == nil {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but %s has no change in the diff", n.ID, n.Change, file),
				"mark the node unchanged, or point the range at changed lines")
			return
		}
		if n.Change == review.ChangeModified && fd.Status == DiffAdded {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked modified but %s is a new file", n.ID, file),
				"mark the node added")
			return
		}
		if len(changedInRange(fd, s, e, false)) == 0 {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked %s but lines %d-%d have no changed line", n.ID, n.Change, s, e),
				fmt.Sprintf("the diff changes %s of this file; adjust the range or the change state", linesWord(hunksText(fd))))
		}
	case review.ChangeDeleted:
		if fd == nil {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked deleted but %s has no change in the diff", n.ID, file),
				"mark the node unchanged, or point the range at removed lines (initial-state numbers)")
			return
		}
		removed := 0
		for _, h := range fd.Hunks {
			removed += overlap(h.OldStart, h.OldEnd(), s, e)
		}
		if removed == 0 {
			c.fail("change_mismatch", fmt.Sprintf("node %q is marked deleted but initial-state lines %d-%d contain no removed line", n.ID, s, e),
				fmt.Sprintf("removed lines in the initial state: %s; adjust the range or the change state", oldHunksText(fd)))
		}
	}
}

func (c *refCheck) checkSite(content *Content) {
	ref := c.ref
	lines := content.Lines
	has := func(n int) bool {
		return n >= 1 && n <= len(lines) && strings.Contains(lines[n-1], ref.Contains)
	}
	if has(ref.Line) {
		return
	}
	for _, d := range []int{1, -1, 2, -2} {
		if has(ref.Line + d) {
			actual := ref.Line + d
			c.res.ActualLine = actual
			c.warn("site_offset", fmt.Sprintf("%q is on line %d, not %d", ref.Contains, actual, ref.Line),
				fmt.Sprintf("set line to %d", actual))
			return
		}
	}
	if ref.Line > len(lines) {
		c.fail("range_out_of_bounds", fmt.Sprintf("line %d is past the last line (%d) of %s", ref.Line, len(lines), ref.File),
			"point the site at an existing line")
		return
	}
	hint := "point `line` at the line holding `contains`, or fix `contains` (exact, case and whitespace as written)"
	if found := findLines(lines, ref.Contains, 3); len(found) > 0 {
		hint = fmt.Sprintf("%q appears on line %s", ref.Contains, joinInts(found))
	}
	c.fail("site_mismatch", fmt.Sprintf("%q not found on line %d of %s (±2 lines)", ref.Contains, ref.Line, ref.File), hint)
}

func (c *refCheck) checkHunk() {
	fd := c.repo.Diff(c.ref.File)
	s, e := c.ref.Lines[0], c.ref.Lines[1]
	if fd != nil {
		for _, h := range fd.Hunks {
			if h.PureDeletion() {
				if overlap(h.OldStart, h.OldEnd(), s, e) > 0 || touchesDeletion(h, s, e) {
					return
				}
			} else if overlap(h.NewStart, h.NewEnd(), s, e) > 0 {
				return
			}
		}
	}
	hint := "no hunk of the diff touches this file"
	if fd != nil {
		hint = fmt.Sprintf("hunks in this file: %s", hunksText(fd))
	}
	c.fail("hunk_not_found", fmt.Sprintf("no hunk of %s overlaps lines %d-%d", c.ref.File, s, e), hint)
}

// changedInRange lists final-state lines in [s,e] that the diff adds. A
// pure deletion counts at the line before it; strict requires it to sit
// strictly inside the range.
func changedInRange(fd *FileDiff, s, e int, strict bool) []int {
	var out []int
	for _, h := range fd.Hunks {
		if h.PureDeletion() {
			n := h.NewStart
			inside := s <= n && n+1 <= e
			if !strict {
				inside = s <= n+1 && n <= e
			}
			if inside {
				out = append(out, max(n, s))
			}
			continue
		}
		for l := max(h.NewStart, s); l <= min(h.NewEnd(), e); l++ {
			out = append(out, l)
		}
	}
	return out
}

func touchesDeletion(h *Hunk, s, e int) bool {
	n := h.NewStart
	return s <= n+1 && n <= e
}

func overlap(a1, a2, b1, b2 int) int {
	lo, hi := max(a1, b1), min(a2, b2)
	if hi < lo {
		return 0
	}
	return hi - lo + 1
}

func hunksText(fd *FileDiff) string {
	var parts []string
	for i, h := range fd.Hunks {
		if i == 6 {
			parts = append(parts, "…")
			break
		}
		if h.PureDeletion() {
			parts = append(parts, fmt.Sprintf("removal after %d", h.NewStart))
		} else if h.NewCount == 1 {
			parts = append(parts, fmt.Sprint(h.NewStart))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", h.NewStart, h.NewEnd()))
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

// linesWord prefixes a list of line numbers and ranges with line or lines.
func linesWord(list string) string {
	if !strings.ContainsAny(list, ",-") && !strings.HasPrefix(list, "removal") {
		return "line " + list
	}
	return "lines " + list
}

func oldHunksText(fd *FileDiff) string {
	var parts []string
	for _, h := range fd.Hunks {
		if h.OldCount > 0 {
			parts = append(parts, fmt.Sprintf("%d-%d", h.OldStart, h.OldEnd()))
		}
	}
	if len(parts) == 0 {
		return "(none)"
	}
	return strings.Join(parts, ", ")
}

// runs groups sorted line numbers into [start, end] runs.
func runs(lines []int) [][2]int {
	var out [][2]int
	for _, l := range lines {
		if n := len(out); n > 0 && out[n-1][1]+1 >= l {
			out[n-1][1] = max(out[n-1][1], l)
			continue
		}
		out = append(out, [2]int{l, l})
	}
	return out
}

func runsText(lines []int) string {
	var parts []string
	for _, r := range runs(lines) {
		if r[0] == r[1] {
			parts = append(parts, fmt.Sprint(r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return strings.Join(parts, ", ")
}

func findLines(lines []string, needle string, max int) []int {
	var out []int
	for i, l := range lines {
		if strings.Contains(l, needle) {
			out = append(out, i+1)
			if len(out) == max {
				break
			}
		}
	}
	return out
}

func joinInts(v []int) string {
	s := make([]string, len(v))
	for i, n := range v {
		s[i] = fmt.Sprint(n)
	}
	return strings.Join(s, ", ")
}
