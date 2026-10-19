// apic list: every request in the project, filtered by a pattern.

package cli

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/httpfile"
)

type listEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name,omitempty"`
	Method      string   `json:"method"`
	URL         string   `json:"url"`
	File        string   `json:"file"`
	Line        int      `json:"line"`
	Description string   `json:"description,omitempty"`
	Captures    []string `json:"captures,omitempty"`
	Asserts     int      `json:"asserts,omitempty"`
	Steps       []string `json:"steps,omitempty"`
	Refs        []string `json:"refs,omitempty"`
	Disabled    bool     `json:"disabled,omitempty"`
}

func (e listEntry) matches(pattern string) bool {
	p := strings.ToLower(pattern)
	return strings.Contains(strings.ToLower(e.ID), p) ||
		strings.Contains(strings.ToLower(e.URL), p) ||
		strings.Contains(strings.ToLower(e.Description), p) ||
		strings.Contains(strings.ToLower(e.File), p)
}

func (a *App) listCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list [pattern]",
		Short: "List every request in the project",
		Long: `List the requests apic found, grouped by file. A pattern narrows the list
to requests whose id, URL, file or description contains it.`,
		Example: `  apic list
  apic list todo
  apic list --json | jq '.requests[].id'`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := a.loadProject()
			if err != nil {
				return err
			}
			var entries []listEntry
			for _, r := range p.Requests() {
				e := listEntry{ID: r.ID(), Name: r.Name, Method: r.Method, URL: r.URL, File: r.File.Path, Line: r.Line, Description: r.Description, Asserts: len(r.Asserts), Steps: r.Steps(), Refs: refIDs(r), Disabled: r.Disabled()}
				for _, c := range r.Captures {
					e.Captures = append(e.Captures, c.Name)
				}
				if len(args) == 1 && !e.matches(args[0]) {
					continue
				}
				entries = append(entries, e)
			}
			if a.g.json {
				if entries == nil {
					entries = []listEntry{}
				}
				hint := ""
				if len(p.Files) == 0 {
					hint = newHereHint
				}
				return a.writeJSON(struct {
					Root     string      `json:"root"`
					Requests []listEntry `json:"requests"`
					Hint     string      `json:"hint,omitempty"`
				}{p.Root, entries, hint})
			}
			if len(entries) == 0 {
				if len(args) == 1 {
					fmt.Fprintf(a.Stdout, "no requests match %q (run `apic list` to see them all)\n", args[0])
					return nil
				}
				fmt.Fprintf(a.Stdout, "no .http files found under %s\n", p.Root)
				if len(p.Files) == 0 {
					fmt.Fprintln(a.Stdout, newHereHint)
				}
				return nil
			}
			files := map[string]bool{}
			for _, e := range entries {
				files[e.File] = true
			}
			hasSteps := false
			for _, e := range entries {
				if len(e.Steps) > 0 {
					hasSteps = true
				}
			}
			cols := []string{"ID", "METHOD", "URL", "LINE", "DESCRIPTION"}
			if len(files) == 1 {
				cols[3] = "FILE"
			}
			if hasSteps {
				cols = append(cols, "PHRASES")
			}
			for i, c := range cols {
				cols[i] = theme.Bold.Render(c)
			}
			rows := [][]string{cols}
			lastFile := ""
			for _, e := range entries {
				if len(files) > 1 && e.File != lastFile {
					rows = append(rows, []string{theme.Accent.Render(e.File)})
					lastFile = e.File
				}
				where := fmt.Sprintf("%s:%d", e.File, e.Line)
				if len(files) > 1 {
					where = fmt.Sprintf(":%d", e.Line)
				}
				id, desc := theme.Bold.Render(e.ID), e.Description
				if e.Disabled {
					id, desc = theme.Dim.Render(e.ID), strings.TrimSpace(theme.Dim.Render("(disabled)")+" "+desc)
				}
				row := []string{id, theme.Method(e.Method), theme.URL.Render(e.URL), theme.Dim.Render(where), desc}
				if hasSteps {
					row = append(row, theme.Dim.Render(strings.Join(e.Steps, " | ")))
				}
				rows = append(rows, row)
			}
			if _, err := fmt.Fprint(a.Stdout, table(rows)); err != nil {
				return err
			}
			fmt.Fprintf(a.Stdout, "\n%s\n", theme.Dim.Render(fmt.Sprintf("%s in %s · apic describe <id> · apic run <id> · apic ui",
				plural(len(entries), "request"), plural(len(files), "file"))))
			return nil
		},
	}
	return cmd
}

// refIDs lists the targets of a request's `# @ref` and `# @forceRef` directives.
func refIDs(r *httpfile.Request) []string {
	var out []string
	for _, ref := range r.Refs() {
		out = append(out, ref.ID)
	}
	return out
}

// newHereHint is what a directory with no request files gets told: the two
// commands that give it some. The project's not-found error says the same.
const newHereHint = "new here? apic init scaffolds a project, and apic ui --demo shows a working one"

// ansiSeq is a colour or style sequence the theme writes.
var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;]*m")

// table lays rows out in columns two spaces apart, sized by what shows on
// the terminal rather than by the bytes, so a bold header lines up with
// the coloured rows beneath it. A short row (a file heading) takes part
// in the sizing without padding, and no line carries trailing spaces,
// which would wrap in a terminal only just wide enough.
func table(rows [][]string) string {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i >= len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], shown(cell))
		}
	}
	var b strings.Builder
	for _, row := range rows {
		// A trailing cell that shows nothing (an empty phrase, styled)
		// would keep the padding before it on the line.
		last := len(row) - 1
		for last > 0 && shown(row[last]) == 0 {
			last--
		}
		for i, cell := range row[:last+1] {
			b.WriteString(cell)
			if i < last {
				b.WriteString(strings.Repeat(" ", widths[i]-shown(cell)+2))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// shown is how many cells of the terminal a styled string takes.
func shown(s string) int {
	return utf8.RuneCountInString(ansiSeq.ReplaceAllString(s, ""))
}
