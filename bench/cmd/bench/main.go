// Command bench runs the task-view benchmark: it replays fixed open-source
// tasks, hands each one to an agent through a harness adapter, and measures
// what writing the review cost. See ../../README.md.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gtindo/task-view/bench/internal/config"
	"github.com/gtindo/task-view/bench/internal/metrics"
	"github.com/gtindo/task-view/bench/internal/replay"
	"github.com/gtindo/task-view/bench/internal/report"
)

const usage = `bench — measure what a task-view review costs, by change size and model

Usage (from the repo root or bench/):
  bench prepare [-root dir] [-task-view path]
  bench run     [-root dir] [-matrix file] [-out dir] [-task-view path] [-keep]
                [-harness dir] [-models a,b] [-efforts a,b] [-cases glob,glob]
                [-repeats n] [-parallel n] [-timeout 20m]
  bench report  <results dir>
  bench publish [-root dir] [-dashboard url] <results dir>

prepare clones the projects into .cache/repos, checks every case and builds
task-view into .cache/bin. run expands the matrix and runs every cell, skipping
runs already in <out>/runs.jsonl that did not fail. report writes report.md and
summary.csv. publish replaces results/ with the run's results and an archive of
its reviews and pages, and writes BENCH.md at the repo root.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "prepare":
		err = cmdPrepare(os.Args[2:])
	case "run":
		err = cmdRun(os.Args[2:])
	case "report":
		err = cmdReport(os.Args[2:])
	case "publish":
		err = cmdPublish(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "bench:", err)
		os.Exit(1)
	}
}

// layout is where the bench inputs and its cache live.
type layout struct {
	root     string // the bench folder: cases.json, matrix.json, ...
	repoRoot string // the task-view checkout holding it
}

func (l layout) path(parts ...string) string {
	return filepath.Join(append([]string{l.root}, parts...)...)
}

func (l layout) cache(parts ...string) string {
	return l.path(append([]string{".cache"}, parts...)...)
}

// findLayout resolves the bench folder: the flag when given, else the
// current folder or its bench/ child, whichever holds cases.json.
func findLayout(flagRoot string) (layout, error) {
	candidates := []string{flagRoot}
	if flagRoot == "" {
		candidates = []string{".", "bench"}
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(filepath.Join(candidate, "cases.json")); err != nil {
			continue
		}

		root, err := filepath.Abs(candidate)
		if err != nil {
			return layout{}, err
		}

		return layout{root: root, repoRoot: filepath.Dir(root)}, nil
	}

	return layout{}, errors.New("no cases.json here or in bench/; pass -root")
}

func cmdPrepare(args []string) error {
	flags := flag.NewFlagSet("prepare", flag.ContinueOnError)
	rootFlag := flags.String("root", "", "the bench folder (default: . or bench/)")
	taskViewFlag := flags.String("task-view", "", "use this task-view binary instead of building one")
	if err := flags.Parse(args); err != nil {
		return err
	}

	paths, err := findLayout(*rootFlag)
	if err != nil {
		return err
	}

	suite, err := config.LoadSuite(paths.path("cases.json"))
	if err != nil {
		return err
	}

	problems := 0
	for _, task := range suite.Cases {
		project, _ := suite.Project(task.Project)
		repo, err := replay.EnsureClone(paths.cache("repos"), project, append([]string{task.Base}, task.Commits...))
		if err != nil {
			return err
		}

		check := replay.Check(repo, task)
		fmt.Printf("%-14s %-11s %3d files %5d lines\n", task.ID, task.Size, check.Files, check.Lines)
		for _, warning := range check.Warnings {
			fmt.Printf("  warning: %s\n", warning)
		}

		for _, problem := range check.Problems {
			fmt.Printf("  problem: %s\n", problem)
			problems++
		}

		if _, err := os.Stat(paths.path("briefs", task.ID+".md")); err != nil {
			fmt.Printf("  problem: no brief at briefs/%s.md\n", task.ID)
			problems++
		}
	}

	binary, version, err := taskViewBinary(paths, *taskViewFlag)
	if err != nil {
		return err
	}

	fmt.Printf("task-view %s at %s\n", version, binary)
	if problems > 0 {
		return fmt.Errorf("%d %s; fix cases.json or the briefs", problems, plural(problems, "problem", "problems"))
	}

	return nil
}

// taskViewBinary is the binary every run uses: the flag's, else one built
// from this checkout into the cache. It returns the path and the version.
func taskViewBinary(paths layout, flagPath string) (string, string, error) {
	if flagPath != "" {
		binary, err := filepath.Abs(flagPath)
		if err != nil {
			return "", "", err
		}

		output, err := exec.Command(binary, "version").Output()
		if err != nil {
			return "", "", fmt.Errorf("%s version: %w", binary, err)
		}

		return binary, firstLine(output), nil
	}

	binary := paths.cache("bin", "task-view")
	version, err := replay.BuildTaskView(paths.repoRoot, binary)

	return binary, "task-view " + version, err
}

// runEnv is everything one run needs besides its own settings.
type runEnv struct {
	paths    layout
	suite    config.Suite
	harness  config.Harness
	prices   config.Prices
	prompt   string
	taskView string
	out      string
	timeout  time.Duration
	keep     bool
	repos    map[string]string // project id → clone
}

func cmdRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	rootFlag := flags.String("root", "", "the bench folder (default: . or bench/)")
	matrixFlag := flags.String("matrix", "", "matrix file (default: <root>/matrix.json)")
	outFlag := flags.String("out", "", "results folder (default: <root>/.cache/results/latest)")
	taskViewFlag := flags.String("task-view", "", "use this task-view binary instead of building one")
	keep := flags.Bool("keep", false, "keep each run's work dir")
	var overrides config.Overrides
	flags.StringVar(&overrides.Harness, "harness", "", "harness folder, relative to the bench folder")
	models := flags.String("models", "", "comma-separated model ids")
	efforts := flags.String("efforts", "", "comma-separated efforts")
	cases := flags.String("cases", "", "comma-separated case id globs")
	flags.IntVar(&overrides.Repeats, "repeats", 0, "repeats per cell")
	flags.IntVar(&overrides.Parallel, "parallel", 0, "runs at the same time")
	flags.StringVar(&overrides.Timeout, "timeout", "", "time limit per run, e.g. 20m")
	if err := flags.Parse(args); err != nil {
		return err
	}

	overrides.Models = splitList(*models)
	overrides.Efforts = splitList(*efforts)
	overrides.Cases = splitList(*cases)

	paths, err := findLayout(*rootFlag)
	if err != nil {
		return err
	}

	env, matrix, err := loadRun(paths, *matrixFlag, *outFlag, overrides)
	if err != nil {
		return err
	}

	env.keep = *keep
	env.taskView, err = runBinary(paths, *taskViewFlag)
	if err != nil {
		return err
	}

	runs := config.Expand(env.suite, matrix)
	done, err := doneRuns(filepath.Join(env.out, "runs.jsonl"))
	if err != nil {
		return err
	}

	if err := writeMeta(env, matrix); err != nil {
		return err
	}

	pending := runs[:0:0]
	for _, run := range runs {
		if !done[run.ID] {
			pending = append(pending, run)
		}
	}

	fmt.Printf("%d runs, %d already done, %d to go, %d at a time; results in %s\n",
		len(runs), len(runs)-len(pending), len(pending), matrix.Parallel, env.out)

	return runAll(env, pending, max(matrix.Parallel, 1))
}

func loadRun(paths layout, matrixFile, out string, overrides config.Overrides) (*runEnv, config.Matrix, error) {
	suite, err := config.LoadSuite(paths.path("cases.json"))
	if err != nil {
		return nil, config.Matrix{}, err
	}

	if matrixFile == "" {
		matrixFile = paths.path("matrix.json")
	}

	base, err := config.LoadMatrix(matrixFile)
	if err != nil {
		return nil, config.Matrix{}, err
	}

	matrix := base.Apply(overrides)
	timeout, err := matrix.TimeoutDuration()
	if err != nil {
		return nil, config.Matrix{}, fmt.Errorf("timeout: %w", err)
	}

	harnessDir := matrix.Harness
	if !filepath.IsAbs(harnessDir) {
		harnessDir = paths.path(harnessDir)
	}

	harness, err := config.LoadHarness(harnessDir)
	if err != nil {
		return nil, config.Matrix{}, err
	}

	prices, err := config.LoadPrices(paths.path("prices.json"))
	if err != nil {
		return nil, config.Matrix{}, err
	}

	prompt, err := os.ReadFile(paths.path("prompt.md"))
	if err != nil {
		return nil, config.Matrix{}, err
	}

	if out == "" {
		out = paths.cache("results", "latest")
	}

	out, err = filepath.Abs(out)
	if err != nil {
		return nil, config.Matrix{}, err
	}

	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, config.Matrix{}, err
	}

	env := &runEnv{
		paths:   paths,
		suite:   suite,
		harness: harness,
		prices:  prices,
		prompt:  string(prompt),
		out:     out,
		timeout: timeout,
		repos:   map[string]string{},
	}

	for _, project := range suite.Projects {
		var shas []string
		for _, task := range suite.Cases {
			if task.Project == project.ID {
				shas = append(shas, append([]string{task.Base}, task.Commits...)...)
			}
		}

		repo, err := replay.EnsureClone(paths.cache("repos"), project, shas)
		if err != nil {
			return nil, config.Matrix{}, err
		}

		env.repos[project.ID] = repo
	}

	return env, matrix, nil
}

// runBinary is prepare's binary when it exists, so a run never rebuilds
// task-view behind the results' back; else the flag's, else a fresh build.
func runBinary(paths layout, flagPath string) (string, error) {
	cached := paths.cache("bin", "task-view")
	if flagPath == "" {
		if _, err := os.Stat(cached); err == nil {
			return cached, nil
		}
	}

	binary, _, err := taskViewBinary(paths, flagPath)

	return binary, err
}

// writeMeta records what the results were produced with. A resumed run
// keeps the first start time, so the results date from when they began.
func writeMeta(env *runEnv, matrix config.Matrix) error {
	version, err := exec.Command(env.taskView, "version").Output()
	if err != nil {
		return err
	}

	metaFile := filepath.Join(env.out, "meta.json")
	started := time.Now().UTC().Format(time.RFC3339)
	var previous report.Meta
	if data, err := os.ReadFile(metaFile); err == nil && json.Unmarshal(data, &previous) == nil && previous.Started != "" {
		started = previous.Started
	}

	meta := report.Meta{
		TaskView:       firstLine(version),
		Harness:        env.harness.Name,
		HarnessVersion: harnessVersion(env.harness),
		Matrix:         matrix,
		Prices:         env.prices.AsOf,
		PriceList:      env.prices,
		OS:             runtime.GOOS + "/" + runtime.GOARCH,
		Started:        started,
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(metaFile, data, 0o644)
}

// harnessVersion runs the harness's version command; a harness without one,
// or one that fails, records no version rather than stopping the run.
func harnessVersion(harness config.Harness) string {
	if harness.VersionCommand == "" {
		return ""
	}

	cmd := exec.Command("sh", "-c", harness.VersionCommand)
	cmd.Dir = harness.Dir
	output, err := cmd.Output()
	if err != nil {
		return ""
	}

	return firstLine(output)
}

func firstLine(output []byte) string {
	return strings.TrimSpace(strings.SplitN(string(output), "\n", 2)[0])
}

// doneRuns lists the run ids already recorded, so a run can be resumed. A
// run whose latest result is a harness failure (a usage limit, a crashed
// adapter) is not done: resuming retries it, and the report keeps only the
// newest result of each run.
func doneRuns(file string) (map[string]bool, error) {
	done := map[string]bool{}
	results, err := readResults(file)
	if errors.Is(err, os.ErrNotExist) {
		return done, nil
	}

	for _, result := range report.Latest(results) {
		done[result.RunID] = result.HarnessError == ""
	}

	return done, err
}

func runAll(env *runEnv, runs []config.Run, parallel int) error {
	var lock sync.Mutex
	var wg sync.WaitGroup
	slots := make(chan struct{}, parallel)
	results, err := os.OpenFile(filepath.Join(env.out, "runs.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer results.Close()

	for index, run := range runs {
		wg.Add(1)
		slots <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-slots }()

			result := runOne(env, run)
			line, _ := json.Marshal(result)

			lock.Lock()
			defer lock.Unlock()

			_, _ = results.Write(append(line, '\n'))
			fmt.Printf("[%d/%d] %s: %s\n", index+1, len(runs), run.ID, resultLine(result))
		}()
	}

	wg.Wait()

	return nil
}

func resultLine(result metrics.Result) string {
	if result.HarnessError != "" {
		return "harness error: " + result.HarnessError
	}

	return fmt.Sprintf("exit %d, %d validate runs, %.0fs, US$%.3f", result.Final.Exit, result.Calls.Validate, result.Seconds, result.CostUSD)
}

// runOne replays one run end to end and never fails: whatever goes wrong
// is recorded in the result, so one broken run does not stop the matrix.
func runOne(env *runEnv, run config.Run) metrics.Result {
	project, _ := env.suite.Project(run.Case.Project)
	result := metrics.Result{
		RunID:      run.ID,
		Case:       run.Case.ID,
		Project:    project.ID,
		Size:       run.Case.Size,
		Model:      run.Model.ID,
		ModelLabel: run.Model.Label,
		Effort:     run.Effort,
		Repeat:     run.Repeat,
		Harness:    env.harness.Name,
		StartedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	runDir := filepath.Join(env.out, "runs", run.ID)
	workDir := env.paths.cache("work", run.ID)
	reviewRel := filepath.Join(".review", "change-"+run.Case.ID+".json")
	if err := os.RemoveAll(runDir); err != nil {
		return failed(result, err)
	}

	for _, dir := range []string{filepath.Join(runDir, "bin"), filepath.Join(runDir, "out")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return failed(result, err)
		}
	}

	if err := replay.Workdir(env.repos[project.ID], workDir, run.Case); err != nil {
		return failed(result, err)
	}

	if !env.keep {
		defer os.RemoveAll(workDir)
	}

	// The change's size, read by gather before the agent starts, so every
	// run of a case reports the same size. The base makes it the whole task:
	// without it gather sees only the uncommitted tip.
	gather := exec.Command(env.taskView, "gather", workDir, "--base", run.Case.Base, "--format", "json", "--review", filepath.Join(workDir, reviewRel))
	gather.Dir = workDir
	gathered, _ := gather.Output()
	result.Change, _ = metrics.ParseGather(gathered)

	before, err := replay.WorkState(workDir)
	if err != nil {
		return failed(result, err)
	}

	promptFile := filepath.Join(runDir, "prompt.md")
	wrapper := filepath.Join(runDir, "bin", "task-view")
	if err := writePrompt(env, run.Case, reviewRel, wrapper, promptFile); err != nil {
		return failed(result, err)
	}

	callLog := filepath.Join(runDir, "task-view-calls.tsv")
	if err := writeWrapper(wrapper, env.taskView, callLog); err != nil {
		return failed(result, err)
	}

	started := time.Now()
	result.TimedOut, err = runAdapter(env, run, workDir, runDir, promptFile)
	result.Seconds = time.Since(started).Seconds()
	if err != nil {
		result.HarnessError = err.Error()
	}

	collect(env, run, &result, workDir, runDir, reviewRel, callLog)
	after, err := replay.WorkState(workDir)
	result.WorkTouched = err != nil || after != before

	return result
}

func failed(result metrics.Result, err error) metrics.Result {
	result.HarnessError = "setup: " + err.Error()

	return result
}

func writePrompt(env *runEnv, task config.Case, reviewRel, wrapper, file string) error {
	brief, err := os.ReadFile(env.paths.path("briefs", task.ID+".md"))
	if err != nil {
		return err
	}

	text := config.RenderPrompt(env.prompt, config.PromptInput{
		Brief:       string(brief),
		TaskView:    wrapper,
		ReviewFile:  filepath.ToSlash(reviewRel),
		Base:        task.Base,
		Commits:     len(task.Commits),
		Uncommitted: task.UncommittedTip,
	})

	return os.WriteFile(file, []byte(text), 0o644)
}

// writeWrapper writes the `task-view` the agent finds on its PATH: it runs
// the pinned binary and logs each call's start, exit code and arguments.
func writeWrapper(file, binary, callLog string) error {
	script := fmt.Sprintf(`#!/bin/sh
start=$(date +%%s)
%q "$@"
code=$?
printf '%%s\t%%s\t%%s\n' "$start" "$code" "$*" >> %q
exit $code
`, binary, callLog)

	return os.WriteFile(file, []byte(script), 0o755)
}

// runAdapter runs the harness adapter with the contract's environment and
// reports whether it hit the time limit.
func runAdapter(env *runEnv, run config.Run, workDir, runDir, promptFile string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), env.timeout)
	defer cancel()

	logFile, err := os.Create(filepath.Join(runDir, "adapter.log"))
	if err != nil {
		return false, err
	}
	defer logFile.Close()

	cmd := exec.CommandContext(ctx, env.harness.CommandPath())
	cmd.Dir = workDir
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Env = append(os.Environ(),
		"BENCH_WORKDIR="+workDir,
		"BENCH_PROMPT_FILE="+promptFile,
		"BENCH_MODEL="+run.Model.ID,
		"BENCH_EFFORT="+run.Effort,
		"BENCH_OUT="+filepath.Join(runDir, "out"),
		"BENCH_TIMEOUT="+strconv.Itoa(int(env.timeout.Seconds())),
		"PATH="+filepath.Join(runDir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
	)

	err = cmd.Run()
	timedOut := errors.Is(ctx.Err(), context.DeadlineExceeded)

	return timedOut, err
}

// collect reads what the run left: usage, the call log, the review file
// and a final validate with the pinned binary.
func collect(env *runEnv, run config.Run, result *metrics.Result, workDir, runDir, reviewRel, callLog string) {
	if data, err := os.ReadFile(filepath.Join(runDir, "out", env.harness.UsageFile)); err == nil {
		usage, err := metrics.ParseUsage(env.harness.UsageFormat, data)
		result.Usage = usage
		if err != nil && result.HarnessError == "" {
			result.HarnessError = err.Error()
		}
	}

	price, havePrice := env.prices.Models[run.Model.Price]
	result.CostUSD, result.CostSource = metrics.Cost(result.Usage, price, havePrice)

	if data, err := os.ReadFile(callLog); err == nil {
		result.Calls = metrics.ParseCallLog(data)
	}

	reviewFile := filepath.Join(workDir, reviewRel)
	reviewData, err := os.ReadFile(reviewFile)
	if err != nil {
		result.Final = metrics.Final{Exit: -1}

		return
	}

	_ = os.WriteFile(filepath.Join(runDir, "review.json"), reviewData, 0o644)
	result.Review, _ = metrics.ParseReview(reviewData)

	validated, _ := exec.Command(env.taskView, "validate", reviewFile, "--format", "json").Output()
	_ = os.WriteFile(filepath.Join(runDir, "validate.json"), validated, 0o644)
	if final, err := metrics.ParseValidate(validated); err == nil {
		result.Final = final
	}

	// The page is rendered now, while the work dir it reads still exists. A
	// review with errors exits 1 but still gets its page, banner included,
	// so the exit code is ignored and only a missing page shows the failure.
	_ = exec.Command(env.taskView, "open", reviewFile, "--no-browser", "--out", filepath.Join(runDir, "review.html")).Run()
}

func cmdReport(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: bench report <results dir>")
	}

	dir := args[0]
	results, err := readResults(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		return err
	}

	var meta report.Meta
	if data, err := os.ReadFile(filepath.Join(dir, "meta.json")); err == nil {
		_ = json.Unmarshal(data, &meta)
	}

	cells := report.Aggregate(report.Latest(results))
	if err := os.WriteFile(filepath.Join(dir, "report.md"), []byte(report.Markdown(meta, cells)), 0o644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "summary.csv"), report.CSV(cells), 0o644); err != nil {
		return err
	}

	fmt.Printf("%d runs in %d cells: %s\n", len(results), len(cells), filepath.Join(dir, "report.md"))

	return nil
}

// archiveName is the published archive of every run's review and page.
const archiveName = "reviews.tar.xz"

func cmdPublish(args []string) error {
	flags := flag.NewFlagSet("publish", flag.ContinueOnError)
	rootFlag := flags.String("root", "", "the bench folder (default: . or bench/)")
	dashboard := flags.String("dashboard", "", "link to the results drawn as charts, shown in BENCH.md")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if flags.NArg() != 1 {
		return errors.New("usage: bench publish [-root dir] [-dashboard url] <results dir>")
	}

	paths, err := findLayout(*rootFlag)
	if err != nil {
		return err
	}

	dir := flags.Arg(0)
	results, err := readResults(filepath.Join(dir, "runs.jsonl"))
	if err != nil {
		return err
	}

	metaData, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if err != nil {
		return err
	}

	var meta report.Meta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return fmt.Errorf("meta.json: %w", err)
	}

	suite, err := config.LoadSuite(paths.path("cases.json"))
	if err != nil {
		return err
	}

	// Only the latest run is kept in the repo, so the folder is replaced
	// whole rather than merged into.
	published := paths.path("results")
	if err := os.RemoveAll(published); err != nil {
		return err
	}

	if err := os.MkdirAll(published, 0o755); err != nil {
		return err
	}

	latest := report.ApplySizes(report.Latest(results), suite)
	cells := report.Aggregate(latest)
	var lines []byte
	for _, result := range latest {
		line, _ := json.Marshal(result)
		lines = append(append(lines, line...), '\n')
	}

	files := map[string][]byte{
		"meta.json":   metaData,
		"runs.jsonl":  lines,
		"summary.csv": report.CSV(cells),
	}

	for name, data := range files {
		if err := os.WriteFile(filepath.Join(published, name), data, 0o644); err != nil {
			return err
		}
	}

	archive := filepath.Join(published, archiveName)
	if err := writeArchive(filepath.Join(dir, "runs"), latest, archive); err != nil {
		return err
	}

	archiveRel, err := filepath.Rel(paths.repoRoot, archive)
	if err != nil {
		return err
	}

	markdown := report.BenchMarkdown(report.Bench{
		Meta:      meta,
		Suite:     suite,
		Results:   latest,
		Cells:     cells,
		Archive:   filepath.ToSlash(archiveRel),
		Dashboard: *dashboard,
	})

	benchFile := filepath.Join(paths.repoRoot, "BENCH.md")
	if err := os.WriteFile(benchFile, []byte(markdown), 0o644); err != nil {
		return err
	}

	fmt.Printf("%d runs in %d cells: %s and %s\n", len(latest), len(cells), benchFile, published)

	return nil
}

// writeArchive packs each run's review file, page and final validate into
// one xz tarball. xz rather than the standard library's gzip or zip: every
// page embeds the same vendored libraries, and only xz's window is wide
// enough to store them once instead of once per page.
func writeArchive(runsDir string, results []metrics.Result, archive string) error {
	staging, err := os.MkdirTemp("", "bench-publish-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)

	const top = "reviews"
	for _, result := range results {
		target := filepath.Join(staging, top, result.RunID)
		if err := os.MkdirAll(target, 0o755); err != nil {
			return err
		}

		copies := map[string]string{
			"review.json":   "change-" + result.Case + ".json",
			"review.html":   "review.html",
			"validate.json": "validate.json",
		}

		for source, name := range copies {
			data, err := os.ReadFile(filepath.Join(runsDir, result.RunID, source))
			if errors.Is(err, os.ErrNotExist) {
				continue
			}

			if err != nil {
				return err
			}

			if err := os.WriteFile(filepath.Join(target, name), data, 0o644); err != nil {
				return err
			}
		}
	}

	absolute, err := filepath.Abs(archive)
	if err != nil {
		return err
	}

	// COPYFILE_DISABLE keeps macOS tar from adding ._ metadata files.
	cmd := exec.Command("tar", "-cJf", absolute, "-C", staging, top)
	cmd.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("tar: %w: %s", err, output)
	}

	return nil
}

func readResults(file string) ([]metrics.Result, error) {
	handle, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer handle.Close()

	return decodeResults(handle)
}

func decodeResults(reader io.Reader) ([]metrics.Result, error) {
	var results []metrics.Result
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		var result metrics.Result
		if err := json.Unmarshal([]byte(line), &result); err != nil {
			return nil, err
		}

		results = append(results, result)
	}

	return results, scanner.Err()
}

func splitList(value string) []string {
	var items []string
	for _, item := range strings.Split(value, ",") {
		if item = strings.TrimSpace(item); item != "" {
			items = append(items, item)
		}
	}

	return items
}

func plural(count int, one, many string) string {
	if count == 1 {
		return one
	}

	return many
}
