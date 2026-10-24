package metrics

import (
	"math"
	"slices"
	"testing"

	"github.com/gtindo/task-view/bench/internal/config"
)

func TestParseUsageClaudeCode(t *testing.T) {
	withModels := []byte(`{"is_error":false,"num_turns":7,"total_cost_usd":0.25,
		"usage":{"input_tokens":1,"output_tokens":2},
		"modelUsage":{"claude-haiku-5-5":{"inputTokens":10,"outputTokens":20,"cacheReadInputTokens":300,"cacheCreationInputTokens":40},
		              "claude-sonnet-5-5":{"inputTokens":1,"outputTokens":2,"cacheReadInputTokens":3,"cacheCreationInputTokens":4}}}`)
	usage, err := ParseUsage("claude-code", withModels)
	if err != nil {
		t.Fatal(err)
	}

	if usage.InputTokens != 11 || usage.OutputTokens != 22 || usage.CacheReadTokens != 303 || usage.CacheWriteTokens != 44 {
		t.Errorf("per-model totals: %+v", usage)
	}

	if usage.CostUSD != 0.25 || usage.Turns != 7 || !slices.Equal(usage.Models, []string{"claude-haiku-5-5", "claude-sonnet-5-5"}) {
		t.Errorf("cost, turns, models: %+v", usage)
	}

	topLevel, err := ParseUsage("claude-code", []byte(`{"usage":{"input_tokens":5,"output_tokens":6,"cache_read_input_tokens":7,"cache_creation_input_tokens":8}}`))
	if err != nil || topLevel.TotalTokens() != 26 {
		t.Errorf("top-level usage: %+v, %v", topLevel, err)
	}

	if _, err := ParseUsage("claude-code", []byte(`{"is_error":true}`)); err == nil {
		t.Error("an errored session gave no error")
	}
}

func TestParseUsageGeneric(t *testing.T) {
	usage, err := ParseUsage("usage-json", []byte(`{"input_tokens":1,"output_tokens":2,"cost_usd":0.5,"model":"m"}`))
	if err != nil || usage.OutputTokens != 2 || usage.CostUSD != 0.5 || usage.Models[0] != "m" {
		t.Errorf("%+v, %v", usage, err)
	}

	if _, err := ParseUsage("yaml", nil); err == nil {
		t.Error("unknown format gave no error")
	}
}

func TestCost(t *testing.T) {
	price := config.Price{Input: 2, Output: 10, CacheRead: 0.2, CacheWrite: 2.5}
	usage := Usage{InputTokens: 1_000_000, OutputTokens: 100_000, CacheReadTokens: 1_000_000, CacheWriteTokens: 0}

	cost, source := Cost(usage, price, true)
	if math.Abs(cost-3.2) > 1e-9 || source != "prices" {
		t.Errorf("computed: %v %s", cost, source)
	}

	usage.CostUSD = 1.5
	if cost, source := Cost(usage, price, true); cost != 1.5 || source != "harness" {
		t.Errorf("reported: %v %s", cost, source)
	}

	if _, source := Cost(Usage{InputTokens: 5}, price, false); source != "" {
		t.Errorf("no price: source %q", source)
	}
}

func TestParseCallLog(t *testing.T) {
	log := []byte("1\t0\tinstructions\n2\t0\tgather --format json\n3\t2\tvalidate x --format json\n4\t0\tschema\n5\t0\tvalidate x\nbroken line\n")
	calls := ParseCallLog(log)
	if calls.Total != 5 || calls.Validate != 2 || !slices.Equal(calls.ValidateExits, []int{2, 0}) || calls.Gather != 1 || calls.Schema != 1 || calls.Instructions != 1 {
		t.Errorf("%+v", calls)
	}
}

func TestParseValidateAndGather(t *testing.T) {
	final, err := ParseValidate([]byte(`{"exitCode":0,"errors":[],"warnings":[{"code":"unmentioned_hunk"},{"code":"head_ahead"}],
		"summary":{"verified":9,"contradicted":0,"unresolved":0,"agentStatements":4}}`))
	if err != nil || final.Verified != 9 || final.Warnings["unmentioned_hunk"] != 1 || final.Accurate() {
		t.Errorf("%+v, %v", final, err)
	}

	change, err := ParseGather([]byte(`{"files":[{"added":3,"removed":1,"hunks":[{},{}]},{"added":10,"removed":0}]}`))
	if err != nil || change != (Change{Files: 2, Hunks: 2, Added: 13, Removed: 1}) || change.Lines() != 14 {
		t.Errorf("%+v, %v", change, err)
	}
}

func TestParseReview(t *testing.T) {
	shape, err := ParseReview([]byte(`{"nodes":[
		{"id":"a","label":"A","change":"modified","ref":{"type":"range","file":"x.go","lines":[3,9]}},
		{"id":"b","label":"B","change":"added","ref":{"type":"file","file":"y.go"}},
		{"id":"c","label":"GET /x","change":"unchanged"}],
		"edges":[{"from":"a","to":"b","kind":"calls"}],"flows":[{}],"notes":["n"]}`))
	if err != nil {
		t.Fatal(err)
	}

	if shape.Nodes != 3 || shape.Edges != 1 || shape.Flows != 1 || shape.Notes != 1 {
		t.Errorf("counts: %+v", shape)
	}

	if !slices.Equal(shape.NodeKeys, []string{"x.go:3-9", "y.go:file"}) {
		t.Errorf("node keys (changed only): %v", shape.NodeKeys)
	}

	if !slices.Equal(shape.EdgeKeys, []string{"x.go:3-9|calls|y.go:file"}) {
		t.Errorf("edge keys: %v", shape.EdgeKeys)
	}
}
