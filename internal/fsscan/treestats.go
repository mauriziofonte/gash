// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// extCount is one extension's running totals during the tree_stats walk.
type extCount struct {
	ext   string // the extension, one value (FS-17)
	count int64
	size  int64
}

// baseName is awk's basename(): the last '/'-separated component of a
// relative path.
func baseName(rel string) string {
	if i := strings.LastIndexByte(rel, '/'); i >= 0 {
		return rel[i+1:]
	}
	return rel
}

// tolowerASCII lowercases A-Z only, as gawk's tolower does in the C locale
// (FS-28: non-ASCII bytes are left as they are).
func tolowerASCII(s string) string {
	var b []byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			if b == nil {
				b = []byte(s)
			}
			b[i] = c + 32
		}
	}
	if b == nil {
		return s
	}
	return string(b)
}

// extOf is the awk ext_of(): drop a single leading dot, then the text after
// the last dot, lowercased (ASCII); "(no ext)" when there is no dot, "" for a
// trailing dot.
func extOf(name string) string {
	if strings.HasPrefix(name, ".") {
		name = name[1:]
	}
	if name == "" {
		return "(no ext)"
	}
	dot := strings.LastIndexByte(name, '.')
	if dot < 0 {
		return "(no ext)"
	}
	return tolowerASCII(name[dot+1:])
}

func treeStats(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	jsonMode := jsonPrescan(c.Args, "--depth", "--max-depth", "--top", "--exclude")
	s := ui.StyleOf(c.Term, false)

	pa := pathArgs{dir: "."}
	var depthInput string
	topInput := "5"
	noIgnore, xdev, follow, allowRoot := false, false, false, false
	noColor := false
	var excludes []string

	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--depth", "--max-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			depthInput = v
		case "--top":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			topInput = v
		case "--exclude":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if e := excludeOK(v); e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			excludes = append(excludes, v)
		case "--no-ignore":
			noIgnore = true
		case "--xdev", "--one-filesystem":
			xdev = true
		case "--follow-symlinks", "-L":
			follow = true
		case "--allow-root":
			allowRoot = true
		case "--json":
			jsonMode = true
		case "--no-color":
			noColor = true
		case "-h", "--help":
			if help.NeedsOf(c, "--help") {
				return 0
			}
		case "--":
			if e := pa.rest(args[i+1:]); e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			i = len(args)
		default:
			if strings.HasPrefix(a, "-") {
				return errf("unknown_option", a, "Use --help to see valid flags").emit(c.Stderr, s, jsonMode)
			}
			if e := pa.take(a); e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
		}
	}

	maxDepth := -1
	if depthInput != "" {
		d, e := intFlag("invalid_depth", depthInput)
		if e != nil {
			return e.emit(c.Stderr, s, jsonMode)
		}
		maxDepth = d
	}
	topN, te := intFlag("invalid_top", topInput)
	if te != nil {
		return te.emit(c.Stderr, s, jsonMode)
	}

	home, pwd := callEnv(c)
	root, pe := safePath(pa.dir, home, pwd, allowRoot)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	elapsed := scanTimer()

	// One walk counts files, directories, sizes, extensions and empty
	// directories with the same prunes, excludes, -L and tab/newline rule,
	// so empty_dirs equals dirs_list_empty --count (FS-08, FS-16).
	res := findWalk(walkOpts{
		root: root, follow: follow, xdev: xdev, maxDepth: maxDepth,
		noIgnore: noIgnore, excludes: excludes, needEmpty: true,
	}, func(e entry) bool { return e.isDir || e.isRegular })
	if res.rootErr {
		return rootUnreadable(pa.dir, home).emit(c.Stderr, s, jsonMode)
	}
	var filesTotal, dirsTotal, sizeTotal, maxDepthVal int64
	exts := map[string]*extCount{}
	emptyCount := 0
	for _, e := range res.entries {
		if d := int64(strings.Count(e.rel, "/")); d > maxDepthVal {
			maxDepthVal = d
		}
		switch {
		case e.isRegular:
			filesTotal++
			sizeTotal += e.size
			key := extOf(baseName(e.rel))
			ec := exts[key]
			if ec == nil {
				ec = &extCount{ext: key}
				exts[key] = ec
			}
			ec.count++
			ec.size += e.size
		case e.isDir:
			dirsTotal++
			if e.empty {
				emptyCount++
			}
		}
	}
	var avg int64
	if filesTotal > 0 {
		avg = sizeTotal / filesTotal
	}
	topCount, topSize := topExtensions(exts, topN)
	st := elapsed()
	errCount := res.errors

	if jsonMode {
		renderTreeJSON(c.Stdout, root, filesTotal, dirsTotal, sizeTotal, avg,
			maxDepthVal, emptyCount, topCount, topSize, st, errCount)
		return 0
	}

	s = ui.StyleOf(c.Term, noColor)
	if s.Color {
		renderTreeRich(c.Stdout, s, root, filesTotal, dirsTotal, sizeTotal, avg,
			maxDepthVal, emptyCount, topN, topCount, topSize, st)
		scanWarn(c.Stderr, s, errCount, false)
		return 0
	}
	renderTreePlain(c.Stdout, s, root, filesTotal, dirsTotal, sizeTotal, avg,
		maxDepthVal, emptyCount, topN, topCount, topSize, st)
	scanWarn(c.Stderr, s, errCount, false)
	return 0
}

// topExtensions orders the extension map the way FS-28 pins: top_by_count by
// count desc, size desc, extension asc (byte order); top_by_size by size
// desc, count desc, extension asc, excluding zero-size extensions.
func topExtensions(exts map[string]*extCount, topN int) (byCount, bySize []extCount) {
	all := make([]extCount, 0, len(exts))
	for _, e := range exts {
		all = append(all, *e)
	}
	cnt := append([]extCount(nil), all...)
	sort.SliceStable(cnt, func(i, j int) bool {
		if cnt[i].count != cnt[j].count {
			return cnt[i].count > cnt[j].count
		}
		if cnt[i].size != cnt[j].size {
			return cnt[i].size > cnt[j].size
		}
		return cnt[i].ext < cnt[j].ext
	})
	if topN < len(cnt) {
		cnt = cnt[:topN]
	}

	var sz []extCount
	for _, e := range all {
		if e.size > 0 {
			sz = append(sz, e)
		}
	}
	sort.SliceStable(sz, func(i, j int) bool {
		if sz[i].size != sz[j].size {
			return sz[i].size > sz[j].size
		}
		if sz[i].count != sz[j].count {
			return sz[i].count > sz[j].count
		}
		return sz[i].ext < sz[j].ext
	})
	if topN < len(sz) {
		sz = sz[:topN]
	}
	return cnt, sz
}

// renderTreeJSON writes the one-line tree_stats envelope. Each extension is
// one value, encoded with the contract escaper (FS-17).
func renderTreeJSON(w io.Writer, root string, files, dirs, size, avg, maxDepth int64,
	empty int, byCount, bySize []extCount, scanTime string, errCount int) {
	var b strings.Builder
	b.WriteString(`{"data":{"path":"`)
	b.WriteString(escFull(root))
	b.WriteString(`","files_total":`)
	b.WriteString(strconv.FormatInt(files, 10))
	b.WriteString(`,"dirs_total":`)
	b.WriteString(strconv.FormatInt(dirs, 10))
	b.WriteString(`,"size_total":`)
	b.WriteString(strconv.FormatInt(size, 10))
	b.WriteString(`,"size_total_human":"`)
	b.WriteString(ui.HumanBytes(size))
	b.WriteString(`","avg_file_size":`)
	b.WriteString(strconv.FormatInt(avg, 10))
	b.WriteString(`,"max_depth":`)
	b.WriteString(strconv.FormatInt(maxDepth, 10))
	b.WriteString(`,"empty_dirs":`)
	b.WriteString(strconv.Itoa(empty))
	b.WriteString(`,"top_by_count":[`)
	for i, e := range byCount {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"ext":"` + escFull(e.ext) + `","count":` + strconv.FormatInt(e.count, 10) +
			`,"size":` + strconv.FormatInt(e.size, 10) + `}`)
	}
	b.WriteString(`],"top_by_size":[`)
	for i, e := range bySize {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(`{"ext":"` + escFull(e.ext) + `","size":` + strconv.FormatInt(e.size, 10) + `}`)
	}
	b.WriteString(`]},"scan_time":"` + scanTime + `","errors":` + strconv.Itoa(errCount) + "}\n")
	_, _ = io.WriteString(w, b.String())
}

// renderTreePlain is the non-color tree_stats layout: a heading, the KV
// summary, then the top extensions by count and by size.
func renderTreePlain(w io.Writer, s ui.Style, root string, files, dirs, size, avg, maxDepth int64,
	empty int, topN int, byCount, bySize []extCount, scanTime string) {
	s.Heading(w, "Tree statistics for "+root)
	s.KV(w, "--indent", "2",
		"Files:", strconv.FormatInt(files, 10),
		"Directories:", strconv.FormatInt(dirs, 10),
		"Total size:", ui.HumanBytes(size)+" ("+strconv.FormatInt(size, 10)+" bytes)",
		"Avg file:", ui.HumanBytes(avg),
		"Max depth:", strconv.FormatInt(maxDepth, 10),
		"Empty dirs:", strconv.Itoa(empty),
		"Scan time:", scanTime)

	if len(byCount) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "Top "+strconv.Itoa(topN)+" extensions by count:")
		rows := make([]string, 0, len(byCount))
		for _, e := range byCount {
			rows = append(rows, e.ext+"\t"+strconv.FormatInt(e.count, 10)+"\tfiles\t"+ui.HumanBytes(e.size))
		}
		s.Table(w, rows, "--no-header", "--indent", "4", "--gap", "1",
			"--min", "1:12", "--right", "2", "--min", "2:6", "--min", "3:6", "--color", "1:accent")
	}
	if len(bySize) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "Top "+strconv.Itoa(topN)+" extensions by size:")
		rows := make([]string, 0, len(bySize))
		for _, e := range bySize {
			rows = append(rows, e.ext+"\t"+ui.HumanBytes(e.size))
		}
		s.Table(w, rows, "--no-header", "--indent", "4", "--gap", "1", "--min", "1:12", "--color", "1:accent")
	}
}

// renderTreeRich is __gash_fs_tree_stats_rich: the summary grid, then the top
// extensions by size and by count with share bars. Counts and sizes are
// integers end to end: no text from a file name is ever evaluated (FS-50).
func renderTreeRich(w io.Writer, s ui.Style, root string, files, dirs, size, avg, maxDepth int64,
	empty int, topN int, byCount, bySize []extCount, scanTime string) {
	_, _ = io.WriteString(w, "\n")
	s.Heading(w, "--indent", "2", "Tree statistics for "+root)
	_, _ = io.WriteString(w, "\n")
	grid := []string{
		"Files\t" + ui.GroupDigits(strconv.FormatInt(files, 10)) +
			"\tTotal size\t" + ui.HumanBytes(size) + " (" + ui.GroupDigits(strconv.FormatInt(size, 10)) + " bytes)",
		"Directories\t" + ui.GroupDigits(strconv.FormatInt(dirs, 10)) +
			"\tAvg file\t" + ui.HumanBytes(avg),
		"Empty dirs\t" + ui.GroupDigits(strconv.Itoa(empty)) +
			"\tMax depth\t" + strconv.FormatInt(maxDepth, 10),
	}
	s.Table(w, grid, "--no-header", "--indent", "2", "--min", "1:12", "--min", "2:10", "--color", "1:bold", "--color", "3:bold")

	width := 8
	for _, e := range append(append([]extCount{}, bySize...), byCount...) {
		if ui.Len(e.ext) > width {
			width = ui.Len(e.ext)
		}
	}
	topts := []string{"--no-header", "--indent", "2", "--min", "1:" + strconv.Itoa(width),
		"--right", "2,4", "--min", "2:10", "--bar", "3:20", "--min", "4:6",
		"--color", "1:accent", "--color", "4:muted"}

	if len(bySize) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "By size")
		rows := make([]string, 0, len(bySize))
		for _, e := range bySize {
			rows = append(rows, e.ext+"\t"+ui.HumanBytes(e.size)+"\t"+
				strconv.FormatInt(e.size, 10)+"/"+strconv.FormatInt(size, 10)+"\t"+pctMille(e.size, size))
		}
		s.Table(w, rows, topts...)
	}
	if len(byCount) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "By count")
		rows := make([]string, 0, len(byCount))
		for _, e := range byCount {
			cnt := strconv.FormatInt(e.count, 10)
			rows = append(rows, e.ext+"\t"+ui.GroupDigits(cnt)+"\t"+
				cnt+"/"+strconv.FormatInt(files, 10)+"\t"+pctMille(e.count, files))
		}
		s.Table(w, rows, topts...)
	}
	_, _ = io.WriteString(w, "\n")
	s.Print(w, "muted", "  Scanned in "+scanTime)
	_, _ = io.WriteString(w, "\n")
}

// pctMille formats n/total as the 2.x `%d.%d%%` of (n*1000/total): one
// decimal of a per-mille ratio, "0.0%" when total is 0.
func pctMille(n, total int64) string {
	if total <= 0 {
		return "0.0%"
	}
	p := n * 1000 / total
	return strconv.FormatInt(p/10, 10) + "." + strconv.FormatInt(p%10, 10) + "%"
}
