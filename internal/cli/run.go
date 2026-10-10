package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/assert"
	"github.com/hungovercoders/apic/internal/datafile"
	"github.com/hungovercoders/apic/internal/httpfile"
	"github.com/hungovercoders/apic/internal/output"
	"github.com/hungovercoders/apic/internal/report"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/internal/selector"
)

func (a *App) runCmd() *cobra.Command {
	var verbose, bodyOnly, keepGoing, noRetry, dryRun bool
	var adHocAsserts, adHocCaptures []string
	var retry, reportPath, outputPath, dataPath, bodyLimit string
	var shareSession bool
	cmd := &cobra.Command{
		Use:   "run <request|file.http|file.http#name>...",
		Short: "Send one request, or every request in a file as a flow",
		Long: `Send requests and report status, timing, body, captures and assertions.

Targets:
  get-user             a request with "# @name get-user"
  users.http           every request in the file, in order (a flow)
  users.http#get-user  a named request in a specific file
  users.http#3         the third request in the file

Captured values (# @capture) are stored per environment in .apic/session.json
so a later invocation can use them. Use --no-session to disable. A request
that declares "# @ref login" runs login first when a value it needs is
missing; "# @forceRef login" runs it first every time. One that declares
"# @retry 10 2s" is re-sent until its assertions pass, up to 10 times, two
seconds apart; each failed attempt prints a line as it happens.

--assert and --capture add a check or a capture to every target for this
run only, the same expressions "# @assert" and "# @capture" take, so a
selector can be tried before it is written into the file. --dry-run
resolves each target as a run would and prints the request instead of
sending it: no dependency runs, no auth is applied and nothing is captured.`,
		Example: `  apic run login
  apic run get-user --env staging --var userId=42
  apic run smoke.http --json | jq .response.status
  apic run get-user --body-only | jq .email
  apic run smoke.http --keep-going --report report.html
  apic run daily-report --output reports/daily.csv
  apic run list-todos --assert 'body.$[?(@.done != true)].# == 0'
  apic run create-todo --capture id=body.$.id --json
  apic run delete-todo --var todoId=3 --dry-run`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			extraAsserts, extraCaptures, err := adHoc(adHocAsserts, adHocCaptures)
			if err != nil {
				return err
			}
			limit, err := parseSize(bodyLimit)
			if err != nil {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("--body-limit %q: %v", bodyLimit, err))
			}
			p, err := a.loadProject()
			if err != nil {
				return err
			}
			// A data run records no history: a response per row would push
			// out what the history is for.
			r, err := a.runnerFor(p, func(o *runner.Options) { o.NoHistory = dataPath != "" })
			if err != nil {
				return err
			}
			if dryRun {
				switch {
				case dataPath != "":
					return runner.Usage(runner.CodeFlag, "--dry-run shows one request per target; it does not take --data")
				case outputPath != "" || reportPath != "":
					return runner.Usage(runner.CodeFlag, "--dry-run sends nothing, so there is no response to save or report")
				}
			}
			var rows []datafile.Row
			if dataPath != "" {
				if rows, err = a.readRows(dataPath); err != nil {
					return err
				}
			}
			// configure applies the run's flags to a runner: the first one,
			// and each iteration's own under --data.
			configure := func(r *runner.Runner) {
				r.Opts.KeepGoing = keepGoing
				r.Opts.Retry, r.Opts.NoRetry = retry, noRetry
				r.Opts.Asserts, r.Opts.Captures = extraAsserts, extraCaptures
				if !a.g.json && !bodyOnly {
					r.Progress = func(p runner.Progress) { fmt.Fprint(a.Stdout, output.Attempt(output.Default(), p)) }
				}
			}
			configure(r)
			reqs, err := targets(r, args)
			if err != nil {
				return err
			}
			flow := len(reqs) > 1
			if dryRun {
				return a.dryRun(r, reqs, verbose, bodyOnly)
			}
			if reportPath != "" {
				if err := outputOverlapsSources(reportPath, r.Project, nil); err != nil {
					return err
				}
			}
			if outputPath != "" {
				switch {
				case flow:
					return runner.Usage(runner.CodeFlag, fmt.Sprintf("--output saves one response; the targets name %d requests (add `>> file` lines to the requests instead)", len(reqs)))
				case len(rows) > 0:
					return runner.Usage(runner.CodeFlag, fmt.Sprintf("--output saves one response; --data runs the request %d times (add a `>> file` line with a {{placeholder}} in the path instead)", len(rows)))
				}
				if err := outputOverlapsSources(outputPath, r.Project, nil); err != nil {
					return err
				}
				r.Opts.Output = outputPath
			}
			started := time.Now()
			// Each result is printed as it lands, so a flow shows progress
			// (and the attempt lines of a retried request sit under the
			// right request) instead of everything at the end.
			var printErr error
			printed := 0
			var iteration *runner.Iteration // the row being run under --data
			onResult := func(res *runner.Result, err error) {
				defer func() { printed++ }()
				if iteration != nil {
					tagIteration(res, iteration)
				}
				if limit > 0 {
					limitBodies(res, limit)
				}
				switch {
				case a.g.json:
					if e := output.JSON(a.Stdout, res); e != nil && printErr == nil {
						printErr = e
					}
				case bodyOnly:
					output.Body(a.Stdout, res)
				default:
					if flow && printed > 0 {
						fmt.Fprintln(a.Stdout)
					}
					if res.Response == nil && err != nil {
						// The error is printed by Execute; show what ran first
						// and the request line for context.
						output.Deps(a.Stdout, res, verbose)
						fmt.Fprintf(a.Stdout, "%s %s\n", res.Request.Method, res.Request.DisplayURL(res.Redact))
						return
					}
					output.Human(a.Stdout, res, verbose)
				}
			}
			var results []*runner.Result
			var runErr error
			passedRows, failedRows := 0, 0
			if len(rows) == 0 {
				r.OnResult = onResult
				results, runErr = r.RunAll(cmd.Context(), reqs)
				if flow && !a.g.json && !bodyOnly {
					output.Summary(a.Stdout, results)
				}
			} else {
				results, runErr = a.runRows(cmd, r, rows, args, shareSession, keepGoing, func(it *runner.Iteration, ir *runner.Runner) {
					iteration, printed = it, 0
					configure(ir)
					ir.OnResult = onResult
					if !a.g.json && !bodyOnly {
						if it.Index > 1 {
							fmt.Fprintln(a.Stdout)
						}
						heading := fmt.Sprintf("iteration %d/%d", it.Index, it.Total)
						if row := rows[it.Index-1].String(); row != "" && !a.g.redact {
							heading += " " + theme.Dim.Render("· "+row)
						}
						fmt.Fprintln(a.Stdout, theme.Bold.Render(heading))
					}
				}, func(iterResults []*runner.Result) {
					if flow && !a.g.json && !bodyOnly {
						output.Summary(a.Stdout, iterResults)
					}
					ok := len(iterResults) > 0
					for _, res := range iterResults {
						ok = ok && res.OK
					}
					if ok {
						passedRows++
					} else {
						failedRows++
					}
				})
				if !a.g.json && !bodyOnly {
					line := theme.OK.Render(fmt.Sprintf("%d passed", passedRows))
					if failedRows > 0 {
						line = theme.Fail.Render(fmt.Sprintf("%d failed", failedRows)) + fmt.Sprintf(", %d passed", passedRows)
					}
					fmt.Fprintf(a.Stdout, "\n%s %s\n", theme.Bold.Render(fmt.Sprintf("%d of %d iterations:", passedRows+failedRows, len(rows))), line)
				}
			}
			if printErr != nil {
				return printErr
			}
			failed := false
			for _, res := range results {
				if !res.OK {
					failed = true
				}
			}
			if reportPath != "" {
				var buf bytes.Buffer
				meta := report.Meta{Version: Version, Env: r.Opts.Env, Time: started, Redacted: a.g.redact, Project: r.Project.Root}
				if err := report.Run(&buf, meta, results); err != nil {
					return runner.Usage(runner.CodeFile, fmt.Sprintf("write report %s: %v", reportPath, err))
				}
				if err := os.WriteFile(reportPath, buf.Bytes(), 0o644); err != nil { //nolint:gosec // a report the user asked for, at the path they named
					return runner.Usage(runner.CodeFile, fmt.Sprintf("write report %s: %v (run result: %v)", reportPath, err, describeOutcome(runErr)))
				}
			}
			if runErr != nil {
				return runErr
			}
			if failed {
				return &exitError{code: runner.ExitAssert}
			}
			return nil
		},
	}
	cmd.ValidArgsFunction = a.completeRequests
	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show request and response headers")
	cmd.Flags().BoolVar(&bodyOnly, "body-only", false, "print only the response body (for piping)")
	cmd.Flags().BoolVar(&keepGoing, "keep-going", false, "in a flow, continue after a failure")
	cmd.Flags().StringVar(&retry, "retry", "", "re-send until the assertions pass: \"<attempts> [interval]\", e.g. \"10 2s\" (requests with # @retry keep their own)")
	cmd.Flags().BoolVar(&noRetry, "no-retry", false, "send every request once, ignoring # @retry, --retry and apic.yaml")
	cmd.Flags().StringVar(&reportPath, "report", "", "also write a self-contained HTML report of the run to this file")
	cmd.Flags().StringVar(&outputPath, "output", "", "save the response body to this file (one request only; like a \">>! file\" line in the request)")
	cmd.Flags().StringVar(&dataPath, "data", "", "run the targets once per row of a CSV file (header row names the variables) or JSON array of objects; - reads stdin")
	cmd.Flags().BoolVar(&shareSession, "data-share-session", false, "with --data, let captures from one iteration reach the next and the session file")
	cmd.Flags().StringArrayVar(&adHocAsserts, "assert", nil, "check the response with an expression, as # @assert would, for this run only (repeatable)")
	cmd.Flags().StringArrayVar(&adHocCaptures, "capture", nil, "capture a value, name=selector, as # @capture would, for this run only (repeatable)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "resolve the targets and print the requests that would be sent, without sending them")
	cmd.Flags().StringVar(&bodyLimit, "body-limit", "", "show at most this much of each response body, e.g. 4k or 64k; the result says body_truncated and the history keeps it all")
	return cmd
}

// adHoc checks the --assert expressions and --capture definitions once,
// with the checks `apic validate` gives the directives (the expression
// parses, the selector is one apic knows, the name is one a directive
// could have), so a typo is a flag error naming the flag before anything
// is sent, not a failure after a request that may have changed something.
func adHoc(asserts, captures []string) ([]httpfile.Assert, []httpfile.Capture, error) {
	var extraAsserts []httpfile.Assert
	for _, raw := range asserts {
		expr, err := assert.Parse(raw)
		if err == nil {
			err = selector.Check(expr.Selector)
		}
		if err != nil {
			return nil, nil, runner.Usage(runner.CodeFlag, fmt.Sprintf("--assert %q: %v", raw, err))
		}
		extraAsserts = append(extraAsserts, httpfile.Assert{Expr: raw})
	}
	var extraCaptures []httpfile.Capture
	for _, def := range captures {
		name, sel, ok := strings.Cut(def, "=")
		name, sel = strings.TrimSpace(name), strings.TrimSpace(sel)
		if !ok || !httpfile.ValidCaptureName(name) || sel == "" {
			return nil, nil, runner.Usage(runner.CodeFlag, fmt.Sprintf("--capture %q: expected name=selector, e.g. token=body.$.access_token", def))
		}
		if err := selector.Check(sel); err != nil {
			return nil, nil, runner.Usage(runner.CodeFlag, fmt.Sprintf("--capture %q: %v", def, err))
		}
		extraCaptures = append(extraCaptures, httpfile.Capture{Name: name, Selector: sel})
	}
	return extraAsserts, extraCaptures, nil
}

// dryRun prints each target as it would be sent and sends nothing. Under
// --body-only it prints the request body, there being no response.
func (a *App) dryRun(r *runner.Runner, reqs []*httpfile.Request, verbose, bodyOnly bool) error {
	for i, req := range reqs {
		res, err := r.DryRun(req)
		if err != nil {
			return err
		}
		switch {
		case a.g.json:
			if err := output.JSON(a.Stdout, res); err != nil {
				return err
			}
		case bodyOnly:
			if body := res.Request.DisplayBody(res.Redact); body != "" {
				fmt.Fprintln(a.Stdout, strings.TrimRight(body, "\n"))
			}
		default:
			if i > 0 {
				fmt.Fprintln(a.Stdout)
			}
			output.Human(a.Stdout, res, verbose)
		}
	}
	return nil
}

// targets resolves the command line's targets in order.
func targets(r *runner.Runner, args []string) ([]*httpfile.Request, error) {
	var reqs []*httpfile.Request
	for _, t := range args {
		rs, err := r.Target(t)
		if err != nil {
			return nil, runner.Usage(runner.CodeUnknownRequest, err.Error())
		}
		reqs = append(reqs, rs...)
	}
	return reqs, nil
}

// readRows reads the rows of --data from a file, or stdin for -.
func (a *App) readRows(path string) ([]datafile.Row, error) {
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(a.Stdin)
	} else {
		data, err = os.ReadFile(path) //nolint:gosec // the data file the user named
	}
	if err != nil {
		return nil, runner.Usage(runner.CodeFile, fmt.Sprintf("--data: %v", err))
	}
	rows, err := datafile.Read(data, path)
	if err != nil {
		return nil, runner.Usage(runner.CodeData, fmt.Sprintf("--data %s: %v", path, err))
	}
	if len(rows) == 0 {
		return nil, runner.Usage(runner.CodeData, fmt.Sprintf("--data %s has no rows to run", path))
	}
	return rows, nil
}

// runRows runs the targets once per row. Each iteration gets the row's
// values as variables, over --var. Unless share is set it runs on a fresh
// runner whose session is a snapshot of the one on disk, so its captures
// neither reach the next iteration nor the session file; with share one
// runner carries them through. start is called before each iteration,
// done after it with its results. A failed iteration stops the run
// unless keepGoing is set.
func (a *App) runRows(cmd *cobra.Command, r *runner.Runner, rows []datafile.Row, args []string, share, keepGoing bool, start func(*runner.Iteration, *runner.Runner), done func([]*runner.Result)) ([]*runner.Result, error) {
	base := r.Opts.Vars
	session := r.Session
	var all []*runner.Result
	var firstErr error
	for i, row := range rows {
		vars := map[string]string{}
		for k, v := range base {
			vars[k] = v
		}
		for k, v := range row.Values {
			vars[k] = v
		}
		ir := r
		if share {
			r.Opts.Vars = vars
		} else {
			var err error
			ir, err = a.runnerFor(r.Project, func(o *runner.Options) {
				o.Vars = vars
				o.NoHistory = true
				if session != nil {
					o.Session = session.Snapshot()
				}
			})
			if err != nil {
				return all, err
			}
		}
		reqs, err := targets(ir, args)
		if err != nil {
			return all, err
		}
		start(&runner.Iteration{Index: i + 1, Total: len(rows), Row: row.Values}, ir)
		results, runErr := ir.RunAll(cmd.Context(), reqs)
		all = append(all, results...)
		done(results)
		ok := runErr == nil
		for _, res := range results {
			ok = ok && res.OK
		}
		if runErr != nil && firstErr == nil {
			firstErr = runErr
		}
		if !ok && !keepGoing {
			break
		}
	}
	return all, firstErr
}

// limitBodies bounds the body shown for a result and for what it ran
// first; the history, written before this, keeps the whole body.
func limitBodies(res *runner.Result, limit int) {
	res.BodyLimit = limit
	for _, d := range res.Deps {
		limitBodies(d, limit)
	}
}

// parseSize reads a byte count with an optional k or m suffix (1024
// based, as `4k` on a command line means); empty means none.
func parseSize(s string) (int, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, nil
	}
	mult := 1
	switch {
	case strings.HasSuffix(s, "kb"), strings.HasSuffix(s, "k"):
		mult = 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "b"), "k")
	case strings.HasSuffix(s, "mb"), strings.HasSuffix(s, "m"):
		mult = 1024 * 1024
		s = strings.TrimSuffix(strings.TrimSuffix(s, "b"), "m")
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("expected a positive size such as 4k, 64k or 1m")
	}
	return n * mult, nil
}

// tagIteration marks a result, and the results of what it ran first, as
// belonging to an iteration.
func tagIteration(res *runner.Result, it *runner.Iteration) {
	res.Iteration = it
	for _, d := range res.Deps {
		tagIteration(d, it)
	}
}
