// Command changelog makes CHANGELOG.md the one place a release is written
// down, and everything else that names a release follow from it:
//
//	go run ./scripts/changelog 0.2.0           # print a version's section (Unreleased with none)
//	go run ./scripts/changelog -write          # the site's changelog page, and the docs' version pins
//	go run ./scripts/changelog -release v0.2.0 # release.yml: the notes, after checking the tag
//
// The newest dated section is the current release. -write renders the file
// as the site's changelog page and rewrites every place the docs pin a
// release (the install commands, setup-apic's version input, the course's
// sample output) to that version; `task docs:changelog` runs it, and a test
// fails when either is stale, so dating a section in a pull request is the
// one manual step of a release. -release is what release.yml runs before
// building anything: it fails unless the tag is the newest dated section
// and the docs already name it, then prints the section as the release's
// notes. Without a flag the command prints a version's section, exiting 1
// when there is none or it is empty.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	file := flag.String("file", "CHANGELOG.md", "the changelog to read")
	write := flag.Bool("write", false, "write the site's changelog page and the docs' version pins")
	release := flag.String("release", "", "check that this tag is the newest dated section and the docs name it, then print its notes")
	flag.Parse()
	data, err := os.ReadFile(*file) //nolint:gosec // the changelog the caller named
	if err != nil {
		fail(err)
	}
	changelog := string(data)
	root := filepath.Dir(*file)
	switch {
	case *write:
		if err := Write(root, changelog); err != nil {
			fail(err)
		}
	case *release != "":
		body, err := Release(root, changelog, strings.TrimPrefix(*release, "v"))
		if err != nil {
			fail(err)
		}
		fmt.Println(body)
	default:
		version := "Unreleased"
		if flag.NArg() == 1 {
			version = strings.TrimPrefix(flag.Arg(0), "v")
		} else if flag.NArg() > 1 {
			fmt.Fprintln(os.Stderr, "usage: changelog [-file CHANGELOG.md] [-write | -release <tag> | <version>]")
			os.Exit(2)
		}
		body, err := Section(changelog, version)
		if err != nil {
			fail(err)
		}
		fmt.Println(body)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

// Latest is the current release: the version of the newest dated section,
// the upper end of a range heading.
func Latest(changelog string) (string, error) {
	for _, line := range strings.Split(changelog, "\n") {
		m := heading.FindStringSubmatch(line)
		if m == nil || m[3] == "" {
			continue
		}
		if m[2] != "" {
			return m[2], nil
		}
		return m[1], nil
	}
	return "", fmt.Errorf("CHANGELOG.md has no dated release section")
}

// Release returns the notes for tag after checking that it is the release
// the changelog and the docs describe: the newest dated section, whose
// version the site's page and pins already carry. A tag cut before the
// release pull request merged, or one that skipped `task docs:changelog`,
// fails here with nothing built.
func Release(root, changelog, tag string) (string, error) {
	latest, err := Latest(changelog)
	if err != nil {
		return "", err
	}
	if tag != latest {
		return "", fmt.Errorf("the tag is %s but CHANGELOG.md's newest dated section is %s: date this release's section (## [%s] - YYYY-MM-DD) and run task docs:changelog before tagging", tag, latest, tag)
	}
	if stale, err := Stale(root, changelog); err != nil {
		return "", err
	} else if len(stale) > 0 {
		return "", fmt.Errorf("%s do not name %s yet: run task docs:changelog before tagging", strings.Join(stale, ", "), tag)
	}
	return Section(changelog, tag)
}

// heading matches a release heading: "## [0.2.0] - 2026-10-27", the
// "## [Unreleased]" section, or a range such as "## [0.1.1] to [0.1.3] -
// 2026-10-10" for releases that changed nothing in the binary.
var heading = regexp.MustCompile(`^## \[([^\]]+)\](?: to \[([^\]]+)\])?(?: - (\d{4}-\d{2}-\d{2}))?\s*$`)

// Section returns the body under version's heading, the lines up to the
// next "## " heading with the blank lines around them removed. A range
// heading serves every version it names and the ones between. The body
// must not be empty: a release with nothing to say has nothing to attach.
func Section(changelog, version string) (string, error) {
	var body []string
	found := false
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "## ") {
			if found {
				break
			}
			m := heading.FindStringSubmatch(line)
			if m != nil && covers(m[1], m[2], version) {
				found = true
			}
			continue
		}
		if found {
			body = append(body, line)
		}
	}
	if !found {
		return "", fmt.Errorf("CHANGELOG.md has no section for %s: add one under its own heading before tagging", version)
	}
	text := strings.TrimSpace(strings.Join(body, "\n"))
	if text == "" {
		return "", fmt.Errorf("CHANGELOG.md's section for %s is empty: write what changed before tagging", version)
	}
	return text, nil
}

// covers reports whether a heading for from (and, for a range, up to
// to) names version: the same string, or one between the two bounds in
// version order.
func covers(from, to, version string) bool {
	if from == version || to == version {
		return true
	}
	if to == "" {
		return false
	}
	return compareVersions(from, version) <= 0 && compareVersions(version, to) <= 0
}

// compareVersions orders dotted numeric versions, a non-numeric part
// sorting after any number, so "0.10.0" is above "0.9.1" and "Unreleased"
// above everything.
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y int
		if i < len(as) {
			x = numeric(as[i])
		}
		if i < len(bs) {
			y = numeric(bs[i])
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// numeric is a version part as a number, with anything that is not one
// (a word such as Unreleased) ranking above every number.
func numeric(part string) int {
	n := 0
	for _, r := range part {
		if r < '0' || r > '9' {
			return 1 << 30
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// Check reports what is wrong with a changelog's shape: the first section
// must be Unreleased, every other heading must carry a date and name a
// version below the one before it, and each version named must have a
// link reference at the end of the file.
func Check(changelog string) []string {
	var problems []string
	refs := map[string]bool{}
	for _, line := range strings.Split(changelog, "\n") {
		if strings.HasPrefix(line, "[") {
			if name, _, ok := strings.Cut(line[1:], "]: "); ok {
				refs[name] = true
			}
		}
	}
	prev := ""
	first := true
	for n, line := range strings.Split(changelog, "\n") {
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		m := heading.FindStringSubmatch(line)
		if m == nil {
			problems = append(problems, fmt.Sprintf("line %d: %q is not a release heading (## [version] - YYYY-MM-DD)", n+1, line))
			continue
		}
		from, to, date := m[1], m[2], m[3]
		if first {
			first = false
			if from != "Unreleased" {
				problems = append(problems, fmt.Sprintf("line %d: the first section must be Unreleased", n+1))
			}
		}
		for _, v := range []string{from, to} {
			if v == "" {
				continue
			}
			if v != "Unreleased" && date == "" {
				problems = append(problems, fmt.Sprintf("line %d: %s has no release date", n+1, v))
			}
			if !refs[v] {
				problems = append(problems, fmt.Sprintf("line %d: no link reference for [%s] at the end of the file", n+1, v))
			}
		}
		top := from
		if to != "" {
			top = to
		}
		if prev != "" && compareVersions(top, prev) >= 0 {
			problems = append(problems, fmt.Sprintf("line %d: %s is not below %s; newest first", n+1, top, prev))
		}
		prev = from
	}
	if first {
		problems = append(problems, "no release headings")
	}
	return problems
}
