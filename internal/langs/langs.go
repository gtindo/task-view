// Package langs maps files to highlighter languages and renders highlighted
// lines. The bundled Chroma lexer list is the single source of valid names.
package langs

import (
	"cmp"
	"html"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// prefixMatchBonus lowers the edit distance of a name that is a prefix of
// the query or the other way round, so a truncated or extended name ranks
// above unrelated names of similar length.
const prefixMatchBonus = 2

// Language is one valid language name with its usual file patterns.
type Language struct {
	Name       string
	Aliases    []string
	Extensions []string
}

var (
	loadOnce        sync.Once
	lexerByName     map[string]chroma.Lexer
	sortedLanguages []Language
)

func load() {
	loadOnce.Do(func() {
		lexerByName = map[string]chroma.Lexer{}

		for _, lexer := range lexers.GlobalLexerRegistry.Lexers {
			config := lexer.Config()
			name := strings.ToLower(config.Name)
			language := Language{Name: name}

			// Names and aliases can collide across lexers; the first
			// registered lexer keeps the name.
			if _, taken := lexerByName[name]; !taken {
				lexerByName[name] = lexer
			}

			for _, alias := range config.Aliases {
				alias = strings.ToLower(alias)
				if alias == name {
					continue
				}

				if _, taken := lexerByName[alias]; !taken {
					lexerByName[alias] = lexer
				}

				language.Aliases = append(language.Aliases, alias)
			}

			language.Extensions = append(language.Extensions, config.Filenames...)
			sortedLanguages = append(sortedLanguages, language)
		}

		slices.SortFunc(sortedLanguages, func(left, right Language) int {
			return strings.Compare(left.Name, right.Name)
		})
	})
}

// All returns every bundled language, sorted by name.
func All() []Language {
	load()

	return sortedLanguages
}

// Known reports whether name is a valid language name or alias.
func Known(name string) bool {
	load()
	_, ok := lexerByName[strings.ToLower(name)]

	return ok
}

// Closest returns up to limit valid names nearest to name, for suggestions
// after an unknown language. Ties in distance are broken by name.
func Closest(name string, limit int) []string {
	load()
	name = strings.ToLower(name)

	type candidate struct {
		name     string
		distance int
	}

	var candidates []candidate

	for validName := range lexerByName {
		distance := levenshtein(name, validName)

		isPrefixMatch := strings.HasPrefix(validName, name) || strings.HasPrefix(name, validName)
		if isPrefixMatch {
			distance -= prefixMatchBonus
		}

		candidates = append(candidates, candidate{validName, distance})
	}

	// Names are unique map keys, so the order is total and stability does
	// not matter.
	slices.SortFunc(candidates, func(left, right candidate) int {
		return cmp.Or(cmp.Compare(left.distance, right.distance), strings.Compare(left.name, right.name))
	})

	var closest []string

	for i, candidate := range candidates {
		if i >= limit {
			break
		}

		closest = append(closest, candidate.name)
	}

	return closest
}

// levenshtein returns the edit distance between source and target in bytes,
// keeping only two rows of the dynamic-programming table.
func levenshtein(source, target string) int {
	previousRow := make([]int, len(target)+1)
	currentRow := make([]int, len(target)+1)

	for j := range previousRow {
		previousRow[j] = j
	}

	for i := 1; i <= len(source); i++ {
		currentRow[0] = i

		for j := 1; j <= len(target); j++ {
			substitutionCost := 1
			if source[i-1] == target[j-1] {
				substitutionCost = 0
			}

			currentRow[j] = min(previousRow[j]+1, currentRow[j-1]+1, previousRow[j-1]+substitutionCost)
		}

		previousRow, currentRow = currentRow, previousRow
	}

	return previousRow[len(target)]
}

var (
	globCacheMu sync.Mutex
	globCache   = map[string]*regexp.Regexp{}
)

// MatchGlob reports whether a repo-relative POSIX path matches a glob.
// `**` crosses directories, `*` and `?` do not, `{a,b}` is an alternation.
func MatchGlob(pattern, path string) bool {
	globCacheMu.Lock()

	globRegexp, ok := globCache[pattern]
	if !ok {
		globRegexp = regexp.MustCompile("^" + globToRegexp(pattern) + "$")
		globCache[pattern] = globRegexp
	}

	globCacheMu.Unlock()

	return globRegexp.MatchString(path)
}

func globToRegexp(pattern string) string {
	var regex strings.Builder

	inAlternation := false

	for i := 0; i < len(pattern); i++ {
		char := pattern[i]

		switch {
		case strings.HasPrefix(pattern[i:], "**/"):
			// `**/` also matches no directory at all, so `**/*.go` matches
			// `main.go` at the repo root.
			i += len("**/") - 1
			regex.WriteString("(?:.*/)?")
		case strings.HasPrefix(pattern[i:], "**"):
			i += len("**") - 1
			regex.WriteString(".*")
		case char == '*':
			regex.WriteString("[^/]*")
		case char == '?':
			regex.WriteString("[^/]")
		case char == '{':
			inAlternation = true
			regex.WriteString("(?:")
		case char == '}' && inAlternation:
			inAlternation = false
			regex.WriteString(")")
		case char == ',' && inAlternation:
			regex.WriteString("|")
		default:
			regex.WriteString(regexp.QuoteMeta(string(char)))
		}
	}

	return regex.String()
}

// Highlight tokenizes content with the named language and returns one HTML
// fragment per line. An empty or unknown language gives escaped plain text.
func Highlight(content []string, language string) []string {
	load()

	lexer, ok := lexerByName[strings.ToLower(language)]
	if language == "" || !ok {
		return escapeLines(content)
	}

	nextToken, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(content, "\n")+"\n")
	if err != nil {
		return escapeLines(content)
	}

	highlighted := make([]string, len(content))

	var lineHTML strings.Builder

	lineIndex := 0
	endLine := func() {
		// A lexer may emit more newlines than the content has; the extra
		// lines are dropped.
		if lineIndex < len(highlighted) {
			highlighted[lineIndex] = lineHTML.String()
		}

		lineHTML.Reset()
		lineIndex++
	}

	// A token can span lines, so it is split at newlines and each piece is
	// wrapped on its own line.
	for token := nextToken(); token != chroma.EOF; token = nextToken() {
		cssClass := tokenClass(token.Type)

		for i, piece := range strings.Split(token.Value, "\n") {
			if i > 0 {
				endLine()
			}

			if piece == "" {
				continue
			}

			escaped := html.EscapeString(piece)
			if cssClass == "" {
				lineHTML.WriteString(escaped)

				continue
			}

			lineHTML.WriteString(`<span class="` + cssClass + `">` + escaped + `</span>`)
		}
	}

	if lineHTML.Len() > 0 {
		endLine()
	}

	return highlighted
}

// escapeLines renders lines as plain text, the fallback when no lexer
// applies.
func escapeLines(lines []string) []string {
	escaped := make([]string, len(lines))
	for i, line := range lines {
		escaped[i] = html.EscapeString(line)
	}

	return escaped
}

// tokenClass maps a token type onto the small class set styled by the viewer.
func tokenClass(tokenType chroma.TokenType) string {
	switch {
	case tokenType.InCategory(chroma.Comment):
		return "tc"
	case tokenType == chroma.KeywordType:
		return "tt"
	case tokenType.InCategory(chroma.Keyword):
		return "tk"
	case tokenType.InSubCategory(chroma.LiteralString):
		return "ts"
	case tokenType.InSubCategory(chroma.LiteralNumber):
		return "tn"
	case tokenType.InCategory(chroma.Literal):
		return "tn"
	case tokenType == chroma.NameFunction || tokenType == chroma.NameFunctionMagic:
		return "tf"
	case tokenType == chroma.NameClass || tokenType == chroma.NameNamespace:
		return "tt"
	case tokenType.InSubCategory(chroma.NameBuiltin):
		return "tb"
	case tokenType == chroma.NameTag:
		return "tg"
	case tokenType == chroma.NameAttribute || tokenType == chroma.NameDecorator:
		return "ta"
	case tokenType == chroma.NameConstant || tokenType == chroma.NameProperty:
		return "tp"
	case tokenType.InCategory(chroma.Operator):
		return "to"
	case tokenType == chroma.Punctuation:
		return "tu"
	case tokenType == chroma.GenericInserted:
		return "ts"
	case tokenType == chroma.GenericDeleted:
		return "tk"
	case tokenType == chroma.GenericHeading || tokenType == chroma.GenericSubheading || tokenType == chroma.GenericStrong:
		return "tf"
	}

	return ""
}
