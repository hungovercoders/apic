package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTemplateRunsEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds the binary and serves the demo")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{Learn, Migrate}, false); err != nil {
		t.Fatal(err)
	}
}
