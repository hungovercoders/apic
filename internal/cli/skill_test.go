package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The skill the binary carries is the one in skills/apic, so an agent that
// runs `apic skill` reads what the repository publishes.
func TestSkillPrintsTheEmbeddedSkill(t *testing.T) {
	code, out, errb := execute(t, "skill")
	if code != 0 || !strings.HasPrefix(out, "---\nname: apic\n") || !strings.Contains(out, "## Running requests") {
		t.Fatalf("code=%d err=%s out=%.80q", code, errb, out)
	}
	repo, err := os.ReadFile(filepath.Join("..", "..", "skills", "apic", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if out != strings.ReplaceAll(string(repo), "\r\n", "\n") {
		t.Fatal("apic skill differs from skills/apic/SKILL.md")
	}
	code, out, _ = execute(t, "--json", "skill")
	var got struct {
		Name  string            `json:"name"`
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v: %s", code, err, out)
	}
	if got.Name != "apic" || !strings.HasPrefix(got.Files["SKILL.md"], "---") || !strings.Contains(got.Files["references/cheatsheet.md"], "# apic cheat sheet") {
		t.Fatalf("json: %+v", got)
	}
}

func TestSkillInstallWritesOnceAndReportsUnchanged(t *testing.T) {
	dir := t.TempDir()
	code, out, errb := execute(t, "skill", "install", dir, "--no-color")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	for _, rel := range []string{".claude/skills/apic/SKILL.md", ".claude/skills/apic/references/cheatsheet.md", ".agents/skills/apic/SKILL.md", ".agents/skills/apic/references/cheatsheet.md"} {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s not written: %v", rel, err)
		}
		if !strings.Contains(out, "wrote "+path) {
			t.Errorf("output lacks wrote %s:\n%s", path, out)
		}
	}
	// A second install finds the same text and changes nothing; an edited
	// copy is put back, since the text is apic's.
	mustWrite(t, filepath.Join(dir, ".agents", "skills", "apic", "SKILL.md"), "edited")
	code, out, _ = execute(t, "--json", "skill", "install", dir)
	var got struct {
		Written   []string `json:"written"`
		Unchanged []string `json:"unchanged"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v: %s", code, err, out)
	}
	if len(got.Written) != 1 || !strings.HasSuffix(got.Written[0], filepath.FromSlash(".agents/skills/apic/SKILL.md")) || len(got.Unchanged) != 3 {
		t.Fatalf("second install: %+v", got)
	}
	// --to picks the directories; -C is the default project.
	only := t.TempDir()
	if code, _, errb = execute(t, "-C", only, "skill", "install", "--to", "tools/skills"); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(only, "tools", "skills", "apic", "SKILL.md")); err != nil {
		t.Error(err)
	}
	if _, err := os.Stat(filepath.Join(only, ".claude")); !os.IsNotExist(err) {
		t.Error("--to should replace the default directories, not add to them")
	}
}

// apic init briefs the project's agents by default, so a scaffolded
// project is agent-ready without a second command.
func TestInitWritesTheSkillUnlessAskedNotTo(t *testing.T) {
	dir := t.TempDir()
	if code, out, errb := execute(t, "init", dir, "--no-color"); code != 0 || !strings.Contains(out, filepath.Join(dir, ".claude", "skills", "apic", "SKILL.md")) {
		t.Fatalf("code=%d out=%s err=%s", code, out, errb)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "apic", "references", "cheatsheet.md")); err != nil {
		t.Error(err)
	}
	// The skill is refreshed on a second init even without --force: the
	// user's files are kept, apic's text is not the user's.
	mustWrite(t, filepath.Join(dir, ".claude", "skills", "apic", "SKILL.md"), "old")
	if code, out, _ := execute(t, "--json", "init", dir); code != 0 || !strings.Contains(out, `.claude`) || !strings.Contains(out, `"skipped"`) {
		t.Fatalf("second init: code=%d out=%s", code, out)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apic", "SKILL.md")); string(got) == "old" {
		t.Error("an old skill copy should be refreshed")
	}
	bare := t.TempDir()
	if code, _, errb := execute(t, "init", bare, "--no-skill"); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(bare, ".claude")); !os.IsNotExist(err) {
		t.Error("--no-skill should write no skill")
	}
}

// The root help tells an agent where the briefing is.
func TestRootHelpPointsAgentsAtTheSkill(t *testing.T) {
	if _, out, _ := execute(t, "--help"); !strings.Contains(out, "apic skill prints the briefing") {
		t.Fatalf("root help:\n%s", out)
	}
}
