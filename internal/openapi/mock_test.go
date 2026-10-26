package openapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const mockSpec = `
openapi: 3.0.3
info: {title: Todos, version: "1"}
servers:
  - url: http://localhost:8089/v1
paths:
  /todos:
    get:
      operationId: listTodos
      responses:
        "200":
          description: ok
          headers:
            X-Total-Count: {schema: {type: integer, example: 2}}
            Content-Type: {schema: {type: string}}
          content:
            application/json:
              schema: {type: array, items: {$ref: "#/components/schemas/Todo"}}
            text/csv:
              example: "id,title\n1,Buy milk\n"
    post:
      operationId: createTodo
      responses:
        "422":
          description: invalid
          content:
            application/json:
              example: {error: "title is required"}
        "201":
          description: created
          content:
            application/json:
              schema: {$ref: "#/components/schemas/Todo"}
  /todos/{todo-id}:
    get:
      operationId: getTodo
      responses:
        "200":
          description: ok
          content:
            application/json:
              examples:
                milk: {value: {id: "1", title: "Buy milk", done: false}}
                docs: {value: {id: "2", title: "Read the apic docs", done: true}}
        "4XX":
          description: client error
          content:
            application/json:
              example: {error: "no such todo"}
    delete:
      operationId: deleteTodo
      responses:
        "204": {description: gone}
  /reports/{name}.csv:
    get:
      responses:
        default:
          description: whatever
          content:
            text/csv:
              schema: {type: string}
  /:
    get:
      responses:
        "200":
          description: root
          content:
            text/plain:
              example: hello
components:
  schemas:
    Todo:
      type: object
      properties:
        id: {type: string, format: uuid}
        title: {type: string, example: Buy milk}
        done: {type: boolean, default: false}
        due: {type: string, format: date-time}
`

func newMockServer(t *testing.T, spec string) (*Mock, *httptest.Server) {
	t.Helper()
	m, err := NewMock([]byte(spec))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(m)
	t.Cleanup(srv.Close)
	return m, srv
}

// mockReply is what mockCall returns: the status and headers, with the
// body already read and closed.
type mockReply struct {
	status int
	header http.Header
}

func mockCall(t *testing.T, srv *httptest.Server, method, path string, hdr map[string]string) (mockReply, string) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	data, _ := io.ReadAll(res.Body)
	return mockReply{status: res.StatusCode, header: res.Header}, string(data)
}

func TestMockRoutesAndDefaultResponses(t *testing.T) {
	m, srv := newMockServer(t, mockSpec)
	if len(m.Routes) != 6 || len(m.Skipped) != 0 {
		t.Fatalf("routes %+v skipped %v", m.Routes, m.Skipped)
	}
	want := []Route{
		{Method: "GET", Path: "/todos", Status: 200, Name: "listTodos"},
		{Method: "POST", Path: "/todos", Status: 201, Name: "createTodo"},
		{Method: "GET", Path: "/todos/{todo-id}", Status: 200, Name: "getTodo"},
		{Method: "DELETE", Path: "/todos/{todo-id}", Status: 204, Name: "deleteTodo"},
		{Method: "GET", Path: "/reports/{name}.csv", Status: 200},
		{Method: "GET", Path: "/", Status: 200},
	}
	for i, r := range want {
		if m.Routes[i] != r {
			t.Errorf("route %d = %+v, want %+v", i, m.Routes[i], r)
		}
	}

	// The schema becomes a body, with the importer's placeholders fixed.
	res, body := mockCall(t, srv, "GET", "/todos", nil)
	if res.status != 200 || res.header.Get("Content-Type") != "application/json" || res.header.Get("X-Total-Count") != "2" {
		t.Fatalf("status %d headers %v", res.status, res.header)
	}
	var list []map[string]any
	if err := json.Unmarshal([]byte(body), &list); err != nil || len(list) != 1 {
		t.Fatalf("body %q: %v", body, err)
	}
	if list[0]["title"] != "Buy milk" || list[0]["done"] != false || list[0]["id"] != "00000000-0000-4000-8000-000000000000" || list[0]["due"] != "2024-01-01T00:00:00Z" {
		t.Fatalf("body %q", body)
	}

	// The first 2xx wins over an earlier 4xx; the server prefix is served too.
	if res, _ := mockCall(t, srv, "POST", "/v1/todos", nil); res.status != 201 {
		t.Fatalf("POST /v1/todos = %d", res.status)
	}
	// The first named example is the body; 204 has none.
	res, body = mockCall(t, srv, "GET", "/todos/1", nil)
	if res.status != 200 || !strings.Contains(body, `"title": "Buy milk"`) {
		t.Fatalf("GET /todos/1 = %d %q", res.status, body)
	}
	if res, body := mockCall(t, srv, "DELETE", "/todos/1", nil); res.status != 204 || body != "" {
		t.Fatalf("DELETE = %d %q", res.status, body)
	}
	// A segment that is not a whole {parameter} still matches, and a
	// default response serves 200 with the schema's string.
	res, body = mockCall(t, srv, "GET", "/reports/daily.csv", nil)
	if res.status != 200 || res.header.Get("Content-Type") != "text/csv" || body != "string" {
		t.Fatalf("report = %d %q %q", res.status, res.header.Get("Content-Type"), body)
	}
	// The root path is exact, not a prefix.
	if res, body := mockCall(t, srv, "GET", "/", nil); res.status != 200 || body != "hello" {
		t.Fatalf("root = %d %q", res.status, body)
	}
}

func TestMockPreferAndAccept(t *testing.T) {
	_, srv := newMockServer(t, mockSpec)
	// An exact code.
	res, body := mockCall(t, srv, "POST", "/todos", map[string]string{"Prefer": "code=422"})
	if res.status != 422 || !strings.Contains(body, "title is required") {
		t.Fatalf("422 = %d %q", res.status, body)
	}
	// A class key serves the asked code with its body.
	res, body = mockCall(t, srv, "GET", "/todos/9", map[string]string{"Prefer": "code=404"})
	if res.status != 404 || !strings.Contains(body, "no such todo") {
		t.Fatalf("404 = %d %q", res.status, body)
	}
	// default serves any code.
	if res, _ := mockCall(t, srv, "GET", "/reports/x.csv", map[string]string{"Prefer": "code=503"}); res.status != 503 {
		t.Fatalf("503 = %d", res.status)
	}
	// A named example, with the two separators.
	for _, h := range []string{"code=200, example=docs", "code=200; example=\"docs\""} {
		res, body = mockCall(t, srv, "GET", "/todos/2", map[string]string{"Prefer": h})
		if res.status != 200 || !strings.Contains(body, "Read the apic docs") {
			t.Fatalf("%q = %d %q", h, res.status, body)
		}
	}
	// Choices the document does not offer.
	for _, c := range []struct{ path, prefer, want string }{
		{"/todos", "code=500", "no 500 response"},
		{"/todos", "code=abc", "not a status code"},
		{"/todos/1", "example=nope", `no example named`},
		{"/todos", "example=any", `has no example named`},
	} {
		res, body = mockCall(t, srv, "GET", c.path, map[string]string{"Prefer": c.prefer})
		if res.status != 406 || !strings.Contains(body, c.want) {
			t.Errorf("%s %s = %d %q, want 406 with %q", c.path, c.prefer, res.status, body, c.want)
		}
	}
	// Accept picks the media type; an unknown one falls back to the first.
	res, body = mockCall(t, srv, "GET", "/todos", map[string]string{"Accept": "text/csv"})
	if res.header.Get("Content-Type") != "text/csv" || body != "id,title\n1,Buy milk\n" {
		t.Fatalf("csv = %q %q", res.header.Get("Content-Type"), body)
	}
	res, _ = mockCall(t, srv, "GET", "/todos", map[string]string{"Accept": "image/png, text/*;q=0.5"})
	if res.header.Get("Content-Type") != "text/csv" {
		t.Fatalf("text/* = %q", res.header.Get("Content-Type"))
	}
	res, _ = mockCall(t, srv, "GET", "/todos", map[string]string{"Accept": "image/png"})
	if res.header.Get("Content-Type") != "application/json" {
		t.Fatalf("fallback = %q", res.header.Get("Content-Type"))
	}
}

func TestMockUnknownPathAndMethod(t *testing.T) {
	_, srv := newMockServer(t, mockSpec)
	res, body := mockCall(t, srv, "GET", "/nope", nil)
	if res.status != 404 || !strings.Contains(body, "GET /nope is not in the document") {
		t.Fatalf("404 = %d %q", res.status, body)
	}
	res, body = mockCall(t, srv, "PUT", "/todos", nil)
	if res.status != 405 || res.header.Get("Allow") != "GET, POST" || !strings.Contains(body, "PUT /todos is not in the document") {
		t.Fatalf("405 = %d allow %q %q", res.status, res.header.Get("Allow"), body)
	}
}

func TestMockRejectsBadDocuments(t *testing.T) {
	for _, c := range []struct{ spec, want string }{
		{`{"not": "a spec"}`, "not an OpenAPI 3 document"},
		{"swagger: '2.0'\npaths: {}\n", "Swagger 2.0"},
		{"openapi: 3.0.3\ninfo: {title: t, version: '1'}\npaths: {}\n", "no operations"},
		{"openapi: 3.0.3\ninfo: {title: t, version: '1'}\npaths:\n  /a:\n    get:\n      responses:\n        '200':\n          content:\n            application/json:\n              schema: {$ref: '#/components/schemas/Missing'}\n", "unresolvable"},
	} {
		if _, err := NewMock([]byte(c.spec)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%q: got %v, want %q", c.spec, err, c.want)
		}
	}
}

// Two templates that ServeMux cannot hold side by side: the second is
// reported rather than panicking or silently vanishing.
func TestMockReportsConflictingPaths(t *testing.T) {
	m, err := NewMock([]byte(`
openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /a/{x}:
    get: {responses: {"200": {description: ok}}}
  /{y}/b:
    get: {responses: {"200": {description: ok}}}
`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Routes) != 1 || len(m.Skipped) != 1 || !strings.Contains(m.Skipped[0], "GET /{y}/b: its path conflicts") {
		t.Fatalf("routes %+v skipped %v", m.Routes, m.Skipped)
	}
}

func TestMockPetstore(t *testing.T) {
	data, err := os.ReadFile("testdata/petstore.yaml")
	if err != nil {
		t.Fatal(err)
	}
	m, err := NewMock(data)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(m)
	defer srv.Close()
	res, body := mockCall(t, srv, "GET", "/v1/pets", nil)
	if res.status != 200 || !strings.Contains(body, `"name": "Rex"`) {
		t.Fatalf("%d %q", res.status, body)
	}
	if res, _ := mockCall(t, srv, "GET", "/health", nil); res.status != 200 {
		t.Fatalf("health = %d", res.status)
	}
}

func TestMuxPattern(t *testing.T) {
	for in, want := range map[string]string{
		"/pets":              "/pets",
		"/pets/{petId}":      "/pets/{p0}",
		"/pets/{pet-id}/x":   "/pets/{p0}/x",
		"/files/{name}.json": "/files/{p0}",
		"/a/{b}/c/{d}":       "/a/{p0}/c/{p1}",
		"/pets/":             "/pets/{$}",
		"/":                  "/{$}",
		"pets":               "/pets",
		"/with space/{x}":    "/withspace/{p0}",
	} {
		if got := muxPattern(in); got != want {
			t.Errorf("muxPattern(%q) = %q, want %q", in, got, want)
		}
	}
}
