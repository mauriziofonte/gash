// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"sort"
)

// Version ordering: GNU `sort -V`, whose key comparison is gnulib's
// filevercmp (LEDGER DEV-13). Lines that filevercmp finds equal keep
// sort's last-resort order, the bytes of the whole line (the C locale).

// verCmp is filevercmp(a, b): negative, zero or positive.
func verCmp(a, b string) int {
	// Empty names first; then ".", "..", other dot names, other names.
	switch {
	case a == "":
		if b == "" {
			return 0
		}
		return -1
	case b == "":
		return 1
	}
	if a[0] == '.' {
		if b[0] != '.' {
			return -1
		}
		switch {
		case a == ".":
			if b == "." {
				return 0
			}
			return -1
		case b == ".":
			return 1
		case a == "..":
			if b == ".." {
				return 0
			}
			return -1
		case b == "..":
			return 1
		}
	} else if b[0] == '.' {
		return 1
	}
	ap, bp := filePrefixLen(a), filePrefixLen(b)
	onePass := ap == len(a) && bp == len(b)
	r := verRevCmp(a[:ap], b[:bp])
	if r != 0 || onePass {
		return r
	}
	return verRevCmp(a, b)
}

// filePrefixLen is the length of s without its longest suffix matching
// (\.[A-Za-z~][A-Za-z0-9~]*)*$ (a file name extension).
func filePrefixLen(s string) int {
	n := len(s)
	prefix := 0
	for i := 0; ; {
		if i == n {
			return prefix
		}
		i++
		prefix = i
		for i+1 < n && s[i] == '.' && (isAlpha(s[i+1]) || s[i+1] == '~') {
			for i += 2; i < n && (isAlnum(s[i]) || s[i] == '~'); i++ {
			}
		}
	}
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }
func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isAlnum(c byte) bool { return isDigit(c) || isAlpha(c) }

// order is the weight of the byte at pos (-1 past the end): digits 0,
// letters their code, ~ first of all, anything else after the letters.
func order(s string, pos int) int {
	if pos == len(s) {
		return -1
	}
	c := s[pos]
	switch {
	case isDigit(c):
		return 0
	case isAlpha(c):
		return int(c)
	case c == '~':
		return -2
	}
	return int(c) + 256
}

// verRevCmp is dpkg's verrevcmp as gnulib modified it.
func verRevCmp(a, b string) int {
	i, j := 0, 0
	for i < len(a) || j < len(b) {
		firstDiff := 0
		for (i < len(a) && !isDigit(a[i])) || (j < len(b) && !isDigit(b[j])) {
			ac, bc := order(a, i), order(b, j)
			if ac != bc {
				return ac - bc
			}
			i++
			j++
		}
		for i < len(a) && a[i] == '0' {
			i++
		}
		for j < len(b) && b[j] == '0' {
			j++
		}
		for i < len(a) && j < len(b) && isDigit(a[i]) && isDigit(b[j]) {
			if firstDiff == 0 {
				firstDiff = int(a[i]) - int(b[j])
			}
			i++
			j++
		}
		if i < len(a) && isDigit(a[i]) {
			return 1
		}
		if j < len(b) && isDigit(b[j]) {
			return -1
		}
		if firstDiff != 0 {
			return firstDiff
		}
	}
	return 0
}

// sortV is `sort -V` of lines.
func sortV(lines []string) []string {
	out := append([]string(nil), lines...)
	sort.SliceStable(out, func(i, j int) bool {
		if c := verCmp(out[i], out[j]); c != 0 {
			return c < 0
		}
		return out[i] < out[j]
	})
	return out
}
