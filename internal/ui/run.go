package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hungovercoders/apic/internal/httpfile"
	"github.com/hungovercoders/apic/internal/output"
	"github.com/hungovercoders/apic/internal/runner"
	"github.com/hungovercoders/apic/internal/snippet"
)

// resultMsg is one finished request (a single run, or one step of a flow).
type resultMsg struct {
	runID int
	req   *httpfile.Request
	res   *runner.Result
	err   error
}

type editorDoneMsg struct{ err error }

type clearStatusMsg struct{ seq int }

func (m *Model) handleKey(msg Key) Cmd {
	switch {
	case m.showHelp:
		return m.helpKey(msg)
	case m.confirmClear:
		return m.confirmKey(msg)
	case m.filtering:
		return m.filterKey(msg)
	}
	for _, a := range m.keyActions() {
		if a.b.matches(msg) {
			return a.do()
		}
	}
	return nil
}

// helpKey closes the help overlay on esc, ? or q and swallows the rest.
func (m *Model) helpKey(msg Key) Cmd {
	k := m.keys
	if k.Esc.matches(msg) || k.Help.matches(msg) || k.Quit.matches(msg) {
		m.showHelp = false
	}
	return nil
}

// confirmKey answers the clear-session prompt: y clears, anything else
// cancels.
func (m *Model) confirmKey(msg Key) Cmd {
	m.confirmClear = false
	if msg.String() == "y" || msg.String() == "Y" {
		return m.clearSession()
	}
	return nil
}

// filterKey edits the filter while it is being typed. The arrows move the
// cursor through the matches, as a fuzzy finder does, so a letter is never
// needed for that. Only a change to the text puts the cursor back on the
// first match; enter, and any key the prompt has no use for, leave it
// where it is.
func (m *Model) filterKey(msg Key) Cmd {
	switch msg.Type {
	case KeyEnter:
		m.filtering = false
	case KeyUp:
		m.move(-1)
	case KeyDown:
		m.move(1)
	case KeyCtrlC:
		return m.quit()
	case KeyEsc:
		m.filtering = false
		m.filter = ""
		m.resetCursor()
	case KeyBackspace:
		if r := []rune(m.filter); len(r) > 0 {
			m.filter = string(r[:len(r)-1])
		}
		m.resetCursor()
	case KeyRune:
		m.filter += string(msg.Rune)
		m.resetCursor()
	}
	return nil
}

// resetCursor puts the cursor on the first row the filter leaves visible.
func (m *Model) resetCursor() {
	m.cursor = 0
	m.clampCursor()
	m.selected = m.selectedReq()
}

// keyAction is what one binding does in the main view.
type keyAction struct {
	b  binding
	do func() Cmd
}

// keyActions lists the main view's bindings in the order they are tried;
// the first that matches the key wins.
func (m *Model) keyActions() []keyAction {
	k := m.keys
	setTab := func(t tab) func() Cmd { return func() Cmd { m.tab = t; return nil } }
	do := func(f func()) func() Cmd { return func() Cmd { f(); return nil } }
	return []keyAction{
		{k.Quit, m.quit},
		{k.Help, do(func() { m.showHelp = true })},
		{k.Esc, m.escape},
		{k.Down, do(func() { m.move(1) })},
		{k.Up, do(func() { m.move(-1) })},
		{k.Top, do(func() { m.moveTo(true) })},
		{k.Bottom, do(func() { m.moveTo(false) })},
		{k.Filter, do(func() { m.filtering = true })},
		{k.Run, m.runSelected},
		{k.RunFile, func() Cmd { return m.runFlow(m.fileRequests()) }},
		{k.RunAll, func() Cmd { return m.runFlow(m.allRequests()) }},
		{k.NextTab, do(func() { m.tab = (m.tab + 1) % tabCount })},
		{k.PrevTab, do(func() { m.tab = (m.tab + tabCount - 1) % tabCount })},
		{k.Tab1, setTab(tabPreview)},
		{k.Tab2, setTab(tabResponse)},
		{k.Tab3, setTab(tabChecks)},
		{k.Tab4, setTab(tabSession)},
		{k.Tab5, setTab(tabHistory)},
		{k.Headers, do(func() { m.showHeaders = !m.showHeaders; m.tab = tabResponse })},
		{k.Curl, do(func() { m.codeLang = (m.codeLang + 1) % (len(snippet.Languages) + 1); m.tab = tabResponse })},
		{k.Open, m.openEditor},
		{k.Env, func() Cmd { return m.cycleEnv(1) }},
		{k.PrevEnv, func() Cmd { return m.cycleEnv(-1) }},
		{k.Reload, m.reload},
		{k.Clear, m.askClear},
		{k.LineUp, do(func() { m.vp.scroll(-1) })},
		{k.LineDown, do(func() { m.vp.scroll(1) })},
		{k.PageUp, do(m.vp.halfPageUp)},
		{k.PageDown, do(m.vp.halfPageDown)},
	}
}

// escape cancels a run, else clears the filter, else closes the snippet.
func (m *Model) escape() Cmd {
	switch {
	case m.inflight != nil:
		m.cancelRun()
		return m.setStatus("run cancelled")
	case m.filter != "":
		m.filter = ""
		m.clampCursor()
		m.selected = m.selectedReq()
	case m.codeLang > 0:
		m.codeLang = 0
	}
	return nil
}

func (m *Model) runSelected() Cmd {
	if sel := m.selectedReq(); sel != nil {
		return m.startRun([]*httpfile.Request{sel}, false)
	}
	return nil
}

func (m *Model) runFlow(reqs []*httpfile.Request) Cmd {
	if len(reqs) > 0 {
		return m.startRun(reqs, true)
	}
	return nil
}

// askClear opens the clear-session prompt when there is a session to
// clear and no run using it.
func (m *Model) askClear() Cmd {
	if m.inflight != nil {
		return m.setStatus("busy: wait for the run to finish")
	}
	if m.runner.Session == nil {
		return m.setStatus("session disabled by --no-session")
	}
	m.tab = tabSession
	m.confirmClear = true
	return nil
}

func (m *Model) quit() Cmd {
	m.cancelRun()
	m.quitting = true
	return Quit
}

// startRun begins a run of one or more requests; only one run at a time.
func (m *Model) startRun(reqs []*httpfile.Request, flow bool) Cmd {
	if m.inflight != nil {
		return m.setStatus("busy: a run is in progress (esc cancels)")
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.runSeq++
	m.inflight = &runState{id: m.runSeq, reqs: reqs, flow: flow, started: time.Now(), ctx: ctx, cancel: cancel}
	for _, r := range reqs {
		delete(m.results, r)
		delete(m.errs, r)
	}
	m.codeLang = 0
	m.tab = tabResponse
	// A flow runs the whole file, filter or not. Its rows light up one by
	// one only if they are on screen, so a filter that hides some of them
	// goes now rather than leaving the cursor behind the pane.
	var cleared Cmd
	if m.filter != "" && !m.allVisible(reqs) {
		m.filter, m.filtering = "", false
		cleared = m.setStatus("filter cleared: the run includes requests it hid")
	}
	m.focus(reqs[0])
	if cleared != nil {
		return batch(cleared, m.stepCmd())
	}
	return m.stepCmd()
}

// stepCmd sends the current request of the run in flight. The request goes
// out on its own goroutine so the UI keeps redrawing while it is in flight.
func (m *Model) stepCmd() Cmd {
	rs := m.inflight
	req := rs.reqs[rs.idx]
	id, ctx, r := rs.id, rs.ctx, m.runner
	return async(func() Msg {
		res, err := r.Run(ctx, req)
		return resultMsg{runID: id, req: req, res: res, err: err}
	})
}

func (m *Model) cancelRun() {
	if m.inflight == nil {
		return
	}
	m.inflight.cancel()
	m.inflight = nil
	m.runSeq++
}

func (m *Model) handleResult(msg resultMsg) Cmd {
	rs := m.inflight
	if rs == nil || msg.runID != rs.id {
		return nil
	}
	if msg.res != nil {
		m.results[msg.req] = msg.res
		m.storeDeps(msg.res)
	}
	if msg.err != nil {
		m.errs[msg.req] = msg.err
	}
	m.focus(msg.req)
	rs.idx++
	if rs.idx < len(rs.reqs) {
		return m.stepCmd()
	}
	rs.cancel()
	m.inflight = nil
	m.finishRun(rs)
	return nil
}

// storeDeps records the results of the requests `# @ref` ran first, so
// their rows show what happened to them too.
func (m *Model) storeDeps(res *runner.Result) {
	for _, dep := range res.Deps {
		if req := dep.Req(); req != nil {
			m.results[req] = dep
			delete(m.errs, req)
		}
		m.storeDeps(dep)
	}
}

// finishRun refreshes derived state and the summary once a run completes.
func (m *Model) finishRun(rs *runState) {
	m.refreshDescs()
	m.refreshSession()
	m.histSeq++
	var results []*runner.Result
	for _, r := range rs.reqs {
		if res := m.results[r]; res != nil {
			results = append(results, res)
		} else if err := m.errs[r]; err != nil {
			results = append(results, &runner.Result{Request: runner.Resolved{Name: r.Name, File: r.File.Path, Line: r.Line, Method: r.Method, URL: r.URL}, Errors: []string{err.Error()}})
		}
	}
	if !rs.flow && len(results) == 1 {
		res := results[0]
		switch {
		case m.errs[rs.reqs[0]] != nil:
			m.lastSummary = m.theme.Fail.Render("✗ " + rs.reqs[0].ID())
		case res.Response != nil:
			m.lastSummary = fmt.Sprintf("%s %s %s %s", m.theme.Mark(res.OK), rs.reqs[0].ID(),
				m.theme.StatusCode(res.Response.Status), m.theme.Dim.Render(fmt.Sprintf("%d ms", res.Response.DurationMs)))
		}
		return
	}
	m.lastSummary = strings.TrimSpace(output.SummaryLine(m.theme, results))
}

// cycleEnv switches to the environment delta places along the list (1 for
// the next, -1 for the previous, wrapping) and rebuilds the runner.
func (m *Model) cycleEnv(delta int) Cmd {
	if m.inflight != nil {
		return m.setStatus("busy: wait for the run to finish")
	}
	if len(m.envs) < 2 {
		if len(m.envs) == 0 {
			return m.setStatus("no environments: create http-client.env.json")
		}
		return m.setStatus("only one environment: " + m.envs[0])
	}
	next := m.envs[0]
	for i, e := range m.envs {
		if e == m.env {
			next = m.envs[(i+delta+len(m.envs))%len(m.envs)]
			break
		}
	}
	return m.switchRunner(next, "env "+next)
}

// reload re-reads the project from disk.
func (m *Model) reload() Cmd {
	if m.inflight != nil {
		return m.setStatus("busy: wait for the run to finish")
	}
	return m.switchRunner(m.env, "project reloaded")
}

func (m *Model) switchRunner(env, status string) Cmd {
	if m.cfg.NewRunner == nil {
		return m.setStatus("cannot rebuild the runner here")
	}
	r, err := m.cfg.NewRunner(env)
	if err != nil {
		return m.setStatus(err.Error())
	}
	sel := ""
	if s := m.selectedReq(); s != nil {
		sel = s.ID()
	}
	m.runner = r
	m.env = env
	m.lastSummary = ""
	m.rebuild(sel)
	return m.setStatus(status)
}

func (m *Model) clearSession() Cmd {
	if m.runner.Session == nil {
		return nil
	}
	m.runner.Session.Clear(m.env)
	if err := m.runner.Session.Save(); err != nil {
		return m.setStatus("session: " + err.Error())
	}
	if m.runner.Jar != nil {
		m.runner.Jar.Clear(m.env)
		if err := m.runner.Jar.Save(); err != nil {
			return m.setStatus("cookies: " + err.Error())
		}
	}
	m.refreshSession()
	m.refreshDescs()
	return m.setStatus("session cleared for " + m.envLabel())
}
