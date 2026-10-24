package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShippedSuiteLoads(t *testing.T) {
	suite, err := LoadSuite(filepath.Join("..", "..", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}

	if len(suite.Projects) != 4 || len(suite.Cases) != 10 {
		t.Fatalf("got %d projects and %d cases, want 4 and 10", len(suite.Projects), len(suite.Cases))
	}

	sizes := map[string]map[string]bool{}
	for _, task := range suite.Cases {
		if sizes[task.Project] == nil {
			sizes[task.Project] = map[string]bool{}
		}

		sizes[task.Project][task.Size] = true
	}

	// flowmap adds one agent-scale Go case; the other projects have one
	// case of each hand-written size.
	for project, seen := range sizes {
		if project == "flowmap" {
			continue
		}

		if len(seen) != len(Sizes)-1 || seen["agent-scale"] {
			t.Errorf("project %s has sizes %v, want one case of each", project, seen)
		}
	}

	matrix, err := LoadMatrix(filepath.Join("..", "..", "matrix.json"))
	if err != nil {
		t.Fatal(err)
	}

	if got := len(Expand(suite, matrix)); got != 10*3*2*3 {
		t.Errorf("the shipped matrix expands to %d runs, want 180", got)
	}

	if _, err := matrix.TimeoutDuration(); err != nil {
		t.Errorf("shipped timeout: %v", err)
	}

	if _, err := LoadHarness(filepath.Join("..", "..", "harness", "claude-code")); err != nil {
		t.Errorf("shipped harness: %v", err)
	}
}

func TestSuiteCheck(t *testing.T) {
	sha := strings.Repeat("a", 40)
	suite := Suite{
		Projects: []Project{{ID: "p"}},
		Cases: []Case{
			{ID: "ok", Project: "p", Size: "small", Base: sha, Commits: []string{sha}},
			{ID: "ok", Project: "q", Size: "huge", Base: "abc", Commits: nil},
		},
	}

	err := suite.check()
	if err == nil {
		t.Fatal("no error for a broken suite")
	}

	for _, want := range []string{"listed twice", "unknown project", "size", "no commits", "not a full SHA"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error lacks %q: %v", want, err)
		}
	}
}

func TestApplyAndExpand(t *testing.T) {
	matrix := Matrix{
		Harness: "harness/claude-code",
		Models:  []Model{{ID: "haiku", Label: "Haiku", Price: "claude-haiku"}},
		Efforts: []string{"medium", "high"},
		Repeats: 3,
		Cases:   []string{"*"},
	}

	applied := matrix.Apply(Overrides{Models: []string{"haiku", "gpt-x"}, Efforts: []string{"low"}, Cases: []string{"a-*"}, Repeats: 1})
	if len(applied.Models) != 2 || applied.Models[0].Label != "Haiku" || applied.Models[1].Price != "gpt-x" {
		t.Errorf("models: %+v", applied.Models)
	}

	if matrix.Repeats != 3 {
		t.Error("Apply changed the original matrix")
	}

	suite := Suite{Cases: []Case{{ID: "a-small"}, {ID: "b-small"}}}
	runs := Expand(suite, applied)
	if len(runs) != 2 || runs[0].ID != "a-small__haiku__low__1" || runs[1].Model.ID != "gpt-x" {
		t.Errorf("runs: %+v", runs)
	}
}

func TestRenderPrompt(t *testing.T) {
	template := "{{brief}}|{{task_view}}|{{review_file}}|{{base}}|{{state}}"
	cases := []struct {
		commits     int
		uncommitted bool
		state       string
	}{
		{1, true, "None of the work is committed yet"},
		{3, true, "You made 2 commits"},
		{2, true, "You made 1 commit on"},
		{2, false, "All of the work is committed"},
	}
	for _, testCase := range cases {
		got := RenderPrompt(template, PromptInput{
			Brief: " brief\n", TaskView: "/tv", ReviewFile: ".review/x.json", Base: "b",
			Commits: testCase.commits, Uncommitted: testCase.uncommitted,
		})
		if !strings.HasPrefix(got, "brief|/tv|.review/x.json|b|") || !strings.Contains(got, testCase.state) {
			t.Errorf("%d commits, uncommitted %v: %q", testCase.commits, testCase.uncommitted, got)
		}
	}
}
