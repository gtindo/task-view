// Package render turns a checked review into one self-contained HTML page.
package render

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	"github.com/gtindo/changes-view/internal/engine"
	"github.com/gtindo/changes-view/internal/gitx"
	"github.com/gtindo/changes-view/internal/langs"
	"github.com/gtindo/changes-view/internal/review"
)

// Row is one line of a code excerpt or hunk.
type Row struct {
	T string `json:"t"`           // ctx, add, del, gap
	O int    `json:"o,omitempty"` // initial-state line number
	N int    `json:"n,omitempty"` // final-state (or read-state) line number
	H string `json:"h"`           // highlighted HTML
	F bool   `json:"f,omitempty"` // inside the referenced range
}

type excerpt struct {
	Repo      string `json:"repo"`
	File      string `json:"file"`
	OldPath   string `json:"oldPath,omitempty"`
	Language  string `json:"language,omitempty"`
	State     string `json:"state"`
	StateKind string `json:"stateKind"`
	Rows      []Row  `json:"rows"`
	Note      string `json:"note,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
	// After is the whole final file for a `file` ref whose rows show only
	// its hunks; for other refs the page drops the removed rows instead.
	After          []Row `json:"after,omitempty"`
	AfterTruncated bool  `json:"afterTruncated,omitempty"`
}

type commitView struct {
	SHA      string `json:"sha"`
	Short    string `json:"short"`
	Subject  string `json:"subject"`
	Author   string `json:"author,omitempty"`
	Date     string `json:"date,omitempty"`
	Worktree bool   `json:"worktree,omitempty"`
}

type repoView struct {
	ID          string       `json:"id"`
	Path        string       `json:"path"`
	Abs         string       `json:"abs"`
	Role        string       `json:"role"`
	OK          bool         `json:"ok"`
	Worktree    bool         `json:"worktree"`
	Commits     []commitView `json:"commits"`
	Base        *commitView  `json:"base,omitempty"`
	Files       int          `json:"files"`
	Added       int          `json:"added"`
	Removed     int          `json:"removed"`
	Uncommitted []string     `json:"uncommitted,omitempty"`
}

type kv struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type nodeView struct {
	ID      string            `json:"id"`
	Kind    string            `json:"kind"`
	Label   string            `json:"label"`
	Change  string            `json:"change"`
	Why     string            `json:"why,omitempty"`
	Meta    []kv              `json:"meta,omitempty"`
	Ref     *review.Ref       `json:"ref,omitempty"`
	Result  *engine.RefResult `json:"result"`
	Excerpt string            `json:"excerpt,omitempty"`
	Context bool              `json:"context,omitempty"`
}

type edgeView struct {
	ID      string            `json:"id"`
	From    string            `json:"from"`
	To      string            `json:"to"`
	Kind    string            `json:"kind"`
	Ref     *review.Ref       `json:"ref,omitempty"`
	Result  *engine.RefResult `json:"result"`
	Excerpt string            `json:"excerpt,omitempty"`
}

type evidenceView struct {
	Ref     *review.Ref       `json:"ref"`
	Result  *engine.RefResult `json:"result"`
	Excerpt string            `json:"excerpt,omitempty"`
}

type claimView struct {
	Text     string         `json:"text"`
	About    []string       `json:"about"`
	Status   string         `json:"status"`
	Evidence []evidenceView `json:"evidence"`
}

type hunkView struct {
	ID        string   `json:"id"`
	OldStart  int      `json:"oldStart"`
	OldCount  int      `json:"oldCount"`
	NewStart  int      `json:"newStart"`
	NewCount  int      `json:"newCount"`
	Coverage  string   `json:"coverage"`
	Nodes     []string `json:"nodes"`
	TestNodes []string `json:"testNodes,omitempty"`
	Uncovered [][2]int `json:"uncovered,omitempty"`
	OldSide   bool     `json:"oldSide,omitempty"`
	Commits   []string `json:"commits,omitempty"`
	Rows      []Row    `json:"rows"`
}

type fileView struct {
	ID        string     `json:"id"`
	Repo      string     `json:"repo"`
	Path      string     `json:"path"`
	OldPath   string     `json:"oldPath,omitempty"`
	Status    string     `json:"status"`
	Binary    bool       `json:"binary,omitempty"`
	Untracked bool       `json:"untracked,omitempty"`
	Language  string     `json:"language,omitempty"`
	Added     int        `json:"added"`
	Removed   int        `json:"removed"`
	Lines     int        `json:"lines"`
	Coverage  string     `json:"coverage"`
	FileNodes []string   `json:"fileNodes,omitempty"`
	HasTests  bool       `json:"hasTests,omitempty"`
	Hunks     []hunkView `json:"hunks"`
}

type viewData struct {
	Version    string                `json:"version"`
	Generated  string                `json:"generated"`
	ReviewPath string                `json:"reviewPath"`
	ReviewName string                `json:"reviewName"`
	ExitCode   int                   `json:"exitCode"`
	Summary    string                `json:"summary"`
	Notes      []string              `json:"notes"`
	Repos      []repoView            `json:"repos"`
	Nodes      []nodeView            `json:"nodes"`
	Edges      []edgeView            `json:"edges"`
	Flows      []*review.Flow        `json:"flows"`
	Claims     []claimView           `json:"claims"`
	Files      []fileView            `json:"files"`
	Excerpts   map[string]*excerpt   `json:"excerpts"`
	Errors     []review.Finding      `json:"errors"`
	Warnings   []review.Finding      `json:"warnings"`
	Counts     engine.Summary        `json:"counts"`
	Unlisted   []engine.UnlistedRepo `json:"unlisted"`
	Untested   []string              `json:"untested"`
}

const ctxLines = 3
const maxRows = 600

type builder struct {
	e        *engine.Engine
	hl       map[string][]string
	excerpts map[string]*excerpt
}

func build(e *engine.Engine, version string) *viewData {
	b := &builder{e: e, hl: map[string][]string{}, excerpts: map[string]*excerpt{}}
	r := e.Review
	errs, warns := review.Split(e.Findings)
	d := &viewData{
		Version: version, Generated: time.Now().Format(time.RFC3339),
		ReviewPath: e.ReviewPath, ReviewName: filepath.Base(e.ReviewPath), ExitCode: e.ExitCode,
		Summary: r.Summary, Notes: nonNil(r.Notes), Flows: r.Flows,
		Errors: errs, Warnings: warns, Counts: e.Summary, Unlisted: e.Unlisted, Untested: nonNil(e.Untested),
		Excerpts: b.excerpts,
	}
	if d.Flows == nil {
		d.Flows = []*review.Flow{}
	}

	for _, s := range e.Repos {
		d.Repos = append(d.Repos, b.repo(s))
	}

	for i, n := range r.Nodes {
		res := e.NodeResults[i]
		nv := nodeView{ID: n.ID, Kind: n.Kind, Label: n.Label, Change: n.Change, Why: n.Why, Ref: n.Ref, Result: res}
		for _, m := range n.Meta {
			nv.Meta = append(nv.Meta, kv{m.Key, m.String()})
		}
		if n.Ref != nil {
			if s := e.Repo(n.Ref.Repo); s != nil && s.Repo.IsContext() {
				nv.Context = true
			}
			id := "n:" + n.ID
			if b.excerpt(id, n.Ref, res, n.Change == review.ChangeDeleted) {
				nv.Excerpt = id
			}
		}
		d.Nodes = append(d.Nodes, nv)
	}

	d.Edges = []edgeView{}
	for i, ed := range r.Edges {
		res := e.EdgeResults[i]
		ev := edgeView{ID: fmt.Sprintf("e%d", i), From: ed.From, To: ed.To, Kind: ed.Kind, Ref: ed.Ref, Result: res}
		if ed.Ref != nil {
			id := "e:" + ev.ID
			if b.excerpt(id, ed.Ref, res, false) {
				ev.Excerpt = id
			}
		}
		d.Edges = append(d.Edges, ev)
	}

	d.Claims = []claimView{}
	for i, c := range r.Claims {
		cv := claimView{Text: c.Text, About: nonNil(c.About), Status: e.ClaimResults[i].Status, Evidence: []evidenceView{}}
		for j, ev := range c.Evidence {
			res := e.ClaimResults[i].Evidence[j]
			x := evidenceView{Ref: ev, Result: res}
			id := fmt.Sprintf("c:%d:%d", i, j)
			if b.excerpt(id, ev, res, false) {
				x.Excerpt = id
			}
			cv.Evidence = append(cv.Evidence, x)
		}
		d.Claims = append(d.Claims, cv)
	}

	d.Files = []fileView{}
	for _, s := range e.Repos {
		for i, fc := range e.Coverage[s.Repo.ID] {
			d.Files = append(d.Files, b.file(s, fc, fmt.Sprintf("f:%s:%d", s.Repo.ID, i)))
		}
	}
	return d
}

func nonNil[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}

func (b *builder) repo(s *engine.RepoState) repoView {
	rv := repoView{ID: s.Repo.ID, Path: s.Repo.Path, Abs: s.Abs, Role: s.Repo.Role, OK: s.OK, Worktree: s.Worktree,
		Commits: []commitView{}, Uncommitted: b.e.Uncommitted[s.Repo.ID]}
	if s.Worktree {
		rv.Commits = append(rv.Commits, commitView{SHA: review.Worktree, Short: "WORKTREE", Subject: "Uncommitted changes in the working tree", Worktree: true})
	}
	for _, sha := range s.Commits {
		rv.Commits = append(rv.Commits, commitInfo(s, sha))
	}
	if !s.OK {
		for _, c := range s.Repo.Commits {
			if c != review.Worktree {
				rv.Commits = append(rv.Commits, commitView{SHA: c, Short: shortSHA(c), Subject: "(unresolved)"})
			}
		}
	}
	if s.OK && !s.Repo.IsContext() {
		if s.Initial == gitx.EmptyTree {
			rv.Base = &commitView{SHA: s.Initial, Short: "∅", Subject: "empty tree (change starts at the root commit)"}
		} else {
			c := commitInfo(s, s.Initial)
			rv.Base = &c
		}
	}
	for _, f := range s.Files {
		rv.Files++
		rv.Added += f.Added()
		rv.Removed += f.Removed()
	}
	return rv
}

func commitInfo(s *engine.RepoState, sha string) commitView {
	if info, ok := s.CommitLog[sha]; ok {
		return commitView{SHA: sha, Short: info.Short, Subject: info.Subject, Author: info.Author, Date: info.Date}
	}
	return commitView{SHA: sha, Short: shortSHA(sha)}
}

func shortSHA(s string) string {
	if len(s) > 7 {
		return s[:7]
	}
	return s
}

// highlighted returns the highlighted lines of a file in a state.
func (b *builder) highlighted(s *engine.RepoState, state, path, language string) ([]string, *engine.Content) {
	c := s.Read(state, path)
	key := s.Repo.ID + "\x00" + s.StateSHA(state) + "\x00" + path + "\x00" + language
	if h, ok := b.hl[key]; ok {
		return h, c
	}
	h := langs.Highlight(c.Lines, language)
	b.hl[key] = h
	return h, c
}

// diffIndex precomputes, for a file diff, which final lines are added,
// which removed lines sit before each final line, and the old-number offset.
type diffIndex struct {
	added  map[int]bool
	delsAt map[int][]int // final line before which removed (old) lines are shown
	hunks  []*engine.Hunk
}

func indexDiff(fd *engine.FileDiff) *diffIndex {
	ix := &diffIndex{added: map[int]bool{}, delsAt: map[int][]int{}}
	if fd == nil {
		return ix
	}
	ix.hunks = fd.Hunks
	for _, h := range fd.Hunks {
		for l := h.NewStart; l <= h.NewEnd(); l++ {
			ix.added[l] = true
		}
		at := h.NewStart
		if h.PureDeletion() {
			at = h.NewStart + 1
		}
		for l := h.OldStart; l <= h.OldEnd(); l++ {
			ix.delsAt[at] = append(ix.delsAt[at], l)
		}
	}
	return ix
}

// oldNumber maps an unchanged final line onto its initial-state number.
func (ix *diffIndex) oldNumber(n int) int {
	off := 0
	for _, h := range ix.hunks {
		before := h.NewEnd() < n
		if h.PureDeletion() {
			before = h.NewStart < n
		}
		if before {
			off += h.NewCount - h.OldCount
		}
	}
	return n - off
}

// diffRows renders final lines [from, to] with removed lines interleaved.
func (b *builder) diffRows(newHL, oldHL []string, ix *diffIndex, from, to int, focus [2]int) []Row {
	var rows []Row
	for n := from; n <= to+1; n++ {
		// Removed lines just past the window belong to it when they close
		// the focused range or the file.
		if n <= to || n == focus[1]+1 || n == len(newHL)+1 {
			for _, o := range ix.delsAt[n] {
				rows = append(rows, Row{T: "del", O: o, H: lineAt(oldHL, o), F: n >= focus[0] && n <= focus[1]+1})
			}
		}
		if n > to {
			break
		}
		r := Row{T: "ctx", N: n, H: lineAt(newHL, n), F: n >= focus[0] && n <= focus[1]}
		if ix.added[n] {
			r.T = "add"
		} else {
			r.O = ix.oldNumber(n)
		}
		rows = append(rows, r)
	}
	return rows
}

func lineAt(hl []string, n int) string {
	if n >= 1 && n <= len(hl) {
		return hl[n-1]
	}
	return ""
}

// excerpt resolves the code a ref points at; it returns false when there is
// nothing to show.
func (b *builder) excerpt(id string, ref *review.Ref, res *engine.RefResult, deleted bool) bool {
	s := b.e.Repo(ref.Repo)
	if s == nil || !s.OK || ref.File == "" || res.State == "" {
		return false
	}
	language := s.Language(ref.File, ref.Language)
	x := &excerpt{Repo: ref.Repo, File: ref.File, Language: language, State: res.State, StateKind: res.StateKind}

	state := engine.StateFinal
	switch res.StateKind {
	case "initial":
		state = engine.StateInitial
	case "at":
		state = ref.At
	}
	hl, content := b.highlighted(s, state, ref.File, language)
	if !content.Exists {
		return false
	}
	if content.Binary {
		x.Note = "binary file"
		b.excerpts[id] = x
		return true
	}

	fd := s.Diff(ref.File)
	if fd != nil {
		x.OldPath = fd.OldPath
		if fd.OldPath == fd.Path {
			x.OldPath = ""
		}
	}
	plain := state != engine.StateFinal || s.Repo.IsContext() || fd == nil

	var oldHL []string
	if !plain && fd.OldPath != "" {
		oldHL, _ = b.highlighted(s, engine.StateInitial, fd.OldPath, language)
	}
	ix := indexDiff(fd)

	lines := len(hl)
	window := func(from, to int, focus [2]int) {
		from, to = max(1, from), min(lines, to)
		if to-from+1 > maxRows {
			to = from + maxRows - 1
			x.Truncated = true
		}
		if plain {
			for n := from; n <= to; n++ {
				r := Row{T: "ctx", N: n, H: hl[n-1], F: n >= focus[0] && n <= focus[1]}
				if state == engine.StateInitial && fd != nil {
					r.O, r.N = n, 0
					for _, h := range fd.Hunks {
						if n >= h.OldStart && n <= h.OldEnd() {
							r.T = "del"
						}
					}
				}
				x.Rows = append(x.Rows, r)
			}
			return
		}
		x.Rows = append(x.Rows, b.diffRows(hl, oldHL, ix, from, to, focus)...)
	}

	switch ref.Type {
	case review.RefRange:
		s0, e0 := ref.Lines[0], ref.Lines[1]
		window(s0-ctxLines, e0+ctxLines, [2]int{s0, e0})
	case review.RefSite:
		line := ref.Line
		if res.ActualLine > 0 {
			line = res.ActualLine
		}
		window(line-4, line+4, [2]int{line, line})
	case review.RefHunk:
		s0, e0 := ref.Lines[0], ref.Lines[1]
		window(s0-ctxLines, e0+ctxLines, [2]int{s0, e0})
	case review.RefFile:
		switch {
		case fd != nil && !plain && fd.Status != engine.DiffAdded && len(fd.Hunks) > 0:
			// Changed file: every hunk with context, gaps between them.
			prevEnd := 0
			for _, h := range fd.Hunks {
				from := max(1, h.NewStart-ctxLines)
				to := h.NewEnd() + ctxLines
				if h.PureDeletion() {
					from, to = max(1, h.NewStart-ctxLines+1), h.NewStart+ctxLines
				}
				if from <= prevEnd {
					from = prevEnd + 1
				} else if prevEnd > 0 {
					x.Rows = append(x.Rows, Row{T: "gap"})
				}
				if to < from {
					continue
				}
				window(from, to, [2]int{0, -1})
				prevEnd = min(to, lines)
				if len(x.Rows) > maxRows {
					x.Truncated = true
					break
				}
			}
			for n := 1; n <= lines; n++ {
				if n > maxRows {
					x.AfterTruncated = true
					break
				}
				r := Row{T: "ctx", N: n, H: hl[n-1]}
				if ix.added[n] {
					r.T = "add"
				}
				x.After = append(x.After, r)
			}
		default:
			window(1, lines, [2]int{0, -1})
		}
	}
	b.excerpts[id] = x
	return true
}

func (b *builder) file(s *engine.RepoState, fc *engine.FileCoverage, id string) fileView {
	fd := fc.Diff
	language := s.Language(fd.Key(), "")
	fv := fileView{ID: id, Repo: s.Repo.ID, Path: fd.Key(), Status: fd.Status, Binary: fd.Binary, Untracked: fd.Untracked,
		Language: language, Added: fd.Added(), Removed: fd.Removed(), Coverage: fc.Coverage,
		FileNodes: fc.FileNodes, HasTests: fc.HasTests, Hunks: []hunkView{}}
	if fd.OldPath != "" && fd.OldPath != fd.Path {
		fv.OldPath = fd.OldPath
	}

	var newHL, oldHL []string
	if !fd.Binary {
		if fd.Status != engine.DiffDeleted {
			newHL, _ = b.highlighted(s, engine.StateFinal, fd.Path, language)
		}
		if fd.OldPath != "" {
			oldHL, _ = b.highlighted(s, engine.StateInitial, fd.OldPath, language)
		}
	}
	fv.Lines = len(newHL)
	if fd.Status == engine.DiffDeleted {
		fv.Lines = len(oldHL)
	}
	ix := indexDiff(fd)

	for i, hc := range fc.Hunks {
		h := hc.Hunk
		hv := hunkView{ID: fmt.Sprintf("%s:%d", id, i), OldStart: h.OldStart, OldCount: h.OldCount, NewStart: h.NewStart,
			NewCount: h.NewCount, Coverage: hc.Coverage, Nodes: nonNil(hc.Nodes), TestNodes: hc.TestNodes,
			Uncovered: hc.Uncovered, OldSide: hc.OldSide, Commits: s.HunkCommits(fd.Path, h)}
		if fd.Status == engine.DiffDeleted {
			for l := h.OldStart; l <= h.OldEnd() && len(hv.Rows) < maxRows; l++ {
				hv.Rows = append(hv.Rows, Row{T: "del", O: l, H: lineAt(oldHL, l)})
			}
		} else {
			from, to := h.NewStart-ctxLines, h.NewEnd()+ctxLines
			if h.PureDeletion() {
				from, to = h.NewStart-ctxLines+1, h.NewStart+ctxLines
			}
			from, to = max(1, from), min(len(newHL), to)
			if to-from > maxRows {
				to = from + maxRows
			}
			// Only this hunk's removed lines: limit dels to this hunk.
			hix := &diffIndex{added: ix.added, delsAt: map[int][]int{}, hunks: ix.hunks}
			at := h.NewStart
			if h.PureDeletion() {
				at = h.NewStart + 1
			}
			for l := h.OldStart; l <= h.OldEnd(); l++ {
				hix.delsAt[at] = append(hix.delsAt[at], l)
			}
			hv.Rows = b.diffRows(newHL, oldHL, hix, from, to, [2]int{h.NewStart, h.NewEnd()})
		}
		fv.Hunks = append(fv.Hunks, hv)
	}
	sort.SliceStable(fv.Hunks, func(i, j int) bool { return fv.Hunks[i].NewStart < fv.Hunks[j].NewStart })
	return fv
}
