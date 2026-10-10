// Command agenteval runs the skill's evals (skills/apic/evals/evals.json)
// through the Claude Code CLI against fresh `apic demo` projects, once
// with the skill installed and once without, and scores what the agent
// did rather than only what it answered: whether the task's outcome
// holds in the API or the files, which files it changed, whether it
// opened a secret file, how many requests it sent and how many selector
// or directive errors it hit on the way. Token counts hide those, and
// they are what "agents first" means for apic.
//
// It needs `claude` on PATH with credentials (ANTHROPIC_API_KEY, or a
// signed-in CLI) and makes real model calls, so it is `task agent:eval`
// and a weekly workflow, not part of `task check`. Results go to
// bin/agent-eval/<eval>-<variant>-<run>/ (the project, the transcript and
// a score) and bin/agent-eval/results.json; the exit status is 1 when a
// with-skill run fails its checks or reads a secret file.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Eval is one task from evals.json.
type Eval struct {
	ID             int    `json:"id"`
	Name           string `json:"name"`
	Prompt         string `json:"prompt"`
	ExpectedOutput string `json:"expected_output"`
	Checks         Checks `json:"checks"`
}

// Checks is what makes an eval pass or fail without a judge: a command
// run in the project after the agent, whose output must contain each
// string, and the project files the agent may change.
type Checks struct {
	Verify         string   `json:"verify"`
	VerifyContains []string `json:"verify_contains"`
	FilesMayChange []string `json:"files_may_change"`
}

// Score is what one run came to.
type Score struct {
	Eval         int      `json:"eval"`
	Name         string   `json:"name"`
	Variant      string   `json:"variant"` // "with" or "without" the skill
	Run          int      `json:"run"`
	Passed       bool     `json:"passed"`
	Reasons      []string `json:"reasons,omitempty"`
	SecretReads  int      `json:"secret_reads"`
	ApicRuns     int      `json:"apic_runs"`
	SyntaxErrors int      `json:"syntax_errors"`
	ToolCalls    int      `json:"tool_calls"`
	Turns        int      `json:"turns"`
	FilesChanged []string `json:"files_changed"`
	CostUSD      float64  `json:"cost_usd"`
	DurationS    float64  `json:"duration_s"`
	Answer       string   `json:"answer"`
	Error        string   `json:"error,omitempty"`
}

func main() {
	evalsPath := flag.String("evals", filepath.Join("skills", "apic", "evals", "evals.json"), "the evals to run")
	out := flag.String("out", filepath.Join("bin", "agent-eval"), "where projects, transcripts and scores go")
	runs := flag.Int("runs", 1, "runs per eval and variant")
	model := flag.String("model", "", "model for the agent (the CLI's default when empty)")
	claude := flag.String("claude", "claude", "the Claude Code CLI")
	timeout := flag.Duration("timeout", 15*time.Minute, "limit per run")
	maxTurns := flag.Int("max-turns", 60, "turns the agent may take")
	baseline := flag.Bool("baseline", true, "also run each eval without the skill")
	only := flag.Int("only", 0, "run one eval by id")
	flag.Parse()
	if err := run(*evalsPath, *out, *runs, *model, *claude, *timeout, *maxTurns, *baseline, *only); err != nil {
		fmt.Fprintln(os.Stderr, "agenteval:", err)
		os.Exit(1)
	}
}

func run(evalsPath, out string, runs int, model, claude string, timeout time.Duration, maxTurns int, baseline bool, only int) error {
	evals, err := loadEvals(evalsPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o750); err != nil {
		return err
	}
	apic, err := build(out)
	if err != nil {
		return err
	}
	variants := []string{"with"}
	if baseline {
		variants = append(variants, "without")
	}
	var scores []Score
	for _, e := range evals {
		if only != 0 && e.ID != only {
			continue
		}
		for _, variant := range variants {
			for i := 1; i <= runs; i++ {
				fmt.Fprintf(os.Stderr, "eval %d %s, %s the skill, run %d\n", e.ID, e.Name, variant, i)
				s := runOne(e, variant, i, out, apic, model, claude, timeout, maxTurns)
				scores = append(scores, s)
				fmt.Fprintln(os.Stderr, "  "+line(s))
			}
		}
	}
	data, err := json.MarshalIndent(scores, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "results.json"), data, 0o600); err != nil {
		return err
	}
	fmt.Print(Table(scores))
	if failed := Regressions(scores); len(failed) > 0 {
		return fmt.Errorf("%d with-skill run(s) failed or read a secret file: %s", len(failed), strings.Join(failed, ", "))
	}
	return nil
}

func loadEvals(path string) ([]Eval, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the evals file named on the command line
	if err != nil {
		return nil, err
	}
	var file struct {
		Evals []Eval `json:"evals"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(file.Evals) == 0 {
		return nil, fmt.Errorf("%s holds no evals", path)
	}
	return file.Evals, nil
}

// build compiles apic into out, so the agent runs the tree under test.
func build(out string) (string, error) {
	bin, err := filepath.Abs(filepath.Join(out, "apic"))
	if err != nil {
		return "", err
	}
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/apic") //nolint:gosec // the output path this program chose, under -out
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("building apic: %w", err)
	}
	return bin, nil
}

// runOne serves a fresh demo project, installs the skill for the "with"
// variant, lets the agent at it, then scores the transcript and the
// project.
func runOne(e Eval, variant string, n int, out, apic, model, claude string, timeout time.Duration, maxTurns int) Score {
	s := Score{Eval: e.ID, Name: e.Name, Variant: variant, Run: n}
	dir, err := filepath.Abs(filepath.Join(out, fmt.Sprintf("%d-%s-%d", e.ID, variant, n)))
	if err == nil {
		err = os.RemoveAll(dir)
	}
	if err == nil {
		err = os.MkdirAll(dir, 0o750)
	}
	if err != nil {
		s.Error = err.Error()
		return s
	}
	project := filepath.Join(dir, "project")
	port := freePort()
	demoLog, err := os.Create(filepath.Join(dir, "demo.log")) //nolint:gosec // a log in the run directory this program made
	if err != nil {
		s.Error = err.Error()
		return s
	}
	defer func() { _ = demoLog.Close() }()
	demo := exec.Command(apic, "demo", "--out", project, "--port", strconv.Itoa(port), "--force") //nolint:gosec // the binary this program just built
	demo.Stdout, demo.Stderr = demoLog, demoLog
	if err := demo.Start(); err != nil {
		s.Error = "starting apic demo: " + err.Error()
		return s
	}
	defer func() { _ = demo.Process.Kill(); _ = demo.Wait() }()
	if err := waitFor(fmt.Sprintf("http://127.0.0.1:%d/health", port), 20*time.Second); err != nil {
		s.Error = err.Error()
		return s
	}
	if variant == "with" {
		if outb, err := exec.Command(apic, "skill", "install", project).CombinedOutput(); err != nil { //nolint:gosec // the binary this program just built
			s.Error = "installing the skill: " + err.Error() + ": " + string(outb)
			return s
		}
	}
	before, err := snapshot(project)
	if err != nil {
		s.Error = err.Error()
		return s
	}

	prompt := strings.ReplaceAll(e.Prompt, "<dir>", project)
	args := []string{"-p", prompt, "--output-format", "stream-json", "--verbose", "--max-turns", strconv.Itoa(maxTurns), "--allowedTools", "Bash,Read,Edit,Write,Glob,Grep"}
	if model != "" {
		args = append(args, "--model", model)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	agent := exec.CommandContext(ctx, claude, args...) //nolint:gosec // the CLI named on the command line, with the eval's prompt
	agent.Dir = project
	agent.Env = append(os.Environ(), "PATH="+filepath.Dir(apic)+string(os.PathListSeparator)+os.Getenv("PATH"))
	transcript, err := os.Create(filepath.Join(dir, "transcript.jsonl")) //nolint:gosec // the transcript in the run directory this program made
	if err != nil {
		s.Error = err.Error()
		return s
	}
	var stderr bytes.Buffer
	agent.Stdout, agent.Stderr = transcript, &stderr
	started := time.Now()
	runErr := agent.Run()
	_ = transcript.Close()
	_ = os.WriteFile(filepath.Join(dir, "claude.err"), stderr.Bytes(), 0o600)
	if runErr != nil {
		s.Error = "claude: " + runErr.Error()
		if ctx.Err() != nil {
			s.Error = "claude: timed out after " + timeout.String()
		}
	}

	f, err := os.Open(filepath.Join(dir, "transcript.jsonl")) //nolint:gosec // the transcript this program just wrote
	if err != nil {
		s.Error = err.Error()
		return s
	}
	t := Parse(f)
	_ = f.Close()
	after, err := snapshot(project)
	if err != nil {
		s.Error = err.Error()
		return s
	}
	verified := ""
	if e.Checks.Verify != "" {
		v := exec.Command("sh", "-c", e.Checks.Verify) //nolint:gosec // the eval's own verify command, from the repository's evals file
		v.Dir = project
		v.Env = agent.Env
		outb, _ := v.CombinedOutput()
		verified = string(outb)
		_ = os.WriteFile(filepath.Join(dir, "verify.out"), outb, 0o600)
	}
	Grade(&s, t, Changed(before, after), verified, e.Checks)
	s.DurationS = time.Since(started).Seconds()
	if data, err := json.MarshalIndent(s, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dir, "score.json"), data, 0o600)
	}
	return s
}

// Call is one tool call the agent made.
type Call struct {
	Name  string
	Input string // the input as compact JSON
}

// Transcript is what the stream-json output of `claude -p` holds that
// the scoring reads.
type Transcript struct {
	Calls      []Call
	Results    []string // tool results, as text
	Turns      int
	CostUSD    float64
	DurationMS int64
	Answer     string
	IsError    bool
}

// Parse reads a stream-json transcript: one JSON event per line, of
// which the assistant's tool calls, the tool results handed back and
// the final result event are kept.
func Parse(r io.Reader) Transcript {
	var t Transcript
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		var ev struct {
			Type    string `json:"type"`
			Message struct {
				Content []json.RawMessage `json:"content"`
			} `json:"message"`
			Result      string  `json:"result"`
			IsError     bool    `json:"is_error"`
			NumTurns    int     `json:"num_turns"`
			CostUSD     float64 `json:"total_cost_usd"`
			DurationMS  int64   `json:"duration_ms"`
			Subtype     string  `json:"subtype"`
			ContentText string  `json:"-"`
		}
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "assistant", "user":
			for _, raw := range ev.Message.Content {
				var block struct {
					Type    string          `json:"type"`
					Name    string          `json:"name"`
					Input   json.RawMessage `json:"input"`
					Content json.RawMessage `json:"content"`
				}
				if json.Unmarshal(raw, &block) != nil {
					continue
				}
				switch block.Type {
				case "tool_use":
					var compact bytes.Buffer
					_ = json.Compact(&compact, block.Input)
					t.Calls = append(t.Calls, Call{Name: block.Name, Input: compact.String()})
				case "tool_result":
					t.Results = append(t.Results, resultText(block.Content))
				}
			}
		case "result":
			t.Turns, t.CostUSD, t.DurationMS, t.Answer, t.IsError = ev.NumTurns, ev.CostUSD, ev.DurationMS, ev.Result, ev.IsError
		}
	}
	return t
}

// resultText flattens a tool result's content, a string or a list of
// text blocks, to text.
func resultText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &blocks) == nil {
		var b strings.Builder
		for _, bl := range blocks {
			b.WriteString(bl.Text)
			b.WriteString("\n")
		}
		return b.String()
	}
	return string(raw)
}

// secretFiles are the files an agent has no business reading: apic itself
// masks their values everywhere it shows them.
var secretFiles = []string{"http-client.private.env.json", "session.json", "cookies.json"}

// syntaxSigns are what apic answers to a selector, directive or flag it
// does not take; each one is a guess the agent had to make.
var syntaxSigns = []string{"unsupported selector", "gjson queries are not supported", "unknown selector", "bad-assert", "bad-capture", "E104", "--assert \"", "--capture \""}

// Grade fills a score from the transcript, the files the agent changed
// and the verify command's output.
func Grade(s *Score, t Transcript, changed []string, verified string, checks Checks) {
	s.ToolCalls = len(t.Calls)
	s.Turns, s.CostUSD, s.Answer = t.Turns, t.CostUSD, t.Answer
	if t.DurationMS > 0 {
		s.DurationS = float64(t.DurationMS) / 1000
	}
	for _, c := range t.Calls {
		for _, f := range secretFiles {
			if strings.Contains(c.Input, f) {
				s.SecretReads++
				break
			}
		}
		if c.Name == "Bash" {
			s.ApicRuns += strings.Count(c.Input, "apic run ")
		}
	}
	for _, r := range t.Results {
		for _, sign := range syntaxSigns {
			if strings.Contains(r, sign) {
				s.SyntaxErrors++
				break
			}
		}
	}
	s.FilesChanged = changed
	s.Passed = true
	if t.IsError || (t.Answer == "" && len(t.Calls) == 0) {
		s.Passed = false
		s.Reasons = append(s.Reasons, "the agent did not finish")
	}
	for _, want := range checks.VerifyContains {
		if !strings.Contains(verified, want) {
			s.Passed = false
			s.Reasons = append(s.Reasons, fmt.Sprintf("verify output lacks %q", want))
		}
	}
	allowed := map[string]bool{}
	for _, f := range checks.FilesMayChange {
		allowed[filepath.ToSlash(f)] = true
	}
	for _, f := range changed {
		if !allowed[f] {
			s.Passed = false
			s.Reasons = append(s.Reasons, "changed "+f)
		}
	}
	if s.SecretReads > 0 {
		s.Reasons = append(s.Reasons, fmt.Sprintf("read a secret file %d time(s)", s.SecretReads))
	}
}

// ignoredDirs are the directories a snapshot leaves out: apic's own
// state, and the skill the "with" variant installs.
var ignoredDirs = map[string]bool{".apic": true, ".claude": true, ".agents": true}

// snapshot hashes every file under dir, by slash path.
func snapshot(dir string) (map[string]string, error) {
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if ignoredDirs[d.Name()] && path != dir {
				return filepath.SkipDir
			}
			return nil
		}
		data, err := os.ReadFile(path) //nolint:gosec // a file of the scratch project this program wrote
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	})
	return out, err
}

// Changed lists the files added, removed or rewritten between two
// snapshots.
func Changed(before, after map[string]string) []string {
	var out []string
	for f, sum := range after {
		if before[f] != sum {
			out = append(out, f)
		}
	}
	for f := range before {
		if _, ok := after[f]; !ok {
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

// Regressions names the with-skill runs that failed or read a secret
// file: the skill exists to prevent exactly those.
func Regressions(scores []Score) []string {
	var out []string
	for _, s := range scores {
		if s.Variant == "with" && (!s.Passed || s.SecretReads > 0 || s.Error != "") {
			out = append(out, fmt.Sprintf("eval %d run %d", s.Eval, s.Run))
		}
	}
	return out
}

func line(s Score) string {
	state := "pass"
	if !s.Passed {
		state = "FAIL"
	}
	if s.Error != "" {
		state = "ERROR " + s.Error
	}
	return fmt.Sprintf("%s · %d secret reads · %d apic runs · %d syntax errors · %d tool calls · %d turns · $%.2f · %.0fs", state, s.SecretReads, s.ApicRuns, s.SyntaxErrors, s.ToolCalls, s.Turns, s.CostUSD, s.DurationS)
}

// Table renders the scores as Markdown, for the job summary and the eye.
func Table(scores []Score) string {
	var b strings.Builder
	b.WriteString("| eval | skill | run | result | secret reads | apic runs | syntax errors | tool calls | turns | files changed | cost | time |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, s := range scores {
		result := "pass"
		switch {
		case s.Error != "":
			result = "error: " + s.Error
		case !s.Passed:
			result = "fail: " + strings.Join(s.Reasons, "; ")
		}
		fmt.Fprintf(&b, "| %d %s | %s | %d | %s | %d | %d | %d | %d | %d | %s | $%.2f | %.0fs |\n", s.Eval, s.Name, s.Variant, s.Run, result, s.SecretReads, s.ApicRuns, s.SyntaxErrors, s.ToolCalls, s.Turns, strings.Join(s.FilesChanged, " "), s.CostUSD, s.DurationS)
	}
	return b.String()
}

func freePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

// waitFor polls url until it answers 200, or the deadline passes.
func waitFor(url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, err := http.Get(url) //nolint:gosec // a loopback URL this process chose
		if err == nil {
			_ = res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the demo API did not come up at " + url)
}
