// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sqlguard

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// reading is one way of reading the locale-dependent parts of the 2.x
// guard: the [[:space:]] class (the mysql "-- " rule, the trims of the
// code and of each line, the blanks before the first word and inside
// LOAD DATA) and the case mapping of ${..^^} and ${..,,}.
//
// LEDGER DB-13: 2.x refuses under a UTF-8 locale queries that it accepts
// under LC_ALL=C (SELECT ſleep(1)), and the other way round. The guard
// refuses when any reading refuses, so it is at least as strict as 2.x
// under either locale.
type reading int

const (
	// readC is LC_ALL=C: ASCII blanks, ASCII case.
	readC reading = iota
	// readUTF8 is a UTF-8 locale with towupper as glibc applies it: a
	// letter whose upper case is ASCII becomes that letter (ı, ſ).
	readUTF8
	// readUTF8Fold also maps a letter whose case-fold orbit holds an ASCII
	// letter (the Kelvin sign). It is not readUTF8's superset: "ſLEEPK"
	// with the Kelvin sign is SLEEP plus a separator under readUTF8 but
	// the word SLEEPK here, so both run.
	readUTF8Fold
)

// readings run in this order, and the first refusal is the one reported:
// the C reading first, so a query 2.x refuses under LC_ALL=C gets the same
// error.
var readings = [...]reading{readC, readUTF8, readUTF8Fold}

// blankAt is the size of the [[:space:]] character at s[i:], 0 when there
// is none. The UTF-8 readings take every Unicode space (unicode.IsSpace):
// a superset of glibc's iswspace, which in the trims, the first-word check
// and LOAD DATA only adds refusals.
func (rd reading) blankAt(s string, i int) int {
	c := s[i]
	if c < utf8.RuneSelf || rd == readC {
		if isSpace(c) {
			return 1
		}
		return 0
	}
	r, size := utf8.DecodeRuneInString(s[i:])
	if r == utf8.RuneError && size == 1 {
		return 0
	}
	if unicode.IsSpace(r) {
		return size
	}
	return 0
}

// commentBlankAt is the mysql rule [[ ${query:i+2:1} =~ ^[[:space:]]?$ ]]
// at s[j:]: the end of the query or a blank. Here a wider class would hide
// text from the checks (a comment ends only at the newline), so the UTF-8
// readings take exactly glibc's iswspace class of UTF-8 locales (checked
// against glibc 2.39): the ASCII blanks, U+1680, U+2000 to U+2006, U+2008
// to U+200A, U+2028, U+2029, U+205F and U+3000; not the no-break spaces.
func (rd reading) commentBlankAt(s string, j int) bool {
	if j >= len(s) {
		return true
	}
	if s[j] < utf8.RuneSelf || rd == readC {
		return isSpace(s[j])
	}
	r, _ := utf8.DecodeRuneInString(s[j:])
	switch {
	case r == 0x1680, r >= 0x2000 && r <= 0x2006, r >= 0x2008 && r <= 0x200A,
		r == 0x2028, r == 0x2029, r == 0x205F, r == 0x3000:
		return true
	}
	return false
}

// trimLeft removes the leading blanks: ${s#"${s%%[![:space:]]*}"}.
func (rd reading) trimLeft(s string) string {
	for s != "" {
		n := rd.blankAt(s, 0)
		if n == 0 {
			break
		}
		s = s[n:]
	}
	return s
}

// trimLeftParen removes the leading blanks and "(" before the first word:
// ${upper#"${upper%%[![:space:](]*}"}.
func (rd reading) trimLeftParen(s string) string {
	for s != "" {
		n := rd.blankAt(s, 0)
		if n == 0 && s[0] == '(' {
			n = 1
		}
		if n == 0 {
			break
		}
		s = s[n:]
	}
	return s
}

// trimRight removes the trailing blanks: ${s%"${s##*[![:space:]]}"}.
func (rd reading) trimRight(s string) string {
	for s != "" {
		c := s[len(s)-1]
		if c < utf8.RuneSelf || rd == readC {
			if !isSpace(c) {
				break
			}
			s = s[:len(s)-1]
			continue
		}
		r, size := utf8.DecodeLastRuneInString(s)
		if r == utf8.RuneError && size == 1 || !unicode.IsSpace(r) {
			break
		}
		s = s[:len(s)-size]
	}
	return s
}

// upper is ${s^^} under the reading. Letters without an ASCII mapping and
// invalid bytes stay as they are (they separate words like any other
// non-word character).
func (rd reading) upper(s string) string {
	b := make([]byte, 0, len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= 'a' && c <= 'z' {
				c -= 'a' - 'A'
			}
			b = append(b, c)
			i++
			continue
		}
		if rd == readC {
			b = append(b, c)
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if a, ok := rd.toASCII(r); ok {
			b = append(b, a)
		} else {
			b = append(b, s[i:i+size]...)
		}
		i += size
	}
	return string(b)
}

// toASCII maps a non-ASCII letter to its ASCII upper-case letter: by
// unicode.ToUpper (ı, ſ), and under readUTF8Fold also through its
// unicode.SimpleFold orbit (the Kelvin sign).
func (rd reading) toASCII(r rune) (byte, bool) {
	if r == utf8.RuneError {
		return 0, false
	}
	if u := unicode.ToUpper(r); u < utf8.RuneSelf && isASCIILetter(u) {
		return byte(u), true
	}
	if rd != readUTF8Fold {
		return 0, false
	}
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < utf8.RuneSelf && isASCIILetter(f) {
			return byte(unicode.ToUpper(f)), true
		}
	}
	return 0, false
}

func isASCIILetter(r rune) bool { return r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' }

// isGo is [[ ${line,,} == go ]] under the reading.
func (rd reading) isGo(line string) bool {
	return rd.upper(line) == "GO"
}

// hasLoadData is [[ $upper =~ LOAD[[:space:]]+(DATA|XML) ]], unanchored.
func (rd reading) hasLoadData(upper string) bool {
	for i := 0; ; {
		k := strings.Index(upper[i:], "LOAD")
		if k < 0 {
			return false
		}
		j := i + k + 4
		start := j
		for j < len(upper) {
			n := rd.blankAt(upper, j)
			if n == 0 {
				break
			}
			j += n
		}
		if j > start && (strings.HasPrefix(upper[j:], "DATA") || strings.HasPrefix(upper[j:], "XML")) {
			return true
		}
		i += k + 1
	}
}

// isSpace is [[:space:]] in the C locale.
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\v' || c == '\f' || c == '\r'
}
