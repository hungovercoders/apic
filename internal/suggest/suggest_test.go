package suggest

import (
	"reflect"
	"testing"
)

func TestClosest(t *testing.T) {
	ids := []string{"login", "whoami", "get-user", "get-users", "list-users", "delete-user", "health"}
	cases := []struct {
		query string
		want  []string
	}{
		{"get-usr", []string{"get-user", "get-users"}},            // one edit
		{"get", []string{"get-user", "get-users"}},                // prefix
		{"users", []string{"get-users", "list-users"}},            // contains
		{"get_user", []string{"get-user", "get-users"}},           // _ reads as -
		{"Login", []string{"login"}},                              // case
		{"lgoin", []string{"login"}},                              // transposition is two edits, allowed at length 5
		{"whomai", []string{"whoami"}},                            //
		{"delete-usre", []string{"delete-user"}},                  //
		{"xyz", nil},                                              // nothing close
		{"", nil},                                                 //
		{"a", nil},                                                // one letter would match everything within one edit
		{"é", nil},                                                // one letter in two bytes, the same
		{"héalth", []string{"health"}},                            // one edit, counted in runes
		{"user", []string{"get-user", "get-users", "list-users"}}, // capped at Max, shortest first
	}
	for _, c := range cases {
		if got := Closest(c.query, ids); !reflect.DeepEqual(got, c.want) {
			t.Errorf("Closest(%q) = %v, want %v", c.query, got, c.want)
		}
	}
}

func TestClosestKeepsTheExactSpellingAndDedupes(t *testing.T) {
	got := Closest("stagin", []string{"staging", "Staging", "staging", "prod"})
	if want := []string{"Staging", "staging"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestPhrase(t *testing.T) {
	cases := map[string][]string{
		"":                        nil,
		"did you mean a?":         {"a"},
		"did you mean a or b?":    {"a", "b"},
		"did you mean a, b or c?": {"a", "b", "c"},
	}
	for want, names := range cases {
		if got := Phrase(names); got != want {
			t.Errorf("Phrase(%v) = %q, want %q", names, got, want)
		}
	}
}

func TestDistance(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0}, {"", "abc", 3}, {"abc", "", 3}, {"abc", "abc", 0},
		{"kitten", "sitting", 3}, {"flaw", "lawn", 2}, {"héllo", "hello", 1},
	}
	for _, c := range cases {
		if got := distance(c.a, c.b); got != c.want {
			t.Errorf("distance(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
