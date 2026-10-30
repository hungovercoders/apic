package cli

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/skills"
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
		Name   string            `json:"name"`
		Files  map[string]string `json:"files"`
		Skills []string          `json:"skills"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v: %s", code, err, out)
	}
	if got.Name != "apic" || !strings.HasPrefix(got.Files["SKILL.md"], "---") || !strings.Contains(got.Files["references/cheatsheet.md"], "# apic cheat sheet") {
		t.Fatalf("json: %+v", got)
	}
	if !slices.Equal(got.Skills, []string{"apic", "api-design", "api-security"}) {
		t.Fatalf("skills: %v", got.Skills)
	}
}

// The pack's other skills are printed by name, and a name that is not in
// the pack is a usage error that lists what is.
func TestSkillPrintsTheNamedSkill(t *testing.T) {
	for _, name := range []string{"api-design", "api-security"} {
		code, out, errb := execute(t, "skill", name)
		if code != 0 || !strings.HasPrefix(out, "---\nname: "+name+"\n") {
			t.Fatalf("%s: code=%d err=%s out=%.80q", name, code, errb, out)
		}
		repo, err := os.ReadFile(filepath.Join("..", "..", "skills", name, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		if out != strings.ReplaceAll(string(repo), "\r\n", "\n") {
			t.Fatalf("apic skill %s differs from skills/%s/SKILL.md", name, name)
		}
	}
	code, out, errb := execute(t, "--json", "skill", "api-security")
	var got struct {
		Name  string            `json:"name"`
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v err=%s: %s", code, err, errb, out)
	}
	if got.Name != "api-security" || !strings.Contains(got.Files["references/checks.md"], "## Release checklist") {
		t.Fatalf("json: name=%s files=%v", got.Name, slices.Sorted(maps.Keys(got.Files)))
	}
	code, _, errb = execute(t, "--json", "skill", "api-desing")
	if code != 2 || !strings.Contains(errb, `"code":"E203"`) || !strings.Contains(errb, "api-design") {
		t.Fatalf("unknown skill: code=%d err=%s", code, errb)
	}
}

func TestSkillInstallWritesOnceAndReportsUnchanged(t *testing.T) {
	dir := t.TempDir()
	code, out, errb := execute(t, "skill", "install", dir, "--all", "--no-color")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	for _, rel := range []string{".claude/skills/apic/SKILL.md", ".claude/skills/apic/references/cheatsheet.md", ".agents/skills/apic/SKILL.md", ".agents/skills/apic/references/cheatsheet.md",
		".claude/skills/api-design/SKILL.md", ".claude/skills/api-design/references/openapi.md", ".agents/skills/api-security/SKILL.md", ".agents/skills/api-security/references/owasp-api-top-10.md"} {
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
	code, out, _ = execute(t, "--json", "skill", "install", dir, "--all")
	var got struct {
		Written   []string `json:"written"`
		Unchanged []string `json:"unchanged"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v: %s", code, err, out)
	}
	if len(got.Written) != 1 || !strings.HasSuffix(got.Written[0], filepath.FromSlash(".agents/skills/apic/SKILL.md")) || len(got.Unchanged) != packFiles()*len(SkillDirs)-1 {
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
	// Without --all the apic skill alone is installed: the other two are
	// opinions about the reader's API, which a project opts into.
	plain := t.TempDir()
	code, out, errb = execute(t, "--json", "skill", "install", plain)
	if code != 0 {
		t.Fatalf("default install: code=%d err=%s", code, errb)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Written) != len(skills.Paths("apic"))*len(SkillDirs) || slices.ContainsFunc(got.Written, func(p string) bool { return strings.Contains(p, "api-") }) {
		t.Errorf("default install wrote %v", got.Written)
	}
	if code, _, errb = execute(t, "skill", "install", plain, "--all", "--skill", "apic"); code != 2 || !strings.Contains(errb, "E203") {
		t.Errorf("--all --skill: code=%d err=%s", code, errb)
	}
	// --skill picks part of the pack, a name given twice counts once, and
	// an unknown name is refused before anything is written.
	some := t.TempDir()
	code, out, errb = execute(t, "--json", "skill", "install", some, "--skill", "apic", "--skill", "api-security", "--skill", "apic")
	if code != 0 {
		t.Fatalf("--skill: code=%d err=%s", code, errb)
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	want := (len(skills.Paths("apic")) + len(skills.Paths("api-security"))) * len(SkillDirs)
	if len(got.Written) != want || len(got.Unchanged) != 0 || slices.ContainsFunc(got.Written, func(p string) bool { return strings.Contains(p, "api-design") }) {
		t.Errorf("--skill apic --skill api-security --skill apic: written %v unchanged %v", got.Written, got.Unchanged)
	}
	if code, _, errb = execute(t, "skill", "install", some, "--skill", "nope"); code != 2 || !strings.Contains(errb, "E203") || !strings.Contains(errb, "api-design") {
		t.Errorf("--skill nope: code=%d err=%s", code, errb)
	}
}

// --user writes the same directories under the home directory, for every
// project, follows a .claude or .agents that a dotfile manager links
// elsewhere, writes nothing into the project, and takes neither a dir,
// -C nor --to, which would contradict it.
func TestSkillInstallUserWritesUnderHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	project := t.TempDir()
	t.Chdir(project)
	code, out, errb := execute(t, "--json", "skill", "install", "--user", "--skill", "apic")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	var got struct{ Written, Unchanged []string }
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Written) != len(skills.Paths("apic"))*len(SkillDirs) || got.Written[0] != filepath.Join(home, ".claude", "skills", "apic", "SKILL.md") {
		t.Errorf("written: %v", got.Written)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "apic", "references", "cheatsheet.md")); err != nil {
		t.Error(err)
	}
	if entries, _ := os.ReadDir(project); len(entries) != 0 {
		t.Error("--user should write nothing into the project")
	}
	if code, _, _ = execute(t, "--json", "skill", "install", "--user", "--skill", "apic"); code != 0 {
		t.Errorf("second --user install: code=%d", code)
	}
	for _, args := range [][]string{{"skill", "install", project, "--user"}, {"skill", "install", "--user", "--to", "x"}, {"-C", project, "skill", "install", "--user"}} {
		if code, _, errb = execute(t, args...); code != 2 || !strings.Contains(errb, "E203") {
			t.Errorf("%v: code=%d err=%s", args, code, errb)
		}
	}
	// A linked ~/.claude is followed to wherever the dotfiles live.
	dotfiles := t.TempDir()
	linked := t.TempDir()
	t.Setenv("HOME", linked)
	t.Setenv("USERPROFILE", linked)
	if err := os.Symlink(dotfiles, filepath.Join(linked, ".claude")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if code, _, errb = execute(t, "skill", "install", "--user", "--skill", "apic"); code != 0 {
		t.Fatalf("linked ~/.claude: code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(dotfiles, "skills", "apic", "SKILL.md")); err != nil {
		t.Error("the link should have been followed:", err)
	}
	// A home that is not a directory names itself, not a project.
	t.Setenv("HOME", filepath.Join(linked, "missing"))
	t.Setenv("USERPROFILE", filepath.Join(linked, "missing"))
	if code, _, errb = execute(t, "skill", "install", "--user"); code != 2 || !strings.Contains(errb, "home directory") || strings.Contains(errb, "apic init") {
		t.Errorf("missing home: code=%d err=%s", code, errb)
	}
}

// The help names every skill of the pack, so adding one without saying
// so fails here rather than leaving the help stale; a mistyped subcommand
// is pointed at the right one, not only at the skill names.
func TestSkillHelpNamesThePack(t *testing.T) {
	_, long, _ := execute(t, "skill", "--help")
	_, install, _ := execute(t, "skill", "install", "--help")
	for _, name := range skills.Names() {
		if !strings.Contains(long, "\n  "+name) {
			t.Errorf("apic skill --help does not list %s", name)
		}
		if !strings.Contains(install, name) {
			t.Errorf("apic skill install --help does not name %s", name)
		}
	}
	if code, _, errb := execute(t, "skill", "instal"); code != 2 || !strings.Contains(errb, "apic skill install") {
		t.Errorf("apic skill instal: code=%d err=%s", code, errb)
	}
}

// packFiles is how many files the whole pack holds; an install writes
// them into each of SkillDirs.
func packFiles() int {
	n := 0
	for _, name := range skills.Names() {
		n += len(skills.Paths(name))
	}
	return n
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
	// init writes the apic skill alone; the pack is `apic skill install --all`.
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", "api-design")); !os.IsNotExist(err) {
		t.Error("init should not write api-design")
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
	if len(got.Written) != len(skills.Paths("apic"))*len(SkillDirs) || got.Written[0] != filepath.Join("link", ".claude", "skills", "apic", "SKILL.md") {
		t.Errorf("written: %v", got.Written)
	}
	if _, err := os.Stat(filepath.Join("real", ".agents", "skills", "apic", "SKILL.md")); err != nil {
		t.Error("the files should land under the real directory the link names")
	}
}
