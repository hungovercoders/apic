package main

import (
	"bytes"
	"fmt"
	"io/fs"
	"os/exec"
	"strings"
)

// fromGit reads files as they were at commit, in the repository at dir.
func fromGit(dir, commit string) reader {
	return func(path string) (string, error) {
		cmd := exec.Command("git", "-C", dir, "show", commit+":"+path) //nolint:gosec // git, with a commit and a path from this repository
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		if err := cmd.Run(); err != nil {
			msg := strings.TrimSpace(errb.String())
			if strings.Contains(msg, "does not exist") || strings.Contains(msg, "exists on disk, but not in") {
				return "", &fs.PathError{Op: "read", Path: commit + ":" + path, Err: fs.ErrNotExist}
			}
			return "", fmt.Errorf("git show %s:%s: %v: %s", commit, path, err, msg)
		}
		return out.String(), nil
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
// landed on main as they were, the second is the one that passes.
func ReleaseCommit(dir, head, tag string) (string, error) {
	out, err := exec.Command("git", "-C", dir, "rev-list", "--first-parent", head).Output() //nolint:gosec // git, with a ref from this repository
	if err != nil {
		return "", fmt.Errorf("git rev-list %s: %v", head, err)
	}
	var candidates []string // newest first
	for _, c := range strings.Fields(string(out)) {
		changelog, err := fromGit(dir, c)("CHANGELOG.md")
		if err != nil {
			break
		}
		if latest, err := Latest(changelog); err != nil || latest != tag {
			break
		}
		candidates = append(candidates, c)
	}
	if len(candidates) == 0 {
		return "", fmt.Errorf("%s is not the newest dated section of CHANGELOG.md at %s", tag, head)
	}
	var last error
	for i := len(candidates) - 1; i >= 0; i-- {
		c := candidates[i]
		read := fromGit(dir, c)
		changelog, err := read("CHANGELOG.md")
		if err != nil {
			return "", err
		}
		if _, err := Release(read, changelog, tag); err != nil {
			last = err
			continue
		}
		return c, nil
	}
	return "", fmt.Errorf("no commit on main since %s was dated passes the release check; at %s: %w", tag, head, last)
}
