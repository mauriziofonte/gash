// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmfmt

import (
	"unicode"
	"unicode/utf8"
)

// Ellipsis marks a line or value that --width cut.
const Ellipsis = "…"

// RuneWidth is the display width of r in a terminal: 0 for combining
// marks, format characters and controls, 2 for East Asian wide and
// fullwidth characters and the emoji blocks, 1 otherwise. An invalid byte
// counts 1 (it prints as one replacement character).
func RuneWidth(r rune) int {
	switch {
	case r == utf8.RuneError:
		return 1
	case r < 0x20 || r == 0x7f:
		return 0
	case r < 0x300:
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case isWide(r):
		return 2
	}
	return 1
}

// isWide covers the East Asian Wide (W) and Fullwidth (F) ranges that
// matter in practice, and the emoji presentation blocks.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals .. CJK symbols
		r >= 0x3041 && r <= 0x33FF, // Hiragana .. CJK compatibility
		r >= 0x3400 && r <= 0x4DBF, // CJK extension A
		r >= 0x4E00 && r <= 0x9FFF, // CJK unified ideographs
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE4F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F, // emoji
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x1FA70 && r <= 0x1FAFF,
		r >= 0x20000 && r <= 0x3FFFD: // CJK extensions B..
		return true
	}
	return false
}

// StringWidth is the display width of s.
func StringWidth(s string) int {
	w := 0
	for i := 0; i < len(s); {
		if s[i] < utf8.RuneSelf {
			if s[i] >= 0x20 && s[i] != 0x7f {
				w++
			}
			i++
			continue
		}
		r, n := utf8.DecodeRuneInString(s[i:])
		w += RuneWidth(r)
		i += n
	}
	return w
}

// Cut returns s cut to at most width display columns: when s is wider, it
// keeps what fits in width-1 columns and appends Ellipsis. width <= 0
// returns s unchanged. Cut never splits a UTF-8 sequence.
func Cut(s string, width int) string {
	if width <= 0 {
		return s
	}
	// Fast path: ASCII no wider than width.
	if len(s) <= width {
		ascii := true
		for i := 0; i < len(s); i++ {
			if s[i] >= utf8.RuneSelf {
				ascii = false
				break
			}
		}
		if ascii {
			return s
		}
	}
	if StringWidth(s) <= width {
		return s
	}
	limit := width - 1
	w := 0
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		rw := RuneWidth(r)
		if w+rw > limit {
			return s[:i] + Ellipsis
		}
		w += rw
		i += n
	}
	return s
}
