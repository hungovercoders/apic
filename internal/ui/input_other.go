//go:build !unix

package ui

import "os"

// newKeySource reads the terminal from a goroutine: there is no poll to
// interrupt a console read with, so a key typed into the editor just as it
// opens can be lost, but Resume keeps it from being acted on afterwards.
func newKeySource(f *os.File) (keySource, error) { return newBgSource(f), nil }
