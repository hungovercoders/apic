package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestMockRejectsZeroPort(t *testing.T) {
	spec := writeMockSpec(t)
	app := New()
	var stdout, stderr bytes.Buffer
	app.Stdout, app.Stderr = &stdout, &stderr
	if code := app.Execute(context.Background(), []string{"mock", spec, "--port", "0"}); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "--port must be between 1 and 65535") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

func TestMockMissingFileIsE210(t *testing.T) {
	app := New()
	var stdout, stderr bytes.Buffer
	app.Stdout, app.Stderr = &stdout, &stderr
	if code := app.Execute(context.Background(), []string{"mock", filepath.Join(t.TempDir(), "missing.yaml")}); code != 2 {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "E210") {
		t.Fatalf("unexpected stderr: %s", stderr.String())
	}
}

// The command prints its startup line, serves the document until its
// context is cancelled, and exits 0.
func TestMockServesUntilCancelled(t *testing.T) {
	spec := writeMockSpec(t)
	port := freeLocalPort(t)
	app := New()
	var stdout, stderr bytes.Buffer
	app.Stdout, app.Stderr = &stdout, &stderr
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan int, 1)
	go func() { done <- app.Execute(ctx, []string{"mock", spec, "--port", strconv.Itoa(port), "--json"}) }()

	url := "http://127.0.0.1:" + strconv.Itoa(port)
	var res *http.Response
	var err error
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		res, err = http.Get(url + "/todos") //nolint:noctx // a test against a loopback server this test started
		if err == nil {
			break
		}
	}
	if err != nil {
		cancel()
		t.Fatalf("mock did not come up: %v (stderr %s)", err, stderr.String())
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode != 200 || !strings.Contains(string(body), `"title": "Buy milk"`) {
		t.Fatalf("GET /todos = %d %s", res.StatusCode, body)
	}
	cancel()
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("exit %d, stderr %s", code, stderr.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the command did not stop after cancel")
	}

	var startup struct {
		Spec      string `json:"spec"`
		URL       string `json:"url"`
		Listening bool   `json:"listening"`
		Routes    []struct {
			Method string `json:"method"`
			Path   string `json:"path"`
			Status int    `json:"status"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &startup); err != nil {
		t.Fatalf("stdout %q: %v", stdout.String(), err)
	}
	if startup.Spec != spec || startup.URL != "http://localhost:"+strconv.Itoa(port) || !startup.Listening || len(startup.Routes) != 2 || startup.Routes[1].Status != 201 {
		t.Fatalf("%+v", startup)
	}
}

func writeMockSpec(t *testing.T) string {
	t.Helper()
	spec := filepath.Join(t.TempDir(), "todos.yaml")
	if err := os.WriteFile(spec, []byte(`
openapi: 3.0.3
info: {title: Todos, version: "1"}
paths:
  /todos:
    get:
      responses:
        "200":
          description: ok
          content:
            application/json:
              example: [{id: "1", title: "Buy milk", done: false}]
    post:
      responses:
        "201": {description: created}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	return spec
}

func freeLocalPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}
