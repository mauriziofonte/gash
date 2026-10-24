// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"errors"
	"regexp"
	"regexp/syntax"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// rgRegex is a search pattern read as ripgrep 14 read it, on Go's RE2
// (LEDGER LLM-37). The pattern text is first rewritten where the two
// dialects differ: \d \w \s and their negations are Unicode classes, \u
// escapes become \x, blanks inside a counted repetition go. A pattern
// Go would read differently from rg, or one RE2 cannot express, is
// refused instead (rgRefusal). Matching is RE2's, except on the lines
// where it differs from rg's: a line that is not UTF-8 (rg matched no
// invalid byte with . or a class) and, for a pattern with \b or \B, a
// line with non-ASCII text (rg's word boundaries are Unicode). Those go
// through an NFA over RE2's own compiled program with rg's rules.
type rgRegex struct {
	re    *regexp.Regexp
	prog  *syntax.Prog
	words bool // the program has \b or \B
}

// rgRefusal is a pattern that cannot be read as rg read it.
type rgRefusal struct{ why string }

func (e *rgRefusal) Error() string { return e.why }

func refuseRg(why string) (string, int, error) { return "", 0, &rgRefusal{why} }

// compileRg compiles an rg pattern.
func compileRg(pattern string) (*rgRegex, error) {
	text, err := rgTranslate(pattern)
	if err != nil {
		return nil, err
	}
	re, err := regexp.Compile(text)
	if err != nil {
		// The kind of error only: its text quotes the rewritten pattern.
		var se *syntax.Error
		if errors.As(err, &se) {
			return nil, &rgRefusal{string(se.Code)}
		}
		return nil, &rgRefusal{err.Error()}
	}
	parsed, err := syntax.Parse(text, syntax.Perl)
	if err != nil {
		return nil, &rgRefusal{err.Error()}
	}
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, &rgRefusal{err.Error()}
	}
	r := &rgRegex{re: re, prog: prog}
	for _, inst := range prog.Inst {
		if inst.Op == syntax.InstEmptyWidth && syntax.EmptyOp(inst.Arg)&(syntax.EmptyWordBoundary|syntax.EmptyNoWordBoundary) != 0 {
			r.words = true
		}
	}
	return r, nil
}

// Match reports whether the line (no line break) has a match.
func (r *rgRegex) Match(line []byte) bool {
	if utf8.Valid(line) && (!r.words || isASCII(line)) {
		return r.re.Match(line)
	}
	return r.nfa(line)
}

func isASCII(b []byte) bool {
	for _, c := range b {
		if c >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

// rgFlagGroup is a group that only sets flags, (?i) or (?-s).
var rgFlagGroup = regexp.MustCompile(`^\(\?[A-Za-z-]*\)`)

// rgTranslate rewrites an rg pattern into the RE2 text that reads the
// same, or refuses it. rg compiled each pattern as (?:PATTERN), which is
// kept: it makes a ) that closes nothing up to there part of the syntax.
func rgTranslate(p string) (string, error) {
	var b strings.Builder
	b.WriteString("(?:")
	flags := false // the last item set flags only
	for i := 0; i < len(p); {
		var s string
		var n int
		var err error
		if flags && strings.IndexByte("*+?{", p[i]) >= 0 {
			// rg: a repetition with nothing to repeat; RE2 would repeat
			// the item before the flags.
			return "", &rgRefusal{"a repetition operator after a flag group"}
		}
		flags = false
		switch p[i] {
		case '\n':
			s, n, err = refuseRg(`the literal "\n" is not allowed in a regex`)
		case '\\':
			s, n, err = rgEscape(p[i:], false)
		case '[':
			s, n, err = rgClass(p[i:])
		case '{':
			s, n, err = rgCount(p[i:])
		case '(':
			s, n = "(", 1
			if m := rgFlagGroup.FindString(p[i:]); m != "" {
				s, n, flags = m, len(m), true
			}
		default:
			s, n = p[i:i+1], 1
		}
		if err != nil {
			return "", err
		}
		b.WriteString(s)
		i += n
	}
	b.WriteString(")")
	return b.String(), nil
}

// rgCount reads a counted repetition as rg did: {n}, {n,} or {n,m}, with
// blanks allowed around the numbers and leading zeros read as decimal.
// Any other { is an error in rg (RE2 would read it as a literal).
func rgCount(s string) (string, int, error) {
	i := 1
	blanks := func() {
		for i < len(s) {
			r, w := utf8.DecodeRuneInString(s[i:])
			if !unicode.Is(unicode.White_Space, r) {
				return
			}
			i += w
		}
	}
	digits := func() (string, bool) {
		j := i
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == j {
			return "", false
		}
		n, err := strconv.ParseUint(s[j:i], 10, 32)
		if err != nil {
			return "", false
		}
		return strconv.FormatUint(n, 10), true
	}
	bad := `a { that does not start a counted repetition {n}, {n,} or {n,m} (escape it as \{)`
	blanks()
	lo, ok := digits()
	if !ok {
		return refuseRg(bad)
	}
	blanks()
	out := "{" + lo
	if i < len(s) && s[i] == ',' {
		i++
		out += ","
		blanks()
		if hi, ok := digits(); ok {
			out += hi
			blanks()
		}
	}
	if i >= len(s) || s[i] != '}' {
		return refuseRg(bad)
	}
	return out + "}", i + 1, nil
}

// rgEscape reads one escape (s starts with a backslash), outside or
// inside a class.
func rgEscape(s string, inClass bool) (string, int, error) {
	if len(s) < 2 {
		return refuseRg("a trailing backslash")
	}
	c := s[1]
	switch c {
	case 'd':
		return `\p{Nd}`, 2, nil
	case 'D':
		return `\P{Nd}`, 2, nil
	case 'w', 'W', 's', 'S':
		set := rgWordSet()
		if c == 's' || c == 'S' {
			set = rgSpaceSet()
		}
		negate := c == 'W' || c == 'S'
		if inClass {
			if negate {
				set = complementSet(set)
			}
			return setText(set), 2, nil
		}
		if negate {
			return "[^" + setText(set) + "]", 2, nil
		}
		return "[" + setText(set) + "]", 2, nil
	case 'u', 'U', 'x':
		t, n, err := rgHexEscape(s)
		if err == nil && t == `\n` && !inClass {
			return refuseRg(`the literal "\n" is not allowed in a regex`)
		}
		return t, n, err
	case 'p', 'P':
		if len(s) > 3 && s[2] == '{' && s[3] == '^' {
			return refuseRg(`\p{^...} is not rg's syntax (write \P{...})`)
		}
		if len(s) > 2 && s[2] == '{' {
			end := strings.IndexByte(s, '}')
			if end < 0 {
				return refuseRg("an unclosed Unicode class")
			}
			return s[:end+1], end + 1, nil
		}
		if len(s) > 2 {
			_, w := utf8.DecodeRuneInString(s[2:])
			return s[:2+w], 2 + w, nil
		}
		return refuseRg("an unclosed Unicode class")
	case 'Q', 'E':
		return refuseRg(`\Q...\E is not rg's syntax`)
	case '<', '>':
		return refuseRg(`\< and \> (word start and end) are not supported`)
	case 'n':
		if inClass {
			return `\n`, 2, nil
		}
		return refuseRg(`the literal "\n" is not allowed in a regex`)
	case 'b':
		if !inClass && len(s) > 2 && s[2] == '{' {
			return refuseRg(`\b{...} word boundaries are not supported`)
		}
	}
	if c >= '0' && c <= '9' {
		return refuseRg("backreferences and octal escapes are not supported")
	}
	return s[:2], 2, nil
}

// rgHexEscape reads \xHH, \x{H..}, \uHHHH, \u{H..}, \UHHHHHHHH and
// \U{H..} into RE2's \x{H..}.
func rgHexEscape(s string) (string, int, error) {
	width := map[byte]int{'x': 2, 'u': 4, 'U': 8}[s[1]]
	var hex string
	var n int
	if len(s) > 2 && s[2] == '{' {
		end := strings.IndexByte(s, '}')
		if end < 0 {
			return refuseRg("an unclosed hexadecimal escape")
		}
		hex, n = s[3:end], end+1
	} else {
		if len(s) < 2+width {
			return refuseRg("a short hexadecimal escape")
		}
		hex, n = s[2:2+width], 2+width
	}
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil || hex == "" || strings.ContainsAny(hex, "+-") || v > unicode.MaxRune || (v >= 0xd800 && v <= 0xdfff) {
		return refuseRg("an invalid hexadecimal escape")
	}
	if v == '\n' {
		return `\n`, n, nil // the caller decides: refused outside a class
	}
	return `\x{` + strconv.FormatUint(v, 16) + `}`, n, nil
}

// rgClass reads a bracket class (s starts with [). rg's nested classes
// and set operations (&&, --, ~~) have no RE2 form and are refused, as
// is a range with a class at one end. A class of nothing but \n is
// refused as rg refused it.
func rgClass(s string) (string, int, error) {
	var b strings.Builder
	b.WriteByte('[')
	i := 1
	if i < len(s) && s[i] == '^' {
		b.WriteByte('^')
		i++
	}
	start := i
	onlyNL := true
	afterClass := false // the last item was a class escape
	dashRange := false  // the last item was a - after an item
	for {
		if i >= len(s) {
			return refuseRg("an unclosed character class")
		}
		c := s[i]
		switch {
		case c == ']' && i > start:
			if onlyNL {
				return refuseRg(`the literal "\n" is not allowed in a regex`)
			}
			b.WriteByte(']')
			return b.String(), i + 1, nil
		case c == '\n':
			return refuseRg(`the literal "\n" is not allowed in a regex`)
		case c == '[':
			if strings.HasPrefix(s[i:], "[:") {
				if end := strings.Index(s[i:], ":]"); end > 0 {
					b.WriteString(s[i : i+end+2])
					i += end + 2
					onlyNL, afterClass, dashRange = false, true, false
					continue
				}
			}
			return refuseRg("nested character classes are not supported")
		case (c == '&' || c == '~' || c == '-') && i+1 < len(s) && s[i+1] == c:
			return refuseRg("character class set operations (&&, --, ~~) are not supported")
		case c == '-' && i > start && i+1 < len(s) && s[i+1] != ']':
			if afterClass {
				return refuseRg("a range with a class at one end")
			}
			b.WriteByte('-')
			i++
			dashRange, afterClass = true, false
			continue
		case c == '\\':
			t, n, err := rgEscape(s[i:], true)
			if err != nil {
				return "", 0, err
			}
			isClass := strings.ContainsRune("dDwWsSpP", rune(s[i+1]))
			if isClass && dashRange {
				return refuseRg("a range with a class at one end")
			}
			if t != `\n` {
				onlyNL = false
			}
			b.WriteString(t)
			i += n
			afterClass, dashRange = isClass, false
			continue
		}
		_, w := utf8.DecodeRuneInString(s[i:])
		b.WriteString(s[i : i+w])
		i += w
		onlyNL, afterClass, dashRange = false, false, false
	}
}

// rgSet is a sorted list of disjoint rune ranges.
type rgSet [][2]rune

var (
	wordOnce, spaceOnce sync.Once
	wordSet, spaceSet   rgSet
)

// rgWordTables is rg's (Rust regex's) Unicode word character, \w and the
// side test of \b: Alphabetic, marks, decimal digits, connector
// punctuation, join controls.
var rgWordTables = []*unicode.RangeTable{unicode.L, unicode.Nl, unicode.Other_Alphabetic,
	unicode.M, unicode.Nd, unicode.Pc, unicode.Join_Control}

func rgWordSet() rgSet {
	wordOnce.Do(func() { wordSet = tablesSet(rgWordTables) })
	return wordSet
}

// rgSpaceSet is rg's \s: the White_Space property.
func rgSpaceSet() rgSet {
	spaceOnce.Do(func() { spaceSet = tablesSet([]*unicode.RangeTable{unicode.White_Space}) })
	return spaceSet
}

func tablesSet(tables []*unicode.RangeTable) rgSet {
	var rs rgSet
	for _, t := range tables {
		for _, r := range t.R16 {
			for lo := rune(r.Lo); lo <= rune(r.Hi); lo += rune(r.Stride) {
				if r.Stride == 1 {
					rs = append(rs, [2]rune{rune(r.Lo), rune(r.Hi)})
					break
				}
				rs = append(rs, [2]rune{lo, lo})
			}
		}
		for _, r := range t.R32 {
			for lo := rune(r.Lo); lo <= rune(r.Hi); lo += rune(r.Stride) {
				if r.Stride == 1 {
					rs = append(rs, [2]rune{rune(r.Lo), rune(r.Hi)})
					break
				}
				rs = append(rs, [2]rune{lo, lo})
			}
		}
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i][0] < rs[j][0] })
	var out rgSet
	for _, r := range rs {
		if n := len(out); n > 0 && r[0] <= out[n-1][1]+1 {
			out[n-1][1] = max(out[n-1][1], r[1])
			continue
		}
		out = append(out, r)
	}
	return out
}

func complementSet(s rgSet) rgSet {
	var out rgSet
	next := rune(0)
	for _, r := range s {
		if r[0] > next {
			out = append(out, [2]rune{next, r[0] - 1})
		}
		next = r[1] + 1
	}
	if next <= unicode.MaxRune {
		out = append(out, [2]rune{next, unicode.MaxRune})
	}
	return out
}

// setText is the ranges as the inside of an RE2 class.
func setText(s rgSet) string {
	var b strings.Builder
	for _, r := range s {
		b.WriteString(`\x{` + strconv.FormatInt(int64(r[0]), 16) + `}`)
		if r[1] != r[0] {
			b.WriteString(`-\x{` + strconv.FormatInt(int64(r[1]), 16) + `}`)
		}
	}
	return b.String()
}

// The runes the NFA sees besides real ones: none (the start or end of
// the line) and an invalid byte.
const (
	rgNone    rune = -1
	rgInvalid rune = -2
)

// rgIsWord is rg's word character; an invalid byte is not one.
func rgIsWord(r rune) bool {
	if r < 0 {
		return false
	}
	if r < utf8.RuneSelf {
		return r == '_' || ('0' <= r && r <= '9') || ('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z')
	}
	return unicode.IsOneOf(rgWordTables, r)
}

// rgEmpty is what holds between two runes at the start or end of a line
// or inside it. As in rg's regex-automata, \b treats an invalid byte as a
// non-word character, and \B never holds next to one.
func rgEmpty(before, after rune, start, end bool) syntax.EmptyOp {
	var op syntax.EmptyOp
	if start {
		op |= syntax.EmptyBeginText | syntax.EmptyBeginLine
	}
	if end {
		op |= syntax.EmptyEndText | syntax.EmptyEndLine
	}
	if rgIsWord(before) != rgIsWord(after) {
		op |= syntax.EmptyWordBoundary
	} else if before != rgInvalid && after != rgInvalid {
		op |= syntax.EmptyNoWordBoundary
	}
	return op
}

// decodeAt is the rune at i: rgInvalid for an invalid byte, rgNone (w 0)
// at the end.
func decodeAt(line []byte, i int) (rune, int) {
	if i >= len(line) {
		return rgNone, 0
	}
	r, w := utf8.DecodeRune(line[i:])
	if r == utf8.RuneError && w == 1 {
		return rgInvalid, 1
	}
	return r, w
}

// rgThreads is a sparse set of program counters.
type rgThreads struct {
	dense  []uint32
	sparse []uint32
}

func newThreads(n int) *rgThreads {
	return &rgThreads{dense: make([]uint32, 0, n), sparse: make([]uint32, n)}
}

func (q *rgThreads) has(pc uint32) bool {
	i := q.sparse[pc]
	return int(i) < len(q.dense) && q.dense[i] == pc
}

func (q *rgThreads) insert(pc uint32) {
	q.sparse[pc] = uint32(len(q.dense))
	q.dense = append(q.dense, pc)
}

// nfa is an unanchored search of the line on the compiled program, a
// match anywhere being enough: invalid bytes match no rune instruction,
// word boundaries are rg's.
func (r *rgRegex) nfa(line []byte) bool {
	p := r.prog
	cur, next := newThreads(len(p.Inst)), newThreads(len(p.Inst))
	after, w := decodeAt(line, 0)
	if r.add(cur, uint32(p.Start), rgEmpty(rgNone, after, true, len(line) == 0)) {
		return true
	}
	for pos := 0; pos < len(line); {
		npos := pos + w
		nafter, nw := decodeAt(line, npos)
		flag := rgEmpty(after, nafter, false, npos == len(line))
		next.dense = next.dense[:0]
		for _, pc := range cur.dense {
			inst := &p.Inst[pc]
			switch inst.Op {
			case syntax.InstRune, syntax.InstRune1, syntax.InstRuneAny, syntax.InstRuneAnyNotNL:
				if after >= 0 && inst.MatchRune(after) && r.add(next, inst.Out, flag) {
					return true
				}
			}
		}
		if r.add(next, uint32(p.Start), flag) {
			return true
		}
		cur, next = next, cur
		pos, after, w = npos, nafter, nw
	}
	return false
}

// add puts pc and what it reaches without reading a rune into q, and
// reports a match.
func (r *rgRegex) add(q *rgThreads, pc uint32, flag syntax.EmptyOp) bool {
	if q.has(pc) {
		return false
	}
	q.insert(pc)
	inst := &r.prog.Inst[pc]
	switch inst.Op {
	case syntax.InstMatch:
		return true
	case syntax.InstAlt, syntax.InstAltMatch:
		return r.add(q, inst.Out, flag) || r.add(q, inst.Arg, flag)
	case syntax.InstCapture, syntax.InstNop:
		return r.add(q, inst.Out, flag)
	case syntax.InstEmptyWidth:
		if syntax.EmptyOp(inst.Arg)&^flag == 0 {
			return r.add(q, inst.Out, flag)
		}
	}
	return false
}
