// Serving an OpenAPI 3 document as a stateless mock, for `apic mock`.

package openapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// Route is one operation the mock serves, as the startup report lists it.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Status int    `json:"status"`         // served when no Prefer header asks otherwise
	Name   string `json:"name,omitempty"` // the operationId
}

// Mock answers every operation of an OpenAPI 3 document with the response
// the document declares: the first 2xx (else default, else the first), a
// body from its example or derived from its schema, and its headers. It
// keeps no state and validates nothing: it is the document read back, so a
// request file can run before the API it describes exists.
type Mock struct {
	Routes   []Route
	Skipped  []string // operations that could not be registered, with the reason
	mux      *http.ServeMux
	patterns map[string]*dispatch // what is registered on the mux, by its "METHOD /path" pattern
}

// NewMock builds the mock for an OpenAPI 3 document (YAML or JSON). The
// paths are registered as written and, when the first server URL has a
// path such as /v1, under that prefix too.
func NewMock(data []byte) (*Mock, error) {
	d, err := parseDocument(data)
	if err != nil {
		return nil, err
	}
	prefix := d.basePath()
	m := &Mock{mux: http.NewServeMux(), patterns: map[string]*dispatch{}}
	for _, p := range d.entries(d.get(d.root, "paths")) {
		if !strings.HasPrefix(strings.TrimSpace(p.key), "/") {
			continue // an extension key, not a path
		}
		for _, e := range d.entries(p.value) {
			if httpMethods[e.key] {
				m.add(d.mockRoute(strings.ToUpper(e.key), p.key, d.resolve(e.value)), prefix)
			}
		}
	}
	if d.err != nil {
		return nil, d.err
	}
	if len(m.Routes) == 0 && len(m.Skipped) == 0 {
		return nil, errors.New("the document declares no operations under paths")
	}
	return m, nil
}

// basePath is the path of the first server URL ("/v1" for
// https://api.example.com/v1/), so the mock answers at /v1/pets as well as
// /pets; "" when the server has none.
func (d *document) basePath() string {
	base, err := d.baseURL()
	if err != nil {
		return ""
	}
	u, err := url.Parse(base)
	if err != nil {
		return ""
	}
	return strings.TrimRight(u.Path, "/")
}

// add registers a route under its own path, or records why it could not
// be, then under the server prefix as well. The prefixed alias is a
// convenience: when the document also declares that path outright, the
// declared operation keeps it and nothing is reported.
func (m *Mock) add(rt *mockRoute, prefix string) {
	pat, partial := muxPattern(rt.path)
	if partial {
		rt.exact = templateRegexp(prefix, rt.path)
	}
	if err := m.attach(rt.method+" "+pat, rt); err != nil {
		m.Skipped = append(m.Skipped, fmt.Sprintf("%s %s: %v", rt.method, rt.path, err))
		return
	}
	if prefix != "" {
		alias, _ := muxPattern(prefix + rt.path)
		_ = m.attach(rt.method+" "+alias, rt)
	}
	m.Routes = append(m.Routes, Route{Method: rt.method, Path: rt.path, Status: rt.def.status, Name: rt.name})
}

// attach adds a route to the dispatcher for a pattern, registering the
// pattern on the mux the first time it is seen.
func (m *Mock) attach(pattern string, rt *mockRoute) error {
	d := m.patterns[pattern]
	if d == nil {
		d = &dispatch{}
		if err := m.register(pattern, d); err != nil {
			return err
		}
		m.patterns[pattern] = d
	}
	switch {
	case rt.exact != nil:
		d.exact = append(d.exact, rt)
	case d.plain == nil:
		d.plain = rt
	default:
		return errors.New("its path is the same as an earlier operation's")
	}
	return nil
}

// register adds a pattern to the mux, turning the panic ServeMux raises for
// a pattern it cannot hold into an error that says why.
func (m *Mock) register(pattern string, h http.Handler) (err error) {
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprint(r)
			if strings.Contains(msg, "conflicts with") {
				err = errors.New("its path conflicts with another operation's")
				return
			}
			err = errors.New("ServeMux rejects its path: " + strings.TrimPrefix(strings.SplitN(msg, "\n", 2)[0], "http: "))
		}
	}()
	m.mux.Handle(pattern, h)
	return nil
}

// dispatch serves the operations that share one ServeMux pattern. A
// template with literal text around a parameter, such as
// /files/{name}.json, needs the same wildcard segment as /files/{id}, so
// the mux pattern is /files/{p0} for both; the routes with literal text
// are tried first, each against its own expression, and the plain
// {parameter} route takes what is left.
type dispatch struct {
	exact []*mockRoute
	plain *mockRoute
}

func (d *dispatch) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	for _, rt := range d.exact {
		if rt.exact.MatchString(r.URL.Path) {
			rt.ServeHTTP(w, r)
			return
		}
	}
	if d.plain != nil {
		d.plain.ServeHTTP(w, r)
		return
	}
	writeMockError(w, http.StatusNotFound, fmt.Sprintf("%s %s is not in the document", r.Method, r.URL.Path))
}

// cleanPath is a path template with its whitespace removed and a leading
// slash, as the importer and the mock both read one.
func cleanPath(path string) string {
	path = strings.Join(strings.Fields(path), "")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

// muxPattern turns an OpenAPI path template into a ServeMux pattern. Each
// segment holding a {parameter} becomes a wildcard segment (ServeMux wants a
// whole segment named by an identifier, where OpenAPI allows {pet-id} and
// {name}.json), and a trailing slash becomes an exact match rather than a
// prefix. partial reports a segment with literal text around its
// parameter, which the wildcard over-matches and templateRegexp narrows.
func muxPattern(path string) (pattern string, partial bool) {
	segs := strings.Split(cleanPath(path), "/")
	n := 0
	for i, s := range segs {
		if !strings.ContainsAny(s, "{}") {
			continue
		}
		whole := strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Count(s, "{") == 1
		if !whole {
			partial = true
		}
		segs[i] = fmt.Sprintf("{p%d}", n)
		n++
	}
	pattern = strings.Join(segs, "/")
	if strings.HasSuffix(pattern, "/") {
		pattern += "{$}"
	}
	return pattern, partial
}

// templateRegexp matches exactly the paths a template describes, with the
// server prefix optional: literal text as written, each {parameter} as one
// non-empty segment or part of one.
func templateRegexp(prefix, path string) *regexp.Regexp {
	var b strings.Builder
	b.WriteString("^")
	if prefix != "" {
		b.WriteString("(?:" + regexp.QuoteMeta(prefix) + ")?")
	}
	rest := cleanPath(path)
	for rest != "" {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			b.WriteString(regexp.QuoteMeta(rest))
			break
		}
		b.WriteString(regexp.QuoteMeta(rest[:open]))
		size := strings.IndexByte(rest[open:], '}')
		if size < 0 {
			b.WriteString(regexp.QuoteMeta(rest[open:]))
			break
		}
		b.WriteString("[^/]+")
		rest = rest[open+size+1:]
	}
	b.WriteString("$")
	return regexp.MustCompile(b.String())
}

// mockRoute is one operation: its documented responses and the one served
// by default.
type mockRoute struct {
	method, path, name string
	responses          []*mockResponse
	def                *mockResponse
	exact              *regexp.Regexp // set when the template has literal text around a parameter
}

// mockResponse is one entry of an operation's responses, rendered once.
type mockResponse struct {
	key     string // "200", "2XX" or "default"
	status  int
	headers [][2]string
	media   []*mockMedia
}

// mockMedia is one content type of a response: its body and named examples.
type mockMedia struct {
	contentType string
	name        string // the media type alone, lower-case, for Accept matching
	body        []byte // nil when the media type declares nothing the mock can serve
	examples    map[string][]byte
}

func (d *document) mockRoute(method, path string, op *yaml.Node) *mockRoute {
	rt := &mockRoute{method: method, path: path, name: str(d.get(op, "operationId"))}
	for _, r := range d.entries(d.get(op, "responses")) {
		status, ok := statusFor(r.key)
		if !ok {
			continue
		}
		res := &mockResponse{key: r.key, status: status, headers: d.mockHeaders(d.get(r.value, "headers"))}
		for _, ct := range d.entries(d.get(r.value, "content")) {
			res.media = append(res.media, d.mockMedia(ct.key, ct.value))
		}
		rt.responses = append(rt.responses, res)
	}
	rt.def = defaultResponse(rt.responses)
	return rt
}

// statusFor reads a responses key: a status code, a class such as 4XX (its
// first code), or default (200). Anything else is not a response.
func statusFor(key string) (int, bool) {
	if key == "default" {
		return http.StatusOK, true
	}
	if !reStatusKey.MatchString(key) {
		return 0, false
	}
	if strings.HasSuffix(key, "XX") {
		return int(key[0]-'0') * 100, true
	}
	n, _ := strconv.Atoi(key)
	return n, true
}

// defaultResponse is the one served without a Prefer header: the first 2xx,
// else default, else the first declared; an operation that declares none
// answers 200 with no body.
func defaultResponse(rs []*mockResponse) *mockResponse {
	for _, r := range rs {
		if strings.HasPrefix(r.key, "2") {
			return r
		}
	}
	for _, r := range rs {
		if r.key == "default" {
			return r
		}
	}
	if len(rs) > 0 {
		return rs[0]
	}
	return &mockResponse{key: "200", status: http.StatusOK}
}

func (d *document) mockMedia(contentType string, media *yaml.Node) *mockMedia {
	media = d.resolve(media)
	mm := &mockMedia{contentType: contentType, name: mediaTypeName(contentType), examples: map[string][]byte{}}
	if v, ok := d.mediaExample(media); ok {
		mm.body = renderBody(contentType, v)
	}
	for _, ex := range d.entries(d.get(media, "examples")) {
		if v := d.getRaw(ex.value, "value"); v != nil {
			if body := renderBody(contentType, decode(v)); body != nil {
				mm.examples[ex.key] = body
			}
		}
	}
	return mm
}

// renderBody serialises an example for the wire. A JSON media type gets
// JSON; any other gets a string as it is and a scalar as text, and nil for
// an object or a list, since JSON under an XML or CSV content type would
// mislead (the importer refuses the same). The importer's placeholders
// become the fixed values it writes into a verbatim body, so a mocked
// response is the same on every call.
func renderBody(contentType string, v any) []byte {
	v = concretize(v)
	if !isJSON(contentType) {
		switch t := v.(type) {
		case string:
			return []byte(t)
		case *orderedObject, []any:
			return nil
		case nil:
			return nil
		default:
			return []byte(fmt.Sprint(t))
		}
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return []byte(fmt.Sprint(v))
	}
	return append(data, '\n')
}

// framingHeaders are set by the server from what it sends, never from the
// document: a documented Content-Length would truncate the body.
var framingHeaders = map[string]bool{"content-type": true, "content-length": true, "transfer-encoding": true, "connection": true}

// mockHeaders renders a response's declared headers from their examples or
// schemas; Content-Type comes from the media type instead.
func (d *document) mockHeaders(hs *yaml.Node) [][2]string {
	var out [][2]string
	for _, h := range d.entries(hs) {
		if framingHeaders[strings.ToLower(h.key)] {
			continue
		}
		if v, ok := d.headerExample(d.resolve(h.value)); ok {
			out = append(out, [2]string{h.key, oneLine(headerText(v))})
		}
	}
	return out
}

func (d *document) headerExample(h *yaml.Node) (any, bool) {
	if n := d.getRaw(h, "example"); n != nil {
		return decode(n), true
	}
	if ex := firstExampleValue(d, d.get(h, "examples")); ex != nil {
		return decode(ex), true
	}
	if s := d.get(h, "schema"); s != nil {
		return d.exampleFromSchema(s, 0)
	}
	return nil, false
}

func headerText(v any) string {
	switch t := concretize(v).(type) {
	case string:
		return t
	case nil:
		return ""
	case *orderedObject, []any:
		data, _ := json.Marshal(t)
		return string(data)
	default:
		return fmt.Sprint(t)
	}
}

// ServeHTTP routes to the operation, or answers 405 (with Allow) when the
// path is known under another method and 404 when it is not. Both carry a
// JSON message rather than the mux's plain text, so `apic run` shows why.
// The mux's own answer for a near miss (a redirect from /pets to a declared
// /pets/) is a miss too: a path the document does not declare is a 404.
func (m *Mock) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if d := m.lookup(r); d != nil {
		d.ServeHTTP(w, r)
		return
	}
	if allow := m.allowed(r); len(allow) > 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		writeMockError(w, http.StatusMethodNotAllowed,
			fmt.Sprintf("%s %s is not in the document; the path has %s", r.Method, r.URL.Path, strings.Join(allow, ", ")))
		return
	}
	writeMockError(w, http.StatusNotFound, fmt.Sprintf("%s %s is not in the document", r.Method, r.URL.Path))
}

// allowed lists the methods under which the request's path is an operation.
func (m *Mock) allowed(r *http.Request) []string {
	var allow []string
	tried := map[string]bool{r.Method: true}
	for _, rt := range m.Routes {
		if tried[rt.Method] {
			continue
		}
		tried[rt.Method] = true
		probe := r.Clone(r.Context())
		probe.Method = rt.Method
		if m.lookup(probe) != nil {
			allow = append(allow, rt.Method)
		}
	}
	return allow
}

// lookup is the dispatcher the mux routes a request to, or nil when the
// mux would answer itself: not found, method not allowed, or a redirect.
func (m *Mock) lookup(r *http.Request) *dispatch {
	h, _ := m.mux.Handler(r)
	d, _ := h.(*dispatch)
	return d
}

// reStatusCode is a concrete status code a Prefer header may ask for.
var reStatusCode = regexp.MustCompile(`^[1-5]\d\d$`)

// ServeHTTP writes the operation's default response, or the one a Prefer
// header asks for: code=<status> picks another documented response (an
// exact key, its class, or default served with that status) and
// example=<name> one of the media type's named examples. A choice the
// document does not offer is a 406 with a JSON message.
func (rt *mockRoute) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pref := parsePrefer(strings.Join(r.Header.Values("Prefer"), ","))
	res, status := rt.def, rt.def.status
	if code := pref["code"]; code != "" {
		if !reStatusCode.MatchString(code) {
			writeMockError(w, http.StatusNotAcceptable, fmt.Sprintf("Prefer: code=%s is not a status code", code))
			return
		}
		found, ok := rt.byCode(code)
		if !ok {
			writeMockError(w, http.StatusNotAcceptable, fmt.Sprintf("the document has no %s response for %s %s", code, rt.method, rt.path))
			return
		}
		res = found
		status, _ = strconv.Atoi(code)
	}
	media := res.pick(strings.Join(r.Header.Values("Accept"), ","))
	var body []byte
	if media != nil {
		body = media.body
	}
	if name := pref["example"]; name != "" {
		var b []byte
		ok := false
		if media != nil {
			b, ok = media.examples[name]
		}
		if !ok {
			writeMockError(w, http.StatusNotAcceptable, fmt.Sprintf("the %s response of %s %s has no example named %q", res.key, rt.method, rt.path, name))
			return
		}
		body = b
	}
	for _, h := range res.headers {
		w.Header().Add(h[0], h[1])
	}
	if media != nil && body != nil {
		w.Header().Set("Content-Type", media.contentType)
	}
	w.WriteHeader(status)
	if len(body) > 0 {
		_, _ = w.Write(body)
	}
}

// byCode finds the response a Prefer: code asks for: the exact key, its
// class (4XX for 404), or default.
func (rt *mockRoute) byCode(code string) (*mockResponse, bool) {
	for _, want := range []string{code, code[:1] + "XX", "default"} {
		for _, r := range rt.responses {
			if r.key == want {
				return r, true
			}
		}
	}
	return nil, false
}

// pick chooses the media type the Accept header asks for, else the first.
func (res *mockResponse) pick(accept string) *mockMedia {
	if len(res.media) == 0 {
		return nil
	}
	for _, a := range strings.Split(accept, ",") {
		a = mediaTypeName(a)
		if a == "" || a == "*/*" {
			break
		}
		for _, m := range res.media {
			if m.name == a || (strings.HasSuffix(a, "/*") && strings.HasPrefix(m.name, strings.TrimSuffix(a, "*"))) {
				return m
			}
		}
	}
	return res.media[0]
}

// mediaTypeName is the media type without its parameters, lower-case:
// "application/json" for "Application/JSON; charset=utf-8".
func mediaTypeName(contentType string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
}

// parsePrefer reads a Prefer header (RFC 7240) into lower-case names and
// values; preferences split on commas and semicolons, so
// "code=404, example=missing" and "code=404; example=missing" read the same.
func parsePrefer(h string) map[string]string {
	out := map[string]string{}
	for _, part := range strings.FieldsFunc(h, func(r rune) bool { return r == ',' || r == ';' }) {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		out[strings.ToLower(strings.TrimSpace(k))] = strings.Trim(strings.TrimSpace(v), `"`)
	}
	return out
}

func writeMockError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
