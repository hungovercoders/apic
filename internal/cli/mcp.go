package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/mcp"
	"github.com/hungovercoders/apic/internal/runner"
)

func (a *App) mcpCmd() *cobra.Command {
	var httpAddr, token string
	cmd := &cobra.Command{
		Use:   "mcp [--http <host:port> [--token <bearer>]]",
		Short: "Serve the project's requests to AI agents over MCP (stdio, or HTTP)",
		Long: `Start a Model Context Protocol server on stdin/stdout exposing the tools
list_requests, describe_request, run_request, run_file, run_features,
list_environments, clear_session, validate_project and curl_request, plus
every .http file as a resource.

Register it with your agent, for example:
  claude mcp add api -- apic mcp --dir ./api --env dev

--http serves the streamable HTTP transport instead, for an agent on
another machine or in a container. Bind the loopback interface, or set
--token (or APIC_MCP_TOKEN): the server runs the project's requests with
the project's credentials, and refuses to listen on any other interface
without a bearer token guarding it.`,
		Example: `  apic mcp --dir ./api --env dev
  apic mcp --http 127.0.0.1:8765
  APIC_MCP_TOKEN=$(openssl rand -hex 16) apic mcp --http 0.0.0.0:8765`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg := mcp.Config{Dir: a.g.dir, Env: a.g.env, Version: Version}
			if httpAddr != "" {
				if token == "" {
					token = os.Getenv("APIC_MCP_TOKEN")
				}
				err := mcp.ServeHTTP(cmd.Context(), cfg, httpAddr, token, func(bound string) {
					guard := "no token: loopback only"
					if token != "" {
						guard = "bearer token required"
					}
					fmt.Fprintf(a.Stderr, "apic mcp listening on http://%s (%s)\n", bound, guard)
				})
				if err != nil {
					return runner.Usage(runner.CodeServer, err.Error())
				}
				return nil
			}
			err := mcp.Serve(cmd.Context(), cfg)
			switch {
			case errors.Is(err, context.Canceled):
				return nil // Ctrl-C or SIGTERM: the server was told to stop
			case errors.Is(err, io.EOF) || (err != nil && strings.HasSuffix(err.Error(), "EOF")):
				return nil // client closed the pipe: normal shutdown
			}
			return err
		},
	}
	cmd.Flags().StringVar(&httpAddr, "http", "", "serve the streamable HTTP transport on this host:port instead of stdio")
	cmd.Flags().StringVar(&token, "token", "", "bearer token clients must send (default $APIC_MCP_TOKEN); required off the loopback interface")
	cmd.AddCommand(a.mcpInstallCmd())
	return cmd
}

// mcpConfigFile is the project-scoped MCP configuration Claude Code reads
// from the directory it starts in, in the {"mcpServers": {…}} shape Cursor
// (.cursor/mcp.json), Windsurf and the other clients accept too. `apic mcp
// install` and `apic init` register the server there under mcpServerName,
// the name the docs' `claude mcp add api` example uses.
const (
	mcpConfigFile = ".mcp.json"
	mcpServerName = "api"
)

func (a *App) mcpInstallCmd() *cobra.Command {
	var into []string
	cmd := &cobra.Command{
		Use:   "install [dir]",
		Short: "Register the server in the project's .mcp.json for the agents that read it",
		Long: `install writes an entry that runs "apic mcp" into <dir>/.mcp.json, the
project-scoped MCP configuration Claude Code reads from the directory it
starts in; dir is the current directory unless given. The entry names the
project (-C) relative to dir when the two differ, and the environment
(--env) when one is given, so an agent gets the same tools "claude mcp
add" would register, from a file every clone of the repository shares.
Other servers in the file are kept; an entry already named "api" is
replaced, and one already holding the same text is left alone. Cursor
reads the same shape from .cursor/mcp.json: --to names the file. Commit
the result.`,
		Example: `  apic mcp install                          # .mcp.json in the current directory
  apic mcp install -C api --env dev         # the entry runs apic mcp --dir api --env dev
  apic mcp install --to .cursor/mcp.json    # Cursor`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			files := []string{mcpConfigFile}
			if len(into) > 0 {
				files = into
			}
			serverArgs, err := mcpServerArgs(dir, a.g.dir, a.g.env)
			if err != nil {
				return err
			}
			written, unchanged, _, err := writeMCPConfig(dir, files, serverArgs, true)
			if err != nil {
				return err
			}
			if a.g.json {
				return a.writeJSON(struct {
					Written   []string `json:"written"`
					Unchanged []string `json:"unchanged"`
				}{orEmpty(written), orEmpty(unchanged)})
			}
			for _, f := range written {
				fmt.Fprintf(a.Stdout, "%s %s\n", theme.OK.Render("wrote"), f)
			}
			for _, f := range unchanged {
				fmt.Fprintf(a.Stdout, "%s %s\n", theme.Dim.Render("unchanged"), f)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVar(&into, "to", nil, "file under dir to write instead of .mcp.json (repeatable)")
	return cmd
}

// mcpServerArgs is the argument list the registered entry runs apic with:
// mcp, then --dir with the project as a path relative to the directory the
// configuration lives in (where the agent starts) when the two differ, in
// forward slashes so the committed file works on every platform, then
// --env when one is set.
func mcpServerArgs(configDir, projectDir, env string) ([]string, error) {
	args := []string{"mcp"}
	absConfig, err := filepath.Abs(configDir)
	if err != nil {
		return nil, runner.Usage(runner.CodeFile, err.Error())
	}
	absProject, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, runner.Usage(runner.CodeFile, err.Error())
	}
	rel, err := filepath.Rel(absConfig, absProject)
	if err != nil {
		rel = absProject // another drive: only an absolute path reaches it
	}
	if rel != "." {
		args = append(args, "--dir", filepath.ToSlash(rel))
	}
	if env != "" {
		args = append(args, "--env", env)
	}
	return args, nil
}

// writeMCPConfig puts the server entry into each of files under dir,
// merging with what a file holds: its other servers and top-level keys
// stay and the apic entry is set. A file already holding the same entry
// is reported as unchanged; one holding a different apic entry is
// overwritten, or with overwrite false kept and reported as skipped, as
// `apic init` keeps every file a user may have tailored. An empty file is
// an empty document; one that is not a JSON object, or whose mcpServers
// is not one, is an error rather than overwritten. Every path is confined
// to dir, as the skill's are to the project.
func writeMCPConfig(dir string, files, serverArgs []string, overwrite bool) (written, unchanged, skipped []string, err error) {
	absDir, err := writeRoot(dir, false, "the configuration is written into an existing project (apic init creates one)")
	if err != nil {
		return nil, nil, nil, err
	}
	entry := map[string]any{"command": "apic", "args": serverArgs}
	for _, name := range files {
		shown, target, err := confinedPath(dir, absDir, name, "the configuration")
		if err != nil {
			return nil, nil, nil, err
		}
		doc := map[string]any{}
		existing, err := os.ReadFile(target) //nolint:gosec // the MCP configuration the user's agent reads, confined to the directory above
		switch {
		case err == nil && len(bytes.TrimSpace(existing)) > 0:
			if err := json.Unmarshal(existing, &doc); err != nil {
				return nil, nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s is not a JSON object: %v", shown, err))
			}
			if doc == nil {
				return nil, nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s is not a JSON object: it holds null", shown))
			}
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return nil, nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s: %v", shown, err))
		}
		var servers map[string]any
		switch v := doc["mcpServers"].(type) {
		case nil:
			servers = map[string]any{}
		case map[string]any:
			servers = v
		default:
			return nil, nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s: mcpServers is not an object, so the entry has nowhere to go; nothing was written", shown))
		}
		if sameJSON(servers[mcpServerName], entry) {
			unchanged = append(unchanged, shown)
			continue
		}
		if _, exists := servers[mcpServerName]; exists && !overwrite {
			skipped = append(skipped, shown)
			continue
		}
		servers[mcpServerName] = entry
		doc["mcpServers"] = servers
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(doc); err != nil {
			return nil, nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s: %v", shown, err))
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // a configuration directory the user's agents read and the user commits
			return nil, nil, nil, runner.Usage(runner.CodeFile, err.Error())
		}
		if err := os.WriteFile(target, buf.Bytes(), 0o644); err != nil { //nolint:gosec // public project configuration the user commits, confined to the directory above
			return nil, nil, nil, runner.Usage(runner.CodeFile, err.Error())
		}
		written = append(written, shown)
	}
	return written, unchanged, skipped, nil
}

// sameJSON reports whether two values encode to the same JSON, which is
// how an entry read back from a file ([]any) is compared with the one
// about to be written ([]string).
func sameJSON(a, b any) bool {
	x, errA := json.Marshal(a)
	y, errB := json.Marshal(b)
	return errA == nil && errB == nil && bytes.Equal(x, y)
}
