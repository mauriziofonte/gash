// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
)

// textGzipMax is the size from which llm_textstats leaves the gzip size
// out (null): compressing it would cost seconds.
const textGzipMax = 64 << 20

// textStats is the answer of llm_textstats for one file.
type textStats struct {
	name                         string
	lines, words, chars, maxLine int64
	bytes, gzip                  int64 // gzip -1: not computed
	err                          string
}

// isMarkdown reports whether a path names a Markdown file, whose words
// leave out code blocks and the front matter.
func isMarkdown(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown", ".mdx", ".mkd":
		return true
	}
	return false
}

// byteCounter counts what is written to it.
type byteCounter struct{ n int64 }

func (b *byteCounter) Write(p []byte) (int, error) { b.n += int64(len(p)); return len(p), nil }

// isWordSpace is wc's word separator: Unicode white space except the
// no-break spaces.
func isWordSpace(r rune) bool {
	switch r {
	case 0xa0, 0x2007, 0x202f:
		return false
	}
	return unicode.IsSpace(r)
}

// countWords counts the words of a line (runs of non-space).
func countWords(line []byte) int64 {
	var n int64
	in := false
	for i := 0; i < len(line); {
		c := line[i]
		var space bool
		if c < utf8.RuneSelf {
			space = c == ' ' || (c >= '\t' && c <= '\r')
			i++
		} else {
			r, size := utf8.DecodeRune(line[i:])
			space = isWordSpace(r)
			i += size
		}
		if !space && !in {
			n++
		}
		in = !space
	}
	return n
}

// mdState tracks the parts of a Markdown file whose words do not count:
// the front matter (--- or +++ on line 1 up to the same line again) and
// fenced code blocks (``` or ~~~, closed by a fence of the same character
// at least as long).
type mdState struct {
	front     string // the front matter's delimiter while inside it
	fence     byte
	fenceLen  int
	firstLine bool
}

// skip reports whether the words of this line are left out.
func (s *mdState) skip(line []byte) bool {
	first := s.firstLine
	s.firstLine = false
	t := bytes.TrimRight(line, " \t\r")
	if first && (string(t) == "---" || string(t) == "+++") {
		s.front = string(t)
		return true
	}
	if s.front != "" {
		if string(t) == s.front || (s.front == "---" && string(t) == "...") {
			s.front = ""
		}
		return true
	}
	lead := len(t) - len(bytes.TrimLeft(t, " "))
	f := t[lead:]
	if lead <= 3 && len(f) >= 3 && (f[0] == '`' || f[0] == '~') {
		n := 0
		for n < len(f) && f[n] == f[0] {
			n++
		}
		switch {
		case s.fence == 0 && n >= 3:
			s.fence, s.fenceLen = f[0], n
			return true
		case s.fence == f[0] && n >= s.fenceLen && len(bytes.TrimSpace(f[n:])) == 0:
			s.fence = 0
			return true
		}
	}
	return s.fence != 0
}

// statText is the statistics of one guarded file in one read: lines (an
// unterminated last line counts), words (wc -w; Markdown without code
// blocks and front matter), UTF-8 characters, bytes, the gzip size at
// level 6 (with the file name in the header, as gzip -c FILE writes it)
// and the longest line in characters.
func statText(ctx context.Context, t textTarget) textStats {
	r := textStats{gzip: -1}
	if t.secret {
		r.err = "secret_file_blocked"
		return r
	}
	f, size, err := openRegular(t.real)
	if err != nil {
		r.err = targetError(ctx, err)
		return r
	}
	defer func() { _ = f.Close() }()
	if bin, err := sniffBinary(f); err != nil {
		r.err = targetError(ctx, err)
		return r
	} else if bin {
		r.err = "binary_file"
		return r
	}
	r.bytes = size
	var gz *gzip.Writer
	var gzn byteCounter
	if size <= textGzipMax {
		gz, _ = gzip.NewWriterLevel(&gzn, gzip.DefaultCompression)
		gz.Name = filepath.Base(t.real)
	}
	var md *mdState
	if isMarkdown(t.real) {
		md = &mdState{firstLine: true}
	}
	err = scanBlocks(ctx, f, 0, size, func(b []byte) {
		if gz != nil {
			_, _ = gz.Write(b)
		}
		for len(b) > 0 {
			line := b
			if i := bytes.IndexByte(b, '\n'); i >= 0 {
				line, b = b[:i], b[i+1:]
				r.chars++ // the newline
			} else {
				b = nil
			}
			r.lines++
			n := int64(utf8.RuneCount(line))
			r.chars += n
			if len(line) > 0 && line[len(line)-1] == '\r' {
				n--
			}
			r.maxLine = max(r.maxLine, n)
			if md == nil || !md.skip(line) {
				r.words += countWords(line)
			}
		}
	})
	if err != nil {
		return textStats{err: targetError(ctx, err), gzip: -1}
	}
	if gz != nil {
		_ = gz.Close()
		r.gzip = gzn.n
	}
	return r
}

// textstats is llm_textstats FILE... (plan F11): wc -l -w -m -c, gzip -c
// | wc -c and the longest line of each file in one read.
func (m *Module) textstats(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var files []string
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
		if a := args[0]; strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		}
		files = append(files, args[0])
		args = args[1:]
	}
	if len(files) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_textstats FILE...")
	}
	targets, ok := m.textTargets(c, files, false)
	if !ok {
		return 1
	}
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.LocalJobs()), o.Timeout, targets, func(ctx context.Context, t textTarget) textStats {
		r := statText(ctx, t)
		r.name = t.name(o)
		return r
	})
	withErr := false
	for _, r := range rows {
		withErr = withErr || r.err != ""
	}
	t := llmfmt.Table{Cols: []string{"p", "lines", "words", "chars", "bytes", "gzip", "max_line"}}
	if withErr {
		t.Cols = append(t.Cols, "error")
	}
	for _, r := range rows {
		row := []any{r.name, nil, nil, nil, nil, nil, nil}
		if r.err == "" {
			row[1], row[2], row[3], row[4], row[6] = r.lines, r.words, r.chars, r.bytes, r.maxLine
			if r.gzip >= 0 {
				row[5] = r.gzip
			}
		}
		if withErr {
			if r.err != "" {
				row = append(row, r.err)
			} else {
				row = append(row, nil)
			}
		}
		t.Rows = append(t.Rows, row)
	}
	o.Write(c, o.FormatOr(llmfmt.JSON), t, nil)
	return 0
}

// The rules of llm_textcheck, in the order a line's issues are listed.
var textRules = []string{"bom", "crlf", "non-ascii", "trailing-ws", "tabs", "long-lines", "mixed-indent", "final-newline"}

// textRulesDefault are the rules without --rules: every rule except tabs
// and long-lines, which depend on the project's style.
var textRulesDefault = []string{"non-ascii", "crlf", "bom", "trailing-ws", "final-newline", "mixed-indent"}

// textIssuesMax is the default cap of the issue list (--head/--max-lines
// change it); textIssuesKept bounds what one file holds in memory.
const (
	textIssuesMax  = 100
	textIssuesKept = 10000
	textLongLines  = 120
)

// textIssue is one finding of llm_textcheck.
type textIssue struct {
	line   int64
	rule   string
	sample string
}

// textCheck is the outcome for one file: its issues (at most
// textIssuesKept), how many there were, by rule, or why it was skipped.
type textCheck struct {
	issues []textIssue
	total  int
	counts map[string]int
	skip   string
}

// textRuleSet is the parsed --rules.
type textRuleSet struct {
	on       map[string]bool
	longLine int
}

// parseTextRules reads the comma-separated --rules list.
func parseTextRules(v string) (textRuleSet, string, bool) {
	rs := textRuleSet{on: map[string]bool{}, longLine: textLongLines}
	for _, r := range strings.Split(v, ",") {
		r = strings.TrimSpace(r)
		if name, n, ok := strings.Cut(r, "="); ok && name == "long-lines" {
			if !uintRE.MatchString(n) || atoi(n) == 0 {
				return rs, r, false
			}
			rs.longLine = atoi(n)
			r = name
		}
		known := false
		for _, k := range textRules {
			known = known || k == r
		}
		if !known {
			return rs, r, false
		}
		rs.on[r] = true
	}
	return rs, "", true
}

// excerpt is the part of a line an issue shows: the whole line when it is
// at most 40 characters, else 40 characters around the byte offset at,
// with an ellipsis where it was cut.
func excerpt(line []byte, at int) string {
	const w = 40
	if utf8.RuneCount(line) <= w {
		return string(line)
	}
	runes := []rune(string(line))
	pos := utf8.RuneCount(line[:min(at, len(line))])
	start := max(0, min(pos-w/4, len(runes)-w))
	end := min(len(runes), start+w)
	s := string(runes[start:end])
	if start > 0 {
		s = llmfmt.Ellipsis + s
	}
	if end < len(runes) {
		s += llmfmt.Ellipsis
	}
	return s
}

// nonASCIISample names the non-ASCII characters of a line: their code
// points (the printable ones shown), four at most, then how many more;
// a byte that is not UTF-8 is named as such.
func nonASCIISample(line []byte) string {
	var parts []string
	seen := map[rune]bool{}
	more := 0
	for i := 0; i < len(line); {
		if line[i] < utf8.RuneSelf {
			i++
			continue
		}
		r, size := utf8.DecodeRune(line[i:])
		var part string
		if r == utf8.RuneError && size == 1 {
			part = fmt.Sprintf("invalid UTF-8 byte 0x%02X", line[i])
			r = -rune(line[i])
		} else {
			part = fmt.Sprintf("U+%04X", r)
			if unicode.IsGraphic(r) && !unicode.IsSpace(r) {
				part += " " + string(r)
			}
		}
		i += size
		if seen[r] {
			continue
		}
		seen[r] = true
		if len(parts) == 4 {
			more++
			continue
		}
		parts = append(parts, part)
	}
	s := strings.Join(parts, ", ")
	if more > 0 {
		s += " +" + strconv.Itoa(more) + " more"
	}
	return s
}

// checkText runs the rules on one guarded file in one read.
func checkText(ctx context.Context, path string, rs textRuleSet) textCheck {
	out := textCheck{counts: map[string]int{}}
	f, size, err := openRegular(path)
	if err != nil {
		out.skip = targetError(ctx, err)
		return out
	}
	defer func() { _ = f.Close() }()
	if bin, err := sniffBinary(f); err != nil {
		out.skip = targetError(ctx, err)
		return out
	} else if bin {
		out.skip = "binary_file"
		return out
	}
	add := func(line int64, rule, sample string) {
		out.total++
		out.counts[rule]++
		if len(out.issues) < textIssuesKept {
			out.issues = append(out.issues, textIssue{line, rule, sample})
		}
	}
	var n, crlfLines int64
	crlfAt := -1 // index of the crlf issue, whose sample is set at the end
	var indent byte
	mixedDone := false
	var last []byte
	err = scanBlocks(ctx, f, 0, size, func(b []byte) {
		for len(b) > 0 {
			line := b
			if i := bytes.IndexByte(b, '\n'); i >= 0 {
				line, b = b[:i], b[i+1:]
			} else {
				b = nil
			}
			n++
			if n == 1 && bytes.HasPrefix(line, []byte{0xef, 0xbb, 0xbf}) {
				if rs.on["bom"] {
					add(n, "bom", "U+FEFF")
				}
				line = line[3:]
			}
			body := line
			if len(body) > 0 && body[len(body)-1] == '\r' {
				body = body[:len(body)-1]
				crlfLines++
				if rs.on["crlf"] && crlfAt < 0 {
					crlfAt = len(out.issues)
					add(n, "crlf", "")
				}
			}
			if rs.on["non-ascii"] {
				for _, c := range body {
					if c >= utf8.RuneSelf {
						add(n, "non-ascii", nonASCIISample(body))
						break
					}
				}
			}
			if rs.on["trailing-ws"] && len(body) > 0 && (body[len(body)-1] == ' ' || body[len(body)-1] == '\t') {
				add(n, "trailing-ws", excerpt(body, len(body)))
			}
			if rs.on["tabs"] {
				if i := bytes.IndexByte(body, '\t'); i >= 0 {
					add(n, "tabs", excerpt(body, i))
				}
			}
			if rs.on["long-lines"] {
				if w := utf8.RuneCount(body); w > rs.longLine {
					add(n, "long-lines", strconv.Itoa(w)+" chars: "+excerpt(body, 0))
				}
			}
			if rs.on["mixed-indent"] && !mixedDone && len(body) > 0 && (body[0] == ' ' || body[0] == '\t') {
				if len(bytes.TrimLeft(body, " \t")) > 0 {
					switch {
					case indent == 0:
						indent = body[0]
					case body[0] != indent:
						add(n, "mixed-indent", excerpt(body, 0))
						mixedDone = true
					}
				}
			}
			if b == nil {
				last = append(last[:0], body...)
			}
		}
	})
	if err != nil {
		return textCheck{skip: targetError(ctx, err)}
	}
	if crlfAt >= 0 && crlfAt < len(out.issues) {
		out.issues[crlfAt].sample = strconv.FormatInt(crlfLines, 10) + " of " + strconv.FormatInt(n, 10) + " lines"
	}
	if rs.on["final-newline"] && size > 0 {
		end := make([]byte, 1)
		if _, err := f.ReadAt(end, size-1); err == nil && end[0] != '\n' {
			add(n, "final-newline", excerpt(last, len(last)))
		}
	}
	order := map[string]int{}
	for i, r := range textRules {
		order[r] = i
	}
	sort.SliceStable(out.issues, func(i, j int) bool {
		a, b := out.issues[i], out.issues[j]
		if a.line != b.line {
			return a.line < b.line
		}
		return order[a.rule] < order[b.rule]
	})
	return out
}

// textcheck is llm_textcheck FILE... [--rules LIST] (plan F12): the text
// hygiene rules on each file in one read; binary files are skipped.
func (m *Module) textcheck(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var files []string
	rs, _, _ := parseTextRules(strings.Join(textRulesDefault, ","))
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
		switch a := args[0]; {
		case a == "--rules":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			var bad string
			if rs, bad, ok = parseTextRules(v); !ok {
				return fail(c, "invalid_rule", bad, "Rules: "+strings.Join(textRules, ", ")+" (long-lines=N sets the limit, default 120)")
			}
			args = args[2:]
			continue
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		default:
			files = append(files, a)
		}
		args = args[1:]
	}
	if len(files) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_textcheck FILE... [--rules LIST]")
	}
	targets, ok := m.textTargets(c, uniq(files), true)
	if !ok {
		return 1
	}
	res := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.LocalJobs()), o.Timeout, targets, func(ctx context.Context, t textTarget) textCheck {
		return checkText(ctx, t.real, rs)
	})
	checked, total := 0, 0
	counts := map[string]int{}
	type skipped struct {
		File   string `json:"file"`
		Reason string `json:"reason"`
	}
	var skips []skipped
	var rows [][]any
	for i, r := range res {
		if r.skip != "" {
			skips = append(skips, skipped{targets[i].name(o), r.skip})
			continue
		}
		checked++
		total += r.total
		for k, v := range r.counts {
			counts[k] += v
		}
		for _, is := range r.issues {
			rows = append(rows, []any{targets[i].name(o), is.line, is.rule, is.sample})
		}
	}
	co := *o
	if !co.Capped() {
		co.Head = textIssuesMax
	}
	head, tail, omitted := llmfmt.Cap(&co, rows)
	// Issues a file held beyond textIssuesKept are cut too.
	omitted += total - len(rows)
	if omitted > 0 {
		c.NoteTruncated(omitted)
	}
	if f := o.FormatOr(llmfmt.Default); f != llmfmt.Default && f != llmfmt.JSON {
		meta := map[string]any{"files": checked}
		if len(skips) > 0 {
			meta["skipped"] = skips
		}
		if omitted > 0 {
			meta["truncated"] = omitted
		}
		co.Head, co.Tail = 0, 0
		co.Write(c, f, llmfmt.Table{Cols: []string{"file", "line", "rule", "sample"}, Rows: append(head[:len(head):len(head)], tail...)}, meta)
		return 0
	}
	type issue struct {
		File   string `json:"file"`
		Line   int64  `json:"line"`
		Rule   string `json:"rule"`
		Sample string `json:"sample"`
	}
	issues := make([]issue, 0, len(head)+len(tail))
	for _, part := range [][][]any{head, tail} {
		for _, r := range part {
			issues = append(issues, issue{r[0].(string), r[1].(int64), r[2].(string), llmfmt.Cut(r[3].(string), o.Width)})
		}
	}
	out := struct {
		Files     int            `json:"files"`
		Issues    []issue        `json:"issues"`
		Counts    map[string]int `json:"counts,omitempty"`
		Truncated int            `json:"truncated,omitempty"`
		Skipped   []skipped      `json:"skipped,omitempty"`
	}{checked, issues, nil, omitted, skips}
	if total > 0 {
		out.Counts = counts
	}
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}
