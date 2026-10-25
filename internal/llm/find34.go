// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// findOpts are llm_find's 3.4 options (plan F2, F4).
type findOpts struct {
	count, tracked, all  bool
	by, sort             string
	newer, since         string
	haveNewer, haveSince bool
	typeSet              bool // --type was given
}

// any reports whether one of them was given.
func (x findOpts) any() bool {
	return x.count || x.tracked || x.all || x.by != "" || x.sort != "" || x.haveNewer || x.haveSince
}

// take parses one 3.4 option of llm_find at args[0]: the number of words
// it consumed (0 when args[0] is not one), false after an error envelope.
func (x *findOpts) take(c *invoke.Call, args []string) (int, bool) {
	switch a := args[0]; a {
	case "--count":
		x.count = true
	case "--tracked":
		x.tracked = true
	case "--all":
		x.all = true
	case "--sort", "--by", "--newer", "--since":
		if len(args) < 2 {
			fail(c, "missing_value", a, a+" needs a value")
			return 0, false
		}
		switch a {
		case "--sort":
			x.sort = args[1]
		case "--by":
			x.by = args[1]
		case "--newer":
			x.newer, x.haveNewer = args[1], true
		default:
			x.since, x.haveSince = args[1], true
		}
		return 2, true
	default:
		return 0, true
	}
	return 1, true
}

// extActive reports whether a walker call uses a 3.4 shared option that
// changes its output (--absolute alone keeps today's output).
func extActive(o *llmfmt.Options) bool {
	return o.Format != llmfmt.Default || o.Width > 0 || o.Capped() || o.Relative
}

// findQuery is one llm_find call that reached the 3.4 code: the parsed
// and validated arguments.
type findQuery struct {
	pattern, fileType, contains string
	limit                       int
	safe                        string
	forbidden                   func(string) bool
	o                           *llmfmt.Options
	x                           findOpts
}

// findHit is one path llm_find selected.
type findHit struct {
	path, rel string // the path as printed absolute, and below the root
	typ       byte
	size      int64
	mtime     time.Time
}

// findExt is llm_find with a 3.4 option: --count [--by], --sort,
// --newer/--since, --tracked, --all, rows for --format, the output budget
// and --relative. Without --count and --sort the listing is the 3.0
// listing (same walk, same order, same -n cut) with those changes only.
func (m *Module) findExt(c *invoke.Call, q findQuery) int {
	x := q.x
	if x.by != "" {
		x.count = true
	}
	switch x.by {
	case "", "ext", "dir", "type":
	default:
		return fail(c, "invalid_by", x.by, "Use --by ext, dir or type")
	}
	switch x.sort {
	case "", "name", "mtime", "size":
	default:
		return fail(c, "invalid_sort", x.sort, "Use --sort name (path order), mtime (newest first) or size (largest first)")
	}
	times, ok := m.walkTimes(c, x.newer, x.since, x.haveNewer, x.haveSince)
	if !ok {
		return 1
	}
	rootInfo, err := os.Lstat(q.safe)
	if err != nil {
		return fail(c, "path_not_found", q.safe, "Provide an existing file or directory")
	}
	sel := &walkSel{prune: findPruned, badRel: "\n", pattern: q.pattern, types: q.fileType, times: times, maxDepth: -1}
	if x.all {
		sel.prune = nil
	}
	if sel.pattern == "*" {
		sel.pattern = "" // every name, the leading dot included (find -name)
	}
	if x.by == "type" && !x.typeSet {
		sel.types = "fdl"
	}
	if x.tracked {
		if sel.tracked, ok = m.loadTracked(c, q.safe, rootInfo.IsDir()); !ok {
			return 1
		}
	}
	q.x = x
	rootDir := rootInfo.IsDir()
	show := func(h findHit) string {
		if !q.o.Relative {
			return h.path
		}
		if h.rel != "" {
			return h.rel
		}
		if rootDir {
			return "."
		}
		return basename(q.safe)
	}
	if q.contains != "" {
		return m.findContentExt(c, q, sel, show)
	}
	if x.count {
		cnt := findCount(c, q.safe, rootInfo, sel, x.by, q.forbidden, m.guardedDirs(c))
		cnt.Write(c, q.o, q.o.Format)
		return 0
	}
	out := newFindSink(c, q, show)
	w := findWalker{maxDepth: -1, withStart: true, forbidden: q.forbidden}
	w.visit = func(e findEntry) bool {
		name := startName(e.path)
		if e.depth > 0 {
			name = e.path[strings.LastIndexByte(e.path, '/')+1:]
		}
		keep, enter := sel.keep(name, e.rel, e.typeLetter(), e.mtime)
		if keep && !out.add(findHit{path: e.path, rel: e.rel, typ: e.typeLetter(), size: e.size, mtime: e.mtime}) {
			w.stop = true
		}
		return enter
	}
	if out.wantsAll() || out.left > 0 {
		w.walk(q.safe)
	}
	out.close()
	return 0
}

// findContentExt is --contains with a 3.4 option: the 3.0 content search
// gives the files, the filters and the output stage are findExt's.
func (m *Module) findContentExt(c *invoke.Call, q findQuery, sel *walkSel, show func(findHit) string) int {
	if m.guard(c).IsSecretFile(q.safe) {
		return fail(c, "secret_file_blocked", q.safe, "This file contains secrets and cannot be searched")
	}
	globs := []string{q.pattern}
	if !q.x.all {
		globs = append(append(globs, noiseGlobs...), "!__pycache__")
	}
	var hits []findHit
	// A FILE PATH whose name PATTERN does not match gives no file, as in
	// 3.0 (before the pattern is read).
	if st, err := os.Stat(q.safe); err != nil || !st.Mode().IsRegular() ||
		pathguard.MatchGlobSegment(q.pattern, q.safe[strings.LastIndexByte(q.safe, '/')+1:]) {
		s, rc, ok := m.search(c, q.contains, globs, true)
		if !ok {
			return rc
		}
		s.filesOnly = true
		s.forbidden = q.forbidden
		s.hit = func(h grepHit) error {
			st, err := os.Lstat(h.path)
			if err != nil {
				return nil
			}
			rel := relBelow(q.safe, h.path)
			if sel.times.active() && !sel.times.ok(st.ModTime()) || sel.tracked != nil && !sel.tracked.has(rel, false) {
				return nil
			}
			hits = append(hits, findHit{path: h.path, rel: rel, typ: 'f', size: st.Size(), mtime: st.ModTime()})
			return nil
		}
		s.run(q.safe, io.Discard, maxLimit)
	}
	if q.x.count {
		cnt := llmfmt.Count{N: len(hits)}
		if q.x.by != "" {
			cnt.By = map[string]int{}
			for _, h := range hits {
				cnt.By[findKey(q.x.by, basename(h.path), h.rel, h.typ)]++
			}
		}
		cnt.Write(c, q.o, q.o.Format)
		return 0
	}
	out := newFindSink(c, q, show)
	for _, h := range hits {
		if !out.add(h) {
			break
		}
	}
	out.close()
	return 0
}

// findKey is the --by key of one selected entry.
func findKey(by, name, rel string, typ byte) string {
	switch by {
	case "ext":
		return extKey(name)
	case "dir":
		return dirKey(rel)
	}
	return typeKey(typ)
}

// findCount is --count of the name search: the walk of the listing, on
// several goroutines, without the per-entry lstat unless a filter needs
// the mtime. The count is the number of lines the same listing prints
// without -n.
func findCount(c *invoke.Call, root string, rootInfo os.FileInfo, sel *walkSel, by string, forbidden func(string) bool, guarded func(string) bool) llmfmt.Count {
	cnt := llmfmt.Count{}
	if by != "" {
		cnt.By = map[string]int{}
	}
	add := func(by string, n *int, keys map[string]int, name, rel string, typ byte) {
		*n++
		if keys != nil {
			keys[findKey(by, name, rel, typ)]++
		}
	}
	rtyp := dirEntryType(rootInfo.Mode())
	keep, enter := sel.keep(startName(root), "", rtyp, rootInfo.ModTime())
	if keep {
		add(by, &cnt.N, cnt.By, startName(root), "", rtyp)
	}
	if !enter || !rootInfo.IsDir() {
		return cnt
	}
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
	for _, t := range tallies {
		cnt.N += t.n
		for k, v := range t.keys {
			cnt.By[k] += v
		}
	}
	return cnt
}

// findSink is the output stage of a llm_find listing: text lines (paths)
// or rows {"p","size","mtime"} for --format, under the -n limit or the
// output budget, sorted by --sort.
type findSink struct {
	c    *invoke.Call
	q    findQuery
	show func(findHit) string
	ro   *llmfmt.Options // the budget the output is written with
	// rows: --format; sorted: --sort (everything is held, then sorted).
	rows, sorted bool
	held         []findHit
	keep         keeper[findHit]
	lw           *llmfmt.LineWriter
	// left is what the -n cut still lets through to the text output, the
	// 3.0 head -n; maxLimit when the budget or the rows decide.
	left int
}

func newFindSink(c *invoke.Call, q findQuery, show func(findHit) string) *findSink {
	s := &findSink{c: c, q: q, show: show, rows: q.o.Format != llmfmt.Default, sorted: q.x.sort != ""}
	s.ro = rowBudget(q.o, q.limit)
	s.left = maxLimit
	switch {
	case s.rows:
		s.keep = keeper[findHit]{head: s.ro.Head, tail: s.ro.Tail}
	case q.o.Capped():
		s.lw = q.o.Lines(c.Stdout, "lines")
	default:
		s.lw = q.o.Lines(c.Stdout, "lines")
		s.left = q.limit
	}
	return s
}

// wantsAll reports whether the output needs every selected path (a count
// of what was cut, or a sort) before it can print.
func (s *findSink) wantsAll() bool { return s.rows || s.sorted || s.q.o.Capped() }

// add takes one selected path; false ends the walk (the -n cut is full).
func (s *findSink) add(h findHit) bool {
	switch {
	case s.sorted:
		s.held = append(s.held, h)
	case s.rows:
		s.keep.add(h)
	default:
		if s.left <= 0 {
			return false
		}
		s.lw.Line(s.show(h))
		s.left--
		return s.left > 0
	}
	return true
}

// close sorts what was held and writes the output.
func (s *findSink) close() {
	if s.sorted {
		sortFindHits(s.held, s.q.x.sort)
		held := s.held
		s.sorted = false
		for _, h := range held {
			if !s.add(h) {
				break
			}
		}
	}
	if !s.rows {
		closeLines(s.c, s.lw)
		return
	}
	if s.q.limit == 0 && !s.q.o.Capped() {
		s.keep = keeper[findHit]{}
	}
	conv := func(h findHit) []any { return []any{s.show(h), h.size, fmtMtime(h.mtime)} }
	s.ro.Write(s.c, s.q.o.Format, llmfmt.Table{Cols: []string{"p", "size", "mtime"}, Rows: s.keep.rows(conv)}, nil)
}

// sortFindHits is --sort: name is the path in byte order, mtime newest
// first, size largest first; ties in path order.
func sortFindHits(hits []findHit, by string) {
	sort.SliceStable(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		switch by {
		case "mtime":
			if !a.mtime.Equal(b.mtime) {
				return a.mtime.After(b.mtime)
			}
		case "size":
			if a.size != b.size {
				return a.size > b.size
			}
		}
		return a.rel < b.rel
	})
}

// relBelow is path below root ("" for root itself), for a path a walk
// from root built.
func relBelow(root, path string) string {
	if path == root {
		return ""
	}
	prefix := root
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return strings.TrimPrefix(path, prefix)
}

// trackedSet is what git tracks below a walk's root (--tracked): the
// files by their path below the root, and every directory that holds one.
// A FILE root is "" when tracked.
type trackedSet struct {
	files, dirs map[string]bool
}

func (t *trackedSet) has(rel string, dir bool) bool {
	if dir {
		return t.dirs[rel]
	}
	return t.files[rel]
}

// loadTracked runs the hardened `git ls-files -z` in the root (or, for a
// FILE root, in its directory for that one name), through the module's
// runner. git missing is git_not_found, a root outside a repository
// not_a_git_repo (both CONTINUE).
func (m *Module) loadTracked(c *invoke.Call, root string, isDir bool) (*trackedSet, bool) {
	dir, name := root, ""
	if !isDir {
		dir, name = filepath.Dir(root), filepath.Base(root)
	}
	if !m.requireGitRepo(c, dir) {
		return nil, false
	}
	args := []string{"-C", dir, "ls-files", "-z", "--cached"}
	if name != "" {
		args = append(args, "--", ":(literal)"+name)
	}
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, nil, io.Discard, args...)
	if !res.Success() {
		fail(c, "not_a_git_repo", dir+" is not a git repository", "Not inside a git repository. Skip git operations")
		return nil, false
	}
	t := &trackedSet{files: map[string]bool{}, dirs: map[string]bool{}}
	for _, p := range bytes.Split(res.Stdout, []byte{0}) {
		if len(p) == 0 {
			continue
		}
		if !isDir {
			t.files[""] = true
			break
		}
		rel := string(p)
		t.files[rel] = true
		t.dirs[""] = true
		for i := 0; i < len(rel); i++ {
			if rel[i] == '/' {
				t.dirs[rel[:i]] = true
			}
		}
	}
	return t, true
}

// countDir counts the selected entries of one directory of a --count pool
// walk into n and keys, and returns the directories to enter.
func countDir(d walkDir, sel *walkSel, by string, n *int, keys map[string]int, forbidden, guarded func(string) bool) []walkDir {
	check := forbidden != nil && guarded(d.path)
	var subs []walkDir
	scanDir(d.path, sel.needInfo(), func(e scanEntry) {
		if e.failed || check && forbidden(findJoin(d.path, e.name)) {
			return
		}
		rel := relJoin(d.rel, e.name)
		keep, enter := sel.keep(e.name, rel, e.typ, e.mtime)
		if keep {
			*n++
			if keys != nil {
				keys[findKey(by, e.name, rel, e.typ)]++
			}
		}
		if enter && e.typ == 'd' && (sel.maxDepth < 0 || d.depth+1 < sel.maxDepth) {
			subs = append(subs, walkDir{path: findJoin(d.path, e.name), rel: rel, depth: d.depth + 1})
		}
	})
	return subs
}
