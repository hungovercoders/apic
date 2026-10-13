package ui

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

// editorCommand builds the command that opens file at line in editor.
// Editors that understand +line get it; VS Code and its relatives get
// -g file:line, and --wait unless the user already passes it, since they
// otherwise return at once and the reload that follows sees no edit.
func editorCommand(editor, file string, line int) (*exec.Cmd, error) {
	parts := strings.Fields(editor)
	if len(parts) == 0 {
		return nil, errors.New("set $EDITOR (or $VISUAL) to open files")
	}
	args := parts[1:]
	base := strings.TrimSuffix(filepath.Base(parts[0]), ".exe")
	switch base {
	case "code", "code-insiders", "codium", "cursor":
		if !slices.Contains(args, "--wait") && !slices.Contains(args, "-w") {
			args = append(args, "--wait")
		}
		args = append(args, "-g", fmt.Sprintf("%s:%d", file, line))
	case "notepad":
		args = append(args, file)
	default:
		args = append(args, fmt.Sprintf("+%d", line), file)
	}
	return exec.Command(parts[0], args...), nil //nolint:gosec // the editor is the user's own $EDITOR; nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
}

// openEditor suspends the UI, opens the selected request's file at its
// line, and reloads the project when the editor exits.
func (m *Model) openEditor() Cmd {
	sel := m.selectedReq()
	if sel == nil {
		return nil
	}
	if m.inflight != nil {
		return m.setStatus("busy: wait for the run to finish")
	}
	path := filepath.Join(m.runner.Project.Root, filepath.FromSlash(sel.File.Path))
	cmd, err := editorCommand(m.cfg.Editor, path, sel.Line)
	if err != nil {
		return m.setStatus(err.Error())
	}
	return func() Msg {
		return execMsg{cmd: cmd, done: func(err error) Msg { return editorDoneMsg{err: err} }}
	}
}
