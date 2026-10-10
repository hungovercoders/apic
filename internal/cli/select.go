// apic select: a value from a request's last recorded response, without
// sending the request again.

package cli

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
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
			t, err := a.historyFor(args[0])
			if err != nil {
				return err
			}
			e, err := t.store.Get(t.env, t.name, entry)
			if err != nil {
				if hint := historyOff(t.project); hint != "" {
					return runner.Usage(runner.CodeSession, fmt.Sprintf("%s has no recorded response in %s: %s, then run it once", t.name, t.env, hint))
				}
				return historyErr(err)
			}
			resp, err := responseOf(e)
			if err != nil {
				return runner.Usage(runner.CodeSession, fmt.Sprintf("history entry %d of %s: %v", entry, t.name, err))
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
	cmd.ValidArgsFunction = a.completeRequests
	return cmd
}

// responseOf rebuilds the response a history entry recorded, from the
// `apic run --json` object it holds, for selectors to read.
func responseOf(e history.Entry) (*selector.Response, error) {
	var stored struct {
		Response *struct {
			Status       int               `json:"status"`
			StatusText   string            `json:"status_text"`
			Headers      map[string]string `json:"headers"`
			Body         json.RawMessage   `json:"body"`
			BodyEncoding string            `json:"body_encoding"`
			DurationMs   int64             `json:"duration_ms"`
		} `json:"response"`
	}
	if err := json.Unmarshal(e.Result, &stored); err != nil {
		return nil, err
	}
	if stored.Response == nil {
		return nil, fmt.Errorf("the request got no response")
	}
	r := stored.Response
	headers := http.Header{}
	for k, v := range r.Headers {
		headers.Set(k, v)
	}
	// The body was stored as parsed JSON, as a string, or as base64 for
	// bytes that are not text.
	body := []byte(r.Body)
	var text string
	switch {
	case r.BodyEncoding == runner.Base64 && json.Unmarshal(r.Body, &text) == nil:
		decoded, err := base64.StdEncoding.DecodeString(text)
		if err != nil {
			return nil, err
		}
		body = decoded
	case len(r.Body) > 0 && r.Body[0] == '"' && json.Unmarshal(r.Body, &text) == nil:
		body = []byte(text)
	}
	return &selector.Response{Status: r.Status, StatusText: r.StatusText, Headers: headers, Body: body, Duration: time.Duration(r.DurationMs) * time.Millisecond}, nil
}
