// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// grepOpts are llm_grep's 3.4 options (plan F5, S4).
type grepOpts struct {
	excludes                                 []string
	onlyMatching, distinct, count, filesOnly bool
	by                                       string
}

func (x grepOpts) any() bool {
	return x.excludes != nil || x.onlyMatching || x.distinct || x.count || x.filesOnly || x.by != ""
}

// take parses one 3.4 option of llm_grep at args[0] (see findOpts.take).
func (x *grepOpts) take(c *invoke.Call, args []string) (int, bool) {
	switch a := args[0]; a {
	case "--only-matching", "-o":
		x.onlyMatching = true
	case "--distinct":
		x.distinct = true
	case "--count", "-c":
		x.count = true
	case "--files-only", "-l":
		x.filesOnly = true
	case "--exclude", "--by":
		if len(args) < 2 {
			fail(c, "missing_value", a, a+" needs a value")
			return 0, false
		}
		if a == "--by" {
			x.by = args[1]
		} else {
			x.excludes = append(x.excludes, args[1])
		}
		return 2, true
	default:
		return 0, true
	}
	return 1, true
}

// grepQuery is one llm_grep call that reached the 3.4 code.
type grepQuery struct {
	safe      string
	limit     int
	context   int
	o         *llmfmt.Options
	x         grepOpts
	search    func() (*rgSearch, int, bool)
	forbidden func(string) bool
}

// grepExt is llm_grep with a 3.4 option: --exclude, --only-matching,
// --distinct, --count [--by], --files-only, rows for --format, the output
// budget and --relative. The text output is the 3.0 printer with those
// changes only.
func (m *Module) grepExt(c *invoke.Call, q grepQuery) int {
	x, o := q.x, q.o
	if x.by != "" && !x.distinct {
		x.count = true
	}
	switch x.by {
	case "", "file", "ext", "dir":
	default:
		return fail(c, "invalid_by", x.by, "Use --by file, ext or dir")
	}
	if x.distinct {
		x.onlyMatching = true
	}
	switch {
	case x.filesOnly && x.onlyMatching:
		return fail(c, "conflicting_options", "--files-only with --only-matching or --distinct",
			"--files-only lists files; --only-matching and --distinct list matches: pick one")
	case x.distinct && x.by != "":
		return fail(c, "conflicting_options", "--distinct with --by", "--distinct already counts each match; drop --by")
	case q.context > 0 && (o.Format != llmfmt.Default || x.onlyMatching || x.filesOnly || x.count):
		return fail(c, "conflicting_options", "--context with --format, --only-matching, --distinct, --files-only or --count",
			"Context lines go with the text output only; drop --context")
	}
	for _, e := range x.excludes {
		if e == "" || strings.ContainsAny(e, "\n") {
			return fail(c, "invalid_filter", e, "--exclude takes a non-empty glob without a newline, e.g. '*.min.js' or 'tests'")
		}
	}
	s, rc, ok := q.search()
	if !ok {
		return rc
	}
	s.context = q.context
	s.forbidden = q.forbidden
	s.excludes = x.excludes
	s.onlyMatching = x.onlyMatching
	s.filesOnly = x.filesOnly
	s.relative = o.Relative
	switch {
	case x.distinct:
		return grepDistinct(c, q, s)
	case x.count:
		cnt := llmfmt.Count{}
		if x.by != "" {
			cnt.By = map[string]int{}
		}
		s.hit = func(h grepHit) error {
			cnt.N++
			if cnt.By != nil {
				cnt.By[grepKey(s, x.by, h.path)]++
			}
			return nil
		}
		s.run(q.safe, io.Discard, maxLimit)
		cnt.Write(c, o, o.Format)
		return 0
	case o.Format != llmfmt.Default:
		return grepRows(c, q, s)
	}
	lw := o.Lines(c.Stdout, "lines")
	limit := q.limit
	if o.Capped() {
		limit = maxLimit
	}
	s.run(q.safe, lineSink{lw}, limit)
	closeLines(c, lw)
	return 0
}

// grepKey is the --by key of a match: the file as printed, its extension,
// or the first directory below the root.
func grepKey(s *rgSearch, by, path string) string {
	switch by {
	case "file":
		return s.show(path)
	case "ext":
		return extKey(basename(path))
	}
	return dirKey(relBelow(s.root, path))
}

// grepRow is one row of --format: {"file","line","text"}, {"file"} with
// --files-only.
type grepRow struct {
	file string
	line int
	text string
}

func grepRows(c *invoke.Call, q grepQuery, s *rgSearch) int {
	ro := rowBudget(q.o, q.limit)
	k := keeper[grepRow]{head: ro.Head, tail: ro.Tail}
	if q.limit > 0 || q.o.Capped() {
		s.hit = func(h grepHit) error {
			k.add(grepRow{file: s.show(h.path), line: h.line, text: string(h.text)})
			return nil
		}
		s.run(q.safe, io.Discard, maxLimit)
	}
	t := llmfmt.Table{Cols: []string{"file", "line", "text"}}
	conv := func(r grepRow) []any { return []any{r.file, r.line, r.text} }
	if s.filesOnly {
		t.Cols = []string{"file"}
		conv = func(r grepRow) []any { return []any{r.file} }
	}
	t.Rows = k.rows(conv)
	ro.Write(c, q.o.Format, t, nil)
	return 0
}

// grepDistinct is --distinct: every distinct match with its number of
// occurrences, most frequent first (ties in byte order), as rows
// {"match","count"} (a JSON array unless --format says otherwise), under
// the -n limit or the output budget.
func grepDistinct(c *invoke.Call, q grepQuery, s *rgSearch) int {
	counts := map[string]int{}
	s.hit = func(h grepHit) error {
		counts[string(h.text)]++
		return nil
	}
	s.run(q.safe, io.Discard, maxLimit)
	keys := make([]string, 0, len(counts))
	for k := range counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if counts[keys[i]] != counts[keys[j]] {
			return counts[keys[i]] > counts[keys[j]]
		}
		return keys[i] < keys[j]
	})
	rows := make([][]any, len(keys))
	for i, k := range keys {
		rows[i] = []any{k, counts[k]}
	}
	if q.limit == 0 && !q.o.Capped() {
		rows = nil
	}
	rowBudget(q.o, q.limit).Write(c, q.o.Format, llmfmt.Table{Cols: []string{"match", "count"}, Rows: rows}, nil)
	return 0
}
