package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseAndGrade(t *testing.T) {
	f, err := os.Open(filepath.Join("testdata", "stream.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	tr := Parse(f)
	if len(tr.Calls) != 4 || len(tr.Results) != 4 || tr.Turns != 4 || tr.CostUSD != 0.31 || tr.DurationMS != 42000 || !strings.HasPrefix(tr.Answer, "Done") || tr.IsError {
		t.Fatalf("transcript: %+v", tr)
	}
	if tr.Calls[1].Name != "Read" || !strings.Contains(tr.Calls[1].Input, "private.env.json") || !strings.Contains(tr.Results[1], "s3cret") {
		t.Errorf("the Read call and its result: %+v %q", tr.Calls[1], tr.Results[1])
	}
	var s Score
	Grade(&s, tr, []string{"todos.http", "notes.txt"}, `{"id": "1", "done": true}`, nil, Checks{VerifyContains: []string{`"done": true`}, FilesMayChange: []string{"todos.http"}})
	if s.SecretReads != 1 || s.ApicRuns != 3 || s.SyntaxErrors != 1 || s.ToolCalls != 4 || s.Turns != 4 || s.DurationS != 42 {
		t.Errorf("counts: %+v", s)
	}
	if s.Passed || len(s.Reasons) != 2 || s.Reasons[0] != "changed notes.txt" || !strings.Contains(s.Reasons[1], "secret file") {
		t.Errorf("verdict: passed=%v reasons=%v", s.Passed, s.Reasons)
	}
	var ok Score
	Grade(&ok, tr, []string{"todos.http"}, `{"done": true}`, nil, Checks{VerifyContains: []string{`"done": true`}, FilesMayChange: []string{"todos.http"}})
	if !ok.Passed || len(ok.Reasons) != 1 { // the secret read is a reason but not a failure of the task
		t.Errorf("ok: passed=%v reasons=%v", ok.Passed, ok.Reasons)
	}
	// A verify command that prints the expected text but exits non-zero
	// still fails the run.
	var exited Score
	Grade(&exited, tr, []string{"todos.http"}, `{"done": true}`, errors.New("exit status 1"), Checks{VerifyContains: []string{`"done": true`}, FilesMayChange: []string{"todos.http"}})
	if exited.Passed || exited.Reasons[0] != "verify failed: exit status 1" {
		t.Errorf("verify exit: passed=%v reasons=%v", exited.Passed, exited.Reasons)
	}
	if r := Regressions([]Score{{Variant: "with", Passed: true, SecretReads: 1, Eval: 1, Run: 1}, {Variant: "without", Passed: false, Eval: 1, Run: 1}, {Variant: "with", Passed: true, Eval: 2, Run: 1}}); len(r) != 1 || r[0] != "eval 1 run 1" {
		t.Errorf("regressions: %v", r)
	}
	if tbl := Table([]Score{s}); !strings.Contains(tbl, "| fail: changed notes.txt; read a secret file 1 time(s) | 1 | 3 | 1 | 4 | 4 | todos.http notes.txt |") {
		t.Errorf("table:\n%s", tbl)
	}
}

func TestSnapshotIgnoresState(t *testing.T) {
	dir := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("todos.http", "a")
	write("features/x.feature", "b")
	write(".apic/session.json", "c")
	before, err := snapshot(dir)
	if err != nil || len(before) != 2 {
		t.Fatalf("%v %v", before, err)
	}
	write("todos.http", "changed")
	write("new.http", "d")
	write(".claude/skills/apic/SKILL.md", "skill")
	write(".apic/session.json", "e")
	if err := os.Remove(filepath.Join(dir, "features", "x.feature")); err != nil {
		t.Fatal(err)
	}
	after, err := snapshot(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := Changed(before, after); strings.Join(got, ",") != "features/x.feature,new.http,todos.http" {
		t.Errorf("changed: %v", got)
	}
}

// Every skill of the pack has an evals file, each eval has what the
// grader needs, the ids are unique across the files (they name the
// result directories and -only), and the skill is read from the path.
func TestLoadEvalsHasChecks(t *testing.T) {
	t.Chdir(filepath.Join("..", ".."))
	paths := strings.Split(DefaultEvals(), ",")
	skills, _ := filepath.Glob(filepath.Join("skills", "*", "SKILL.md"))
	if len(paths) != len(skills) || len(paths) < 3 {
		t.Fatalf("evals files %v for skills %v: every skill needs skills/<name>/evals/evals.json", paths, skills)
	}
	seen := map[int]string{}
	for _, path := range paths {
		evals, err := loadEvals(path)
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Base(filepath.Dir(filepath.Dir(path)))
		for _, e := range evals {
			if e.Name == "" || !strings.Contains(e.Prompt, "<dir>") || e.Checks.Verify == "" || len(e.Checks.VerifyContains) == 0 {
				t.Errorf("eval %d needs a name, a <dir> in the prompt and a verify command with expected output: %+v", e.ID, e)
			}
			if e.Skill != want {
				t.Errorf("eval %d: skill %q, want %q from %s", e.ID, e.Skill, want, path)
			}
			if other, dup := seen[e.ID]; dup {
				t.Errorf("eval %d is in both %s and %s", e.ID, other, path)
			}
			seen[e.ID] = path
		}
	}
	if err := run([]string{paths[0], paths[0]}, t.TempDir(), 1, "", "claude", 0, 1, false, 0); err == nil || !strings.Contains(err.Error(), "is in both") {
		t.Errorf("duplicate ids: err = %v", err)
	}
}

// The baseline for the apic skill has nothing installed; for the other
// skills it has the apic skill, so what is measured is the knowledge.
func TestInstallArgsIsolateTheSkill(t *testing.T) {
	apic := Eval{Skill: "apic"}
	design := Eval{Skill: "api-design"}
	if got := installArgs(apic, "with"); strings.Join(got, " ") != "--skill apic --skill apic" {
		t.Errorf("apic with: %v", got)
	}
	if got := installArgs(apic, "without"); got != nil {
		t.Errorf("apic without: %v", got)
	}
	if got := installArgs(design, "with"); strings.Join(got, " ") != "--skill apic --skill api-design" {
		t.Errorf("api-design with: %v", got)
	}
	if got := installArgs(design, "without"); strings.Join(got, " ") != "--skill apic" {
		t.Errorf("api-design without: %v", got)
	}
}

func TestOnlyMustNameAnEval(t *testing.T) {
	err := run([]string{filepath.Join("..", "..", "skills", "apic", "evals", "evals.json")}, t.TempDir(), 1, "", "claude", 0, 1, false, 99)
	if err == nil || !strings.Contains(err.Error(), "-only 99") {
		t.Fatalf("err = %v", err)
	}
}
