// Command changelog makes CHANGELOG.md the one place a release is written
// down, and everything else that names a release follow from it:
//
//	go run ./scripts/changelog 0.2.0           # print a version's section (Unreleased with none)
//	go run ./scripts/changelog -write          # the site's changelog page, and the docs' version pins
//	go run ./scripts/changelog -release v0.2.0 # release.yml: the notes, after checking the tag
//	go run ./scripts/changelog -latest         # release.yml: the version to release from main
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
	latest := flag.Bool("latest", false, "print the newest dated section's version, which a push to main releases")
	flag.Parse()
	data, err := os.ReadFile(*file) //nolint:gosec // the changelog the caller named
	if err != nil {
		fail(err)
	}
	changelog := string(data)
	root := filepath.Dir(*file)
	switch {
	case *latest:
		v, err := Latest(changelog)
		if err != nil {
			fail(err)
		}
		fmt.Println(v)
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
		version := unreleased
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

// unreleased is the name of the section a change goes under until its
// release is cut.
const unreleased = "Unreleased"

// semver matches a release's version: three numbers, and a prerelease
// suffix for a release candidate (0.3.0-rc.1).
var semver = regexp.MustCompile(`^` + ver + `$`)

// Latest is the newest release: the version of the newest dated section,
// the upper end of a range heading. It is what release.yml checks the tag
// against.
func Latest(changelog string) (string, error) {
	return newest(changelog, func(string) bool { return true })
}

// Stable is the newest release that is not a prerelease, which is what the
// docs pin: a release candidate is for those who ask for it by name.
func Stable(changelog string) (string, error) {
	return newest(changelog, func(v string) bool { return !strings.Contains(v, "-") })
}

func newest(changelog string, want func(string) bool) (string, error) {
	for _, line := range strings.Split(changelog, "\n") {
		m := heading.FindStringSubmatch(line)
		if m == nil || m[3] == "" {
			continue
		}
		v := m[1]
		if m[2] != "" {
			v = m[2]
		}
		if !semver.MatchString(v) {
			return "", fmt.Errorf("CHANGELOG.md: %q is not a version like 0.2.0 or 0.3.0-rc.1", v)
		}
		if want(v) {
			return v, nil
		}
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
	stale, err := Stale(root, changelog)
	if err != nil {
		return "", err
	}
	var problems []string
	for _, f := range stale {
		if f == Page {
			problems = append(problems, f+" does not match CHANGELOG.md's releases")
		} else {
			problems = append(problems, f+" pins another version than the newest stable release")
		}
	}
	if len(problems) > 0 {
		return "", fmt.Errorf("%s: run task docs:changelog and merge it before tagging", strings.Join(problems, "; "))
	}
	return Section(changelog, tag)
}

// linkRef matches a Markdown link reference definition, "[0.2.0]: url",
// which the file's compare links at the end are.
var linkRef = regexp.MustCompile(`^\[([^\]]+)\]: `)

// heading matches a release heading: "## [0.2.0] - 2026-10-27", the
// "## [Unreleased]" section, or a range such as "## [0.1.1] to [0.1.3] -
// 2026-10-10" for releases that changed nothing in the binary.
var heading = regexp.MustCompile(`^## \[([^\]]+)\](?: to \[([^\]]+)\])?(?: - (\d{4}-\d{2}-\d{2}))?\s*$`)

// Section returns the body under version's heading, the lines up to the
// next "## " heading or the link references that end the file, with the
// blank lines around them removed. A range
// heading serves every version it names and the ones between. The body
// must not be empty: a release with nothing to say has nothing to attach.
func Section(changelog, version string) (string, error) {
	var body []string
	found := false
	for _, line := range strings.Split(changelog, "\n") {
		if found && linkRef.MatchString(line) {
			break
		}
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

// compareVersions orders versions: numerically by their dotted parts, so
// "0.10.0" is above "0.9.1"; a prerelease below the release it leads to,
// so "0.3.0-rc.1" is below "0.3.0" and above "0.2.0"; and a word such as
// Unreleased above everything.
func compareVersions(a, b string) int {
	a, preA, _ := strings.Cut(a, "-")
	b, preB, _ := strings.Cut(b, "-")
	if c := compareParts(a, b); c != 0 || preA == preB {
		return c
	}
	switch {
	case preA == "":
		return 1
	case preB == "":
		return -1
	}
	return comparePrerelease(preA, preB)
}

// comparePrerelease orders two prerelease suffixes as semver does: dot by
// dot, numbers numerically, words by their bytes, a number below a word,
// and a suffix that runs out first below a longer one with the same start.
// So alpha.1 < alpha.2 < beta.1 < rc.1.
func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		x, y := as[i], bs[i]
		xn, yn := isNumber(x), isNumber(y)
		switch {
		case xn && yn:
			if c := compareParts(x, y); c != 0 {
				return c
			}
		case xn:
			return -1
		case yn:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	return compareParts(fmt.Sprint(len(as)), fmt.Sprint(len(bs)))
}

func isNumber(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func compareParts(a, b string) int {
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
		if m := linkRef.FindStringSubmatch(line); m != nil {
			refs[m[1]] = true
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
		for _, v := range []string{from, to} {
			if v != "" && v != unreleased && !semver.MatchString(v) {
				problems = append(problems, fmt.Sprintf("line %d: %q is not a version like 0.2.0 or 0.3.0-rc.1", n+1, v))
			}
		}
		if first {
			first = false
			if from != unreleased {
				problems = append(problems, fmt.Sprintf("line %d: the first section must be Unreleased", n+1))
			}
		}
		for _, v := range []string{from, to} {
			if v == "" {
				continue
			}
			if v != unreleased && date == "" {
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
