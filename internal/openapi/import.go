// Package openapi scaffolds .http files from an OpenAPI 3 document: one file
// per tag, one named request per operation, path parameters as {{vars}} and
// an example body derived from the schema.
package openapi

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hungovercoders/apic/internal/httpfile"
)

// Result summarises what was generated.
type Result struct {
	Files    []string `json:"files"`
	Requests int      `json:"requests"`
	BaseURL  string   `json:"base_url"`
	EnvFile  string   `json:"env_file,omitempty"`
	Skipped  []string `json:"skipped,omitempty"`
}

func (d *document) resolveServerURL(srv *yaml.Node) (string, error) {
	rawURL := strings.TrimSpace(str(d.get(srv, "url")))
	if rawURL == "" {
		return "", nil
	}
	defaults := map[string]string{}
	for _, v := range d.entries(d.get(srv, "variables")) {
		if def := d.get(v.value, "default"); def != nil { // absent default stays unresolved
			defaults[v.key] = str(def)
		}
	}
	out := reServerVar.ReplaceAllStringFunc(rawURL, func(match string) string {
		name := strings.TrimSuffix(strings.TrimPrefix(match, "{"), "}")
		if v, ok := defaults[name]; ok { // an empty default is a valid value
			return v
		}
		return match
	})
	if unresolved := reServerVar.FindStringSubmatch(out); unresolved != nil {
		return "", fmt.Errorf("server URL %q has unresolved variable {%s}", rawURL, unresolved[1])
	}
	return strings.TrimRight(out, "/"), nil
}

// Options controls generation.
type Options struct {
	OutDir  string // directory to write into
	EnvName string // environment name for the generated http-client.env.json (default "dev")
	Force   bool   // overwrite existing files
}

// Import reads an OpenAPI document (JSON or YAML) and writes .http files.
func Import(specPath string, opts Options) (*Result, error) {
	data, err := os.ReadFile(specPath) //nolint:gosec // the OpenAPI spec the user named on the command line
	if err != nil {
		return nil, err
	}
	doc, err := parseDocument(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", specPath, err)
	}
	if opts.EnvName == "" {
		opts.EnvName = "dev"
	}
	if opts.OutDir == "" {
		opts.OutDir = "."
	}
	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil { //nolint:gosec // a scaffolded project directory the user browses and edits
		return nil, err
	}

	baseURL, err := doc.baseURL()
	if err != nil {
		return nil, err
	}
	byTag, tagOrder := doc.operationsByTag()
	// Everything that resolves references has run by now; stop before
	// writing any file if the document was structurally broken.
	if doc.err != nil {
		return nil, fmt.Errorf("%s: %w", specPath, doc.err)
	}

	w := &writer{opts: opts, specPath: specPath, res: &Result{BaseURL: baseURL}}
	usedFiles := map[string]bool{}
	for _, tag := range tagOrder {
		if err := w.writeTag(fileStem(tag, usedFiles), tag, byTag[tag]); err != nil {
			return nil, err
		}
	}
	if err := w.writeEnv(); err != nil {
		return nil, err
	}
	return w.res, nil
}

// baseURL resolves the first server that declares a URL; without one it is
// https://example.com.
func (d *document) baseURL() (string, error) {
	for _, srv := range d.items(d.get(d.root, "servers")) {
		if strings.TrimSpace(str(d.get(srv, "url"))) == "" {
			continue
		}
		return d.resolveServerURL(srv)
	}
	return "https://example.com", nil
}

// operationsByTag reads every operation in path order, named uniquely across
// the document, and groups them by first tag ("api" when untagged). The tags
// come back sorted.
func (d *document) operationsByTag() (map[string][]*operation, []string) {
	byTag := map[string][]*operation{}
	var tagOrder []string
	seenNames := map[string]int{}
	for _, pathEntry := range d.entries(d.get(d.root, "paths")) {
		path := pathEntry.key
		item := d.resolve(pathEntry.value)
		shared := d.parameters(d.get(item, "parameters"))
		for _, e := range d.entries(item) {
			if !httpMethods[e.key] {
				continue
			}
			op := d.resolve(e.value)
			o := d.operation(path, strings.ToUpper(e.key), op, shared)
			o.Name = uniqueName(requestName(o, o.Method, path), seenNames)
			tag := "api"
			if tags := d.items(d.get(op, "tags")); len(tags) > 0 {
				tag = str(tags[0])
			}
			if _, ok := byTag[tag]; !ok {
				tagOrder = append(tagOrder, tag)
			}
			byTag[tag] = append(byTag[tag], o)
		}
	}
	sort.Strings(tagOrder)
	return byTag, tagOrder
}

// fileStem names a tag's .http file: the tag in kebab case, numbered when an
// earlier tag already took that name (foo/bar and foo-bar both kebab to
// foo-bar).
func fileStem(tag string, used map[string]bool) string {
	base := kebab(tag)
	name := base
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}

// writer writes the generated files into opts.OutDir and records each one,
// written or kept, on res.
type writer struct {
	opts     Options
	specPath string
	res      *Result
}

// kept reports whether path already exists and must be left alone because
// Force is off; a kept path is recorded as skipped.
func (w *writer) kept(path string) bool {
	if _, err := os.Stat(path); err == nil && !w.opts.Force {
		w.res.Skipped = append(w.res.Skipped, path)
		return true
	}
	return false
}

// writeTag writes one tag's requests to name.http, with the upload
// placeholders and body files they refer to.
func (w *writer) writeTag(name, tag string, ops []*operation) error {
	file := filepath.Join(w.opts.OutDir, name+".http")
	if w.kept(file) {
		return nil
	}
	var b strings.Builder
	// A label first, so a tag spelled like a directive (`@auth ...`)
	// stays a comment; the file name is spec-controlled too.
	fmt.Fprintf(&b, "# Tag %q — generated by `apic import` from %q. Edit freely.\n", oneLine(tag), oneLine(filepath.Base(w.specPath)))
	for _, o := range ops {
		if err := w.writeUploads(name, o); err != nil {
			return err
		}
		if err := w.writeBodyFile(name, o); err != nil {
			return err
		}
		b.WriteString(o.render())
		w.res.Requests++
	}
	if err := os.WriteFile(file, []byte(b.String()), 0o644); err != nil { //nolint:gosec // a .http file the user edits and commits
		return err
	}
	w.res.Files = append(w.res.Files, file)
	return nil
}

// writeUploads writes a placeholder file for each file part of o's
// multipart body and points the part at it.
func (w *writer) writeUploads(name string, o *operation) error {
	for i := range o.Parts {
		p := &o.Parts[i]
		if !p.File {
			continue
		}
		// A placeholder the user replaces with the real upload, so
		// the generated request validates and runs as written.
		side := name + "." + o.Name + "." + kebab(p.Name) + ".bin"
		sidePath := filepath.Join(w.opts.OutDir, side)
		p.Path = "./" + side
		if w.kept(sidePath) {
			continue
		}
		content := "replace this file with the " + p.Name + " to upload\n"
		//nolint:gosec // a placeholder upload the user replaces; it holds no credentials
		if err := os.WriteFile(sidePath, []byte(content), 0o644); err != nil {
			return err
		}
		w.res.Files = append(w.res.Files, sidePath)
	}
	return nil
}

// writeBodyFile writes o's body to a file of its own when it must be sent
// verbatim, and points o at it.
func (w *writer) writeBodyFile(name string, o *operation) error {
	if !o.BodyRaw {
		return nil
	}
	// The runner renders inline bodies as templates; a literal
	// {{...}} in an example must stay literal, so it goes into a
	// body file read verbatim (`< file`, not `<@ file`).
	ext := ".txt"
	if isJSON(o.ContentType) {
		ext = ".json"
	}
	side := name + "." + o.Name + ".body" + ext
	sidePath := filepath.Join(w.opts.OutDir, side)
	o.BodyFile = "./" + side
	if w.kept(sidePath) { // kept, like an existing .http file
		return nil
	}
	//nolint:gosec // a request body file the user edits and commits; it holds no credentials
	if err := os.WriteFile(sidePath, []byte(o.Body), 0o644); err != nil { // verbatim: no added newline
		return err
	}
	w.res.Files = append(w.res.Files, sidePath)
	return nil
}

// writeEnv writes http-client.env.json with the base URL, unless one exists
// and Force is off.
func (w *writer) writeEnv() error {
	envFile := filepath.Join(w.opts.OutDir, "http-client.env.json")
	if _, err := os.Stat(envFile); err != nil || w.opts.Force {
		env := map[string]map[string]string{w.opts.EnvName: {"baseUrl": w.res.BaseURL}}
		data, _ := json.MarshalIndent(env, "", "  ")
		// The public env file: base URLs only. Secrets belong in
		// http-client.private.env.json, which apic writes 0600.
		if err := os.WriteFile(envFile, append(data, '\n'), 0o644); err != nil { //nolint:gosec // public, non-secret project file
			return err
		}
		w.res.EnvFile = envFile
	}
	return nil
}

var httpMethods = map[string]bool{"get": true, "put": true, "post": true, "delete": true, "options": true, "head": true, "patch": true, "trace": true}

// operation is the subset of an OpenAPI operation the generator needs.
type operation struct {
	Path        string
	Method      string
	Name        string
	OperationID string
	Summary     string
	Description string
	Params      []parameter
	Body        string // example request body, "" when none
	ContentType string
	Success     string // first 2xx response code; empty when none is declared
	BodyRaw     bool   // Body holds literal {{ from the spec and must not be rendered as a template
	BodyFile    string // `< file` reference used instead of Body when BodyRaw
	WantsJSON   bool   // a response advertises a JSON content type
	// Parts is a multipart/form-data body, one part per property; Body is
	// then empty and ContentType carries the boundary.
	Parts []formPart
}

// formPart is one part of a generated multipart/form-data body.
type formPart struct {
	Name        string
	Value       string // text content; empty for a file part
	ContentType string // the part's Content-Type, when it has one
	File        bool   // a binary property: the part reads a placeholder file
	Path        string // the `< path` of a file part, set once the file is named
}

// multipartBoundary is the boundary generated multipart bodies declare; it
// is what the REST Client and JetBrains examples use, so it reads familiar.
const multipartBoundary = "WebAppBoundary"

type parameter struct {
	Name     string
	In       string
	Required bool
}

// parameters reads a parameter list ($ref-able entries), in order.
func (d *document) parameters(n *yaml.Node) []parameter {
	var out []parameter
	for _, p := range d.items(n) {
		name, in := str(d.get(p, "name")), str(d.get(p, "in"))
		if name == "" || in == "" {
			continue
		}
		out = append(out, parameter{Name: name, In: in, Required: in == "path" || boolean(d.get(p, "required"))})
	}
	return out
}

func (d *document) operation(path, method string, op *yaml.Node, shared []parameter) *operation {
	// The path lands on the request line, which the runner renders as a
	// template: no line breaks, no template markers, and always rooted so a
	// key such as `@evil.example` cannot rewrite the host.
	path = noTemplate(strings.Join(strings.Fields(path), ""), "%7B", "%7D")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	o := &operation{Path: path, Method: method,
		OperationID: str(d.get(op, "operationId")), Summary: str(d.get(op, "summary")), Description: str(d.get(op, "description"))}
	o.Params = mergeParams(shared, d.parameters(d.get(op, "parameters")))
	o.Body, o.ContentType, o.BodyRaw = d.exampleBody(d.get(op, "requestBody"))
	if o.ContentType != "" && strings.EqualFold(strings.TrimSpace(strings.SplitN(o.ContentType, ";", 2)[0]), "multipart/form-data") {
		o.Parts = d.multipartParts(d.get(op, "requestBody"))
		if len(o.Parts) > 0 {
			o.Body, o.BodyRaw = "", false
			o.ContentType = "multipart/form-data; boundary=" + multipartBoundary
		}
	}
	responses := d.get(op, "responses")
	for _, r := range d.entries(responses) {
		// Only a well-formed status key may become a directive value.
		if strings.HasPrefix(r.key, "2") && reStatusKey.MatchString(r.key) {
			o.Success = r.key
			break
		}
	}
	for _, r := range d.entries(responses) {
		for _, ct := range d.entries(d.get(r.value, "content")) {
			if isJSON(ct.key) {
				o.WantsJSON = true
			}
		}
	}
	return o
}

// mergeParams lists the path-level parameters an operation does not
// override, then its own: an operation parameter overrides a path-level one
// with the same name and location.
func mergeParams(shared, own []parameter) []parameter {
	var out []parameter
	for _, p := range shared {
		overridden := false
		for _, q := range own {
			if q.Name == p.Name && q.In == p.In {
				overridden = true
			}
		}
		if !overridden {
			out = append(out, p)
		}
	}
	return append(out, own...)
}

// render writes the operation as one named request block.
func (o *operation) render() string {
	var b strings.Builder
	o.writeHeading(&b)
	lines := o.paramLines()
	fmt.Fprintf(&b, "%s {{baseUrl}}%s\n", o.Method, lines.path)
	writeQuery(&b, lines.query)
	o.writeHeaders(&b, lines)
	o.writeBody(&b)
	return b.String()
}

// writeHeading writes the ### heading, the request's name and description,
// and the status it asserts.
func (o *operation) writeHeading(b *strings.Builder) {
	title := oneLine(o.Summary) // one line: it is the ### heading
	if title == "" {
		title = o.Method + " " + o.Path
	}
	fmt.Fprintf(b, "\n### %s\n", title)
	fmt.Fprintf(b, "# @name %s\n", o.Name)
	if d := strings.TrimSpace(strings.SplitN(o.Description, "\n", 2)[0]); d != "" && d != title {
		fmt.Fprintf(b, "# @description %s\n", d)
	}
	switch {
	case o.Success == "":
		// No 2xx declared (only `default` or error codes): nothing to assert.
	case strings.HasSuffix(o.Success, "XX") && len(o.Success) == 3:
		// Patterned key such as 2XX: assert the range instead of a literal.
		fmt.Fprintf(b, "# @assert status >= %s00\n# @assert status < %s00\n", o.Success[:1], string(o.Success[0]+1))
	default:
		fmt.Fprintf(b, "# @assert status == %s\n", o.Success)
	}
}

// paramLines is where an operation's parameters land in the generated
// request: the path with its variables filled in, query continuations
// (optional ones prefixed "# "), header lines and cookies.
type paramLines struct {
	path                     string
	query                    []string
	headers                  []string
	cookies, optionalCookies []string
}

// paramLines places each parameter as a template variable in the form its
// location allows.
func (o *operation) paramLines() paramLines {
	l := paramLines{path: o.Path}
	names := paramVarNames(o.Params)
	for _, p := range o.Params {
		v := "{{" + names[p.In+":"+p.Name] + "}}"
		required := p.Required
		// A spec is untrusted input: a name is written in the form its
		// location allows, so it can neither add lines to the generated
		// file nor change the request's meaning (`a&b`, `x;y`).
		name := strings.Join(strings.Fields(p.Name), " ") // no line breaks; spaces are encoded or dropped per location
		switch p.In {
		case "cookie":
			name = reHeaderJunk.ReplaceAllString(name, "") // cookie names are tokens (spaces included)
			if name == "" {
				l.headers = append(l.headers, fmt.Sprintf("# skipped cookie parameter %q: not a valid cookie name", p.Name))
				continue
			}
			if required {
				l.cookies = append(l.cookies, name+"="+v)
			} else {
				l.optionalCookies = append(l.optionalCookies, name+"="+v)
			}
		case "path":
			l.path = strings.ReplaceAll(l.path, "{"+p.Name+"}", v) // the spec's own spelling
		case "query":
			name = queryNameEscaper.Replace(name) // only what would change the query's shape
			if required {
				l.query = append(l.query, name+"="+v)
			} else {
				l.query = append(l.query, "# "+name+"="+v)
			}
		case "header":
			// `#` is a token character but opens a comment in .http files.
			name = strings.TrimLeft(reHeaderJunk.ReplaceAllString(name, ""), "#")
			if name == "" {
				l.headers = append(l.headers, fmt.Sprintf("# skipped header parameter %q: not a valid header name", p.Name))
				continue
			}
			line := name + ": " + v
			if !required {
				line = "# " + line
			}
			l.headers = append(l.headers, line)
		}
	}
	return l
}

// writeQuery writes the query continuation lines, required before optional.
func writeQuery(b *strings.Builder, query []string) {
	// Active continuation lines first: the parser stops reading query
	// continuations at the first comment, so optional parameters follow.
	active := 0
	for _, q := range query {
		if strings.HasPrefix(q, "# ") {
			continue
		}
		sep := "&"
		if active == 0 {
			sep = "?"
		}
		active++
		fmt.Fprintf(b, "    %s%s\n", sep, q)
	}
	shown := active
	for _, q := range query {
		if !strings.HasPrefix(q, "# ") {
			continue
		}
		// Commented parameters are not sent; show the separator a caller
		// would add when uncommenting them in order.
		sep := "&"
		if shown == 0 {
			sep = "?"
		}
		shown++
		fmt.Fprintf(b, "    # %s%s  (optional)\n", sep, strings.TrimPrefix(q, "# "))
	}
}

// writeHeaders writes the Content-Type and Accept headers, then the
// parameters' header and cookie lines.
func (o *operation) writeHeaders(b *strings.Builder, l paramLines) {
	if o.ContentType != "" {
		fmt.Fprintf(b, "Content-Type: %s\n", o.ContentType)
	}
	if o.WantsJSON {
		b.WriteString("Accept: application/json\n")
	}
	for _, h := range l.headers {
		b.WriteString(h + "\n")
	}
	if len(l.cookies) > 0 {
		fmt.Fprintf(b, "Cookie: %s\n", strings.Join(l.cookies, "; "))
	}
	for _, c := range l.optionalCookies {
		fmt.Fprintf(b, "# Cookie: %s  (optional)\n", c)
	}
}

// writeBody writes the request body: multipart parts, a body file
// reference or the inline example, whichever the operation has.
func (o *operation) writeBody(b *strings.Builder) {
	switch {
	case len(o.Parts) > 0:
		b.WriteString("\n")
		for _, p := range o.Parts {
			p.write(b)
		}
		fmt.Fprintf(b, "--%s--\n", multipartBoundary)
	case o.BodyFile != "":
		b.WriteString("\n< " + o.BodyFile + "\n")
	case o.Body != "":
		b.WriteString("\n" + o.Body + "\n")
	}
}

// write writes the part with its boundary line: a file part reads its
// placeholder, a text part carries its value.
func (p formPart) write(b *strings.Builder) {
	fmt.Fprintf(b, "--%s\n", multipartBoundary)
	if p.File {
		fmt.Fprintf(b, "Content-Disposition: form-data; name=%s; filename=%s\n", httpfile.QuoteParam(p.Name), httpfile.QuoteParam(filepath.Base(p.Path)))
	} else {
		fmt.Fprintf(b, "Content-Disposition: form-data; name=%s\n", httpfile.QuoteParam(p.Name))
	}
	if p.ContentType != "" {
		fmt.Fprintf(b, "Content-Type: %s\n", p.ContentType)
	}
	b.WriteString("\n")
	if p.File {
		fmt.Fprintf(b, "< %s\n", p.Path)
	} else {
		b.WriteString(p.Value + "\n")
	}
}

// placeholder is a template expression the importer generates itself
// ({{$uuid}}, {{$isoTimestamp}}); unlike spec text it is meant to render.
type placeholder string

func (p placeholder) MarshalJSON() ([]byte, error) { return json.Marshal(string(p)) }

// hasLiteralBraces reports whether spec-provided text in v contains a
// template marker, which the runner would otherwise try to render.
func hasLiteralBraces(v any) bool {
	switch t := v.(type) {
	case string:
		return strings.Contains(t, "{{") || strings.Contains(t, "}}")
	case *orderedObject:
		for _, k := range t.keys {
			if strings.Contains(k, "{{") || hasLiteralBraces(t.vals[k]) {
				return true
			}
		}
	case []any:
		for _, e := range t {
			if hasLiteralBraces(e) {
				return true
			}
		}
	}
	return false
}

// concretize replaces generated placeholders with fixed sample values, for
// bodies that must be written verbatim because they contain literal {{.
func concretize(v any) any {
	switch t := v.(type) {
	case placeholder:
		if t == "{{$uuid}}" {
			return "00000000-0000-4000-8000-000000000000"
		}
		return "2024-01-01T00:00:00Z"
	case *orderedObject:
		for _, k := range t.keys {
			t.vals[k] = concretize(t.vals[k])
		}
	case []any:
		for i := range t {
			t[i] = concretize(t[i])
		}
	}
	return v
}

// exampleBody builds a request body from the first media type that can
// provide one: its example, first named example, or a value derived from
// the schema. Without any, the first media type's content type is kept.
// raw reports that the body contains literal {{ from the spec and must be
// sent verbatim rather than rendered.
func (d *document) exampleBody(rb *yaml.Node) (body, contentType string, raw bool) {
	mts := d.entries(d.get(rb, "content"))
	if len(mts) == 0 {
		return "", "", false
	}
	for _, mt := range mts {
		ct := noTemplate(oneLine(mt.key), "", "") // a header value the runner renders
		v, selected := d.mediaExample(d.resolve(mt.value))
		if !selected {
			continue
		}
		raw := hasLiteralBraces(v)
		if raw {
			v = concretize(v) // a verbatim body cannot carry placeholders
		}
		if isJSON(ct) {
			data, verbatim := jsonBody(v, raw)
			return data, ct, verbatim
		}
		if s, ok := v.(string); ok {
			return s, ct, raw || splitsBlock(s)
		}
		if p, ok := v.(placeholder); ok {
			return string(p), ct, false
		}
		switch v.(type) {
		case nil:
			return "null", ct, false // an explicit null example is a body
		case int, int64, uint64, float64, bool, json.Number:
			return fmt.Sprint(v), ct, false // a scalar serialises the same under any media type
		}
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]), "application/x-www-form-urlencoded") {
			if obj, ok := v.(*orderedObject); ok {
				// Percent-encoding would turn a {{$uuid}} placeholder into
				// literal text the runner no longer recognises, so form
				// bodies use fixed sample values instead.
				return formEncode(concretize(obj).(*orderedObject)), ct, raw
			}
		}
		if strings.EqualFold(strings.TrimSpace(strings.SplitN(ct, ";", 2)[0]), "multipart/form-data") {
			if _, ok := v.(*orderedObject); ok {
				return "", ct, false // the operation builds the parts from the schema
			}
		}
		// XML and other structured non-JSON bodies cannot be rendered
		// faithfully from a schema: try the next media type rather than emit
		// JSON under a misleading content type.
	}
	return "", noTemplate(oneLine(mts[0].key), "", ""), false
}

// mediaExample takes a media type's example, else its first named example,
// else a value derived from its schema; ok is false when none gives one.
func (d *document) mediaExample(media *yaml.Node) (any, bool) {
	switch {
	case d.getRaw(media, "example") != nil:
		return decode(d.getRaw(media, "example")), true
	case firstExampleValue(d, d.get(media, "examples")) != nil:
		return decode(firstExampleValue(d, d.get(media, "examples"))), true
	case d.get(media, "schema") != nil:
		return d.exampleFromSchema(d.get(media, "schema"), 0)
	}
	return nil, false
}

// jsonBody indents v as a JSON body. A body the .http parser would misread
// is written with fixed sample values instead and reported raw, so it goes
// to a body file.
func jsonBody(v any, raw bool) (string, bool) {
	data, _ := json.MarshalIndent(v, "", "  ")
	if !raw && splitsBlock(string(data)) {
		// Verbatim for the parser's sake: placeholders cannot render there.
		data, _ = json.MarshalIndent(concretize(v), "", "  ")
		raw = true
	}
	return string(data), raw
}

// multipartParts builds one part per property of a multipart/form-data
// request body: a string property with a binary format (or a content media
// type) becomes a file part, anything else a text part with its example.
func (d *document) multipartParts(rb *yaml.Node) []formPart {
	for _, mt := range d.entries(d.get(rb, "content")) {
		if !strings.EqualFold(strings.TrimSpace(strings.SplitN(mt.key, ";", 2)[0]), "multipart/form-data") {
			continue
		}
		media := d.resolve(mt.value)
		schema := d.resolve(d.get(media, "schema"))
		encoding := d.get(media, "encoding")
		var parts []formPart
		for _, p := range d.entries(d.get(schema, "properties")) {
			parts = append(parts, d.propertyPart(p.key, d.resolve(p.value), encoding))
		}
		if len(parts) == 0 {
			// No property list: an example object names the fields, and
			// with nothing at all one placeholder keeps the body valid.
			parts = exampleParts(decode(d.getRaw(media, "example")))
			if len(parts) == 0 {
				parts = []formPart{{Name: "field", Value: "value"}}
			}
		}
		return parts
	}
	return []formPart{{Name: "field", Value: "value"}}
}

// propertyPart builds the part for one schema property: a file part for a
// binary string, otherwise a text part carrying its example. The property's
// encoding entry sets the part's content type.
func (d *document) propertyPart(key string, ps, encoding *yaml.Node) formPart {
	part := formPart{Name: oneLine(key)}
	format := str(d.get(ps, "format"))
	if enc := d.get(encoding, key); enc != nil {
		part.ContentType = oneLine(str(d.get(enc, "contentType")))
	}
	if schemaType(d, ps) == "string" && (format == "binary" || format == "byte" || d.get(ps, "contentMediaType") != nil) {
		part.File = true
		if part.ContentType == "" {
			part.ContentType = oneLine(str(d.get(ps, "contentMediaType")))
		}
		if part.ContentType == "" {
			part.ContentType = "application/octet-stream"
		}
		return part
	}
	v, _ := d.exampleFromSchema(ps, 1)
	switch t := v.(type) {
	case placeholder:
		part.Value = string(t)
	case string:
		part.Value = oneLine(noTemplate(t, "", ""))
	case nil:
		part.Value = ""
	default:
		data, _ := json.Marshal(concretize(v))
		part.Value = string(data)
		if part.ContentType == "" {
			part.ContentType = "application/json"
		}
	}
	return part
}

// exampleParts builds one text part per field of an example object, with
// fixed sample values; anything but an object gives none.
func exampleParts(example any) []formPart {
	obj, ok := example.(*orderedObject)
	if !ok {
		return nil
	}
	var parts []formPart
	for _, k := range obj.keys {
		v := concretize(obj.vals[k])
		part := formPart{Name: oneLine(k)}
		if s, isStr := v.(string); isStr {
			part.Value = oneLine(noTemplate(s, "", ""))
		} else if v != nil {
			data, _ := json.Marshal(v)
			part.Value, part.ContentType = string(data), "application/json"
		}
		parts = append(parts, part)
	}
	return parts
}

// queryNameEscaper encodes the characters that would let a parameter name
// change the structure of the generated query string.
var queryNameEscaper = strings.NewReplacer("%", "%25", "&", "%26", "=", "%3D", "#", "%23", "+", "%2B", ";", "%3B", " ", "%20", "{", "%7B", "}", "%7D")

// noTemplate neutralises {{ and }} in spec text that ends up where the
// runner renders templates, so a spec cannot make a generated request read
// local variables. Braces become open/close, or vanish when empty.
func noTemplate(s, open, close string) string {
	s = strings.ReplaceAll(s, "{{", open+open)
	return strings.ReplaceAll(s, "}}", close+close)
}

// reStatusKey is the OpenAPI responses key grammar: a status code or a
// class pattern such as 2XX.
var reStatusKey = regexp.MustCompile(`^[1-5](\d\d|XX)$`)

// splitsBlock reports whether an inline body would be misread by the .http
// parser: a line starting with ### opens a new request block and a body
// starting with `< ` refers to a file. Such bodies are written to a body
// file instead.
func splitsBlock(body string) bool {
	if t := strings.TrimSpace(body); strings.HasPrefix(t, "< ") || strings.HasPrefix(t, "<@ ") {
		return true
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "###") {
			return true
		}
	}
	return false
}

// oneLine collapses spec text into a single line for use in a comment or
// heading, so a value containing line breaks cannot inject request blocks
// or directives into the generated file.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// formEncode renders an object as application/x-www-form-urlencoded, in
// property order; nested values are JSON-encoded.
func formEncode(obj *orderedObject) string {
	var parts []string
	for _, k := range obj.keys {
		var val string
		switch v := obj.vals[k].(type) {
		case string:
			val = v
		case nil:
			val = ""
		case *orderedObject, []any:
			data, _ := json.Marshal(v)
			val = string(data)
		default:
			val = fmt.Sprint(v)
		}
		parts = append(parts, url.QueryEscape(k)+"="+url.QueryEscape(val))
	}
	return strings.Join(parts, "&")
}

// isJSON reports whether a media type carries JSON; names are
// case-insensitive, so application/JSON and application/problem+JSON count.
func isJSON(mediaType string) bool {
	return strings.Contains(strings.ToLower(mediaType), "json")
}

// firstExampleValue returns the `value` of the first named example that has one.
func firstExampleValue(d *document, examples *yaml.Node) *yaml.Node {
	for _, ex := range d.entries(examples) {
		if v := d.getRaw(ex.value, "value"); v != nil {
			return v
		}
	}
	return nil
}

// exampleFromSchema derives a value from a schema. ok is false only when
// nothing could be derived; a JSON null (type: null or example: null) is a
// real value.
func (d *document) exampleFromSchema(s *yaml.Node, depth int) (any, bool) {
	s = d.resolve(s)
	if s == nil || depth > 6 {
		return nil, false
	}
	if n := d.getRaw(s, "example"); n != nil {
		return decode(n), true
	}
	if ex := d.itemsRaw(d.getRaw(s, "examples")); len(ex) > 0 {
		return decode(ex[0]), true
	}
	if n := d.getRaw(s, "default"); n != nil {
		return decode(n), true
	}
	if en := d.itemsRaw(d.getRaw(s, "enum")); len(en) > 0 {
		return decode(en[0]), true
	}
	typ := schemaType(d, s)
	props := d.entries(d.get(s, "properties"))
	// allOf: every branch contributes, so object branches are merged in
	// order (later keys win) together with the properties declared here.
	if sub := d.items(d.get(s, "allOf")); len(sub) > 0 {
		merged := &orderedObject{}
		var scalar any
		object, found := false, false
		for _, branch := range sub {
			v, ok := d.exampleFromSchema(branch, depth+1)
			if !ok {
				continue
			}
			if obj, isObj := v.(*orderedObject); isObj {
				for _, k := range obj.keys {
					merged.set(k, obj.vals[k])
				}
				object = true
			} else if !found {
				scalar, found = v, true
			}
		}
		if object || typ == "object" || len(props) > 0 {
			for _, p := range props {
				v, _ := d.exampleFromSchema(p.value, depth+1)
				merged.set(p.key, v)
			}
			return merged, true
		}
		if found {
			return scalar, true
		}
	}
	for _, key := range []string{"oneOf", "anyOf"} {
		if sub := d.items(d.get(s, key)); len(sub) > 0 {
			return d.exampleFromSchema(sub[0], depth+1)
		}
	}
	if typ == "" && len(props) > 0 {
		typ = "object"
	}
	switch typ {
	case "object":
		obj := &orderedObject{}
		for _, p := range props {
			v, _ := d.exampleFromSchema(p.value, depth+1)
			obj.set(p.key, v)
		}
		return obj, true
	case "array":
		if items := d.get(s, "items"); items != nil {
			v, _ := d.exampleFromSchema(items, depth+1)
			return []any{v}, true
		}
		return []any{}, true
	case "integer":
		return 1, true
	case "number":
		return 1.5, true
	case "boolean":
		return true, true
	case "null":
		return nil, true
	case "string":
		switch str(d.get(s, "format")) {
		case "date-time":
			return placeholder("{{$isoTimestamp}}"), true
		case "date":
			return "2026-01-01", true
		case "email":
			return "user@example.com", true
		case "uuid":
			return placeholder("{{$uuid}}"), true
		case "uri", "url":
			return "https://example.com", true
		}
		return "string", true
	}
	return nil, false
}

// schemaType returns the schema type, taking the first non-null entry of an
// OpenAPI 3.1 type array.
func schemaType(d *document, s *yaml.Node) string {
	t := d.get(s, "type")
	if t == nil {
		return ""
	}
	if t.Kind == yaml.SequenceNode {
		for _, n := range d.items(t) {
			if v := str(n); v != "" && v != "null" {
				return v
			}
		}
		if len(d.items(t)) > 0 {
			return "null"
		}
		return ""
	}
	return str(t)
}

var reNonWord = regexp.MustCompile(`[^A-Za-z0-9]+`)
var reServerVar = regexp.MustCompile(`\{([^{}]+)\}`)

func kebab(s string) string {
	s = reNonWord.ReplaceAllString(s, "-")
	s = strings.Trim(strings.ToLower(s), "-")
	if s == "" {
		return "api"
	}
	return s
}

// varName turns a parameter name into a template variable name: camelCase
// letters and digits only, never `$`-prefixed (that is the built-in
// namespace), never starting with a digit and never the reserved baseUrl.
func varName(s string) string {
	s = reVarJunk.ReplaceAllString(s, "")
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' || r == '.' || r == ' ' })
	if len(parts) == 0 {
		return "value"
	}
	parts[0] = strings.ToLower(parts[0][:1]) + parts[0][1:]
	for i := 1; i < len(parts); i++ {
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	name := strings.Join(parts, "")
	if name[0] >= '0' && name[0] <= '9' {
		name = "p" + name
	}
	if strings.EqualFold(name, "baseUrl") {
		name += "Param"
	}
	return name
}

var reVarJunk = regexp.MustCompile(`[^A-Za-z0-9_. -]`)

// reHeaderJunk matches anything that is not an RFC 7230 token character.
var reHeaderJunk = regexp.MustCompile("[^!#$%&'*+.^_`|~0-9A-Za-z-]")

func requestName(op *operation, method, path string) string {
	if op.OperationID != "" {
		return kebab(splitCamel(op.OperationID))
	}
	p := strings.NewReplacer("{", "by-", "}", "").Replace(path)
	return kebab(strings.ToLower(method) + " " + p)
}

// paramVarNames assigns a variable name to every parameter of an operation,
// disambiguating names that would otherwise collide (user-id and user_id
// both camel-case to userId) with the parameter location and, if needed, a
// counter. Keys are "<in>:<name>".
func paramVarNames(params []parameter) map[string]string {
	out := map[string]string{}
	used := map[string]bool{}
	for _, p := range params {
		base := varName(p.Name)
		name := base
		if used[name] {
			name = base + strings.ToUpper(p.In[:1]) + p.In[1:]
		}
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s%d", base, i)
		}
		used[name] = true
		out[p.In+":"+p.Name] = name
	}
	return out
}

var reCamel = regexp.MustCompile(`([a-z0-9])([A-Z])`)

func splitCamel(s string) string {
	return reCamel.ReplaceAllString(s, "$1-$2")
}

func uniqueName(name string, seen map[string]int) string {
	seen[name]++
	if seen[name] == 1 {
		return name
	}
	return fmt.Sprintf("%s-%d", name, seen[name])
}

// orderedObject marshals to a JSON object keeping schema property order.
type orderedObject struct {
	keys []string
	vals map[string]any
}

func (o *orderedObject) set(k string, v any) {
	if o.vals == nil {
		o.vals = map[string]any{}
	}
	if _, ok := o.vals[k]; !ok {
		o.keys = append(o.keys, k)
	}
	o.vals[k] = v
}

func (o *orderedObject) MarshalJSON() ([]byte, error) {
	var b strings.Builder
	b.WriteByte('{')
	for i, k := range o.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, _ := json.Marshal(k)
		vb, err := json.Marshal(o.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		b.Write(vb)
	}
	b.WriteByte('}')
	return []byte(b.String()), nil
}
