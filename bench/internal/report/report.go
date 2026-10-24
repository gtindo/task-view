// Package report aggregates run results into cells (case × model × effort)
// and renders them as Markdown and CSV. It is pure: results in, text out.
package report

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/gtindo/task-view/bench/internal/config"
	"github.com/gtindo/task-view/bench/internal/metrics"
)

// Stat is a mean with its range over a cell's runs.
type Stat struct {
	Mean, Min, Max float64
}

func stat(values []float64) Stat {
	if len(values) == 0 {
		return Stat{}
	}

	total := 0.0
	for _, value := range values {
		total += value
	}

	return Stat{Mean: total / float64(len(values)), Min: slices.Min(values), Max: slices.Max(values)}
}

// Cell is one case × model × effort over its repeats.
type Cell struct {
	Case       string
	Project    string
	Size       string
	Model      string
	ModelLabel string
	Effort     string
	Runs       int

	// Accurate counts runs that end with an account matching the diff;
	// FirstPass those whose first validate already did.
	Accurate  int
	FirstPass int
	Failed    int // harness errors and timeouts

	ChangeLines int
	Nodes       Stat
	Edges       Stat
	Validates   Stat
	Seconds     Stat
	Tokens      Stat
	Output      Stat // output tokens alone, the part the model wrote
	Cost        Stat
	CostSource  string

	// NodeAgreement and EdgeAgreement are the mean pairwise Jaccard
	// similarity of the runs' changed-node ranges and edges: 1 when every
	// repeat drew the same map.
	NodeAgreement float64
	EdgeAgreement float64
}

// CostPer100Lines is the mean cost per hundred changed lines.
func (c Cell) CostPer100Lines() float64 {
	if c.ChangeLines == 0 {
		return 0
	}

	const hundred = 100

	return c.Cost.Mean * hundred / float64(c.ChangeLines)
}

// Aggregate groups results into cells, ordered by size, case, model and
// effort.
func Aggregate(results []metrics.Result) []Cell {
	groups := map[string][]metrics.Result{}
	var order []string
	for _, result := range results {
		key := strings.Join([]string{result.Case, result.Model, result.Effort}, "\x00")
		if _, seen := groups[key]; !seen {
			order = append(order, key)
		}

		groups[key] = append(groups[key], result)
	}

	cells := make([]Cell, 0, len(order))
	for _, key := range order {
		cells = append(cells, cellOf(groups[key]))
	}

	sort.SliceStable(cells, func(i, j int) bool {
		left, right := cells[i], cells[j]
		if left.Size != right.Size {
			return slices.Index(config.Sizes, left.Size) < slices.Index(config.Sizes, right.Size)
		}

		if left.Case != right.Case {
			return left.Case < right.Case
		}

		if left.Model != right.Model {
			return left.Model < right.Model
		}

		return left.Effort < right.Effort
	})

	return cells
}

func cellOf(runs []metrics.Result) Cell {
	first := runs[0]
	cell := Cell{
		Case:       first.Case,
		Project:    first.Project,
		Size:       first.Size,
		Model:      first.Model,
		ModelLabel: first.ModelLabel,
		Effort:     first.Effort,
		Runs:       len(runs),
	}

	var nodes, edges, validates, seconds, tokens, outputs, costs []float64
	var nodeSets, edgeSets [][]string
	sources := map[string]bool{}
	for _, run := range runs {
		cell.ChangeLines = max(cell.ChangeLines, run.Change.Lines())
		if run.HarnessError != "" || run.TimedOut {
			cell.Failed++
		}

		if run.Final.Accurate() && run.Review.Exists && !run.WorkTouched {
			cell.Accurate++
		}

		firstPassed := len(run.Calls.ValidateExits) > 0 && run.Calls.ValidateExits[0] == 0
		if firstPassed {
			cell.FirstPass++
		}

		nodes = append(nodes, float64(run.Review.Nodes))
		edges = append(edges, float64(run.Review.Edges))
		validates = append(validates, float64(run.Calls.Validate))
		seconds = append(seconds, run.Seconds)
		tokens = append(tokens, float64(run.Usage.TotalTokens()))
		outputs = append(outputs, float64(run.Usage.OutputTokens))
		costs = append(costs, run.CostUSD)
		sources[run.CostSource] = true
		nodeSets = append(nodeSets, run.Review.NodeKeys)
		edgeSets = append(edgeSets, run.Review.EdgeKeys)
	}

	cell.Nodes = stat(nodes)
	cell.Edges = stat(edges)
	cell.Validates = stat(validates)
	cell.Seconds = stat(seconds)
	cell.Tokens = stat(tokens)
	cell.Output = stat(outputs)
	cell.Cost = stat(costs)
	cell.CostSource = costSource(sources)
	cell.NodeAgreement = MeanPairwiseJaccard(nodeSets)
	cell.EdgeAgreement = MeanPairwiseJaccard(edgeSets)

	return cell
}

// costSource names where a cell's costs came from: "harness", "prices",
// "mixed" or "unknown".
func costSource(sources map[string]bool) string {
	delete(sources, "")
	switch {
	case len(sources) == 0:
		return "unknown"
	case len(sources) > 1:
		return "mixed"
	}

	for source := range sources {
		return source
	}

	return "unknown"
}

// Jaccard is the size of the intersection over the size of the union of
// two sets of keys; two empty sets are identical.
func Jaccard(left, right []string) float64 {
	leftSet, rightSet := setOf(left), setOf(right)
	union, shared := len(leftSet), 0
	for key := range rightSet {
		if leftSet[key] {
			shared++

			continue
		}

		union++
	}

	if union == 0 {
		return 1
	}

	return float64(shared) / float64(union)
}

func setOf(keys []string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, key := range keys {
		set[key] = true
	}

	return set
}

// MeanPairwiseJaccard averages Jaccard over every pair of sets; a single
// set has nothing to disagree with and scores 1.
func MeanPairwiseJaccard(sets [][]string) float64 {
	total, pairs := 0.0, 0
	for i := range sets {
		for j := i + 1; j < len(sets); j++ {
			total += Jaccard(sets[i], sets[j])
			pairs++
		}
	}

	if pairs == 0 {
		return 1
	}

	return total / float64(pairs)
}

// Meta describes a results folder, written by the runner as meta.json.
// PriceList is prices.json as it was when the run started, so a published
// result keeps the prices it was costed with after the file moves on.
type Meta struct {
	TaskView       string         `json:"task_view"`
	Harness        string         `json:"harness"`
	HarnessVersion string         `json:"harness_version,omitempty"`
	Matrix         config.Matrix  `json:"matrix"`
	Prices         string         `json:"prices_as_of"`
	PriceList      config.Prices  `json:"price_list"`
	OS             string         `json:"os"`
	Started        string         `json:"started"`
	Extra          map[string]any `json:"extra,omitempty"`
}

// Markdown renders the report: what was run, then one table of every cell
// and one per model over all cases.
func Markdown(meta Meta, cells []Cell) string {
	var out strings.Builder
	fmt.Fprintf(&out, "# task-view benchmark\n\n")
	fmt.Fprintf(&out, "- task-view: `%s`\n- harness: `%s`\n- started: %s on %s\n- prices as of: %s (used only where the harness reported no cost)\n\n",
		meta.TaskView, meta.Harness, meta.Started, meta.OS, meta.Prices)
	out.WriteString("Accurate = the final review validates with exit 0 and no unmentioned hunk. ")
	out.WriteString("Agreement = mean pairwise overlap (Jaccard) of changed-node ranges and of edges across repeats; 1 means every repeat drew the same map.\n\n")

	out.WriteString("## Every cell\n\n")
	writeCellTable(&out, cells)

	out.WriteString("\n## By model and effort, all cases\n\n")
	writeModelTable(&out, summarize(cells, nil))

	return out.String()
}

func writeCellTable(out *strings.Builder, cells []Cell) {
	out.WriteString("| Case | Size | Model | Effort | Runs | Accurate | First validate passed | Validate runs | Nodes | Edges | Minutes | Tokens (M) | Cost (US$) | US$ per 100 lines | Node agreement | Edge agreement |\n")
	out.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, cell := range cells {
		fmt.Fprintf(out, "| %s | %s | %s | %s | %d | %d/%d | %d/%d | %s | %s | %s | %s | %s | %s | %.3f | %.2f | %.2f |\n",
			cell.Case, cell.Size, cell.ModelLabel, cell.Effort, cell.Runs,
			cell.Accurate, cell.Runs, cell.FirstPass, cell.Runs,
			rangeText(cell.Validates, 1, 1), rangeText(cell.Nodes, 1, 0), rangeText(cell.Edges, 1, 0), rangeText(cell.Seconds, 1.0/60, 1),
			rangeText(cell.Tokens, 1e-6, 2), costText(cell), cell.CostPer100Lines(),
			cell.NodeAgreement, cell.EdgeAgreement)
	}
}

func writeModelTable(out *strings.Builder, rows []summaryRow) {
	out.WriteString("| Model | Effort | Runs | Accurate | Mean cost per run (US$) | Mean tokens per run (M) | Mean minutes per run | Mean node agreement |\n")
	out.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range rows {
		fmt.Fprintf(out, "| %s | %s | %d | %d/%d | %.3f | %.2f | %.1f | %.2f |\n",
			row.label, row.effort, row.runs, row.accurate, row.runs, row.cost, row.tokens*1e-6, row.minutes, row.agreement)
	}
}

func rangeText(value Stat, scale float64, decimals int) string {
	format := "%." + strconv.Itoa(decimals) + "f"
	mean := fmt.Sprintf(format, value.Mean*scale)
	if value.Min == value.Max {
		return mean
	}

	return fmt.Sprintf("%s ("+format+"–"+format+")", mean, value.Min*scale, value.Max*scale)
}

func costText(cell Cell) string {
	text := rangeText(cell.Cost, 1, 3)
	if cell.CostSource == "prices" {
		return text + " (computed)"
	}

	return text
}

// summaryRow sums one model and effort, over every case or over the cases
// of one group (a size, a project). Cost, minutes and tokens are
// run-weighted means, the cost range spans every run, agreement is a mean
// over cells, and costPer100Lines is the summed cost over the summed
// changed lines.
type summaryRow struct {
	group, model, label, effort string
	runs, accurate, firstPass   int
	cells                       int
	cost, minutes               float64
	costMin, costMax            float64
	costPer100Lines             float64
	nodes, edges                float64
	tokens, output              float64
	agreement                   float64
}

// summarize groups cells by model and effort, first by groupOf's key (a
// size, a project) when it is given, keeping the cells' order.
func summarize(cells []Cell, groupOf func(Cell) string) []summaryRow {
	const hundred = 100

	var rows []summaryRow
	lines := map[int]float64{}
	index := map[string]int{}
	for _, cell := range cells {
		group := ""
		if groupOf != nil {
			group = groupOf(cell)
		}

		key := group + "\x00" + cell.Model + "\x00" + cell.Effort

		position, ok := index[key]
		if !ok {
			position = len(rows)
			index[key] = position
			rows = append(rows, summaryRow{
				group:   group,
				model:   cell.Model,
				label:   cell.ModelLabel,
				effort:  cell.Effort,
				costMin: cell.Cost.Min,
				costMax: cell.Cost.Max,
			})
		}

		row := &rows[position]
		row.runs += cell.Runs
		row.accurate += cell.Accurate
		row.firstPass += cell.FirstPass
		row.cost += cell.Cost.Mean * float64(cell.Runs)
		row.minutes += cell.Seconds.Mean / 60 * float64(cell.Runs)
		row.nodes += cell.Nodes.Mean * float64(cell.Runs)
		row.edges += cell.Edges.Mean * float64(cell.Runs)
		row.tokens += cell.Tokens.Mean * float64(cell.Runs)
		row.output += cell.Output.Mean * float64(cell.Runs)
		row.costMin = min(row.costMin, cell.Cost.Min)
		row.costMax = max(row.costMax, cell.Cost.Max)
		row.agreement += cell.NodeAgreement
		row.cells++
		lines[position] += float64(cell.ChangeLines * cell.Runs)
	}

	for i := range rows {
		if lines[i] > 0 {
			rows[i].costPer100Lines = rows[i].cost * hundred / lines[i]
		}

		rows[i].cost /= float64(rows[i].runs)
		rows[i].minutes /= float64(rows[i].runs)
		rows[i].nodes /= float64(rows[i].runs)
		rows[i].edges /= float64(rows[i].runs)
		rows[i].tokens /= float64(rows[i].runs)
		rows[i].output /= float64(rows[i].runs)
		rows[i].agreement /= float64(rows[i].cells)
	}

	return rows
}

// CSV renders one row per cell, for spreadsheets and plots.
func CSV(cells []Cell) []byte {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	_ = writer.Write([]string{"case", "project", "size", "model", "effort", "runs", "accurate", "first_pass", "failed",
		"change_lines", "nodes_mean", "edges_mean", "validate_mean", "seconds_mean", "tokens_mean", "output_tokens_mean", "cost_mean_usd", "cost_min_usd", "cost_max_usd",
		"cost_source", "usd_per_100_lines", "node_agreement", "edge_agreement"})
	for _, cell := range cells {
		_ = writer.Write([]string{cell.Case, cell.Project, cell.Size, cell.Model, cell.Effort,
			strconv.Itoa(cell.Runs), strconv.Itoa(cell.Accurate), strconv.Itoa(cell.FirstPass), strconv.Itoa(cell.Failed),
			strconv.Itoa(cell.ChangeLines), float(cell.Nodes.Mean), float(cell.Edges.Mean), float(cell.Validates.Mean), float(cell.Seconds.Mean), float(cell.Tokens.Mean), float(cell.Output.Mean),
			float(cell.Cost.Mean), float(cell.Cost.Min), float(cell.Cost.Max), cell.CostSource,
			float(cell.CostPer100Lines()), float(cell.NodeAgreement), float(cell.EdgeAgreement)})
	}

	writer.Flush()

	return buffer.Bytes()
}

func float(value float64) string { return strconv.FormatFloat(value, 'f', 4, 64) }
