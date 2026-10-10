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

// selectProject serves a JSON object that changes on every call and
// keeps a history of two responses.
func selectProject(t *testing.T, history string) (string, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "sid=secret")
		_, _ = w.Write([]byte(`{"call": ` + string(rune('0'+n)) + `, "items": [{"id": "a"}, {"id": "b"}], "big": "` + strings.Repeat("x", 3000) + `"}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "apic.yaml"), "env: dev\n"+history)
	mustWrite(t, filepath.Join(dir, "api.http"), "### thing\n# @name thing\nGET {{baseUrl}}/thing\n")
	mustWrite(t, filepath.Join(dir, "http-client.env.json"), `{"dev":{"baseUrl":"`+srv.URL+`"}}`)
	return dir, &hits
}

// select reads the recorded response, so looking again costs no call.
func TestSelectReadsTheLastResponseWithoutSending(t *testing.T) {
	dir, hits := selectProject(t, "history: 2\n")
	for range 2 {
		if code, _, errb := execute(t, "-C", dir, "run", "thing", "--json"); code != 0 {
			t.Fatalf("run: code=%d err=%s", code, errb)
		}
	}
	code, out, errb := execute(t, "-C", dir, "select", "thing", "body.$.items[1].id")
	if code != 0 || out != "b\n" {
		t.Fatalf("code=%d out=%q err=%s", code, out, errb)
	}
	if code, out, _ = execute(t, "-C", dir, "select", "thing", "body.$.items", "--no-color"); code != 0 || !strings.HasPrefix(out, "[\n  {\n") {
		t.Errorf("an array pretty-printed: code=%d out=%q", code, out)
	}
	code, out, _ = execute(t, "-C", dir, "--json", "select", "thing", "body.$.call", "--entry", "2")
	var got struct {
		Entry    int    `json:"entry"`
		Found    bool   `json:"found"`
		Value    any    `json:"value"`
		Selector string `json:"selector"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || code != 0 || got.Entry != 2 || !got.Found || got.Value != float64(1) || got.Selector != "body.$.call" {
		t.Errorf("entry 2 as json: code=%d %v %s", code, err, out)
	}
	if code, out, _ = execute(t, "-C", dir, "select", "thing", "status"); code != 0 || out != "200\n" {
		t.Errorf("status: code=%d out=%q", code, out)
	}
	if code, out, _ = execute(t, "-C", dir, "select", "thing", "header.content-type"); code != 0 || out != "application/json\n" {
		t.Errorf("header: code=%d out=%q", code, out)
	}
	// Nothing at the path is exit 1, like a failed capture; a bad selector
	// or entry is a flag error.
	if code, out, errb = execute(t, "-C", dir, "--json", "select", "thing", "body.$.nope"); code != 1 || !strings.Contains(out, `"found": false`) {
		t.Errorf("missing path: code=%d out=%s err=%s", code, out, errb)
	}
	if code, _, errb = execute(t, "-C", dir, "select", "thing", "nope"); code != 2 || !strings.Contains(errb, "selector") {
		t.Errorf("bad selector: code=%d err=%s", code, errb)
	}
	if code, _, errb = execute(t, "-C", dir, "select", "thing", "status", "--entry", "3"); code != 2 {
		t.Errorf("entry past the history: code=%d err=%s", code, errb)
	}
	if code, _, errb = execute(t, "-C", dir, "select", "nothing", "status"); code != 2 || !strings.Contains(errb, "nothing") {
		t.Errorf("unknown request: code=%d err=%s", code, errb)
	}
	if hits.Load() != 2 {
		t.Fatalf("select sent a request: %d calls", hits.Load())
	}
}

// With history off the error says how to switch it on.
func TestSelectNeedsHistory(t *testing.T) {
	dir, _ := selectProject(t, "")
	if code, _, errb := execute(t, "-C", dir, "run", "thing"); code != 0 {
		t.Fatalf("run: code=%d err=%s", code, errb)
	}
	code, _, errb := execute(t, "-C", dir, "select", "thing", "status")
	if code != 2 || !strings.Contains(errb, "history: 20") || !strings.Contains(errb, "no recorded response") {
		t.Fatalf("code=%d err=%s", code, errb)
	}
}

// --body-limit bounds what the displays show, marks it, and leaves the
// history whole for select to read.
func TestRunBodyLimit(t *testing.T) {
	dir, _ := selectProject(t, "history: 1\n")
	code, out, errb := execute(t, "-C", dir, "--json", "run", "thing", "--body-limit", "1k")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb)
	}
	var obj struct {
		Response struct {
			Body          any  `json:"body"`
			BodyTruncated bool `json:"body_truncated"`
			Size          int  `json:"size"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatal(err)
	}
	body, _ := obj.Response.Body.(string)
	if !obj.Response.BodyTruncated || len(body) != 1024 || obj.Response.Size <= 1024 || !strings.HasPrefix(body, `{"call": `) {
		t.Errorf("truncated: %s", out[:200])
	}
	if code, out, _ = execute(t, "-C", dir, "run", "thing", "--body-limit", "100", "--body-only"); code != 0 || len(out) != 101 {
		t.Errorf("body-only: code=%d len=%d", code, len(out))
	}
	if code, out, _ = execute(t, "-C", dir, "run", "thing", "--body-limit", "100", "--no-color"); code != 0 || !strings.Contains(out, "100 B of 3.") || !strings.Contains(out, "apic select thing body.$.<path>") {
		t.Errorf("human: code=%d\n%s", code, out)
	}
	// The whole body is in the history.
	if code, out, _ = execute(t, "-C", dir, "select", "thing", "body.$.big"); code != 0 || len(out) != 3001 {
		t.Errorf("select after a limited run: code=%d len=%d", code, len(out))
	}
	// A body within the limit is untouched, and a bad size is a flag error.
	if code, out, _ = execute(t, "-C", dir, "--json", "run", "thing", "--body-limit", "1m"); code != 0 || strings.Contains(out, "body_truncated") {
		t.Errorf("within limit: code=%d", code)
	}
	if code, _, errb = execute(t, "-C", dir, "run", "thing", "--body-limit", "lots"); code != 2 || !strings.Contains(errb, "--body-limit") {
		t.Errorf("bad size: code=%d err=%s", code, errb)
	}
}

func TestParseSize(t *testing.T) {
	for in, want := range map[string]int{"": 0, "512": 512, "4k": 4096, "4K": 4096, "4kb": 4096, "1m": 1 << 20, "2MB": 2 << 20} {
		if got, err := parseSize(in); err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"0", "-1", "k", "4g", "lots", "9223372036854775807k", "99999999999999999999"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}

// A history entry keeps one value per header, so the count and index
// forms are refused with a reason rather than answering wrongly.
func TestSelectRefusesMultiValueHeaderSelectors(t *testing.T) {
	dir, _ := selectProject(t, "history: 1\n")
	if code, _, errb := execute(t, "-C", dir, "run", "thing"); code != 0 {
		t.Fatalf("run: code=%d err=%s", code, errb)
	}
	for _, sel := range []string{"header.content-type.#", "header.content-type[0]", "headers.x[-1]"} {
		if code, _, errb := execute(t, "-C", dir, "select", "thing", sel); code != 2 || !strings.Contains(errb, "one value per header") {
			t.Errorf("%s: code=%d err=%s", sel, code, errb)
		}
	}
}

// The truncation note offers select only when the history took the
// response; a binary body keeps its raw prefix under the limit.
func TestBodyLimitNoteAndBinaryPrefix(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/bin" {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write(append([]byte{0xff, 0xfe, 0x00, 0x01}, []byte(strings.Repeat("z", 300))...))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"big": "` + strings.Repeat("x", 500) + `"}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "api.http"), "### thing\n# @name thing\nGET {{baseUrl}}/thing\n\n### bin\n# @name bin\nGET {{baseUrl}}/bin\n\n### anon\nGET {{baseUrl}}/thing\n")
	mustWrite(t, filepath.Join(dir, "http-client.env.json"), `{"dev":{"baseUrl":"`+srv.URL+`"}}`)
	// No history: the note says what to set.
	code, out, _ := execute(t, "-C", dir, "--env", "dev", "run", "thing", "--body-limit", "100", "--no-color")
	if code != 0 || !strings.Contains(out, "the rest is not kept: set history: N") || strings.Contains(out, "reads the rest") {
		t.Errorf("without history: code=%d\n%s", code, out)
	}
	mustWrite(t, filepath.Join(dir, "apic.yaml"), "env: dev\nhistory: 2\n")
	if code, out, _ = execute(t, "-C", dir, "run", "thing", "--body-limit", "100", "--no-color"); code != 0 || !strings.Contains(out, "apic select thing body.$.<path> reads the rest") {
		t.Errorf("with history: code=%d\n%s", code, out)
	}
	// An unnamed request is never recorded.
	if code, out, _ = execute(t, "-C", dir, "run", "api.http#3", "--body-limit", "100", "--no-color"); code != 0 || strings.Contains(out, "reads the rest") {
		t.Errorf("unnamed: code=%d\n%s", code, out)
	}
	// Bytes that are not text: the first 4 as they are, not rune-trimmed
	// to nothing; the JSON carries their base64.
	if code, out, _ = execute(t, "-C", dir, "run", "bin", "--body-limit", "4", "--body-only"); code != 0 || out != "\xff\xfe\x00\x01\n" {
		t.Errorf("binary body-only: code=%d out=%q", code, out)
	}
	if code, out, _ = execute(t, "-C", dir, "--json", "run", "bin", "--body-limit", "4"); code != 0 || !strings.Contains(out, `"body":"//4AAQ=="`) || !strings.Contains(out, `"body_encoding":"base64"`) || !strings.Contains(out, `"body_truncated":true`) {
		t.Errorf("binary json: code=%d %s", code, out)
	}
}

// An entry past a history that has some is a bad argument, not "history
// is off"; a redacted entry is refused with its reason; a data run says
// why nothing was kept.
func TestSelectErrorsSayWhy(t *testing.T) {
	dir, _ := selectProject(t, "history: 3\n")
	for range 2 {
		if code, _, errb := execute(t, "-C", dir, "run", "thing"); code != 0 {
			t.Fatalf("run: code=%d err=%s", code, errb)
		}
	}
	// History switched off afterwards: the two entries stay readable, and
	// a third is out of range, not missing history.
	mustWrite(t, filepath.Join(dir, "apic.yaml"), "env: dev\n")
	if code, out, _ := execute(t, "-C", dir, "select", "thing", "status", "--entry", "2"); code != 0 || out != "200\n" {
		t.Errorf("entry 2 with history off: code=%d out=%q", code, out)
	}
	if code, _, errb := execute(t, "-C", dir, "select", "thing", "status", "--entry", "3"); code != 2 || strings.Contains(errb, "history is off") || !strings.Contains(errb, "no #3") {
		t.Errorf("entry 3: code=%d err=%s", code, errb)
	}
	mustWrite(t, filepath.Join(dir, "apic.yaml"), "env: dev\nhistory: 3\n")
	if code, _, errb := execute(t, "-C", dir, "run", "thing", "--redact"); code != 0 {
		t.Fatalf("redacted run: code=%d err=%s", code, errb)
	}
	if code, _, errb := execute(t, "-C", dir, "select", "thing", "body.$.call"); code != 2 || !strings.Contains(errb, "--redact run") {
		t.Errorf("redacted entry: code=%d err=%s", code, errb)
	}
	mustWrite(t, filepath.Join(dir, "rows.csv"), "n\n1\n")
	if code, out, errb := execute(t, "-C", dir, "run", "thing", "--data", filepath.Join(dir, "rows.csv"), "--body-limit", "100", "--no-color"); code != 0 || !strings.Contains(out, "a data run keeps no history") {
		t.Errorf("data run note: code=%d err=%s\n%s", code, errb, out)
	}
}
