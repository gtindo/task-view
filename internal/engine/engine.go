// Package engine resolves a review file against the filesystem and Git:
// commit chains, reference checks, coverage and diff-level checks.
package engine

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gtindo/changes-view/internal/gitx"
	"github.com/gtindo/changes-view/internal/review"
)

// beforeWorktreeCheck lets tests change files during a run.
var beforeWorktreeCheck func()

// Options tune a run.
type Options struct {
	// ScanSiblings enables the unlisted-repo scan of folders next to the
	// listed repos. Off by default: only the listed repos are read.
	ScanSiblings bool
}

// HunkCoverage says which nodes cover one hunk of the diff.
type HunkCoverage struct {
	Hunk      *Hunk
	Nodes     []string // node ids whose file or range covers part of the hunk
	TestNodes []string
	Uncovered [][2]int // uncovered runs; final-state numbers, or initial-state for pure deletions
	OldSide   bool     // Uncovered uses initial-state numbers
	Coverage  string   // covered, partial, uncovered
}

// FileCoverage is the coverage of one changed file.
type FileCoverage struct {
	Diff      *FileDiff
	Hunks     []*HunkCoverage
	FileNodes []string // nodes with a `file` ref on it
	Coverage  string
	HasTests  bool
}

// ClaimResult is the outcome of one claim's evidence.
type ClaimResult struct {
	Status   string
	Evidence []*RefResult
}

// UnlistedRepo is a sibling repo with uncommitted changes.
type UnlistedRepo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Changes int    `json:"changes"`
}

// Summary counts statuses across nodes, edges and claim evidence.
type Summary struct {
	Verified        int `json:"verified"`
	Contradicted    int `json:"contradicted"`
	Unresolved      int `json:"unresolved"`
	AgentStatements int `json:"agentStatements"`
}

// Engine holds one run over a review file.
type Engine struct {
	ReviewPath string
	Base       string
	Review     *review.Review
	Repos      []*RepoState
	Findings   []review.Finding
	ExitCode   int

	NodeResults  []*RefResult
	EdgeResults  []*RefResult
	ClaimResults []*ClaimResult
	Coverage     map[string][]*FileCoverage // by repo id
	Unlisted     []UnlistedRepo
	Uncommitted  map[string][]string
	Untested     []string
	Summary      Summary

	repoByID    map[string]*RepoState
	nodeByID    map[string]*review.Node
	uncommitted map[string][]string
	opts        Options
}

// Repo returns the state of a listed repo.
func (e *Engine) Repo(id string) *RepoState { return e.repoByID[id] }

// Run checks a review file end to end.
func Run(path string, opts Options) *Engine {
	e := &Engine{repoByID: map[string]*RepoState{}, nodeByID: map[string]*review.Node{},
		uncommitted: map[string][]string{}, Coverage: map[string][]*FileCoverage{}, opts: opts}
	abs, err := filepath.Abs(path)
	if err == nil {
		e.ReviewPath = abs
	}

	r, fs := review.Load(path)
	e.Findings = append(e.Findings, fs...)
	if r == nil {
		return e.finish()
	}
	e.Review = r
	for _, n := range r.Nodes {
		e.nodeByID[n.ID] = n
	}

	if !gitx.Available() {
		e.Findings = append(e.Findings, review.NewFinding("git_not_found", "", "no `git` executable on PATH",
			"install Git or add it to PATH"))
		return e.finish()
	}

	dir := filepath.Dir(e.ReviewPath)
	if top, err := gitx.Toplevel(dir); err == nil {
		e.Base = top
	} else {
		e.Base = dir
	}

	if env := e.resolveRepos(); len(env) > 0 {
		e.Findings = append(e.Findings, env...)
		return e.finish()
	}

	for _, s := range e.Repos {
		e.Findings = append(e.Findings, e.checkChain(s)...)
		if s.OK {
			if err := e.loadDiff(s); err != nil {
				s.OK = false
				f := review.NewFinding("commit_chain_broken", fmt.Sprintf("/repos/%d/commits", s.Index),
					fmt.Sprintf("cannot diff repo %q: %v", s.Repo.ID, err), "check that the listed commits exist")
				f.Repo = s.Repo.ID
				e.Findings = append(e.Findings, f)
			} else if !s.Repo.IsContext() {
				e.attribute(s)
			}
		}
	}
	e.Uncommitted = e.uncommitted

	for i, n := range r.Nodes {
		if n.Ref == nil {
			e.NodeResults = append(e.NodeResults, &RefResult{Status: review.StatusAgent})
			continue
		}
		e.NodeResults = append(e.NodeResults, e.checkRef(n.Ref, fmt.Sprintf("/nodes/%d/ref", i), n))
	}
	for i, ed := range r.Edges {
		if ed.Ref == nil {
			e.EdgeResults = append(e.EdgeResults, &RefResult{Status: review.StatusAgent})
			continue
		}
		e.EdgeResults = append(e.EdgeResults, e.checkRef(ed.Ref, fmt.Sprintf("/edges/%d/ref", i), nil))
	}
	for i, c := range r.Claims {
		cr := &ClaimResult{Status: review.StatusAgent}
		for j, ev := range c.Evidence {
			res := e.checkRef(ev, fmt.Sprintf("/claims/%d/evidence/%d", i, j), nil)
			cr.Evidence = append(cr.Evidence, res)
		}
		cr.Status = worst(cr.Evidence)
		e.ClaimResults = append(e.ClaimResults, cr)
	}

	e.computeCoverage()
	e.checkUntested()
	e.checkLanguages()
	if beforeWorktreeCheck != nil {
		beforeWorktreeCheck()
	}
	e.checkWorktreeChanged()
	if opts.ScanSiblings {
		e.scanSiblings()
	}
	return e.finish()
}

func (e *Engine) finish() *Engine {
	e.ExitCode = review.ExitCode(e.Findings)
	e.Summary = e.summarize()
	return e
}

func worst(results []*RefResult) string {
	if len(results) == 0 {
		return review.StatusAgent
	}
	status := review.StatusVerified
	for _, r := range results {
		switch r.Status {
		case review.StatusUnresolved:
			status = review.StatusUnresolved
		case review.StatusContradicted:
			if status != review.StatusUnresolved {
				status = review.StatusContradicted
			}
		}
	}
	return status
}

func (e *Engine) summarize() Summary {
	var s Summary
	count := func(status string) {
		switch status {
		case review.StatusVerified:
			s.Verified++
		case review.StatusContradicted:
			s.Contradicted++
		case review.StatusUnresolved:
			s.Unresolved++
		default:
			s.AgentStatements++
		}
	}
	for _, r := range e.NodeResults {
		count(r.Status)
	}
	for _, r := range e.EdgeResults {
		count(r.Status)
	}
	for _, c := range e.ClaimResults {
		if len(c.Evidence) == 0 {
			count(review.StatusAgent)
		}
		for _, r := range c.Evidence {
			count(r.Status)
		}
	}
	if e.Review != nil {
		for _, n := range e.Review.Nodes {
			if n.Why != "" {
				s.AgentStatements++
			}
			s.AgentStatements += len(n.Meta)
		}
	}
	return s
}

type interval struct {
	s, e int
	node string
	test bool
}

// computeCoverage maps every hunk of every changed repo onto the nodes
// whose `file` or `range` refs cover it, and raises unmentioned_hunk and
// unreferenced_test_hunk warnings.
func (e *Engine) computeCoverage() {
	type fileKey struct{ repo, file string }
	newSide := map[fileKey][]interval{}
	oldSide := map[fileKey][]interval{}
	whole := map[fileKey][]interval{}
	testFiles := map[fileKey]bool{}

	for _, n := range e.Review.Nodes {
		ref := n.Ref
		if ref == nil || ref.At != "" || ref.File == "" {
			continue
		}
		s := e.repoByID[ref.Repo]
		if s == nil || !s.OK || s.Repo.IsContext() {
			continue
		}
		k := fileKey{ref.Repo, ref.File}
		iv := interval{node: n.ID, test: n.IsTest()}
		if n.IsTest() {
			testFiles[k] = true
		}
		switch ref.Type {
		case review.RefFile:
			whole[k] = append(whole[k], iv)
		case review.RefRange, review.RefHunk:
			iv.s, iv.e = ref.Lines[0], ref.Lines[1]
			if n.Change == review.ChangeDeleted {
				oldSide[k] = append(oldSide[k], iv)
			} else {
				newSide[k] = append(newSide[k], iv)
			}
		}
	}

	for _, s := range e.Repos {
		if !s.OK || s.Repo.IsContext() {
			continue
		}
		var files []*FileCoverage
		for _, fd := range s.Files {
			k := fileKey{s.Repo.ID, fd.Key()}
			fc := &FileCoverage{Diff: fd, HasTests: testFiles[k]}
			for _, iv := range whole[k] {
				fc.FileNodes = appendUnique(fc.FileNodes, iv.node)
			}
			wholeTest := false
			for _, iv := range whole[k] {
				if iv.test {
					wholeTest = true
				}
			}

			if len(fd.Hunks) == 0 {
				fc.Coverage = "covered"
				if len(fc.FileNodes) == 0 {
					fc.Coverage = "uncovered"
					what := "binary file changed"
					if !fd.Binary {
						what = fmt.Sprintf("file renamed from %s", fd.OldPath)
					}
					f := review.NewFinding("unmentioned_hunk", "", what+", not covered by any node",
						"add a node with a `file` ref on it, or explain in notes")
					f.Repo, f.File = s.Repo.ID, fd.Key()
					e.Findings = append(e.Findings, f)
				}
				files = append(files, fc)
				continue
			}

			for _, h := range fd.Hunks {
				hc := &HunkCoverage{Hunk: h}
				var uncovered, untestedLines []int
				mark := func(iv interval) {
					hc.Nodes = appendUnique(hc.Nodes, iv.node)
					if iv.test {
						hc.TestNodes = appendUnique(hc.TestNodes, iv.node)
					}
				}
				for _, iv := range whole[k] {
					mark(iv)
				}
				if h.PureDeletion() {
					hc.OldSide = true
					covered := len(whole[k]) > 0
					testCovered := wholeTest
					for _, iv := range newSide[k] {
						if iv.s <= h.NewStart+1 && iv.e >= h.NewStart {
							mark(iv)
							covered = true
							testCovered = testCovered || iv.test
						}
					}
					for l := h.OldStart; l <= h.OldEnd(); l++ {
						lc := covered
						for _, iv := range oldSide[k] {
							if l >= iv.s && l <= iv.e {
								mark(iv)
								lc = true
							}
						}
						if !lc {
							uncovered = append(uncovered, l)
						}
						if !testCovered {
							untestedLines = append(untestedLines, l)
						}
					}
				} else {
					for l := h.NewStart; l <= h.NewEnd(); l++ {
						lc := len(whole[k]) > 0
						tc := wholeTest
						for _, iv := range newSide[k] {
							if l >= iv.s && l <= iv.e {
								mark(iv)
								lc = true
								tc = tc || iv.test
							}
						}
						if !lc {
							uncovered = append(uncovered, l)
						}
						if !tc {
							untestedLines = append(untestedLines, l)
						}
					}
					for _, iv := range oldSide[k] {
						if overlap(h.OldStart, h.OldEnd(), iv.s, iv.e) > 0 {
							mark(iv)
						}
					}
				}

				hc.Uncovered = runs(uncovered)
				total := h.NewCount
				if h.PureDeletion() {
					total = h.OldCount
				}
				switch {
				case len(uncovered) == 0:
					hc.Coverage = "covered"
				case len(uncovered) == total:
					hc.Coverage = "uncovered"
				default:
					hc.Coverage = "partial"
				}
				fc.Hunks = append(fc.Hunks, hc)

				for _, r := range hc.Uncovered {
					msg := "changed lines not covered by any node"
					hint := fmt.Sprintf("add a node with a range covering %s, or explain in notes", linesText(r))
					if hc.OldSide {
						msg = fmt.Sprintf("removed lines (initial state %s) not covered by any node", rangeText(r))
						hint = fmt.Sprintf("cover the removal with a range around line %d, or a deleted node on initial lines %s", h.NewStart, rangeText(r))
					}
					f := review.NewFinding("unmentioned_hunk", "", msg, hint)
					f.Repo, f.File, f.Lines = s.Repo.ID, fd.Key(), []int{r[0], r[1]}
					e.Findings = append(e.Findings, f)
				}
				if testFiles[k] {
					for _, r := range runs(untestedLines) {
						f := review.NewFinding("unreferenced_test_hunk", "",
							"changed lines in a test file outside every test node",
							fmt.Sprintf("add a test node covering %s, or widen an existing test node's range", linesText(r)))
						f.Repo, f.File, f.Lines = s.Repo.ID, fd.Key(), []int{r[0], r[1]}
						e.Findings = append(e.Findings, f)
					}
				}
			}
			fc.Coverage = fileCoverage(fc.Hunks)
			files = append(files, fc)
		}
		e.Coverage[s.Repo.ID] = files
	}
}

func fileCoverage(hunks []*HunkCoverage) string {
	cov, unc := 0, 0
	for _, h := range hunks {
		switch h.Coverage {
		case "covered":
			cov++
		case "uncovered":
			unc++
		}
	}
	switch {
	case cov == len(hunks):
		return "covered"
	case unc == len(hunks):
		return "uncovered"
	}
	return "partial"
}

func linesText(r [2]int) string {
	if r[0] == r[1] {
		return fmt.Sprintf("line %d", r[0])
	}
	return fmt.Sprintf("lines %d-%d", r[0], r[1])
}

func rangeText(r [2]int) string {
	if r[0] == r[1] {
		return fmt.Sprint(r[0])
	}
	return fmt.Sprintf("%d-%d", r[0], r[1])
}

// checkUntested warns on changed non-test nodes without a `tests` edge from
// a verified test node. Nodes on non-source files (documentation, data,
// configuration, binary assets) are skipped: there is nothing to test.
func (e *Engine) checkUntested() {
	tested := map[string]bool{}
	for _, ed := range e.Review.Edges {
		if ed.Kind != "tests" {
			continue
		}
		from := e.nodeByID[ed.From]
		if from == nil || !from.IsTest() {
			continue
		}
		if e.nodeStatus(ed.From) == review.StatusVerified {
			tested[ed.To] = true
		}
	}
	for i, n := range e.Review.Nodes {
		if n.IsTest() || !n.IsChanged() || tested[n.ID] {
			continue
		}
		if n.Ref != nil {
			if s := e.repoByID[n.Ref.Repo]; s != nil && (s.Repo.IsContext() || s.nonSource(n.Ref)) {
				continue
			}
		}
		e.Untested = append(e.Untested, n.ID)
		f := review.NewFinding("untested_change", fmt.Sprintf("/nodes/%d", i),
			fmt.Sprintf("changed node %q has no `tests` edge from a test node whose reference is verified (the test exists as described; whether it passed is never checked)", n.ID),
			"add a test node and a `tests` edge to this node, or say in notes why it is not tested")
		if n.Ref != nil {
			f.Repo, f.File = n.Ref.Repo, n.Ref.File
		}
		e.Findings = append(e.Findings, f)
	}
}

func (e *Engine) nodeStatus(id string) string {
	for i, n := range e.Review.Nodes {
		if n.ID == id && i < len(e.NodeResults) {
			return e.NodeResults[i].Status
		}
	}
	return ""
}

// checkLanguages warns once per referenced file that matches no language.
func (e *Engine) checkLanguages() {
	seen := map[string]bool{}
	for _, site := range e.Review.AllRefs() {
		ref := site.Ref
		if ref.File == "" {
			continue
		}
		s := e.repoByID[ref.Repo]
		if s == nil {
			continue
		}
		key := ref.Repo + "\x00" + ref.File
		if seen[key] || s.Language(ref.File, ref.Language) != "" {
			continue
		}
		seen[key] = true
		f := review.NewFinding("no_language", site.Path, fmt.Sprintf("%s matches no language; it is shown as plain text", ref.File),
			fmt.Sprintf("add a glob for it to repo %q's `languages`, or set `language` on the ref", ref.Repo))
		f.Repo, f.File = ref.Repo, ref.File
		e.Findings = append(e.Findings, f)
	}
}

func (e *Engine) checkWorktreeChanged() {
	for _, s := range e.Repos {
		if !s.Worktree {
			continue
		}
		for _, p := range s.changedWorktreeFiles() {
			f := review.NewFinding("worktree_changed", "", fmt.Sprintf("%s changed on disk while it was being checked", p),
				"wait until edits are finished, then validate again")
			f.Repo, f.File = s.Repo.ID, p
			e.Findings = append(e.Findings, f)
		}
	}
}

// scanSiblings looks one level deep in the parent folders of listed repos
// for Git repos with uncommitted changes that the review does not list.
func (e *Engine) scanSiblings() {
	listed := map[string]bool{}
	parents := map[string]bool{}
	for _, s := range e.Repos {
		p, _ := filepath.EvalSymlinks(s.Abs)
		listed[filepath.Clean(p)] = true
		parents[filepath.Dir(filepath.Clean(p))] = true
	}
	var dirs []string
	for p := range parents {
		dirs = append(dirs, p)
	}
	sort.Strings(dirs)
	for _, parent := range dirs {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}
		for _, en := range entries {
			if !en.IsDir() || strings.HasPrefix(en.Name(), ".") {
				continue
			}
			dir := filepath.Join(parent, en.Name())
			if listed[dir] {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
				continue
			}
			status, err := gitx.Repo{Dir: dir}.Status()
			if err != nil || len(status) == 0 {
				continue
			}
			e.Unlisted = append(e.Unlisted, UnlistedRepo{Name: en.Name(), Path: dir, Changes: len(status)})
			f := review.NewFinding("unlisted_repo", "",
				fmt.Sprintf("sibling repo %s has %d uncommitted %s and is not in repos", en.Name(), len(status), plural(len(status), "change", "changes")),
				"if the change touched it, list it in `repos` with \"WORKTREE\"; otherwise ignore")
			f.Repo = en.Name()
			e.Findings = append(e.Findings, f)
		}
	}
}

func appendUnique(list []string, v string) []string {
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}
