// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

// readMaxBytes is the default --max-bytes of llm_read: the content bytes
// printed per file.
const readMaxBytes = 256 << 10

// readLineMax bounds the bytes of one line llm_read holds (and matches
// --around against); the rest of a longer line is dropped.
const readLineMax = 16 << 20

// lineRange is a 1-based inclusive line range; end 0 is the end of file.
type lineRange struct{ from, end int }

func (r lineRange) has(n int) bool { return n >= r.from && (r.end == 0 || n <= r.end) }

// parseRange reads A:B, A-B or A,B (also A: and :B, open ends, and a lone
// A); A and B are positive and A <= B.
func parseRange(s string) (lineRange, bool) {
	i := strings.IndexAny(s, ":-,")
	a, b := s, s
	if i >= 0 {
		a, b = s[:i], s[i+1:]
	}
	r := lineRange{from: 1}
	if a != "" {
		if !uintRE.MatchString(a) || atoi(a) == 0 {
			return r, false
		}
		r.from = atoi(a)
	}
	if b != "" {
		if !uintRE.MatchString(b) || atoi(b) == 0 {
			return r, false
		}
		r.end = atoi(b)
	}
	if (a == "" && b == "") || (r.end != 0 && r.end < r.from) {
		return r, false
	}
	return r, true
}

// splitPathRange reads a PATH:A-B (or PATH:A:B) word: the path and its
// range, when the word as a whole is not an existing path and its tail is
// a range.
func splitPathRange(word string) (string, lineRange, bool) {
	if _, err := os.Lstat(word); err == nil {
		return word, lineRange{}, false
	}
	// PATH:A:B: the range starts at the second-last colon; PATH:A-B at
	// the last one.
	for _, i := range []int{lastIndexNth(word, ':', 2), strings.LastIndexByte(word, ':')} {
		if i <= 0 {
			continue
		}
		spec := word[i+1:]
		if !strings.ContainsAny(spec, ":-") && !uintRE.MatchString(spec) {
			continue
		}
		if r, ok := parseRange(spec); ok {
			return word[:i], r, true
		}
	}
	return word, lineRange{}, false
}

// lastIndexNth is the index of the n-th last c in s, -1 when there is none.
func lastIndexNth(s string, c byte, n int) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			if n--; n == 0 {
				return i
			}
		}
	}
	return -1
}

// readSpec is what llm_read selects in one file.
type readSpec struct {
	ranges   []lineRange
	around   []*rgRegex
	context  int
	maxBytes int
	head     int
	tail     int
}

// selLine is one selected line: its number, its separator (':' for a
// line of a range or a match, '-' for a context line of --around) and
// its text without the line break.
type selLine struct {
	n    int
	sep  byte
	text []byte
}

// readResult is the selection of one file.
type readResult struct {
	head, tail []selLine // kept lines; the omitted ones sit between them
	omitted    int
	total      int // lines read (the file's lines when the scan ran to the end)
	selected   int // lines selected before the budget
}

// readCollector keeps the selected lines under the budget of a readSpec:
// the first head lines and the last tail lines, or, without either, the
// lines that fit in maxBytes.
type readCollector struct {
	spec    *readSpec
	res     readResult
	bytes   int
	ring    []selLine
	ringPos int
}

func (k *readCollector) add(n int, sep byte, text []byte) {
	k.res.selected++
	s := k.spec
	if s.head == 0 && s.tail == 0 {
		if k.bytes+len(text) > s.maxBytes && len(k.res.head) > 0 {
			k.res.omitted++
			return
		}
		if len(text) > s.maxBytes {
			text = append(cutBytes(text, s.maxBytes), llmfmt.Ellipsis...)
		}
		k.bytes += len(text)
		k.res.head = append(k.res.head, selLine{n, sep, bytes.Clone(text)})
		return
	}
	if len(k.res.head) < s.head {
		k.res.head = append(k.res.head, selLine{n, sep, bytes.Clone(text)})
		return
	}
	if s.tail == 0 {
		k.res.omitted++
		return
	}
	l := selLine{n, sep, bytes.Clone(text)}
	if len(k.ring) < s.tail {
		k.ring = append(k.ring, l)
		return
	}
	k.ring[k.ringPos] = l
	k.ringPos = (k.ringPos + 1) % s.tail
	k.res.omitted++
}

// spent reports that no further line can be kept: --head is full, or
// (without --head and --tail) --max-bytes was reached.
func (k *readCollector) spent() bool {
	if k.spec.head > 0 {
		return len(k.res.head) >= k.spec.head
	}
	return k.spec.tail == 0 && k.res.omitted > 0
}

// countRest counts the lines left in r (a last line without a newline
// counts).
func countRest(r io.Reader) (int, error) {
	buf := make([]byte, 64<<10)
	n, last := 0, byte('\n')
	for {
		got, err := r.Read(buf)
		if chunk := buf[:got]; len(chunk) > 0 {
			n += bytes.Count(chunk, []byte{'\n'})
			last = chunk[len(chunk)-1]
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, err
		}
	}
	if last != '\n' {
		n++
	}
	return n, nil
}

// done returns the selection, the tail in order, with --max-bytes applied
// to the kept lines when --head or --tail chose them.
func (k *readCollector) done() readResult {
	r := k.res
	for i := range k.ring {
		r.tail = append(r.tail, k.ring[(k.ringPos+i)%len(k.ring)])
	}
	if k.spec.head == 0 && k.spec.tail == 0 {
		return r
	}
	used := 0
	cut := func(lines []selLine) []selLine {
		for i, l := range lines {
			if used+len(l.text) > k.spec.maxBytes && used > 0 {
				r.omitted += len(lines) - i
				return lines[:i]
			}
			used += len(l.text)
		}
		return lines
	}
	r.head = cut(r.head)
	if len(r.head) < len(k.res.head) {
		r.omitted += len(r.tail)
		r.tail = nil
	}
	r.tail = cut(r.tail)
	return r
}

// cutBytes is b cut to at most n bytes at a UTF-8 boundary.
func cutBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return b
	}
	for n > 0 && n < len(b) && !utf8.RuneStart(b[n]) {
		n--
	}
	return b[:n]
}

// selectLines reads r line by line and selects what spec asks for: the
// lines of its ranges, the matches of --around with their context, or
// every line without either.
func selectLines(ctx context.Context, r io.Reader, spec *readSpec) (readResult, error) {
	br := bufio.NewReaderSize(r, 64<<10)
	k := &readCollector{spec: spec}
	whole := len(spec.ranges) == 0 && len(spec.around) == 0
	lastRange := 0 // the last line a range can select; 0 = to the end
	for _, rg := range spec.ranges {
		if rg.end == 0 {
			lastRange = -1
			break
		}
		lastRange = max(lastRange, rg.end)
	}
	// Context before a match: the last lines not yet selected.
	var before []selLine
	after := 0
	n := 0
	var long []byte
	for {
		if n&1023 == 0 {
			if err := ctx.Err(); err != nil {
				return readResult{}, err
			}
		}
		chunk, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			if len(long) < readLineMax {
				long = append(long, chunk[:min(len(chunk), readLineMax-len(long))]...)
			}
			continue
		}
		if err != nil && !errors.Is(err, io.EOF) {
			return readResult{}, err
		}
		if len(chunk) == 0 && len(long) == 0 {
			break
		}
		line := chunk
		if long != nil {
			if len(long) < readLineMax {
				long = append(long, chunk[:min(len(chunk), readLineMax-len(long))]...)
			}
			line, long = long, nil
		}
		line = bytes.TrimSuffix(line, []byte{'\n'})
		n++
		inRange := whole
		for _, rg := range spec.ranges {
			if rg.has(n) {
				inRange = true
				break
			}
		}
		match := false
		for _, re := range spec.around {
			if re.Match(line) {
				match = true
				break
			}
		}
		switch {
		case match || inRange:
			if match {
				for _, b := range before {
					k.add(b.n, '-', b.text)
				}
			}
			before = before[:0]
			k.add(n, ':', line)
			if match {
				after = spec.context
			} else if after > 0 {
				after--
			}
		case after > 0:
			k.add(n, '-', line)
			after--
		case len(spec.around) > 0 && spec.context > 0:
			if len(before) == spec.context {
				copy(before, before[1:])
				before = before[:len(before)-1]
			}
			before = append(before, selLine{n, '-', bytes.Clone(line)})
		}
		if errors.Is(err, io.EOF) {
			break
		}
		// Past the last range, nothing more can be selected.
		if len(spec.around) == 0 && lastRange > 0 && n >= lastRange && spec.tail == 0 {
			res := k.done()
			res.total = -1
			return res, nil
		}
		// A whole-file read whose budget is spent only counts the rest.
		if whole && spec.tail == 0 && k.spent() {
			rest, err := countRest(br)
			if err != nil {
				return readResult{}, err
			}
			res := k.done()
			res.omitted += rest
			res.selected += rest
			res.total = n + rest
			return res, nil
		}
	}
	res := k.done()
	res.total = n
	return res, nil
}

// readFile is one opened file of llm_read.
type readFile struct {
	t    textTarget
	f    *os.File
	spec readSpec
}

// read is llm_read PATH... [--lines A:B] [--around RE -C N] [--max-bytes N]
// (plan F8): ranges of several files in one call, or the lines around the
// matches of RE, with line numbers; text blocks "==> file:A-B <==" or S1
// rows {"file","line","text"}.
func (m *Module) read(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.PathFlags)
	type pathArg struct {
		path   string
		rng    lineRange
		ranged bool
	}
	var paths []pathArg
	var ranges []lineRange
	var around []string
	spec := readSpec{context: -1, maxBytes: readMaxBytes}
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
		a := args[0]
		switch a {
		case "--lines", "--around", "-C", "--context", "--max-bytes":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			switch a {
			case "--lines":
				r, ok := parseRange(v)
				if !ok {
					return fail(c, "invalid_range", v, "Give --lines A:B with 1 <= A <= B (A: and :B leave an end open)")
				}
				ranges = append(ranges, r)
			case "--around":
				around = append(around, v)
			case "-C", "--context":
				if !uintRE.MatchString(v) {
					return fail(c, "invalid_context", v, "Give -C N, N a non-negative integer")
				}
				spec.context = atoi(v)
			default:
				mb, ok := takePositive(c, "invalid_size", v)
				if !ok {
					return 1
				}
				spec.maxBytes = mb
			}
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		}
		p, r, ranged := splitPathRange(a)
		paths = append(paths, pathArg{p, r, ranged})
		args = args[1:]
	}
	if len(paths) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_read FILE... [--lines A:B]")
	}
	if spec.context >= 0 && len(around) == 0 {
		return fail(c, "invalid_context", strconv.Itoa(spec.context), "-C N sets the context of --around RE; give --around too")
	}
	if spec.context < 0 {
		spec.context = 3
	}
	for _, p := range around {
		re, err := compileRg(p)
		if err != nil {
			return fail(c, "invalid_regex", p, "Patterns use ripgrep's regex syntax: "+err.Error())
		}
		spec.around = append(spec.around, re)
	}
	spec.head, spec.tail = o.Head, o.Tail
	words := make([]string, len(paths))
	for i, p := range paths {
		words[i] = p.path
	}
	targets, ok := m.textTargets(c, words, true)
	if !ok {
		return 1
	}
	files := make([]readFile, 0, len(targets))
	defer func() {
		for _, rf := range files {
			_ = rf.f.Close()
		}
	}()
	for i, t := range targets {
		f, _, err := openRegular(t.real)
		if err != nil {
			hint := "Check the file path and try again"
			if errors.Is(err, errNotAFile) {
				hint = "Not a regular file: list a directory with llm_tree or llm_find"
			}
			return fail(c, "file_not_found", t.arg, hint)
		}
		files = append(files, readFile{t: t, f: f, spec: spec})
		bin, err := sniffBinary(f)
		if err != nil {
			return fail(c, "path_unreadable", t.arg, "The file could not be read")
		}
		if bin {
			return fail(c, "binary_file", t.arg, "A binary file (it holds NUL bytes) is not read as text: use llm_stat or llm_hash")
		}
		if paths[i].ranged {
			files[i].spec.ranges = []lineRange{paths[i].rng}
		} else {
			files[i].spec.ranges = ranges
		}
	}
	f := o.FormatOr(llmfmt.Default)
	var w *bufio.Writer
	var table llmfmt.Table
	if f == llmfmt.Default {
		w = bufio.NewWriterSize(c.Stdout, 32<<10)
	} else {
		table.Cols = []string{"file", "line", "text"}
	}
	omitted := 0
	for _, rf := range files {
		res, err := selectLines(c.Ctx, rf.f, &rf.spec)
		if err != nil {
			if w != nil {
				_ = w.Flush()
			}
			return fail(c, "path_unreadable", rf.t.arg, "The file could not be read")
		}
		omitted += res.omitted
		name := rf.t.name(o)
		if w == nil {
			for _, part := range [][]selLine{res.head, res.tail} {
				for _, l := range part {
					table.Rows = append(table.Rows, []any{name, l.n, string(l.text)})
				}
			}
			continue
		}
		writeReadBlocks(w, name, &rf.spec, res, o.Width)
	}
	if w != nil {
		_ = w.Flush()
		c.NoteTruncated(omitted)
		return 0
	}
	// The budget was applied per file: the table goes out as it is, with
	// the omitted count in the metadata line.
	wo := *o
	wo.Head, wo.Tail = 0, 0
	var meta map[string]any
	if omitted > 0 {
		meta = map[string]any{"truncated": omitted}
		c.NoteTruncated(omitted)
	}
	wo.Write(c, f, table, meta)
	return 0
}

// writeReadBlocks writes the selection of one file as text blocks: a
// "==> name:A-B <==" header before each run of consecutive lines, each
// line as N:text (N-text for a context line of --around), the omitted
// marker where the budget cut. A selection with no line says why.
func writeReadBlocks(w *bufio.Writer, name string, spec *readSpec, res readResult, width int) {
	if len(res.head) == 0 && len(res.tail) == 0 {
		_, _ = w.WriteString("==> " + name + " <==\n")
		switch {
		case res.selected > 0:
			_, _ = w.WriteString(llmfmt.OmittedMarker(res.omitted, "lines") + "\n")
		case len(spec.around) > 0:
			_, _ = w.WriteString("[no match in " + strconv.Itoa(res.total) + " lines]\n")
		case len(spec.ranges) > 0:
			_, _ = w.WriteString("[no line in range: the file has " + strconv.Itoa(res.total) + " lines]\n")
		default:
			_, _ = w.WriteString("[empty file]\n")
		}
		return
	}
	block := func(lines []selLine) {
		for i := 0; i < len(lines); {
			j := i + 1
			for j < len(lines) && lines[j].n == lines[j-1].n+1 {
				j++
			}
			_, _ = w.WriteString("==> " + name + ":" + strconv.Itoa(lines[i].n) + "-" + strconv.Itoa(lines[j-1].n) + " <==\n")
			for _, l := range lines[i:j] {
				s := strconv.Itoa(l.n) + string(l.sep) + string(l.text)
				_, _ = w.WriteString(llmfmt.Cut(s, width))
				_ = w.WriteByte('\n')
			}
			i = j
		}
	}
	block(res.head)
	if res.omitted > 0 {
		_, _ = w.WriteString(llmfmt.OmittedMarker(res.omitted, "lines") + "\n")
	}
	block(res.tail)
}
