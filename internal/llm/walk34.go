// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"io/fs"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// The 3.4 modes of the walkers (plan F2-F5): --count [--by], --top, the
// --newer/--since filters, --tracked, rows for --format and the output
// budget. Every one is opt-in: without its flag a walker prints what it
// printed before, through its own 3.0 code.

// mtimeFilter is --newer REF (modified after REF, as find -newer) and
// --since T (modified at T or later); both may be set.
type mtimeFilter struct {
	newer, since       time.Time
	hasNewer, hasSince bool
}

func (f mtimeFilter) active() bool { return f.hasNewer || f.hasSince }

func (f mtimeFilter) ok(mt time.Time) bool {
	if f.hasNewer && !mt.After(f.newer) {
		return false
	}
	return !f.hasSince || !mt.Before(f.since)
}

// walkTimes resolves --newer REF and --since T of a walker call; ok is
// false when it wrote an error envelope.
func (m *Module) walkTimes(c *invoke.Call, newer, since string, haveNewer, haveSince bool) (mtimeFilter, bool) {
	var f mtimeFilter
	if haveNewer {
		ref, ok := m.validatePath(c, newer)
		if !ok {
			return f, false
		}
		st, err := os.Stat(ref)
		if err != nil {
			fail(c, "path_not_found", ref, "--newer needs an existing file or directory to compare with")
			return f, false
		}
		f.newer, f.hasNewer = st.ModTime(), true
	}
	if haveSince {
		t, ok := parseSince(since, time.Now(), envLocation(c))
		if !ok {
			fail(c, "invalid_since", since, "Give an age (30s, 15m, 2h, 3d, 1w), a date (2026-10-25), a date and time (2026-10-25T14:30, local time unless it carries a zone) or @EPOCH")
			return f, false
		}
		f.since, f.hasSince = t, true
	}
	return f, true
}

// fmtMtime is the mtime of a row: RFC 3339 in UTC, to the second, so the
// output does not depend on the caller's zone (plan R4).
func fmtMtime(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05Z")
}

// extKey is the --by ext key of a name, tree_stats' rule: a single
// leading dot dropped, the text after the last dot lowercased (ASCII),
// "(no ext)" without a dot.
func extKey(name string) string {
	name = strings.TrimPrefix(name, ".")
	dot := strings.LastIndexByte(name, '.')
	if name == "" || dot < 0 {
		return "(no ext)"
	}
	b := []byte(name[dot+1:])
	for i, ch := range b {
		if ch >= 'A' && ch <= 'Z' {
			b[i] = ch + 'a' - 'A'
		}
	}
	return string(b)
}

// dirKey is the --by dir key of an entry: the first directory below the
// walk's root, "." for an entry directly in it (and the root itself).
func dirKey(rel string) string {
	if i := strings.IndexByte(rel, '/'); i >= 0 {
		return rel[:i]
	}
	return "."
}

// typeKey is the --by type key: find's %y letter.
func typeKey(t byte) string { return string(t) }

// walkExcluded is the --exclude rule of the filesystem scanners (AGENTS.md
// 3.2, FS-12): GLOB matches the entry's base name or its path below the
// walk's root, with fnmatch semantics where '*', '?' and a bracket never
// match '/'.
func walkExcluded(globs []string, rel, name string) bool {
	for _, g := range globs {
		if globPathname(g, name) || globPathname(g, rel) {
			return true
		}
	}
	return false
}

// globPathname is fnmatch with FNM_PATHNAME: as many '/'-separated
// components in the pattern as in the name, each pair matching.
func globPathname(pat, name string) bool {
	if strings.Count(pat, "/") != strings.Count(name, "/") {
		return false
	}
	ps, ns := strings.Split(pat, "/"), strings.Split(name, "/")
	for i := range ps {
		if !pathguard.MatchGlobSegment(ps[i], ns[i]) {
			return false
		}
	}
	return true
}

// walkSel is what a 3.4 walk keeps: the filters llm_find and llm_tree
// share, applied the way their listings apply them.
type walkSel struct {
	// prune names entries neither kept nor entered (nil with --all).
	prune map[string]bool
	// badRel leaves out an entry whose path below the root holds one of
	// its characters, and everything below it ("\n" for llm_find, "\n\t"
	// for llm_tree, as their listings do).
	badRel string
	// pattern is the name glob; "" keeps every name.
	pattern string
	// types are the type letters kept ("f", "d", or "fdl").
	types string
	times mtimeFilter
	// tracked, when set, keeps only what git tracks (--tracked).
	tracked *trackedSet
	// maxDepth bounds the walk below the root; < 0 for none.
	maxDepth int
}

// keep reports whether one entry is selected and whether a directory is
// entered. name is the name the glob tests, rel the path below the root
// ("" for the root itself).
func (s *walkSel) keep(name, rel string, typ byte, mtime time.Time) (keep, enter bool) {
	if s.prune[name] {
		return false, false
	}
	if s.badRel != "" && strings.ContainsAny(rel, s.badRel) {
		return false, false
	}
	enter = true
	if typ == '?' || !strings.ContainsRune(s.types, rune(typ)) {
		return false, enter
	}
	if s.pattern != "" && !pathguard.MatchGlobSegment(s.pattern, name) {
		return false, enter
	}
	if s.times.active() && !s.times.ok(mtime) {
		return false, enter
	}
	if s.tracked != nil && !s.tracked.has(rel, typ == 'd') {
		return false, enter
	}
	return true, enter
}

// needInfo reports whether keep needs each entry's lstat (its mtime).
func (s *walkSel) needInfo() bool { return s.times.active() }

// dirEntryType is find's %y letter of a directory entry, from its type
// bits (no lstat).
func dirEntryType(m fs.FileMode) byte {
	switch {
	case m.IsDir():
		return 'd'
	case m&fs.ModeSymlink != 0:
		return 'l'
	case m.IsRegular():
		return 'f'
	}
	return '?'
}

// walkDir is one directory of a pool walk.
type walkDir struct {
	path, rel string
	depth     int // 0 for the root
}

// poolWalk visits a directory tree on up to jobs goroutines: visit reads
// one directory and returns the subdirectories to visit next; it gets the
// index of its worker (0 to jobs-1) for worker-local totals, so the walk
// takes no lock per entry. The order of the visits is not fixed: callers
// only sum. A cancelled ctx stops handing out directories.
func poolWalk(ctx context.Context, jobs int, root walkDir, visit func(w int, d walkDir) []walkDir) {
	jobs = max(jobs, 1)
	var mu sync.Mutex
	cond := sync.NewCond(&mu)
	stack := []walkDir{root}
	busy := 0
	var wg sync.WaitGroup
	for w := 0; w < jobs; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for {
				mu.Lock()
				for len(stack) == 0 && busy > 0 {
					cond.Wait()
				}
				if len(stack) == 0 {
					mu.Unlock()
					cond.Broadcast()
					return
				}
				d := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				busy++
				mu.Unlock()
				var subs []walkDir
				if ctx == nil || ctx.Err() == nil {
					subs = visit(w, d)
				}
				mu.Lock()
				busy--
				stack = append(stack, subs...)
				wake := len(subs) > 0 || busy == 0 && len(stack) == 0
				mu.Unlock()
				if wake {
					cond.Broadcast()
				}
			}
		}(w)
	}
	wg.Wait()
}

// scanEntry is one entry scanDir read: its name, find's %y letter, and
// the size and mtime of its lstat when it was asked for (failed: the
// lstat failed, the entry vanished).
type scanEntry struct {
	name   string
	typ    byte
	size   int64
	mtime  time.Time
	failed bool
}

// guardedDirs returns the per-directory form of the protected-tree test
// of a walk (walkRoot's forbidden): whether an entry of dir may be at or
// below a protected tree. Only the directories on the way to one need the
// per-entry test, which a walk of thousands of entries then skips.
func (m *Module) guardedDirs(c *invoke.Call) func(dir string) bool {
	trees := m.guard(c).Trees()
	// below reports whether a is b or below it, without building b+"/".
	below := func(a, b string) bool {
		return strings.HasPrefix(a, b) && (len(a) == len(b) || a[len(b)] == '/')
	}
	return func(dir string) bool {
		for _, cand := range [2]string{dir, strings.TrimPrefix(dir, "/private")} {
			for _, f := range trees {
				if below(f, cand) || below(cand, f) {
					return true
				}
			}
		}
		return false
	}
}

// relJoin is the path below the root of a child of rel.
func relJoin(rel, name string) string {
	if rel == "" {
		return name
	}
	return rel + "/" + name
}

// keeper holds what a line or row budget keeps of a stream: the first head
// items, a ring of the last tail ones, and the count of all. With neither
// set it keeps everything.
type keeper[T any] struct {
	head, tail int
	first      []T
	ring       []T
	start      int
	n          int
}

func (k *keeper[T]) add(v T) {
	k.n++
	if k.head == 0 && k.tail == 0 || len(k.first) < k.head {
		k.first = append(k.first, v)
		return
	}
	if k.tail == 0 {
		return
	}
	if len(k.ring) < k.tail {
		k.ring = append(k.ring, v)
		return
	}
	k.ring[k.start] = v
	k.start = (k.start + 1) % k.tail
}

// rows returns n rows with the kept items at the positions the same budget
// keeps (the first head, the last tail) and nil at the others: llmfmt's
// Cap reads only the kept positions and counts the rest as omitted.
func (k *keeper[T]) rows(conv func(T) []any) [][]any {
	out := make([][]any, k.n)
	for i, v := range k.first {
		out[i] = conv(v)
	}
	for i := range k.ring {
		out[k.n-len(k.ring)+i] = conv(k.ring[(k.start+i)%len(k.ring)])
	}
	return out
}

// rowBudget is the budget of a row or text output that has its own limit
// (-n): --head/--tail/--max-lines replace it when given, otherwise the
// first limit items are kept. It returns the Options to write with.
func rowBudget(o *llmfmt.Options, limit int) *llmfmt.Options {
	ro := *o
	if !ro.Capped() {
		ro.Head = limit
	}
	return &ro
}

// lineSink feeds the lines an rgSearch prints (one per Write) to a
// LineWriter.
type lineSink struct{ lw *llmfmt.LineWriter }

func (s lineSink) Write(b []byte) (int, error) {
	s.lw.Line(string(bytes.TrimSuffix(b, []byte("\n"))))
	return len(b), nil
}

// closeLines closes a LineWriter and notes what it cut on the call.
func closeLines(c *invoke.Call, lw *llmfmt.LineWriter) {
	if k := lw.Close(); k > 0 {
		c.NoteTruncated(k)
	}
}

// maxLimit stands for "no -n cut" in the modes that see every match.
const maxLimit = int(^uint(0) >> 1)
