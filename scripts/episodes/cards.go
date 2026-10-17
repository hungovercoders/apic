package main

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// The cards are HTML in the docs theme, rendered by headless Chromium at
// the canvas size; the thumbnail is the same at YouTube's 1280x720.

const logoSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64" role="img" aria-label="apic"><defs><linearGradient id="g" x1="0" y1="0" x2="1" y2="1"><stop offset="0" stop-color="#7c5cff"/><stop offset="1" stop-color="#ff5ca8"/></linearGradient></defs><rect width="64" height="64" rx="14" fill="url(#g)"/><path d="M14 22 L24 32 L14 42" fill="none" stroke="#fff" stroke-width="6" stroke-linecap="round" stroke-linejoin="round"/><path d="M30 42 L38 22 L46 42" fill="none" stroke="#fff" stroke-width="6" stroke-linecap="round" stroke-linejoin="round"/></svg>`

var cardTmpl = template.Must(template.New("card").Funcs(template.FuncMap{"hl": highlight, "lines": lines}).Parse(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1920px;height:1080px;background:` + bg + `;color:#cdd6f4;font-family:Inter,sans-serif;overflow:hidden}
.wrap{position:absolute;inset:0;display:flex;flex-direction:column;justify-content:center;padding:0 160px 90px;box-sizing:border-box}
.series{font-size:36px;color:#c9bbff;letter-spacing:.02em}
h1{font-size:88px;line-height:1.1;margin:20px 0 28px;font-weight:700;background:linear-gradient(100deg,#7c5cff,#ff5ca8);-webkit-background-clip:text;color:transparent}
h2{font-size:64px;line-height:1.15;margin:0 0 36px;font-weight:700;color:#fff}
.logo{position:absolute;top:72px;right:160px;width:140px;height:140px}
.logo svg{width:140px;height:140px}
code,pre{font-family:"DejaVu Sans Mono",monospace}
pre{font-size:32px;line-height:1.5;margin:0;color:#cdd6f4;white-space:pre-wrap}
pre .hl,td .hl{color:#a6e3a1}
.cmds{display:grid;grid-template-columns:1fr 1fr;gap:28px 48px}
.cmd{background:#181825;border:1px solid #313244;border-radius:16px;padding:26px 32px}
.cmd .os{font-size:26px;color:#c9bbff;margin-bottom:12px}
.cmd pre{font-size:28px;color:#a6e3a1;word-break:break-all}
table{border-collapse:collapse;font-size:30px;width:100%}
td{padding:10px 24px 10px 0;vertical-align:top;border-bottom:1px solid #313244}
td:first-child{white-space:nowrap;color:#a6e3a1;font-family:"DejaVu Sans Mono",monospace;font-size:28px}
.dim{color:#a6adc8}
.foot{position:absolute;left:160px;bottom:120px;font-size:30px;color:#a6adc8}
.foot .hl{color:#7c5cff;font-family:Inter,sans-serif}
</style></head><body><div class="logo">{{.Logo}}</div><div class="wrap">
{{if .Card.Series}}<div class="series">{{.Card.Series}}</div>{{end}}
{{if .Card.Title}}<h1>{{lines .Card.Title}}</h1>{{end}}
{{if .Card.Heading}}<h2>{{.Card.Heading}}</h2>{{end}}
{{if .Card.Commands}}<div class="cmds">{{range .Card.Commands}}<div class="cmd"><div class="os">{{.Label}}</div><pre>{{.Command}}</pre></div>{{end}}</div>{{end}}
{{if .Card.Table}}<table>{{range .Card.Table}}<tr>{{range .}}<td>{{hl .}}</td>{{end}}</tr>{{end}}</table>{{end}}
{{if .Card.Text}}<pre>{{hl .Card.Text}}</pre>{{end}}
{{if .Card.Foot}}<div class="foot">{{hl .Card.Foot}}</div>{{end}}
</div></body></html>`))

var thumbTmpl = template.Must(template.New("thumb").Funcs(template.FuncMap{"lines": lines}).Parse(`<!doctype html><html><head><meta charset="utf-8"><style>
html,body{margin:0;width:1280px;height:720px;background:` + bg + `;overflow:hidden;font-family:Inter,sans-serif;color:#fff}
.l{position:absolute;left:64px;top:64px;width:600px}
.logo svg{width:120px;height:120px}
.n{font-size:150px;font-weight:800;line-height:1;margin-top:24px;background:linear-gradient(100deg,#7c5cff,#ff5ca8);-webkit-background-clip:text;color:transparent}
.t{font-size:64px;font-weight:700;line-height:1.1;margin-top:16px}
.s{font-size:30px;color:#c9bbff;margin-top:22px}
.shot{position:absolute;left:660px;top:90px;width:900px;border-radius:18px;overflow:hidden;box-shadow:0 30px 80px rgba(0,0,0,.6);border:2px solid #45475a;transform:rotate(-3deg)}
.shot img{display:block;width:900px}
</style></head><body>
<div class="l"><div class="logo">{{.Logo}}</div><div class="n">#{{.Lesson}}</div><div class="t">{{lines .Words}}</div><div class="s">From zero to apic</div></div>
<div class="shot"><img src="{{.Shot}}"></div>
</body></html>`))

var reTick = regexp.MustCompile("`([^`]+)`")

// highlight escapes text, then turns backtick spans into highlighted
// spans and dims a leading "$ " prompt on each line.
func highlight(s string) template.HTML {
	esc := template.HTMLEscapeString(s)
	esc = reTick.ReplaceAllString(esc, `<code class="hl">$1</code>`)
	var out []string
	for _, line := range strings.Split(esc, "\n") {
		if strings.HasPrefix(line, "$ ") {
			line = `<span class="dim">$</span>` + line[1:]
		}
		out = append(out, line)
	}
	return template.HTML(strings.Join(out, "\n")) //nolint:gosec // escaped above; the only markup is what this function added
}

// lines escapes text and turns its newlines into line breaks.
func lines(s string) template.HTML {
	return template.HTML(strings.ReplaceAll(template.HTMLEscapeString(s), "\n", "<br>")) //nolint:gosec // escaped above
}

// renderCard writes a card's HTML next to the PNG it becomes.
func renderCard(c *Card, html string) error {
	var b bytes.Buffer
	err := cardTmpl.Execute(&b, struct {
		Logo template.HTML
		Card *Card
	}{template.HTML(logoSVG), c}) //nolint:gosec // a constant of this file
	if err != nil {
		return err
	}
	return os.WriteFile(html, b.Bytes(), 0o600)
}

// renderThumb writes the thumbnail's HTML; shot is the PNG it frames.
func renderThumb(ep *Episode, shot, html string) error {
	var b bytes.Buffer
	err := thumbTmpl.Execute(&b, struct {
		Logo   template.HTML
		Lesson int
		Words  string
		Shot   string
	}{template.HTML(logoSVG), ep.Lesson, ep.Thumbnail.Words, shot}) //nolint:gosec // a constant of this file
	if err != nil {
		return err
	}
	return os.WriteFile(html, b.Bytes(), 0o600)
}

// findChromium is the browser the cards are rendered with: $CHROMIUM, or
// the first of the usual names on PATH.
func findChromium() (string, error) {
	if p := os.Getenv("CHROMIUM"); p != "" {
		return p, nil
	}
	for _, name := range []string{"chromium", "chromium-browser", "google-chrome", "chrome"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no chromium, chromium-browser or google-chrome on PATH; set CHROMIUM to the browser to render the cards with")
}

// screenshot renders an HTML file to a PNG of the given size. Chromium
// refuses to run as root without --no-sandbox, which a container's build
// usually is.
func screenshot(chromium, html, png string, w, h int) error {
	abs, err := filepath.Abs(html)
	if err != nil {
		return err
	}
	args := []string{"--headless=new", "--disable-gpu", "--hide-scrollbars",
		fmt.Sprintf("--window-size=%d,%d", w, h), "--screenshot=" + png, "file://" + abs}
	if os.Geteuid() == 0 {
		args = append([]string{"--no-sandbox"}, args...)
	}
	cmd := exec.Command(chromium, args...) //nolint:gosec // the browser found on PATH or named by CHROMIUM, on a file this process wrote; nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chromium: %w\n%s", err, out)
	}
	if _, err := os.Stat(png); err != nil {
		return fmt.Errorf("chromium wrote nothing for %s", html)
	}
	return nil
}
