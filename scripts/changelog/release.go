package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// A gitError is git failing, as against a file that is not at a commit:
// the walk in ReleaseCommit stops on it rather than reading it as "this
// commit fails the release check" and tagging a later one.
type gitError struct{ err error }

func (e *gitError) Error() string { return e.err.Error() }
func (e *gitError) Unwrap() error { return e.err }

// git runs git in dir with its messages in English whatever the locale,
// and returns its output or a gitError carrying what it wrote to stderr.
func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // git, with refs and paths from this repository
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", &gitError{fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))}
	}
	return out.String(), nil
}

// fromGit reads files as they were at commit. Paths are relative to dir,
// the directory holding the changelog, as they are on disk, whichever
// directory of the repository that is: both git commands are given the
// path from the repository's top. A path the commit does not have is
// fs.ErrNotExist, found by listing the tree rather than by reading git's
// message; anything else that fails is a gitError.
func fromGit(dir, commit string) reader {
	prefix, prefixErr := git(dir, "rev-parse", "--show-prefix")
	prefix = strings.TrimSpace(prefix)
	return func(path string) (string, error) {
		if prefixErr != nil {
			return "", prefixErr
		}
		full := prefix + path
		listed, err := git(dir, "ls-tree", "--full-tree", "--name-only", commit, "--", full)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(listed) == "" {
			return "", &fs.PathError{Op: "read", Path: commit + ":" + full, Err: fs.ErrNotExist}
		}
		return git(dir, "show", commit+":"+full)
	}
}

// ReleaseCommit picks the commit to tag for tag: walking head's
// first-parent history (main's own commits, a merge counting as one) back
// to where tag stopped being the newest dated section, it returns the
// oldest commit at which the release check passes. That is the commit the
// release pull request left on main, not whichever later push ran the
// workflow, so changes merged since, still under Unreleased, are not
// shipped under this version's notes. When a pull request dated the
// section in one commit and caught the docs up in the next, and both
// landed on main as they were, the second is the one that passes. A
// commit whose changelog is malformed is skipped, not taken for the start
// of the release: the walk goes on to the commit that dated it. A git
// failure anywhere ends the walk with an error rather than narrowing it.
// name is the changelog's file name in dir.
func ReleaseCommit(dir, name, head, tag string) (string, error) {
	out, err := git(dir, "rev-list", "--first-parent", head)
	if err != nil {
		return "", err
	}
	type candidate struct{ commit, changelog string }
	var candidates []candidate // newest first
	for _, c := range strings.Fields(out) {
		changelog, err := fromGit(dir, c)(name)
		if errors.Is(err, fs.ErrNotExist) {
			break // before the changelog existed
		}
		if err != nil {
			return "", err
		}
		latest, err := Latest(changelog)
		if err != nil {
			continue // malformed here: not a candidate, and not the start either
		}
		if latest != tag {
			break // before this version was dated
		}
		candidates = append(candidates, candidate{c, changelog})
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("%s is not the newest dated section of CHANGELOG.md at %s", tag, head)
	}
	var last error
	for i := len(candidates) - 1; i >= 0; i-- {
		c := candidates[i]
		_, err := Release(fromGit(dir, c.commit), c.changelog, tag)
		var ge *gitError
		if errors.As(err, &ge) {
			return "", err
		}
		if err != nil {
			last = err
			continue
		}
		return c.commit, nil
	}
	return "", fmt.Errorf("no commit on main since %s was dated passes the release check; at %s: %w", tag, head, last)
}

// NewerTags lists the version tags in the repository at dir that are above
// tag, lowest first. Releasing tag again while one exists would move the image's latest,
// the cask and setup-apic's v0 back to an older release, so the release
// check refuses it.
func NewerTags(dir, tag string) ([]string, error) {
	out, err := git(dir, "tag", "--list", "v[0-9]*")
	if err != nil {
		return nil, err
	}
	var newer []string
	for _, t := range strings.Fields(out) {
		v := strings.TrimPrefix(t, "v")
		if semver.MatchString(v) && compareVersions(v, tag) > 0 {
			newer = append(newer, t)
		}
	}
	slices.SortFunc(newer, func(a, b string) int { return compareVersions(a[1:], b[1:]) })
	return newer, nil
}
