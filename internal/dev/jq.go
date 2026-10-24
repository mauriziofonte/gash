// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/contract"
)

// The two jq programs of 2.x, `jq -r '.require.php // empty' composer.json`
// and `jq -r '.engines.node // empty' package.json`, evaluated natively as
// jq 1.7 ran them (LEDGER DEV-10): the file is a stream of JSON values,
// each one run through the path; a value whose path fails (a string
// indexed) is skipped; a parse error ends the stream after the values
// before it; numbers print as the literal jq read (decNumber text), other
// non-string values as jq's indented JSON.

// jv is a JSON value as jq holds it.
type jv struct {
	kind byte // 'z' null, 't' true, 'f' false, 'n' number, 's' string, 'a' array, 'o' object
	num  string
	str  string
	arr  []jv
	keys []string
	vals []jv
}

// jqPathRaw is `jq -r '.K1.K2 // empty' FILE` on data: what jq printed,
// one line per output (the trailing newline of the last one included).
func jqPathRaw(data []byte, keys ...string) string {
	p := &jqParser{s: string(data)}
	p.s = strings.TrimPrefix(p.s, "\ufeff")
	var b strings.Builder
	for {
		p.ws()
		if p.i >= len(p.s) {
			break
		}
		v, ok := p.value(0)
		if !ok {
			break
		}
		failed := false
		for _, k := range keys {
			switch v.kind {
			case 'z':
				// null.key is null, no error (jq).
			case 'o':
				v = v.get(k)
			default:
				// Indexing a string, number, bool or array is a jq
				// runtime error, which stops the whole stream (the `//
				// empty` alternative does not catch it); the values
				// already printed stand, 2>/dev/null drops the message.
				failed = true
			}
			if failed {
				break
			}
		}
		if failed {
			break
		}
		if v.kind == 'z' || v.kind == 'f' {
			continue
		}
		if v.kind == 's' {
			b.WriteString(v.str)
		} else {
			v.pretty(&b, "")
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// get is .k on an object (null when absent).
func (v jv) get(k string) jv {
	for i, key := range v.keys {
		if key == k {
			return v.vals[i]
		}
	}
	return jv{kind: 'z'}
}

// pretty writes v as jq prints it by default (indent 2).
func (v jv) pretty(b *strings.Builder, indent string) {
	switch v.kind {
	case 'z':
		b.WriteString("null")
	case 't':
		b.WriteString("true")
	case 'f':
		b.WriteString("false")
	case 'n':
		b.WriteString(v.num)
	case 's':
		b.Write(contract.AppendString(nil, v.str, contract.EscapeJQ))
	case 'a':
		if len(v.arr) == 0 {
			b.WriteString("[]")
			return
		}
		b.WriteString("[\n")
		for i, e := range v.arr {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(indent + "  ")
			e.pretty(b, indent+"  ")
		}
		b.WriteString("\n" + indent + "]")
	default:
		if len(v.keys) == 0 {
			b.WriteString("{}")
			return
		}
		b.WriteString("{\n")
		for i, k := range v.keys {
			if i > 0 {
				b.WriteString(",\n")
			}
			b.WriteString(indent + "  ")
			b.Write(contract.AppendString(nil, k, contract.EscapeJQ))
			b.WriteString(": ")
			v.vals[i].pretty(b, indent+"  ")
		}
		b.WriteString("\n" + indent + "}")
	}
}

// jqParser reads JSON as jq 1.7's parser does: literals are runs of
// non-structural bytes checked as true, false, null or a decNumber
// literal (nan, infinity, 1., .5, +1, 01 included); strings take raw
// control bytes; a high surrogate escape must be followed by a low one;
// a repeated key keeps its first place and its last value.
type jqParser struct {
	s string
	i int
}

func (p *jqParser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jqParser) value(depth int) (jv, bool) {
	if depth > 10000 {
		return jv{}, false
	}
	p.ws()
	if p.i >= len(p.s) {
		return jv{}, false
	}
	switch p.s[p.i] {
	case '{':
		p.i++
		o := jv{kind: 'o'}
		at := map[string]int{}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return o, true
		}
		for {
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return jv{}, false
			}
			k, ok := p.str()
			if !ok {
				return jv{}, false
			}
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return jv{}, false
			}
			p.i++
			v, ok := p.value(depth + 1)
			if !ok {
				return jv{}, false
			}
			if j, dup := at[k]; dup {
				o.vals[j] = v
			} else {
				at[k] = len(o.keys)
				o.keys = append(o.keys, k)
				o.vals = append(o.vals, v)
			}
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return o, true
			}
			return jv{}, false
		}
	case '[':
		p.i++
		a := jv{kind: 'a'}
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return a, true
		}
		for {
			v, ok := p.value(depth + 1)
			if !ok {
				return jv{}, false
			}
			a.arr = append(a.arr, v)
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return a, true
			}
			return jv{}, false
		}
	case '"':
		s, ok := p.str()
		return jv{kind: 's', str: s}, ok
	case '}', ']', ',', ':':
		return jv{}, false
	}
	start := p.i
	for p.i < len(p.s) && !strings.ContainsRune(" \t\n\r{}[],:\"", rune(p.s[p.i])) {
		p.i++
	}
	return literal(p.s[start:p.i])
}

// literal is a jq literal token.
func literal(tok string) (jv, bool) {
	switch tok {
	case "true":
		return jv{kind: 't'}, true
	case "false":
		return jv{kind: 'f'}, true
	case "null":
		return jv{kind: 'z'}, true
	}
	body := strings.TrimLeft(tok, "+-")
	if len(tok)-len(body) > 1 {
		return jv{}, false
	}
	switch l := strings.ToLower(body); {
	case l == "inf" || l == "infinity":
		if strings.HasPrefix(tok, "-") {
			return jv{kind: 'n', num: "-1.7976931348623157e+308"}, true
		}
		return jv{kind: 'n', num: "1.7976931348623157e+308"}, true
	case strings.HasPrefix(l, "nan") && allDigits(l[3:]), strings.HasPrefix(l, "snan") && allDigits(l[4:]):
		return jv{kind: 'n', num: "null"}, true
	}
	if !decLiteral(body) {
		return jv{}, false
	}
	return jv{kind: 'n', num: decCanonical(tok)}, true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return false
		}
	}
	return true
}

// decLiteral is decNumber's syntax of a finite number without its sign:
// digits with an optional point (at least one digit), then an optional
// exponent.
func decLiteral(s string) bool {
	i, digits := 0, 0
	for i < len(s) && isDigit(s[i]) {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isDigit(s[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return false
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i == len(s) || !allDigits(s[i:]) {
			return false
		}
		return true
	}
	return i == len(s)
}

// decCanonical is the text jq 1.7 prints for a number literal it read:
// decNumber's to-scientific-string (coefficient without leading zeros,
// trailing zeros kept; exponential form when the exponent is positive or
// the adjusted exponent is below -6).
func decCanonical(lit string) string {
	s := lit
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	mant, expPart := s, ""
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		mant, expPart = s[:i], s[i+1:]
	}
	intPart, frac := mant, ""
	if i := strings.IndexByte(mant, '.'); i >= 0 {
		intPart, frac = mant[:i], mant[i+1:]
	}
	exp := 0
	if expPart != "" {
		e, err := strconv.Atoi(strings.TrimPrefix(expPart, "+"))
		if err != nil {
			return lit
		}
		exp = e
	}
	coef := strings.TrimLeft(intPart+frac, "0")
	if coef == "" {
		coef = "0"
	}
	exp -= len(frac)
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	adj := exp + len(coef) - 1
	if exp <= 0 && adj >= -6 {
		switch {
		case exp == 0:
			b.WriteString(coef)
		case len(coef) > -exp:
			b.WriteString(coef[:len(coef)+exp] + "." + coef[len(coef)+exp:])
		default:
			b.WriteString("0." + strings.Repeat("0", -exp-len(coef)) + coef)
		}
		return b.String()
	}
	b.WriteByte(coef[0])
	if len(coef) > 1 {
		b.WriteString("." + coef[1:])
	}
	b.WriteByte('E')
	if adj < 0 {
		b.WriteByte('-')
		adj = -adj
	} else {
		b.WriteByte('+')
	}
	b.WriteString(strconv.Itoa(adj))
	return b.String()
}

// str reads a JSON string as jq does.
func (p *jqParser) str() (string, bool) {
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '"' {
			p.i++
			return b.String(), true
		}
		if c != '\\' {
			b.WriteByte(c)
			p.i++
			continue
		}
		if p.i+1 >= len(p.s) {
			return "", false
		}
		e := p.s[p.i+1]
		p.i += 2
		switch e {
		case '"', '\\', '/':
			b.WriteByte(e)
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'u':
			r, ok := p.hex4()
			if !ok {
				return "", false
			}
			switch {
			case r >= 0xd800 && r < 0xdc00:
				if !strings.HasPrefix(p.s[p.i:], `\u`) {
					return "", false
				}
				p.i += 2
				lo, ok := p.hex4()
				if !ok || lo < 0xdc00 || lo >= 0xe000 {
					return "", false
				}
				r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
			case r >= 0xdc00 && r < 0xe000:
				r = utf8.RuneError
			}
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	return "", false
}

func (p *jqParser) hex4() (rune, bool) {
	if p.i+4 > len(p.s) {
		return 0, false
	}
	n, err := strconv.ParseUint(p.s[p.i:p.i+4], 16, 32)
	if err != nil {
		return 0, false
	}
	p.i += 4
	return rune(n), true
}
