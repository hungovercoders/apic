package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readMCPConfig returns the parsed .mcp.json (or another file) under dir.
func readMCPConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path) //nolint:gosec // a test file in a temporary directory
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("%s: %v\n%s", path, err, data)
	}
	return doc
}

// serverArgs returns the args of the apic entry in a parsed configuration.
func serverArgs(t *testing.T, doc map[string]any) []string {
	t.Helper()
	servers, _ := doc["mcpServers"].(map[string]any)
	entry, _ := servers[mcpServerName].(map[string]any)
	if entry == nil || entry["command"] != "apic" {
		t.Fatalf("no apic entry: %v", doc)
	}
	raw, _ := entry["args"].([]any)
	var args []string
	for _, a := range raw {
		args = append(args, a.(string)) //nolint:forcetypeassert // a test reading what it wrote
	}
	return args
}

func TestMCPInstallWritesTheProjectEntry(t *testing.T) {
	dir := t.TempDir()
	code, out, errb := execute(t, "-C", dir, "mcp", "install", dir, "--no-color")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	path := filepath.Join(dir, ".mcp.json")
	if !strings.Contains(out, "wrote "+path) {
		t.Fatalf("output lacks wrote %s:\n%s", path, out)
	}
	if got := serverArgs(t, readMCPConfig(t, path)); strings.Join(got, " ") != "mcp" {
		t.Fatalf("args = %q, want just mcp when the project is the directory", got)
	}
	// The project below the directory the agent starts in, and an
	// environment, go into the entry as flags, the path in forward slashes.
	api := filepath.Join(dir, "api")
	if err := os.Mkdir(api, 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, _ = execute(t, "--json", "-C", api, "--env", "dev", "mcp", "install", dir)
	var got struct {
		Written   []string `json:"written"`
		Unchanged []string `json:"unchanged"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 {
		t.Fatalf("code=%d %v: %s", code, err, out)
	}
	if len(got.Written) != 1 || len(got.Unchanged) != 0 {
		t.Fatalf("second install: %+v", got)
	}
	if args := serverArgs(t, readMCPConfig(t, path)); strings.Join(args, " ") != "mcp --dir api --env dev" {
		t.Fatalf("args = %q", args)
	}
	// The same entry again is unchanged.
	code, out, _ = execute(t, "--json", "-C", api, "--env", "dev", "mcp", "install", dir)
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || len(got.Written) != 0 || len(got.Unchanged) != 1 {
		t.Fatalf("third install: code=%d %v %+v", code, err, got)
	}
}

func TestMCPInstallMergesAndRefusesWhatItCannotRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".mcp.json")
	mustWrite(t, path, `{"mcpServers": {"other": {"command": "x"}}, "note": "<kept>"}`)
	if code, _, errb := execute(t, "-C", dir, "mcp", "install", dir); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	doc := readMCPConfig(t, path)
	servers, _ := doc["mcpServers"].(map[string]any)
	if servers["other"] == nil || doc["note"] != "<kept>" {
		t.Fatalf("merge lost what the file held: %v", doc)
	}
	if data, _ := os.ReadFile(path); strings.Contains(string(data), `\u003c`) { //nolint:gosec // a test file in a temporary directory
		t.Fatalf("HTML escaped: %s", data)
	}
	serverArgs(t, doc)

	// --to picks the file, under the directory; a subdirectory is made,
	// and an absolute path is reported as given.
	if code, _, errb := execute(t, "-C", dir, "mcp", "install", dir, "--to", ".cursor/mcp.json"); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	serverArgs(t, readMCPConfig(t, filepath.Join(dir, ".cursor", "mcp.json")))
	abs := filepath.Join(dir, "tools", "mcp.json")
	if code, out, errb := execute(t, "-C", dir, "mcp", "install", dir, "--to", abs, "--no-color"); code != 0 || !strings.Contains(out, "wrote "+abs+"\n") {
		t.Fatalf("absolute --to: code=%d err=%s out=%s", code, errb, out)
	}
	if code, _, errb := execute(t, "-C", dir, "mcp", "install", dir, "--to", "../outside.json"); code != 2 || !strings.Contains(errb, "E210") {
		t.Fatalf("outside the directory: code=%d err=%s", code, errb)
	}

	// An empty file is an empty document; one that is not JSON, or whose
	// mcpServers is not an object, is reported and left as it is.
	mustWrite(t, path, "")
	if code, _, errb := execute(t, "-C", dir, "mcp", "install", dir); code != 0 {
		t.Fatalf("empty file: code=%d err=%s", code, errb)
	}
	serverArgs(t, readMCPConfig(t, path))
	for _, bad := range []string{"not json", "null", `{"mcpServers": ["x"]}`} {
		mustWrite(t, path, bad)
		if code, _, errb := execute(t, "-C", dir, "mcp", "install", dir); code != 2 || !strings.Contains(errb, "E210") || !strings.Contains(errb, "not a") {
			t.Fatalf("%s: code=%d err=%s", bad, code, errb)
		}
		if data, _ := os.ReadFile(path); string(data) != bad { //nolint:gosec // a test file in a temporary directory
			t.Fatalf("%s: the file was replaced: %s", bad, data)
		}
	}
	// A directory that does not exist is a mistyped project.
	if code, _, errb := execute(t, "mcp", "install", filepath.Join(dir, "missing")); code != 2 || !strings.Contains(errb, "E210") {
		t.Fatalf("missing dir: code=%d err=%s", code, errb)
	}
}

func TestInitRegistersTheServerUnlessToldNotTo(t *testing.T) {
	dir := t.TempDir()
	if code, out, errb := execute(t, "init", dir, "--no-color"); code != 0 || !strings.Contains(out, "wrote "+filepath.Join(dir, ".mcp.json")) {
		t.Fatalf("code=%d err=%s out=%s", code, errb, out)
	}
	if args := serverArgs(t, readMCPConfig(t, filepath.Join(dir, ".mcp.json"))); strings.Join(args, " ") != "mcp" {
		t.Fatalf("args = %q", args)
	}
	// A second init finds the entry there, so it is neither written nor
	// "kept". An entry the team has changed is kept unless --force, like
	// every other file.
	if code, out, _ := execute(t, "--json", "init", dir); code != 0 || strings.Contains(out, ".mcp.json") {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if code, _, errb := execute(t, "-C", filepath.Join(dir, "api"), "--env", "dev", "mcp", "install", dir); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	path := filepath.Join(dir, ".mcp.json")
	if code, out, _ := execute(t, "init", dir, "--no-color"); code != 0 || !strings.Contains(out, "kept  "+path) {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if args := serverArgs(t, readMCPConfig(t, path)); strings.Join(args, " ") != "mcp --dir api --env dev" {
		t.Fatalf("init replaced the entry without --force: %q", args)
	}
	if code, out, _ := execute(t, "init", dir, "--force", "--no-color"); code != 0 || !strings.Contains(out, "wrote "+path) {
		t.Fatalf("code=%d out=%s", code, out)
	}
	if args := serverArgs(t, readMCPConfig(t, path)); strings.Join(args, " ") != "mcp" {
		t.Fatalf("--force should replace the entry: %q", args)
	}
	bare := t.TempDir()
	if code, _, errb := execute(t, "init", bare, "--no-mcp"); code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	if _, err := os.Stat(filepath.Join(bare, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatal("--no-mcp should leave .mcp.json out")
	}
}

// Off a terminal, a plain error ends with the way to get it as JSON: the
// reader is more likely a script or an agent than a person.
func TestErrorOffTerminalPointsAtJSON(t *testing.T) {
	code, _, errb := execute(t, "-C", t.TempDir(), "run", "nope")
	if code != 2 || !strings.Contains(errb, "\n  --json prints this error as one") {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	code, _, errb = execute(t, "--json", "-C", t.TempDir(), "run", "nope")
	if code != 2 || strings.Contains(errb, "--json prints") || !strings.HasPrefix(errb, `{"error":`) {
		t.Fatalf("under --json the object stands alone: code=%d err=%s", code, errb)
	}
	// Not on the one error --json cannot help with: apic ui refuses it.
	code, _, errb = execute(t, "-C", t.TempDir(), "ui")
	if code != 2 || !strings.Contains(errb, "E213") || strings.Contains(errb, "--json prints") {
		t.Fatalf("apic ui: code=%d err=%s", code, errb)
	}
}
