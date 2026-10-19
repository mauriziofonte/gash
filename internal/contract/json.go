// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package contract

import (
	"encoding/json/jsontext"
	"fmt"
	"strconv"
	"strings"
)

// Escape selects how strings are written, matching the 2.x path that
// produced the bytes.
type Escape int

const (
	// EscapeJQ writes strings as jq 1.7 prints them: `"` `\` and
	// \b \f \n \r \t escaped, the other controls and DEL as \u00xx
	// (lowercase hex), invalid UTF-8 replaced by U+FFFD the way jq's
	// decoder replaces it (one U+FFFD per bad sequence, see jqNext),
	// everything else (non-ASCII, U+2028, U+2029, < > &) raw.
	EscapeJQ Escape = iota
	// EscapeBash writes strings as 2.x __gash_json_escape did: `"` `\`
	// and \b \f \n \r \t escaped, the other controls U+0001..U+001F as
	// \u00xx, DEL and every byte >= 0x80 (valid or not) raw. NUL, which
	// a Bash string cannot hold, is written as \u0000.
	EscapeBash
)

const hexDigits = "0123456789abcdef"

// AppendString appends s as a quoted JSON string.
func AppendString(dst []byte, s string, mode Escape) []byte {
	dst = append(dst, '"')
	for i := 0; i < len(s); {
		c := s[i]
		if c >= 0x80 {
			if mode == EscapeBash {
				dst = append(dst, c)
				i++
				continue
			}
			size, valid := jqNext(s[i:])
			if !valid {
				dst = append(dst, "\uFFFD"...)
			} else {
				dst = append(dst, s[i:i+size]...)
			}
			i += size
			continue
		}
		switch c {
		case '"':
			dst = append(dst, '\\', '"')
		case '\\':
			dst = append(dst, '\\', '\\')
		case '\n':
			dst = append(dst, '\\', 'n')
		case '\r':
			dst = append(dst, '\\', 'r')
		case '\t':
			dst = append(dst, '\\', 't')
		case '\b':
			dst = append(dst, '\\', 'b')
		case '\f':
			dst = append(dst, '\\', 'f')
		default:
			if c < 0x20 || (c == 0x7f && mode == EscapeJQ) {
				dst = append(dst, '\\', 'u', '0', '0', hexDigits[c>>4], hexDigits[c&0xf])
			} else {
				dst = append(dst, c)
			}
		}
		i++
	}
	return append(dst, '"')
}

// utf8Length is jq's utf8_coding_length table: the length of the sequence
// a first byte starts, 0 for a byte that cannot start one, -1 for a
// continuation byte.
func utf8Length(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b < 0xC0:
		return -1
	case b < 0xC2:
		return 0
	case b < 0xE0:
		return 2
	case b < 0xF0:
		return 3
	case b < 0xF5:
		return 4
	}
	return 0
}

// jqNext is jq's jvp_utf8_next on a string starting with a byte >= 0x80:
// the number of bytes the next character takes and whether it is valid.
// An invalid sequence is replaced by one U+FFFD: a stray byte alone, a
// sequence cut by a byte that is not a continuation up to that byte, a
// sequence cut by the end of the string together with everything left
// (whatever those bytes are), an overlong, surrogate or out-of-range
// sequence whole.
func jqNext(s string) (int, bool) {
	length := utf8Length(s[0])
	if length <= 0 {
		return 1, false
	}
	if length > len(s) {
		return len(s), false
	}
	cp := rune(s[0]) & []rune{0, 0x7f, 0x1f, 0x0f, 0x07}[length]
	for i := 1; i < length; i++ {
		if utf8Length(s[i]) != -1 {
			return i, false
		}
		cp = cp<<6 | rune(s[i])&0x3f
	}
	if cp < []rune{0, 0, 0x80, 0x800, 0x10000}[length] || (cp >= 0xD800 && cp <= 0xDFFF) || cp > 0x10FFFF {
		return length, false
	}
	return length, true
}

// Quote returns s as a quoted JSON string.
func Quote(s string, mode Escape) string {
	return string(AppendString(nil, s, mode))
}

// kind of an object member value.
type kind int

const (
	kindString kind = iota
	kindBool
	kindInt
	kindRaw
)

type member struct {
	key  string
	kind kind
	str  string
	b    bool
	n    int64
	raw  []byte
}

// Object is a JSON object with members in insertion order, written
// compact (no spaces), the layout of jq -c and of the 2.x printf
// envelopes.
type Object struct {
	members []member
}

// String adds a string member.
func (o *Object) String(key, value string) *Object {
	o.members = append(o.members, member{key: key, kind: kindString, str: value})
	return o
}

// Bool adds a boolean member.
func (o *Object) Bool(key string, value bool) *Object {
	o.members = append(o.members, member{key: key, kind: kindBool, b: value})
	return o
}

// Int adds an integer member.
func (o *Object) Int(key string, value int64) *Object {
	o.members = append(o.members, member{key: key, kind: kindInt, n: value})
	return o
}

// Raw adds a member whose value is already JSON (a number as the source
// wrote it, a nested object). The value must be valid JSON; it is written
// compact.
func (o *Object) Raw(key string, value jsontext.Value) *Object {
	o.members = append(o.members, member{key: key, kind: kindRaw, raw: value})
	return o
}

// Len returns the number of members.
func (o *Object) Len() int { return len(o.members) }

// Append appends the object to dst.
func (o *Object) Append(dst []byte, mode Escape) ([]byte, error) {
	dst = append(dst, '{')
	for i, m := range o.members {
		if i > 0 {
			dst = append(dst, ',')
		}
		dst = AppendString(dst, m.key, mode)
		dst = append(dst, ':')
		switch m.kind {
		case kindString:
			dst = AppendString(dst, m.str, mode)
		case kindBool:
			dst = strconv.AppendBool(dst, m.b)
		case kindInt:
			dst = strconv.AppendInt(dst, m.n, 10)
		case kindRaw:
			v := jsontext.Value(append([]byte(nil), m.raw...))
			if err := v.Compact(); err != nil {
				return dst, fmt.Errorf("contract: member %q: %w", m.key, err)
			}
			dst = append(dst, v...)
		}
	}
	return append(dst, '}'), nil
}

// Bytes returns the object as compact JSON.
func (o *Object) Bytes(mode Escape) ([]byte, error) {
	return o.Append(nil, mode)
}

// JQString is what __gash_json_str VALUE prints: jq -cn --arg s VALUE '$s',
// the JSON string in jq's escaping and a newline.
func JQString(v string) string { return Quote(v, EscapeJQ) + "\n" }

// JQArray is what __gash_json_arr VALUE... prints: the values as a compact
// jq array of strings and a newline (2.x escaped each in Bash, then jq -c
// normalized the array, so the bytes are jq's), or "[]" without a newline
// when there are none.
func JQArray(vals ...string) string {
	if len(vals) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vals {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(AppendString(nil, v, EscapeJQ))
	}
	b.WriteString("]\n")
	return b.String()
}
