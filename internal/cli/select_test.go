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
	for _, bad := range []string{"0", "-1", "k", "4g", "lots"} {
		if _, err := parseSize(bad); err == nil {
			t.Errorf("%q should be refused", bad)
		}
	}
}
