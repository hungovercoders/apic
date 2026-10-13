//go:build windows

package ui

import "github.com/muesli/termenv"

// enableVT turns on virtual terminal processing for the console, which the
// legacy console (cmd.exe, and PowerShell outside Windows Terminal) leaves
// off. lipgloss turns it on too, but only inside the first style it
// renders, and Run writes the alternate-screen and cursor sequences before
// the first frame: without this they would print as text. It returns the
// func that puts the mode back.
func enableVT() func() {
	mode, err := termenv.EnableWindowsANSIConsole()
	if err != nil {
		return func() {}
	}
	return func() { _ = termenv.RestoreWindowsConsole(mode) }
}
