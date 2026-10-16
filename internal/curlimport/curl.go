// Package curlimport turns a curl command line into a `.http` request
// block, the reverse of `apic curl`: the common flags map onto the request
// line, headers, body and auth; the rest is reported, never fatal.
package curlimport

import (
	"errors"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/hungovercoders/apic/internal/httpfile"
)

// Request is what a curl command line asked for.
type Request struct {
	Method   string
	URL      string
	Headers  []Header
	Body     string // inline body
	BodyFile string // `-d @file`: sent as `< ./file`
	Parts    []Part // -F fields, a multipart body
	User     string // -u user:password, as `# @auth basic`
	Insecure bool   // -k
	Follow   bool   // -L
	// HTTPVersion is HTTP/1.1 for --http1.1 and HTTP/2 for --http2, written
	// on the request line; empty otherwise.
	HTTPVersion string
	Warnings    []string
}

// Header is one -H header.
type Header struct{ Name, Value string }

// Part is one -F field.
type Part struct {
	Name        string
	Value       string
	File        string // name=@path
	Filename    string // ;filename=
	ContentType string // ;type=
}

// Parse reads a curl command line, with its shell quoting and line
// continuations, and returns the request it describes. Unknown flags are
// warnings, not errors; an unreadable line or a missing URL is an error.
func Parse(command string) (*Request, error) {
	tokens, err := tokenize(command)
	if err != nil {
		return nil, err
	}
	if len(tokens) > 0 && (tokens[0] == "curl" || strings.HasSuffix(tokens[0], "/curl")) {
		tokens = tokens[1:]
	}
	p := &parser{r: &Request{}}
	if err := p.readTokens(tokens); err != nil {
		return nil, err
	}
	if err := p.resolveURL(); err != nil {
		return nil, err
	}
	p.resolveBody()
	p.resolveMethodAndContentType()
	return p.r, nil
}

// parser is the state Parse builds up while it reads the flags, before it
// settles the URL, the body, the method and the content type.
type parser struct {
	r              *Request
	data           []string
	dataFile       string
	get, head      bool
	contentTypeSet bool
	positional     []string
}

func (p *parser) warn(format string, args ...any) {
	p.r.Warnings = append(p.r.Warnings, fmt.Sprintf(format, args...))
}

// readTokens walks the arguments: positional ones are kept for the URL,
// flags are looked up and applied, a bundle of boolean short flags (-sSL)
// is split into its letters.
func (p *parser) readTokens(tokens []string) error {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok == "--" {
			p.positional = append(p.positional, tokens[i+1:]...)
			return nil
		}
		if !strings.HasPrefix(tok, "-") || tok == "-" {
			p.positional = append(p.positional, tok)
			continue
		}
		name, value, attached := splitFlag(tok)
		spec, known := flags[name]
		if !known {
			if bundle := splitBundle(tok); bundle != nil {
				tokens = append(tokens[:i], append(bundle, tokens[i+1:]...)...)
				i--
				continue
			}
			p.warn("unknown flag %s (ignored)", tok)
			continue
		}
		if spec.takesValue && !attached {
			if i+1 >= len(tokens) {
				return fmt.Errorf("%s needs a value", tok)
			}
			i++
			value = tokens[i]
		}
		if apply := handlers[spec.name]; apply != nil {
			apply(p, tok, value)
		}
	}
	return nil
}

// splitBundle turns -sSL into -s -S -L when every letter is a boolean flag
// curl knows, and returns nil otherwise.
func splitBundle(tok string) []string {
	if strings.HasPrefix(tok, "--") || len(tok) <= 2 || !allBoolean(tok[1:]) {
		return nil
	}
	single := make([]string, 0, len(tok)-1)
	for _, c := range tok[1:] {
		single = append(single, "-"+string(c))
	}
	return single
}

// handlers applies each flag meaning (flagSpec.name) to the request being
// built. A meaning with no entry, such as "quiet", is read and dropped.
var handlers = map[string]func(p *parser, tok, value string){
	"request":        func(p *parser, _, v string) { p.r.Method = strings.ToUpper(v) },
	"header":         (*parser).header,
	"data":           (*parser).dataOrFile,
	"data-binary":    (*parser).dataOrFile,
	"data-ascii":     (*parser).dataOrFile,
	"data-raw":       func(p *parser, _, v string) { p.data = append(p.data, v) },
	"data-urlencode": func(p *parser, _, v string) { p.data = append(p.data, urlencodeData(v, p.warn)) },
	"form":           func(p *parser, _, v string) { p.form(v, false) },
	"form-string":    func(p *parser, _, v string) { p.form(v, true) },
	"user":           func(p *parser, _, v string) { p.r.User = v },
	"url":            func(p *parser, _, v string) { p.positional = append(p.positional, v) },
	"json":           (*parser).json,
	"oauth2-bearer":  func(p *parser, _, v string) { p.addHeader("Authorization", "Bearer "+v) },
	"get":            func(p *parser, _, _ string) { p.get = true },
	"head":           func(p *parser, _, _ string) { p.head = true },
	"cookie":         (*parser).cookie,
	"user-agent":     func(p *parser, _, v string) { p.addHeader("User-Agent", v) },
	"referer":        func(p *parser, _, v string) { p.addHeader("Referer", v) },
	"insecure":       func(p *parser, _, _ string) { p.r.Insecure = true },
	"location":       func(p *parser, _, _ string) { p.r.Follow = true },
	// apic decompresses gzip responses on its own, so --compressed needs nothing.
	"http1.1": func(p *parser, _, _ string) { p.r.HTTPVersion = "HTTP/1.1" },
	"http2":   func(p *parser, _, _ string) { p.r.HTTPVersion = "HTTP/2" },
	"tls": func(p *parser, tok, _ string) {
		p.warn("%s: configure tls: in apic.yaml, or pass %s to apic itself", tok, tok)
	},
	"ignored": func(p *parser, tok, _ string) {
		p.warn("%s is about curl's own output or transport and has no place in a request file (ignored)", tok)
	},
}

func (p *parser) addHeader(name, value string) {
	p.r.Headers = append(p.r.Headers, Header{Name: name, Value: value})
}

func (p *parser) header(_, value string) {
	k, v, ok := strings.Cut(value, ":")
	k = strings.TrimSpace(k)
	if !ok {
		if strings.HasSuffix(k, ";") { // curl's spelling of an empty header
			p.r.Headers = append(p.r.Headers, Header{Name: strings.TrimSuffix(k, ";")})
		} else {
			p.warn("header %q is not `Name: value` (ignored)", value)
		}
		return
	}
	if strings.EqualFold(k, "Content-Type") {
		p.contentTypeSet = true
	}
	p.addHeader(k, strings.TrimSpace(v))
}

func (p *parser) dataOrFile(_, value string) {
	if !strings.HasPrefix(value, "@") {
		p.data = append(p.data, value)
		return
	}
	if p.dataFile != "" || len(p.data) > 0 {
		p.warn("more than one body: %s dropped", value)
		return
	}
	p.dataFile = strings.TrimPrefix(value, "@")
}

func (p *parser) form(value string, literal bool) {
	part, ok := parseForm(value, literal)
	if !ok {
		p.warn("form field %q is not name=value (ignored)", value)
		return
	}
	p.r.Parts = append(p.r.Parts, part)
}

func (p *parser) json(_, value string) {
	p.data = append(p.data, value)
	if !p.contentTypeSet {
		p.addHeader("Content-Type", "application/json")
		p.contentTypeSet = true
	}
	p.addHeader("Accept", "application/json")
}

func (p *parser) cookie(_, value string) {
	if !strings.Contains(value, "=") {
		p.warn("-b %s names a cookie file, which apic does not read; switch the jar on with --cookies instead", value)
		return
	}
	p.addHeader("Cookie", value)
}

// resolveURL picks the URL: the argument that looks like one; when none
// does, the first, with curl's own http:// default. Anything else is
// reported.
func (p *parser) resolveURL() error {
	for _, arg := range p.positional {
		if strings.Contains(arg, "://") && p.r.URL == "" {
			p.r.URL = arg
		}
	}
	for _, arg := range p.positional {
		switch {
		case arg == p.r.URL && p.r.URL != "":
		case p.r.URL == "":
			p.r.URL = "http://" + arg
		default:
			p.warn("extra argument %q (ignored)", arg)
		}
	}
	if p.r.URL == "" {
		return errors.New("no URL in the command")
	}
	return nil
}

// resolveBody joins the data arguments into the body, or, under -G, onto
// the query string.
func (p *parser) resolveBody() {
	body := strings.Join(p.data, "&")
	switch {
	case p.get && (body != "" || p.dataFile != ""):
		if p.dataFile != "" {
			p.warn("-G with a body file: the file is not appended to the URL")
		}
		if body != "" {
			sep := "?"
			if strings.Contains(p.r.URL, "?") {
				sep = "&"
			}
			p.r.URL += sep + body
		}
	case p.dataFile != "":
		p.r.BodyFile = p.dataFile
	default:
		p.r.Body = body
	}
}

// resolveMethodAndContentType fills in what curl would infer: the method
// from -I, -G and the body, and the content type of a body or a form.
func (p *parser) resolveMethodAndContentType() {
	r := p.r
	hasBody := r.Body != "" || r.BodyFile != "" || len(r.Parts) > 0
	if r.Method == "" {
		switch {
		case p.head:
			r.Method = "HEAD"
		case hasBody && !p.get:
			r.Method = "POST"
		default:
			r.Method = "GET"
		}
	}
	if len(r.Parts) > 0 {
		r.Headers = withoutContentType(r.Headers)
		p.addHeader("Content-Type", "multipart/form-data; boundary=WebAppBoundary")
	} else if (r.Body != "" || r.BodyFile != "") && !p.contentTypeSet {
		// curl's default for -d.
		p.addHeader("Content-Type", "application/x-www-form-urlencoded")
	}
}

type flagSpec struct {
	name       string
	takesValue bool
}

// flags maps every curl option apic reads (or knowingly ignores) to what
// it means. Long options are keyed with their dashes, short ones too.
var flags = map[string]flagSpec{
	"-X": {"request", true}, "--request": {"request", true},
	"-H": {"header", true}, "--header": {"header", true},
	"-d": {"data", true}, "--data": {"data", true}, "--data-ascii": {"data-ascii", true},
	"--data-raw": {"data-raw", true}, "--data-binary": {"data-binary", true}, "--data-urlencode": {"data-urlencode", true},
	"-F": {"form", true}, "--form": {"form", true}, "--form-string": {"form-string", true},
	"-u": {"user", true}, "--user": {"user", true},
	"--url": {"url", true},
	"-G":    {"get", false}, "--get": {"get", false},
	"-I": {"head", false}, "--head": {"head", false},
	"-b": {"cookie", true}, "--cookie": {"cookie", true},
	"-A": {"user-agent", true}, "--user-agent": {"user-agent", true},
	"-e": {"referer", true}, "--referer": {"referer", true},
	"-k": {"insecure", false}, "--insecure": {"insecure", false},
	"-L": {"location", false}, "--location": {"location", false},
	"--compressed": {"compressed", false},
	"-s":           {"quiet", false}, "--silent": {"quiet", false}, "-S": {"quiet", false}, "--show-error": {"quiet", false},
	"-v": {"quiet", false}, "--verbose": {"quiet", false}, "-i": {"quiet", false}, "--include": {"quiet", false},
	"-#": {"quiet", false}, "--progress-bar": {"quiet", false}, "-f": {"quiet", false}, "--fail": {"quiet", false},
	"--cacert": {"tls", true}, "--cert": {"tls", true}, "--key": {"tls", true}, "-E": {"tls", true},
	"--json": {"json", true}, "--oauth2-bearer": {"oauth2-bearer", true},
	// Value-taking options apic knows nothing to do with: named so their
	// value is never mistaken for the URL.
	"--max-redirs": {"ignored", true}, "--resolve": {"ignored", true}, "-T": {"ignored", true}, "--upload-file": {"ignored", true},
	"--limit-rate": {"ignored", true}, "-U": {"ignored", true}, "--proxy-user": {"ignored", true}, "--interface": {"ignored", true},
	"--dns-servers": {"ignored", true}, "--unix-socket": {"ignored", true}, "--abstract-unix-socket": {"ignored", true},
	"-K": {"ignored", true}, "--config": {"ignored", true}, "--keepalive-time": {"ignored", true}, "--speed-limit": {"ignored", true},
	"--speed-time": {"ignored", true}, "--ciphers": {"ignored", true}, "--tls-max": {"ignored", true}, "--pinnedpubkey": {"ignored", true},
	"-r": {"ignored", true}, "--range": {"ignored", true}, "--stderr": {"ignored", true}, "--trace": {"ignored", true},
	"--trace-ascii": {"ignored", true}, "-z": {"ignored", true}, "--time-cond": {"ignored", true}, "--aws-sigv4": {"ignored", true},
	"--proto": {"ignored", true}, "--proto-default": {"ignored", true}, "--url-query": {"ignored", true}, "--variable": {"ignored", true},
	"--connect-to": {"ignored", true}, "--happy-eyeballs-timeout-ms": {"ignored", true}, "--retry-delay": {"ignored", true},
	"--retry-max-time": {"ignored", true}, "--max-filesize": {"ignored", true}, "-y": {"ignored", true}, "-Y": {"ignored", true},
	"--ntlm": {"quiet", false}, "--digest": {"quiet", false}, "--negotiate": {"quiet", false}, "--basic": {"quiet", false}, "--anyauth": {"quiet", false},
	"--http1.1": {"http1.1", false}, "--http1.0": {"http1.1", false}, "-0": {"http1.1", false}, "--http2": {"http2", false}, "--http3": {"quiet", false}, "--tlsv1.2": {"quiet", false}, "--tlsv1.3": {"quiet", false},
	"-4": {"quiet", false}, "-6": {"quiet", false}, "-N": {"quiet", false}, "--no-buffer": {"quiet", false}, "--retry-all-errors": {"quiet", false},
	"-o": {"ignored", true}, "--output": {"ignored", true}, "-w": {"ignored", true}, "--write-out": {"ignored", true},
	"-x": {"ignored", true}, "--proxy": {"ignored", true}, "--max-time": {"ignored", true}, "-m": {"ignored", true},
	"--connect-timeout": {"ignored", true}, "--retry": {"ignored", true}, "-c": {"ignored", true}, "--cookie-jar": {"ignored", true},
	"-D": {"ignored", true}, "--dump-header": {"ignored", true}, "-O": {"ignored", false}, "--remote-name": {"ignored", false},
}

// splitFlag separates `--name=value` and `-Xvalue` forms.
func splitFlag(tok string) (name, value string, attached bool) {
	if strings.HasPrefix(tok, "--") {
		if k, v, ok := strings.Cut(tok, "="); ok {
			return k, v, true
		}
		return tok, "", false
	}
	if len(tok) > 2 {
		short := tok[:2]
		if spec, ok := flags[short]; ok && spec.takesValue {
			return short, tok[2:], true
		}
	}
	return tok, "", false
}

func allBoolean(chars string) bool {
	for _, c := range chars {
		spec, ok := flags["-"+string(c)]
		if !ok || spec.takesValue {
			return false
		}
	}
	return true
}

// urlencodeData handles --data-urlencode's four spellings: content,
// =content, name=content and name@file.
func urlencodeData(v string, warn func(string, ...any)) string {
	if name, file, ok := strings.Cut(v, "@"); ok && !strings.Contains(name, "=") {
		warn("--data-urlencode %s reads a file, which apic does not do here; paste its content", v)
		return name + "=" + url.QueryEscape("@"+file)
	}
	if strings.HasPrefix(v, "=") {
		return escape(v[1:])
	}
	if name, content, ok := strings.Cut(v, "="); ok {
		return name + "=" + escape(content)
	}
	return escape(v)
}

// escape percent-encodes a value, keeping {{placeholders}} for the runner.
func escape(s string) string {
	return strings.NewReplacer("%7B%7B", "{{", "%7D%7D", "}}").Replace(url.QueryEscape(s))
}

// parseForm reads a -F field: name=value, name=@file[;type=..][;filename=..]
// or name=<file (the content of a file as text).
func parseForm(v string, literal bool) (Part, bool) {
	name, rest, ok := strings.Cut(v, "=")
	if !ok || name == "" {
		return Part{}, false
	}
	p := Part{Name: name}
	if literal || (!strings.HasPrefix(rest, "@") && !strings.HasPrefix(rest, "<")) {
		p.Value = rest
		return p, true
	}
	spec := strings.Split(rest, ";")
	p.File = strings.TrimLeft(spec[0], "@<")
	for _, opt := range spec[1:] {
		k, val, _ := strings.Cut(opt, "=")
		switch strings.TrimSpace(k) {
		case "type":
			p.ContentType = strings.TrimSpace(val)
		case "filename":
			p.Filename = strings.TrimSpace(val)
		}
	}
	if strings.HasPrefix(rest, "<") {
		// `<file` sends the file's content as a text field: same thing
		// from a request file's point of view, without a filename.
		p.Filename = ""
	} else if p.Filename == "" {
		p.Filename = path.Base(strings.ReplaceAll(p.File, "\\", "/"))
	}
	return p, true
}

func withoutContentType(hs []Header) []Header {
	out := hs[:0:0]
	for _, h := range hs {
		if !strings.EqualFold(h.Name, "Content-Type") {
			out = append(out, h)
		}
	}
	return out
}

// tokenize splits a shell command line: whitespace separates words,
// single quotes are literal, double quotes keep spaces and take backslash
// escapes, $'...' takes C escapes, and a backslash before a newline joins
// lines.
func tokenize(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if inWord {
				out = append(out, cur.String())
				cur.Reset()
				inWord = false
			}
			continue
		}
		read := quoteReader(s, i)
		if read == nil {
			cur.WriteByte(c)
			inWord = true
			continue
		}
		last, word, err := read(s, i, &cur)
		if err != nil {
			return nil, err
		}
		i = last
		inWord = inWord || word
	}
	if inWord {
		out = append(out, cur.String())
	}
	return out, nil
}

// A quoteRead consumes one shell construct that starts at s[i], writes
// what it stands for to cur, and returns the index of the last byte it
// consumed and whether it contributed to a word (a line continuation does
// not).
type quoteRead func(s string, i int, cur *strings.Builder) (last int, word bool, err error)

// quoteReader picks the reader for the construct at s[i], or nil for a
// plain byte.
func quoteReader(s string, i int) quoteRead {
	switch {
	case s[i] == '\\':
		return readEscape
	case s[i] == '\'':
		return readSingleQuoted
	case s[i] == '"':
		return readDoubleQuoted
	case s[i] == '$' && i+1 < len(s) && s[i+1] == '\'':
		return readANSIQuoted
	}
	return nil
}

// readEscape handles a backslash outside quotes: a line continuation
// (a backslash before a newline or CRLF) disappears, anything else is
// taken literally.
func readEscape(s string, i int, cur *strings.Builder) (int, bool, error) {
	if i+1 >= len(s) {
		return 0, false, errors.New("trailing backslash")
	}
	i++
	if s[i] == '\n' {
		return i, false, nil // line continuation
	}
	if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
		return i + 1, false, nil
	}
	cur.WriteByte(s[i])
	return i, true, nil
}

// readSingleQuoted takes everything up to the next single quote literally.
func readSingleQuoted(s string, i int, cur *strings.Builder) (int, bool, error) {
	end := strings.IndexByte(s[i+1:], '\'')
	if end < 0 {
		return 0, false, errors.New("unterminated single quote")
	}
	cur.WriteString(s[i+1 : i+1+end])
	return i + end + 1, true, nil
}

// readDoubleQuoted honours the backslash escapes the shell keeps inside
// double quotes: a backslash before ", another backslash, $, ` or a newline.
func readDoubleQuoted(s string, i int, cur *strings.Builder) (int, bool, error) {
	for i++; i < len(s); i++ {
		if s[i] == '"' {
			return i, true, nil
		}
		if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\"\\$`\n", s[i+1]) >= 0 {
			i++
			if s[i] == '\n' {
				continue
			}
		}
		cur.WriteByte(s[i])
	}
	return 0, false, errors.New("unterminated double quote")
}

// readANSIQuoted reads bash's $'...' form, with its n, t and r escapes.
func readANSIQuoted(s string, i int, cur *strings.Builder) (int, bool, error) {
	for i += 2; i < len(s); i++ {
		if s[i] == '\'' {
			return i, true, nil
		}
		if s[i] == '\\' && i+1 < len(s) {
			i++
			cur.WriteByte(ansiEscape(s[i]))
			continue
		}
		cur.WriteByte(s[i])
	}
	return 0, false, errors.New("unterminated $'...' quote")
}

func ansiEscape(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	}
	return c
}

var reNonWord = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Name derives a request name from the method and path when --name is not
// given: `POST /todos/42` becomes post-todos-42.
func Name(r *Request) string {
	u, err := url.Parse(r.URL)
	p := ""
	if err == nil {
		p = u.Path
	}
	name := strings.Trim(strings.ToLower(reNonWord.ReplaceAllString(r.Method+" "+p, "-")), "-")
	if name == "" {
		return "request"
	}
	return name
}

// SplitsBlock reports whether the inline body would be misread by the
// .http parser: a line starting with ### opens a new request block and a
// body starting with `< ` refers to a file. Such a body belongs in a side
// file (BodyFile).
func SplitsBlock(body string) bool {
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

// Render writes the request as a `###` block. baseURL, when it prefixes
// the URL, is replaced by {{baseUrl}}.
func Render(r *Request, name, baseURL string) string {
	var b strings.Builder
	rawURL := r.URL
	if baseURL != "" {
		base := strings.TrimRight(baseURL, "/")
		if strings.HasPrefix(rawURL, base+"/") || rawURL == base || strings.HasPrefix(rawURL, base+"?") {
			rawURL = "{{baseUrl}}" + strings.TrimPrefix(rawURL, base)
		}
	}
	fmt.Fprintf(&b, "### %s %s\n", r.Method, urlPath(rawURL))
	fmt.Fprintf(&b, "# @name %s\n", name)
	if r.User != "" {
		u, p, _ := strings.Cut(r.User, ":")
		fmt.Fprintf(&b, "# @auth basic %s %s\n", authArg(u), authArg(p))
	}
	if r.Insecure {
		b.WriteString("# curl was run with --insecure; run this with `apic --insecure` or set tls.verifyHost in apic.yaml\n")
	}
	b.WriteString("# @assert status == 200\n")
	line := r.Method + " " + oneLine(rawURL)
	if r.HTTPVersion != "" {
		line += " " + r.HTTPVersion
	}
	b.WriteString(line + "\n")
	for _, h := range r.Headers {
		fmt.Fprintf(&b, "%s: %s\n", oneLine(h.Name), oneLine(h.Value))
	}
	switch {
	case len(r.Parts) > 0:
		b.WriteString("\n")
		for _, p := range r.Parts {
			b.WriteString("--WebAppBoundary\n")
			if p.File != "" {
				if p.Filename != "" {
					fmt.Fprintf(&b, "Content-Disposition: form-data; name=%s; filename=%s\n", httpfile.QuoteParam(p.Name), httpfile.QuoteParam(p.Filename))
				} else {
					fmt.Fprintf(&b, "Content-Disposition: form-data; name=%s\n", httpfile.QuoteParam(p.Name))
				}
				if p.ContentType != "" {
					fmt.Fprintf(&b, "Content-Type: %s\n", oneLine(p.ContentType))
				}
				fmt.Fprintf(&b, "\n< %s\n", relativeFile(p.File))
				continue
			}
			fmt.Fprintf(&b, "Content-Disposition: form-data; name=%s\n", httpfile.QuoteParam(p.Name))
			if p.ContentType != "" {
				fmt.Fprintf(&b, "Content-Type: %s\n", oneLine(p.ContentType))
			}
			fmt.Fprintf(&b, "\n%s\n", p.Value)
		}
		b.WriteString("--WebAppBoundary--\n")
	case r.BodyFile != "":
		fmt.Fprintf(&b, "\n< %s\n", relativeFile(r.BodyFile))
	case r.Body != "":
		fmt.Fprintf(&b, "\n%s\n", r.Body)
	}
	return b.String()
}

// relativeFile keeps a `< file` reference relative to the .http file, the
// only place the runner reads it from.
func relativeFile(p string) string {
	p = strings.ReplaceAll(p, "\\", "/")
	if strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "/") {
		return p
	}
	return "./" + p
}

func urlPath(rawURL string) string {
	u, err := url.Parse(strings.ReplaceAll(rawURL, "{{baseUrl}}", "http://baseurl"))
	if err != nil || u.Path == "" {
		return oneLine(rawURL)
	}
	return u.Path
}

func authArg(s string) string {
	s = oneLine(s)
	if s == "" {
		return `""`
	}
	if strings.ContainsAny(s, " \t\"") {
		return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"`
	}
	return s
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
