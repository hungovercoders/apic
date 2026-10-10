package runner

import "testing"

// The cut of --body-limit lands on a character boundary, and only the
// last character is looked at: an invalid byte earlier is the body's own.
func TestTextPrefix(t *testing.T) {
	euro := "\xe2\x82\xac" // a three-byte character
	for in, want := range map[string]string{
		"abc":                  "abc",
		"ab" + euro:            "ab" + euro,
		"ab" + euro[:2]:        "ab",
		"ab" + euro[:1]:        "ab",
		"\xff" + "abcdefgh":    "\xff" + "abcdefgh",
		"\xffab" + euro[:1]:    "\xffab",
		"":                     "",
		euro[:2]:               "",
		"a\xff\xfe\xfd" + euro: "a\xff\xfe\xfd" + euro,
	} {
		if got := string(textPrefix([]byte(in))); got != want {
			t.Errorf("%q: got %q, want %q", in, got, want)
		}
	}
}
