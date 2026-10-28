package main

import (
	"regexp"
	"strings"
)

// linkRef matches a Markdown link reference definition, "[0.2.0]: url".
var linkRef = regexp.MustCompile(`^\[([^\]]+)\]: `)

// heading matches a release heading: "## [0.2.0] - 2026-10-27", the
// "## [Unreleased]" section, or a range such as "## [0.1.1] to [0.1.3] -
// 2026-10-10" for releases that changed nothing in the binary.
var heading = regexp.MustCompile(`^## \[([^\]]+)\](?: to \[([^\]]+)\])?(?: - (\d{4}-\d{2}-\d{2}))?\s*$`)

// A line is one line of the changelog as every reader of it sees it, so
// Section, Check, the site page and the release checks cannot disagree on
// where a section starts or the file's link references begin.
type line struct {
	n    int    // 1-based
	text string // as written
	// h2 is a "## " line outside a fenced block before the link
	// references, which ends the section above it; heading holds its parts
	// when it is a well-formed release heading, nil when it is not.
	h2      bool
	heading []string
	// trailing is a line of the block of link references (and the blank
	// lines among them) that ends the file; ref holds a reference's parts.
	// A reference written inside a section's body is part of that body.
	trailing bool
	ref      []string
}

// scan splits a changelog into lines. A fenced code block (``` or ~~~) is
// body text, whatever its lines look like: a release note can show a
// Markdown or .http example with a "## " line in it.
func scan(changelog string) []line {
	texts := strings.Split(changelog, "\n")
	lines := make([]line, len(texts))
	fence := ""
	for i, text := range texts {
		lines[i] = line{n: i + 1, text: text}
		trimmed := strings.TrimLeft(text, " ")
		switch {
		case fence != "":
			if strings.HasPrefix(trimmed, fence) {
				fence = ""
			}
			continue
		case strings.HasPrefix(trimmed, "```"):
			fence = "```"
			continue
		case strings.HasPrefix(trimmed, "~~~"):
			fence = "~~~"
			continue
		}
		if strings.HasPrefix(text, "## ") {
			lines[i].h2 = true
			lines[i].heading = heading.FindStringSubmatch(text)
		}
	}
	// The trailing block: back from the end over blank lines and link
	// references, stopping at the first line that is neither. A fence left
	// open runs to the end of the file, so it has no trailing block.
	if fence != "" {
		return lines
	}
	for i := len(lines) - 1; i >= 0; i-- {
		text := lines[i].text
		ref := linkRef.FindStringSubmatch(text)
		if strings.TrimSpace(text) != "" && ref == nil {
			break
		}
		lines[i].trailing = true
		lines[i].ref = ref
		lines[i].h2, lines[i].heading = false, nil
	}
	return lines
}

// version is a heading's version, the upper end of a range.
func version(h []string) string {
	if h[2] != "" {
		return h[2]
	}
	return h[1]
}
