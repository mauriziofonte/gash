// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/ui"
)

// extCount is one extension's running totals during the tree_stats walk.
type extCount struct {
	ext   string // the full extension as computed (may contain a tab, FS-17)
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
	jsonMode := jsonPrescan(c.Args)
	s := ui.StyleOf(c.Term, false)

	dir := "."
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
			if i+1 < len(args) {
				dir = args[i+1]
			}
			i = len(args)
		default:
			if strings.HasPrefix(a, "-") {
				return errf("unknown_option", a, "Use --help to see valid flags").emit(c.Stderr, s, jsonMode)
			}
			dir = a
		}
	}

	maxDepth := -1
	depthOverflow := false
	if depthInput != "" {
		if !isUint(depthInput) {
			return errf("invalid_depth", depthInput, "Provide a non-negative integer").emit(c.Stderr, s, jsonMode)
		}
		d, err := strconv.ParseInt(depthInput, 10, 64)
		if err != nil || d > findDepthMax {
			depthOverflow = true
		} else {
			maxDepth = int(d)
		}
	}
	if !isUint(topInput) {
		return errf("invalid_top", topInput, "Provide a positive integer").emit(c.Stderr, s, jsonMode)
	}
	topN := int(limitValue(topInput))

	home, pwd := callEnv(c)
	root, pe := safePath(dir, home, pwd, allowRoot)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	elapsed := scanTimer()

	var filesTotal, dirsTotal, sizeTotal, maxDepthVal int64
	exts := map[string]*extCount{}
	errCount := 0
	emptyCount := 0
	if depthOverflow {
		errCount = 1 // find refused the -maxdepth argument (FS-14)
	} else {
		res := findWalk(walkOpts{
			root: root, follow: follow, xdev: xdev, maxDepth: maxDepth,
			noIgnore: noIgnore, excludes: excludes, skipNL: true,
		}, func(e entry) bool { return e.isDir || e.isRegular })
		errCount = res.errors
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
			}
		}
		emptyCount = treeEmptyCount(walkOpts{
			root: root, xdev: xdev, maxDepth: maxDepth,
			noIgnore: noIgnore, excludes: excludes,
		})
	}
	var avg int64
	if filesTotal > 0 {
		avg = sizeTotal / filesTotal
	}
	topCount, topSize := topExtensions(exts, topN)
	st := elapsed()

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

// treeEmptyCount reproduces the second find of tree_stats:
// `find ROOT [-xdev] [-maxdepth N] PRUNES -type d -empty | wc -l`, with no
// -L, no newline test and find's implicit -print of every pruned entry and
// every --exclude match, files included (FS-16, FS-08). The count is the
// number of stderr-free output lines, so a printed path with an embedded
// newline adds a line. Walk errors are discarded (sent to /dev/null).
func treeEmptyCount(o walkOpts) int {
	pruneNames := map[string]bool{}
	if !o.noIgnore {
		for _, n := range defaultPrune {
			pruneNames[n] = true
		}
	}
	var rootDev uint64
	if o.xdev {
		if st, err := os.Stat(o.root); err == nil {
			rootDev = deviceOf(st)
		}
	}
	lines := 0
	printEntry := func(abs string) { lines += 1 + strings.Count(abs, "\n") }

	var visit func(abs string, depth int)
	visit = func(abs string, depth int) {
		info, err := os.Lstat(abs)
		if err != nil {
			return
		}
		isDir := info.IsDir()
		if o.follow && info.Mode()&os.ModeSymlink != 0 {
			if st, e := os.Stat(abs); e == nil {
				isDir = st.IsDir()
			}
		}
		// Prune clause: a default-prune directory, or any --exclude -path
		// match (files too), is printed and not descended.
		if isDir && pruneNames[filepath.Base(abs)] {
			printEntry(abs)
			return
		}
		for _, ex := range o.excludes {
			if ex != "" && pathguard.MatchGlobSegment(ex, abs) {
				printEntry(abs)
				return
			}
		}
		if isDir && isEmptyDir(abs) {
			printEntry(abs)
		}
		if !isDir {
			return
		}
		if o.maxDepth >= 0 && depth >= o.maxDepth {
			return
		}
		names, err := readDirNames(abs)
		if err != nil {
			return
		}
		for _, name := range names {
			child := filepath.Join(abs, name)
			ci, cerr := os.Lstat(child)
			if cerr != nil {
				continue
			}
			if o.xdev && ci.IsDir() && deviceOf(ci) != rootDev {
				continue
			}
			visit(child, depth+1)
		}
	}
	visit(o.root, 0)
	return lines
}

// renderTreeJSON writes the one-line tree_stats envelope. A tab inside an
// extension is split into ext, count and size exactly as the 2.x
// `IFS=$'\t' read -r ext cnt sz` did, so such a row stays byte-for-byte (and
// invalid JSON) as in 2.x (FS-17).
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
		ext, cnt, sz := splitExt3(e.ext, e.count, e.size)
		b.WriteString(`{"ext":"` + escFull(ext) + `","count":` + cnt + `,"size":` + sz + `}`)
	}
	b.WriteString(`],"top_by_size":[`)
	for i, e := range bySize {
		if i > 0 {
			b.WriteByte(',')
		}
		ext, sz := splitExt2(e.ext, e.size)
		b.WriteString(`{"ext":"` + escFull(ext) + `","size":` + sz + `}`)
	}
	b.WriteString(`]},"scan_time":"` + scanTime + `","errors":` + strconv.Itoa(errCount) + "}\n")
	_, _ = io.WriteString(w, b.String())
}

// splitExt3 emulates `IFS=$'\t' read -r ext cnt sz` over the TSV row
// `EXT\tCOUNT\tSIZE`: ext is the first tab field, cnt the second, sz the
// rest; an empty field becomes "0" (the shell's `${v:-0}`).
func splitExt3(ext string, count, size int64) (string, string, string) {
	row := ext + "\t" + strconv.FormatInt(count, 10) + "\t" + strconv.FormatInt(size, 10)
	f := strings.SplitN(row, "\t", 3)
	e, cnt, sz := f[0], "", ""
	if len(f) > 1 {
		cnt = f[1]
	}
	if len(f) > 2 {
		sz = f[2]
	}
	return e, zeroDefault(cnt), zeroDefault(sz)
}

// splitExt2 emulates `IFS=$'\t' read -r ext sz` over `EXT\tSIZE`.
func splitExt2(ext string, size int64) (string, string) {
	row := ext + "\t" + strconv.FormatInt(size, 10)
	f := strings.SplitN(row, "\t", 2)
	e, sz := f[0], ""
	if len(f) > 1 {
		sz = f[1]
	}
	return e, zeroDefault(sz)
}

func zeroDefault(s string) string {
	if s == "" {
		return "0"
	}
	return s
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
			ext, cnt, sz := splitExt3(e.ext, e.count, e.size)
			rows = append(rows, ext+"\t"+cnt+"\tfiles\t"+ui.HumanSize(sz))
		}
		s.Table(w, rows, "--no-header", "--indent", "4", "--gap", "1",
			"--min", "1:12", "--right", "2", "--min", "2:6", "--min", "3:6", "--color", "1:accent")
	}
	if len(bySize) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "Top "+strconv.Itoa(topN)+" extensions by size:")
		rows := make([]string, 0, len(bySize))
		for _, e := range bySize {
			ext, sz := splitExt2(e.ext, e.size)
			rows = append(rows, ext+"\t"+ui.HumanSize(sz))
		}
		s.Table(w, rows, "--no-header", "--indent", "4", "--gap", "1", "--min", "1:12", "--color", "1:accent")
	}
}

// renderTreeRich is __gash_fs_tree_stats_rich: the summary grid, then the top
// extensions by size and by count with share bars. The percentage uses
// integer handling (never Bash arithmetic on a tab-broken field).
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
		ext, _ := splitExt2(e.ext, e.size)
		if ui.Len(ext) > width {
			width = ui.Len(ext)
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
			ext, szStr := splitExt2(e.ext, e.size)
			sz := int64(ui.Int(szStr))
			rows = append(rows, ext+"\t"+ui.HumanSize(szStr)+"\t"+
				strconv.FormatInt(sz, 10)+"/"+strconv.FormatInt(size, 10)+"\t"+pctMille(sz, size))
		}
		s.Table(w, rows, topts...)
	}
	if len(byCount) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "By count")
		rows := make([]string, 0, len(byCount))
		for _, e := range byCount {
			ext, cntStr, _ := splitExt3(e.ext, e.count, e.size)
			cnt := int64(ui.Int(cntStr))
			rows = append(rows, ext+"\t"+ui.GroupDigits(cntStr)+"\t"+
				cntStr+"/"+strconv.FormatInt(files, 10)+"\t"+pctMille(cnt, files))
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
