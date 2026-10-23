package review

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/gtindo/changes-view/internal/langs"
)

//go:embed schema.json
var SchemaJSON []byte

const schemaURL = "https://github.com/gtindo/changes-view/schema/v1/change.json"

var (
	schemaOnce sync.Once
	schema     *jsonschema.Schema
	schemaErr  error
)

func compiled() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
		if err != nil {
			schemaErr = err
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource(schemaURL, doc); err != nil {
			schemaErr = err
			return
		}
		schema, schemaErr = c.Compile(schemaURL)
	})
	return schema, schemaErr
}

// Load reads and checks a review file without touching Git. When the
// returned findings hold an exit-2 error, the review is nil.
func Load(path string) (*Review, []Finding) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, []Finding{NewFinding("file_unreadable", "", fmt.Sprintf("cannot read review file: %v", err),
			"check the path; it may be absolute or relative to the current directory")}
	}
	return Parse(data)
}

// Parse checks review file bytes: JSON, version, schema, cross-references.
func Parse(data []byte) (*Review, []Finding) {
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, []Finding{NewFinding("file_unreadable", "", fmt.Sprintf("review file is not valid JSON: %v", err),
			"write the file as a single JSON object")}
	}

	if f, ok := checkVersion(inst); !ok {
		return nil, []Finding{f}
	}

	sch, err := compiled()
	if err != nil {
		panic("embedded schema does not compile: " + err.Error())
	}
	if err := sch.Validate(inst); err != nil {
		var verr *jsonschema.ValidationError
		if errors.As(err, &verr) {
			return nil, schemaFindings(verr)
		}
		return nil, []Finding{NewFinding("schema_invalid", "", err.Error(), "run `change-view schema` for the exact JSON Schema")}
	}

	var r Review
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return nil, []Finding{NewFinding("schema_invalid", "", err.Error(), "run `change-view schema` for the exact JSON Schema")}
	}
	for _, repo := range r.Repos {
		if repo.Role == "" {
			repo.Role = RoleChanged
		}
	}

	findings := crossCheck(&r)
	if ExitCode(findings) == ExitSchema {
		return nil, findings
	}
	return &r, findings
}

func checkVersion(inst any) (Finding, bool) {
	obj, ok := inst.(map[string]any)
	if !ok {
		return Finding{}, true
	}
	raw, present := obj["version"]
	if !present {
		return Finding{}, true
	}
	num, isNum := raw.(json.Number)
	if !isNum {
		return Finding{}, true
	}
	for _, v := range SupportedVersions {
		if num.String() == fmt.Sprint(v) {
			return Finding{}, true
		}
	}
	return NewFinding("unsupported_version", "/version",
		fmt.Sprintf("version %s is not supported by this binary", num),
		fmt.Sprintf("use \"version\": %d; run `change-view version` for supported versions", SchemaVersion)), false
}

// schemaFindings flattens a validation error into one finding per leaf
// failure, each with the JSON Pointer of the failing value.
func schemaFindings(verr *jsonschema.ValidationError) []Finding {
	p := message.NewPrinter(language.English)
	seen := map[string]bool{}
	var findings []Finding
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) > 0 {
			for _, c := range e.Causes {
				walk(c)
			}
			return
		}
		ptr := ""
		for _, tok := range e.InstanceLocation {
			ptr += "/" + escapePointer(tok)
		}
		msg := e.ErrorKind.LocalizedString(p)
		hint := "fix the value at this path; run `change-view schema` for the exact JSON Schema"
		kw := strings.Join(e.ErrorKind.KeywordPath(), "/")
		if strings.Contains(e.SchemaURL, "/not") || kw == "not" {
			msg = "field not allowed for this ref type"
			hint = "file: repo+file; range/hunk: repo+file+lines; site: repo+file+line+contains; commit: repo+sha"
		}
		key := ptr + "|" + msg
		if seen[key] {
			return
		}
		seen[key] = true
		findings = append(findings, NewFinding("schema_invalid", ptr, msg, hint))
	}
	walk(verr)
	if len(findings) == 0 {
		findings = append(findings, NewFinding("schema_invalid", "", verr.Error(), "run `change-view schema` for the exact JSON Schema"))
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].Path < findings[j].Path })
	return findings
}

var badPathRe = regexp.MustCompile(`(^|/)\.\.(/|$)`)

// RefLoc is a ref together with its JSON Pointer in the review file.
type RefLoc struct {
	Ref  *Ref
	Path string
}

// AllRefs lists every ref in the file with its JSON Pointer.
func (r *Review) AllRefs() []RefLoc {
	var out []RefLoc
	for i, n := range r.Nodes {
		if n.Ref != nil {
			out = append(out, RefLoc{n.Ref, fmt.Sprintf("/nodes/%d/ref", i)})
		}
	}
	for i, e := range r.Edges {
		if e.Ref != nil {
			out = append(out, RefLoc{e.Ref, fmt.Sprintf("/edges/%d/ref", i)})
		}
	}
	for i, c := range r.Claims {
		for j, ev := range c.Evidence {
			out = append(out, RefLoc{ev, fmt.Sprintf("/claims/%d/evidence/%d", i, j)})
		}
	}
	return out
}

func crossCheck(r *Review) []Finding {
	var fs []Finding

	repos := map[string]*Repo{}
	for i, repo := range r.Repos {
		if _, dup := repos[repo.ID]; dup {
			fs = append(fs, NewFinding("duplicate_id", fmt.Sprintf("/repos/%d/id", i),
				fmt.Sprintf("repo id %q is used more than once", repo.ID), "give every repo a unique id"))
		}
		repos[repo.ID] = repo
		for _, kv := range repo.Languages {
			name := kv.String()
			if !langs.Known(name) {
				fs = append(fs, unknownLanguage(fmt.Sprintf("/repos/%d/languages/%s", i, escapePointer(kv.Key)), name))
			}
		}
	}

	nodes := map[string]*Node{}
	for i, n := range r.Nodes {
		if _, dup := nodes[n.ID]; dup {
			fs = append(fs, NewFinding("duplicate_id", fmt.Sprintf("/nodes/%d/id", i),
				fmt.Sprintf("node id %q is used more than once", n.ID), "give every node a unique id"))
		}
		nodes[n.ID] = n
		if n.IsChanged() && strings.TrimSpace(n.Why) == "" {
			fs = append(fs, NewFinding("missing_why", fmt.Sprintf("/nodes/%d", i),
				fmt.Sprintf("node %q is %s but has no why", n.ID, n.Change),
				"add a one-sentence `why` giving the reason for the change"))
		}
	}

	flows := map[string]bool{}
	for i, f := range r.Flows {
		if flows[f.ID] {
			fs = append(fs, NewFinding("duplicate_id", fmt.Sprintf("/flows/%d/id", i),
				fmt.Sprintf("flow id %q is used more than once", f.ID), "give every flow a unique id"))
		}
		flows[f.ID] = true
	}

	missingNode := func(path, id string) {
		fs = append(fs, NewFinding("unknown_node", path, fmt.Sprintf("no node with id %q", id),
			"use an id from `nodes`, or add the missing node"))
	}
	for i, e := range r.Edges {
		if nodes[e.From] == nil {
			missingNode(fmt.Sprintf("/edges/%d/from", i), e.From)
		}
		if nodes[e.To] == nil {
			missingNode(fmt.Sprintf("/edges/%d/to", i), e.To)
		}
	}
	for i, f := range r.Flows {
		for j, id := range f.Path {
			if nodes[id] == nil {
				missingNode(fmt.Sprintf("/flows/%d/path/%d", i, j), id)
			}
		}
		for j, id := range f.Tests {
			n := nodes[id]
			if n == nil {
				missingNode(fmt.Sprintf("/flows/%d/tests/%d", i, j), id)
			} else if !n.IsTest() {
				fs = append(fs, NewFinding("not_a_test", fmt.Sprintf("/flows/%d/tests/%d", i, j),
					fmt.Sprintf("node %q has kind %q, not test", id, n.Kind),
					"list only nodes of kind `test` in flow tests; put other nodes in `path`"))
			}
		}
	}
	for i, c := range r.Claims {
		for j, id := range c.About {
			if nodes[id] == nil {
				missingNode(fmt.Sprintf("/claims/%d/about/%d", i, j), id)
			}
		}
	}

	for _, site := range r.AllRefs() {
		ref := site.Ref
		if repos[ref.Repo] == nil {
			fs = append(fs, NewFinding("unknown_repo", site.Path+"/repo",
				fmt.Sprintf("no repo with id %q", ref.Repo), "use an id from `repos`, or list the repo"))
		}
		if ref.File != "" && !cleanPath(ref.File) {
			fs = append(fs, NewFinding("bad_path", site.Path+"/file",
				fmt.Sprintf("%q is not a clean repo-relative POSIX path", ref.File),
				"use a path relative to the repo root with forward slashes, no leading `/` and no `..`"))
		}
		if len(ref.Lines) == 2 && ref.Lines[0] > ref.Lines[1] {
			fs = append(fs, NewFinding("schema_invalid", site.Path+"/lines",
				fmt.Sprintf("lines [%d, %d] start after they end", ref.Lines[0], ref.Lines[1]),
				"write lines as [start, end] with start ≤ end"))
		}
		if ref.Language != "" && !langs.Known(ref.Language) {
			fs = append(fs, unknownLanguage(site.Path+"/language", ref.Language))
		}
	}
	return fs
}

func cleanPath(p string) bool {
	return !strings.HasPrefix(p, "/") && !strings.Contains(p, `\`) && !badPathRe.MatchString(p) &&
		!strings.HasPrefix(p, "./") && !strings.Contains(p, "//")
}

func unknownLanguage(path, name string) Finding {
	return NewFinding("unknown_language", path, fmt.Sprintf("unknown language %q", name),
		fmt.Sprintf("closest valid names: %s; run `change-view languages` for the full list",
			strings.Join(langs.Closest(name, 3), ", ")))
}

func escapePointer(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, "~", "~0"), "/", "~1")
}
