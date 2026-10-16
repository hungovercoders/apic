package snippet

import (
	"strconv"
	"strings"

	"github.com/hungovercoders/apic/internal/runner"
)

// python renders a script using the requests library.
func python(m *request) string {
	var b strings.Builder
	for _, n := range m.notes(true, true, true) {
		b.WriteString("# " + n + "\n")
	}
	env := usesEnv(m)
	if env {
		b.WriteString("import os\n\n")
	}
	b.WriteString("import requests\n")
	if m.Cred != nil && m.Cred.Kind == "digest" {
		b.WriteString("from requests.auth import HTTPDigestAuth\n")
	}
	b.WriteString("\n")
	args := []string{pyQuote(m.Method), pyQuote(m.URL)}
	args = append(args, pyAuthArgs(m)...)
	args = append(args, pyPartArgs(m)...)
	if m.Body != "" {
		args = append(args, "data="+pyQuote(m.Body)+".encode()")
	}
	args = append(args, pyTLSArgs(m.TLS)...)
	if m.Proxy != "" {
		args = append(args, "proxies={\"http\": "+pyQuote(m.Proxy)+", \"https\": "+pyQuote(m.Proxy)+"}")
	}
	b.WriteString("response = requests.request(\n    " + strings.Join(args, ",\n    ") + ",\n)\n")
	b.WriteString("print(response.status_code)\nprint(response.text)")
	return b.String()
}

// pyAuthArgs is the headers= argument, with a header credential last, then
// the params= or auth= one a query, basic or digest credential needs.
func pyAuthArgs(m *request) []string {
	var args, headers []string
	for _, h := range m.Headers {
		headers = append(headers, "        "+pyQuote(h.Name)+": "+pyQuote(h.Value)+",")
	}
	c := m.Cred
	if c != nil && c.Kind == "header" {
		headers = append(headers, "        "+pyQuote(c.Name)+": "+pyConcat(c.Prefix, c.Value)+",")
	}
	if len(headers) > 0 {
		args = append(args, "headers={\n"+strings.Join(headers, "\n")+"\n    }")
	}
	if c != nil && c.Kind == "query" {
		args = append(args, "params={"+pyQuote(c.Name)+": "+pyConcat(c.Prefix, c.Value)+"}")
	}
	if c != nil && (c.Kind == "basic" || c.Kind == "digest") {
		pair := pySecret(c.User) + ", " + pySecret(c.Pass)
		if c.Kind == "digest" {
			args = append(args, "auth=HTTPDigestAuth("+pair+")")
		} else {
			args = append(args, "auth=("+pair+")")
		}
	}
	return args
}

// pyPartArgs is the data= and files= arguments for a multipart body.
func pyPartArgs(m *request) []string {
	var args, data, files []string
	for _, p := range m.Parts {
		if p.File == "" {
			v := p.Value
			if m.Redact {
				v = "***"
			}
			data = append(data, "        "+pyQuote(p.Name)+": "+pyQuote(v)+",")
			continue
		}
		files = append(files, "        "+pyQuote(p.Name)+": "+pyFile(p)+"),")
	}
	if len(data) > 0 {
		args = append(args, "data={\n"+strings.Join(data, "\n")+"\n    }")
	}
	if len(files) > 0 {
		args = append(args, "files={\n"+strings.Join(files, "\n")+"\n    }")
	}
	return args
}

// pyFile is a file part's tuple, less its closing parenthesis.
func pyFile(p runner.FormPart) string {
	name := p.Filename
	if name == "" {
		name = baseName(p.File)
	}
	entry := "(" + pyQuote(name) + ", open(" + pyQuote(p.File) + ", \"rb\")"
	if p.ContentType != "" {
		entry += ", " + pyQuote(p.ContentType)
	}
	return entry
}

// pyTLSArgs is the verify= and cert= arguments for apic's TLS settings.
func pyTLSArgs(t *runner.TLSInfo) []string {
	if t == nil {
		return nil
	}
	var args []string
	switch {
	case t.Insecure:
		args = append(args, "verify=False")
	case t.CAFile != "":
		args = append(args, "verify="+pyQuote(t.CAFile))
	}
	if t.CertFile != "" {
		if t.KeyFile != "" && t.KeyFile != t.CertFile {
			args = append(args, "cert=("+pyQuote(t.CertFile)+", "+pyQuote(t.KeyFile)+")")
		} else {
			args = append(args, "cert="+pyQuote(t.CertFile))
		}
	}
	return args
}

// pyQuote is a Python string literal. Go's escapes (\n, \t, \xhh, \uhhhh,
// \Uhhhhhhhh) are Python's too.
func pyQuote(s string) string { return strconv.Quote(s) }

func pySecret(v secret) string {
	if v.Env != "" {
		return "os.environ[" + pyQuote(v.Env) + "]"
	}
	return pyQuote(v.Literal)
}

func pyConcat(prefix string, v secret) string {
	if v.Env != "" {
		if prefix == "" {
			return pySecret(v)
		}
		return pyQuote(prefix) + " + " + pySecret(v)
	}
	return pyQuote(prefix + v.Literal)
}

// usesEnv reports whether the credential is read from the environment.
func usesEnv(m *request) bool {
	c := m.Cred
	return c != nil && (c.Value.Env != "" || c.User.Env != "")
}

func baseName(p string) string {
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}
