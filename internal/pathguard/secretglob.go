// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

// The llm_exec wrappers (grep, rg, diff, git) keep secret files out with the
// tools' own exclude globs, but an exclude glob cannot say "except": grep
// --include, rg whitelist globs and git pathspecs all change which other files
// are searched once one is given (BSD grep and rg then search only the
// included names). So the exceptions are taken out of the patterns
// themselves: SecretExcludeGlobs is a list of plain globs whose union is
// exactly the names SecretName calls secret by pattern, the exceptions left
// out (F-05). Only the patterns an exception can match are rewritten:
// "LIT*" and "*LIT*" (.env.*, id_rsa*, ...).
//
// A literal exception (.env.example, ...) is honored by every pattern it
// matches. A "*Y" exception is honored by the patterns suffixScope lists:
// *.pub by the key names id_rsa*, id_dsa*, id_ecdsa*, id_ed25519*; the
// remaining overlaps (credentials.pub, secrets.env.example, x.tfstate.pub,
// .env.local.env.example, ...) stay excluded inside llm_exec, the safe
// direction, which keeps the list short enough for the child's bash -c
// argument.
//
// For such a pattern a name is prefix+LIT+t. Whether it is an exception
// depends on t only: t equals the rest of a literal exception (.env.example
// is .env. + "example"), or the name ends with the literal part of a "*Y"
// exception (*.pub, *.env.example), which for a t shorter than Y can take
// its first bytes from LIT. For each length of t below the longest
// exception that is a finite set of fixed-length patterns, complemented
// position by position into globs of literals, '?' and negated classes;
// from that length on only the last bytes of t matter, so one "LIT*" plus
// a fixed-length complement covers every longer t. The construction is
// exact in byte and in character semantics (exceptions are ASCII), so it
// holds whether a tool's '?' matches a byte or a UTF-8 character.

// globElem is one element of a generated glob: a literal byte, '?' (any),
// '*' (star) or a negated class of the bytes in not.
type globElem struct {
	kind byte // 'l' literal, '?' any, '*' star, '!' negated class
	c    byte
	not  string
}

// suffixScope lists, for each "*Y" exception, the patterns that honor it.
var suffixScope = map[string][]string{
	"*.pub": {"id_rsa*", "id_dsa*", "id_ecdsa*", "id_ed25519*"},
}

var (
	secretGlobsOnce sync.Once
	secretGlobs     [][]globElem
)

// SecretExcludeGlobs returns the secret-pattern globs with the exceptions
// taken out, rendered for a case-insensitive matcher when icase is false
// (rg --iglob, git :(icase)) or as case-folded globs for grep and diff when
// icase is true (a letter x becomes [xX], a class lists both cases).
func SecretExcludeGlobs(icase bool) []string {
	secretGlobsOnce.Do(func() {
		var err error
		secretGlobs, err = buildSecretGlobs(SecretPatterns, SecretExceptions, suffixScope)
		if err != nil {
			panic("pathguard: " + err.Error())
		}
	})
	out := make([]string, len(secretGlobs))
	for i, g := range secretGlobs {
		out[i] = renderGlob(g, icase)
	}
	return out
}

// buildSecretGlobs is patterns minus exceptions; scope says which patterns
// honor each "*Y" exception (nil: every pattern).
func buildSecretGlobs(patterns, exceptions []string, scope map[string][]string) ([][]globElem, error) {
	var words, suffixes []string
	for _, e := range exceptions {
		switch {
		case !strings.ContainsAny(e, "*?[\\"):
			words = append(words, e)
		case e[0] == '*' && !strings.ContainsAny(e[1:], "*?[\\"):
			suffixes = append(suffixes, e[1:])
		default:
			return nil, fmt.Errorf("exception %q is neither a word nor *WORD", e)
		}
	}
	var out [][]globElem
	for _, p := range patterns {
		var applied []string
		for _, y := range suffixes {
			if scope == nil || contains(scope["*"+y], p) {
				applied = append(applied, y)
			}
		}
		gs, err := patternMinusExceptions(p, words, applied)
		if err != nil {
			return nil, err
		}
		out = append(out, gs...)
	}
	return out, nil
}

// patternMinusExceptions is one secret pattern without the exception names.
func patternMinusExceptions(p string, words, suffixes []string) ([][]globElem, error) {
	if strings.ContainsAny(p, "?[\\") {
		return nil, fmt.Errorf("pattern %q: only literals and * are supported", p)
	}
	if !strings.HasSuffix(p, "*") {
		// A pattern ending in a literal X: no exception may match it (none
		// does in the tables; the tests prove it), so it stays as it is.
		x := p[strings.LastIndexByte(p, '*')+1:]
		for _, w := range words {
			if matchGlob(p, w) {
				return nil, fmt.Errorf("pattern %q matches exception %q", p, w)
			}
		}
		for _, y := range suffixes {
			if strings.HasSuffix(x, y) || strings.HasSuffix(y, x) {
				return nil, fmt.Errorf("pattern %q overlaps exception *%s", p, y)
			}
		}
		return [][]globElem{literalElems(p)}, nil
	}
	body := p[:len(p)-1]
	lead := strings.HasPrefix(body, "*")
	lit := strings.TrimPrefix(body, "*")
	if strings.Contains(lit, "*") {
		return nil, fmt.Errorf("pattern %q: only LIT* and *LIT* are supported", p)
	}
	head := literalElems(body)
	if lead {
		// prefix+LIT+t: an exception must not depend on the prefix.
		for _, w := range words {
			if matchGlob(p, w) {
				return nil, fmt.Errorf("pattern %q matches exception %q", p, w)
			}
		}
		for _, y := range suffixes {
			for m := len(lit) + 1; m <= len(y); m++ {
				if strings.HasSuffix(y[:m], lit) {
					return nil, fmt.Errorf("pattern %q: exception *%s spans its prefix", p, y)
				}
			}
		}
	}
	// limit is the first tail length from which only "t ends with Y" counts.
	limit := 0
	for _, y := range suffixes {
		limit = max(limit, len(y))
	}
	for _, w := range words {
		if !lead && strings.HasPrefix(w, lit) {
			limit = max(limit, len(w)-len(lit)+1)
		}
	}
	var out [][]globElem
	for n := 0; n < limit; n++ {
		var excl []string // fixed-length patterns of t, '?' any
		for _, w := range words {
			if !lead && strings.HasPrefix(w, lit) && len(w)-len(lit) == n {
				excl = append(excl, w[len(lit):])
			}
		}
		for _, y := range suffixes {
			switch m := len(y) - n; {
			case m <= 0:
				excl = append(excl, strings.Repeat("?", -m)+y)
			case m <= len(lit) && strings.HasSuffix(lit, y[:m]):
				excl = append(excl, y[m:])
			}
		}
		for _, tail := range complementFixed(n, excl) {
			out = append(out, append(append([]globElem{}, head...), tail...))
		}
	}
	var excl []string
	for _, y := range suffixes {
		excl = append(excl, strings.Repeat("?", limit-len(y))+y)
	}
	star := append(append([]globElem{}, head...), globElem{kind: '*'})
	for _, tail := range complementFixed(limit, excl) {
		out = append(out, append(append([]globElem{}, star...), tail...))
	}
	return out, nil
}

// complementFixed returns globs matching exactly the strings of length n
// that match none of pats (each of length n, '?' any, other bytes literal,
// compared ignoring ASCII case).
func complementFixed(n int, pats []string) [][]globElem {
	var out [][]globElem
	var walk func(pos int, pats []string, prefix []globElem)
	walk = func(pos int, pats []string, prefix []globElem) {
		if len(pats) == 0 {
			g := append([]globElem{}, prefix...)
			for i := pos; i < n; i++ {
				g = append(g, globElem{kind: '?'})
			}
			out = append(out, g)
			return
		}
		if pos == n {
			return // an exception: left out
		}
		set := map[byte]bool{}
		var anyPats []string
		for _, p := range pats {
			if p[pos] == '?' {
				anyPats = append(anyPats, p)
			} else {
				set[asciiLowerByte(p[pos])] = true
			}
		}
		lits := make([]byte, 0, len(set))
		for c := range set {
			lits = append(lits, c)
		}
		sort.Slice(lits, func(i, j int) bool { return lits[i] < lits[j] })
		for _, c := range lits {
			var sub []string
			for _, p := range pats {
				if p[pos] == '?' || asciiLowerByte(p[pos]) == c {
					sub = append(sub, p)
				}
			}
			walk(pos+1, sub, append(prefix[:len(prefix):len(prefix)], globElem{kind: 'l', c: c}))
		}
		other := globElem{kind: '?'}
		if len(lits) > 0 {
			other = globElem{kind: '!', not: string(lits)}
		}
		walk(pos+1, anyPats, append(prefix[:len(prefix):len(prefix)], other))
	}
	walk(0, pats, nil)
	return out
}

func asciiLowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c | 0x20
	}
	return c
}

func literalElems(s string) []globElem {
	var g []globElem
	for i := 0; i < len(s); i++ {
		if s[i] == '*' {
			g = append(g, globElem{kind: '*'})
		} else {
			g = append(g, globElem{kind: 'l', c: asciiLowerByte(s[i])})
		}
	}
	return g
}

// renderGlob writes g as fnmatch/globset/wildmatch syntax. A negated class
// lists both cases of a letter in either mode; '-' goes last so it is a
// member, not a range.
func renderGlob(g []globElem, icase bool) string {
	var b strings.Builder
	for _, e := range g {
		switch e.kind {
		case '*', '?':
			b.WriteByte(e.kind)
		case 'l':
			if icase && e.c >= 'a' && e.c <= 'z' {
				b.WriteString("[" + string(e.c) + string(e.c&^0x20) + "]")
			} else {
				b.WriteByte(e.c)
			}
		case '!':
			b.WriteString("[!")
			dash := false
			for i := 0; i < len(e.not); i++ {
				c := e.not[i]
				switch {
				case c == '-':
					dash = true
				case c >= 'a' && c <= 'z':
					b.WriteByte(c)
					b.WriteByte(c &^ 0x20)
				default:
					b.WriteByte(c)
				}
			}
			if dash {
				b.WriteByte('-')
			}
			b.WriteByte(']')
		}
	}
	return b.String()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
