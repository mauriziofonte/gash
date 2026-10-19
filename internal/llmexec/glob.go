// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// globExpand expands the glob pattern pat the way `compgen -G` does when run
// in cwd, but in byte (C) order instead of the locale collation (EXEC-06, the
// forced divergence of architecture.md 8.3: Go expands globs itself so bash
// never re-globs a word). The returned matches are in the same form the
// pattern was written (relative to cwd for a relative pattern, absolute for
// an absolute one), so they read back exactly as compgen's output. No match
// returns an empty slice.
//
// Dotglob and globstar are off, as in the 2.x child: a "*" does not match a
// leading dot and does not cross "/".
func globExpand(pat, cwd string) []string {
	if strings.HasPrefix(pat, "/") {
		return globWalk("/", "/", strings.Split(pat[1:], "/"))
	}
	return globWalk("", cwd, strings.Split(pat, "/"))
}

// globWalk expands the remaining segments against realDir, building result
// paths with the display prefix resultPrefix.
func globWalk(resultPrefix, realDir string, segs []string) []string {
	if len(segs) == 0 {
		return []string{resultPrefix}
	}
	seg := segs[0]
	rest := segs[1:]
	if seg == "" {
		return globWalk(resultPrefix, realDir, rest)
	}
	if !hasMeta(seg) {
		lit := unescapeGlob(seg)
		newPrefix := joinDisplay(resultPrefix, lit)
		newReal := joinReal(realDir, lit)
		if len(rest) == 0 {
			if _, err := os.Lstat(newReal); err == nil {
				return []string{newPrefix}
			}
			return nil
		}
		return globWalk(newPrefix, newReal, rest)
	}
	entries, err := os.ReadDir(realDir)
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names) // byte order (EXEC-06)
	var out []string
	for _, name := range names {
		if strings.HasPrefix(name, ".") && !strings.HasPrefix(seg, ".") {
			continue
		}
		if !matchSeg(seg, name) {
			continue
		}
		newPrefix := joinDisplay(resultPrefix, name)
		newReal := joinReal(realDir, name)
		if len(rest) == 0 {
			out = append(out, newPrefix)
		} else {
			out = append(out, globWalk(newPrefix, newReal, rest)...)
		}
	}
	return out
}

// hasMeta reports whether seg contains an unescaped glob metacharacter.
func hasMeta(seg string) bool {
	for i := 0; i < len(seg); i++ {
		if seg[i] == '\\' {
			i++
			continue
		}
		switch seg[i] {
		case '*', '?', '[':
			return true
		}
	}
	return false
}

// unescapeGlob removes backslash escapes from a non-glob segment, so a quoted
// metacharacter is looked up as its literal name.
func unescapeGlob(seg string) string {
	if !strings.Contains(seg, "\\") {
		return seg
	}
	var b strings.Builder
	for i := 0; i < len(seg); i++ {
		if seg[i] == '\\' && i+1 < len(seg) {
			i++
			b.WriteByte(seg[i])
			continue
		}
		b.WriteByte(seg[i])
	}
	return b.String()
}

// matchSeg matches one path segment with the shell pattern rules, exported
// via pathguard's matcher so the two packages agree.
func matchSeg(pat, name string) bool {
	return pathguard.MatchGlobSegment(pat, name)
}

func joinDisplay(prefix, seg string) string {
	switch prefix {
	case "":
		return seg
	case "/":
		return "/" + seg
	default:
		return prefix + "/" + seg
	}
}

func joinReal(realDir, name string) string {
	if realDir == "/" {
		return "/" + name
	}
	return realDir + "/" + name
}
