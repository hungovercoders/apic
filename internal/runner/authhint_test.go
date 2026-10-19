package runner

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/auth"
	"github.com/hungovercoders/apic/internal/httpfile"
)

// The `# @auth` hint that validate, the language server's completion and
// hover show names every type the auth package accepts, so a new type
// cannot be left out of it again.
func TestAuthHintNamesEveryType(t *testing.T) {
	m := regexp.MustCompile("`# @auth ([a-z0-9|]+) ").FindStringSubmatch(httpfile.KnownDirectives["auth"])
	if m == nil {
		t.Fatalf("auth hint has no type list: %q", httpfile.KnownDirectives["auth"])
	}
	got := strings.Split(m[1], "|")
	want := make([]string, 0, len(auth.Types))
	for name := range auth.Types {
		want = append(want, name)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("auth hint types %v, auth.Types %v", got, want)
	}
}
