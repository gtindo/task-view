// Package config loads the benchmark's inputs: the suite of cases, the run
// matrix, a harness folder and the price list, and expands them into runs.
// Everything here is pure apart from reading the files.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
)

// Sizes are the change classes a case may claim, smallest first.
// agent-scale is beyond what a person writes by hand in one sitting:
// thousands of lines produced with agents in a short time.
var Sizes = []string{"small", "medium", "big", "agent-scale"}

var fullSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Project is one open-source repository the suite replays tasks from.
type Project struct {
	ID       string `json:"id"`
	Repo     string `json:"repo"`
	Language string `json:"language"`
	License  string `json:"license"`
}

// Case is one task: a run of consecutive first-parent commits after Base,
// newest first, replayed as the state an agent leaves when it finishes.
type Case struct {
	ID             string   `json:"id"`
	Project        string   `json:"project"`
	Size           string   `json:"size"`
	Title          string   `json:"title"`
	Source         string   `json:"source"`
	Base           string   `json:"base"`
	Commits        []string `json:"commits"`
	UncommittedTip bool     `json:"uncommitted_tip"`
}

// Tip is the newest commit of the task.
func (c Case) Tip() string { return c.Commits[0] }

// Suite is the cases.json file.
type Suite struct {
	Projects []Project `json:"projects"`
	Cases    []Case    `json:"cases"`
}

// Project returns the project a case belongs to.
func (s Suite) Project(id string) (Project, bool) {
	index := slices.IndexFunc(s.Projects, func(project Project) bool { return project.ID == id })
	if index < 0 {
		return Project{}, false
	}

	return s.Projects[index], true
}

// Model is one model of the matrix: the id the harness gets, a label for
// reports and the key of its entry in prices.json.
type Model struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Price string `json:"price"`
}

// Matrix is matrix.json: which harness, models, efforts and cases to run.
type Matrix struct {
	Harness  string   `json:"harness"`
	Models   []Model  `json:"models"`
	Efforts  []string `json:"efforts"`
	Repeats  int      `json:"repeats"`
	Parallel int      `json:"parallel"`
	Cases    []string `json:"cases"`
	Timeout  string   `json:"timeout"`
}

// TimeoutDuration is the per-run time limit.
func (m Matrix) TimeoutDuration() (time.Duration, error) {
	return time.ParseDuration(m.Timeout)
}

// Overrides are the command-line replacements for matrix fields; zero
// values leave a field as it is.
type Overrides struct {
	Harness  string
	Models   []string
	Efforts  []string
	Cases    []string
	Repeats  int
	Parallel int
	Timeout  string
}

// Apply returns the matrix with the overrides applied. A model id the
// matrix does not list is kept with the id as its label and price key, so a
// new model needs no file edit.
func (m Matrix) Apply(overrides Overrides) Matrix {
	result := m
	if overrides.Harness != "" {
		result.Harness = overrides.Harness
	}

	if len(overrides.Models) > 0 {
		result.Models = nil
		for _, id := range overrides.Models {
			index := slices.IndexFunc(m.Models, func(model Model) bool { return model.ID == id })
			if index >= 0 {
				result.Models = append(result.Models, m.Models[index])

				continue
			}

			result.Models = append(result.Models, Model{ID: id, Label: id, Price: id})
		}
	}

	if len(overrides.Efforts) > 0 {
		result.Efforts = overrides.Efforts
	}

	if len(overrides.Cases) > 0 {
		result.Cases = overrides.Cases
	}

	if overrides.Repeats > 0 {
		result.Repeats = overrides.Repeats
	}

	if overrides.Parallel > 0 {
		result.Parallel = overrides.Parallel
	}

	if overrides.Timeout != "" {
		result.Timeout = overrides.Timeout
	}

	return result
}

// Harness is a harness folder's harness.json, with the folder it lives in.
// VersionCommand, when set, is a shell command printing the harness's
// version, recorded with the results since the harness shapes them as much
// as the model does.
type Harness struct {
	Name           string `json:"name"`
	Command        string `json:"command"`
	UsageFormat    string `json:"usage_format"`
	UsageFile      string `json:"usage_file"`
	VersionCommand string `json:"version_command"`
	Dir            string `json:"-"`
}

// CommandPath is the adapter's absolute path.
func (h Harness) CommandPath() string { return filepath.Join(h.Dir, h.Command) }

// Price is one model's list price in USD per million tokens.
type Price struct {
	Input      float64 `json:"input"`
	Output     float64 `json:"output"`
	CacheRead  float64 `json:"cache_read"`
	CacheWrite float64 `json:"cache_write"`
}

// Prices is prices.json.
type Prices struct {
	AsOf   string           `json:"as_of"`
	Source string           `json:"source"`
	Models map[string]Price `json:"models"`
}

// Run is one agent session of the matrix.
type Run struct {
	ID     string
	Case   Case
	Model  Model
	Effort string
	Repeat int
}

// Expand lists every run of the matrix over the suite, in a stable order:
// case, model, effort, repeat. Case patterns are path globs on case ids.
func Expand(suite Suite, matrix Matrix) []Run {
	var runs []Run
	for _, task := range suite.Cases {
		if !matchesAny(matrix.Cases, task.ID) {
			continue
		}

		for _, model := range matrix.Models {
			for _, effort := range matrix.Efforts {
				for repeat := 1; repeat <= matrix.Repeats; repeat++ {
					runs = append(runs, Run{
						ID:     fmt.Sprintf("%s__%s__%s__%d", task.ID, model.ID, effort, repeat),
						Case:   task,
						Model:  model,
						Effort: effort,
						Repeat: repeat,
					})
				}
			}
		}
	}

	return runs
}

func matchesAny(patterns []string, id string) bool {
	return slices.ContainsFunc(patterns, func(pattern string) bool {
		matched, err := path.Match(pattern, id)

		return err == nil && matched
	})
}

// PromptInput is what a prompt template is filled with.
type PromptInput struct {
	Brief       string
	TaskView    string
	ReviewFile  string
	Base        string
	Commits     int
	Uncommitted bool
}

// RenderPrompt fills the {{placeholders}} of a prompt template. The
// {{state}} sentence says where the task's work sits, since the agent did
// not see the replay set it up.
func RenderPrompt(template string, input PromptInput) string {
	committed := input.Commits
	if input.Uncommitted {
		committed--
	}

	state := "All of the work is committed on the current branch."
	switch {
	case committed == 0:
		state = "None of the work is committed yet: it is all uncommitted in the working tree."
	case input.Uncommitted:
		state = fmt.Sprintf("You made %d %s on the current branch, and the last part of the work is still uncommitted.",
			committed, plural(committed, "commit", "commits"))
	}

	replacer := strings.NewReplacer(
		"{{brief}}", strings.TrimSpace(input.Brief),
		"{{task_view}}", input.TaskView,
		"{{review_file}}", input.ReviewFile,
		"{{base}}", input.Base,
		"{{state}}", state,
	)

	return replacer.Replace(template)
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}

	return many
}

// LoadSuite reads and checks cases.json.
func LoadSuite(file string) (Suite, error) {
	var suite Suite
	if err := readJSON(file, &suite); err != nil {
		return Suite{}, err
	}

	return suite, suite.check()
}

func (s Suite) check() error {
	var problems []string
	seen := map[string]bool{}
	for _, task := range s.Cases {
		if seen[task.ID] {
			problems = append(problems, fmt.Sprintf("case %q is listed twice", task.ID))
		}

		seen[task.ID] = true
		if _, ok := s.Project(task.Project); !ok {
			problems = append(problems, fmt.Sprintf("case %q: unknown project %q", task.ID, task.Project))
		}

		if !slices.Contains(Sizes, task.Size) {
			problems = append(problems, fmt.Sprintf("case %q: size %q is not one of %v", task.ID, task.Size, Sizes))
		}

		if len(task.Commits) == 0 {
			problems = append(problems, fmt.Sprintf("case %q lists no commits", task.ID))
		}

		for _, sha := range append([]string{task.Base}, task.Commits...) {
			if !fullSHA.MatchString(sha) {
				problems = append(problems, fmt.Sprintf("case %q: %q is not a full SHA", task.ID, sha))
			}
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("cases: %s", strings.Join(problems, "; "))
	}

	return nil
}

// LoadMatrix reads matrix.json.
func LoadMatrix(file string) (Matrix, error) {
	var matrix Matrix
	err := readJSON(file, &matrix)

	return matrix, err
}

// LoadHarness reads a harness folder's harness.json.
func LoadHarness(dir string) (Harness, error) {
	var harness Harness
	if err := readJSON(filepath.Join(dir, "harness.json"), &harness); err != nil {
		return Harness{}, err
	}

	harness.Dir = dir
	knownFormat := harness.UsageFormat == "usage-json" || harness.UsageFormat == "claude-code"
	if harness.Command == "" || !knownFormat {
		return Harness{}, fmt.Errorf("%s: harness.json needs a command and a usage_format of usage-json or claude-code", dir)
	}

	return harness, nil
}

// LoadPrices reads prices.json.
func LoadPrices(file string) (Prices, error) {
	var prices Prices
	err := readJSON(file, &prices)

	return prices, err
}

func readJSON(file string, into any) error {
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(data, into); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}

	return nil
}
