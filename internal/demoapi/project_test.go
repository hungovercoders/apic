package demoapi

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/httpfile"
)

// The demo project is what a lesson, a screenshot and an agent's first
// session see, and `apic fmt` is what they run after an edit, so it must
// already be in canonical form or the formatter touches what they did not.
func TestProjectIsFormatted(t *testing.T) {
	err := fs.WalkDir(projectFS, "project", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || path.Ext(p) != ".http" {
			return err
		}
		data, err := fs.ReadFile(projectFS, p)
		if err != nil {
			return err
		}
		if src := strings.ReplaceAll(string(data), "\r\n", "\n"); httpfile.Format(src) != src {
			t.Errorf("%s is not formatted; run `apic fmt -C internal/demoapi/project`", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
