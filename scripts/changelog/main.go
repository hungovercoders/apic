// Command changelog makes CHANGELOG.md the one place a release is written
// down, and everything else that names a release follow from it:
//
//	go run ./scripts/changelog 0.2.0           # print a version's section (Unreleased with none)
//	go run ./scripts/changelog -write          # the site's changelog page, and the docs' version pins
//	go run ./scripts/changelog -release v0.2.0 # release.yml: the notes, after checking the tag
//	go run ./scripts/changelog -latest         # release.yml: the version to release from main
//	go run ./scripts/changelog -release-commit v0.2.0 # release.yml: the commit on main to tag
//
// The newest dated section is the release to cut, and the newest one that
// is not a prerelease is the one the docs name. -write renders the file as
// the site's changelog page and rewrites every place the docs pin a release
// (the install commands, setup-apic's version input, the image tags, the
// course's sample output) to that stable version; `task docs:changelog`
// runs it, and a test fails when either is stale, so dating a section in a
// pull request is the one manual step of a release. -release is what
// release.yml runs before building anything: it fails unless the tag is the
// newest dated section, no higher version is tagged and the docs are
// current (naming the newest stable release, which is the tag unless it is
// a prerelease), then prints the section as the release's notes, unwrapped
// as GitHub shows them. Without a flag the command prints a version's
// section the same way, exiting 1 when there is none or it is empty.
package main

import (
	"cmp"
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
	release := flag.String("release", "", "check that this tag is the newest dated section, that no higher version is tagged and that the docs are current, then print its notes as GitHub shows them")
	latest := flag.Bool("latest", false, "print the newest dated section's version, which a merge to main releases")
	commit := flag.String("release-commit", "", "print the commit on HEAD's first-parent history to tag for this tag: the first since it was dated that passes the release check")
	flag.Parse()
	modes := 0
	for _, set := range []bool{*write, *release != "", *latest, *commit != "", flag.NArg() > 0} {
		if set {
			modes++
		}
	}
	if modes > 1 || flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: changelog [-file CHANGELOG.md] [-write | -latest | -release <tag> | -release-commit <tag> | <version>]")
		os.Exit(2)
	}
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
		tag := strings.TrimPrefix(*release, "v")
		body, err := Release(fromDisk(root), changelog, tag)
		if err != nil {
			fail(err)
		}
		if err := noNewerTags(root, tag); err != nil {
			fail(err)
		}
		fmt.Println(body)
	case *commit != "":
		tag := strings.TrimPrefix(*commit, "v")
		if err := noNewerTags(root, tag); err != nil {
			fail(err)
		}
		c, err := ReleaseCommit(root, filepath.Base(*file), "HEAD", tag)
		if err != nil {
			fail(err)
		}
		fmt.Println(c)
	default:
		version := unreleased
		if flag.NArg() == 1 {
			version = strings.TrimPrefix(flag.Arg(0), "v")
		}
		body, err := Section(changelog, version)
		if err != nil {
			fail(err)
		}
		fmt.Println(Unwrap(body))
	}
}

// noNewerTags refuses a release of tag while a higher version is tagged.
func noNewerTags(root, tag string) error {
	newer, err := NewerTags(root, tag)
	if err != nil {
		return err
	}
	if len(newer) > 0 {
		return fmt.Errorf("%s is tagged above v%s: releasing it again would move latest, the cask and v0 back to it", strings.Join(newer, ", "), tag)
	}
	return nil
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
// against. The changelog must pass Check first: a section dated below an
// older one would otherwise hide it, and the release would be skipped
// with a green run.
func Latest(changelog string) (string, error) {
	return newest(changelog, func(string) bool { return true })
}

// Stable is the newest release that is not a prerelease, which is what the
// docs pin: a release candidate is for those who ask for it by name.
func Stable(changelog string) (string, error) {
	return newest(changelog, func(v string) bool { return !strings.Contains(v, "-") })
}

func newest(changelog string, want func(string) bool) (string, error) {
	if problems := Check(changelog); len(problems) > 0 {
		return "", fmt.Errorf("CHANGELOG.md:\n  %s", strings.Join(problems, "\n  "))
	}
	for _, l := range scan(changelog) {
		if l.heading == nil || l.heading[3] == "" || l.heading[1] == unreleased {
			continue
		}
		if v := version(l.heading); want(v) {
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
func Release(read reader, changelog, tag string) (string, error) {
	latest, err := Latest(changelog)
	if err != nil {
		return "", err
	}
	if tag != latest {
		return "", fmt.Errorf("the tag is %s but CHANGELOG.md's newest dated section is %s: date this release's section (## [%s] - YYYY-MM-DD) and run task docs:changelog before tagging", tag, latest, tag)
	}
	stale, err := Stale(read, changelog)
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
	body, err := Section(changelog, tag)
	if err != nil {
		return "", err
	}
	return Unwrap(body), nil
}

// Section returns the body under version's heading: the lines up to the
// next section or the link references that end the file, with the blank
// lines around them removed. A range heading serves every version it names
// and the ones between. The body must not be empty: a release with nothing
// to say has nothing to attach.
func Section(changelog, version string) (string, error) {
	var body []string
	found := false
	for _, l := range scan(changelog) {
		if found {
			if l.h2 || l.trailing {
				break
			}
			body = append(body, l.text)
			continue
		}
		if l.heading != nil && covers(l.heading[1], l.heading[2], version) {
			found = true
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
	return cmp.Compare(len(as), len(bs))
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
	lines := scan(changelog)
	refs := map[string]bool{}
	for _, l := range lines {
		if l.ref != nil {
			refs[l.ref[1]] = true
		}
	}
	prev := ""
	first := true
	for _, l := range lines {
		if !l.h2 {
			continue
		}
		m := l.heading
		if m == nil {
			problems = append(problems, fmt.Sprintf("line %d: %q is not a release heading (## [version] - YYYY-MM-DD)", l.n, l.text))
			continue
		}
		from, to, date := m[1], m[2], m[3]
		for _, v := range []string{from, to} {
			if v != "" && v != unreleased && !semver.MatchString(v) {
				problems = append(problems, fmt.Sprintf("line %d: %q is not a version like 0.2.0 or 0.3.0-rc.1", l.n, v))
			}
		}
		if first {
			first = false
			if from != unreleased {
				problems = append(problems, fmt.Sprintf("line %d: the first section must be Unreleased", l.n))
			}
		} else if from == unreleased || to == unreleased {
			problems = append(problems, fmt.Sprintf("line %d: only the first section is Unreleased", l.n))
		}
		if from == unreleased && date != "" {
			problems = append(problems, fmt.Sprintf("line %d: Unreleased has a date; a release's section is ## [x.y.z] - YYYY-MM-DD", l.n))
		}
		if to != "" && semver.MatchString(from) && semver.MatchString(to) && compareVersions(from, to) >= 0 {
			problems = append(problems, fmt.Sprintf("line %d: the range %s to %s runs backwards; the lower version comes first", l.n, from, to))
		}
		for _, v := range []string{from, to} {
			if v == "" {
				continue
			}
			if v != unreleased && date == "" {
				problems = append(problems, fmt.Sprintf("line %d: %s has no release date", l.n, v))
			}
			if !refs[v] {
				problems = append(problems, fmt.Sprintf("line %d: no link reference for [%s] at the end of the file", l.n, v))
			}
		}
		top := version(m)
		if prev != "" && compareVersions(top, prev) >= 0 {
			problems = append(problems, fmt.Sprintf("line %d: %s is not below %s; newest first", l.n, top, prev))
		}
		prev = from
	}
	if first {
		problems = append(problems, "no release headings")
	}
	return problems
}
