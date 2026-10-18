// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package fsscan ports the five filesystem scanners of lib/modules/files.sh
// (files_largest, dirs_largest, dirs_find_large, dirs_list_empty,
// tree_stats). The walks, sizes, counts and empty-directory tests are
// native (architecture.md 7), so the result never depends on which find,
// du or awk is installed; the GNU-path 2.x output is reproduced, defects
// included (LEDGER FS-*).
package fsscan

import (
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Register binds the five scanners to t.
func Register(t *invoke.Table) {
	t.Handle("files_largest", filesLargest)
	t.Handle("dirs_largest", dirsLargest)
	t.Handle("dirs_find_large", dirsFindLarge)
	t.Handle("dirs_list_empty", dirsListEmpty)
	t.Handle("tree_stats", treeStats)
}

// findDepthMax is INT_MAX: a find -maxdepth above it makes GNU find refuse
// its arguments, which 2.x reported as an empty success with errors 1
// (FS-14).
const findDepthMax = 2147483647

// jsonPrescan reproduces the 2.x pre-scan that turns on JSON mode when any
// word before "--" equals "--json", flag values included (FS-19).
func jsonPrescan(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" {
			return true
		}
	}
	return false
}

// isUint reports [[ s =~ ^[0-9]+$ ]] (ASCII digits, any length). A
// non-ASCII digit of a UTF-8 locale never matches (FS-29).
func isUint(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// limitValue is an awk NR<=n cap: a run of digits of any length read in
// base 10 (so "010" is 10), clamped to the platform int (FS-05).
func limitValue(s string) int {
	n, err := strconv.ParseInt(strings.TrimLeft(s, "0"), 10, 64)
	if err != nil || n > math.MaxInt32<<8 {
		if len(strings.TrimLeft(s, "0")) == 0 {
			return 0
		}
		return math.MaxInt32 << 8
	}
	return int(n)
}

// human is the tri-state of --human/--no-human: auto until set.
type humanMode int

const (
	humanAuto humanMode = iota
	humanOff
	humanOn
)

func (h humanMode) resolve(json, null bool) bool {
	switch h {
	case humanOn:
		return true
	case humanOff:
		return false
	default:
		return !(json || null)
	}
}

// missingValue is the error of a value flag at the end of the line.
func missingValue(flag string) *fsError {
	return errf("missing_value", flag, flag+" needs a value")
}

// excludeOK refuses an --exclude value with a newline (FS-20): Go never
// re-parses a user string into find predicates (MUST NOT 7).
func excludeOK(v string) *fsError {
	if strings.ContainsRune(v, '\n') {
		return errf("invalid_filter", v, "--exclude takes a glob without newlines, e.g. '*/node_modules'")
	}
	return nil
}

// callEnv reads HOME and PWD the way the stub exported them.
func callEnv(c *invoke.Call) (home, pwd string) {
	return c.Env.Get("HOME"), c.Env.Get("PWD")
}

// scanTimer returns a function that reports the elapsed time as 2.x
// formatted it (__gash_fs_format_elapsed).
func scanTimer() func() string {
	t0 := nowClock()
	return func() string { return formatElapsed(nowClock().Sub(t0).Milliseconds()) }
}

// --- files_largest ---------------------------------------------------------

func filesLargest(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	// The --json pre-scan sets the mode before the loop and is never reset,
	// so a --json seen as a flag value still turns on JSON output and the
	// JSON error contract (FS-19, reproduced).
	jsonMode := jsonPrescan(c.Args)
	s := ui.StyleOf(c.Term, false)

	dir := "."
	limit := 100
	var minInput, depthInput string
	noIgnore, xdev, follow, allowRoot := false, false, false, false
	nullMode, noColor := false, false
	hm := humanAuto
	var excludes []string

	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-n", "--limit":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if !isUint(v) {
				return errf("invalid_limit", v, "Provide a positive integer").emit(c.Stderr, s, jsonMode)
			}
			limit = limitValue(v)
		case "--min-size":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			minInput = v
		case "--exclude":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if e := excludeOK(v); e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			excludes = append(excludes, v)
		case "--depth", "--max-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			depthInput = v
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
		case "--null", "-0":
			nullMode = true
		case "--human":
			hm = humanOn
		case "--no-human":
			hm = humanOff
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

	var minBytes int64
	if minInput != "" {
		sz, ok := ParseSize(minInput)
		if !ok || sz.Wrapped {
			return errf("invalid_size", minInput, "Use a size like 10M, 1G, 500K").emit(c.Stderr, s, jsonMode)
		}
		minBytes = sz.Bytes
	}
	maxDepth := -1
	depthOverflow := false
	if depthInput != "" {
		if !isUint(depthInput) {
			return errf("invalid_depth", depthInput, "Provide a non-negative integer").emit(c.Stderr, s, jsonMode)
		}
		d, _ := strconv.ParseInt(depthInput, 10, 64)
		if d > findDepthMax {
			depthOverflow = true
		} else {
			maxDepth = int(d)
		}
	}

	home, pwd := callEnv(c)
	root, fe := safePath(dir, home, pwd, allowRoot)
	if fe != nil {
		return fe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	var rows []sizeRow
	errCount := 0
	if depthOverflow {
		errCount = 1 // find refused the argument (FS-14)
	} else {
		res := findWalk(walkOpts{
			root: root, follow: follow, xdev: xdev, maxDepth: maxDepth,
			noIgnore: noIgnore, excludes: excludes, skipTab: true, skipNL: true,
		}, func(e entry) bool { return e.isRegular && e.size >= minBytes })
		errCount = res.errors
		for _, e := range res.entries {
			rows = append(rows, sizeRow{bytes: e.size, mtime: mtimeFmt(e.mtime), path: e.abs})
		}
		sortSizeRows(rows)
		if limit < len(rows) {
			rows = rows[:limit]
		}
	}
	st := elapsed()

	switch {
	case jsonMode:
		renderSizeJSON(c.Stdout, rows, st, errCount, "files")
	case nullMode:
		renderSizeNull(c.Stdout, rows)
	default:
		s = ui.StyleOf(c.Term, noColor)
		renderSizeText(c.Stdout, s, rows, human, "files", st, root, "")
		scanWarn(c.Stderr, s, errCount, false)
	}
	return 0
}

// next advances i past a value flag and returns its value, as `shift 2`.
func next(args []string, i *int) (string, bool) {
	if *i+1 >= len(args) {
		*i = len(args)
		return "", false
	}
	*i++
	return args[*i], true
}

// mtimeFmt is find's %TY-%Tm-%Td %TH:%TM in local time, minute precision.
func mtimeFmt(t interface{ Format(string) string }) string {
	return t.Format("2006-01-02 15:04")
}

// isEmptyDir reports `find -type d -empty`: a directory that reads back no
// entries. An unreadable directory is not empty (find would report an error,
// not an empty hit).
func isEmptyDir(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if err != nil && err != io.EOF {
		return false
	}
	return len(names) == 0
}

// parseDuDepth validates a du -d depth under the FS-05/FS-21 rule: ASCII
// digits only, base 10, no leading zero except "0" itself, at most
// 9223372036854775807. A rejected value gets invalid_depth with the
// scanner's 2.x hint. Those meanings 2.x took from Bash arithmetic (octal,
// base errors, 64-bit wrap) are a forced divergence; Go never evaluates a
// user string as arithmetic.
func parseDuDepth(s, hint string) (int64, *fsError) {
	if !isUint(s) {
		return 0, errf("invalid_depth", s, hint)
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, errf("invalid_depth", s, hint)
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, errf("invalid_depth", s, hint)
	}
	return n, nil
}

// substrExcludes reproduces the awk post-filter of dirs_largest and
// dirs_find_large, which joined the --exclude values with '|' and split them
// again, then dropped any path that contains one as a substring (FS-12: the
// du globs filter the walk, these substrings filter the surviving list).
func substrExcludes(excludes []string) []string {
	joined := strings.Join(excludes, "|")
	var out []string
	for _, p := range strings.Split(joined, "|") {
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func pathHasSubstr(subs []string, abs string) bool {
	for _, s := range subs {
		if strings.Contains(abs, s) {
			return true
		}
	}
	return false
}

// newestFileMtime is `find DIR -maxdepth 10 -type f -printf '%T@\t%TY-..' |
// sort -nr | head -1`: the newest regular file within ten levels of dir,
// formatted in local time, or "N/A" when there is none. No prunes, no -xdev,
// symlinks not followed (find -type f without -L).
func newestFileMtime(dir string) string {
	var best time.Time
	found := false
	var walk func(abs string, depth int)
	walk = func(abs string, depth int) {
		names, err := readDirNames(abs)
		if err != nil {
			return
		}
		for _, name := range names {
			child := filepath.Join(abs, name)
			info, err := os.Lstat(child)
			if err != nil {
				continue
			}
			if info.Mode().IsRegular() {
				if !found || info.ModTime().After(best) {
					best, found = info.ModTime(), true
				}
			} else if info.IsDir() && depth < 10 {
				walk(child, depth+1)
			}
		}
	}
	walk(dir, 1)
	if !found {
		return "N/A"
	}
	return best.Format("2006-01-02 15:04")
}

// --- dirs_largest ----------------------------------------------------------

func dirsLargest(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	jsonMode := jsonPrescan(c.Args)
	s := ui.StyleOf(c.Term, false)

	dir := "."
	limit := 100
	depthInput := "1"
	var minInput string
	noIgnore, xdev, allowRoot := false, false, false
	nullMode, noColor := false, false
	hm := humanAuto
	var excludes []string

	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "-n", "--limit":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if !isUint(v) {
				return errf("invalid_limit", v, "Provide a positive integer").emit(c.Stderr, s, jsonMode)
			}
			limit = limitValue(v)
		case "--depth", "--max-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			depthInput = v
		case "--min-size":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			minInput = v
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
		case "--allow-root":
			allowRoot = true
		case "--json":
			jsonMode = true
		case "--null", "-0":
			nullMode = true
		case "--human":
			hm = humanOn
		case "--no-human":
			hm = humanOff
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

	depth, fe := parseDuDepth(depthInput, "Provide a positive integer")
	if fe != nil {
		return fe.emit(c.Stderr, s, jsonMode)
	}
	if depth < 1 {
		depth = 1
	}
	var minBytes int64
	if minInput != "" {
		sz, ok := ParseSize(minInput)
		if !ok || sz.Wrapped {
			return errf("invalid_size", minInput, "Use a size like 10M, 1G").emit(c.Stderr, s, jsonMode)
		}
		minBytes = sz.Bytes
	}

	home, pwd := callEnv(c)
	root, pe := safePath(dir, home, pwd, allowRoot)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	res := duWalk(duOpts{root: root, xdev: xdev, maxDepth: int(depth), noIgnore: noIgnore, excludes: excludes})
	subs := substrExcludes(excludes)
	var rows []sizeRow
	for _, d := range res.dirs {
		if d.abs == root { // drop the root operand (du prints it last)
			continue
		}
		if pathHasSubstr(subs, d.abs) {
			continue
		}
		bytes := d.kib * 1024
		if bytes < minBytes {
			continue
		}
		rows = append(rows, sizeRow{bytes: bytes, path: d.abs})
	}
	sortSizeRows(rows)
	if limit < len(rows) {
		rows = rows[:limit]
	}
	st := elapsed()

	switch {
	case jsonMode:
		renderSizeJSON(c.Stdout, rows, st, res.errors, "directories")
	case nullMode:
		renderSizeNull(c.Stdout, rows)
	default:
		s = ui.StyleOf(c.Term, noColor)
		renderSizeText(c.Stdout, s, rows, human, "directories", st, root, "")
		scanWarn(c.Stderr, s, res.errors, false)
	}
	return 0
}

// --- dirs_find_large -------------------------------------------------------

func dirsFindLarge(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	jsonMode := jsonPrescan(c.Args)
	s := ui.StyleOf(c.Term, false)

	dir := "."
	sizeThreshold := "20M"
	depthInput := ""
	limit := 1000
	noIgnore, xdev, allowRoot := false, false, false
	nullMode, noColor, withMtime := false, false, false
	hm := humanAuto
	var excludes []string

	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--size":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			sizeThreshold = v
		case "--depth", "--max-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			depthInput = v
		case "-n", "--limit":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if !isUint(v) {
				return errf("invalid_limit", v, "Provide a positive integer").emit(c.Stderr, s, jsonMode)
			}
			limit = limitValue(v)
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
		case "--allow-root":
			allowRoot = true
		case "--with-mtime":
			withMtime = true
		case "--json":
			jsonMode = true
		case "--null", "-0":
			nullMode = true
		case "--human":
			hm = humanOn
		case "--no-human":
			hm = humanOff
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

	// Order matches 2.x: size, then limit, then depth.
	sz, ok := ParseSize(sizeThreshold)
	if !ok || sz.Wrapped {
		return errf("invalid_size", sizeThreshold, "Use a size like 20M, 1G, 500K").emit(c.Stderr, s, jsonMode)
	}
	if sz.Bytes > maxInt64-1023 { // (bytes+1023)/1024 would overflow (FS-22)
		return errf("invalid_size", sizeThreshold, "Use a size like 20M, 1G, 500K").emit(c.Stderr, s, jsonMode)
	}
	thresholdKiB := (sz.Bytes + 1023) / 1024
	maxDepth := -1
	if depthInput != "" {
		depth, fe := parseDuDepth(depthInput, "Provide a non-negative integer")
		if fe != nil {
			return fe.emit(c.Stderr, s, jsonMode)
		}
		if depth > 0 { // 0 (and no flag) means unbounded
			maxDepth = int(depth)
		}
	}

	home, pwd := callEnv(c)
	root, pe := safePath(dir, home, pwd, allowRoot)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	res := duWalk(duOpts{root: root, xdev: xdev, maxDepth: maxDepth, noIgnore: noIgnore, excludes: excludes})
	subs := substrExcludes(excludes)
	var rows []sizeRow
	for _, d := range res.dirs {
		if d.abs == root {
			continue
		}
		if d.kib < thresholdKiB {
			continue
		}
		if pathHasSubstr(subs, d.abs) {
			continue
		}
		rows = append(rows, sizeRow{bytes: d.kib * 1024, path: d.abs})
	}
	// 2.x sorts the `bytes\tpath` lines (no mtime yet), ties by the whole
	// line in C byte order descending, then caps at the limit; --with-mtime
	// enrichment happens afterwards (so mtime is never part of the sort key).
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].bytes != rows[j].bytes {
			return rows[i].bytes > rows[j].bytes
		}
		li := strconv.FormatInt(rows[i].bytes, 10) + "\t" + rows[i].path
		lj := strconv.FormatInt(rows[j].bytes, 10) + "\t" + rows[j].path
		return li > lj
	})
	if limit < len(rows) {
		rows = rows[:limit]
	}
	// Enrich / truncate: with --with-mtime keep the full path and the newest
	// file's time; without it the path is cut at its first tab (FS-09).
	for i := range rows {
		if withMtime {
			rows[i].mtime = newestFileMtime(rows[i].path)
		} else if idx := strings.IndexByte(rows[i].path, '\t'); idx >= 0 {
			rows[i].path = rows[i].path[:idx]
		}
	}
	st := elapsed()

	title := "Directories over " + sizeThreshold + " in " + root
	switch {
	case jsonMode:
		renderSizeJSON(c.Stdout, rows, st, res.errors, "directories")
	case nullMode:
		renderSizeNull(c.Stdout, rows)
	default:
		s = ui.StyleOf(c.Term, noColor)
		renderSizeText(c.Stdout, s, rows, human, "directories", st, root, title)
		scanWarn(c.Stderr, s, res.errors, false)
	}
	return 0
}

// --- dirs_list_empty -------------------------------------------------------

func dirsListEmpty(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	jsonMode := jsonPrescan(c.Args)
	s := ui.StyleOf(c.Term, false)

	dir := "."
	var minDepthInput, maxDepthInput string
	ignoreDotfiles, noIgnore, xdev, allowRoot := false, false, false, false
	nullMode, countOnly := false, false
	var excludes []string

	args := c.Args
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--min-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			minDepthInput = v
		case "--depth", "--max-depth":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			maxDepthInput = v
		case "--exclude":
			v, ok := next(args, &i)
			if !ok {
				return missingValue(a).emit(c.Stderr, s, jsonMode)
			}
			if e := excludeOK(v); e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			excludes = append(excludes, v)
		case "--ignore-dotfiles":
			ignoreDotfiles = true
		case "--no-ignore":
			noIgnore = true
		case "--xdev", "--one-filesystem":
			xdev = true
		case "--allow-root":
			allowRoot = true
		case "--null", "-0":
			nullMode = true
		case "--count", "-c":
			countOnly = true
		case "--json":
			jsonMode = true
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

	minDepth := 0
	if minDepthInput != "" {
		if !isUint(minDepthInput) {
			return errf("invalid_depth", minDepthInput, "Provide a non-negative integer").emit(c.Stderr, s, jsonMode)
		}
		minDepth = int(limitValue(minDepthInput))
	}
	maxDepth := -1
	depthOverflow := false
	if maxDepthInput != "" {
		if !isUint(maxDepthInput) {
			return errf("invalid_depth", maxDepthInput, "Provide a non-negative integer").emit(c.Stderr, s, jsonMode)
		}
		// find -mindepth/-maxdepth read decimal; a value above INT_MAX (or
		// above int64) makes GNU find refuse its arguments (FS-14).
		d, err := strconv.ParseInt(maxDepthInput, 10, 64)
		if err != nil || d > findDepthMax {
			depthOverflow = true
		} else {
			maxDepth = int(d)
		}
	}

	home, pwd := callEnv(c)
	root, pe := safePath(dir, home, pwd, allowRoot)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}

	var paths []string
	errCount := 0
	if depthOverflow {
		errCount = 1 // GNU find prints one line for the bad -maxdepth arg
	} else {
		res := findWalk(walkOpts{
			root: root, xdev: xdev, maxDepth: maxDepth, minDepth: minDepth,
			noIgnore: noIgnore, excludes: excludes, skipNL: true, rawOrder: true,
		}, func(e entry) bool {
			if !e.isDir || !isEmptyDir(e.abs) {
				return false
			}
			if ignoreDotfiles && strings.HasPrefix(filepath.Base(e.abs), ".") {
				return false
			}
			return true
		})
		errCount = res.errors * 2 // GNU find reports each unreadable dir twice (FS-15)
		for _, e := range res.entries {
			paths = append(paths, e.abs)
		}
	}

	switch {
	case jsonMode:
		var b strings.Builder
		b.WriteString(`{"data":[`)
		for i, p := range paths {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`"` + escEmpty(p) + `"`)
		}
		b.WriteString(`],"count":` + strconv.Itoa(len(paths)))
		b.WriteString(`,"path":"` + escFull(root) + `"`)
		b.WriteString(`,"errors":` + strconv.Itoa(errCount) + "}\n")
		_, _ = io.WriteString(c.Stdout, b.String())
	case countOnly:
		_, _ = io.WriteString(c.Stdout, strconv.Itoa(len(paths))+"\n")
		scanWarn(c.Stderr, s, errCount, false)
	case nullMode:
		var b strings.Builder
		for _, p := range paths {
			b.WriteString(p)
			b.WriteByte(0)
		}
		_, _ = io.WriteString(c.Stdout, b.String())
		scanWarn(c.Stderr, s, errCount, false)
	default:
		// 2.x `printf '%s\n' "$output"`: the paths joined by newlines with a
		// trailing newline, or one blank line when none were found (FS-18).
		_, _ = io.WriteString(c.Stdout, strings.Join(paths, "\n")+"\n")
		scanWarn(c.Stderr, s, errCount, false)
	}
	return 0
}
