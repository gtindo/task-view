package engine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
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

// Content is one file read in one state.
type Content struct {
	Exists bool
	Binary bool
	Lines  []string
}

// RepoState is a listed repo with its resolved states and diff.
type RepoState struct {
	Repo  *review.Repo
	Index int
	Abs   string
	Git   gitx.Repo

	// OK is false when the repo or its commit chain could not be resolved;
	// refs into it are then unresolved.
	OK       bool
	Worktree bool
	Commits  []string // full SHAs, WORKTREE excluded, newest first
	Final    string   // SHA, or review.Worktree
	Initial  string   // SHA or the empty tree; equals Final for context repos
	Head     string

	Files     []*FileDiff
	byPath    map[string]*FileDiff
	CommitLog map[string]gitx.CommitInfo

	owners    map[string]map[int]string // final path → line → commit that wrote it
	cache     map[string]*Content
	stamps    map[string]stamp
	reviewRel string // the review file, relative to this repo, when inside it
}

type stamp struct {
	size int64
	mod  time.Time
	ok   bool
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
	for _, f := range s.Files {
		if f.OldPath == path {
			return f
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
	for _, kv := range s.Repo.Languages {
		if langs.MatchGlob(kv.Key, file) {
			return kv.String()
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

// Read returns a file in a state: StateFinal, StateInitial or a SHA.
func (s *RepoState) Read(state, path string) *Content {
	sha := s.StateSHA(state)
	key := sha + "\x00" + path
	if c, ok := s.cache[key]; ok {
		return c
	}
	c := &Content{}
	if sha == review.Worktree {
		full := filepath.Join(s.Abs, filepath.FromSlash(path))
		if st, err := os.Stat(full); err == nil && !st.IsDir() {
			if _, seen := s.stamps[path]; !seen {
				s.stamps[path] = stamp{st.Size(), st.ModTime(), true}
			}
			if data, err := os.ReadFile(full); err == nil {
				c.Exists = true
				c.Binary = IsBinary(data)
				if !c.Binary {
					c.Lines = SplitLines(data)
				}
			}
		} else if _, seen := s.stamps[path]; !seen {
			s.stamps[path] = stamp{}
		}
	} else if sha != gitx.EmptyTree && s.Git.Exists(sha, path) {
		if data, err := s.Git.Show(sha, path); err == nil {
			c.Exists = true
			c.Binary = IsBinary(data)
			if f := s.Diff(path); f != nil && f.Binary {
				c.Binary = true
			}
			if !c.Binary {
				c.Lines = SplitLines(data)
			}
		}
	}
	s.cache[key] = c
	return c
}

// changedWorktreeFiles returns referenced worktree files that changed since
// they were first read during this run.
func (s *RepoState) changedWorktreeFiles() []string {
	var out []string
	for path, before := range s.stamps {
		st, err := os.Stat(filepath.Join(s.Abs, filepath.FromSlash(path)))
		now := stamp{}
		if err == nil {
			now = stamp{st.Size(), st.ModTime(), true}
		}
		if now != before {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// resolveRepos resolves every repo path. It returns exit-3 findings on
// environment errors.
func (e *Engine) resolveRepos() []review.Finding {
	var fs []review.Finding
	for i, repo := range e.Review.Repos {
		path := repo.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(e.Base, filepath.FromSlash(path))
		}
		path = filepath.Clean(path)
		st := &RepoState{Repo: repo, Index: i, Abs: path, Git: gitx.Repo{Dir: path},
			cache: map[string]*Content{}, stamps: map[string]stamp{}}
		e.Repos = append(e.Repos, st)
		e.repoByID[repo.ID] = st

		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			f := review.NewFinding("repo_not_found", fmt.Sprintf("/repos/%d/path", i),
				fmt.Sprintf("repo %q: path %s does not exist", repo.ID, path),
				fmt.Sprintf("relative paths resolve from %s, the Git root containing the review file", e.Base))
			f.Repo = repo.ID
			fs = append(fs, f)
			continue
		}
		root, err := gitx.IsRepoRoot(path)
		if err != nil || !root {
			hint := "point `path` at the directory that contains `.git`"
			if top, err := gitx.Toplevel(path); err == nil {
				hint = fmt.Sprintf("the Git root is %s; point `path` at it", top)
			}
			f := review.NewFinding("not_repo_root", fmt.Sprintf("/repos/%d/path", i),
				fmt.Sprintf("repo %q: %s is not the root of a Git repo", repo.ID, path), hint)
			f.Repo = repo.ID
			fs = append(fs, f)
			continue
		}
		if e.ReviewPath != "" {
			if rel, err := filepath.Rel(path, e.ReviewPath); err == nil && !strings.HasPrefix(rel, "..") {
				st.reviewRel = filepath.ToSlash(rel)
			}
		}
	}
	return fs
}

// checkChain validates the commit list of one repo and sets its states.
func (e *Engine) checkChain(s *RepoState) []review.Finding {
	var fs []review.Finding
	repo := s.Repo
	base := fmt.Sprintf("/repos/%d/commits", s.Index)
	add := func(code, path, msg, hint string) {
		f := review.NewFinding(code, path, msg, hint)
		f.Repo = repo.ID
		fs = append(fs, f)
	}

	s.Head, _ = s.Git.Head()

	if repo.IsContext() {
		if len(repo.Commits) != 1 || repo.Commits[0] == review.Worktree {
			add("context_commits", base, fmt.Sprintf("context repo %q must list exactly one SHA", repo.ID),
				fmt.Sprintf("set commits to the one commit you read: [%q]", s.Head))
			return fs
		}
		sha, ok := s.Git.ResolveCommit(repo.Commits[0])
		if !ok {
			add("commit_not_found", base+"/0", fmt.Sprintf("commit %s not found in %s", repo.Commits[0], repo.ID),
				fmt.Sprintf("use the commit you read: `git rev-parse HEAD` gives %s", s.Head))
			return fs
		}
		s.Commits = []string{sha}
		s.Final, s.Initial = sha, sha
		s.OK = true
		s.loadLog()
		return fs
	}

	shas := repo.Commits
	for i, c := range repo.Commits {
		if c == review.Worktree {
			if i != 0 {
				add("commit_chain_broken", fmt.Sprintf("%s/%d", base, i), "WORKTREE may only be the first entry",
					"move \"WORKTREE\" to commits[0]: it is the newest state")
				return fs
			}
			s.Worktree = true
		}
	}
	if s.Worktree {
		shas = shas[1:]
	}

	resolved := make([]string, len(shas))
	missing := false
	for i, c := range shas {
		full, ok := s.Git.ResolveCommit(c)
		if !ok {
			idx := i
			if s.Worktree {
				idx++
			}
			add("commit_not_found", fmt.Sprintf("%s/%d", base, idx), fmt.Sprintf("commit %s not found in %s", c, repo.ID),
				"list full SHAs that exist in this repo: `git rev-list --first-parent <start>..HEAD`")
			missing = true
			continue
		}
		resolved[i] = full
	}
	if missing {
		return fs
	}

	expected := func() string {
		if len(resolved) == 0 {
			return ""
		}
		newest := resolved[0]
		if s.Worktree && s.Head != "" {
			newest = s.Head
		}
		for _, c := range resolved {
			if s.Head != "" && c == s.Head {
				newest = s.Head
			}
		}
		list, err := s.Git.FirstParentList(newest, len(resolved))
		if err != nil {
			return ""
		}
		return "[" + quoteJoin(list) + "]"
	}

	for i := 0; i+1 < len(resolved); i++ {
		parent, _ := s.Git.FirstParent(resolved[i])
		if parent != resolved[i+1] {
			idx := i + 1
			if s.Worktree {
				idx++
			}
			hint := "commits must be contiguous first-parent order, newest first"
			if rev := reversed(resolved); chainOK(s, rev) {
				hint = "the list is oldest first; reverse it so commits[0] is the newest"
			} else if exp := expected(); exp != "" {
				hint += "; `git rev-list --first-parent` gives " + exp
			}
			add("commit_chain_broken", fmt.Sprintf("%s/%d", base, idx),
				fmt.Sprintf("%s is not the first parent of %s", short(resolved[i+1]), short(resolved[i])), hint)
			return fs
		}
	}

	if s.Worktree {
		if len(resolved) > 0 && resolved[0] != s.Head {
			add("commit_chain_broken", base+"/1",
				fmt.Sprintf("WORKTREE's parent is HEAD (%s) but commits[1] is %s", short(s.Head), short(resolved[0])),
				"after \"WORKTREE\", list commits from HEAD backwards: `git rev-list --first-parent <start>..HEAD`")
			return fs
		}
	}

	s.Commits = resolved
	switch {
	case s.Worktree && len(resolved) == 0:
		s.Final = review.Worktree
		s.Initial = s.Head
		if s.Initial == "" {
			s.Initial = gitx.EmptyTree
		}
	default:
		if s.Worktree {
			s.Final = review.Worktree
		} else {
			s.Final = resolved[0]
		}
		last := resolved[len(resolved)-1]
		if p, ok := s.Git.FirstParent(last); ok {
			s.Initial = p
		} else {
			s.Initial = gitx.EmptyTree
		}
	}
	s.OK = true
	s.loadLog()

	// Uncommitted work outside the review, and later commits.
	if entries, err := s.Git.Status(); err == nil && !s.Worktree {
		var paths []string
		for _, en := range entries {
			if s.skipPath(en.Path) {
				continue
			}
			paths = append(paths, en.Path)
		}
		if len(paths) > 0 {
			f := review.NewFinding("uncommitted_changes", base,
				fmt.Sprintf("repo %q has %d uncommitted %s not part of the review", repo.ID, len(paths), plural(len(paths), "path", "paths")),
				"if they belong to the change, put \"WORKTREE\" first in commits and describe them; otherwise they show as unreviewed work")
			f.Repo = repo.ID
			e.uncommitted[repo.ID] = paths
			fs = append(fs, f)
		}
	}
	if !s.Worktree && s.Head != "" && s.Head != s.Final {
		if later, err := s.Git.FirstParentList(s.Final+".."+s.Head, 0); err == nil && len(later) > 0 {
			f := review.NewFinding("head_ahead", base,
				fmt.Sprintf("the branch has %d %s after commits[0] that the review does not cover", len(later), plural(len(later), "commit", "commits")),
				"add the later commits to the front of commits, or check out commits[0]")
			f.Repo = repo.ID
			fs = append(fs, f)
		}
	}
	return fs
}

// skipPath reports whether a path is the review file itself or sits in the
// review folder (`.review/`), which are never part of the change.
func (s *RepoState) skipPath(p string) bool {
	if s.reviewRel == "" {
		return false
	}
	p = strings.TrimSuffix(p, "/")
	if p == s.reviewRel || strings.HasPrefix(s.reviewRel, p+"/") {
		return true
	}
	dir := path.Dir(s.reviewRel)
	return path.Base(dir) == ".review" && strings.HasPrefix(p, dir+"/")
}

func (s *RepoState) loadLog() {
	shas := append([]string{}, s.Commits...)
	if s.Initial != "" && s.Initial != gitx.EmptyTree && s.Initial != s.Final {
		shas = append(shas, s.Initial)
	}
	s.CommitLog, _ = s.Git.Log(shas)
	if s.CommitLog == nil {
		s.CommitLog = map[string]gitx.CommitInfo{}
	}
}

// loadDiff computes the diff from the initial to the final state.
func (e *Engine) loadDiff(s *RepoState) error {
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
	for _, f := range ParseDiff(string(out)) {
		if s.skipPath(f.Key()) {
			continue
		}
		s.Files = append(s.Files, f)
	}
	if s.Worktree {
		untracked, err := s.Git.Untracked()
		if err != nil {
			return err
		}
		for _, p := range untracked {
			if s.skipPath(p) {
				continue
			}
			data, err := os.ReadFile(filepath.Join(s.Abs, filepath.FromSlash(p)))
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return err
			}
			f := &FileDiff{Path: p, Status: DiffAdded, Untracked: true, Binary: IsBinary(data)}
			if !f.Binary {
				lines := SplitLines(data)
				if len(lines) > 0 {
					f.Hunks = []*Hunk{{OldStart: 0, OldCount: 0, NewStart: 1, NewCount: len(lines), New: lines}}
				}
			}
			s.Files = append(s.Files, f)
		}
	}
	sort.SliceStable(s.Files, func(i, j int) bool { return s.Files[i].Key() < s.Files[j].Key() })
	for _, f := range s.Files {
		s.byPath[f.Key()] = f
	}
	return nil
}

func chainOK(s *RepoState, list []string) bool {
	for i := 0; i+1 < len(list); i++ {
		if p, _ := s.Git.FirstParent(list[i]); p != list[i+1] {
			return false
		}
	}
	return len(list) > 1
}

func reversed(in []string) []string {
	out := make([]string, len(in))
	for i, v := range in {
		out[len(in)-1-i] = v
	}
	return out
}

func quoteJoin(list []string) string {
	q := make([]string, len(list))
	for i, v := range list {
		q[i] = `"` + v + `"`
	}
	return strings.Join(q, ", ")
}

func short(sha string) string {
	if len(sha) > 10 {
		return sha[:10]
	}
	return sha
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
