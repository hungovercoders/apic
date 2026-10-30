package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/project"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/skills"
)

// SkillDirs are where `apic skill install` and `apic init` put the skills,
// relative to the project: the directory Claude Code reads, and the one
// the Agent Skills ecosystem (Codex, Cursor and the `skills` CLI) shares.
var SkillDirs = []string{filepath.Join(".claude", "skills"), filepath.Join(".agents", "skills")}

func (a *App) skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill [name]",
		Short: "Print an Agent Skill from the pack that briefs an AI agent on apic and on APIs",
		Long: `skill prints a skill's SKILL.md, the briefing an AI agent reads before
working on a project. The pack has three, each a directory of the apic
repository compiled into the binary:

  apic          how to run, write and test requests with apic: the validate,
                list, describe, run workflow, the --json shape and exit
                codes, the error codes to branch on, how to write a request
                and where secrets belong (the default)
  api-design    how to design the HTTP API those requests call: resources,
                methods and status codes, errors, pagination, versioning,
                idempotency and OpenAPI, each proved with an apic request
  api-security  how to keep that API safe and show it: the OWASP API
                Security Top 10 as checks, authentication, authorisation,
                input handling, headers and secrets, each a request or a
                scenario apic runs

An agent that can run commands can read one with "apic skill <name>";
"apic skill install" writes the pack into the project so agents that load
skills find it on their own.`,
		Example: `  apic skill                  # print SKILL.md of the apic skill
  apic skill api-design       # another skill of the pack
  apic skill --json           # every file of the skill, and the pack's names
  apic skill install          # the pack, into .claude/skills and .agents/skills of the project`,
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: skills.Names(),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := skills.Name
			if len(args) == 1 {
				name = args[0]
			}
			if !skills.Has(name) {
				return runner.Usage(runner.CodeFlag, fmt.Sprintf("unknown skill %q; the pack has %s", name, strings.Join(skills.Names(), ", ")))
			}
			files := skills.Files(name)
			if a.g.json {
				return a.writeJSON(struct {
					Name   string            `json:"name"`
					Files  map[string]string `json:"files"`
					Skills []string          `json:"skills"`
				}{name, files, skills.Names()})
			}
			_, err := fmt.Fprint(a.Stdout, files["SKILL.md"])
			return err
		},
	}
	cmd.AddCommand(a.skillInstallCmd())
	return cmd
}

func (a *App) skillInstallCmd() *cobra.Command {
	var into, only []string
	var user bool
	cmd := &cobra.Command{
		Use:   "install [dir]",
		Short: "Write the skill pack into a project, or your home directory, for the agents that load skills",
		Long: `install writes each skill of the pack into <dir>/.claude/skills/<name>
(Claude Code) and <dir>/.agents/skills/<name> (Codex, Cursor and the other
agents that share that directory), dir being the project (-C) unless
given; it must exist (apic init creates a project, and installs the pack
as it does). --user writes the same directories under your home directory
instead, where those agents read skills for every project, so one install
briefs them everywhere; it takes no dir and no --to. --skill picks some of
the pack instead of all of it. Files that already hold the same text are
left alone; others are overwritten, since the skills are apic's text
rather than the project's. Commit a project install so every clone briefs
its agents.`,
		Example: `  apic skill install
  apic skill install ./api
  apic skill install --user                   # once, for every project on this machine
  apic skill install --to .claude/skills      # Claude Code only
  apic skill install --skill apic             # only the apic skill`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := a.g.dir
			if len(args) == 1 {
				dir = args[0]
			}
			dirs := SkillDirs
			if len(into) > 0 {
				dirs = into
			}
			if user {
				if len(args) == 1 || len(into) > 0 {
					return runner.Usage(runner.CodeFlag, "--user installs under your home directory, so it takes no dir and no --to")
				}
				home, err := os.UserHomeDir()
				if err != nil {
					return runner.Usage(runner.CodeFile, fmt.Sprintf("--user: cannot find your home directory: %v", err))
				}
				dir = home
			}
			names := skills.Names()
			if len(only) > 0 {
				names = only
			}
			for _, name := range names {
				if !skills.Has(name) {
					return runner.Usage(runner.CodeFlag, fmt.Sprintf("--skill %q: unknown skill; the pack has %s", name, strings.Join(skills.Names(), ", ")))
				}
			}
			written, unchanged, _, err := writeSkill(dir, dirs, names, false, true)
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
	cmd.Flags().StringArrayVar(&into, "to", nil, "directory under the project to install into, instead of .claude/skills and .agents/skills (repeatable)")
	cmd.Flags().StringArrayVar(&only, "skill", nil, "a skill of the pack to install, instead of all of them: apic, api-design or api-security (repeatable)")
	cmd.Flags().BoolVar(&user, "user", false, "install into .claude/skills and .agents/skills of your home directory, for every project, instead of the project's")
	_ = cmd.RegisterFlagCompletionFunc("skill", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return skills.Names(), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

// writeSkill writes the named skills into each of dirs under root, as
// <dir>/<skill>/<file>. A file already holding the same text is reported
// as unchanged rather than rewritten, so a second install is a no-op; one
// holding other text is overwritten, or with overwrite false kept and
// reported as skipped, which is what `apic init` does to every existing
// file. create says whether a missing root is made (init) or an error
// (install, where it is a mistyped project). Every path is confined to
// root, symlinks followed, so neither `--to ../x` nor a link under
// .claude/skills can put the files outside the project.
func writeSkill(root string, dirs, names []string, create, overwrite bool) (written, unchanged, skipped []string, err error) {
	absRoot, err := writeRoot(root, create, "the skill")
	if err != nil {
		return nil, nil, nil, err
	}
	for _, dir := range dirs {
		for _, skill := range names {
			files := skills.Files(skill)
			for _, name := range skills.Paths(skill) {
				shown, target, err := confinedPath(root, absRoot, filepath.Join(dir, skill, filepath.FromSlash(name)), "the skill")
				if err != nil {
					return nil, nil, nil, err
				}
				content := []byte(files[name])
				if existing, err := os.ReadFile(target); err == nil { //nolint:gosec // the skill file this command wrote before, confined to the project above
					if bytes.Equal(existing, content) {
						unchanged = append(unchanged, shown)
						continue
					}
					if !overwrite {
						skipped = append(skipped, shown)
						continue
					}
				}
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // a skill directory the user's agents read and the user commits
					return nil, nil, nil, runner.Usage(runner.CodeFile, err.Error())
				}
				if err := os.WriteFile(target, content, 0o644); err != nil { //nolint:gosec // public text the user commits, confined to the project above
					return nil, nil, nil, runner.Usage(runner.CodeFile, err.Error())
				}
				written = append(written, shown)
			}
		}
	}
	return written, unchanged, skipped, nil
}

// writeRoot returns the absolute path of root, the directory the user
// named for the files a command writes: made when create is set (apic
// init), otherwise required to exist (install, where a missing one is a
// mistyped project). what names the files in the error. The path is
// absolute because Confine compares real paths: `apic init my-api` names
// the root relative to the working directory, and on macOS a temporary
// directory is a symlink.
func writeRoot(root string, create bool, what string) (string, error) {
	if create {
		if err := os.MkdirAll(root, 0o755); err != nil { //nolint:gosec // the project directory the user named, which they browse and commit
			return "", runner.Usage(runner.CodeFile, err.Error())
		}
	} else if info, err := os.Stat(root); err != nil || !info.IsDir() {
		return "", runner.Usage(runner.CodeFile, fmt.Sprintf("%s is not a directory; %s is written into an existing project (apic init creates one)", root, what))
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", runner.Usage(runner.CodeFile, err.Error())
	}
	return abs, nil
}

// confinedPath resolves rel, a path the user named under root, for a file
// a command writes: the path to report (root joined with rel, as the
// scaffold's are, or rel itself when it is absolute) and the real target,
// which must lie under absRoot, symlinks followed, so neither `--to ../x`
// nor a link under the root can put the file outside it. what names the
// files in the error.
func confinedPath(root, absRoot, rel, what string) (shown, target string, err error) {
	shown = rel
	if !filepath.IsAbs(rel) {
		shown = filepath.Join(root, rel)
	}
	target, err = project.Confine(absRoot, absRoot, rel)
	if errors.Is(err, project.ErrOutsideRoot) {
		return "", "", runner.Usage(runner.CodeFile, fmt.Sprintf("%s resolves outside %s; %s is written under it", shown, root, what))
	}
	if err != nil {
		return "", "", runner.Usage(runner.CodeFile, fmt.Sprintf("%s: %v", shown, err))
	}
	return shown, target, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
