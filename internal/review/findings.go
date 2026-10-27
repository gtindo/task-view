package review

// Level of a finding: errors decide the exit code, warnings never do.
type Level string

const (
	// LevelError marks a finding that fails the check.
	LevelError Level = "error"
	// LevelWarning marks a finding worth a look that does not fail the check.
	LevelWarning Level = "warning"
)

// Reference statuses: every node, edge and claim ends with one of them.
// StatusAgent marks an element without a ref, which is only displayed.
const (
	StatusVerified     = "verified"
	StatusContradicted = "contradicted"
	StatusUnresolved   = "unresolved"
	StatusAgent        = "agent"
)

// Exit codes, ordered by severity: the highest one among the error
// findings wins.
const (
	// ExitOK means no error findings.
	ExitOK = 0
	// ExitReferences means the review file disagrees with the repositories.
	ExitReferences = 1
	// ExitSchema means the review file itself is invalid.
	ExitSchema = 2
	// ExitEnv means the environment prevents checking (no git, missing repo).
	ExitEnv = 3
)

// Finding is one error or warning about the review file.
type Finding struct {
	Code    string `json:"code"`
	Level   Level  `json:"-"`
	Path    string `json:"path,omitempty"`
	Status  string `json:"status,omitempty"`
	Repo    string `json:"repo,omitempty"`
	File    string `json:"file,omitempty"`
	Lines   []int  `json:"lines,omitempty"`
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// CodeInfo describes one entry of the code catalog.
type CodeInfo struct {
	Level Level
	Exit  int
	// Status is the reference status an error code gives the element it
	// concerns; empty for codes that are not about one reference.
	Status string
	Title  string
}

// Catalog is the complete list of codes for schema version 1.
var Catalog = map[string]CodeInfo{
	"file_unreadable":     {LevelError, ExitSchema, "", "Review file unreadable"},
	"unsupported_version": {LevelError, ExitSchema, "", "Unsupported version"},
	"schema_invalid":      {LevelError, ExitSchema, "", "Schema violation"},
	"duplicate_id":        {LevelError, ExitSchema, "", "Duplicate id"},
	"unknown_node":        {LevelError, ExitSchema, "", "Unknown node"},
	"not_a_test":          {LevelError, ExitSchema, "", "Not a test node"},
	"unknown_repo":        {LevelError, ExitSchema, "", "Unknown repo"},
	"bad_path":            {LevelError, ExitSchema, "", "Bad path"},
	"unknown_language":    {LevelError, ExitSchema, "", "Unknown language"},
	"not_flow_step":       {LevelError, ExitSchema, "", "Not a flow step"},

	"git_not_found":  {LevelError, ExitEnv, "", "git not found"},
	"repo_not_found": {LevelError, ExitEnv, "", "Repo not found"},
	"not_repo_root":  {LevelError, ExitEnv, "", "Not a repo root"},
	"bad_base":       {LevelError, ExitEnv, "", "Bad base or tip"},

	"commit_not_found":    {LevelError, ExitReferences, StatusUnresolved, "Commit not found"},
	"commit_chain_broken": {LevelError, ExitReferences, "", "Commit chain broken"},
	"context_commits":     {LevelError, ExitReferences, "", "Context repo commits"},
	"bad_at":              {LevelError, ExitReferences, StatusUnresolved, "Bad at"},
	"file_not_found":      {LevelError, ExitReferences, StatusUnresolved, "File not found"},
	"range_out_of_bounds": {LevelError, ExitReferences, StatusUnresolved, "Range out of bounds"},
	"binary_file":         {LevelError, ExitReferences, StatusUnresolved, "Binary file"},
	"change_mismatch":     {LevelError, ExitReferences, StatusContradicted, "Change mismatch"},
	"site_mismatch":       {LevelError, ExitReferences, StatusContradicted, "Site mismatch"},
	"hunk_not_found":      {LevelError, ExitReferences, StatusContradicted, "Hunk not found"},
	"commit_not_listed":   {LevelError, ExitReferences, StatusContradicted, "Commit not listed"},

	"site_offset":            {LevelWarning, ExitOK, "", "Site offset"},
	"unmentioned_hunk":       {LevelWarning, ExitOK, "", "Unmentioned hunk"},
	"unreferenced_test_hunk": {LevelWarning, ExitOK, "", "Unreferenced test hunk"},
	"unlisted_repo":          {LevelWarning, ExitOK, "", "Unlisted repo"},
	"uncommitted_changes":    {LevelWarning, ExitOK, "", "Uncommitted changes"},
	"head_ahead":             {LevelWarning, ExitOK, "", "HEAD ahead"},
	"worktree_changed":       {LevelWarning, ExitOK, "", "Worktree changed"},
	"no_language":            {LevelWarning, ExitOK, "", "No language"},
	"missing_why":            {LevelWarning, ExitOK, "", "Missing why"},
	"empty_change":           {LevelWarning, ExitOK, "", "Empty change"},
	"flow_gap":               {LevelWarning, ExitOK, "", "Flow gap"},
	"flow_without_code":      {LevelWarning, ExitOK, "", "Flow without code"},
	"node_not_in_flow":       {LevelWarning, ExitOK, "", "Node not in a flow"},
}

// NewFinding builds a finding with the level taken from the catalog.
func NewFinding(code, path, message, hint string) Finding {
	info, ok := Catalog[code]
	if !ok {
		panic("unknown finding code " + code)
	}

	return Finding{
		Code:    code,
		Level:   info.Level,
		Path:    path,
		Status:  info.Status,
		Message: message,
		Hint:    hint,
	}
}

// ExitCode returns the exit code for a set of findings: the most severe exit
// of its error findings, or ExitOK.
func ExitCode(findings []Finding) int {
	exit := ExitOK

	for _, finding := range findings {
		if finding.Level != LevelError {
			continue
		}

		exit = max(exit, Catalog[finding.Code].Exit)
	}

	return exit
}

// Split separates errors from warnings, keeping order. Both results are
// non-nil so they encode as JSON arrays.
func Split(findings []Finding) (errors, warnings []Finding) {
	errors, warnings = []Finding{}, []Finding{}

	for _, finding := range findings {
		if finding.Level == LevelError {
			errors = append(errors, finding)

			continue
		}

		warnings = append(warnings, finding)
	}

	return errors, warnings
}
