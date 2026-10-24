package report

import (
	"math"
	"strings"
	"testing"

	"github.com/gtindo/task-view/bench/internal/config"
	"github.com/gtindo/task-view/bench/internal/metrics"
)

func TestJaccard(t *testing.T) {
	cases := []struct {
		left, right []string
		want        float64
	}{
		{nil, nil, 1},
		{[]string{"a", "b"}, []string{"a", "b"}, 1},
		{[]string{"a", "b"}, []string{"b", "c"}, 1.0 / 3},
		{[]string{"a"}, nil, 0},
	}
	for _, testCase := range cases {
		if got := Jaccard(testCase.left, testCase.right); math.Abs(got-testCase.want) > 1e-9 {
			t.Errorf("Jaccard(%v, %v) = %v, want %v", testCase.left, testCase.right, got, testCase.want)
		}
	}

	sets := [][]string{{"a", "b"}, {"a", "b"}, {"a"}}
	if got := MeanPairwiseJaccard(sets); math.Abs(got-(1+0.5+0.5)/3) > 1e-9 {
		t.Errorf("mean pairwise = %v", got)
	}
}

func run(caseID, size, model string, exits []int, cost float64, nodes []string) metrics.Result {
	return metrics.Result{
		Case: caseID, Project: "p", Size: size, Model: model, ModelLabel: strings.ToUpper(model), Effort: "high",
		Seconds: 60, CostUSD: cost, CostSource: "harness",
		Calls:  metrics.Calls{Validate: len(exits), ValidateExits: exits},
		Final:  metrics.Final{Exit: 0, Warnings: map[string]int{}},
		Review: metrics.Shape{Exists: true, NodeKeys: nodes},
		Change: metrics.Change{Added: 150, Removed: 50},
	}
}

func TestAggregateAndRender(t *testing.T) {
	results := []metrics.Result{
		run("p-big", "big", "opus", []int{0}, 2, []string{"a"}),
		run("p-small", "small", "opus", []int{2, 0}, 1, []string{"a", "b"}),
		run("p-small", "small", "opus", []int{0}, 3, []string{"a"}),
	}
	results[2].Final.Warnings["unmentioned_hunk"] = 1

	cells := Aggregate(results)
	if len(cells) != 2 || cells[0].Case != "p-small" {
		t.Fatalf("cells not ordered small first: %+v", cells)
	}

	small := cells[0]
	if small.Runs != 2 || small.Accurate != 1 || small.FirstPass != 1 || small.Cost.Mean != 2 || small.Cost.Min != 1 || small.Cost.Max != 3 {
		t.Errorf("small cell: %+v", small)
	}

	if math.Abs(small.NodeAgreement-0.5) > 1e-9 || math.Abs(small.CostPer100Lines()-1) > 1e-9 {
		t.Errorf("agreement %v, per 100 lines %v", small.NodeAgreement, small.CostPer100Lines())
	}

	markdown := Markdown(Meta{TaskView: "task-view 1.0", Harness: "stub"}, cells)
	for _, want := range []string{"| p-small | small | OPUS | high | 2 | 1/2 | 1/2 |", "## By model and effort", "| OPUS | high | 3 |"} {
		if !strings.Contains(markdown, want) {
			t.Errorf("markdown lacks %q:\n%s", want, markdown)
		}
	}

	if rows := strings.Count(string(CSV(cells)), "\n"); rows != 3 {
		t.Errorf("CSV has %d lines, want a header and 2 cells", rows)
	}
}

func TestLatest(t *testing.T) {
	results := []metrics.Result{
		{RunID: "a", HarnessError: "usage limit"},
		{RunID: "b"},
		{RunID: "a", Seconds: 5},
	}

	latest := Latest(results)
	if len(latest) != 2 || latest[0].RunID != "a" || latest[0].HarnessError != "" || latest[0].Seconds != 5 {
		t.Errorf("latest = %+v", latest)
	}
}

func TestBenchMarkdown(t *testing.T) {
	results := []metrics.Result{
		run("p-small", "small", "haiku", []int{0}, 0.1, []string{"a"}),
		run("p-small", "small", "opus", []int{0}, 1, []string{"a"}),
		run("p-big", "big", "haiku", []int{1, 0}, 0.2, []string{"a"}),
		run("p-big", "big", "opus", []int{0}, 2, []string{"a"}),
	}
	results[2].Final.Exit = 1
	results[0].StartedAt = "2026-10-24T10:00:00Z"
	results[3].StartedAt = "2026-10-25T01:00:00Z"

	bench := Bench{
		Meta: Meta{
			TaskView:       "task-view 1.1.0",
			Harness:        "claude-code",
			HarnessVersion: "2.1.304 (Claude Code)",
			Matrix: config.Matrix{
				Models:  []config.Model{{ID: "haiku", Label: "HAIKU", Price: "h"}, {ID: "opus", Label: "OPUS", Price: "o"}},
				Efforts: []string{"high"},
				Repeats: 1,
				Timeout: "20m",
			},
			PriceList: config.Prices{AsOf: "2026-10-24", Models: map[string]config.Price{"h": {Input: 0.1, Output: 0.5, CacheRead: 0.01, CacheWrite: 0.125}}},
		},
		Suite: config.Suite{
			Projects: []config.Project{{ID: "p", Language: "Go"}},
			Cases: []config.Case{
				{ID: "p-small", Project: "p", Size: "small", Commits: []string{"x"}, Title: "Small", Source: "https://example.com"},
				{ID: "p-big", Project: "p", Size: "big", Commits: []string{"y"}},
				{ID: "p-unrun", Project: "p", Size: "medium", Commits: []string{"z"}},
			},
		},
		Results:   results,
		Cells:     Aggregate(results),
		Archive:   "bench/results/reviews.tar.xz",
		Dashboard: "https://claude.ai/artifact/example",
	}

	markdown := BenchMarkdown(bench)
	if strings.Index(markdown, "| big | HAIKU | high |") > strings.Index(markdown, "| big | OPUS | high |") {
		t.Errorf("rows not in matrix model order:\n%s", markdown)
	}

	for _, want := range []string{
		"- Date: 2026-10-24 to 2026-10-25",
		"- task-view: `task-view 1.1.0`",
		"`claude-code` (`2.1.304 (Claude Code)`)",
		"| HAIKU | 0.1 | 0.5 | 0.01 | 0.125 |",
		"| OPUS | – | – | – | – |",
		"| small | HAIKU, high effort | 0.100 |",
		"| big | OPUS, high effort | 2.000 |",
		"| big | HAIKU | high | 1 | 0/1 | 0/1 |",
		"| [p-small](bench/briefs/p-small.md) | small | Go | 1 | 0 | 200 | [Small](https://example.com) |",
		"tar -xJf bench/results/reviews.tar.xz",
		"[task-view benchmark dashboard](https://claude.ai/artifact/example)",
		"Every run still includes the full diff",
		"## Tokens by project",
		"| p | HAIKU | high | 2 | 0.00 | 0.0 | 0.150 |",
		"- **small**: a focused fix in a couple of files (p-small; 200 lines)",
		"- **big**: a large pull request across many files (p-big; 200 lines)",
		"- **medium**: a feature or refactor across a few files (p-unrun)\n",
	} {
		if !strings.Contains(markdown, want) {
			t.Errorf("BENCH.md lacks %q:\n%s", want, markdown)
		}
	}
}

func TestApplySizes(t *testing.T) {
	results := []metrics.Result{{Case: "x-big", Size: "big"}, {Case: "gone", Size: "small"}}
	suite := config.Suite{Cases: []config.Case{{ID: "x-big", Size: "agent-scale"}}}

	labelled := ApplySizes(results, suite)
	if labelled[0].Size != "agent-scale" || labelled[1].Size != "small" || results[0].Size != "big" {
		t.Errorf("labelled %+v from %+v", labelled, results)
	}
}

// A cheaper setting whose repeats drew different maps is not recommended,
// however accurate each run was.
func TestChoiceNeedsConsistentMaps(t *testing.T) {
	results := []metrics.Result{
		run("p-small", "small", "haiku", []int{0}, 0.1, []string{"a"}),
		run("p-small", "small", "haiku", []int{0}, 0.1, []string{"b"}),
		run("p-small", "small", "opus", []int{0}, 1, []string{"a"}),
		run("p-small", "small", "opus", []int{0}, 1, []string{"a"}),
	}

	var out strings.Builder
	writeChoiceSection(&out, config.Suite{}, results, Aggregate(results))
	if !strings.Contains(out.String(), "| small | OPUS, high effort | 1.000 |") {
		t.Errorf("choice:\n%s", out.String())
	}
}
