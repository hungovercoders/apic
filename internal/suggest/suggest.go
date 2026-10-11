// Package suggest ranks the names a user probably meant when the one they
// typed matches nothing: request ids, environment names and the like.
package suggest

import (
	"sort"
	"strings"
)

// Max is how many names Closest returns at most: enough to show the
// likely intended one, few enough to read in an error line.
const Max = 3

// Closest returns up to Max of candidates that are close to query, best
// first: a candidate query is a prefix of, then one query occurs in, then
// one within a few edits of it (one edit for a short query, three for a
// long one). Case, and the difference between - and _, do not count. It
// returns nil when nothing is close enough, or the query is a single
// letter: a wrong name should not get a guess for the sake of it.
func Closest(query string, candidates []string) []string {
	q := normalize(query)
	if len(q) < 2 {
		return nil // one letter is inside half the names
	}
	limit := 1 + len(q)/5
	if limit > 3 {
		limit = 3
	}
	type scored struct {
		name string
		rank int // 0 prefix, 1 contains, 2 edits
		dist int
	}
	var found []scored
	seen := map[string]bool{}
	for _, c := range candidates {
		if seen[c] {
			continue
		}
		seen[c] = true
		n := normalize(c)
		switch {
		case n == q:
			found = append(found, scored{c, 0, 0})
		case strings.HasPrefix(n, q):
			found = append(found, scored{c, 0, len(n) - len(q)})
		case strings.Contains(n, q):
			found = append(found, scored{c, 1, len(n) - len(q)})
		default:
			if d := distance(q, n); d <= limit && d < len(q) {
				found = append(found, scored{c, 2, d})
			}
		}
	}
	sort.Slice(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if a.rank != b.rank {
			return a.rank < b.rank
		}
		if a.dist != b.dist {
			return a.dist < b.dist
		}
		return a.name < b.name
	})
	if len(found) == 0 {
		return nil
	}
	if len(found) > Max {
		found = found[:Max]
	}
	out := make([]string, 0, len(found))
	for _, s := range found {
		out = append(out, s.name)
	}
	return out
}

// Phrase renders names as "did you mean a, b or c?", or "" for none.
func Phrase(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return "did you mean " + names[0] + "?"
	}
	return "did you mean " + strings.Join(names[:len(names)-1], ", ") + " or " + names[len(names)-1] + "?"
}

func normalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
}

// distance is the Levenshtein edit distance between a and b, by runes.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 {
		return len(rb)
	}
	if len(rb) == 0 {
		return len(ra)
	}
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}
