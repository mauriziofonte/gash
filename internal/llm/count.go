// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// counter is the scanner of llm_count and llm_assert (plan F6, F7): every
// pattern counted in one read of each file. A literal pattern is found
// with bytes.Index over a block of lines; a regex (ripgrep's syntax, as
// llm_grep reads it) is tried on the lines that hold its literal prefix.
// A pattern never matches across a line break.
type counter struct {
	lits  [][]byte   // literal patterns, nil with --regex
	res   []*rgRegex // the patterns with --regex
	occur bool       // count every occurrence, not the matching lines
}

// newCounter compiles the patterns. A pattern with a newline is
// invalid_filter, an empty one missing_pattern, a regex rg would refuse
// invalid_regex; each writes its envelope and returns false.
func newCounter(c *invoke.Call, pats []string, regex, occur bool) (*counter, bool) {
	k := &counter{occur: occur}
	for _, p := range pats {
		if p == "" {
			fail(c, "missing_pattern", "", "A pattern cannot be empty")
			return nil, false
		}
		if strings.ContainsAny(p, "\n\r") {
			fail(c, "invalid_filter", p, "A pattern cannot hold a line break: patterns match within one line")
			return nil, false
		}
		if !regex {
			k.lits = append(k.lits, []byte(p))
			continue
		}
		re, err := compileRg(p)
		if err != nil {
			fail(c, "invalid_regex", p, "Patterns use ripgrep's regex syntax: "+err.Error())
			return nil, false
		}
		k.res = append(k.res, re)
	}
	return k, true
}

// n is the number of patterns.
func (k *counter) n() int { return len(k.lits) + len(k.res) }

// countFile counts every pattern in the regular file at path: one read of
// the file (a large one in parallel parts that start at a line start).
func (k *counter) countFile(ctx context.Context, path string) ([]int64, error) {
	f, size, err := openRegular(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	bounds, err := lineBounds(f, size, fileParts(size))
	if err != nil {
		return nil, err
	}
	type part struct {
		acc []int64
		err error
	}
	ranges := make([][2]int64, len(bounds)-1)
	for i := range ranges {
		ranges[i] = [2]int64{bounds[i], bounds[i+1]}
	}
	parts := llmfmt.Map(ctx, len(ranges), 0, ranges, func(ctx context.Context, r [2]int64) part {
		acc := make([]int64, k.n())
		err := scanBlocks(ctx, f, r[0], r[1], func(b []byte) { k.block(b, acc) })
		return part{acc, err}
	})
	total := make([]int64, k.n())
	for _, p := range parts {
		if p.err != nil {
			return nil, p.err
		}
		for i, v := range p.acc {
			total[i] += v
		}
	}
	return total, nil
}

// block adds the counts of one block of whole lines to acc.
func (k *counter) block(b []byte, acc []int64) {
	for i, p := range k.lits {
		if k.occur {
			acc[i] += int64(bytes.Count(b, p))
			continue
		}
		var n int64
		for rest := b; ; {
			j := bytes.Index(rest, p)
			if j < 0 {
				break
			}
			n++
			e := bytes.IndexByte(rest[j+len(p):], '\n')
			if e < 0 {
				break
			}
			rest = rest[j+len(p)+e+1:]
		}
		acc[i] += n
	}
	for i, re := range k.res {
		if !re.MayMatch(b) {
			continue
		}
		acc[len(k.lits)+i] += k.regexBlock(re, b)
	}
}

// regexBlock counts one regex in a block: the lines that match, or the
// non-empty matches with occur. With a literal prefix only the lines that
// hold it are tried.
func (k *counter) regexBlock(re *rgRegex, b []byte) int64 {
	var n int64
	for len(b) > 0 {
		start := 0
		if re.lit != nil {
			j := bytes.Index(b, re.lit)
			if j < 0 {
				break
			}
			start = bytes.LastIndexByte(b[:j], '\n') + 1
		}
		end := bytes.IndexByte(b[start:], '\n')
		line, next := b[start:], len(b)
		if end >= 0 {
			line, next = b[start:start+end], start+end+1
		}
		if re.Match(line) {
			if k.occur {
				for _, m := range re.re.FindAllIndex(line, -1) {
					if m[1] > m[0] {
						n++
					}
				}
			} else {
				n++
			}
		}
		b = b[next:]
	}
	return n
}

// countResult is the counts of one file, or the error that stood for them.
type countResult struct {
	counts []int64
	err    string
}

// countTargets counts the patterns of k in every target, jobs at a time.
func countTargets(ctx context.Context, k *counter, targets []textTarget, jobs int, o *llmfmt.Options) []countResult {
	return llmfmt.Map(ctx, jobs, o.Timeout, targets, func(ctx context.Context, t textTarget) countResult {
		counts, err := k.countFile(ctx, t.real)
		if err != nil {
			return countResult{err: targetError(ctx, err)}
		}
		return countResult{counts: counts}
	})
}

// count is llm_count FILE... --pat P... [--regex] [--lines|--occurrences]
// (plan F6): the matrix file x pattern from one pass over each file.
func (m *Module) count(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var files, pats []string
	regex, occur := false, false
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
		case a == "--pat":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			pats = append(pats, v)
			args = args[2:]
			continue
		case a == "--regex":
			regex = true
		case a == "--lines":
			occur = false
		case a == "--occurrences":
			occur = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --pat P (repeatable), --regex, --lines, --occurrences, --format, -j or --timeout")
		default:
			files = append(files, a)
		}
		args = args[1:]
	}
	if len(files) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_count FILE... --pat P")
	}
	if len(pats) == 0 {
		return fail(c, "missing_pattern", "", "Give at least one --pat P")
	}
	files, pats = uniq(files), uniq(pats)
	k, ok := newCounter(c, pats, regex, occur)
	if !ok {
		return 1
	}
	targets, ok := m.textTargets(c, files, true)
	if !ok {
		return 1
	}
	res := countTargets(c.Ctx, k, targets, o.JobsOr(llmfmt.LocalJobs()), o)
	f := o.FormatOr(llmfmt.Default)
	if f == llmfmt.Default {
		_, _ = c.Stdout.Write(countJSON(targets, pats, res, o))
		return 0
	}
	cols := []string{"file", "pat", "count"}
	withErr := false
	for _, r := range res {
		withErr = withErr || r.err != ""
	}
	if withErr {
		cols = append(cols, "error")
	}
	t := llmfmt.Table{Cols: cols}
	for i, r := range res {
		for j, p := range pats {
			row := []any{targets[i].name(o), p, nil}
			if r.err == "" {
				row[2] = r.counts[j]
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
	}
	o.Write(c, f, t, nil)
	return 0
}

// countJSON is the default output of llm_count: one object, files then
// patterns in input order, {"<file>":{"<pat>":N}} or
// {"<file>":{"error":"file_not_found"}}.
func countJSON(targets []textTarget, pats []string, res []countResult, o *llmfmt.Options) []byte {
	b := []byte{'{'}
	for i, r := range res {
		if i > 0 {
			b = append(b, ',')
		}
		b = appendJSONString(b, targets[i].name(o))
		b = append(b, ':', '{')
		if r.err != "" {
			b = append(b, `"error":`...)
			b = appendJSONString(b, r.err)
		} else {
			for j, p := range pats {
				if j > 0 {
					b = append(b, ',')
				}
				b = appendJSONString(b, p)
				b = append(b, ':')
				b = strconv.AppendInt(b, r.counts[j], 10)
			}
		}
		b = append(b, '}')
	}
	return append(b, '}', '\n')
}

// appendJSONString appends s as a JSON string (llmfmt.Quote).
func appendJSONString(b []byte, s string) []byte { return append(b, llmfmt.Quote(s)...) }

// uniq drops repeated words, keeping the first of each in order.
func uniq(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0:0]
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// assertCheck is one assertion of llm_assert.
type assertCheck struct {
	kind string // has, lacks, count
	pat  string
	want int64 // count only
}

// label is the check as the output names it: "has:P", "lacks:P",
// "count:P=N".
func (a assertCheck) label() string {
	if a.kind == "count" {
		return "count:" + a.pat + "=" + strconv.FormatInt(a.want, 10)
	}
	return a.kind + ":" + a.pat
}

// assert is llm_assert FILE... (--has P | --lacks P | --count P=N)...
// (plan F7): every assertion on every file, from the llm_count scanner;
// exit 0 only when all pass.
func (m *Module) assert(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.JobsFlag | llmfmt.TimeoutFlag | llmfmt.PathFlags)
	var files []string
	var checks []assertCheck
	regex, occur := false, false
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
		case a == "--has" || a == "--lacks" || a == "--count":
			v, ok := takeOption(c, args)
			if !ok {
				return 1
			}
			chk := assertCheck{kind: a[2:], pat: v}
			if a == "--count" {
				i := strings.LastIndexByte(v, '=')
				if i <= 0 || !uintRE.MatchString(v[i+1:]) {
					return fail(c, "invalid_assertion", v, "Write --count PATTERN=N, N a non-negative integer (the last = splits)")
				}
				chk.pat, chk.want = v[:i], int64(atoi(v[i+1:]))
			}
			checks = append(checks, chk)
			args = args[2:]
			continue
		case a == "--regex":
			regex = true
		case a == "--lines":
			occur = false
		case a == "--occurrences":
			occur = true
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use --has P, --lacks P, --count P=N (repeatable), --regex, --lines, --occurrences, -j or --timeout")
		default:
			files = append(files, a)
		}
		args = args[1:]
	}
	if len(files) == 0 {
		return fail(c, "missing_file", "", "Give one or more files: llm_assert FILE... --has P")
	}
	if len(checks) == 0 {
		return fail(c, "missing_pattern", "", "Give at least one --has P, --lacks P or --count P=N")
	}
	// One scanner over the distinct patterns of every check.
	var pats []string
	index := map[string]int{}
	for _, chk := range checks {
		if _, ok := index[chk.pat]; !ok {
			index[chk.pat] = len(pats)
			pats = append(pats, chk.pat)
		}
	}
	k, ok := newCounter(c, pats, regex, occur)
	if !ok {
		return 1
	}
	targets, ok := m.textTargets(c, uniq(files), true)
	if !ok {
		return 1
	}
	res := countTargets(c.Ctx, k, targets, o.JobsOr(llmfmt.LocalJobs()), o)
	type failure struct {
		File  string `json:"file"`
		Check string `json:"check,omitempty"`
		Found *int64 `json:"found,omitempty"`
		Error string `json:"error,omitempty"`
	}
	var failed []failure
	for i, r := range res {
		name := targets[i].name(o)
		if r.err != "" {
			failed = append(failed, failure{File: name, Error: r.err})
			continue
		}
		for _, chk := range checks {
			got := r.counts[index[chk.pat]]
			pass := (chk.kind == "has" && got > 0) || (chk.kind == "lacks" && got == 0) || (chk.kind == "count" && got == chk.want)
			if !pass {
				failed = append(failed, failure{File: name, Check: chk.label(), Found: &got})
			}
		}
	}
	checked := len(targets) * len(checks)
	if len(failed) == 0 {
		_, _ = c.Stdout.Write([]byte(`{"pass":true,"checked":` + strconv.Itoa(checked) + "}\n"))
		return 0
	}
	_ = llmfmt.JSONLine(c.Stdout, struct {
		Pass    bool      `json:"pass"`
		Checked int       `json:"checked"`
		Failed  []failure `json:"failed"`
	}{false, checked, failed})
	return 1
}
