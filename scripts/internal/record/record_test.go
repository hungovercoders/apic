package record

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hungovercoders/apic/internal/lessons"
)

// A step's code is its snippet, or its blocks in order with the
// substitution applied across them; a block the lesson lacks or a
// substitution that matches nothing is an error.
func TestStepCode(t *testing.T) {
	blocks := []lessons.Block{{Code: "one\n"}, {Code: "two\nrm it\n"}, {Code: "three\n"}}
	if got, _ := (Step{Sh: "echo hi"}).Code(nil); got != "echo hi\n" {
		t.Errorf("snippet = %q", got)
	}
	got, err := (Step{Lesson: "01", Blocks: []int{1, 2}, Replace: [2]string{"rm it\n", ""}}).Code(blocks)
	if err != nil || got != "one\ntwo\n" {
		t.Errorf("blocks with substitution = %q, %v", got, err)
	}
	if _, err := (Step{Lesson: "01", Blocks: []int{4}}).Code(blocks); err == nil || !strings.Contains(err.Error(), "no block 4") {
		t.Errorf("missing block: %v", err)
	}
	if _, err := (Step{Lesson: "01", Blocks: []int{1}, Replace: [2]string{"rm it\n", ""}}).Code(blocks); err == nil || !strings.Contains(err.Error(), "nothing to replace") {
		t.Errorf("substitution matching nothing: %v", err)
	}
}

// The recording environment carries none of the machine's apic settings,
// proxies or colour switches, and sets its own PATH, HOME and demo URL.
func TestEnvironment(t *testing.T) {
	for _, kv := range []string{"APIC_VAR_user=me", "apic_var_baseUrl=x", "APIC_USER=me", "HTTPS_PROXY=http://p", "no_proxy=*", "NO_COLOR=1", "KEEP_ME=1"} {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	keys := map[string]string{}
	for _, kv := range Environment("/bin-dir", "/work-dir") {
		k, v, _ := strings.Cut(kv, "=")
		keys[k] = v
	}
	for _, gone := range []string{"APIC_VAR_user", "apic_var_baseUrl", "APIC_USER", "HTTPS_PROXY", "no_proxy", "NO_COLOR"} {
		if _, ok := keys[gone]; ok {
			t.Errorf("environment still has %s", gone)
		}
	}
	for k, want := range map[string]string{"KEEP_ME": "1", "HOME": filepath.Join("/work-dir", "home"), "TZ": "UTC", "APIC_DEMO_URL": "http://localhost:8089"} {
		if keys[k] != want {
			t.Errorf("environment has %s=%q, want %q", k, keys[k], want)
		}
	}
	if !strings.HasPrefix(keys["PATH"], "/bin-dir") {
		t.Errorf("PATH=%q does not start with the binary's directory", keys["PATH"])
	}
}

// Pick keeps the named items in the order asked for and refuses a name
// that matches nothing.
func TestPick(t *testing.T) {
	items := []string{"01", "02", "demo"}
	id := func(s string) string { return s }
	if got, err := Pick(items, id, ""); err != nil || len(got) != 3 {
		t.Errorf("Pick(all) = %v, %v", got, err)
	}
	if got, err := Pick(items, id, "demo, 01"); err != nil || strings.Join(got, ",") != "demo,01" {
		t.Errorf("Pick(demo, 01) = %v, %v", got, err)
	}
	if _, err := Pick(items, id, "03"); err == nil {
		t.Error("Pick(03) found something")
	}
}
