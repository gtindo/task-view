// Package metrics turns what one run leaves behind (the harness's usage
// file, the task-view call log, validate and gather output, the review file)
// into a Result. Every function is pure: it takes bytes and returns values.
package metrics

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gtindo/task-view/bench/internal/config"
)

// Usage is what a harness reports about one agent session.
type Usage struct {
	InputTokens      int      `json:"input_tokens"`
	OutputTokens     int      `json:"output_tokens"`
	CacheReadTokens  int      `json:"cache_read_tokens"`
	CacheWriteTokens int      `json:"cache_write_tokens"`
	CostUSD          float64  `json:"cost_usd,omitempty"`
	Turns            int      `json:"turns,omitempty"`
	Models           []string `json:"models,omitempty"`
}

// TotalTokens counts every token the session processed.
func (u Usage) TotalTokens() int {
	return u.InputTokens + u.OutputTokens + u.CacheReadTokens + u.CacheWriteTokens
}

// genericUsage is the usage-json format of harness.md.
type genericUsage struct {
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	CacheReadTokens  int     `json:"cache_read_tokens"`
	CacheWriteTokens int     `json:"cache_write_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	Turns            int     `json:"turns"`
	Model            string  `json:"model"`
}

// claudeResult is the part of Claude Code's `--output-format json` result
// the benchmark reads.
type claudeResult struct {
	IsError      bool    `json:"is_error"`
	NumTurns     int     `json:"num_turns"`
	TotalCostUSD float64 `json:"total_cost_usd"`
	Usage        struct {
		InputTokens              int `json:"input_tokens"`
		OutputTokens             int `json:"output_tokens"`
		CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	} `json:"usage"`
	ModelUsage map[string]struct {
		InputTokens              int `json:"inputTokens"`
		OutputTokens             int `json:"outputTokens"`
		CacheReadInputTokens     int `json:"cacheReadInputTokens"`
		CacheCreationInputTokens int `json:"cacheCreationInputTokens"`
	} `json:"modelUsage"`
}

// ParseUsage reads a harness's usage file in one of the formats harness.md
// names. Claude Code's per-model totals are preferred over its top-level
// usage, since they add up every model the session called.
func ParseUsage(format string, data []byte) (Usage, error) {
	switch format {
	case "usage-json":
		var generic genericUsage
		if err := json.Unmarshal(data, &generic); err != nil {
			return Usage{}, err
		}

		usage := Usage{
			InputTokens:      generic.InputTokens,
			OutputTokens:     generic.OutputTokens,
			CacheReadTokens:  generic.CacheReadTokens,
			CacheWriteTokens: generic.CacheWriteTokens,
			CostUSD:          generic.CostUSD,
			Turns:            generic.Turns,
		}
		if generic.Model != "" {
			usage.Models = []string{generic.Model}
		}

		return usage, nil
	case "claude-code":
		return parseClaude(data)
	}

	return Usage{}, fmt.Errorf("unknown usage format %q", format)
}

func parseClaude(data []byte) (Usage, error) {
	var result claudeResult
	if err := json.Unmarshal(data, &result); err != nil {
		return Usage{}, err
	}

	usage := Usage{CostUSD: result.TotalCostUSD, Turns: result.NumTurns}
	if len(result.ModelUsage) == 0 {
		usage.InputTokens = result.Usage.InputTokens
		usage.OutputTokens = result.Usage.OutputTokens
		usage.CacheReadTokens = result.Usage.CacheReadInputTokens
		usage.CacheWriteTokens = result.Usage.CacheCreationInputTokens
	}

	for model, counts := range result.ModelUsage {
		usage.Models = append(usage.Models, model)
		usage.InputTokens += counts.InputTokens
		usage.OutputTokens += counts.OutputTokens
		usage.CacheReadTokens += counts.CacheReadInputTokens
		usage.CacheWriteTokens += counts.CacheCreationInputTokens
	}

	sort.Strings(usage.Models)
	if result.IsError {
		return usage, errors.New("the Claude Code session ended with an error")
	}

	return usage, nil
}

// Cost is the session's cost and where it came from: the harness's own
// figure when it gave one, else tokens times the list price. The source is
// empty when neither is known.
func Cost(usage Usage, price config.Price, havePrice bool) (float64, string) {
	if usage.CostUSD > 0 {
		return usage.CostUSD, "harness"
	}

	if !havePrice || usage.TotalTokens() == 0 {
		return 0, ""
	}

	const perMillion = 1e6
	dollars := (float64(usage.InputTokens)*price.Input +
		float64(usage.OutputTokens)*price.Output +
		float64(usage.CacheReadTokens)*price.CacheRead +
		float64(usage.CacheWriteTokens)*price.CacheWrite) / perMillion

	return dollars, "prices"
}

// Calls counts what the agent ran through the task-view wrapper.
type Calls struct {
	Total         int   `json:"total"`
	Validate      int   `json:"validate"`
	ValidateExits []int `json:"validate_exits"`
	Gather        int   `json:"gather"`
	Schema        int   `json:"schema"`
	Instructions  int   `json:"instructions"`
}

// callLogFields is the number of tab-separated fields per wrapper log line:
// start time, exit code, arguments.
const callLogFields = 3

// ParseCallLog reads the wrapper's log, one call per line.
func ParseCallLog(data []byte) Calls {
	calls := Calls{ValidateExits: []int{}}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		fields := strings.SplitN(line, "\t", callLogFields)
		if len(fields) < callLogFields {
			continue
		}

		calls.Total++
		exit, _ := strconv.Atoi(fields[1])
		command := strings.Fields(fields[2])
		if len(command) == 0 {
			continue
		}

		switch command[0] {
		case "validate":
			calls.Validate++
			calls.ValidateExits = append(calls.ValidateExits, exit)
		case "gather":
			calls.Gather++
		case "schema":
			calls.Schema++
		case "instructions":
			calls.Instructions++
		}
	}

	return calls
}

// Final is the review file's state after the run, from one validate.
type Final struct {
	Exit            int            `json:"exit"`
	Errors          map[string]int `json:"errors"`
	Warnings        map[string]int `json:"warnings"`
	Verified        int            `json:"verified"`
	Contradicted    int            `json:"contradicted"`
	Unresolved      int            `json:"unresolved"`
	AgentStatements int            `json:"agent_statements"`
}

// Accurate reports whether the account matches the diff: exit 0 and no
// changed line left out.
func (f Final) Accurate() bool {
	return f.Exit == 0 && f.Warnings["unmentioned_hunk"] == 0
}

type validateOutput struct {
	ExitCode int `json:"exitCode"`
	Errors   []struct {
		Code string `json:"code"`
	} `json:"errors"`
	Warnings []struct {
		Code string `json:"code"`
	} `json:"warnings"`
	Summary struct {
		Verified        int `json:"verified"`
		Contradicted    int `json:"contradicted"`
		Unresolved      int `json:"unresolved"`
		AgentStatements int `json:"agentStatements"`
	} `json:"summary"`
}

// ParseValidate reads `task-view validate --format json` output.
func ParseValidate(data []byte) (Final, error) {
	var output validateOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return Final{}, err
	}

	final := Final{
		Exit:            output.ExitCode,
		Errors:          map[string]int{},
		Warnings:        map[string]int{},
		Verified:        output.Summary.Verified,
		Contradicted:    output.Summary.Contradicted,
		Unresolved:      output.Summary.Unresolved,
		AgentStatements: output.Summary.AgentStatements,
	}
	for _, finding := range output.Errors {
		final.Errors[finding.Code]++
	}

	for _, finding := range output.Warnings {
		final.Warnings[finding.Code]++
	}

	return final, nil
}

// Change is the size of a task's change, as gather reads it.
type Change struct {
	Files   int `json:"files"`
	Hunks   int `json:"hunks"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// Lines counts added and removed lines.
func (c Change) Lines() int { return c.Added + c.Removed }

type gatherOutput struct {
	Files []struct {
		Added   int               `json:"added"`
		Removed int               `json:"removed"`
		Hunks   []json.RawMessage `json:"hunks"`
	} `json:"files"`
}

// ParseGather reads `task-view gather --format json` output.
func ParseGather(data []byte) (Change, error) {
	var output gatherOutput
	if err := json.Unmarshal(data, &output); err != nil {
		return Change{}, err
	}

	change := Change{Files: len(output.Files)}
	for _, file := range output.Files {
		change.Hunks += len(file.Hunks)
		change.Added += file.Added
		change.Removed += file.Removed
	}

	return change, nil
}

// Shape is the review file's structure, with keys that identify its nodes
// and edges across runs for the consistency measure.
type Shape struct {
	Exists   bool     `json:"exists"`
	Nodes    int      `json:"nodes"`
	Edges    int      `json:"edges"`
	Flows    int      `json:"flows"`
	Claims   int      `json:"claims"`
	Notes    int      `json:"notes"`
	NodeKeys []string `json:"node_keys"`
	EdgeKeys []string `json:"edge_keys"`
}

type reviewFile struct {
	Nodes []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Change string `json:"change"`
		Ref    *struct {
			Type  string `json:"type"`
			Repo  string `json:"repo"`
			File  string `json:"file"`
			Lines []int  `json:"lines"`
		} `json:"ref"`
	} `json:"nodes"`
	Edges []struct {
		From string `json:"from"`
		To   string `json:"to"`
		Kind string `json:"kind"`
	} `json:"edges"`
	Flows  []json.RawMessage `json:"flows"`
	Claims []json.RawMessage `json:"claims"`
	Notes  []json.RawMessage `json:"notes"`
}

// ParseReview reads a review file's shape. A node's key is where it points
// (file and lines, the file, or its label when it has no ref), since ids
// differ between runs while the code they describe does not. NodeKeys holds
// changed nodes only: unchanged context nodes are a separate choice.
func ParseReview(data []byte) (Shape, error) {
	var review reviewFile
	if err := json.Unmarshal(data, &review); err != nil {
		return Shape{Exists: true}, err
	}

	shape := Shape{
		Exists:   true,
		Nodes:    len(review.Nodes),
		Edges:    len(review.Edges),
		Flows:    len(review.Flows),
		Claims:   len(review.Claims),
		Notes:    len(review.Notes),
		NodeKeys: []string{},
		EdgeKeys: []string{},
	}

	keyByID := map[string]string{}
	for _, node := range review.Nodes {
		key := "noref:" + strings.ToLower(node.Label)
		switch {
		case node.Ref != nil && node.Ref.Type == "range" && len(node.Ref.Lines) == 2:
			key = fmt.Sprintf("%s:%d-%d", node.Ref.File, node.Ref.Lines[0], node.Ref.Lines[1])
		case node.Ref != nil && node.Ref.File != "":
			key = node.Ref.File + ":file"
		}

		keyByID[node.ID] = key
		if node.Change != "unchanged" {
			shape.NodeKeys = append(shape.NodeKeys, key)
		}
	}

	for _, edge := range review.Edges {
		shape.EdgeKeys = append(shape.EdgeKeys, keyByID[edge.From]+"|"+edge.Kind+"|"+keyByID[edge.To])
	}

	sort.Strings(shape.NodeKeys)
	sort.Strings(shape.EdgeKeys)

	return shape, nil
}

// Result is one run's line in runs.jsonl.
type Result struct {
	RunID      string `json:"run_id"`
	Case       string `json:"case"`
	Project    string `json:"project"`
	Size       string `json:"size"`
	Model      string `json:"model"`
	ModelLabel string `json:"model_label"`
	Effort     string `json:"effort"`
	Repeat     int    `json:"repeat"`
	Harness    string `json:"harness"`
	StartedAt  string `json:"started_at"`

	Seconds      float64 `json:"seconds"`
	HarnessError string  `json:"harness_error,omitempty"`
	TimedOut     bool    `json:"timed_out,omitempty"`

	Usage      Usage   `json:"usage"`
	CostUSD    float64 `json:"cost_usd"`
	CostSource string  `json:"cost_source"`

	Calls  Calls  `json:"calls"`
	Final  Final  `json:"final"`
	Review Shape  `json:"review"`
	Change Change `json:"change"`

	// WorkTouched is true when the agent changed the task's code, which the
	// prompt forbids: the account then describes a different change.
	WorkTouched bool `json:"work_touched,omitempty"`
}
