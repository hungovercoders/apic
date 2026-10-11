package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/runner"
)

// suggestProject has a request to misspell and an environment to misspell.
func suggestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"api.http":             "# @name login\nGET http://localhost/login\n\n### \n# @name get-user\nGET http://localhost/me\n",
		"http-client.env.json": `{"dev": {"a": "1"}, "staging": {"a": "2"}}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func jsonError(t *testing.T, errb string) runner.ErrorInfo {
	t.Helper()
	var got struct {
		Error runner.ErrorInfo `json:"error"`
	}
	if err := json.Unmarshal([]byte(errb), &got); err != nil {
		t.Fatalf("stderr is not one JSON error object: %v\n%s", err, errb)
	}
	return got.Error
}

func TestUnknownNamesSuggestTheClosest(t *testing.T) {
	dir := suggestProject(t)
	cases := []struct {
		args    []string
		code    runner.Code
		message string   // in the human message
		suggest []string // in the --json object
	}{
		{[]string{"run", "get-usr"}, runner.CodeUnknownRequest, `no request named "get-usr"; did you mean get-user? (run `, []string{"get-user"}},
		{[]string{"run", "Get_User"}, runner.CodeUnknownRequest, "did you mean get-user?", []string{"get-user"}},
		{[]string{"describe", "logn"}, runner.CodeUnknownRequest, "did you mean login?", []string{"login"}},
		{[]string{"run", "apo.http"}, runner.CodeUnknownRequest, `no request named "apo.http"; did you mean api.http?`, []string{"api.http"}},
		{[]string{"run", "apo.http#1"}, runner.CodeUnknownRequest, `no .http file "apo.http" in `, []string{"api.http"}},
		{[]string{"run", "apo.http#1"}, runner.CodeUnknownRequest, `; did you mean api.http?`, []string{"api.http"}},
		{[]string{"run", "api.http#get-usr"}, runner.CodeUnknownRequest, `no request named "get-usr" in api.http; did you mean get-user?`, []string{"get-user"}},
		{[]string{"fmt", "api.http#get-usr"}, runner.CodeUnknownRequest, `no request named "get-usr"; did you mean get-user?`, []string{"get-user"}},
		{[]string{"run", "login", "--env", "stagin"}, runner.CodeEnvironment, `environment "stagin" not found in http-client.env.json; did you mean staging? (have: dev, staging)`, []string{"staging"}},
		// An absolute path is compared by its path from the root.
		{[]string{"run", filepath.Join(dir, "apo.http") + "#1"}, runner.CodeUnknownRequest, `; did you mean api.http?`, []string{"api.http"}},
		// Nothing close: what there is, and no suggestions key.
		{[]string{"run", "zzzzzz"}, runner.CodeUnknownRequest, `no request named "zzzzzz"; known: get-user, login (run `, nil},
		{[]string{"run", "zzz.http#1"}, runner.CodeUnknownRequest, `no .http file "zzz.http" in `, nil},
		{[]string{"run", "zzz.http#1"}, runner.CodeUnknownRequest, `; files: api.http (run `, nil},
		{[]string{"run", "api.http#zzz"}, runner.CodeUnknownRequest, `no request named "zzz" in api.http; it has: login, get-user`, nil},
		{[]string{"run", "login", "--env", "zzz"}, runner.CodeEnvironment, `environment "zzz" not found in http-client.env.json (have: dev, staging)`, nil},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			args := append([]string{"-C", dir, "--no-session"}, c.args...)
			code, _, errb := execute(t, args...)
			if code != 2 || !strings.Contains(errb, c.message) {
				t.Errorf("human: code=%d, want the message to contain %q:\n%s", code, c.message, errb)
			}
			code, _, errb = execute(t, append(args, "--json")...)
			info := jsonError(t, errb)
			if code != 2 || info.Code != c.code {
				t.Fatalf("json: code=%d %s, want 2 %s: %s", code, info.Code, c.code, info.Message)
			}
			if strings.Join(info.Suggestions, ",") != strings.Join(c.suggest, ",") {
				t.Errorf("suggestions = %v, want %v", info.Suggestions, c.suggest)
			}
			if c.suggest == nil && strings.Contains(errb, "suggestions") {
				t.Errorf("an error with nothing to suggest should omit the key:\n%s", errb)
			}
		})
	}
}

func TestHumanErrorsCarryTheHintAndCodeOffATerminal(t *testing.T) {
	dir := suggestProject(t)
	if err := os.WriteFile(filepath.Join(dir, "var.http"), []byte("GET http://localhost/{{token}}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A missing variable's message does not name a command, so the
	// catalogue's hint follows it, then the code line; neither is coloured
	// because the test's stderr is a buffer, not a terminal.
	_, _, errb := execute(t, "-C", dir, "--no-session", "run", "var.http")
	e, _ := runner.Lookup(runner.CodeMissingVariable)
	for _, want := range []string{"error: ", "\n  hint: " + e.Hint + "\n", "\n  E101 missing variable · see https://apic.sh/errors/#e101\n"} {
		if !strings.Contains(errb, want) {
			t.Errorf("missing %q in:\n%s", want, errb)
		}
	}
	if strings.Contains(errb, "\x1b[") {
		t.Errorf("escape codes off a terminal:\n%s", errb)
	}
	// An unknown request's message already says to run apic list, so the
	// hint, which says the same, is left out; the code line stays.
	_, _, errb = execute(t, "-C", dir, "--no-session", "run", "get-usr")
	if strings.Contains(errb, "hint:") || !strings.Contains(errb, "\n  E201 unknown request · see https://apic.sh/errors/#e201\n") {
		t.Errorf("want the code line and no hint when the message names a command:\n%s", errb)
	}
	// A message that merely mentions apic is not one that names a command.
	if err := os.WriteFile(filepath.Join(dir, "h2.http"), []byte("GET http://localhost/ HTTP/2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, errb = execute(t, "-C", dir, "--no-session", "run", "h2.http")
	if !strings.Contains(errb, "apic does not send") || !strings.Contains(errb, "\n  hint: ") {
		t.Errorf("want the hint under a message that mentions apic without naming a command:\n%s", errb)
	}
}
