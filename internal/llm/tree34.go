// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"sort"
	"strconv"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// treeOpts are llm_tree's 3.4 options (plan F2, F3).
type treeOpts struct {
	count, all             bool
	by, typ, name, top     string
	newer, since           string
	haveNewer, haveSince   bool
	haveTop, haveTyp, have bool // have: a filter (--type, --name, --newer, --since)
}

func (x treeOpts) any() bool { return x.count || x.all || x.by != "" || x.haveTop || x.have }

// take parses one 3.4 option of llm_tree at args[0] (see findOpts.take).
func (x *treeOpts) take(c *invoke.Call, args []string) (int, bool) {
	switch a := args[0]; a {
	case "--count":
		x.count = true
	case "--all":
		x.all = true
	case "--by", "--type", "--name", "--newer", "--since", "--top":
		if len(args) < 2 {
			fail(c, "missing_value", a, a+" needs a value")
			return 0, false
		}
		v := args[1]
		switch a {
		case "--by":
			x.by = v
		case "--type":
			x.typ, x.haveTyp, x.have = v, true, true
		case "--name":
			x.name, x.have = v, true
		case "--newer":
			x.newer, x.haveNewer, x.have = v, true, true
		case "--since":
			x.since, x.haveSince, x.have = v, true, true
		default:
			x.top, x.haveTop = v, true
		}
		return 2, true
	default:
		return 0, true
	}
	return 1, true
}

// treeQuery is one llm_tree call that reached the 3.4 code.
type treeQuery struct {
	safe      string
	depth     int
	depthSet  bool // --depth was given
	textMode  bool
	withStats bool
	forbidden func(string) bool
	o         *llmfmt.Options
	x         treeOpts
}

// treeExt is llm_tree with a 3.4 option: --count [--by], --top N, --all,
// --relative, --format and the budget of the --text listing.
func (m *Module) treeExt(c *invoke.Call, q treeQuery) int {
	x, o := q.x, q.o
	if x.by != "" && !x.haveTop {
		x.count = true
	}
	conflict := func(details, hint string) int { return fail(c, "conflicting_options", details, hint) }
	switch {
	case x.haveTop && x.count:
		return conflict("--top with --count", "--top lists sizes, --count counts entries: pick one")
	case (x.haveTop || x.count) && q.textMode:
		return conflict("--text with --count or --top", "--count and --top print JSON; drop --text")
	case x.haveTop && x.haveTyp:
		return conflict("--type with --top", "--top sums the files below each directory; drop --type")
	case x.have && !x.count && !x.haveTop:
		return conflict("--type, --name, --newer or --since without --count or --top",
			"These filters select what --count counts and --top sums; add one of them")
	case !x.count && !x.haveTop && o.Format != llmfmt.Default && o.Format != llmfmt.JSON:
		return conflict("--format "+string(o.Format)+" with the tree listing",
			"The tree is JSON (or --text); --format jsonl, tsv and table go with --count and --top")
	case !x.count && !x.haveTop && !q.textMode && (o.Width > 0 || o.Capped()):
		return conflict("--width, --max-lines, --head or --tail with the JSON tree",
			"The output budget applies to --text, --count and --top; the JSON tree stops at 500 entries by itself")
	}
	if x.haveTyp && x.typ != "f" && x.typ != "d" {
		return fail(c, "invalid_type", x.typ, "Use --type f (files) or --type d (directories)")
	}
	times, ok := m.walkTimes(c, x.newer, x.since, x.haveNewer, x.haveSince)
	if !ok {
		return 1
	}
	prune := prunedNames
	if x.all {
		prune = nil
	}
	switch {
	case x.haveTop:
		return m.treeTop(c, q, times)
	case x.count:
		switch x.by {
		case "", "ext", "dir", "type":
		default:
			return fail(c, "invalid_by", x.by, "Use --by ext, dir or type")
		}
		sel := &walkSel{prune: prune, badRel: "\n\t", pattern: x.name, types: "fdl", times: times, maxDepth: -1}
		if x.haveTyp {
			sel.types = x.typ
		}
		if q.depthSet {
			sel.maxDepth = q.depth
		}
		treeCount(c, q.safe, sel, x.by, q.forbidden, m.guardedDirs(c)).Write(c, o, o.Format)
		return 0
	case q.textMode:
		limit := textCap
		if o.Capped() {
			limit = maxLimit
		}
		lw := o.Lines(c.Stdout, "lines")
		text := treeText(q.safe, q.depth, q.forbidden, prune, limit)
		for _, l := range bytes.SplitAfter(text, []byte("\n")) {
			if len(l) > 0 {
				lw.Line(string(bytes.TrimSuffix(l, []byte("\n"))))
			}
		}
		closeLines(c, lw)
		return 0
	}
	shown := q.safe
	if o.Relative {
		shown = "."
	}
	_, _ = c.Stdout.Write(treeJSON(q.safe, shown, q.depth, q.withStats, q.forbidden, prune))
	return 0
}

// treeCount is llm_tree --count: the entries below PATH the listing would
// show at any depth (or down to an explicit --depth), on several
// goroutines.
func treeCount(c *invoke.Call, root string, sel *walkSel, by string, forbidden, guarded func(string) bool) llmfmt.Count {
	jobs := llmfmt.LocalJobs()
	type tally struct {
		n    int
		keys map[string]int
	}
	tallies := make([]tally, jobs)
	for i := range tallies {
		if by != "" {
			tallies[i].keys = map[string]int{}
		}
	}
	poolWalk(c.Ctx, jobs, walkDir{path: root}, func(w int, d walkDir) []walkDir {
		return countDir(d, sel, by, &tallies[w].n, tallies[w].keys, forbidden, guarded)
	})
	cnt := llmfmt.Count{}
	if by != "" {
		cnt.By = map[string]int{}
	}
	for _, t := range tallies {
		cnt.N += t.n
		for k, v := range t.keys {
			cnt.By[k] += v
		}
	}
	return cnt
}

// duRow is one row of --top: a directory (its path below PATH) or an
// extension, with the bytes and the number of files below it.
type duRow struct {
	key         string
	size, files int64
}

// treeTop is llm_tree --top N (plan F3): disk usage below PATH, the
// largest first. Sizes are apparent sizes (st_size, as --stats gives
// them) of every file below PATH, node_modules, vendor and .git included
// (disk usage hides nothing); the protected trees are left out and the
// directories that cannot be read are counted in "skipped". --by dir (the
// default) lists every directory down to --depth (default 1) with its
// cumulative size, as du -d does; --by ext the extensions.
func (m *Module) treeTop(c *invoke.Call, q treeQuery, times mtimeFilter) int {
	x, o := q.x, q.o
	if !uintRE.MatchString(x.top) || atoi(x.top) < 1 {
		return fail(c, "invalid_top", x.top, "Give --top a positive integer (1 to 999999999)")
	}
	switch x.by {
	case "", "dir", "ext":
	default:
		return fail(c, "invalid_by", x.by, "--top takes --by dir or --by ext")
	}
	byExt := x.by == "ext"
	depth := 1
	if q.depthSet {
		depth = q.depth
	}
	jobs := llmfmt.LocalJobs()
	type acc struct {
		size, files int64
		skipped     int
		rows        map[string]*duRow
	}
	accs := make([]acc, jobs)
	for i := range accs {
		accs[i].rows = map[string]*duRow{}
	}
	bump := func(a *acc, key string, size, files int64) {
		r := a.rows[key]
		if r == nil {
			r = &duRow{key: key}
			a.rows[key] = r
		}
		r.size += size
		r.files += files
	}
	guarded := m.guardedDirs(c)
	poolWalk(c.Ctx, jobs, walkDir{path: q.safe}, func(w int, d walkDir) []walkDir {
		a := &accs[w]
		check := q.forbidden != nil && guarded(d.path)
		var subs []walkDir
		var size, files int64
		ok := scanDir(d.path, true, func(e scanEntry) {
			if check && q.forbidden(findJoin(d.path, e.name)) {
				return
			}
			if e.failed {
				a.skipped++
				return
			}
			if e.typ == 'd' {
				rel := relJoin(d.rel, e.name)
				subs = append(subs, walkDir{path: findJoin(d.path, e.name), rel: rel, depth: d.depth + 1})
				if !byExt && d.depth+1 <= depth {
					bump(a, rel, 0, 0) // listed even when empty
				}
				return
			}
			if x.name != "" && !pathguard.MatchGlobSegment(x.name, e.name) || times.active() && !times.ok(e.mtime) {
				return
			}
			size += e.size
			files++
			if byExt {
				bump(a, extKey(e.name), e.size, 1)
			}
		})
		if !ok {
			a.skipped++
		}
		a.size += size
		a.files += files
		if !byExt && files > 0 {
			// The directory's own files count in each listed directory
			// above it (and in itself when listed).
			for i, n := 0, 0; i <= len(d.rel) && n < depth && d.rel != ""; i++ {
				if i == len(d.rel) || d.rel[i] == '/' {
					bump(a, d.rel[:i], size, files)
					n++
				}
			}
		}
		return subs
	})
	var total, files int64
	skipped := 0
	merged := map[string]*duRow{}
	for _, a := range accs {
		total += a.size
		files += a.files
		skipped += a.skipped
		for k, r := range a.rows {
			mr := merged[k]
			if mr == nil {
				mr = &duRow{key: k}
				merged[k] = mr
			}
			mr.size += r.size
			mr.files += r.files
		}
	}
	rows := make([]duRow, 0, len(merged))
	for _, r := range merged {
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].size != rows[j].size {
			return rows[i].size > rows[j].size
		}
		return rows[i].key < rows[j].key
	})
	keyCol := "p"
	if byExt {
		keyCol = "ext"
	}
	show := func(r duRow) string {
		if byExt || !o.Absolute {
			return r.key
		}
		return findJoin(q.safe, r.key)
	}
	ro := *o
	if !ro.Capped() {
		ro.Head = atoi(x.top)
	}
	if o.Format != llmfmt.Default && o.Format != llmfmt.JSON {
		table := llmfmt.Table{Cols: []string{keyCol, "size", "files"}, Rows: make([][]any, len(rows))}
		for i, r := range rows {
			table.Rows[i] = []any{show(r), r.size, r.files}
		}
		ro.Write(c, o.Format, table, map[string]any{"total": total, "files": files, "skipped": skipped})
		return 0
	}
	head, tail, omitted := llmfmt.Cap(&ro, rows)
	var b []byte
	b = append(b, `{"total":`...)
	b = strconv.AppendInt(b, total, 10)
	b = append(b, `,"files":`...)
	b = strconv.AppendInt(b, files, 10)
	b = append(b, `,"top":[`...)
	for i, r := range append(append([]duRow{}, head...), tail...) {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '{', '"')
		b = append(b, keyCol...)
		b = append(b, `":`...)
		b = contract.AppendString(b, llmfmt.Cut(show(r), o.Width), contract.EscapeJQ)
		b = append(b, `,"size":`...)
		b = strconv.AppendInt(b, r.size, 10)
		b = append(b, `,"files":`...)
		b = strconv.AppendInt(b, r.files, 10)
		b = append(b, '}')
	}
	b = append(b, `],"skipped":`...)
	b = strconv.AppendInt(b, int64(skipped), 10)
	if omitted > 0 {
		b = append(b, `,"truncated":`...)
		b = strconv.AppendInt(b, int64(omitted), 10)
		c.NoteTruncated(omitted)
	}
	b = append(b, "}\n"...)
	_, _ = c.Stdout.Write(b)
	return 0
}
