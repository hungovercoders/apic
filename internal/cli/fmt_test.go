package cli

import (
	"regexp"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/httpfile"
)

// The order `apic fmt --help` lists is the order the formatter writes, so
// a directive added to the formatter cannot go missing from the help.
func TestFmtHelpListsTheFormatterOrder(t *testing.T) {
	_, out, _ := execute(t, "fmt", "--help")
	m := regexp.MustCompile(`(?s)directives in a fixed order \((.*?), then the rest as written\)`).FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("fmt help has no directive order:\n%s", out)
	}
	got := strings.Join(strings.Fields(m[1]), " ")
	want := strings.Join(httpfile.DirectiveOrder(), ", ")
	if got != want {
		t.Errorf("fmt help order:\n got %s\nwant %s", got, want)
	}
}
