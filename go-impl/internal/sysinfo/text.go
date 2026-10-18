// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The text tools of 2.x, as functions over bytes and lines, so that the
// output never depends on which grep, awk or sort is installed. Listings
// and sorts are in byte order (LEDGER SYS-41).

// lines is how `while IFS= read -r line || [[ -n $line ]]` reads data: NUL
// bytes dropped, a last line without newline kept.
func lines(data []byte) []string {
	s := string(data)
	if strings.IndexByte(s, 0) >= 0 {
		s = strings.ReplaceAll(s, "\x00", "")
	}
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// readLines is a `while read -r ...` loop without the `|| [[ -n ]]` guard:
// a last line without newline is not read.
func readLines(data []byte) []string {
	s := strings.ReplaceAll(string(data), "\x00", "")
	i := strings.LastIndexByte(s, '\n')
	if i < 0 {
		return nil
	}
	return strings.Split(s[:i], "\n")
}

// grepLines are the lines of data that GNU grep reads and may print: nil
// for a file holding a NUL byte (binary, nothing printed); in a UTF-8
// locale (u8) a line that is not valid UTF-8 is binary data too and is
// never printed.
func grepLines(data []byte, u8 bool) []string {
	if len(data) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return nil
	}
	ls := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if !u8 {
		return ls
	}
	out := ls[:0]
	for _, l := range ls {
		if utf8.ValidString(l) {
			out = append(out, l)
		}
	}
	return out
}

// filter is `grep -E RE` (keep) or `grep -Ev RE` (keep false) over lines.
func filter(ls []string, re *regexp.Regexp, keep bool) []string {
	var out []string
	for _, l := range ls {
		if re.MatchString(l) == keep {
			out = append(out, l)
		}
	}
	return out
}

// grep is `grep [-v] -E RE FILE` over the bytes of FILE.
func (f files) grep(data []byte, re *regexp.Regexp, keep bool) []string {
	return filter(grepLines(data, f.u8), re, keep)
}

// head is `head -N` over lines.
func head(ls []string, n int) []string {
	if len(ls) > n {
		return ls[:n]
	}
	return ls
}

// pasteS is `paste -sd, -`: the lines joined by commas, and one empty
// line for no input.
func pasteS(ls []string) []string {
	return []string{strings.Join(ls, ",")}
}

// subst is `$(...)` of a stream of lines: joined, trailing newlines gone.
func subst(ls []string) string {
	return strings.TrimRight(strings.Join(ls, "\n"), "\n")
}

// awkFields splits a line as awk's default FS does: runs of blanks
// (space, tab), leading and trailing ones ignored.
func awkFields(l string) []string {
	return strings.FieldsFunc(l, func(r rune) bool { return r == ' ' || r == '\t' })
}

// awkField is `awk '{print $N}'` for one line (N 0 is $NF).
func awkField(l string, n int) string {
	f := awkFields(l)
	if n == 0 {
		if len(f) == 0 {
			return ""
		}
		return f[len(f)-1]
	}
	if n > len(f) {
		return ""
	}
	return f[n-1]
}

// awkNum reports the value of a field awk reads as a number (a numeric
// string: optional blanks, sign, digits, a fraction, an exponent).
func awkNum(f string) (float64, bool) {
	t := strings.Trim(f, " \t\n")
	if t == "" || strings.ContainsAny(t, "xXiInN") {
		return 0, false
	}
	v, err := strconv.ParseFloat(t, 64)
	return v, err == nil
}

// awkCmp is the comparison awk makes of field f against the constant c:
// numeric when f looks numeric, else as strings.
func awkCmp(f string, c float64) int {
	if v, ok := awkNum(f); ok {
		switch {
		case v < c:
			return -1
		case v > c:
			return 1
		}
		return 0
	}
	return strings.Compare(f, strconv.FormatFloat(c, 'f', -1, 64))
}

// squeezeSpaces is `tr -s ' '`.
func squeezeSpaces(s string) string {
	for strings.Contains(s, "  ") {
		s = strings.ReplaceAll(s, "  ", " ")
	}
	return s
}

// basenameSuffix is `basename PATH SUFFIX`.
func basenameSuffix(p, suffix string) string {
	p = strings.TrimRight(p, "/")
	if p == "" {
		return "/"
	}
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		p = p[i+1:]
	}
	if suffix != "" && p != suffix {
		p = strings.TrimSuffix(p, suffix)
	}
	return p
}

// sortUniq is `sort -u` in byte order.
func sortUniq(ls []string) []string {
	out := append([]string(nil), ls...)
	sort.Strings(out)
	j := 0
	for i, l := range out {
		if i == 0 || l != out[j-1] {
			out[j] = l
			j++
		}
	}
	return out[:j]
}

// sorted is `sort` in byte order.
func sorted(ls []string) []string {
	out := append([]string(nil), ls...)
	sort.Strings(out)
	return out
}

// fnmatch matches s against a shell pattern of literal characters, `*`
// (any run, `/` included) and `?` (one character), as find -name and
// -path, and one glob component, use it.
func fnmatch(pattern, s string) bool {
	p, t := []rune(pattern), []rune(s)
	var match func(i, j int) bool
	match = func(i, j int) bool {
		for i < len(p) {
			switch p[i] {
			case '*':
				for i < len(p) && p[i] == '*' {
					i++
				}
				if i == len(p) {
					return true
				}
				for k := j; k <= len(t); k++ {
					if match(i, k) {
						return true
					}
				}
				return false
			case '?':
				if j >= len(t) {
					return false
				}
			default:
				if j >= len(t) || p[i] != t[j] {
					return false
				}
			}
			i++
			j++
		}
		return j == len(t)
	}
	return match(0, 0)
}

// pathLess orders paths as a lexical walk lists them: component by
// component in byte order.
func pathLess(a, b string) bool {
	ac, bc := strings.Split(a, "/"), strings.Split(b, "/")
	for i := 0; i < len(ac) && i < len(bc); i++ {
		if ac[i] != bc[i] {
			return ac[i] < bc[i]
		}
	}
	return len(ac) < len(bc)
}

// sortPaths orders paths as a lexical walk lists them.
func sortPaths(ps []string) {
	sort.SliceStable(ps, func(i, j int) bool { return pathLess(ps[i], ps[j]) })
}

// headN is `head -N` over a byte stream: the first N lines, the last one
// as it is when the input ends without a newline.
func headN(b []byte, n int) []byte {
	i := 0
	for k := 0; k < n; k++ {
		j := bytes.IndexByte(b[i:], '\n')
		if j < 0 {
			return b
		}
		i += j + 1
	}
	return b[:i]
}

// tailN is `tail -n N` over a byte stream: a last line without newline
// counts as a line.
func tailN(b []byte, n int) []byte {
	if n <= 0 {
		return nil
	}
	pos := len(b)
	if pos > 0 && b[pos-1] == '\n' {
		pos--
	}
	for k := 0; k < n; k++ {
		j := bytes.LastIndexByte(b[:pos], '\n')
		if j < 0 {
			return b
		}
		pos = j
	}
	return b[pos+1:]
}

// skipLine is `tail -n +2`: everything after the first line.
func skipLine(b []byte) []byte {
	if i := bytes.IndexByte(b, '\n'); i >= 0 {
		return b[i+1:]
	}
	return nil
}

// grepOAll is `grep -oP RE` over lines for an expression whose printed
// part is group 1 (a \K expression): empty matches are not printed.
func grepOAll(ls []string, re *regexp.Regexp) []string {
	var out []string
	for _, l := range ls {
		for _, m := range re.FindAllStringSubmatch(l, -1) {
			if m[1] != "" {
				out = append(out, m[1])
			}
		}
	}
	return out
}

// withEnv is env with NAME=VALUE set (an earlier NAME dropped).
func withEnv(env []string, kv string) []string {
	name := kv[:strings.IndexByte(kv, '=')+1]
	out := make([]string, 0, len(env)+1)
	for _, e := range env {
		if !strings.HasPrefix(e, name) {
			out = append(out, e)
		}
	}
	return append(out, kv)
}
