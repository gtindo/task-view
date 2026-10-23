package engine

import (
	"regexp"
	"strconv"
	"strings"
)

// File diff statuses.
const (
	DiffAdded    = "added"
	DiffModified = "modified"
	DiffDeleted  = "deleted"
	DiffRenamed  = "renamed"
)

// Hunk is one zero-context hunk. Old lines use initial-state numbers, new
// lines final-state numbers.
type Hunk struct {
	OldStart, OldCount int
	NewStart, NewCount int
	Old, New           []string
}

// NewEnd is the last final-state line the hunk adds, or NewStart-1.
func (h *Hunk) NewEnd() int { return h.NewStart + h.NewCount - 1 }

// OldEnd is the last initial-state line the hunk removes, or OldStart-1.
func (h *Hunk) OldEnd() int { return h.OldStart + h.OldCount - 1 }

// PureDeletion reports whether the hunk only removes lines.
func (h *Hunk) PureDeletion() bool { return h.NewCount == 0 }

// FileDiff is the diff of one file between the initial and final states.
type FileDiff struct {
	Path      string // final path; empty for a deleted file
	OldPath   string // initial path; empty for an added file
	Status    string
	Binary    bool
	Untracked bool
	Hunks     []*Hunk
}

// Key is the path the file is referenced by: final path, or initial path
// for a deleted file.
func (f *FileDiff) Key() string {
	if f.Path != "" {
		return f.Path
	}
	return f.OldPath
}

// Added and Removed count changed lines.
func (f *FileDiff) Added() int {
	n := 0
	for _, h := range f.Hunks {
		n += h.NewCount
	}
	return n
}

func (f *FileDiff) Removed() int {
	n := 0
	for _, h := range f.Hunks {
		n += h.OldCount
	}
	return n
}

// ChangeState is the node change state matching this file diff.
func (f *FileDiff) ChangeState() string {
	switch f.Status {
	case DiffAdded:
		return "added"
	case DiffDeleted:
		return "deleted"
	}
	return "modified"
}

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// ParseDiff parses `git diff --unified=0 -M` output.
func ParseDiff(out string) []*FileDiff {
	var files []*FileDiff
	var cur *FileDiff
	var hunk *Hunk
	inHeader := false

	for _, line := range strings.Split(out, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			a, b := splitGitHeader(strings.TrimPrefix(line, "diff --git "))
			cur = &FileDiff{Path: b, OldPath: a, Status: DiffModified}
			files = append(files, cur)
			hunk = nil
			inHeader = true
		case cur == nil:
			continue
		case inHeader && strings.HasPrefix(line, "new file mode"):
			cur.Status = DiffAdded
			cur.OldPath = ""
		case inHeader && strings.HasPrefix(line, "deleted file mode"):
			cur.Status = DiffDeleted
			cur.Path = ""
		case inHeader && strings.HasPrefix(line, "rename from "):
			cur.OldPath = unquote(strings.TrimPrefix(line, "rename from "))
			cur.Status = DiffRenamed
		case inHeader && strings.HasPrefix(line, "rename to "):
			cur.Path = unquote(strings.TrimPrefix(line, "rename to "))
			cur.Status = DiffRenamed
		case inHeader && strings.HasPrefix(line, "Binary files "):
			cur.Binary = true
		case inHeader && strings.HasPrefix(line, "--- "):
			if p := diffPath(strings.TrimPrefix(line, "--- ")); p != "" {
				cur.OldPath = p
			}
		case inHeader && strings.HasPrefix(line, "+++ "):
			if p := diffPath(strings.TrimPrefix(line, "+++ ")); p != "" {
				cur.Path = p
			}
		case strings.HasPrefix(line, "@@ "):
			inHeader = false
			m := hunkRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			hunk = &Hunk{
				OldStart: atoi(m[1]), OldCount: atoiDefault(m[2], 1),
				NewStart: atoi(m[3]), NewCount: atoiDefault(m[4], 1),
			}
			cur.Hunks = append(cur.Hunks, hunk)
		case hunk != nil && strings.HasPrefix(line, "-"):
			hunk.Old = append(hunk.Old, trimCR(line[1:]))
		case hunk != nil && strings.HasPrefix(line, "+"):
			hunk.New = append(hunk.New, trimCR(line[1:]))
		}
	}

	// A pure mode change has no hunks and is not part of the change.
	var kept []*FileDiff
	for _, f := range files {
		if f.Status == DiffModified && len(f.Hunks) == 0 && !f.Binary {
			continue
		}
		if f.Status == DiffRenamed && len(f.Hunks) == 0 && !f.Binary && f.Path == f.OldPath {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}

func diffPath(s string) string {
	s = strings.TrimRight(s, "\t")
	if s == "/dev/null" {
		return ""
	}
	s = unquote(s)
	if strings.HasPrefix(s, "a/") || strings.HasPrefix(s, "b/") {
		return s[2:]
	}
	return s
}

// splitGitHeader splits the "a/x b/y" part of a `diff --git` line.
func splitGitHeader(rest string) (string, string) {
	if strings.HasPrefix(rest, `"`) {
		a, tail := readQuoted(rest)
		b := strings.TrimSpace(tail)
		return strings.TrimPrefix(unquote(a), "a/"), strings.TrimPrefix(unquote(b), "b/")
	}
	if (len(rest)-5)%2 == 0 && len(rest) > 5 {
		l := (len(rest) - 5) / 2
		a, b := rest[2:2+l], rest[2+l+3:]
		if a == b {
			return a, b
		}
	}
	if i := strings.Index(rest, " b/"); i >= 0 {
		return strings.TrimPrefix(rest[:i], "a/"), unquote(rest[i+3:])
	}
	return rest, rest
}

func readQuoted(s string) (string, string) {
	for i := 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			return s[:i+1], s[i+1:]
		}
	}
	return s, ""
}

func unquote(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
	}
	return s
}

func trimCR(s string) string { return strings.TrimSuffix(s, "\r") }

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	return atoi(s)
}

// SplitLines splits file content into lines; CRLF and LF both end a line,
// and a final line break does not start a new line.
func SplitLines(data []byte) []string {
	if len(data) == 0 {
		return nil
	}
	s := string(data)
	s = strings.TrimSuffix(s, "\n")
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimSuffix(l, "\r")
	}
	return lines
}

// IsBinary applies Git's heuristic: a NUL byte in the first 8000 bytes.
func IsBinary(data []byte) bool {
	n := min(len(data), 8000)
	for i := 0; i < n; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}
