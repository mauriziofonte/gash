// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/jsonpath"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// llm_json (plan D4): a JSON file (or the standard input) read by a
// validating scanner that never builds the whole value: --path walks to
// one value and prints its bytes compacted (numbers and escapes as
// written), --keys lists keys, --shape summarizes types and lengths two
// levels deep, --format prints an array of objects as rows. One pass
// validates the whole text first, so a broken file is invalid_json
// whatever part of it was asked for.

// jsonMaxDepth bounds nesting (a deeper text is refused, not a crash).
const jsonMaxDepth = 10000

// jsonShapeKeys caps the keys listed per object in --shape.
const jsonShapeKeys = 100

// jsonError is a syntax error at byte offset at.
type jsonError struct {
	at  int
	msg string
}

// jscan is a validating scanner over a whole JSON text. Its methods
// panic with *jsonError on bad syntax; parse recovers it.
type jscan struct {
	b []byte
	i int
}

func (s *jscan) fail(msg string) {
	panic(&jsonError{at: s.i, msg: msg})
}

// catch turns a scanner panic into an error, for the caller's defer.
func catch(err *error) {
	if r := recover(); r != nil {
		je, ok := r.(*jsonError)
		if !ok {
			panic(r)
		}
		*err = je
	}
}

func (e *jsonError) Error() string { return e.msg }

// where is the 1-based line and column of byte offset at in b.
func where(b []byte, at int) (int, int) {
	at = min(at, len(b))
	line := 1 + bytes.Count(b[:at], []byte{'\n'})
	col := at - bytes.LastIndexByte(b[:at], '\n')
	return line, col
}

func (s *jscan) ws() {
	for s.i < len(s.b) {
		switch s.b[s.i] {
		case ' ', '\t', '\n', '\r':
			s.i++
		default:
			return
		}
	}
}

// kindAt is the kind of the value at s.i (after blanks): o a s n b z.
func (s *jscan) kindAt() byte {
	s.ws()
	if s.i >= len(s.b) {
		s.fail("unexpected end of input, expected a value")
	}
	switch c := s.b[s.i]; {
	case c == '{':
		return 'o'
	case c == '[':
		return 'a'
	case c == '"':
		return 's'
	case c == '-' || (c >= '0' && c <= '9'):
		return 'n'
	case c == 't' || c == 'f':
		return 'b'
	case c == 'n':
		return 'z'
	}
	s.fail("invalid character " + strconv.QuoteRune(rune(s.b[s.i])) + ", expected a value")
	return 0
}

// strStop marks the bytes that end the fast part of a string scan.
var strStop = func() (t [256]bool) {
	for c := 0; c < 0x20; c++ {
		t[c] = true
	}
	t['"'], t['\\'] = true, true
	return t
}()

// str skips the string at s.i and returns its bytes without the quotes
// (escapes kept) and whether it holds an escape.
func (s *jscan) str() ([]byte, bool) {
	s.i++ // the opening quote
	start := s.i
	esc := false
	b := s.b
	for {
		i := s.i
		for i < len(b) && !strStop[b[i]] {
			i++
		}
		s.i = i
		if i >= len(b) {
			s.fail("unterminated string")
		}
		switch c := b[i]; {
		case c == '"':
			s.i++
			return b[start:i], esc
		case c == '\\':
			esc = true
			if i+1 >= len(b) {
				s.fail("unterminated string")
			}
			switch b[i+1] {
			case '"', '\\', '/', 'b', 'f', 'n', 'r', 't':
				s.i = i + 2
			case 'u':
				if i+6 > len(b) || !isHex4(b[i+2:i+6]) {
					s.i = i + 1
					s.fail(`invalid \u escape`)
				}
				s.i = i + 6
			default:
				s.i = i + 1
				s.fail("invalid escape in string")
			}
		default:
			s.fail("control character in string")
		}
	}
}

func isHex4(b []byte) bool {
	for _, c := range b {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

// num skips the number at s.i (JSON grammar).
func (s *jscan) num() {
	b := s.b
	i := s.i
	if b[i] == '-' {
		i++
	}
	switch {
	case i < len(b) && b[i] == '0':
		i++
	case i < len(b) && b[i] >= '1' && b[i] <= '9':
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	default:
		s.i = i
		s.fail("invalid number")
	}
	if i < len(b) && b[i] == '.' {
		i++
		if i >= len(b) || !isDigit(b[i]) {
			s.i = i
			s.fail("invalid number: a digit must follow the decimal point")
		}
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	}
	if i < len(b) && (b[i] == 'e' || b[i] == 'E') {
		i++
		if i < len(b) && (b[i] == '+' || b[i] == '-') {
			i++
		}
		if i >= len(b) || !isDigit(b[i]) {
			s.i = i
			s.fail("invalid number: a digit must follow the exponent")
		}
		for i < len(b) && isDigit(b[i]) {
			i++
		}
	}
	s.i = i
}

// lit skips the literal word w at s.i.
func (s *jscan) lit(w string) {
	if !bytes.HasPrefix(s.b[s.i:], []byte(w)) {
		s.fail("invalid literal, expected " + w)
	}
	s.i += len(w)
}

// skip validates and skips the value at s.i and returns its kind.
func (s *jscan) skip(depth int) byte {
	k := s.kindAt()
	switch k {
	case 'o':
		s.members(depth, func([]byte, bool) { s.skip(depth + 1) })
	case 'a':
		s.elements(depth, func(int) { s.skip(depth + 1) })
	case 's':
		s.str()
	case 'n':
		s.num()
	case 'b':
		if s.b[s.i] == 't' {
			s.lit("true")
		} else {
			s.lit("false")
		}
	default:
		s.lit("null")
	}
	return k
}

// members walks the object at s.i: for each member it calls fn with the
// key (raw, and whether it holds escapes) and s.i at the value, which fn
// must consume. It returns the member count.
func (s *jscan) members(depth int, fn func(key []byte, esc bool)) int {
	if depth > jsonMaxDepth {
		s.fail("nested too deep")
	}
	s.i++ // {
	s.ws()
	if s.i < len(s.b) && s.b[s.i] == '}' {
		s.i++
		return 0
	}
	n := 0
	for {
		s.ws()
		if s.i >= len(s.b) || s.b[s.i] != '"' {
			s.fail("expected a string key")
		}
		key, esc := s.str()
		s.ws()
		if s.i >= len(s.b) || s.b[s.i] != ':' {
			s.fail("expected ':' after an object key")
		}
		s.i++
		s.ws()
		fn(key, esc)
		n++
		s.ws()
		if s.i < len(s.b) && s.b[s.i] == ',' {
			s.i++
			continue
		}
		if s.i < len(s.b) && s.b[s.i] == '}' {
			s.i++
			return n
		}
		s.fail("expected ',' or '}' after an object member")
	}
}

// elements walks the array at s.i, calling fn with each index and s.i at
// the element, which fn must consume. It returns the element count.
func (s *jscan) elements(depth int, fn func(int)) int {
	if depth > jsonMaxDepth {
		s.fail("nested too deep")
	}
	s.i++ // [
	s.ws()
	if s.i < len(s.b) && s.b[s.i] == ']' {
		s.i++
		return 0
	}
	n := 0
	for {
		s.ws()
		fn(n)
		n++
		s.ws()
		if s.i < len(s.b) && s.b[s.i] == ',' {
			s.i++
			continue
		}
		if s.i < len(s.b) && s.b[s.i] == ']' {
			s.i++
			return n
		}
		s.fail("expected ',' or ']' after an array element")
	}
}

// decodeKey is a raw string's text (escapes resolved).
func decodeKey(raw []byte, esc bool) string {
	if !esc {
		return string(raw)
	}
	var out string
	if json.Unmarshal(append(append([]byte{'"'}, raw...), '"'), &out) != nil {
		return string(raw)
	}
	return out
}

// validJSON checks that b is one JSON value (UTF-8, blanks around it) and
// returns the offset of the value.
func validJSON(b []byte) (int, error) {
	return scanJSON(b, func(s *jscan) { s.skip(0) })
}

// scanJSON checks that b is UTF-8 and runs read on the scanner at the
// one value of b (after a byte order mark and blanks), then checks that
// only blanks follow. It returns the offset of the value.
func scanJSON(b []byte, read func(*jscan)) (start int, err error) {
	defer catch(&err)
	if !utf8.Valid(b) {
		at := 0
		for at < len(b) {
			r, n := utf8.DecodeRune(b[at:])
			if r == utf8.RuneError && n <= 1 {
				break
			}
			at += n
		}
		return 0, &jsonError{at: at, msg: "not valid UTF-8"}
	}
	s := &jscan{b: b}
	if bytes.HasPrefix(b, []byte("\xef\xbb\xbf")) {
		s.i = 3
	}
	s.ws()
	start = s.i
	read(s)
	s.ws()
	if s.i < len(b) {
		s.fail("extra data after the value")
	}
	return start, nil
}

// jsonValue is one value of a validated text: its kind and byte range.
type jsonValue struct {
	b          []byte
	start, end int
	kind       byte
}

func (v jsonValue) raw() []byte { return v.b[v.start:v.end] }

// valueAt is the value starting at offset i of a validated text.
func valueAt(b []byte, i int) jsonValue {
	s := &jscan{b: b, i: i}
	s.ws()
	i = s.i
	k := s.skip(0)
	return jsonValue{b: b, start: i, end: s.i, kind: k}
}

// walkPath follows steps from the root value. On a missing step it
// returns false and the number of steps that exist.
func walkPath(root jsonValue, steps []jsonpath.Step) (jsonValue, int, bool) {
	cur := root
	for n, st := range steps {
		s := &jscan{b: cur.b, i: cur.start}
		switch {
		case cur.kind == 'o' && !st.IsIndex:
			found := -1
			s.members(0, func(key []byte, esc bool) {
				if (!esc && string(key) == st.Key) || (esc && decodeKey(key, esc) == st.Key) {
					found = s.i // the last one wins, as in jq and Python
				}
				s.skip(1)
			})
			if found < 0 {
				return cur, n, false
			}
			cur = valueAt(cur.b, found)
		case cur.kind == 'a' && st.IsIndex:
			idx := st.Index
			if idx < 0 {
				idx += s.elements(0, func(int) { s.skip(1) })
				s.i = cur.start
			}
			found := -1
			s.elements(0, func(i int) {
				if i == idx {
					found = s.i
				}
				s.skip(1)
			})
			if found < 0 {
				return cur, n, false
			}
			cur = valueAt(cur.b, found)
		default:
			return cur, n, false
		}
	}
	return cur, len(steps), true
}

// appendCompact appends the validated value raw without blanks; with
// width > 0 each string is cut at width display columns.
func appendCompact(dst, raw []byte, width int) []byte {
	for i := 0; i < len(raw); {
		c := raw[i]
		switch c {
		case ' ', '\t', '\n', '\r':
			i++
		case '"':
			j := i + 1
			for raw[j] != '"' {
				if raw[j] == '\\' {
					j++
				}
				j++
			}
			tok := raw[i : j+1]
			if width > 0 {
				var s string
				if json.Unmarshal(tok, &s) == nil && llmfmt.StringWidth(s) > width {
					dst = append(dst, llmfmt.Quote(llmfmt.Cut(s, width))...)
					i = j + 1
					continue
				}
			}
			dst = append(dst, tok...)
			i = j + 1
		default:
			j := i + 1
			for j < len(raw) && !blankOrQuote(raw[j]) {
				j++
			}
			dst = append(dst, raw[i:j]...)
			i = j
		}
	}
	return dst
}

func blankOrQuote(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '"':
		return true
	}
	return false
}

// kindName is the JSON type name of a kind letter.
func kindName(k byte) string {
	switch k {
	case 'o':
		return "object"
	case 'a':
		return "array"
	case 's':
		return "string"
	case 'n':
		return "number"
	case 'b':
		return "boolean"
	}
	return "null"
}

// cellOf is a value as an llmfmt.Table cell: strings decoded, numbers as
// written, containers as compact JSON.
func cellOf(v jsonValue) any {
	raw := v.raw()
	switch v.kind {
	case 's':
		s := &jscan{b: raw}
		body, esc := s.str()
		return decodeKey(body, esc)
	case 'n':
		return json.Number(raw)
	case 'b':
		return raw[0] == 't'
	case 'z':
		return nil
	}
	return json.RawMessage(appendCompact(nil, raw, 0))
}

// keysOf lists the keys of an object, or of the objects of an array
// (first-seen order, each once); ok is false when there is no object.
func keysOf(v jsonValue) ([]string, bool) {
	var keys []string
	seen := map[string]bool{}
	collect := func(obj jsonValue) {
		s := &jscan{b: obj.b, i: obj.start}
		s.members(0, func(key []byte, esc bool) {
			k := decodeKey(key, esc)
			if !seen[k] {
				seen[k] = true
				keys = append(keys, k)
			}
			s.skip(1)
		})
	}
	switch v.kind {
	case 'o':
		collect(v)
		return keys, true
	case 'a':
		found := false
		s := &jscan{b: v.b, i: v.start}
		s.elements(0, func(int) {
			start := s.i
			if s.skip(1) == 'o' {
				found = true
				collect(jsonValue{b: v.b, start: start, end: s.i, kind: 'o'})
			}
		})
		return keys, found
	}
	return nil, false
}

// elementsOf are the elements of an array value.
func elementsOf(v jsonValue) []jsonValue {
	var out []jsonValue
	s := &jscan{b: v.b, i: v.start}
	s.elements(0, func(int) {
		start := s.i
		k := s.skip(1)
		out = append(out, jsonValue{b: v.b, start: start, end: s.i, kind: k})
	})
	return out
}

// membersOf are the members of an object value (keys decoded).
func membersOf(v jsonValue) ([]string, []jsonValue) {
	var keys []string
	var vals []jsonValue
	s := &jscan{b: v.b, i: v.start}
	s.members(0, func(key []byte, esc bool) {
		start := s.i
		k := s.skip(1)
		keys = append(keys, decodeKey(key, esc))
		vals = append(vals, jsonValue{b: v.b, start: start, end: s.i, kind: k})
	})
	return keys, vals
}

// rowsOf is a value as a table: an array of objects as rows with the
// union of the keys as columns; an array of other values as a "value"
// column (objects among them as compact JSON); an object as key/value
// rows; anything else as one "value" row.
func rowsOf(v jsonValue) llmfmt.Table {
	switch v.kind {
	case 'a':
		elems := elementsOf(v)
		allObjects := len(elems) > 0
		for _, e := range elems {
			if e.kind != 'o' {
				allObjects = false
				break
			}
		}
		if !allObjects {
			rows := make([][]any, len(elems))
			for i, e := range elems {
				rows[i] = []any{cellOf(e)}
			}
			return llmfmt.Table{Cols: []string{"value"}, Rows: rows}
		}
		var cols []string
		at := map[string]int{}
		rows := make([][]any, len(elems))
		for i, e := range elems {
			keys, vals := membersOf(e)
			row := make([]any, len(cols), len(cols)+len(keys))
			for j, k := range keys {
				c, ok := at[k]
				if !ok {
					c = len(cols)
					at[k] = c
					cols = append(cols, k)
				}
				for len(row) <= c {
					row = append(row, nil)
				}
				row[c] = cellOf(vals[j])
			}
			rows[i] = row
		}
		for i, row := range rows {
			for len(row) < len(cols) {
				row = append(row, nil)
			}
			rows[i] = row
		}
		return llmfmt.Table{Cols: cols, Rows: rows}
	case 'o':
		keys, vals := membersOf(v)
		rows := make([][]any, len(keys))
		for i, k := range keys {
			rows[i] = []any{k, cellOf(vals[i])}
		}
		return llmfmt.Table{Cols: []string{"key", "value"}, Rows: rows}
	}
	return llmfmt.Table{Cols: []string{"value"}, Rows: [][]any{{cellOf(v)}}}
}

// shapeAcc accumulates the shape of the values met at one place: their
// kinds (first-seen order), the lengths of the containers and, for a
// place that is opened, the members of the objects and the elements of
// the arrays.
type shapeAcc struct {
	kinds          []byte
	objects        int
	oMin, oMax     int
	aMin, aMax     int
	arrays         int
	keys           []string
	kidx           map[string]int
	kids           []*shapeAcc
	kseen          []int
	klast          []int
	items          *shapeAcc
	truncatedNames int
}

func (a *shapeAcc) see(k byte) {
	for _, x := range a.kinds {
		if x == k {
			return
		}
	}
	a.kinds = append(a.kinds, k)
}

// shape reads the value at s.i into a; level 1 is the root, and the
// containers of levels 1 and 2 are opened.
func (s *jscan) shape(a *shapeAcc, level int) {
	k := s.kindAt()
	a.see(k)
	switch k {
	case 'o':
		a.objects++
		inst := a.objects
		open := level <= 2
		n := s.members(level, func(key []byte, esc bool) {
			if !open {
				s.skip(level + 1)
				return
			}
			if a.kidx == nil {
				a.kidx = map[string]int{}
			}
			j, ok := a.kidx[string(key)]
			if !ok && esc {
				j, ok = a.kidx[decodeKey(key, esc)]
			}
			if !ok {
				if len(a.keys) >= jsonShapeKeys {
					a.truncatedNames++
					s.skip(level + 1)
					return
				}
				name := decodeKey(key, esc)
				j = len(a.keys)
				a.kidx[name] = j
				a.keys = append(a.keys, name)
				a.kids = append(a.kids, &shapeAcc{})
				a.kseen = append(a.kseen, 0)
				a.klast = append(a.klast, 0)
			}
			if a.klast[j] != inst {
				a.klast[j] = inst
				a.kseen[j]++
			}
			s.shape(a.kids[j], level+1)
		})
		a.oMin, a.oMax = span2(a.objects, n, a.oMin, a.oMax)
	case 'a':
		a.arrays++
		open := level <= 2
		if open && a.items == nil {
			a.items = &shapeAcc{}
		}
		n := s.elements(level, func(int) {
			if open {
				s.shape(a.items, level+1)
			} else {
				s.skip(level + 1)
			}
		})
		a.aMin, a.aMax = span2(a.arrays, n, a.aMin, a.aMax)
	default:
		s.skip(level)
	}
}

// span2 folds the length n of the count-th container into lo..hi.
func span2(count, n, lo, hi int) (int, int) {
	if count == 1 {
		return n, n
	}
	return min(lo, n), max(hi, n)
}

// typeText is the type of the place: the kinds joined by "|", with the
// length of objects as {N} and of arrays as [N] (a range min..max when the
// values differ).
func (a *shapeAcc) typeText() string {
	parts := make([]string, len(a.kinds))
	length := func(lo, hi int) string {
		if lo == hi {
			return strconv.Itoa(lo)
		}
		return strconv.Itoa(lo) + ".." + strconv.Itoa(hi)
	}
	for i, k := range a.kinds {
		switch k {
		case 'o':
			parts[i] = "object{" + length(a.oMin, a.oMax) + "}"
		case 'a':
			parts[i] = "array[" + length(a.aMin, a.aMax) + "]"
		default:
			parts[i] = kindName(k)
		}
	}
	return strings.Join(parts, "|")
}

// opened reports whether a was read with its members (a container of
// level 1 or 2).
func (a *shapeAcc) opened() bool { return a.kidx != nil || a.items != nil }

// appendShape writes a: an opened place as {"type", "keys", "optional",
// "truncated", "items"}, any other as its type text.
func (a *shapeAcc) appendShape(dst []byte, root bool) []byte {
	if !a.opened() && !root {
		return append(dst, llmfmt.Quote(a.typeText())...)
	}
	dst = append(dst, `{"type":`...)
	dst = append(dst, llmfmt.Quote(a.typeText())...)
	if len(a.keys) > 0 {
		dst = append(dst, `,"keys":{`...)
		var optional []string
		for i, k := range a.keys {
			if i > 0 {
				dst = append(dst, ',')
			}
			dst = append(dst, llmfmt.Quote(k)...)
			dst = append(dst, ':')
			dst = a.kids[i].appendShape(dst, false)
			if a.kseen[i] < a.objects {
				optional = append(optional, k)
			}
		}
		dst = append(dst, '}')
		if len(optional) > 0 {
			dst = append(dst, `,"optional":[`...)
			for i, k := range optional {
				if i > 0 {
					dst = append(dst, ',')
				}
				dst = append(dst, llmfmt.Quote(k)...)
			}
			dst = append(dst, ']')
		}
		if a.truncatedNames > 0 {
			dst = append(dst, `,"truncated":`...)
			dst = strconv.AppendInt(dst, int64(a.truncatedNames), 10)
		}
	}
	if a.items != nil && len(a.items.kinds) > 0 {
		dst = append(dst, `,"items":`...)
		dst = a.items.appendShape(dst, false)
	}
	return append(dst, '}')
}

// shapeRows flattens a into path/type rows (--shape --format tsv).
func (a *shapeAcc) shapeRows(path string, rows [][]any) [][]any {
	rows = append(rows, []any{path, a.typeText()})
	join := func(k string) string {
		step := jsonpath.Format([]jsonpath.Step{{Key: k}})
		if path == "." {
			return step
		}
		if strings.HasPrefix(step, "[") {
			return path + step
		}
		return path + "." + step
	}
	for i, k := range a.keys {
		rows = a.kids[i].shapeRows(join(k), rows)
	}
	if a.items != nil && len(a.items.kinds) > 0 {
		p := path + "[]"
		if path == "." {
			p = "[]"
		}
		rows = a.items.shapeRows(p, rows)
	}
	return rows
}

// shapeOf reads the shape of v.
func shapeOf(v jsonValue) *shapeAcc {
	a := &shapeAcc{}
	s := &jscan{b: v.b, i: v.start}
	s.shape(a, 1)
	return a
}

// jsonArg is llm_json's mode.
type jsonArg struct {
	path        string
	pathSet     bool
	keys, shape bool
}

// json is llm_json.
func (m *Module) json(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	var a jsonArg
	var p pathTaker
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		w := args[0]
		switch {
		case w == "--path":
			if len(args) < 2 {
				return fail(c, "missing_value", w, w+" needs a value")
			}
			a.path, a.pathSet = args[1], true
			args = args[2:]
			continue
		case w == "--keys":
			a.keys = true
		case w == "--shape":
			a.shape = true
		case w == "-":
			if !p.take(c, w) {
				return 1
			}
		case strings.HasPrefix(w, "-"):
			return fail(c, "unknown_option", w, "Use FILE (or - for stdin), --path P, --keys, --shape, --format F, --width N, --head N, --tail N")
		default:
			if !p.take(c, w) {
				return 1
			}
		}
		args = args[1:]
	}
	if !p.set {
		return fail(c, "missing_file", "", "Provide a JSON file path as argument, or - for the standard input")
	}
	if a.keys && a.shape {
		return fail(c, "unexpected_argument", "--keys", "Use one of --keys and --shape")
	}
	var steps []jsonpath.Step
	if a.pathSet {
		var err error
		if steps, err = jsonpath.Parse(a.path); err != nil {
			return fail(c, "invalid_json_path", a.path, `Write keys with dots and indexes in brackets: a.b[0].c, paths["/api/x"].post, [-1]`)
		}
	}
	data, rc, ok := m.readJSONInput(c, p.path)
	if !ok {
		return rc
	}
	var sh *shapeAcc
	read := func(s *jscan) { s.skip(0) }
	if a.shape && !a.pathSet {
		// One pass: the shape of the root reads (and checks) every byte.
		sh = &shapeAcc{}
		read = func(s *jscan) { s.shape(sh, 1) }
	}
	start, err := scanJSON(data, read)
	if err != nil {
		return fail(c, "invalid_json", jsonErrorText(data, err).Error(),
			"The input is not valid JSON; fix it or read the raw text with llm_config")
	}
	v := valueAt(data, start)
	if a.pathSet {
		got, n, found := walkPath(v, steps)
		if !found {
			return fail(c, "json_path_not_found", a.path, pathHint(steps[:n], got))
		}
		v = got
	}
	f := o.FormatOr(llmfmt.Default)
	switch {
	case a.shape:
		if sh == nil {
			sh = shapeOf(v)
		}
		if f == llmfmt.TSV || f == llmfmt.TableF || f == llmfmt.JSONL {
			o.Write(c, f, llmfmt.Table{Cols: []string{"path", "type"}, Rows: sh.shapeRows(".", nil)}, nil)
			return 0
		}
		_, _ = c.Stdout.Write(append(sh.appendShape(nil, true), '\n'))
		return 0
	case a.keys:
		keys, ok := keysOf(v)
		if !ok {
			return fail(c, "not_an_object", kindName(v.kind), "--keys reads an object or an array of objects: reach one with --path, or use --shape")
		}
		if f == llmfmt.TSV || f == llmfmt.TableF || f == llmfmt.JSONL {
			rows := make([][]any, len(keys))
			for i, k := range keys {
				rows[i] = []any{k}
			}
			o.Write(c, f, llmfmt.Table{Cols: []string{"key"}, Rows: rows}, nil)
			return 0
		}
		b := []byte{'['}
		for i, k := range keys {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, llmfmt.Quote(k)...)
		}
		_, _ = c.Stdout.Write(append(b, ']', '\n'))
		return 0
	case f == llmfmt.TSV || f == llmfmt.TableF:
		o.Write(c, f, rowsOf(v), nil)
		return 0
	case f == llmfmt.JSONL:
		elems := []jsonValue{v}
		if v.kind == 'a' {
			elems = elementsOf(v)
		}
		head, tail, omitted := llmfmt.Cap(o, elems)
		var b []byte
		for _, part := range [][]jsonValue{head, tail} {
			for _, e := range part {
				b = append(appendCompact(b, e.raw(), o.Width), '\n')
			}
		}
		_, _ = c.Stdout.Write(b)
		noteOmitted(c, omitted)
		return 0
	}
	// The value as compact JSON; a line budget keeps the first and last
	// elements of an array.
	if v.kind == 'a' && o.Capped() {
		head, tail, omitted := llmfmt.Cap(o, elementsOf(v))
		b := []byte{'['}
		first := true
		for _, part := range [][]jsonValue{head, tail} {
			for _, e := range part {
				if !first {
					b = append(b, ',')
				}
				first = false
				b = appendCompact(b, e.raw(), o.Width)
			}
		}
		_, _ = c.Stdout.Write(append(b, ']', '\n'))
		noteOmitted(c, omitted)
		return 0
	}
	_, _ = c.Stdout.Write(append(appendCompact(make([]byte, 0, len(v.raw())+1), v.raw(), o.Width), '\n'))
	return 0
}

// jsonErrorText is a syntax error of data with its line and column.
func jsonErrorText(data []byte, err error) error {
	var je *jsonError
	if !errors.As(err, &je) {
		return err
	}
	line, col := where(data, je.at)
	return errors.New("line " + strconv.Itoa(line) + " column " + strconv.Itoa(col) + ": " + je.msg)
}

// noteOmitted writes the metadata line of a cut array (plan D8).
func noteOmitted(c *invoke.Call, omitted int) {
	if omitted > 0 {
		c.NoteTruncated(omitted)
		llmfmt.Meta(c, map[string]any{"truncated": omitted})
	}
}

// pathHint names the nearest existing prefix of a missing path and what
// is there (its type, and the first keys of an object or the length of
// an array).
func pathHint(prefix []jsonpath.Step, at jsonValue) string {
	switch at.kind {
	case 'o':
		keys, _ := keysOf(at)
		return prefixHint(prefix, "object", keys, 0)
	case 'a':
		return prefixHint(prefix, "array", nil, len(elementsOf(at)))
	}
	return prefixHint(prefix, kindName(at.kind), nil, 0)
}

// prefixHint is the hint text of a missing path: the prefix, its type
// and the first keys of an object or the length of an array.
func prefixHint(prefix []jsonpath.Step, kind string, keys []string, length int) string {
	h := "Nearest existing prefix: " + jsonpath.Format(prefix) + " (" + kind
	switch kind {
	case "object":
		shown := keys
		if len(shown) > 10 {
			shown = shown[:10]
		}
		h += ", keys: " + strings.Join(shown, ", ")
		if len(keys) > len(shown) {
			h += ", ... (" + strconv.Itoa(len(keys)) + " in all)"
		}
	case "array":
		h += ", length " + strconv.Itoa(length)
	}
	return h + ")"
}

// readJSONInput reads FILE (path rails, secret files refused) or, for
// "-", the standard input.
func (m *Module) readJSONInput(c *invoke.Call, file string) ([]byte, int, bool) {
	if file == "-" {
		data, err := io.ReadAll(c.Stdin)
		if err != nil {
			return nil, fail(c, "path_unreadable", "-", "The standard input could not be read"), false
		}
		return data, 0, true
	}
	safe, rc, ok := m.readableFile(c, file)
	if !ok {
		return nil, rc, false
	}
	data, err := m.readFile(safe)
	if err != nil {
		return nil, fail(c, "path_unreadable", safe, "The file could not be read"), false
	}
	return data, 0, true
}

// readableFile applies the llm_config rules to a file argument: secret
// files refused (FATAL), the path guard, a regular readable file.
func (m *Module) readableFile(c *invoke.Call, file string) (string, int, bool) {
	if m.guard(c).IsSecretFile(file) {
		return "", fail(c, "secret_file_blocked", file, "This file contains secrets and cannot be read"), false
	}
	safe, ok := m.validatePath(c, file)
	if !ok {
		return "", 1, false
	}
	st, err := os.Stat(safe)
	if err != nil || !st.Mode().IsRegular() {
		return "", fail(c, "file_not_found", safe, "Check the file path and try again"), false
	}
	return safe, 0, true
}
