package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/hungovercoders/apic/internal/project"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/skills"
)

// SkillDirs are where `apic skill install` and `apic init` put the skill,
// relative to the project: the directory Claude Code reads, and the one
// the Agent Skills ecosystem (Codex, Cursor and the `skills` CLI) shares.
var SkillDirs = []string{filepath.Join(".claude", "skills"), filepath.Join(".agents", "skills")}

func (a *App) skillCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Print the Agent Skill that briefs an AI agent on apic",
		Long: `skill prints SKILL.md, the briefing an AI agent reads before working with
a project's .http files: the validate, list, describe, run workflow, the
--json shape and exit codes, the error codes to branch on, how to write a
request and where secrets belong. It is the same skill as skills/apic in
the apic repository, compiled into the binary.

An agent that can run commands can read it with "apic skill"; "apic skill
install" writes it into the project so agents that load skills find it on
their own.`,
		Example: `  apic skill                  # print SKILL.md
  apic skill --json           # every file of the skill
  apic skill install          # into .claude/skills and .agents/skills of the project`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			files := skills.Files()
			if a.g.json {
				return a.writeJSON(struct {
					Name  string            `json:"name"`
					Files map[string]string `json:"files"`
				}{skills.Name, files})
			}
			_, err := fmt.Fprint(a.Stdout, files["SKILL.md"])
			return err
		},
	}
	cmd.AddCommand(a.skillInstallCmd())
	return cmd
}

func (a *App) skillInstallCmd() *cobra.Command {
	var into []string
	cmd := &cobra.Command{
		Use:   "install [dir]",
		Short: "Write the skill into a project for the agents that load skills",
		Long: `install writes the skill's files into <dir>/.claude/skills/apic (Claude
Code) and <dir>/.agents/skills/apic (Codex, Cursor and the other agents
that share that directory), dir being the project (-C) unless given.
Files that already hold the same text are left alone; others are
overwritten, since the skill is apic's text rather than the project's.
Commit the result so every clone briefs its agents.`,
		Example: `  apic skill install
  apic skill install ./api
  apic skill install --to .claude/skills      # Claude Code only`,
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
			written, unchanged, err := writeSkill(dir, dirs)
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
	return cmd
}

// writeSkill writes the skill into each of dirs under root, as
// <dir>/apic/<file>. A file already holding the same text is reported as
// unchanged rather than rewritten, so a second install is a no-op. Every
// path is confined to root, symlinks followed, so neither `--to ../x` nor
// a link under .claude/skills can put the files outside the project.
func writeSkill(root string, dirs []string) (written, unchanged []string, err error) {
	if err := os.MkdirAll(root, 0o755); err != nil { //nolint:gosec // the project directory the user named, which they browse and commit
		return nil, nil, runner.Usage(runner.CodeFile, err.Error())
	}
	// Confine compares real, absolute paths: `apic init my-api` names the
	// root relative to the working directory, and on macOS a temporary
	// directory is a symlink. The paths reported are the ones the user
	// named, joined as the scaffold's are, not the resolved ones.
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, nil, runner.Usage(runner.CodeFile, err.Error())
	}
	files := skills.Files()
	for _, dir := range dirs {
		for _, name := range skills.Paths() {
			rel := filepath.Join(dir, skills.Name, filepath.FromSlash(name))
			shown := filepath.Join(root, rel)
			target, err := project.Confine(absRoot, absRoot, rel)
			if errors.Is(err, project.ErrOutsideRoot) {
				return nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s resolves outside %s; the skill is installed under the project", shown, root))
			}
			if err != nil {
				return nil, nil, runner.Usage(runner.CodeFile, fmt.Sprintf("%s: %v", shown, err))
			}
			content := []byte(files[name])
			if existing, err := os.ReadFile(target); err == nil && bytes.Equal(existing, content) { //nolint:gosec // the skill file this command wrote before, confined to the project above
				unchanged = append(unchanged, shown)
				continue
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil { //nolint:gosec // a skill directory the user's agents read and the user commits
				return nil, nil, runner.Usage(runner.CodeFile, err.Error())
			}
			if err := os.WriteFile(target, content, 0o644); err != nil { //nolint:gosec // public text the user commits, confined to the project above
				return nil, nil, runner.Usage(runner.CodeFile, err.Error())
			}
			written = append(written, shown)
		}
	}
	return written, unchanged, nil
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
