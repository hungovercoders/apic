package main

import (
	"regexp"
	"strings"
)

// listItem matches the start of a list item, nested or not: "- ", "* ",
// "+ " or "1. ".
var listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s`)

// Unwrap joins the hard-wrapped lines of each paragraph and list item into
// one, as a release's notes on GitHub need: GitHub renders a release body's
// single newlines as line breaks, so CHANGELOG.md's 72-column wrapping
// would break every bullet mid-sentence there. Blank lines, headings, list
// items, table rows, link references and fenced code keep their lines.
func Unwrap(text string) string {
	var out []string
	joinable := false // whether the last line out may take a continuation
	for _, l := range scan(text) {
		trimmed := strings.TrimSpace(l.text)
		switch {
		case l.fenced:
			out = append(out, l.text)
			joinable = false
		case trimmed == "":
			out = append(out, "")
			joinable = false
		case strings.HasPrefix(trimmed, "#"), strings.HasPrefix(trimmed, "|"), linkRef.MatchString(trimmed):
			out = append(out, l.text)
			joinable = false
		case listItem.MatchString(l.text) || !joinable:
			out = append(out, strings.TrimRight(l.text, " "))
			joinable = true
		default:
			out[len(out)-1] += " " + trimmed
		}
	}
	return strings.Join(out, "\n")
}
