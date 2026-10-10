package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// adHocProject serves one JSON object and counts the requests it gets.
func adHocProject(t *testing.T) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id": 7, "items": [{"done": true}, {"done": false}]}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "api.http"), `
### thing
# @name thing
# @assert status == 200
GET {{baseUrl}}/thing
X-Token: {{token}}

### other
# @name other
POST {{baseUrl}}/other
Content-Type: application/json

{"id": {{id}}}
`)
	mustWrite(t, filepath.Join(dir, "http-client.env.json"), `{"dev":{"baseUrl":"`+srv.URL+`","token":"t"}}`)
	return dir, &hits
}

type runObject struct {
	OK       bool              `json:"ok"`
	DryRun   bool              `json:"dry_run"`
	Request  map[string]any    `json:"request"`
	Response map[string]any    `json:"response"`
	Captures map[string]string `json:"captures"`
	Asserts  []struct {
		Expr   string `json:"expr"`
		Pass   bool   `json:"pass"`
		Actual string `json:"actual"`
	} `json:"asserts"`
}

func decodeRun(t *testing.T, out string) runObject {
	t.Helper()
	var obj runObject
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &obj); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	return obj
}

// --assert and --capture add to the request for this run, so a selector
// can be tried before it is written into the file.
func TestRunAssertAndCaptureFlags(t *testing.T) {
	dir, _ := adHocProject(t)
	base := []string{"-C", dir, "--env", "dev", "--no-session", "--json", "run", "thing"}
	code, out, errb := execute(t, append(base, "--assert", "body.$.items[?(@.done != true)].# == 1", "--assert", "body.$.id isInteger", "--capture", "count = body.$.items.#")...)
	if code != 0 {
		t.Fatalf("code=%d err=%s out=%s", code, errb, out)
	}
	obj := decodeRun(t, out)
	if len(obj.Asserts) != 3 || !obj.Asserts[1].Pass || obj.Asserts[1].Actual != "1" || !obj.Asserts[2].Pass {
		t.Errorf("asserts: %+v", obj.Asserts)
	}
	if obj.Captures["count"] != "2" {
		t.Errorf("captures: %v", obj.Captures)
	}
	// A failing flag assertion fails the run like a directive would.
	code, out, _ = execute(t, append(base, "--assert", "body.$.id == 8")...)
	if obj = decodeRun(t, out); code != 1 || obj.OK || obj.Asserts[1].Pass || obj.Asserts[1].Actual != "7" {
		t.Errorf("failing: code=%d %+v", code, obj.Asserts)
	}
	// A bad expression is a flag error that names the flag, before anything
	// is sent.
	for _, bad := range [][]string{{"--assert", "body.$.id ==="}, {"--capture", "nosel"}, {"--capture", "=body"}} {
		code, _, errb = execute(t, append(base, bad...)...)
		if code != 2 || !strings.Contains(errb, bad[0]+" ") {
			t.Errorf("%v: code=%d err=%s", bad, code, errb)
		}
	}
	// The project's own request is untouched: a plain run has one assert.
	if code, out, _ = execute(t, base...); code != 0 || len(decodeRun(t, out).Asserts) != 1 {
		t.Errorf("plain run after ad hoc: code=%d %s", code, out)
	}
}

// A dry run prints what would be sent and sends nothing.
func TestRunDryRun(t *testing.T) {
	dir, hits := adHocProject(t)
	code, out, errb := execute(t, "-C", dir, "--env", "dev", "--no-session", "--json", "run", "other", "--var", "id=3", "--dry-run")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	obj := decodeRun(t, out)
	if !obj.DryRun || !obj.OK || obj.Response != nil || obj.Request["method"] != "POST" || !strings.HasSuffix(obj.Request["url"].(string), "/other") {
		t.Errorf("dry run object: %s", out)
	}
	if hits.Load() != 0 {
		t.Fatal("a dry run sent a request")
	}
	// The text form shows the resolved body and says it was not sent.
	code, out, _ = execute(t, "-C", dir, "--env", "dev", "--no-session", "--no-color", "run", "other", "thing", "--var", "id=3", "--dry-run")
	if code != 0 || !strings.Contains(out, `{"id": 3}`) || strings.Count(out, "dry run · not sent") != 2 || !strings.Contains(out, "X-Token: t") {
		t.Errorf("text: code=%d\n%s", code, out)
	}
	// A missing variable is the same error a real run gives, and nothing
	// runs to supply it.
	code, _, errb = execute(t, "-C", dir, "--env", "dev", "--no-session", "run", "other", "--dry-run")
	if code != 2 || !strings.Contains(errb, "{{id}}") || hits.Load() != 0 {
		t.Errorf("missing: code=%d err=%s hits=%d", code, errb, hits.Load())
	}
	if code, _, errb = execute(t, "-C", dir, "--env", "dev", "run", "thing", "--dry-run", "--data", "-"); code != 2 || !strings.Contains(errb, "--dry-run") {
		t.Errorf("with --data: code=%d err=%s", code, errb)
	}
}

// fmt file.http#name formats the one request and keeps the rest byte for
// byte, so an agent's change stays its own.
func TestFmtOneRequest(t *testing.T) {
	dir := t.TempDir()
	src := "###   first\n# @assert status == 200\n# @name first\nGET   https://x/a\n\n\n### second\n# @capture id = body.$.id\n# @name second\nPOST https://x/b\ncontent-type: application/json\n\n{\"a\":1}\n"
	mustWrite(t, filepath.Join(dir, "api.http"), src)
	code, out, errb := execute(t, "-C", dir, "fmt", "api.http#second", "--no-color")
	if code != 0 || !strings.Contains(out, "formatted api.http") {
		t.Fatalf("code=%d out=%s err=%s", code, out, errb)
	}
	got := mustReadFile(t, filepath.Join(dir, "api.http"))
	want := "###   first\n# @assert status == 200\n# @name first\nGET   https://x/a\n\n\n### second\n# @name second\n# @capture id = body.$.id\nPOST https://x/b\nContent-Type: application/json\n\n{\n  \"a\": 1\n}\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// The number form, --check reporting, and an unknown name.
	if code, out, _ = execute(t, "-C", dir, "fmt", "api.http#1", "--check"); code != 1 || strings.TrimSpace(out) != "api.http" {
		t.Errorf("check: code=%d out=%q", code, out)
	}
	if code, out, _ = execute(t, "-C", dir, "fmt", "api.http#2", "--check"); code != 0 || !strings.Contains(out, "already formatted") {
		t.Errorf("second is formatted: code=%d out=%q", code, out)
	}
	if code, _, errb = execute(t, "-C", dir, "fmt", "api.http#third"); code != 2 || !strings.Contains(errb, `no request named "third"`) {
		t.Errorf("unknown: code=%d err=%s", code, errb)
	}
}

// A selector or a name a directive would be refused for is refused on the
// flag too, before the request is sent: an agent trying `--capture` on a
// DELETE must not learn of its typo from the response.
func TestRunAdHocFlagsAreCheckedBeforeSending(t *testing.T) {
	dir, hits := adHocProject(t)
	for _, bad := range [][]string{
		{"--assert", "nope == 1"},
		{"--assert", "header == 1"},
		{"--capture", "x=not-a-selector"},
		{"--capture", "1bad=body.$.id"},
		{"--capture", "two words=body.$.id"},
	} {
		code, _, errb := execute(t, "-C", dir, "--env", "dev", "--no-session", "run", "other", "--var", "id=1", bad[0], bad[1])
		if code != 2 || !strings.Contains(errb, bad[0]+" ") {
			t.Errorf("%v: code=%d err=%s", bad, code, errb)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("%d requests were sent before the flag check", hits.Load())
	}
}
