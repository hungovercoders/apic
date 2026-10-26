package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/demoapi"
)

func (a *App) demoCmd() *cobra.Command {
	var out string
	var port int
	var force bool
	cmd := &cobra.Command{
		Use:   "demo",
		Short: "Scaffold and serve a fake API, so apic can be tried with no setup",
		Long: `demo writes a small example .http project (login, whoami, and a todos
CRUD resource) into --out, then serves the fake API those requests target.
No network access or git clone needed. Existing files are left alone
unless --force is given.

Run it, then use the project from another terminal (the examples below;
substitute your --out if you set one).

Every apic command, including ui, resolves .http files relative to -C
(default: the current directory) — apic ui on its own won't find the
project this command just wrote unless you cd into it or pass -C.`,
		Example: `  apic demo                                  # terminal 1: writes ./apic-demo, serves the API
  apic run login whoami -C apic-demo         # terminal 2
  apic ui -C apic-demo --env local           # or the UI, same directory`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// Bind first: a port already in use fails here, before anything is
			// written or "listening" is printed.
			ln, err := listenLocal(port)
			if err != nil {
				return err
			}
			defer func() { _ = ln.Close() }()
			written, skipped, err := demoapi.WriteProject(out, port, force)
			if err != nil {
				return err
			}
			url := fmt.Sprintf("http://localhost:%d", port)
			if a.g.json {
				if err := a.writeJSON(struct {
					Out       string   `json:"out"`
					URL       string   `json:"url"`
					Written   []string `json:"written"`
					Skipped   []string `json:"skipped,omitempty"`
					Listening bool     `json:"listening"`
				}{
					Out:       out,
					URL:       url,
					Written:   written,
					Skipped:   skipped,
					Listening: true,
				}); err != nil {
					return err
				}
			} else {
				for _, f := range written {
					fmt.Fprintf(a.Stdout, "wrote %s\n", f)
				}
				for _, f := range skipped {
					fmt.Fprintf(a.Stdout, "kept  %s (use --force to overwrite)\n", f)
				}
				fmt.Fprintf(a.Stdout, "demo api listening on %s\n", url)
				fmt.Fprintf(a.Stdout, "try: apic run login whoami -C %s --env local\n", out)
			}
			return serveLocal(cmd.Context(), ln, demoapi.New())
		},
	}
	cmd.Flags().StringVarP(&out, "out", "o", "apic-demo", "directory to write the example project into")
	cmd.Flags().IntVar(&port, "port", 8089, "port to serve the demo API on")
	cmd.Flags().BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}
