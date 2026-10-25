// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmfmt

import (
	"bytes"
	"encoding/json"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// Table is row data: named columns and rows of values. A value is nil
// (SQL NULL, JSON null, TSV \N), a string, a bool, an integer or float
// type, json.Number, json.RawMessage (written as is), or anything
// encoding/json encodes (written as compact JSON in TSV and table cells).
type Table struct {
	Cols []string
	Rows [][]any
}

// Write writes t to c.Stdout in format f (Default is JSON) under the
// budget of o, and the metadata line on c.Stderr when rows were cut or
// meta is not empty (meta gets "truncated" added). It returns how many
// rows were omitted, and notes them on the call for the telemetry.
//
//   - json: one compact array of objects, keys in column order;
//   - jsonl: one compact object per row;
//   - tsv: a header line, then one line per row; tab, newline, carriage
//     return and backslash in a value are written \t, \n, \r, \\, a nil
//     value \N;
//   - table: aligned columns for humans, the omitted marker inline.
func (o *Options) Write(c *invoke.Call, f Format, t Table, meta map[string]any) int {
	if f == Default {
		f = JSON
	}
	head, tail, omitted := Cap(o, t.Rows)
	var b bytes.Buffer
	switch f {
	case JSONL:
		for _, r := range head {
			o.writeObject(&b, t.Cols, r)
			b.WriteByte('\n')
		}
		for _, r := range tail {
			o.writeObject(&b, t.Cols, r)
			b.WriteByte('\n')
		}
	case TSV:
		lines := make([]string, 0, len(head)+len(tail)+1)
		lines = append(lines, tsvLine(t.Cols))
		for _, r := range head {
			lines = append(lines, tsvRow(r))
		}
		for _, r := range tail {
			lines = append(lines, tsvRow(r))
		}
		for _, l := range lines {
			b.WriteString(Cut(l, o.Width))
			b.WriteByte('\n')
		}
	case TableF:
		o.writeAligned(&b, t.Cols, head, tail, omitted)
	default:
		b.WriteByte('[')
		first := true
		for _, part := range [][][]any{head, tail} {
			for _, r := range part {
				if !first {
					b.WriteByte(',')
				}
				first = false
				o.writeObject(&b, t.Cols, r)
			}
		}
		b.WriteString("]\n")
	}
	_, _ = c.Stdout.Write(b.Bytes())
	if omitted > 0 {
		if meta == nil {
			meta = map[string]any{}
		}
		meta["truncated"] = omitted
		c.NoteTruncated(omitted)
	}
	if len(meta) > 0 {
		Meta(c, meta)
	}
	return omitted
}

// writeObject writes one row as a compact JSON object, keys in column
// order; --width cuts string values.
func (o *Options) writeObject(b *bytes.Buffer, cols []string, row []any) {
	b.WriteByte('{')
	for i, col := range cols {
		if i > 0 {
			b.WriteByte(',')
		}
		writeJSONString(b, col)
		b.WriteByte(':')
		var v any
		if i < len(row) {
			v = row[i]
		}
		if s, ok := v.(string); ok && o.Width > 0 {
			v = Cut(s, o.Width)
		}
		writeJSONValue(b, v)
	}
	b.WriteByte('}')
}

// writeAligned writes a table for humans: a header, rows padded to the
// widest cell of each column (two spaces between columns), the omitted
// marker between the head and the tail rows.
func (o *Options) writeAligned(b *bytes.Buffer, cols []string, head, tail [][]any, omitted int) {
	cells := func(r []any) []string {
		out := make([]string, len(cols))
		for i := range cols {
			if i < len(r) {
				out[i] = humanCell(r[i])
			}
		}
		return out
	}
	rows := [][]string{cols}
	for _, r := range head {
		rows = append(rows, cells(r))
	}
	split := len(rows)
	for _, r := range tail {
		rows = append(rows, cells(r))
	}
	widths := make([]int, len(cols))
	for _, r := range rows {
		for i, s := range r {
			widths[i] = max(widths[i], StringWidth(s))
		}
	}
	line := func(r []string) {
		var l strings.Builder
		for i, s := range r {
			if i > 0 {
				l.WriteString("  ")
			}
			l.WriteString(s)
			if i < len(r)-1 {
				l.WriteString(strings.Repeat(" ", widths[i]-StringWidth(s)))
			}
		}
		b.WriteString(Cut(l.String(), o.Width))
		b.WriteByte('\n')
	}
	for i, r := range rows {
		if i == split && omitted > 0 {
			b.WriteString(OmittedMarker(omitted, "rows") + "\n")
		}
		line(r)
	}
	if split == len(rows) && omitted > 0 {
		b.WriteString(OmittedMarker(omitted, "rows") + "\n")
	}
}

// humanCell is a value as a table cell: nil is empty, controls are spaces.
func humanCell(v any) string {
	if v == nil {
		return ""
	}
	s := scalarText(v)
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}

func tsvLine(cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = TSVEscape(c)
	}
	return strings.Join(parts, "\t")
}

func tsvRow(r []any) string {
	parts := make([]string, len(r))
	for i, v := range r {
		if v == nil {
			parts[i] = `\N`
			continue
		}
		parts[i] = TSVEscape(scalarText(v))
	}
	return strings.Join(parts, "\t")
}

// TSVEscape escapes a TSV value: backslash, tab, newline and carriage
// return become \\, \t, \n and \r.
func TSVEscape(s string) string {
	if !strings.ContainsAny(s, "\\\t\n\r") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// scalarText is a non-nil value as text: strings as is, numbers in their
// shortest form, booleans true/false, anything else as compact JSON.
func scalarText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case float64:
		return formatFloat(x)
	case float32:
		return formatFloat(float64(x))
	case json.Number:
		return x.String()
	case json.RawMessage:
		return string(x)
	case []byte:
		return string(x)
	}
	var b bytes.Buffer
	writeJSONValue(&b, v)
	return b.String()
}

func formatFloat(f float64) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	if f == math.Trunc(f) && math.Abs(f) < 1e15 {
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// Quote returns s as a JSON string (no HTML escaping; invalid UTF-8
// becomes U+FFFD, as encoding/json writes it).
func Quote(s string) string {
	enc, _ := marshal(s)
	return string(enc)
}

// writeJSONString writes s as a JSON string (no HTML escaping).
func writeJSONString(b *bytes.Buffer, s string) {
	enc, _ := marshal(s)
	b.Write(enc)
}

// writeJSONValue writes v as compact JSON.
func writeJSONValue(b *bytes.Buffer, v any) {
	switch x := v.(type) {
	case nil:
		b.WriteString("null")
	case json.RawMessage:
		b.Write(x)
	case float64:
		if math.IsInf(x, 0) || math.IsNaN(x) {
			b.WriteString("null")
			return
		}
		b.WriteString(formatFloat(x))
	default:
		enc, err := marshal(v)
		if err != nil {
			b.WriteString("null")
			return
		}
		b.Write(enc)
	}
}

// marshal is json.Marshal without HTML escaping and without the trailing
// newline of an Encoder.
func marshal(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n")), nil
}

// JSONLine writes v to w as one compact JSON line (no HTML escaping; map
// keys sorted, as encoding/json writes them).
func JSONLine(w io.Writer, v any) error {
	enc, err := marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(append(enc, '\n'))
	return err
}

// Meta writes the metadata of a call as one JSON line on stderr (plan D8:
// stdout stays pure data), keys sorted. It never holds an "error" key, so
// it cannot be read as an error envelope.
func Meta(c *invoke.Call, meta map[string]any) {
	if len(meta) == 0 {
		return
	}
	delete(meta, "error")
	_ = JSONLine(c.Stderr, meta)
}

// Count is the answer of a --count mode: {"count":N} or
// {"count":N,"by":{KEY:N,...}} (plan S4).
type Count struct {
	N  int
	By map[string]int
}

// Write writes the count in format f: json/jsonl one object; tsv a
// "key\tcount" header and one row per key, most frequent first (ties by
// key), or "count" and N without --by; table the same, aligned.
func (n Count) Write(c *invoke.Call, o *Options, f Format) {
	switch f {
	case TSV, TableF:
		if n.By == nil {
			o.Write(c, f, Table{Cols: []string{"count"}, Rows: [][]any{{n.N}}}, nil)
			return
		}
		keys := make([]string, 0, len(n.By))
		for k := range n.By {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool {
			if n.By[keys[i]] != n.By[keys[j]] {
				return n.By[keys[i]] > n.By[keys[j]]
			}
			return keys[i] < keys[j]
		})
		rows := make([][]any, len(keys))
		for i, k := range keys {
			rows[i] = []any{k, n.By[k]}
		}
		o.Write(c, f, Table{Cols: []string{"key", "count"}, Rows: rows}, map[string]any{"count": n.N})
	default:
		var b bytes.Buffer
		b.WriteString(`{"count":` + strconv.Itoa(n.N))
		if n.By != nil {
			b.WriteString(`,"by":`)
			writeJSONValue(&b, n.By)
		}
		b.WriteString("}\n")
		_, _ = c.Stdout.Write(b.Bytes())
	}
}
