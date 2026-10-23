package engine

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gtindo/changes-view/internal/gitx"
	"github.com/gtindo/changes-view/internal/langs"
	"github.com/gtindo/changes-view/internal/review"
)

// State names used when reading files.
const (
	StateFinal   = "final"
	StateInitial = "initial"
)

// shortSHALen is how many SHA characters messages show.
const shortSHALen = 10

// Content is one file read in one state.
type Content struct {
	// Exists is false when the file is missing in that state.
	Exists bool
	// Binary files carry no Lines.
	Binary bool
	// Lines holds the text without line breaks.
	Lines []string
}

// RepoState is a listed repo with its resolved states and diff.
type RepoState struct {
	// Repo is the entry from the review file.
	Repo *review.Repo
	// Index is the repo's position in the review's repos, for JSON paths.
	Index int
	// Abs is the repo's absolute root.
	Abs string
	// Git reads the repo.
	Git gitx.Repo

	// OK is false when the repo or its commit chain could not be resolved;
	// refs into it are then unresolved.
	OK bool
	// Worktree is true when the change ends in uncommitted work.
	Worktree bool
	Commits  []string // full SHAs, WORKTREE excluded, newest first
	Final    string   // SHA, or review.Worktree
	Initial  string   // SHA or the empty tree; equals Final for context repos
	// Head is the repo's HEAD SHA, or empty when it has none.
	Head string

	// Files is the diff from the initial to the final state, by path.
	Files  []*FileDiff
	byPath map[string]*FileDiff
	// CommitLog describes the listed commits and the initial commit.
	CommitLog map[string]gitx.CommitInfo

	owners    lineOwners
	cache     map[string]*Content
	stamps    map[string]stamp
	reviewRel string // the review file, relative to this repo, when inside it
}

// stamp is what a worktree file looked like when it was first read, to
// notice edits made while the run is in progress.
type stamp struct {
	size    int64
	modTime time.Time
	exists  bool
}

func stampOf(info fs.FileInfo) stamp {
	return stamp{
		size:    info.Size(),
		modTime: info.ModTime(),
		exists:  true,
	}
}

// Diff returns the file diff for a path (final path, or initial path of a
// deleted file), or nil when the file is not in the diff.
func (s *RepoState) Diff(path string) *FileDiff {
	if s.byPath == nil {
		return nil
	}

	return s.byPath[path]
}

// DiffByOld finds a file diff by its initial path.
func (s *RepoState) DiffByOld(path string) *FileDiff {
	for _, fileDiff := range s.Files {
		if fileDiff.OldPath == path {
			return fileDiff
		}
	}

	return nil
}

// Language returns the language for a file: the ref override, else the
// first matching repo glob, else "".
func (s *RepoState) Language(file, override string) string {
	if override != "" {
		return override
	}

	for _, language := range s.Repo.Languages {
		if langs.MatchGlob(language.Key, file) {
			return language.String()
		}
	}

	return ""
}

// StateSHA maps a state name onto the commit it reads (or WORKTREE).
func (s *RepoState) StateSHA(state string) string {
	switch state {
	case StateFinal:
		return s.Final
	case StateInitial:
		return s.Initial
	}

	return state
}

// hasDiff reports whether the repo resolved and is part of the change, so
// that coverage and attribution apply to it.
func (s *RepoState) hasDiff() bool { return s.OK && !s.Repo.IsContext() }

// diskPath is where a repo-relative path lives on disk.
func (s *RepoState) diskPath(file string) string {
	return filepath.Join(s.Abs, filepath.FromSlash(file))
}

// Read returns a file in a state: StateFinal, StateInitial or a SHA.
func (s *RepoState) Read(state, path string) *Content {
	sha := s.StateSHA(state)
	key := sha + "\x00" + path
	if content, ok := s.cache[key]; ok {
		return content
	}

	var content *Content
	switch {
	case sha == review.Worktree:
		content = s.readWorktree(path)
	case sha != gitx.EmptyTree && s.Git.Exists(sha, path):
		content = s.readCommit(sha, path)
	default:
		content = &Content{}
	}

	s.cache[key] = content

	return content
}

// readWorktree reads a file from disk, stamping it on first sight.
func (s *RepoState) readWorktree(path string) *Content {
	full := s.diskPath(path)
	info, err := os.Stat(full)
	if err != nil || info.IsDir() {
		s.stampOnce(path, stamp{})

		return &Content{}
	}

	s.stampOnce(path, stampOf(info))
	data, err := os.ReadFile(full)
	if err != nil {
		return &Content{}
	}

	return newContent(data, IsBinary(data))
}

// readCommit reads a file at a commit. A file the diff calls binary is
// binary even when its bytes pass the NUL heuristic.
func (s *RepoState) readCommit(sha, path string) *Content {
	data, err := s.Git.Show(sha, path)
	if err != nil {
		return &Content{}
	}

	fileDiff := s.Diff(path)
	diffSaysBinary := fileDiff != nil && fileDiff.Binary

	return newContent(data, IsBinary(data) || diffSaysBinary)
}

func newContent(data []byte, binary bool) *Content {
	content := &Content{Exists: true, Binary: binary}
	if !binary {
		content.Lines = SplitLines(data)
	}

	return content
}

// stampOnce keeps the first stamp of a path, so later reads compare with
// the file as it was when the run first saw it.
func (s *RepoState) stampOnce(path string, first stamp) {
	if _, seen := s.stamps[path]; !seen {
		s.stamps[path] = first
	}
}

// changedWorktreeFiles returns referenced worktree files that changed since
// they were first read during this run.
func (s *RepoState) changedWorktreeFiles() []string {
	var changed []string
	for path, before := range s.stamps {
		now := stamp{}
		if info, err := os.Stat(s.diskPath(path)); err == nil {
			now = stampOf(info)
		}

		if now != before {
			changed = append(changed, path)
		}
	}

	slices.Sort(changed)

	return changed
}

// resolveRepos resolves every repo path. It returns exit-3 findings on
// environment errors.
func (e *Engine) resolveRepos() []review.Finding {
	var findings []review.Finding
	for i, repo := range e.Review.Repos {
		repoPath := repo.Path
		if !filepath.IsAbs(repoPath) {
			repoPath = filepath.Join(e.Base, filepath.FromSlash(repoPath))
		}

		repoPath = filepath.Clean(repoPath)
		repoState := &RepoState{
			Repo:   repo,
			Index:  i,
			Abs:    repoPath,
			Git:    gitx.Repo{Dir: repoPath},
			cache:  map[string]*Content{},
			stamps: map[string]stamp{},
		}
		e.Repos = append(e.Repos, repoState)
		e.repoByID[repo.ID] = repoState

		pathPointer := fmt.Sprintf("/repos/%d/path", i)
		info, err := os.Stat(repoPath)
		if err != nil || !info.IsDir() {
			findings = append(findings, repoFinding(repo.ID, "repo_not_found", pathPointer,
				fmt.Sprintf("repo %q: path %s does not exist", repo.ID, repoPath),
				fmt.Sprintf("relative paths resolve from %s, the Git root containing the review file", e.Base)))

			continue
		}

		isRoot, err := gitx.IsRepoRoot(repoPath)
		if err != nil || !isRoot {
			hint := "point `path` at the directory that contains `.git`"
			if gitRoot, err := gitx.Toplevel(repoPath); err == nil {
				hint = fmt.Sprintf("the Git root is %s; point `path` at it", gitRoot)
			}

			findings = append(findings, repoFinding(repo.ID, "not_repo_root", pathPointer,
				fmt.Sprintf("repo %q: %s is not the root of a Git repo", repo.ID, repoPath), hint))

			continue
		}

		if e.ReviewPath == "" {
			continue
		}

		relPath, err := filepath.Rel(repoPath, e.ReviewPath)
		reviewInsideRepo := err == nil && !strings.HasPrefix(relPath, "..")
		if reviewInsideRepo {
			repoState.reviewRel = filepath.ToSlash(relPath)
		}
	}

	return findings
}

// repoFinding is a finding about a repo as a whole.
func repoFinding(repoID, code, path, msg, hint string) review.Finding {
	finding := review.NewFinding(code, path, msg, hint)
	finding.Repo = repoID

	return finding
}

// commitsPath is the JSON path of the repo's commits list.
func (s *RepoState) commitsPath() string { return fmt.Sprintf("/repos/%d/commits", s.Index) }

// resolvedCommitPath is the JSON path of the commits entry that holds the
// resolved SHA at index i, which sits one further when WORKTREE leads.
func (s *RepoState) resolvedCommitPath(i int) string {
	entry := i
	if s.Worktree {
		entry++
	}

	return fmt.Sprintf("%s/%d", s.commitsPath(), entry)
}

func (s *RepoState) finding(code, path, msg, hint string) review.Finding {
	return repoFinding(s.Repo.ID, code, path, msg, hint)
}

// checkChain validates the commit list of one repo and sets its states.
func (e *Engine) checkChain(repoState *RepoState) []review.Finding {
	repoState.Head, _ = repoState.Git.Head()

	if repoState.Repo.IsContext() {
		return repoState.resolveContextCommit()
	}

	resolved, findings := repoState.resolveChain()
	if len(findings) > 0 {
		return findings
	}

	repoState.setStates(resolved)
	repoState.OK = true
	repoState.loadLog()

	return e.checkOutsideReview(repoState)
}

// resolveContextCommit sets the one state a context repo is read at.
func (s *RepoState) resolveContextCommit() []review.Finding {
	commits := s.Repo.Commits
	if len(commits) != 1 || commits[0] == review.Worktree {
		return []review.Finding{s.finding("context_commits", s.commitsPath(),
			fmt.Sprintf("context repo %q must list exactly one SHA", s.Repo.ID),
			fmt.Sprintf("set commits to the one commit you read: [%q]", s.Head))}
	}

	sha, ok := s.Git.ResolveCommit(commits[0])
	if !ok {
		return []review.Finding{s.finding("commit_not_found", s.commitsPath()+"/0",
			fmt.Sprintf("commit %s not found in %s", commits[0], s.Repo.ID),
			fmt.Sprintf("use the commit you read: `git rev-parse HEAD` gives %s", s.Head))}
	}

	s.Commits = []string{sha}
	s.Final, s.Initial = sha, sha
	s.OK = true
	s.loadLog()

	return nil
}

// resolveChain resolves a changed repo's commits to full SHAs, newest
// first, and checks that they form one first-parent chain that starts at
// HEAD when WORKTREE leads. It stops at the first kind of problem.
func (s *RepoState) resolveChain() ([]string, []review.Finding) {
	shas, finding := s.splitWorktree()
	if finding != nil {
		return nil, []review.Finding{*finding}
	}

	resolved, findings := s.resolveCommits(shas)
	if len(findings) > 0 {
		return nil, findings
	}

	if finding := s.checkFirstParents(resolved); finding != nil {
		return nil, []review.Finding{*finding}
	}

	if finding := s.checkWorktreeParent(resolved); finding != nil {
		return nil, []review.Finding{*finding}
	}

	return resolved, nil
}

// splitWorktree sets Worktree from a leading WORKTREE entry and returns the
// listed commits after it.
func (s *RepoState) splitWorktree() ([]string, *review.Finding) {
	commits := s.Repo.Commits
	for i, commit := range commits {
		if commit != review.Worktree {
			continue
		}

		if i != 0 {
			finding := s.finding("commit_chain_broken", fmt.Sprintf("%s/%d", s.commitsPath(), i),
				"WORKTREE may only be the first entry", "move \"WORKTREE\" to commits[0]: it is the newest state")

			return nil, &finding
		}

		s.Worktree = true
	}

	if s.Worktree {
		return commits[1:], nil
	}

	return commits, nil
}

// resolveCommits expands listed SHAs to full SHAs, reporting every one
// that does not exist.
func (s *RepoState) resolveCommits(shas []string) ([]string, []review.Finding) {
	resolved := make([]string, len(shas))
	var findings []review.Finding
	for i, sha := range shas {
		full, ok := s.Git.ResolveCommit(sha)
		if !ok {
			findings = append(findings, s.finding("commit_not_found", s.resolvedCommitPath(i),
				fmt.Sprintf("commit %s not found in %s", sha, s.Repo.ID),
				"list full SHAs that exist in this repo: `git rev-list --first-parent <start>..HEAD`"))

			continue
		}

		resolved[i] = full
	}

	return resolved, findings
}

// checkFirstParents reports the first commit whose successor in the list is
// not its first parent.
func (s *RepoState) checkFirstParents(resolved []string) *review.Finding {
	for i := range len(resolved) - 1 {
		parent, _ := s.Git.FirstParent(resolved[i])
		if parent == resolved[i+1] {
			continue
		}

		finding := s.finding("commit_chain_broken", s.resolvedCommitPath(i+1),
			fmt.Sprintf("%s is not the first parent of %s", shortSHA(resolved[i+1]), shortSHA(resolved[i])),
			s.chainHint(resolved))

		return &finding
	}

	return nil
}

// chainHint suggests how to fix a broken chain: reverse an oldest-first
// list, or use the chain Git reports.
func (s *RepoState) chainHint(resolved []string) string {
	reversed := slices.Clone(resolved)
	slices.Reverse(reversed)
	if s.isFirstParentChain(reversed) {
		return "the list is oldest first; reverse it so commits[0] is the newest"
	}

	hint := "commits must be contiguous first-parent order, newest first"
	if expected := s.expectedChain(resolved); expected != "" {
		hint += "; `git rev-list --first-parent` gives " + expected
	}

	return hint
}

// expectedChain is the first-parent chain of the same length the agent
// probably meant, as a JSON list: from HEAD when HEAD is listed or WORKTREE
// leads, else from commits[0]. Empty when Git cannot list it.
func (s *RepoState) expectedChain(resolved []string) string {
	newest := resolved[0]
	headListed := slices.Contains(resolved, s.Head)
	if s.Head != "" && (s.Worktree || headListed) {
		newest = s.Head
	}

	list, err := s.Git.FirstParentList(newest, len(resolved))
	if err != nil {
		return ""
	}

	return "[" + quoteJoin(list) + "]"
}

// checkWorktreeParent requires the newest listed commit to be HEAD when
// WORKTREE leads, since uncommitted work sits on top of HEAD.
func (s *RepoState) checkWorktreeParent(resolved []string) *review.Finding {
	headMismatch := s.Worktree && len(resolved) > 0 && resolved[0] != s.Head
	if !headMismatch {
		return nil
	}

	finding := s.finding("commit_chain_broken", s.commitsPath()+"/1",
		fmt.Sprintf("WORKTREE's parent is HEAD (%s) but commits[1] is %s", shortSHA(s.Head), shortSHA(resolved[0])),
		"after \"WORKTREE\", list commits from HEAD backwards: `git rev-list --first-parent <start>..HEAD`")

	return &finding
}

// setStates derives the final and initial states from a valid chain: the
// initial state is the parent of the oldest commit, or HEAD when the change
// is only uncommitted work, or the empty tree when there is no parent.
func (s *RepoState) setStates(resolved []string) {
	s.Commits = resolved
	if s.Worktree && len(resolved) == 0 {
		s.Final = review.Worktree
		s.Initial = cmp.Or(s.Head, gitx.EmptyTree)

		return
	}

	s.Final = resolved[0]
	if s.Worktree {
		s.Final = review.Worktree
	}

	s.Initial = gitx.EmptyTree
	if parent, ok := s.Git.FirstParent(resolved[len(resolved)-1]); ok {
		s.Initial = parent
	}
}

// checkOutsideReview flags work a commit-only review does not describe:
// uncommitted paths and commits after commits[0].
func (e *Engine) checkOutsideReview(repoState *RepoState) []review.Finding {
	if repoState.Worktree {
		return nil
	}

	var findings []review.Finding
	repoID := repoState.Repo.ID
	if paths := repoState.uncommittedPaths(); len(paths) > 0 {
		e.Uncommitted[repoID] = paths
		findings = append(findings, repoState.finding("uncommitted_changes", repoState.commitsPath(),
			fmt.Sprintf("repo %q has %d uncommitted %s not part of the review", repoID, len(paths), plural(len(paths), "path", "paths")),
			"if they belong to the change, put \"WORKTREE\" first in commits and describe them; otherwise they show as unreviewed work"))
	}

	if later := repoState.laterCommits(); len(later) > 0 {
		findings = append(findings, repoState.finding("head_ahead", repoState.commitsPath(),
			fmt.Sprintf("the branch has %d %s after commits[0] that the review does not cover", len(later), plural(len(later), "commit", "commits")),
			"add the later commits to the front of commits, or check out commits[0]"))
	}

	return findings
}

// uncommittedPaths lists paths with uncommitted changes, leaving out the
// review file and folder.
func (s *RepoState) uncommittedPaths() []string {
	entries, err := s.Git.Status()
	if err != nil {
		return nil
	}

	var paths []string
	for _, entry := range entries {
		if s.skipPath(entry.Path) {
			continue
		}

		paths = append(paths, entry.Path)
	}

	return paths
}

// laterCommits lists commits on the branch after the final state.
func (s *RepoState) laterCommits() []string {
	if s.Head == "" || s.Head == s.Final {
		return nil
	}

	later, err := s.Git.FirstParentList(s.Final+".."+s.Head, 0)
	if err != nil {
		return nil
	}

	return later
}

// skipPath reports whether a path is the review file itself or sits in the
// review folder (`.review/`), which are never part of the change.
func (s *RepoState) skipPath(file string) bool {
	if s.reviewRel == "" {
		return false
	}

	file = strings.TrimSuffix(file, "/")
	isReviewFileOrParent := file == s.reviewRel || strings.HasPrefix(s.reviewRel, file+"/")
	if isReviewFileOrParent {
		return true
	}

	reviewDir := path.Dir(s.reviewRel)

	return path.Base(reviewDir) == ".review" && strings.HasPrefix(file, reviewDir+"/")
}

func (s *RepoState) loadLog() {
	shas := slices.Clone(s.Commits)
	initialIsSeparateCommit := s.Initial != "" && s.Initial != gitx.EmptyTree && s.Initial != s.Final
	if initialIsSeparateCommit {
		shas = append(shas, s.Initial)
	}

	s.CommitLog, _ = s.Git.Log(shas)
	if s.CommitLog == nil {
		s.CommitLog = map[string]gitx.CommitInfo{}
	}
}

// loadDiff computes the diff from the initial to the final state.
func (s *RepoState) loadDiff() error {
	s.byPath = map[string]*FileDiff{}
	if s.Repo.IsContext() {
		return nil
	}

	to := s.Final
	if s.Worktree {
		to = ""
	}

	out, err := s.Git.Diff(s.Initial, to)
	if err != nil {
		return err
	}

	for _, fileDiff := range ParseDiff(string(out)) {
		if s.skipPath(fileDiff.Key()) {
			continue
		}

		s.Files = append(s.Files, fileDiff)
	}

	if s.Worktree {
		untracked, err := s.untrackedDiffs()
		s.Files = append(s.Files, untracked...)
		if err != nil {
			return err
		}
	}

	slices.SortStableFunc(s.Files, func(left, right *FileDiff) int { return strings.Compare(left.Key(), right.Key()) })
	for _, fileDiff := range s.Files {
		s.byPath[fileDiff.Key()] = fileDiff
	}

	return nil
}

// untrackedDiffs reads the untracked files of a WORKTREE change, which
// `git diff` leaves out. On a read error it returns the files read so far.
func (s *RepoState) untrackedDiffs() ([]*FileDiff, error) {
	untracked, err := s.Git.Untracked()
	if err != nil {
		return nil, err
	}

	var diffs []*FileDiff
	for _, file := range untracked {
		if s.skipPath(file) {
			continue
		}

		data, err := os.ReadFile(s.diskPath(file))
		if errors.Is(err, fs.ErrNotExist) {
			// Deleted between `git ls-files` and the read.
			continue
		}

		if err != nil {
			return diffs, err
		}

		diffs = append(diffs, untrackedDiff(file, data))
	}

	return diffs, nil
}

// untrackedDiff is the diff of an untracked file: added whole, as one hunk
// over all its lines unless it is binary or empty.
func untrackedDiff(file string, data []byte) *FileDiff {
	fileDiff := &FileDiff{
		Path:      file,
		Status:    DiffAdded,
		Untracked: true,
		Binary:    IsBinary(data),
	}
	if fileDiff.Binary {
		return fileDiff
	}

	if lines := SplitLines(data); len(lines) > 0 {
		fileDiff.Hunks = []*Hunk{{NewStart: 1, NewCount: len(lines), New: lines}}
	}

	return fileDiff
}

// isFirstParentChain reports whether list has at least two commits, each
// the first parent of the one before.
func (s *RepoState) isFirstParentChain(list []string) bool {
	if len(list) < 2 {
		return false
	}

	for i := range len(list) - 1 {
		if parent, _ := s.Git.FirstParent(list[i]); parent != list[i+1] {
			return false
		}
	}

	return true
}

func quoteJoin(list []string) string {
	quoted := make([]string, len(list))
	for i, item := range list {
		quoted[i] = `"` + item + `"`
	}

	return strings.Join(quoted, ", ")
}

func shortSHA(sha string) string {
	if len(sha) > shortSHALen {
		return sha[:shortSHALen]
	}

	return sha
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}

	return many
}
