// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

// llm_csv (plan D5): the header, a sample and, with --count, the row count
// of a CSV file, read by a streaming RFC 4180 parser: quoted fields may
// hold the delimiter, quotes ("") and newlines, so a record is not a line;
// CRLF and LF both end a record; a blank line is no record. The parser is
// lenient where Python's csv module is (a quote inside an unquoted field,
// text after a closing quote, an unterminated quote at the end of the
// file) and reports each such record, and each record whose field count
// differs from the header's, in "errors" (the first 10; error_count is the
// total with --count). Without --count the read stops as soon as the
// header and the sample are complete.

// csvChunk is the read size of the parser.
const csvChunk = 256 << 10

// csvMaxErrors is how many malformed records are listed.
const csvMaxErrors = 10

// csvSniffBytes is how much of the file the delimiter sniffing reads.
const csvSniffBytes = 64 << 10

// The parser states.
const (
	csvField  = iota // at the start of a field
	csvUnq           // inside an unquoted field
	csvQ             // inside a quoted field
	csvQQ            // a quote inside a quoted field: "" or the closing one
	csvAfterQ        // after a closing quote
)

// csvRecord is one record as the parser hands it over.
type csvRecord struct {
	line   int      // the physical line it starts on (1-based)
	n      int      // its field count
	fields []string // its fields, when it was kept
	col    string   // the field the filter reads, when asked
	hasCol bool
	err    string // a quoting problem, if any
}

// csvParser is a streaming RFC 4180 record reader.
type csvParser struct {
	delim  byte
	stop   [256]bool // the bytes that end the fast part of an unquoted field
	state  int
	line   int
	cr     bool // a carriage return ended the last chunk
	rec    csvRecord
	any    bool // the current record has content
	field  []byte
	keep   func() bool // keep the fields of the next record
	keepIt bool
	col    int // the field index the filter reads; -1 for none
	emit   func(*csvRecord) bool
	done   bool
}

func newCSVParser(delim byte, col int, keep func() bool, emit func(*csvRecord) bool) *csvParser {
	p := &csvParser{delim: delim, line: 1, keep: keep, col: col, emit: emit}
	p.stop[delim], p.stop['"'], p.stop['\n'], p.stop['\r'] = true, true, true, true
	p.begin()
	return p
}

// begin starts a record at the current line.
func (p *csvParser) begin() {
	p.rec = csvRecord{line: p.line}
	p.any = false
	p.field = p.field[:0]
	p.keepIt = p.keep()
	p.state = csvField
}

// keeping reports whether the bytes of the current field are kept.
func (p *csvParser) keeping() bool {
	return p.keepIt || p.rec.n == p.col
}

func (p *csvParser) endField() {
	if p.keepIt {
		p.rec.fields = append(p.rec.fields, string(p.field))
	}
	if p.rec.n == p.col {
		p.rec.col, p.rec.hasCol = string(p.field), true
	}
	p.rec.n++
	p.field = p.field[:0]
	p.state = csvField
}

func (p *csvParser) setErr(msg string) {
	if p.rec.err == "" {
		p.rec.err = msg
	}
}

// endLine ends the record at a newline; a line with nothing on it is no
// record.
func (p *csvParser) endLine() {
	if !p.any && p.rec.n == 0 {
		p.line++
		p.rec.line = p.line
		p.state = csvField
		return
	}
	p.endField()
	p.line++
	if !p.emit(&p.rec) {
		p.done = true
		return
	}
	p.begin()
}

// feed parses one chunk.
func (p *csvParser) feed(b []byte) {
	i := 0
	if p.cr {
		p.cr = false
		if len(b) > 0 && b[0] == '\n' {
			i = 1
			p.endLine()
		} else {
			p.dataCR()
		}
	}
	n := len(b)
	stop := &p.stop
	for i < n && !p.done {
		switch p.state {
		case csvField:
			if b[i] == '"' {
				p.state = csvQ
				p.any = true
				i++
				continue
			}
			p.state = csvUnq
		case csvUnq:
			j := i
			for j < n && !stop[b[j]] {
				j++
			}
			if j > i {
				p.any = true
				if p.keeping() {
					p.field = append(p.field, b[i:j]...)
				}
				i = j
			}
			if i >= n {
				return
			}
			switch c := b[i]; c {
			case p.delim:
				p.any = true
				p.endField()
				i++
			case '\n':
				p.endLine()
				i++
			case '\r':
				i = p.carriage(b, i)
			default: // a quote inside an unquoted field: kept as text
				p.setErr("quote in an unquoted field")
				p.any = true
				if p.keeping() {
					p.field = append(p.field, '"')
				}
				i++
			}
		case csvQ:
			j := bytes.IndexByte(b[i:], '"')
			end := n
			if j >= 0 {
				end = i + j
			}
			span := b[i:end]
			p.line += bytes.Count(span, []byte{'\n'})
			if p.keeping() {
				p.field = append(p.field, span...)
			}
			if j < 0 {
				return
			}
			i = end + 1
			p.state = csvQQ
		case csvQQ:
			if b[i] == '"' {
				if p.keeping() {
					p.field = append(p.field, '"')
				}
				p.state = csvQ
				i++
				continue
			}
			p.state = csvAfterQ
		case csvAfterQ:
			switch c := b[i]; c {
			case p.delim:
				p.endField()
				i++
			case '\n':
				p.endLine()
				i++
			case '\r':
				i = p.carriage(b, i)
			default: // text after a closing quote: kept, as Python keeps it
				p.setErr("text after a closing quote")
				p.state = csvUnq
			}
		}
	}
}

// carriage handles a carriage return at b[i]: CRLF ends the record, a
// lone CR is text; at the end of the chunk the next one decides.
func (p *csvParser) carriage(b []byte, i int) int {
	if i+1 >= len(b) {
		p.cr = true
		return i + 1
	}
	if b[i+1] == '\n' {
		p.endLine()
		return i + 2
	}
	p.dataCR()
	return i + 1
}

// dataCR keeps a lone carriage return as text of the field.
func (p *csvParser) dataCR() {
	p.any = true
	if p.state == csvAfterQ {
		p.setErr("text after a closing quote")
	}
	p.state = csvUnq
	if p.keeping() {
		p.field = append(p.field, '\r')
	}
}

// finish ends the input: a pending carriage return ends the record, an
// open quote is reported, a last record without a newline counts.
func (p *csvParser) finish() {
	if p.done {
		return
	}
	if p.cr {
		p.cr = false
		p.endLine()
		if p.done {
			return
		}
	}
	if p.state == csvQ {
		p.setErr("unterminated quoted field")
	}
	if p.any || p.rec.n > 0 {
		p.endField()
		p.emit(&p.rec)
	}
}

// sniffDelim picks the delimiter of a CSV text from its first lines: the
// candidate (, ; tab |) found the same number of times, at least once, on
// every line read (outside quotes), the most frequent first; else the most
// frequent one; else a comma.
func sniffDelim(b []byte) byte {
	cands := []byte{',', '\t', ';', '|'}
	var lines [][4]int
	var cur [4]int
	inQ, any := false, false
	for i := 0; i < len(b) && len(lines) < 10; i++ {
		c := b[i]
		switch {
		case c == '"':
			inQ = !inQ
		case c == '\n' && !inQ:
			if any {
				lines = append(lines, cur)
			}
			cur, any = [4]int{}, false
			continue
		case !inQ:
			for k, d := range cands {
				if c == d {
					cur[k]++
				}
			}
		}
		if c != '\r' {
			any = true
		}
	}
	if any && len(lines) < 10 && len(b) < csvSniffBytes {
		lines = append(lines, cur) // the last line of a short file
	}
	best, bestN := -1, 0
	for k := range cands {
		n := -1
		for _, l := range lines {
			if n == -1 {
				n = l[k]
			} else if l[k] != n {
				n = 0
				break
			}
		}
		if n > bestN {
			best, bestN = k, n
		}
	}
	if best >= 0 {
		return cands[best]
	}
	total := [4]int{}
	for _, l := range lines {
		for k := range cands {
			total[k] += l[k]
		}
	}
	for k := range cands {
		if total[k] > bestN {
			best, bestN = k, total[k]
		}
	}
	if best >= 0 {
		return cands[best]
	}
	return ','
}

// parseDelim reads --delim: one byte, or tab written as "tab" or \t.
func parseDelim(v string) (byte, bool) {
	switch v {
	case "tab", `\t`, "\t":
		return '\t', true
	}
	if len(v) != 1 || v[0] == '"' || v[0] == '\n' || v[0] == '\r' {
		return 0, false
	}
	return v[0], true
}

// csvError is one malformed record of the "errors" list.
type csvError struct {
	line int
	msg  string
}

// csv is llm_csv.
func (m *Module) csv(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag)
	var p pathTaker
	head, colsOnly, count := 5, false, false
	var delim byte
	whereSet, whereCol, whereVal := false, "", ""
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
		switch w {
		case "--head", "--delim", "--where":
			if len(args) < 2 {
				return fail(c, "missing_value", w, w+" needs a value")
			}
			v := args[1]
			switch w {
			case "--head":
				if !validUint(c, "head", v) {
					return 1
				}
				head = atoi(v)
			case "--delim":
				d, ok := parseDelim(v)
				if !ok {
					return fail(c, "invalid_delimiter", v, "Give one character (, ; | or tab; \\t and tab mean a tab), not a quote or a newline")
				}
				delim = d
			default:
				k, val, ok := strings.Cut(v, "=")
				if !ok || k == "" {
					return fail(c, "invalid_where", v, "Write --where COLUMN=VALUE (an exact match; COLUMN is a header name or a 1-based number)")
				}
				whereSet, whereCol, whereVal = true, k, val
			}
			args = args[2:]
			continue
		case "--cols":
			colsOnly = true
		case "--count":
			count = true
		default:
			if strings.HasPrefix(w, "-") {
				return fail(c, "unknown_option", w, registry.OptionsHint(c.Name))
			}
			if !p.take(c, w) {
				return 1
			}
		}
		args = args[1:]
	}
	if !p.set {
		return fail(c, "missing_file", "", "Provide a CSV file path as argument")
	}
	safe, rc, ok := m.readableFile(c, p.path)
	if !ok {
		return rc
	}
	f, err := os.Open(safe)
	if err != nil {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, csvChunk)
	first, err := io.ReadFull(f, buf)
	if errors.Is(err, io.ErrUnexpectedEOF) {
		err = io.EOF
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	chunk := buf[:first]
	chunk = bytes.TrimPrefix(chunk, []byte("\xef\xbb\xbf"))
	if delim == 0 {
		delim = sniffDelim(chunk[:min(len(chunk), csvSniffBytes)])
	}

	var header []string
	var sample [][]string
	var errs []csvError
	rows, matches, errCount := 0, 0, 0
	col := -1           // the filter's field index, once the header is read
	unknownCol := false // the filter names no column
	needSample := func() bool { return !colsOnly && len(sample) < head }
	keep := func() bool { return header == nil || needSample() }
	var parser *csvParser
	emit := func(r *csvRecord) bool {
		if header == nil {
			header = r.fields
			if header == nil {
				header = []string{}
			}
			if whereSet {
				col = columnIndex(header, whereCol)
				if col < 0 {
					unknownCol = true
					return false
				}
				parser.col = col
			}
			return count || needSample()
		}
		rows++
		msg := r.err
		if r.n != len(header) {
			fc := strconv.Itoa(r.n) + " fields, header has " + strconv.Itoa(len(header))
			if msg == "" {
				msg = fc
			} else {
				msg = fc + "; " + msg
			}
		}
		if msg != "" {
			errCount++
			if len(errs) < csvMaxErrors {
				errs = append(errs, csvError{line: r.line, msg: msg})
			}
		}
		match := !whereSet || (r.hasCol && r.col == whereVal)
		if match {
			matches++
			if needSample() && r.fields != nil {
				sample = append(sample, r.fields)
			}
		}
		return count || needSample()
	}
	parser = newCSVParser(delim, -1, keep, emit)
	parser.feed(chunk)
	for !parser.done && err == nil {
		var k int
		k, err = io.ReadFull(f, buf)
		if k > 0 {
			parser.feed(buf[:k])
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			err = io.EOF
		}
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return fail(c, "path_unreadable", safe, "The file could not be read")
	}
	parser.finish()
	if unknownCol {
		shown := header
		if len(shown) > 20 {
			shown = shown[:20]
		}
		return fail(c, "unknown_column", whereCol, "Columns: "+strings.Join(shown, ", ")+" (or a 1-based number)")
	}

	f2 := o.FormatOr(llmfmt.Default)
	if f2 == llmfmt.TSV || f2 == llmfmt.TableF || f2 == llmfmt.JSONL {
		meta := map[string]any{"delimiter": string(delim)}
		if count {
			meta["rows"] = rows
			meta["error_count"] = errCount
			if whereSet {
				meta["matches"] = matches
			}
		}
		if len(errs) > 0 {
			meta["errors"] = errorObjects(errs)
		}
		if colsOnly {
			t := llmfmt.Table{Cols: []string{"column"}}
			for _, h := range header {
				t.Rows = append(t.Rows, []any{h})
			}
			o.Write(c, f2, t, meta)
			return 0
		}
		t := llmfmt.Table{Cols: header}
		for _, r := range sample {
			row := make([]any, len(r))
			for i, v := range r {
				row[i] = v
			}
			t.Rows = append(t.Rows, row)
		}
		o.Write(c, f2, t, meta)
		return 0
	}
	b := []byte{'{'}
	sep := func() {
		if len(b) > 1 {
			b = append(b, ',')
		}
	}
	key := func(k string) {
		sep()
		b = append(b, '"')
		b = append(b, k...)
		b = append(b, '"', ':')
	}
	strs := func(list []string) {
		b = append(b, '[')
		for i, s := range list {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, llmfmt.Quote(s)...)
		}
		b = append(b, ']')
	}
	if count {
		key("rows")
		b = strconv.AppendInt(b, int64(rows), 10)
		if whereSet {
			key("matches")
			b = strconv.AppendInt(b, int64(matches), 10)
		}
	}
	key("cols")
	b = strconv.AppendInt(b, int64(len(header)), 10)
	key("header")
	strs(header)
	if !colsOnly {
		key("sample")
		b = append(b, '[')
		for i, r := range sample {
			if i > 0 {
				b = append(b, ',')
			}
			strs(r)
		}
		b = append(b, ']')
	}
	key("delimiter")
	b = append(b, llmfmt.Quote(string(delim))...)
	key("errors")
	b = append(b, '[')
	for i, e := range errs {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, `{"line":`...)
		b = strconv.AppendInt(b, int64(e.line), 10)
		b = append(b, `,"error":`...)
		b = append(b, llmfmt.Quote(e.msg)...)
		b = append(b, '}')
	}
	b = append(b, ']')
	if count {
		key("error_count")
		b = strconv.AppendInt(b, int64(errCount), 10)
	}
	b = append(b, '}', '\n')
	_, _ = c.Stdout.Write(b)
	return 0
}

// errorObjects is the errors list for the metadata line.
func errorObjects(errs []csvError) []map[string]any {
	out := make([]map[string]any, len(errs))
	for i, e := range errs {
		out[i] = map[string]any{"line": e.line, "error": e.msg}
	}
	return out
}

// columnIndex is the index of the column named name (exact), or of the
// 1-based column number name; -1 when there is none.
func columnIndex(header []string, name string) int {
	for i, h := range header {
		if h == name {
			return i
		}
	}
	if n, err := strconv.Atoi(name); err == nil && n >= 1 && n <= len(header) && strings.Trim(name, "0123456789") == "" {
		return n - 1
	}
	return -1
}
