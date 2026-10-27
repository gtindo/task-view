package review

import (
	"slices"
	"testing"
)

// flowFixtureNodes is a small change: an entry, code that branches into two
// chains through an unchanged service, an endpoint and the off-flow kinds.
func flowFixtureNodes() []*Node {
	return []*Node{
		{ID: "page", Kind: "entry", Change: ChangeUnchanged},
		{ID: "handler", Kind: "code", Change: ChangeModified},
		{ID: "service", Kind: "code", Change: ChangeUnchanged},
		{ID: "store", Kind: "code", Change: ChangeAdded},
		{ID: "view", Kind: "component", Change: ChangeModified},
		{ID: "ep", Kind: "endpoint", Change: ChangeModified},
		{ID: "old", Kind: "code", Change: ChangeDeleted},
		{ID: "test", Kind: "test", Change: ChangeModified},
		{ID: "readme", Kind: "doc", Change: ChangeModified},
		{ID: "settings", Kind: "config", Change: ChangeModified},
	}
}

func flowFixtureEdges() []*Edge {
	return []*Edge{
		{From: "page", To: "handler", Kind: "calls"},
		{From: "handler", To: "service", Kind: "calls"},
		{From: "service", To: "store", Kind: "calls"},
		{From: "handler", To: "view", Kind: "renders"},
		// A flow runs against `serves`: the endpoint reaches the handler.
		{From: "handler", To: "ep", Kind: "serves"},
		{From: "test", To: "store", Kind: "tests"},
	}
}

func nodesByID(nodes []*Node) map[string]*Node {
	byID := map[string]*Node{}

	for _, node := range nodes {
		byID[node.ID] = node
	}

	return byID
}

// codesAt lists each finding as "code path", in order.
func codesAt(findings []Finding) []string {
	described := []string{}

	for _, finding := range findings {
		described = append(described, finding.Code+" "+finding.Path)
	}

	return described
}

func TestCheckFlowConnected(t *testing.T) {
	cases := []struct {
		name string
		path []string
		want []string
	}{
		{"linear", []string{"page", "handler", "service", "store"}, []string{}},
		{"branching", []string{"page", "handler", "service", "store", "view"}, []string{}},
		{"reversed edge", []string{"ep", "handler", "view"}, []string{}},
		{"skips the unchanged node", []string{"page", "handler", "store"}, []string{"flow_gap /flows/0/path/2"}},
		{"tests edges do not join steps", []string{"test", "store"}, []string{"flow_gap /flows/0/path/1"}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			flows := []*Flow{{ID: "f", Path: testCase.path}}
			got := codesAt(checkFlowConnected(flows, flowFixtureEdges()))

			if !slices.Equal(got, testCase.want) {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCheckFlowSteps(t *testing.T) {
	cases := []struct {
		name string
		path []string
		want []string
	}{
		{"code only", []string{"page", "handler"}, []string{}},
		{"test step", []string{"handler", "test"}, []string{"not_flow_step /flows/0/path/1"}},
		{"doc and config steps", []string{"readme", "settings", "handler"}, []string{"not_flow_step /flows/0/path/0", "not_flow_step /flows/0/path/1"}},
		{"no code", []string{"ep"}, []string{"flow_without_code /flows/0"}},
		{"unknown ids left to checkFlowNodes", []string{"missing", "handler"}, []string{}},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			flows := []*Flow{{ID: "f", Path: testCase.path}}
			got := codesAt(checkFlowSteps(flows, nodesByID(flowFixtureNodes())))

			if !slices.Equal(got, testCase.want) {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}

func TestCheckFlowCoverage(t *testing.T) {
	cases := []struct {
		name  string
		flows []*Flow
		want  []string
	}{
		{
			name:  "every changed code node on a flow",
			flows: []*Flow{{ID: "f", Path: []string{"page", "handler", "service", "store", "view"}}},
			want:  []string{},
		},
		{
			name:  "a branch left out",
			flows: []*Flow{{ID: "f", Path: []string{"page", "handler", "service", "store"}}},
			want:  []string{"node_not_in_flow /nodes/4"},
		},
		{
			// Deleted code, unchanged code and off-flow kinds never need a flow.
			name:  "no flows",
			flows: nil,
			want:  []string{"node_not_in_flow /nodes/1", "node_not_in_flow /nodes/3", "node_not_in_flow /nodes/4"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := codesAt(checkFlowCoverage(flowFixtureNodes(), testCase.flows))

			if !slices.Equal(got, testCase.want) {
				t.Errorf("got %v, want %v", got, testCase.want)
			}
		})
	}
}
