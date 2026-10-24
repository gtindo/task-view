package engine

import (
	"cmp"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/gtindo/task-view/internal/gitx"
	"github.com/gtindo/task-view/internal/langs"
	"github.com/gtindo/task-view/internal/review"
)

// Gather defaults, shared with the CLI flags.
const (
	// DefaultContextLines is how many unchanged lines gather shows around
	// each hunk unless asked for more: none, since agents read the files to
	// find where each piece starts anyway and context only grows the output.
	DefaultContextLines = 0
	// DefaultMaxHunkLines caps the added and removed lines gather copies per
	// hunk, so generated or vendored files do not flood the agent.
	DefaultMaxHunkLines = 200
)

// defaultReviewFile is where gather assumes the review file will live,
// relative to the Git root of the current directory, as the agent
// instructions place it.
const defaultReviewFile = ".review/change.json"

// maxBaseIDLen keeps a derived repo id short enough for a "-N" suffix to fit
// the 64-character id pattern.
const maxBaseIDLen = 60

// fallbackRepoID names a repo whose folder name has no usable character.
const fallbackRepoID = "repo"

// RepoSpec names one repo for gather and where its change starts.
type RepoSpec struct {
	// Path is the repo root or any folder inside it.
	Path string
	// Base is the commit the change started from. Empty means the change is
	// only the uncommitted work on top of HEAD.
	Base string
	// Context marks a repo the agent only read: one commit, no diff.
	Context bool
}

// GatherOptions selects what gather reads.
type GatherOptions struct {
	// ReviewPath is where the review file will live; repo paths are written
	// relative to its Git root and its `.review/` folder is left out of the
	// change. Empty means `.review/change.json` under the current Git root.
	ReviewPath string
	// Repos are the repos to read, changed and context ones.
	Repos []RepoSpec
	// Tip is the newest commit of the change. Empty means HEAD plus any
	// uncommitted work; an explicit tip never adds WORKTREE.
	Tip string
	// ContextLines and MaxHunkLines size each hunk's excerpt; a MaxHunkLines
	// of zero or less keeps every line.
	ContextLines int
	MaxHunkLines int
}

// Evidence is everything a review file must agree with, read the same way
// validate reads it, so the agent writes the account instead of collecting
// the facts.
type Evidence struct {
	// Repos have the review file's shape: id, path, role, commits and
	// language globs for the changed files.
	Repos []*review.Repo `json:"repos"`
	// Commits describe each repo's listed commits, newest first.
	Commits map[string][]gitx.CommitInfo `json:"commits"`
	// Files are the changed files of every changed repo, with their hunks.
	Files []*FileEvidence `json:"files"`
	// Findings and ExitCode report what kept a repo out of the evidence.
	Findings []review.Finding `json:"-"`
	ExitCode int              `json:"-"`
}

// FileEvidence is one changed file and a ref that covers all of it.
type FileEvidence struct {
	Repo      string          `json:"repo"`
	File      string          `json:"file"`
	OldFile   string          `json:"oldFile,omitempty"`
	Status    string          `json:"status"`
	Change    string          `json:"change"`
	Binary    bool            `json:"binary,omitempty"`
	Untracked bool            `json:"untracked,omitempty"`
	Language  string          `json:"language,omitempty"`
	Added     int             `json:"added"`
	Removed   int             `json:"removed"`
	Ref       *review.Ref     `json:"ref"`
	Hunks     []*HunkEvidence `json:"hunks,omitempty"`
}

// HunkEvidence is one zero-context hunk. Lines is the smallest final-state
// range that covers it the way validate's coverage reads refs: the added
// lines, or the line before a pure deletion. A hunk without Lines (in a
// deleted or emptied file) is covered by its file's ref.
type HunkEvidence struct {
	Lines     []int    `json:"lines,omitempty"`
	OldLines  []int    `json:"oldLines,omitempty"`
	Commits   []string `json:"commits,omitempty"`
	Added     []string `json:"added,omitempty"`
	Removed   []string `json:"removed,omitempty"`
	Truncated bool     `json:"truncated,omitempty"`
	Context   *Excerpt `json:"context,omitempty"`
}

// Excerpt is a run of final-state lines starting at line Start.
type Excerpt struct {
	Start int      `json:"start"`
	Lines []string `json:"lines"`
}

// excerptLimits size the text copied for each hunk.
type excerptLimits struct {
	contextLines int
	maxHunkLines int
}

// gatherTarget is a repo spec with its resolved Git root.
type gatherTarget struct {
	spec RepoSpec
	root string
}

// Gather reads the repos and returns the evidence a review file is checked
// against: commit chains, the diff with final-state line numbers, the commit
// that wrote each hunk and language globs. It runs only the read-only Git
// commands validate runs.
func Gather(options GatherOptions) *Evidence {
	evidence := &Evidence{
		Repos:   []*review.Repo{},
		Commits: map[string][]gitx.CommitInfo{},
		Files:   []*FileEvidence{},
	}

	if !gitx.Available() {
		evidence.Findings = append(evidence.Findings, review.NewFinding("git_not_found", "", "no `git` executable on PATH",
			"install Git or add it to PATH"))

		return evidence.finish()
	}

	run := &Engine{
		repoByID:    map[string]*RepoState{},
		Coverage:    map[string][]*FileCoverage{},
		Uncommitted: map[string][]string{},
	}
	run.ReviewPath = gatherReviewPath(options.ReviewPath)
	run.Base = resolvedPath(baseDir(run.ReviewPath))

	targets, findings := resolveTargets(options.Repos)
	if len(findings) > 0 {
		evidence.Findings = findings

		return evidence.finish()
	}

	run.Review = &review.Review{Version: review.SchemaVersion, Repos: targetRepos(run.Base, targets)}
	if envFindings := run.resolveRepos(); len(envFindings) > 0 {
		evidence.Findings = envFindings

		return evidence.finish()
	}

	limits := excerptLimits{contextLines: options.ContextLines, maxHunkLines: options.MaxHunkLines}
	for i, repoState := range run.Repos {
		commits, finding := repoState.gatherCommits(targets[i].spec, options.Tip)
		if finding != nil {
			run.Findings = append(run.Findings, *finding)

			continue
		}

		repoState.Repo.Commits = commits
		run.loadRepo(repoState)
		if !repoState.OK {
			continue
		}

		evidence.addRepo(repoState, limits)
	}

	evidence.Findings = run.Findings

	return evidence.finish()
}

func (e *Evidence) finish() *Evidence {
	e.ExitCode = review.ExitCode(e.Findings)

	return e
}

// gatherReviewPath is the absolute path the review file will have.
func gatherReviewPath(reviewPath string) string {
	if reviewPath == "" {
		workingDir, err := os.Getwd()
		if err != nil {
			return ""
		}

		root := workingDir
		if gitRoot, err := gitx.Toplevel(workingDir); err == nil {
			root = gitRoot
		}

		return filepath.Join(root, filepath.FromSlash(defaultReviewFile))
	}

	abs, err := filepath.Abs(reviewPath)
	if err != nil {
		return reviewPath
	}

	return abs
}

// baseDir is the folder relative repo paths resolve from: the Git root
// holding the review file, or the review file's folder outside Git. Gather
// runs before the review file and its `.review/` folder exist, so the Git
// root is looked up from the nearest folder that does.
func baseDir(reviewPath string) string {
	folder := filepath.Dir(reviewPath)
	if gitRoot, err := gitx.Toplevel(existingAncestor(folder)); err == nil {
		return gitRoot
	}

	return folder
}

// existingAncestor is folder itself when it exists, else its nearest
// existing parent.
func existingAncestor(folder string) string {
	for {
		if _, err := os.Stat(folder); err == nil {
			return folder
		}

		parent := filepath.Dir(folder)
		if parent == folder {
			return folder
		}

		folder = parent
	}
}

// resolveTargets finds each spec's Git root, so a folder inside a repo
// names the repo. A root listed twice keeps its first spec.
func resolveTargets(specs []RepoSpec) ([]gatherTarget, []review.Finding) {
	var targets []gatherTarget
	var findings []review.Finding
	seen := map[string]bool{}
	for _, spec := range specs {
		info, err := os.Stat(spec.Path)
		if err != nil || !info.IsDir() {
			findings = append(findings, review.NewFinding("repo_not_found", "",
				fmt.Sprintf("repo path %s does not exist", spec.Path),
				"pass the folder of a Git repo; relative paths resolve from the current directory"))

			continue
		}

		root, err := gitx.Toplevel(spec.Path)
		if err != nil {
			findings = append(findings, review.NewFinding("not_repo_root", "",
				fmt.Sprintf("%s is not inside a Git repo", spec.Path),
				"pass the folder of a Git repo, or a folder inside one"))

			continue
		}

		if seen[root] {
			continue
		}

		seen[root] = true
		targets = append(targets, gatherTarget{spec: spec, root: root})
	}

	return targets, findings
}

// targetRepos builds the review's repo entries, without commits yet: ids
// from folder names and paths relative to base.
func targetRepos(base string, targets []gatherTarget) []*review.Repo {
	roots := make([]string, len(targets))
	for i, target := range targets {
		roots[i] = target.root
	}

	ids := repoIDs(roots)
	repos := make([]*review.Repo, len(targets))
	for i, target := range targets {
		repo := &review.Repo{ID: ids[i], Path: relativeRepoPath(base, target.root)}
		if target.spec.Context {
			repo.Role = review.RoleContext
		}

		repos[i] = repo
	}

	return repos
}

// relativeRepoPath is root as a POSIX path relative to base, or root itself
// when no relative path exists (another volume).
func relativeRepoPath(base, root string) string {
	relPath, err := filepath.Rel(base, root)
	if err != nil {
		return filepath.ToSlash(root)
	}

	return filepath.ToSlash(relPath)
}

// repoIDs derives one id per root from its folder name, numbering repeats
// ("web", "web-2") so ids stay unique.
func repoIDs(roots []string) []string {
	taken := map[string]bool{}
	ids := make([]string, 0, len(roots))
	for _, root := range roots {
		baseID := idFromName(filepath.Base(root))
		id := baseID
		for suffix := 2; taken[id]; suffix++ {
			id = fmt.Sprintf("%s-%d", baseID, suffix)
		}

		taken[id] = true
		ids = append(ids, id)
	}

	return ids
}

// idFromName turns a folder name into the id pattern: lowercase letters
// and digits joined by single dashes.
func idFromName(name string) string {
	lowered := strings.ToLower(name)
	words := strings.FieldsFunc(lowered, func(char rune) bool {
		isIDChar := char < unicode.MaxASCII && (unicode.IsLetter(char) || unicode.IsDigit(char))

		return !isIDChar
	})

	id := strings.Join(words, "-")
	if len(id) > maxBaseIDLen {
		id = strings.TrimRight(id[:maxBaseIDLen], "-")
	}

	return cmp.Or(id, fallbackRepoID)
}

// gatherCommits works out a repo's commits entry: one commit for a context
// repo; otherwise the first-parent commits from base to tip, with WORKTREE
// first when the tip is HEAD and work is uncommitted. A repo with nothing
// to review returns an empty_change warning instead.
func (s *RepoState) gatherCommits(spec RepoSpec, tip string) ([]string, *review.Finding) {
	if spec.Context {
		return s.contextHead()
	}

	commits, finding := s.committedRange(spec.Base, tip)
	if finding != nil {
		return nil, finding
	}

	worktreeLeads := tip == "" && len(s.uncommittedPaths()) > 0
	if worktreeLeads {
		commits = slices.Concat([]string{review.Worktree}, commits)
	}

	if len(commits) > 0 {
		return commits, nil
	}

	hint := "pass --base <the commit you started from>, or --context if you only read this repo"
	if tip != "" && spec.Base == "" {
		hint = "--tip alone is an empty range; describe one commit with `--base <sha>^ --tip <sha>`"
	}

	emptyFinding := s.finding("empty_change", "",
		fmt.Sprintf("repo %q has no commits in the range and no uncommitted work; it is left out", s.Repo.ID), hint)

	return nil, &emptyFinding
}

// contextHead is the one commit a context repo is read at: its HEAD.
func (s *RepoState) contextHead() ([]string, *review.Finding) {
	head, ok := s.Git.Head()
	if !ok {
		finding := s.finding("bad_base", "", fmt.Sprintf("context repo %q has no commit to read", s.Repo.ID),
			"commit the files you read first, or leave the repo out")

		return nil, &finding
	}

	return []string{head}, nil
}

// committedRange lists the first-parent commits after base up to tip,
// newest first. No base means no commits. The base must sit on the tip's
// first-parent line, or the list would not form the chain validate expects.
func (s *RepoState) committedRange(base, tip string) ([]string, *review.Finding) {
	if base == "" {
		return nil, nil
	}

	tipRev := cmp.Or(tip, "HEAD")
	tipSHA, ok := s.Git.ResolveCommit(tipRev)
	if !ok {
		return nil, s.badBase(fmt.Sprintf("tip %q is not a commit in %s", tipRev, s.Repo.ID),
			"pass --tip a commit of this repo, or leave it out to describe HEAD and uncommitted work")
	}

	baseSHA, ok := s.Git.ResolveCommit(base)
	if !ok {
		return nil, s.badBase(fmt.Sprintf("base %q is not a commit in %s", base, s.Repo.ID),
			"pass the commit the change started from; `git log --oneline --first-parent` lists the candidates")
	}

	commits, err := s.Git.FirstParentList(baseSHA+".."+tipSHA, 0)
	if err != nil {
		return nil, s.badBase(fmt.Sprintf("cannot list commits %s..%s in %s: %v", shortSHA(baseSHA), shortSHA(tipSHA), s.Repo.ID, err),
			"check that base and tip are commits of this repo")
	}

	if len(commits) == 0 {
		if baseSHA == tipSHA {
			return nil, nil
		}

		return nil, s.badBase(fmt.Sprintf("base %s is not before tip %s", shortSHA(baseSHA), shortSHA(tipSHA)),
			"the base must be an ancestor of the tip; swap them or pick an older base")
	}

	oldest := commits[len(commits)-1]
	parent, hasParent := s.Git.FirstParent(oldest)
	if parent == baseSHA {
		return commits, nil
	}

	hint := "the base is not an ancestor of the tip; pick a commit from `git log --first-parent`"
	if hasParent {
		hint = fmt.Sprintf("the tip's first-parent line meets the base's history at %s; pass that as the base", parent)
	}

	return nil, s.badBase(fmt.Sprintf("base %s is not on the first-parent line of %s", shortSHA(baseSHA), shortSHA(tipSHA)), hint)
}

func (s *RepoState) badBase(msg, hint string) *review.Finding {
	finding := s.finding("bad_base", "", msg, hint)

	return &finding
}

// addRepo records a loaded repo: its entry, its commit log and, for a
// changed repo, its files and language globs.
func (e *Evidence) addRepo(repoState *RepoState, limits excerptLimits) {
	repo := repoState.Repo
	e.Repos = append(e.Repos, repo)
	e.Commits[repo.ID] = listedCommits(repoState.Commits, repoState.CommitLog)
	if repo.IsContext() {
		return
	}

	var textFiles []string
	for _, fileDiff := range repoState.Files {
		file := fileDiff.Key()
		var language string
		var finalLines []string
		if !fileDiff.Binary {
			language = langs.ForFile(file)
			textFiles = append(textFiles, file)
		}

		readsFinalState := !fileDiff.Binary && fileDiff.Status != DiffDeleted
		if readsFinalState {
			finalLines = repoState.Read(StateFinal, file).Lines
		}

		fileEvidence := newFileEvidence(repo.ID, fileDiff, finalLines, language, limits)
		for j, hunk := range fileDiff.Hunks {
			fileEvidence.Hunks[j].Commits = repoState.hunkWriters(file, hunk)
		}

		e.Files = append(e.Files, fileEvidence)
	}

	repo.Languages = languageGlobs(textFiles, langs.ForFile)
}

// hunkWriters lists the states that wrote a hunk's added lines. A change
// with a single state is not attributed line by line, so that state wrote
// every added line.
func (s *RepoState) hunkWriters(file string, hunk *Hunk) []string {
	singleState := s.owners == nil
	if singleState && !hunk.PureDeletion() {
		return []string{s.Final}
	}

	return s.HunkCommits(file, hunk)
}

// listedCommits describes commits in list order, skipping any Git could not
// describe.
func listedCommits(commits []string, commitLog map[string]gitx.CommitInfo) []gitx.CommitInfo {
	infos := []gitx.CommitInfo{}
	for _, sha := range commits {
		if info, ok := commitLog[sha]; ok {
			infos = append(infos, info)
		}
	}

	return infos
}

// newFileEvidence describes one changed file from its diff and, unless it
// is deleted or binary, its final-state lines.
func newFileEvidence(repoID string, fileDiff *FileDiff, finalLines []string, language string, limits excerptLimits) *FileEvidence {
	fileRef := &review.Ref{Type: review.RefFile, Repo: repoID, File: fileDiff.Key()}
	fileEvidence := &FileEvidence{
		Repo:      repoID,
		File:      fileDiff.Key(),
		Status:    fileDiff.Status,
		Change:    fileDiff.ChangeState(),
		Binary:    fileDiff.Binary,
		Untracked: fileDiff.Untracked,
		Language:  language,
		Added:     fileDiff.Added(),
		Removed:   fileDiff.Removed(),
		Ref:       fileRef,
	}
	if fileDiff.Status == DiffRenamed {
		fileEvidence.OldFile = fileDiff.OldPath
	}

	for _, hunk := range fileDiff.Hunks {
		fileEvidence.Hunks = append(fileEvidence.Hunks, newHunkEvidence(fileDiff.Status, hunk, finalLines, limits))
	}

	return fileEvidence
}

// newHunkEvidence describes one hunk: its text, cut to the limit, and the
// lines a range ref must cover, with context only when it was asked for.
func newHunkEvidence(status string, hunk *Hunk, finalLines []string, limits excerptLimits) *HunkEvidence {
	added, addedCut := truncateLines(hunk.New, limits.maxHunkLines)
	removed, removedCut := truncateLines(hunk.Old, limits.maxHunkLines)
	hunkEvidence := &HunkEvidence{
		Added:     added,
		Removed:   removed,
		Truncated: addedCut || removedCut,
	}
	if hunk.OldCount > 0 {
		hunkEvidence.OldLines = []int{hunk.OldStart, hunk.OldEnd()}
	}

	emptiedFile := hunk.PureDeletion() && len(finalLines) == 0
	if status == DiffDeleted || emptiedFile {
		return hunkEvidence
	}

	// For a pure deletion the excerpt centres on the removal point: an
	// empty span between NewStart and the line after it.
	first, last := hunk.NewStart, hunk.NewEnd()
	hunkEvidence.Lines = []int{first, last}
	if hunk.PureDeletion() {
		anchor := max(hunk.NewStart, 1)
		hunkEvidence.Lines = []int{anchor, anchor}
		first, last = hunk.NewStart+1, hunk.NewStart
	}

	// A truncated hunk is too long to show whole, so the agent reads the
	// file; in an added file the context would only repeat the added lines.
	showsContext := limits.contextLines > 0 && !hunkEvidence.Truncated && status != DiffAdded
	if showsContext {
		hunkEvidence.Context = contextWindow(finalLines, first, last, limits.contextLines)
	}

	return hunkEvidence
}

// truncateLines keeps at most limit lines; a limit of zero or less keeps
// them all. It reports whether any line was dropped.
func truncateLines(lines []string, limit int) ([]string, bool) {
	if limit <= 0 || len(lines) <= limit {
		return lines, false
	}

	return lines[:limit], true
}

// contextWindow is lines first-last (1-based, inclusive; last may be
// first-1 for an empty span) with size lines around them, clipped to the
// file, or nil when nothing is left.
func contextWindow(lines []string, first, last, size int) *Excerpt {
	start := max(first-size, 1)
	end := min(last+size, len(lines))
	if end < start {
		return nil
	}

	return &Excerpt{Start: start, Lines: lines[start-1 : end]}
}

// languageGlobs proposes the repo's language mapping for the given files:
// one glob per extension when the extension alone picks the language, else
// one per file name, with names first so that they win over extensions.
func languageGlobs(files []string, languageOf func(string) string) review.OrderedMap {
	languageByGlob := map[string]string{}
	for _, file := range files {
		language := languageOf(file)
		if language == "" {
			continue
		}

		languageByGlob[languageGlob(file, language, languageOf)] = language
	}

	globs := slices.SortedFunc(maps.Keys(languageByGlob), func(left, right string) int {
		return cmp.Or(
			cmp.Compare(isExtensionGlob(left), isExtensionGlob(right)),
			strings.Compare(left, right),
		)
	})

	var mapping review.OrderedMap
	for _, glob := range globs {
		mapping = append(mapping, review.KV{Key: glob, Value: languageByGlob[glob]})
	}

	return mapping
}

// languageGlob is the widest glob that keeps file's language: its extension
// when any file with that extension gets the same language, else its name.
func languageGlob(file, language string, languageOf func(string) string) string {
	name := path.Base(file)
	extension := path.Ext(name)
	extensionDecides := extension != "" && extension != name && languageOf("file"+extension) == language
	if extensionDecides {
		return "**/*" + extension
	}

	return "**/" + name
}

// isExtensionGlob reports, as 1 or 0 for sorting, whether a glob matches by
// extension rather than by file name.
func isExtensionGlob(glob string) int {
	if strings.HasPrefix(glob, "**/*.") {
		return 1
	}

	return 0
}
