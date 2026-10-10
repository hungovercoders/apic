package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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
	// An edited copy is kept on a second init, like every other file, and
	// reported as skipped; --force replaces it, as `apic skill install`
	// always does.
	edited := filepath.Join(dir, ".claude", "skills", "apic", "SKILL.md")
	mustWrite(t, edited, "tailored")
	code, out, _ := execute(t, "--json", "init", dir)
	var got struct{ Written, Skipped []string }
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("second init: code=%d %v: %s", code, err, out)
	}
	if len(got.Written) != 0 || !slices.Contains(got.Skipped, edited) {
		t.Errorf("second init: %+v", got)
	}
	if b, _ := os.ReadFile(edited); string(b) != "tailored" {
		t.Error("init overwrote an edited skill file without --force")
	}
	if code, _, errb := execute(t, "init", dir, "--force"); code != 0 {
		t.Fatalf("--force: code=%d err=%s", code, errb)
	}
	if b, _ := os.ReadFile(edited); string(b) == "tailored" {
		t.Error("--force should replace the edited skill file")
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

// The install stays under the project: a --to that climbs out, or a
// symlink in the way, is refused rather than followed. A project named
// relative to the working directory, as `apic init my-api` does, is still
// inside it.
func TestSkillInstallStaysUnderTheProject(t *testing.T) {
	t.Chdir(t.TempDir())
	if code, _, errb := execute(t, "init", "my-api"); code != 0 {
		t.Fatalf("relative project: code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join("my-api", ".agents", "skills", "apic", "SKILL.md")); err != nil {
		t.Error(err)
	}
	dir := t.TempDir()
	code, _, errb := execute(t, "skill", "install", dir, "--to", "../outside")
	if code != 2 || !strings.Contains(errb, "resolves outside") {
		t.Fatalf("--to ../outside: code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(dir), "outside")); !os.IsNotExist(err) {
		t.Error("something was written outside the project")
	}
	// A mistyped project is an error, not a new directory tree.
	typo := filepath.Join(dir, "aip")
	if code, _, errb = execute(t, "skill", "install", typo); code != 2 || !strings.Contains(errb, "not a directory") {
		t.Errorf("missing project: code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(typo); !os.IsNotExist(err) {
		t.Error("a missing project directory was created")
	}
	elsewhere := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(dir, ".claude", "skills")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	code, _, errb = execute(t, "skill", "install", dir)
	if code != 2 || !strings.Contains(errb, "resolves outside") {
		t.Fatalf("symlinked skills dir: code=%d err=%s", code, errb)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Error("the symlink was followed out of the project")
	}
}

// The paths reported are the ones the user named, joined like the
// scaffold's, not the resolved ones: on macOS a temporary directory is a
// symlink into /private, and `apic init --json` must list one shape.
func TestSkillInstallReportsPathsAsNamed(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.MkdirAll("real", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real", "link"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	code, out, errb := execute(t, "--json", "skill", "install", "link")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	var got struct{ Written []string }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Written) != 4 || got.Written[0] != filepath.Join("link", ".claude", "skills", "apic", "SKILL.md") {
		t.Errorf("written: %v", got.Written)
	}
	if _, err := os.Stat(filepath.Join("real", ".agents", "skills", "apic", "SKILL.md")); err != nil {
		t.Error("the files should land under the real directory the link names")
	}
}
