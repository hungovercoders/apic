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

func TestLoadEvalsHasChecks(t *testing.T) {
	evals, err := loadEvals(filepath.Join("..", "..", "skills", "apic", "evals", "evals.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range evals {
		if e.Name == "" || !strings.Contains(e.Prompt, "<dir>") || e.Checks.Verify == "" || len(e.Checks.VerifyContains) == 0 {
			t.Errorf("eval %d needs a name, a <dir> in the prompt and a verify command with expected output: %+v", e.ID, e)
		}
	}
}
