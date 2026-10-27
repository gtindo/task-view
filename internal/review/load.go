package review

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"

	"github.com/gtindo/task-view/internal/langs"
)

// SchemaJSON is the JSON Schema of the review file. Parse validates against
// it and `task-view schema` prints it so agents can check their output.
//
//go:embed schema.json
var SchemaJSON []byte

const schemaURL = "https://github.com/gtindo/task-view/schema/v1/change.json"

// schemaHint points an agent at the full schema when a finding cannot name
// the exact rule it broke.
const schemaHint = "run `task-view schema` for the exact JSON Schema"

// languageSuggestions is how many close names an unknown language finding
// suggests.
const languageSuggestions = 3

// parentDirSegment matches a `..` path segment, which would leave the repo.
var parentDirSegment = regexp.MustCompile(`(^|/)\.\.(/|$)`)

var (
	schemaOnce        sync.Once
	compiledSchema    *jsonschema.Schema
	compiledSchemaErr error
)

// compiled returns the embedded schema, compiled once on first use.
func compiled() (*jsonschema.Schema, error) {
	schemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(SchemaJSON))
		if err != nil {
			compiledSchemaErr = err

			return
		}

		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(schemaURL, doc); err != nil {
			compiledSchemaErr = err

			return
		}

		compiledSchema, compiledSchemaErr = compiler.Compile(schemaURL)
	})

	return compiledSchema, compiledSchemaErr
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
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, []Finding{NewFinding("file_unreadable", "", fmt.Sprintf("review file is not valid JSON: %v", err),
			"write the file as a single JSON object")}
	}

	if finding, ok := checkVersion(instance); !ok {
		return nil, []Finding{finding}
	}

	schema, err := compiled()
	if err != nil {
		panic("embedded schema does not compile: " + err.Error())
	}

	if err := schema.Validate(instance); err != nil {
		var validationErr *jsonschema.ValidationError
		if errors.As(err, &validationErr) {
			return nil, schemaFindings(validationErr)
		}

		return nil, []Finding{NewFinding("schema_invalid", "", err.Error(), schemaHint)}
	}

	var parsed Review

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&parsed); err != nil {
		return nil, []Finding{NewFinding("schema_invalid", "", err.Error(), schemaHint)}
	}

	// The role is optional in the file: a repo the agent did not mark as
	// context is one it changed.
	for _, repo := range parsed.Repos {
		if repo.Role == "" {
			repo.Role = RoleChanged
		}
	}

	findings := parsed.crossCheck()
	if ExitCode(findings) == ExitSchema {
		return nil, findings
	}

	return &parsed, findings
}

// checkVersion runs before schema validation so an unsupported version gets
// one clear finding instead of a list of schema violations. It returns
// ok=false with that finding; anything that is not a numeric version is left
// to the schema.
func checkVersion(instance any) (Finding, bool) {
	object, ok := instance.(map[string]any)
	if !ok {
		return Finding{}, true
	}

	raw, present := object["version"]
	if !present {
		return Finding{}, true
	}

	version, isNumber := raw.(json.Number)
	if !isNumber {
		return Finding{}, true
	}

	isSupported := slices.ContainsFunc(SupportedVersions, func(supported int) bool {
		return version.String() == fmt.Sprint(supported)
	})
	if isSupported {
		return Finding{}, true
	}

	return NewFinding("unsupported_version", "/version",
		fmt.Sprintf("version %s is not supported by this binary", version),
		fmt.Sprintf("use \"version\": %d; run `task-view version` for supported versions", SchemaVersion)), false
}

// schemaFindings flattens a validation error into one finding per leaf
// failure, each with the JSON Pointer of the failing value.
func schemaFindings(validationErr *jsonschema.ValidationError) []Finding {
	printer := message.NewPrinter(language.English)
	seen := map[string]bool{}

	var findings []Finding

	var collectLeaves func(failure *jsonschema.ValidationError)
	collectLeaves = func(failure *jsonschema.ValidationError) {
		if len(failure.Causes) > 0 {
			for _, cause := range failure.Causes {
				collectLeaves(cause)
			}

			return
		}

		pointer := ""
		for _, token := range failure.InstanceLocation {
			pointer += "/" + escapePointer(token)
		}

		description := failure.ErrorKind.LocalizedString(printer)
		hint := "fix the value at this path; " + schemaHint

		// The schema forbids the fields of other ref types with `not`, whose
		// generic message does not tell an agent which fields belong where.
		keywordPath := strings.Join(failure.ErrorKind.KeywordPath(), "/")

		isForbiddenField := strings.Contains(failure.SchemaURL, "/not") || keywordPath == "not"
		if isForbiddenField {
			description = "field not allowed for this ref type"
			hint = "file: repo+file; range/hunk: repo+file+lines; site: repo+file+line+contains; commit: repo+sha"
		}

		// Alternative branches of the schema can report the same failure.
		key := pointer + "|" + description
		if seen[key] {
			return
		}

		seen[key] = true
		findings = append(findings, NewFinding("schema_invalid", pointer, description, hint))
	}

	collectLeaves(validationErr)

	if len(findings) == 0 {
		findings = append(findings, NewFinding("schema_invalid", "", validationErr.Error(), schemaHint))
	}

	// Stable, so failures at one path keep the validator's order.
	slices.SortStableFunc(findings, func(left, right Finding) int {
		return strings.Compare(left.Path, right.Path)
	})

	return findings
}

// RefLoc is a ref together with its JSON Pointer in the review file.
type RefLoc struct {
	Ref  *Ref
	Path string
}

// AllRefs lists every ref in the file with its JSON Pointer.
func (r *Review) AllRefs() []RefLoc {
	var refs []RefLoc

	for i, node := range r.Nodes {
		if node.Ref != nil {
			refs = append(refs, RefLoc{Ref: node.Ref, Path: fmt.Sprintf("/nodes/%d/ref", i)})
		}
	}

	for i, edge := range r.Edges {
		if edge.Ref != nil {
			refs = append(refs, RefLoc{Ref: edge.Ref, Path: fmt.Sprintf("/edges/%d/ref", i)})
		}
	}

	for i, claim := range r.Claims {
		for j, evidence := range claim.Evidence {
			refs = append(refs, RefLoc{Ref: evidence, Path: fmt.Sprintf("/claims/%d/evidence/%d", i, j)})
		}
	}

	return refs
}

// crossCheck applies the rules the JSON Schema cannot express: unique ids,
// links between elements, clean paths and known languages. Findings come in
// the order the checks run.
func (r *Review) crossCheck() []Finding {
	repoIDs, findings := checkRepos(r.Repos)
	nodesByID, nodeFindings := checkNodes(r.Nodes)

	findings = append(findings, nodeFindings...)
	findings = append(findings, checkFlowIDs(r.Flows)...)
	findings = append(findings, checkEdgeNodes(r.Edges, nodesByID)...)
	findings = append(findings, checkFlowNodes(r.Flows, nodesByID)...)
	findings = append(findings, checkFlowSteps(r.Flows, nodesByID)...)
	findings = append(findings, checkFlowConnected(r.Flows, r.Edges)...)
	findings = append(findings, checkFlowCoverage(r.Nodes, r.Flows)...)
	findings = append(findings, checkClaimNodes(r.Claims, nodesByID)...)
	findings = append(findings, checkRefs(r.AllRefs(), repoIDs)...)

	return findings
}

// checkRepos reports duplicate repo ids and unknown language names, and
// returns the repo ids refs may use.
func checkRepos(repos []*Repo) (map[string]bool, []Finding) {
	repoIDs := map[string]bool{}

	var findings []Finding

	for i, repo := range repos {
		if repoIDs[repo.ID] {
			findings = append(findings, duplicateID(fmt.Sprintf("/repos/%d/id", i), "repo", repo.ID))
		}

		repoIDs[repo.ID] = true

		for _, mapping := range repo.Languages {
			name := mapping.String()
			if !langs.Known(name) {
				path := fmt.Sprintf("/repos/%d/languages/%s", i, escapePointer(mapping.Key))
				findings = append(findings, unknownLanguage(path, name))
			}
		}
	}

	return repoIDs, findings
}

// checkNodes reports duplicate node ids and changed nodes without a why, and
// returns the nodes by id. With duplicates, the last node wins.
func checkNodes(nodes []*Node) (map[string]*Node, []Finding) {
	nodesByID := map[string]*Node{}

	var findings []Finding

	for i, node := range nodes {
		if _, duplicate := nodesByID[node.ID]; duplicate {
			findings = append(findings, duplicateID(fmt.Sprintf("/nodes/%d/id", i), "node", node.ID))
		}

		nodesByID[node.ID] = node

		isUnexplainedChange := node.IsChanged() && strings.TrimSpace(node.Why) == ""
		if isUnexplainedChange {
			findings = append(findings, NewFinding("missing_why", fmt.Sprintf("/nodes/%d", i),
				fmt.Sprintf("node %q is %s but has no why", node.ID, node.Change),
				"add a one-sentence `why` giving the reason for the change"))
		}
	}

	return nodesByID, findings
}

// checkFlowIDs reports duplicate flow ids.
func checkFlowIDs(flows []*Flow) []Finding {
	flowIDs := map[string]bool{}

	var findings []Finding

	for i, flow := range flows {
		if flowIDs[flow.ID] {
			findings = append(findings, duplicateID(fmt.Sprintf("/flows/%d/id", i), "flow", flow.ID))
		}

		flowIDs[flow.ID] = true
	}

	return findings
}

// checkEdgeNodes reports edge ends that name no node.
func checkEdgeNodes(edges []*Edge, nodesByID map[string]*Node) []Finding {
	var findings []Finding

	for i, edge := range edges {
		if nodesByID[edge.From] == nil {
			findings = append(findings, unknownNode(fmt.Sprintf("/edges/%d/from", i), edge.From))
		}

		if nodesByID[edge.To] == nil {
			findings = append(findings, unknownNode(fmt.Sprintf("/edges/%d/to", i), edge.To))
		}
	}

	return findings
}

// checkFlowNodes reports flow steps and tests that name no node, and flow
// tests that are not test nodes.
func checkFlowNodes(flows []*Flow, nodesByID map[string]*Node) []Finding {
	var findings []Finding

	for i, flow := range flows {
		for j, id := range flow.Path {
			if nodesByID[id] == nil {
				findings = append(findings, unknownNode(fmt.Sprintf("/flows/%d/path/%d", i, j), id))
			}
		}

		for j, id := range flow.Tests {
			path := fmt.Sprintf("/flows/%d/tests/%d", i, j)

			node := nodesByID[id]
			if node == nil {
				findings = append(findings, unknownNode(path, id))

				continue
			}

			if !node.IsTest() {
				findings = append(findings, NewFinding("not_a_test", path,
					fmt.Sprintf("node %q has kind %q, not test", id, node.Kind),
					"list only nodes of kind `test` in flow tests; put other nodes in `path`"))
			}
		}
	}

	return findings
}

// checkFlowSteps reports flow steps of a kind that never sits on a flow, and
// flows with no code to walk through. Unknown ids are left to checkFlowNodes.
func checkFlowSteps(flows []*Flow, nodesByID map[string]*Node) []Finding {
	var findings []Finding

	for i, flow := range flows {
		for j, id := range flow.Path {
			node := nodesByID[id]
			if node == nil || node.CanBeFlowStep() {
				continue
			}

			findings = append(findings, NewFinding("not_flow_step", fmt.Sprintf("/flows/%d/path/%d", i, j),
				fmt.Sprintf("node %q has kind %q, which never sits on a flow", id, node.Kind),
				"keep only code in `path`: tests reach code through `tests` edges; docs, config and imports stay off flows"))
		}

		hasCode := slices.ContainsFunc(flow.Path, func(id string) bool {
			node := nodesByID[id]

			return node != nil && node.IsFlowSubject()
		})
		if !hasCode {
			findings = append(findings, NewFinding("flow_without_code", fmt.Sprintf("/flows/%d", i),
				fmt.Sprintf("flow %q has no code, component or entry node", flow.ID),
				"add the code its entry runs to `path`, or drop the flow"))
		}
	}

	return findings
}

// checkFlowConnected reports flow steps the entry cannot reach through edges
// between the flow's own nodes.
func checkFlowConnected(flows []*Flow, edges []*Edge) []Finding {
	var findings []Finding

	for i, flow := range flows {
		reached := reachableSteps(flow.Path, edges)

		for j, id := range flow.Path {
			if reached[id] {
				continue
			}

			findings = append(findings, NewFinding("flow_gap", fmt.Sprintf("/flows/%d/path/%d", i, j),
				fmt.Sprintf("flow %q: no edges between its nodes join %q to the entry %q", flow.ID, id, flow.Path[0]),
				"add the edge that links this node to the flow, or the unchanged node between them (rule 6 under Nodes)"))
		}
	}

	return findings
}

// reachableSteps returns the steps of path that its first step reaches
// through edges with both ends in path. Edges count in either direction,
// since a flow runs against some of them (an endpoint reaches the handler
// that serves it); `tests` edges never join steps.
func reachableSteps(path []string, edges []*Edge) map[string]bool {
	if len(path) == 0 {
		return map[string]bool{}
	}

	inFlow := map[string]bool{}
	for _, id := range path {
		inFlow[id] = true
	}

	neighbours := map[string][]string{}

	for _, edge := range edges {
		joinsSteps := edge.Kind != EdgeTests && inFlow[edge.From] && inFlow[edge.To]
		if !joinsSteps {
			continue
		}

		neighbours[edge.From] = append(neighbours[edge.From], edge.To)
		neighbours[edge.To] = append(neighbours[edge.To], edge.From)
	}

	reached := map[string]bool{path[0]: true}
	queue := []string{path[0]}

	for len(queue) > 0 {
		step := queue[0]
		queue = queue[1:]

		for _, next := range neighbours[step] {
			if reached[next] {
				continue
			}

			reached[next] = true
			queue = append(queue, next)
		}
	}

	return reached
}

// checkFlowCoverage reports added or modified code that no flow walks
// through, so the flows cover the whole change. Deleted code no longer runs.
func checkFlowCoverage(nodes []*Node, flows []*Flow) []Finding {
	onFlow := map[string]bool{}

	for _, flow := range flows {
		for _, id := range flow.Path {
			onFlow[id] = true
		}
	}

	var findings []Finding

	for i, node := range nodes {
		isRunningChange := node.IsChanged() && node.Change != ChangeDeleted
		if !isRunningChange || !node.IsFlowSubject() || onFlow[node.ID] {
			continue
		}

		findings = append(findings, NewFinding("node_not_in_flow", fmt.Sprintf("/nodes/%d", i),
			fmt.Sprintf("%s node %q is on no flow", node.Change, node.ID),
			"add it to the `path` of the flow whose entry reaches it; a flow may branch, so add a branch rather than a new flow"))
	}

	return findings
}

// checkClaimNodes reports nodes a claim is about that do not exist.
func checkClaimNodes(claims []*Claim, nodesByID map[string]*Node) []Finding {
	var findings []Finding

	for i, claim := range claims {
		for j, id := range claim.About {
			if nodesByID[id] == nil {
				findings = append(findings, unknownNode(fmt.Sprintf("/claims/%d/about/%d", i, j), id))
			}
		}
	}

	return findings
}

// checkRefs reports refs to unlisted repos, unclean paths, reversed line
// ranges and unknown languages.
func checkRefs(refs []RefLoc, repoIDs map[string]bool) []Finding {
	var findings []Finding

	for _, location := range refs {
		ref := location.Ref

		if !repoIDs[ref.Repo] {
			findings = append(findings, NewFinding("unknown_repo", location.Path+"/repo",
				fmt.Sprintf("no repo with id %q", ref.Repo), "use an id from `repos`, or list the repo"))
		}

		hasUncleanPath := ref.File != "" && !cleanPath(ref.File)
		if hasUncleanPath {
			findings = append(findings, NewFinding("bad_path", location.Path+"/file",
				fmt.Sprintf("%q is not a clean repo-relative POSIX path", ref.File),
				"use a path relative to the repo root with forward slashes, no leading `/` and no `..`"))
		}

		isReversedSpan := ref.hasLineSpan() && ref.Lines[0] > ref.Lines[1]
		if isReversedSpan {
			findings = append(findings, NewFinding("schema_invalid", location.Path+"/lines",
				fmt.Sprintf("lines [%d, %d] start after they end", ref.Lines[0], ref.Lines[1]),
				"write lines as [start, end] with start ≤ end"))
		}

		hasUnknownLanguage := ref.Language != "" && !langs.Known(ref.Language)
		if hasUnknownLanguage {
			findings = append(findings, unknownLanguage(location.Path+"/language", ref.Language))
		}
	}

	return findings
}

// cleanPath reports whether path is relative to the repo root, uses forward
// slashes and has no `.`, `..` or empty segments that could alias another
// file or leave the repo.
func cleanPath(path string) bool {
	isAbsolute := strings.HasPrefix(path, "/")
	hasBackslash := strings.Contains(path, `\`)
	leavesDirectory := parentDirSegment.MatchString(path)
	startsWithDot := strings.HasPrefix(path, "./")
	hasEmptySegment := strings.Contains(path, "//")

	return !isAbsolute && !hasBackslash && !leavesDirectory && !startsWithDot && !hasEmptySegment
}

func duplicateID(path, element, id string) Finding {
	return NewFinding("duplicate_id", path,
		fmt.Sprintf("%s id %q is used more than once", element, id),
		fmt.Sprintf("give every %s a unique id", element))
}

func unknownNode(path, id string) Finding {
	return NewFinding("unknown_node", path, fmt.Sprintf("no node with id %q", id),
		"use an id from `nodes`, or add the missing node")
}

func unknownLanguage(path, name string) Finding {
	return NewFinding("unknown_language", path, fmt.Sprintf("unknown language %q", name),
		fmt.Sprintf("closest valid names: %s; run `task-view languages` for the full list",
			strings.Join(langs.Closest(name, languageSuggestions), ", ")))
}

// escapePointer escapes one JSON Pointer token (RFC 6901). `~` goes first so
// the `~1` written for `/` is not escaped again.
func escapePointer(token string) string {
	return strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
}
