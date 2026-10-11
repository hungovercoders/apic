package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// A directory with no request files is where a new user is most likely to
// be standing: every command they might try there names apic init and
// apic ui --demo.
func TestEmptyDirectoryPointsAtInitAndDemo(t *testing.T) {
	dir := t.TempDir()
	code, out, _ := execute(t, "-C", dir, "list")
	if code != 0 || !strings.Contains(out, "no .http files found under") || !strings.Contains(out, "apic init") || !strings.Contains(out, "apic ui --demo") {
		t.Errorf("list: code=%d\n%s", code, out)
	}
	code, out, _ = execute(t, "-C", dir, "list", "--json")
	var got struct {
		Requests []any  `json:"requests"`
		Hint     string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("list --json: code=%d err=%v\n%s", code, err, out)
	}
	if len(got.Requests) != 0 || !strings.Contains(got.Hint, "apic init") {
		t.Errorf("list --json: %+v", got)
	}
	for _, args := range [][]string{{"run", "login"}, {"describe", "login"}, {"curl", "login"}} {
		code, _, errb := execute(t, append([]string{"-C", dir, "--no-session"}, args...)...)
		if code != 2 || !strings.Contains(errb, "no .http files found under") || !strings.Contains(errb, "apic init") || !strings.Contains(errb, "apic ui --demo") {
			t.Errorf("%v: code=%d\n%s", args, code, errb)
		}
	}
}

// A project with files but no # @name keeps the message about naming a
// request: init and demo are for an empty directory, not a bare one.
func TestUnnamedProjectSaysToNameARequest(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.http"), []byte("GET http://localhost/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, errb := execute(t, "-C", dir, "--no-session", "run", "login")
	if !strings.Contains(errb, "add `# @name login` above a request line") || strings.Contains(errb, "apic init") {
		t.Errorf("got:\n%s", errb)
	}
}

// apic with no arguments is a first contact: the help it prints carries
// examples, as does every command's help.
func TestEveryCommandHasExamples(t *testing.T) {
	code, out, _ := execute(t)
	if code != 0 || !strings.Contains(out, "Examples:") || !strings.Contains(out, "apic ui --demo") || !strings.Contains(out, "apic init") {
		t.Errorf("apic with no args: code=%d\n%s", code, out)
	}
	var walk func(c *cobra.Command, path string)
	walk = func(c *cobra.Command, path string) {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			return
		}
		if strings.TrimSpace(c.Example) == "" {
			t.Errorf("%s has no Example: block", path)
		}
		for _, sub := range c.Commands() {
			walk(sub, path+" "+sub.Name())
		}
	}
	walk(New().Root, "apic")
}
