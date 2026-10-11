// Package lessons reads the runnable blocks out of the course pages: the
// fenced code blocks under website/src/content/docs/learn (and the
// migration guides) that an `<!-- learn -->` comment marks as something a
// reader types. scripts/learncheck runs them so a lesson cannot rot, and
// scripts/tapes runs the ones a tape expects to have happened before it
// starts recording.
package lessons

import (
	"regexp"
	"strings"
)

// Block is one runnable fence: where it came from and what it holds.
type Block struct {
	File string
	Line int // line of the opening fence, 1-based
	Code string
}

var (
	reMarker = regexp.MustCompile(`^\s*<!--\s*learn\s*-->\s*$`)
	reFence  = regexp.MustCompile("^(\\s*)(```+|~~~+)\\s*(\\S*)")
)

// Extract returns the runnable blocks of one Markdown page in order.
func Extract(file string, content string) []Block {
	var out []Block
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	armed := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if reMarker.MatchString(line) {
			armed = true
			continue
		}
		if strings.TrimSpace(line) == "" {
			continue // a blank line between the marker and the fence is fine
		}
		m := reFence.FindStringSubmatch(line)
		if m == nil {
			armed = false // something else came before a fence
			continue
		}
		indent, fence := m[1], m[2]
		start := i
		var body []string
		for i++; i < len(lines); i++ {
			if closesFence(lines[i], fence) {
				break
			}
			body = append(body, strings.TrimPrefix(lines[i], indent))
		}
		if armed {
			out = append(out, Block{File: file, Line: start + 1, Code: strings.Join(body, "\n") + "\n"})
		}
		armed = false
	}
	return out
}

// closesFence reports whether line closes a block opened with fence: the
// same character, at least as many of them (CommonMark allows more), and
// nothing else on the line.
func closesFence(line, fence string) bool {
	t := strings.TrimSpace(line)
	if !strings.HasPrefix(t, fence) {
		return false
	}
	return strings.Trim(t, fence[:1]) == ""
}
