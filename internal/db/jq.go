// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/contract"
)

// jv is a JSON value as jq 1.7 holds it. 2.x passed every database result
// through jq -c (the envelope), so the bytes rc.A prints are jq's: numbers
// as jq prints a literal it read (decNumber), strings in jq's escaping,
// objects in insertion order with a repeated key keeping its first place
// and its last value.
type jv struct {
	kind byte // 'z' null, 't' true, 'f' false, 'n' number, 's' string, 'a' array, 'o' object
	num  string
	// lit is a number as the 3.4 row formats write it: the literal as
	// read, or for a SQLite REAL its shortest form (rows.go).
	lit  string
	str  string
	arr  []jv
	keys []string
	vals []jv
}

var jvNull = jv{kind: 'z'}

func jvString(s string) jv { return jv{kind: 's', str: s} }

// jvNumber is a number from its literal text (a JSON number, or the text a
// client printed for one).
func jvNumber(lit string) jv { return jv{kind: 'n', num: decCanonical(lit), lit: lit} }

func jvArray(items []jv) jv { return jv{kind: 'a', arr: items} }

// object is a jq object under construction.
type object struct {
	keys []string
	vals []jv
	at   map[string]int
}

// set is jq's object assignment: an existing key keeps its place.
func (o *object) set(k string, v jv) {
	if o.at == nil {
		o.at = map[string]int{}
	}
	if i, ok := o.at[k]; ok {
		o.vals[i] = v
		return
	}
	o.at[k] = len(o.keys)
	o.keys = append(o.keys, k)
	o.vals = append(o.vals, v)
}

func (o *object) value() jv { return jv{kind: 'o', keys: o.keys, vals: o.vals} }

// get is .key (null when absent).
func (v jv) get(k string) jv {
	if v.kind != 'o' {
		return jvNull
	}
	for i, key := range v.keys {
		if key == k {
			return v.vals[i]
		}
	}
	return jvNull
}

// truthy is jq's truth: everything but null and false.
func (v jv) truthy() bool { return v.kind != 'z' && v.kind != 'f' }

// length is jq's length for the values a database result can be.
func (v jv) length() int {
	switch v.kind {
	case 'a':
		return len(v.arr)
	case 'o':
		return len(v.keys)
	case 's':
		return utf8.RuneCountInString(v.str)
	}
	return 0
}

// appendJSON appends v as jq -c prints it.
func (v jv) appendJSON(dst []byte) []byte {
	switch v.kind {
	case 'z':
		return append(dst, "null"...)
	case 't':
		return append(dst, "true"...)
	case 'f':
		return append(dst, "false"...)
	case 'n':
		return append(dst, v.num...)
	case 's':
		return contract.AppendString(dst, v.str, contract.EscapeJQ)
	case 'a':
		dst = append(dst, '[')
		for i, e := range v.arr {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = e.appendJSON(dst)
		}
		return append(dst, ']')
	}
	dst = append(dst, '{')
	for i, k := range v.keys {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = contract.AppendString(dst, k, contract.EscapeJQ)
		dst = append(dst, ':')
		dst = v.vals[i].appendJSON(dst)
	}
	return append(dst, '}')
}

func (v jv) String() string { return string(v.appendJSON(nil)) }

// decCanonical is the text jq 1.7 prints for a number literal it read:
// decNumber's to-scientific-string of the literal (coefficient without
// leading zeros, trailing zeros kept; exponential form when the exponent
// is positive or the adjusted exponent is below -6).
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
		if exp == 0 {
			b.WriteString(coef)
		} else if len(coef) > -exp {
			b.WriteString(coef[:len(coef)+exp])
			b.WriteByte('.')
			b.WriteString(coef[len(coef)+exp:])
		} else {
			b.WriteString("0.")
			b.WriteString(strings.Repeat("0", -exp-len(coef)))
			b.WriteString(coef)
		}
		return b.String()
	}
	b.WriteByte(coef[0])
	if len(coef) > 1 {
		b.WriteByte('.')
		b.WriteString(coef[1:])
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

var errJSON = errors.New("invalid JSON")

// parseJSON reads the whitespace-separated JSON values of text, as jq
// reads its input.
func parseJSON(text string) ([]jv, error) {
	p := &jsonParser{s: text}
	var out []jv
	for {
		p.ws()
		if p.i >= len(p.s) {
			return out, nil
		}
		v, err := p.value(0)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

type jsonParser struct {
	s string
	i int
}

func (p *jsonParser) ws() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *jsonParser) value(depth int) (jv, error) {
	if depth > 10000 {
		return jv{}, errJSON
	}
	p.ws()
	if p.i >= len(p.s) {
		return jv{}, errJSON
	}
	switch c := p.s[p.i]; {
	case c == '{':
		p.i++
		var o object
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == '}' {
			p.i++
			return jv{kind: 'o'}, nil
		}
		for {
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != '"' {
				return jv{}, errJSON
			}
			k, err := p.str()
			if err != nil {
				return jv{}, err
			}
			p.ws()
			if p.i >= len(p.s) || p.s[p.i] != ':' {
				return jv{}, errJSON
			}
			p.i++
			v, err := p.value(depth + 1)
			if err != nil {
				return jv{}, err
			}
			o.set(k, v)
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == '}' {
				p.i++
				return o.value(), nil
			}
			return jv{}, errJSON
		}
	case c == '[':
		p.i++
		var items []jv
		p.ws()
		if p.i < len(p.s) && p.s[p.i] == ']' {
			p.i++
			return jvArray(nil), nil
		}
		for {
			v, err := p.value(depth + 1)
			if err != nil {
				return jv{}, err
			}
			items = append(items, v)
			p.ws()
			if p.i < len(p.s) && p.s[p.i] == ',' {
				p.i++
				continue
			}
			if p.i < len(p.s) && p.s[p.i] == ']' {
				p.i++
				return jvArray(items), nil
			}
			return jv{}, errJSON
		}
	case c == '"':
		s, err := p.str()
		return jvString(s), err
	case c == '-' || (c >= '0' && c <= '9'):
		start := p.i
		p.i++
		for p.i < len(p.s) && strings.IndexByte("0123456789.eE+-", p.s[p.i]) >= 0 {
			p.i++
		}
		lit := p.s[start:p.i]
		if _, err := strconv.ParseFloat(lit, 64); err != nil && !errors.Is(err, strconv.ErrRange) {
			return jv{}, errJSON
		}
		return jvNumber(lit), nil
	}
	for _, w := range []struct {
		lit  string
		kind byte
	}{{"null", 'z'}, {"true", 't'}, {"false", 'f'}} {
		if strings.HasPrefix(p.s[p.i:], w.lit) {
			p.i += len(w.lit)
			return jv{kind: w.kind}, nil
		}
	}
	return jv{}, errJSON
}

// str reads a JSON string. Invalid UTF-8 is kept as it is (the jq encoder
// replaces it on output); a lone surrogate escape becomes U+FFFD, as jq.
func (p *jsonParser) str() (string, error) {
	p.i++ // opening quote
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '"':
			p.i++
			return b.String(), nil
		case c != '\\':
			b.WriteByte(c)
			p.i++
			continue
		}
		if p.i+1 >= len(p.s) {
			return "", errJSON
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
				return "", errJSON
			}
			if r >= 0xd800 && r < 0xdc00 && strings.HasPrefix(p.s[p.i:], `\u`) {
				save := p.i
				p.i += 2
				if lo, ok := p.hex4(); ok && lo >= 0xdc00 && lo < 0xe000 {
					r = 0x10000 + (r-0xd800)<<10 + (lo - 0xdc00)
				} else {
					p.i = save
				}
			}
			if r >= 0xd800 && r < 0xe000 {
				r = utf8.RuneError
			}
			b.WriteRune(r)
		default:
			return "", errJSON
		}
	}
	return "", errJSON
}

func (p *jsonParser) hex4() (rune, bool) {
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
