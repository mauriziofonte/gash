// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"sort"
	"strings"
)

// Suggest returns up to two canonical names close to typed (the words
// after gash, joined by spaces): a misspelled command (git tagg add), a
// misspelled namespace, or a shortcut or former name typed after gash
// (gash flf, gash git_sync).
func Suggest(typed string) []string {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return nil
	}
	if c, ok := CanonicalName(typed); ok {
		return []string{c.Name()}
	}
	type cand struct {
		name string
		d    int
	}
	var cands []cand
	seen := map[string]bool{}
	consider := func(key, name string) {
		if seen[key] {
			return
		}
		seen[key] = true
		d := distance(typed, key)
		// The typed words may carry arguments: compare as many words.
		if n := len(strings.Fields(key)); n < len(strings.Fields(typed)) {
			d = min(d, distance(strings.Join(strings.Fields(typed)[:n], " "), key))
		}
		limit := max(2, len(key)/4)
		if len(key) <= 4 {
			limit = 1 // a short word is one edit away, or another word
		}
		if d <= limit {
			cands = append(cands, cand{name, d})
		}
	}
	for i := range tree {
		c := &tree[i]
		if c.Hidden {
			continue
		}
		consider(c.Words(), c.Name())
		for _, f := range c.FormerNames() {
			consider(f, c.Name())
		}
		if c.Shortcut != "" {
			consider(c.Shortcut, c.Name())
		}
	}
	first, _, _ := strings.Cut(typed, " ")
	for _, ns := range namespaces {
		if ns.Name != first {
			consider(ns.Name, "gash "+ns.Name)
		}
	}
	sort.SliceStable(cands, func(i, j int) bool { return cands[i].d < cands[j].d })
	var out []string
	for _, c := range cands {
		if len(out) == 2 {
			break
		}
		if !containsString(out, c.name) {
			out = append(out, c.name)
		}
	}
	return out
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// distance is the optimal string alignment distance of a and b (edits,
// with a swap of two neighbours counted as one).
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	d := make([][]int, len(ra)+1)
	for i := range d {
		d[i] = make([]int, len(rb)+1)
		d[i][0] = i
	}
	for j := range d[0] {
		d[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			d[i][j] = min(d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				d[i][j] = min(d[i][j], d[i-2][j-2]+1)
			}
		}
	}
	return d[len(ra)][len(rb)]
}
