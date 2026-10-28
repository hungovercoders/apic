package main

import (
	"regexp"
	"strings"
)

// Block starts, by CommonMark's rules for what may interrupt a paragraph:
// an ATX heading (# and a space, at most three spaces in), a bullet, an
// ordered item, and a blockquote.
var (
	atxHeading = regexp.MustCompile(`^ {0,3}#{1,6}(?:\s|$)`)
	bulletItem = regexp.MustCompile(`^(\s*)([-*+])(\s+)`)
	orderItem  = regexp.MustCompile(`^(\s*)(\d{1,9})([.)])(\s+)`)
	quoteStart = regexp.MustCompile(`^ {0,3}>`)
	// tableRule is a table's delimiter row, "|---|:--:|", which makes the
	// line above it a header.
	tableRule = regexp.MustCompile(`^\s*\|?\s*:?-+:?\s*(?:\|\s*:?-+:?\s*)*\|?\s*$`)
)

// listItem matches the start of a list item, nested or not.
var listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d{1,9}[.)])\s`)

// Unwrap joins the hard-wrapped lines of each paragraph, list item and
// blockquote into one, as a release's notes on GitHub need: GitHub renders
// a release body's single newlines as line breaks, so CHANGELOG.md's
// 72-column wrapping would break every bullet mid-sentence there.
//
// A line continues the paragraph above it unless it could start a block of
// its own there: a heading, a bullet, an ordered item numbered 1 (or any
// number, outdented to a sibling's place), or a blockquote. So "#157" or
// "2." wrapped onto the next line of a bullet is joined, as Markdown reads
// it. A blockquote's lines join without their markers. Blank lines, table
// rows (a header with its delimiter row, and the rows after), link
// references, fenced code and indented code keep their lines.
func Unwrap(text string) string {
	var out []string
	joinable := false // the last line out is prose that may take a continuation
	quote := false    // and it is a blockquote's
	content := 0      // the column the current list item's text starts at
	prevBlank := true
	table := false // the last line out is a table row
	lines := scan(text)
	for i, l := range lines {
		raw := strings.TrimRight(l.text, " ")
		trimmed := strings.TrimSpace(raw)
		indent := len(raw) - len(strings.TrimLeft(raw, " "))
		blank := trimmed == ""
		switch {
		case l.fenced:
			out = append(out, l.text)
			joinable = false
		case blank:
			out = append(out, "")
			joinable = false
		case !joinable && prevBlank && indent >= content+4:
			out = append(out, l.text) // indented code
		case quoteStart.MatchString(raw):
			body := strings.TrimSpace(strings.TrimPrefix(strings.TrimLeft(raw, " "), ">"))
			if joinable && quote && body != "" && !startsBlock(body, 0, 0) {
				out[len(out)-1] += " " + body
			} else {
				out = append(out, raw)
				joinable, quote = body != "", true
			}
		case strings.HasPrefix(trimmed, "|") && (table || (i+1 < len(lines) && tableRule.MatchString(lines[i+1].text))):
			out = append(out, raw)
			joinable, table = false, true
			prevBlank = false
			continue
		case linkRef.MatchString(trimmed):
			out = append(out, raw)
			joinable = false
		case joinable && !startsBlock(raw, indent, content):
			out[len(out)-1] += " " + trimmed
		default:
			out = append(out, raw)
			joinable, quote = !atxHeading.MatchString(raw), false
			if m := bulletItem.FindStringSubmatch(raw); m != nil {
				content = len(m[0])
			} else if m := orderItem.FindStringSubmatch(raw); m != nil {
				content = len(m[0])
			} else if indent < content {
				content = indent // a paragraph outside the list
			}
		}
		prevBlank = blank
		table = false
	}
	return strings.Join(out, "\n")
}

// startsBlock reports whether line, read while a paragraph is open, starts
// a block instead of continuing it. content is the column the open list
// item's text starts at: a line indented less than that and shaped like an
// item is a sibling item whatever its number.
func startsBlock(line string, indent, content int) bool {
	switch {
	case atxHeading.MatchString(line), bulletItem.MatchString(line), quoteStart.MatchString(line):
		return true
	}
	if m := orderItem.FindStringSubmatch(line); m != nil {
		return m[2] == "1" || indent < content
	}
	return false
}
