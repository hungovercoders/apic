package cli

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/openapi"
	"github.com/hungovercoders/apic/internal/runner"
)

func (a *App) mockCmd() *cobra.Command {
	var port int
	cmd := &cobra.Command{
		Use:   "mock <openapi.yaml|openapi.json>",
		Short: "Serve an OpenAPI 3 document as a mock API, so requests can run before the backend exists",
		Long: `mock reads an OpenAPI 3 document and serves every operation in it with the
response the document declares: the first 2xx (else default, else the first),
a body from its example or built from its schema, and its headers. It keeps
no state and validates nothing: it is the document read back, so the
requests apic import wrote from the same document run green against it
before the API exists.

A Prefer header line on a request picks another documented response:
Prefer: code=404 serves that response (its class, or default, when the exact
code is not declared), and Prefer: example=name one of a media type's named
examples. A choice the document does not offer is a 406 with the reason.

The paths are served as written and, when the first server URL has a path
such as /v1, under that prefix too. The server listens on localhost only.`,
		Example: `  apic mock openapi.yaml                       # terminal 1
  apic import openapi.yaml -o api              # terminal 2: the requests it describes
  apic run list-pets -C api --var baseUrl=http://localhost:8090`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := os.ReadFile(args[0]) //nolint:gosec // the OpenAPI document the user named on the command line
			if err != nil {
				return runner.Usage(runner.CodeFile, err.Error())
			}
			m, err := openapi.NewMock(data)
			if err != nil {
				return runner.Usagef(runner.CodeMock, "%s: %v", args[0], err)
			}
			ln, err := listenLocal(port)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()
			url := fmt.Sprintf("http://localhost:%d", port)
			if a.g.json {
				if err := a.writeJSON(struct {
					Spec      string          `json:"spec"`
					URL       string          `json:"url"`
					Routes    []openapi.Route `json:"routes"`
					Skipped   []string        `json:"skipped,omitempty"`
					Listening bool            `json:"listening"`
				}{Spec: args[0], URL: url, Routes: m.Routes, Skipped: m.Skipped, Listening: true}); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(a.Stdout, "mock api listening on %s · %s from %s\n", url, plural(len(m.Routes), "route"), args[0])
				tw := tabwriter.NewWriter(a.Stdout, 0, 0, 2, ' ', 0)
				for _, r := range m.Routes {
					fmt.Fprintf(tw, "  %s\t%s\t%d\n", r.Method, r.Path, r.Status)
				}
				_ = tw.Flush()
				for _, s := range m.Skipped {
					fmt.Fprintf(a.Stdout, "skipped %s\n", s)
				}
				fmt.Fprintf(a.Stdout, "try: apic run <request> --var baseUrl=%s\n", url)
			}
			return serveLocal(cmd.Context(), ln, m)
		},
	}
	cmd.Flags().IntVar(&port, "port", 8090, "port to serve the mock API on")
	return cmd
}
