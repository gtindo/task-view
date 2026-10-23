// Package engine resolves a review file against the filesystem and Git:
// commit chains, reference checks, coverage and diff-level checks.
package engine

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/gtindo/changes-view/internal/gitx"
	"github.com/gtindo/changes-view/internal/review"
)

// Coverage statuses of a hunk or a file.
const (
	coverageCovered   = "covered"
	coveragePartial   = "partial"
	coverageUncovered = "uncovered"
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
	// Hunk is the covered hunk.
	Hunk  *Hunk
	Nodes []string // node ids whose file or range covers part of the hunk
	// TestNodes are the test nodes among Nodes.
	TestNodes []string
	Uncovered [][2]int // uncovered runs; final-state numbers, or initial-state for pure deletions
	OldSide   bool     // Uncovered uses initial-state numbers
	Coverage  string   // covered, partial, uncovered
}

// FileCoverage is the coverage of one changed file.
type FileCoverage struct {
	// Diff is the file's diff.
	Diff *FileDiff
	// Hunks covers each hunk of Diff, in order.
	Hunks     []*HunkCoverage
	FileNodes []string // nodes with a `file` ref on it
	// Coverage rolls up the hunks: covered, partial or uncovered.
	Coverage string
	// HasTests is true when a test node points into the file.
	HasTests bool
}

// ClaimResult is the outcome of one claim's evidence.
type ClaimResult struct {
	// Status is the worst status of the evidence, or agent without any.
	Status string
	// Evidence holds one result per evidence ref, in order.
	Evidence []*RefResult
}

// UnlistedRepo is a sibling repo with uncommitted changes.
type UnlistedRepo struct {
	// Name is the repo folder's name.
	Name string `json:"name"`
	// Path is the repo's absolute path.
	Path string `json:"path"`
	// Changes counts its uncommitted paths.
	Changes int `json:"changes"`
}

// Summary counts statuses across nodes, edges and claim evidence.
type Summary struct {
	// Verified counts refs the engine verified.
	Verified int `json:"verified"`
	// Contradicted counts refs the repo contradicts.
	Contradicted int `json:"contradicted"`
	// Unresolved counts refs that could not be checked.
	Unresolved int `json:"unresolved"`
	// AgentStatements counts what rests on the agent's word: statements
	// without a ref, node `why` texts and node metadata entries.
	AgentStatements int `json:"agentStatements"`
}

// Engine holds one run over a review file.
type Engine struct {
	// ReviewPath is the review file's absolute path, empty when unknown.
	ReviewPath string
	// Base is the Git root holding the review file, which relative repo
	// paths resolve from; the review file's folder outside Git.
	Base string
	// Review is the loaded review file, nil when it failed to load.
	Review *review.Review
	// Repos are the listed repos, in review order.
	Repos []*RepoState
	// Findings are all findings, in the order they were raised.
	Findings []review.Finding
	// ExitCode is the process exit code the findings call for.
	ExitCode int

	// NodeResults, EdgeResults and ClaimResults line up with the review's
	// nodes, edges and claims.
	NodeResults  []*RefResult
	EdgeResults  []*RefResult
	ClaimResults []*ClaimResult
	Coverage     map[string][]*FileCoverage // by repo id
	// Unlisted are sibling repos with uncommitted changes.
	Unlisted []UnlistedRepo
	// Uncommitted lists, by repo id, uncommitted paths outside the review.
	Uncommitted map[string][]string
	// Summary counts the statuses of the run.
	Summary Summary

	repoByID map[string]*RepoState
}

// Repo returns the state of a listed repo.
func (e *Engine) Repo(id string) *RepoState { return e.repoByID[id] }

// Run checks a review file end to end.
func Run(path string, options Options) *Engine {
	run := &Engine{
		repoByID: map[string]*RepoState{},
		Coverage: map[string][]*FileCoverage{},
	}
	if abs, err := filepath.Abs(path); err == nil {
		run.ReviewPath = abs
	}

	reviewFile, loadFindings := review.Load(path)
	run.Findings = append(run.Findings, loadFindings...)
	if reviewFile == nil {
		return run.finish()
	}

	run.Review = reviewFile

	if !gitx.Available() {
		run.Findings = append(run.Findings, review.NewFinding("git_not_found", "", "no `git` executable on PATH",
			"install Git or add it to PATH"))

		return run.finish()
	}

	run.Base = filepath.Dir(run.ReviewPath)
	if gitRoot, err := gitx.Toplevel(run.Base); err == nil {
		run.Base = gitRoot
	}

	if envFindings := run.resolveRepos(); len(envFindings) > 0 {
		run.Findings = append(run.Findings, envFindings...)

		return run.finish()
	}

	run.Uncommitted = map[string][]string{}
	for _, repoState := range run.Repos {
		run.loadRepo(repoState)
	}

	run.checkRefs()
	run.computeCoverage()
	run.checkLanguages()
	if beforeWorktreeCheck != nil {
		beforeWorktreeCheck()
	}

	run.checkWorktreeChanged()
	if options.ScanSiblings {
		run.scanSiblings()
	}

	return run.finish()
}

func (e *Engine) finish() *Engine {
	e.ExitCode = review.ExitCode(e.Findings)
	e.Summary = e.summarize()

	return e
}

// loadRepo checks a repo's commit chain, then loads its diff and, for a
// changed repo, which state wrote each added line.
func (e *Engine) loadRepo(repoState *RepoState) {
	e.Findings = append(e.Findings, e.checkChain(repoState)...)
	if !repoState.OK {
		return
	}

	if err := repoState.loadDiff(); err != nil {
		repoState.OK = false
		e.Findings = append(e.Findings, repoState.finding("commit_chain_broken", repoState.commitsPath(),
			fmt.Sprintf("cannot diff repo %q: %v", repoState.Repo.ID, err), "check that the listed commits exist"))

		return
	}

	if !repoState.Repo.IsContext() {
		repoState.attribute()
	}
}

// checkRefs checks the refs of every node, edge and claim. Statements
// without a ref rest on the agent's word.
func (e *Engine) checkRefs() {
	for i, node := range e.Review.Nodes {
		e.NodeResults = append(e.NodeResults, e.checkOptionalRef(node.Ref, fmt.Sprintf("/nodes/%d/ref", i), node))
	}

	for i, edge := range e.Review.Edges {
		e.EdgeResults = append(e.EdgeResults, e.checkOptionalRef(edge.Ref, fmt.Sprintf("/edges/%d/ref", i), nil))
	}

	for i, claim := range e.Review.Claims {
		claimResult := &ClaimResult{}
		for j, evidence := range claim.Evidence {
			claimResult.Evidence = append(claimResult.Evidence, e.checkRef(evidence, fmt.Sprintf("/claims/%d/evidence/%d", i, j), nil))
		}

		claimResult.Status = worstStatus(claimResult.Evidence)
		e.ClaimResults = append(e.ClaimResults, claimResult)
	}
}

func (e *Engine) checkOptionalRef(ref *review.Ref, path string, node *review.Node) *RefResult {
	if ref == nil {
		return &RefResult{Status: review.StatusAgent}
	}

	return e.checkRef(ref, path, node)
}

// worstStatus rolls up evidence: unresolved beats contradicted beats
// verified, and no evidence leaves the agent's word.
func worstStatus(results []*RefResult) string {
	if len(results) == 0 {
		return review.StatusAgent
	}

	status := review.StatusVerified
	for _, result := range results {
		switch result.Status {
		case review.StatusUnresolved:
			return review.StatusUnresolved
		case review.StatusContradicted:
			status = review.StatusContradicted
		}
	}

	return status
}

func (e *Engine) summarize() Summary {
	var summary Summary
	count := func(status string) {
		switch status {
		case review.StatusVerified:
			summary.Verified++
		case review.StatusContradicted:
			summary.Contradicted++
		case review.StatusUnresolved:
			summary.Unresolved++
		default:
			summary.AgentStatements++
		}
	}

	for _, result := range e.NodeResults {
		count(result.Status)
	}

	for _, result := range e.EdgeResults {
		count(result.Status)
	}

	for _, claimResult := range e.ClaimResults {
		if len(claimResult.Evidence) == 0 {
			count(review.StatusAgent)
		}

		for _, result := range claimResult.Evidence {
			count(result.Status)
		}
	}

	if e.Review == nil {
		return summary
	}

	for _, node := range e.Review.Nodes {
		if node.Why != "" {
			summary.AgentStatements++
		}

		summary.AgentStatements += len(node.Meta)
	}

	return summary
}

// fileKey identifies one file of one repo.
type fileKey struct{ repo, file string }

// span is the lines one node's `file` or `range` ref claims in a file.
type span struct {
	start, end int
	node       string
	test       bool
}

func (s span) contains(line int) bool { return line >= s.start && line <= s.end }

// fileSpans are the node spans that point into one file, by how they
// cover it.
type fileSpans struct {
	whole    []span // `file` refs: the whole file
	final    []span // ranges in final-state numbers
	initial  []span // ranges of deleted nodes, in initial-state numbers
	hasTests bool   // a test node points into the file
}

func (f fileSpans) wholeFileTested() bool {
	return slices.ContainsFunc(f.whole, func(whole span) bool { return whole.test })
}

// computeCoverage maps every hunk of every changed repo onto the nodes
// whose `file` or `range` refs cover it, and raises unmentioned_hunk and
// unreferenced_test_hunk warnings.
func (e *Engine) computeCoverage() {
	spans := indexSpans(e.Review.Nodes, e.repoByID)
	for _, repoState := range e.Repos {
		if !repoState.hasDiff() {
			continue
		}

		repoID := repoState.Repo.ID
		var files []*FileCoverage
		for _, fileDiff := range repoState.Files {
			fileCoverage, findings := coverFile(repoID, fileDiff, spans[fileKey{repoID, fileDiff.Key()}])
			e.Findings = append(e.Findings, findings...)
			files = append(files, fileCoverage)
		}

		e.Coverage[repoID] = files
	}
}

// indexSpans groups the spans of node refs by the file they cover. Refs
// pinned with `at` read an older state and cover nothing in the diff.
func indexSpans(nodes []*review.Node, repoByID map[string]*RepoState) map[fileKey]fileSpans {
	spans := map[fileKey]fileSpans{}
	for _, node := range nodes {
		ref := node.Ref
		coversDiff := ref != nil && ref.At == "" && ref.File != ""
		if !coversDiff {
			continue
		}

		repoState := repoByID[ref.Repo]
		if repoState == nil || !repoState.hasDiff() {
			continue
		}

		key := fileKey{ref.Repo, ref.File}
		into := spans[key]
		nodeSpan := span{node: node.ID, test: node.IsTest()}
		into.hasTests = into.hasTests || node.IsTest()
		switch ref.Type {
		case review.RefFile:
			into.whole = append(into.whole, nodeSpan)
		case review.RefRange, review.RefHunk:
			nodeSpan.start, nodeSpan.end = ref.Lines[0], ref.Lines[1]
			if node.Change == review.ChangeDeleted {
				into.initial = append(into.initial, nodeSpan)
			} else {
				into.final = append(into.final, nodeSpan)
			}
		}

		spans[key] = into
	}

	return spans
}

// coverFile computes one changed file's coverage and the findings for its
// uncovered and untested lines.
func coverFile(repoID string, fileDiff *FileDiff, spans fileSpans) (*FileCoverage, []review.Finding) {
	fileCoverage := &FileCoverage{Diff: fileDiff, HasTests: spans.hasTests}
	for _, whole := range spans.whole {
		fileCoverage.FileNodes = appendUnique(fileCoverage.FileNodes, whole.node)
	}

	// Binary files and pure renames have no hunks: only a `file` ref covers them.
	if len(fileDiff.Hunks) == 0 {
		if len(fileCoverage.FileNodes) > 0 {
			fileCoverage.Coverage = coverageCovered

			return fileCoverage, nil
		}

		fileCoverage.Coverage = coverageUncovered
		what := "binary file changed"
		if !fileDiff.Binary {
			what = fmt.Sprintf("file renamed from %s", fileDiff.OldPath)
		}

		finding := review.NewFinding("unmentioned_hunk", "", what+", not covered by any node",
			"add a node with a `file` ref on it, or explain in notes")
		finding.Repo, finding.File = repoID, fileDiff.Key()

		return fileCoverage, []review.Finding{finding}
	}

	var findings []review.Finding
	for _, hunk := range fileDiff.Hunks {
		hunkCoverage, untested := coverHunk(hunk, spans)
		fileCoverage.Hunks = append(fileCoverage.Hunks, hunkCoverage)
		findings = append(findings, unmentionedFindings(repoID, fileDiff.Key(), hunkCoverage)...)
		if spans.hasTests {
			findings = append(findings, untestedFindings(repoID, fileDiff.Key(), untested)...)
		}
	}

	fileCoverage.Coverage = rollUpCoverage(fileCoverage.Hunks)

	return fileCoverage, findings
}

// coverHunk finds the nodes covering a hunk. It also returns the hunk's
// changed lines that no test node covers.
func coverHunk(hunk *Hunk, spans fileSpans) (*HunkCoverage, []int) {
	hunkCoverage := &HunkCoverage{Hunk: hunk}
	for _, whole := range spans.whole {
		hunkCoverage.addNode(whole)
	}

	var uncovered, untested []int
	changedCount := hunk.NewCount
	if hunk.PureDeletion() {
		changedCount = hunk.OldCount
		uncovered, untested = hunkCoverage.coverRemoval(spans)
	} else {
		uncovered, untested = hunkCoverage.coverAddition(spans)
	}

	hunkCoverage.Uncovered = runs(uncovered)
	hunkCoverage.Coverage = hunkStatus(len(uncovered), changedCount)

	return hunkCoverage, untested
}

func (h *HunkCoverage) addNode(nodeSpan span) {
	h.Nodes = appendUnique(h.Nodes, nodeSpan.node)
	if nodeSpan.test {
		h.TestNodes = appendUnique(h.TestNodes, nodeSpan.node)
	}
}

// coverRemoval covers a pure deletion, line by line in initial-state
// numbers. A final-state range touching the removal point covers all of
// it; an initial-state range covers the removed lines it spans.
func (h *HunkCoverage) coverRemoval(spans fileSpans) ([]int, []int) {
	hunk := h.Hunk
	h.OldSide = true
	covered := len(spans.whole) > 0
	tested := spans.wholeFileTested()
	for _, finalSpan := range spans.final {
		if !touchesDeletion(hunk, finalSpan.start, finalSpan.end) {
			continue
		}

		h.addNode(finalSpan)
		covered = true
		tested = tested || finalSpan.test
	}

	var uncovered, untested []int
	for line := hunk.OldStart; line <= hunk.OldEnd(); line++ {
		lineCovered := covered
		for _, initialSpan := range spans.initial {
			if initialSpan.contains(line) {
				h.addNode(initialSpan)
				lineCovered = true
			}
		}

		if !lineCovered {
			uncovered = append(uncovered, line)
		}

		if !tested {
			untested = append(untested, line)
		}
	}

	return uncovered, untested
}

// coverAddition covers a hunk that adds lines, line by line in final-state
// numbers. Deleted nodes over the lines it replaces count as covering it
// too, without making any line covered.
func (h *HunkCoverage) coverAddition(spans fileSpans) ([]int, []int) {
	hunk := h.Hunk
	wholeCovered := len(spans.whole) > 0
	wholeTested := spans.wholeFileTested()

	var uncovered, untested []int
	for line := hunk.NewStart; line <= hunk.NewEnd(); line++ {
		lineCovered, lineTested := wholeCovered, wholeTested
		for _, finalSpan := range spans.final {
			if !finalSpan.contains(line) {
				continue
			}

			h.addNode(finalSpan)
			lineCovered = true
			lineTested = lineTested || finalSpan.test
		}

		if !lineCovered {
			uncovered = append(uncovered, line)
		}

		if !lineTested {
			untested = append(untested, line)
		}
	}

	for _, initialSpan := range spans.initial {
		if overlap(hunk.OldStart, hunk.OldEnd(), initialSpan.start, initialSpan.end) > 0 {
			h.addNode(initialSpan)
		}
	}

	return uncovered, untested
}

func hunkStatus(uncoveredCount, changedCount int) string {
	switch uncoveredCount {
	case 0:
		return coverageCovered
	case changedCount:
		return coverageUncovered
	}

	return coveragePartial
}

// rollUpCoverage is a file's status: covered or uncovered when all its
// hunks are, else partial.
func rollUpCoverage(hunks []*HunkCoverage) string {
	covered, uncovered := 0, 0
	for _, hunkCoverage := range hunks {
		switch hunkCoverage.Coverage {
		case coverageCovered:
			covered++
		case coverageUncovered:
			uncovered++
		}
	}

	switch {
	case covered == len(hunks):
		return coverageCovered
	case uncovered == len(hunks):
		return coverageUncovered
	}

	return coveragePartial
}

// unmentionedFindings reports each run of a hunk that no node covers.
func unmentionedFindings(repoID, file string, hunkCoverage *HunkCoverage) []review.Finding {
	var findings []review.Finding
	for _, run := range hunkCoverage.Uncovered {
		msg := "changed lines not covered by any node"
		hint := fmt.Sprintf("add a node with a range covering %s, or explain in notes", linesText(run))
		if hunkCoverage.OldSide {
			msg = fmt.Sprintf("removed lines (initial state %s) not covered by any node", rangeText(run))
			hint = fmt.Sprintf("cover the removal with a range around line %d, or a deleted node on initial lines %s",
				hunkCoverage.Hunk.NewStart, rangeText(run))
		}

		findings = append(findings, linesFinding("unmentioned_hunk", msg, hint, repoID, file, run))
	}

	return findings
}

// untestedFindings reports each run of changed lines in a test file that
// no test node covers.
func untestedFindings(repoID, file string, untested []int) []review.Finding {
	var findings []review.Finding
	for _, run := range runs(untested) {
		findings = append(findings, linesFinding("unreferenced_test_hunk",
			"changed lines in a test file outside every test node",
			fmt.Sprintf("add a test node covering %s, or widen an existing test node's range", linesText(run)),
			repoID, file, run))
	}

	return findings
}

func linesFinding(code, msg, hint, repoID, file string, run [2]int) review.Finding {
	finding := review.NewFinding(code, "", msg, hint)
	finding.Repo, finding.File, finding.Lines = repoID, file, []int{run[0], run[1]}

	return finding
}

func linesText(run [2]int) string {
	if run[0] == run[1] {
		return fmt.Sprintf("line %d", run[0])
	}

	return fmt.Sprintf("lines %d-%d", run[0], run[1])
}

func rangeText(run [2]int) string {
	if run[0] == run[1] {
		return fmt.Sprint(run[0])
	}

	return fmt.Sprintf("%d-%d", run[0], run[1])
}

// checkLanguages warns once per referenced file that matches no language.
func (e *Engine) checkLanguages() {
	seen := map[fileKey]bool{}
	for _, site := range e.Review.AllRefs() {
		ref := site.Ref
		if ref.File == "" {
			continue
		}

		repoState := e.repoByID[ref.Repo]
		if repoState == nil {
			continue
		}

		key := fileKey{ref.Repo, ref.File}
		if seen[key] || repoState.Language(ref.File, ref.Language) != "" {
			continue
		}

		seen[key] = true
		finding := review.NewFinding("no_language", site.Path, fmt.Sprintf("%s matches no language; it is shown as plain text", ref.File),
			fmt.Sprintf("add a glob for it to repo %q's `languages`, or set `language` on the ref", ref.Repo))
		finding.Repo, finding.File = ref.Repo, ref.File
		e.Findings = append(e.Findings, finding)
	}
}

// checkWorktreeChanged warns about worktree files edited while the run read
// them, since the results may mix two versions.
func (e *Engine) checkWorktreeChanged() {
	for _, repoState := range e.Repos {
		if !repoState.Worktree {
			continue
		}

		for _, file := range repoState.changedWorktreeFiles() {
			finding := review.NewFinding("worktree_changed", "", fmt.Sprintf("%s changed on disk while it was being checked", file),
				"wait until edits are finished, then validate again")
			finding.Repo, finding.File = repoState.Repo.ID, file
			e.Findings = append(e.Findings, finding)
		}
	}
}

// scanSiblings looks one level deep in the parent folders of listed repos
// for Git repos with uncommitted changes that the review does not list.
func (e *Engine) scanSiblings() {
	listed := map[string]bool{}
	parents := map[string]bool{}
	for _, repoState := range e.Repos {
		resolved, _ := filepath.EvalSymlinks(repoState.Abs)
		listed[filepath.Clean(resolved)] = true
		parents[filepath.Dir(filepath.Clean(resolved))] = true
	}

	for _, parent := range slices.Sorted(maps.Keys(parents)) {
		entries, err := os.ReadDir(parent)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			name := entry.Name()
			dir := filepath.Join(parent, name)
			candidate := entry.IsDir() && !strings.HasPrefix(name, ".") && !listed[dir]
			if !candidate {
				continue
			}

			changes := uncommittedCount(dir)
			if changes == 0 {
				continue
			}

			e.Unlisted = append(e.Unlisted, UnlistedRepo{Name: name, Path: dir, Changes: changes})
			e.Findings = append(e.Findings, repoFinding(name, "unlisted_repo", "",
				fmt.Sprintf("sibling repo %s has %d uncommitted %s and is not in repos", name, changes, plural(changes, "change", "changes")),
				"if the change touched it, list it in `repos` with \"WORKTREE\"; otherwise ignore"))
		}
	}
}

// uncommittedCount counts uncommitted paths in dir; 0 when dir is not a Git
// repo or Git cannot read it.
func uncommittedCount(dir string) int {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return 0
	}

	status, err := gitx.Repo{Dir: dir}.Status()
	if err != nil {
		return 0
	}

	return len(status)
}

func appendUnique(list []string, value string) []string {
	if slices.Contains(list, value) {
		return list
	}

	return append(list, value)
}
