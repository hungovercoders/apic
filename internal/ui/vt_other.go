//go:build !windows

package ui

// enableVT has nothing to do outside Windows: every other terminal
// processes escape sequences as a matter of course.
func enableVT() func() { return func() {} }
