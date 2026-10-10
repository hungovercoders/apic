// apic select: a value from a request's last recorded response, without
// sending the request again.

package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/history"
	"github.com/hungovercoders/apic/internal/output"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/internal/selector"
)

func (a *App) selectCmd() *cobra.Command {
	var entry int
	cmd := &cobra.Command{
		Use:   "select <request> <selector>",
		Short: "Read a value from a request's last recorded response, without sending it again",
		Long: `select evaluates a selector, the kind "# @assert" and "# @capture" take,
against the newest response recorded for a request in the current
environment (or entry N with --entry, 1 being the newest), and prints the
value. Nothing is sent: it is for looking at a response again, or at the
rest of a body that apic run --body-limit cut, without repeating a call
that may have changed something.

It reads the response history, so apic.yaml needs history: N (apic init
and apic demo set history: 10). Sensitive response headers are stored
masked, so header.set-cookie and cookie.<name> read as ***.`,
		Example: `  apic select list-todos 'body.$.items[0].id'
  apic select list-todos 'body.$.items.#'
  apic select login status
  apic select login body --entry 2 --json`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if entry < 1 {
				return runner.Usage(runner.CodeFlag, "--entry counts from 1, the newest")
			}
			sel := args[1]
			if err := selector.Check(sel); err != nil {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("selector %q: %v", sel, err))
			}
			if reHeaderMulti.MatchString(strings.TrimSpace(sel)) {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("selector %q: the history keeps one value per header, joined with commas, so header.<name>.# and header.<name>[n] cannot be read from it; use header.<name>", sel))
			}
			t, err := a.historyFor(args[0])
			if err != nil {
				return err
			}
			e, err := t.store.Get(t.env, t.name, entry)
			if err != nil {
				// Nothing recorded at all, with history off, is the one
				// case the hint helps; an entry past a history that has
				// some is a bad argument like any other.
				var re *history.RangeError
				if errors.As(err, &re) && re.Have == 0 {
					if hint := historyOff(t.project); hint != "" {
						return runner.Usage(runner.CodeSession, fmt.Sprintf("%s has no recorded response in %s: %s, then run it once", t.name, t.env, hint))
					}
				}
				return historyErr(err)
			}
			res, err := runner.ParseResult(e.Result)
			if err != nil {
				return runner.Usage(runner.CodeSession, fmt.Sprintf("history entry %d of %s: %v", entry, t.name, err))
			}
			resp := res.Raw()
			if resp == nil {
				return runner.Usage(runner.CodeSession, fmt.Sprintf("history entry %d of %s: the request got no response", entry, t.name))
			}
			if string(resp.Body) == runner.Masked {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("history entry %d of %s was recorded by a --redact run: its body and header values are masked", entry, t.name))
			}
			value, found, err := selector.SelectValue(resp, sel)
			if err != nil {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("selector %q: %v", sel, err))
			}
			if a.g.json {
				var v any
				if found {
					v = value.Text
					if value.Kind != selector.KindString {
						// A number, boolean, null, array or object as itself.
						var parsed any
						if json.Unmarshal([]byte(value.Text), &parsed) == nil {
							v = parsed
						}
					}
				}
				if err := a.writeJSON(struct {
					Request  string    `json:"request"`
					Env      string    `json:"env"`
					Entry    int       `json:"entry"`
					Time     time.Time `json:"time"`
					Selector string    `json:"selector"`
					Found    bool      `json:"found"`
					Value    any       `json:"value"`
				}{t.name, t.env, e.Index, e.Time, sel, found, v}); err != nil {
					return err
				}
				if !found {
					return &exitError{code: runner.ExitAssert}
				}
				return nil
			}
			if !found {
				return &exitError{code: runner.ExitAssert, msg: fmt.Sprintf("nothing at %s in %s's response of %s", sel, t.name, output.HistoryTime(e.Time))}
			}
			text := value.Text
			if value.Kind == selector.KindObject || value.Kind == selector.KindArray {
				var buf bytes.Buffer
				if json.Indent(&buf, []byte(text), "", "  ") == nil {
					text = buf.String()
				}
			}
			fmt.Fprintln(a.Stdout, text)
			return nil
		},
	}
	cmd.Flags().IntVar(&entry, "entry", 1, "which recorded response, 1 being the newest")
	cmd.ValidArgsFunction = func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp // the selector is typed, not picked
		}
		return a.completeRequests(cmd, args, toComplete)
	}
	return cmd
}

// reHeaderMulti matches the header selectors that need a header's
// separate values, which a history entry does not keep: it holds one
// value per header, joined with commas as `apic run --json` prints them,
// and that is what header.<name> reads.
var reHeaderMulti = regexp.MustCompile(`^headers?\.[^.\[\]]+(\.#|\[-?\d+\])$`)
