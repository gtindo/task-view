// Package langs maps files to highlighter languages and renders highlighted
// lines. The bundled Chroma lexer list is the single source of valid names.
package langs

import (
	"html"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// Language is one valid language name with its usual file patterns.
type Language struct {
	Name       string
	Aliases    []string
	Extensions []string
}

var (
	once    sync.Once
	byName  map[string]chroma.Lexer
	ordered []Language
)

func load() {
	once.Do(func() {
		byName = map[string]chroma.Lexer{}
		for _, lx := range lexers.GlobalLexerRegistry.Lexers {
			cfg := lx.Config()
			name := strings.ToLower(cfg.Name)
			lang := Language{Name: name}
			if _, dup := byName[name]; !dup {
				byName[name] = lx
			}
			for _, a := range cfg.Aliases {
				a = strings.ToLower(a)
				if a == name {
					continue
				}
				if _, dup := byName[a]; !dup {
					byName[a] = lx
				}
				lang.Aliases = append(lang.Aliases, a)
			}
			lang.Extensions = append(lang.Extensions, cfg.Filenames...)
			ordered = append(ordered, lang)
		}
		sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })
	})
}

// All returns every bundled language, sorted by name.
func All() []Language {
	load()
	return ordered
}

// Known reports whether name is a valid language name or alias.
func Known(name string) bool {
	load()
	_, ok := byName[strings.ToLower(name)]
	return ok
}

// nonSource lists the languages of documentation, data and configuration
// files: text with no behaviour of its own for a test to exercise.
var nonSource = map[string]bool{
	"markdown": true, "restructuredtext": true, "plaintext": true, "org mode": true,
	"tex": true, "bibtex": true, "groff": true, "csv": true, "json": true, "yaml": true,
	"toml": true, "ini": true, "properties": true, "xml": true, "diff": true, "reg": true,
}

// NonSource reports whether name (or an alias) is a documentation, data or
// configuration language rather than source code.
func NonSource(name string) bool {
	load()
	lx, ok := byName[strings.ToLower(name)]
	return ok && nonSource[strings.ToLower(lx.Config().Name)]
}

// Closest returns up to n valid names nearest to name.
func Closest(name string, n int) []string {
	load()
	name = strings.ToLower(name)
	type cand struct {
		name string
		dist int
	}
	var cands []cand
	for k := range byName {
		d := levenshtein(name, k)
		if strings.HasPrefix(k, name) || strings.HasPrefix(name, k) {
			d -= 2
		}
		cands = append(cands, cand{k, d})
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].dist != cands[j].dist {
			return cands[i].dist < cands[j].dist
		}
		return cands[i].name < cands[j].name
	})
	var out []string
	for i := 0; i < len(cands) && i < n; i++ {
		out = append(out, cands[i].name)
	}
	return out
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

var (
	globMu    sync.Mutex
	globCache = map[string]*regexp.Regexp{}
)

// MatchGlob reports whether a repo-relative POSIX path matches a glob.
// `**` crosses directories, `*` and `?` do not, `{a,b}` is an alternation.
func MatchGlob(pattern, path string) bool {
	globMu.Lock()
	re, ok := globCache[pattern]
	if !ok {
		re = regexp.MustCompile("^" + globToRegexp(pattern) + "$")
		globCache[pattern] = re
	}
	globMu.Unlock()
	return re.MatchString(path)
}

func globToRegexp(p string) string {
	var b strings.Builder
	inAlt := false
	for i := 0; i < len(p); i++ {
		c := p[i]
		switch {
		case c == '*' && i+1 < len(p) && p[i+1] == '*':
			i++
			if i+1 < len(p) && p[i+1] == '/' {
				i++
				b.WriteString("(?:.*/)?")
			} else {
				b.WriteString(".*")
			}
		case c == '*':
			b.WriteString("[^/]*")
		case c == '?':
			b.WriteString("[^/]")
		case c == '{':
			inAlt = true
			b.WriteString("(?:")
		case c == '}' && inAlt:
			inAlt = false
			b.WriteString(")")
		case c == ',' && inAlt:
			b.WriteString("|")
		default:
			b.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return b.String()
}

// Highlight tokenizes content with the named language and returns one HTML
// fragment per line. An empty or unknown language gives escaped plain text.
func Highlight(content []string, language string) []string {
	out := make([]string, len(content))
	load()
	lx, ok := byName[strings.ToLower(language)]
	if language == "" || !ok {
		for i, l := range content {
			out[i] = html.EscapeString(l)
		}
		return out
	}

	lx = chroma.Coalesce(lx)
	it, err := lx.Tokenise(nil, strings.Join(content, "\n")+"\n")
	if err != nil {
		for i, l := range content {
			out[i] = html.EscapeString(l)
		}
		return out
	}

	var b strings.Builder
	line := 0
	flush := func() {
		if line < len(out) {
			out[line] = b.String()
		}
		b.Reset()
		line++
	}
	for tok := it(); tok != chroma.EOF; tok = it() {
		cls := class(tok.Type)
		parts := strings.Split(tok.Value, "\n")
		for i, part := range parts {
			if i > 0 {
				flush()
			}
			if part == "" {
				continue
			}
			esc := html.EscapeString(part)
			if cls == "" {
				b.WriteString(esc)
			} else {
				b.WriteString(`<span class="`)
				b.WriteString(cls)
				b.WriteString(`">`)
				b.WriteString(esc)
				b.WriteString(`</span>`)
			}
		}
	}
	if b.Len() > 0 {
		flush()
	}
	return out
}

// class maps a token type onto the small class set styled by the viewer.
func class(t chroma.TokenType) string {
	switch {
	case t.InCategory(chroma.Comment):
		return "tc"
	case t == chroma.KeywordType:
		return "tt"
	case t.InCategory(chroma.Keyword):
		return "tk"
	case t.InSubCategory(chroma.LiteralString):
		return "ts"
	case t.InSubCategory(chroma.LiteralNumber):
		return "tn"
	case t.InCategory(chroma.Literal):
		return "tn"
	case t == chroma.NameFunction || t == chroma.NameFunctionMagic:
		return "tf"
	case t == chroma.NameClass || t == chroma.NameNamespace:
		return "tt"
	case t.InSubCategory(chroma.NameBuiltin):
		return "tb"
	case t == chroma.NameTag:
		return "tg"
	case t == chroma.NameAttribute || t == chroma.NameDecorator:
		return "ta"
	case t == chroma.NameConstant || t == chroma.NameProperty:
		return "tp"
	case t.InCategory(chroma.Operator):
		return "to"
	case t == chroma.Punctuation:
		return "tu"
	case t == chroma.GenericInserted:
		return "ts"
	case t == chroma.GenericDeleted:
		return "tk"
	case t == chroma.GenericHeading || t == chroma.GenericSubheading || t == chroma.GenericStrong:
		return "tf"
	}
	return ""
}
