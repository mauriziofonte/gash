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

// jsonPrescan turns on JSON mode before parsing when a word before "--"
// is "--json", so errors met before it still get the JSON contract. The
// word after each of the scanner's value flags is a value, never the flag
// (FS-19): `--exclude --json` uses "--json" as the glob.
func jsonPrescan(args []string, valueFlags ...string) bool {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return false
		}
		if a == "--json" {
			return true
		}
		for _, f := range valueFlags {
			if a == f {
				i++
				break
			}
		}
	}
	return false
}

// intHint is the hint of invalid_limit, invalid_depth and invalid_top in
// every scanner (FS-49).
const intHint = "Provide a non-negative integer (at most 9 digits)"

// parseUint9 is the integer rule of every scanner flag (FS-05): 1 to 9
// ASCII digits, read in base 10 (so "010" is 10). Non-ASCII digits never
// match (FS-29).
func parseUint9(s string) (int, bool) {
	if s == "" || len(s) > 9 {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// intFlag validates an integer flag value, or returns its invalid_* error.
func intFlag(code, v string) (int, *fsError) {
	n, ok := parseUint9(v)
	if !ok {
		return 0, errf(code, v, intHint)
	}
	return n, nil
}

// pathArgs is the one-PATH rule of the scanners (FS-04): the first bare
// word is PATH; a second one, before or after "--", fails with
// unexpected_argument.
type pathArgs struct {
	dir   string
	taken bool
}

func (p *pathArgs) take(word string) *fsError {
	if p.taken {
		return errf("unexpected_argument", word, "Pass at most one PATH")
	}
	p.dir, p.taken = word, true
	return nil
}

// rest handles the words after "--": the first is PATH, any other is an
// unexpected_argument.
func (p *pathArgs) rest(words []string) *fsError {
	for _, w := range words {
		if e := p.take(w); e != nil {
			return e
		}
	}
	return nil
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
		return errf("invalid_filter", v, "--exclude takes a glob without newlines, e.g. 'node_modules'")
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

// rootUnreadable is the error of a scan root that cannot be listed
// (FS-14), worded as the safe_path check.
func rootUnreadable(input, home string) *fsError {
	return errf("path_unreadable", "Path '"+expandHome(input, home)+"' is not readable", "Check directory permissions")
}

// --- files_largest ---------------------------------------------------------

func filesLargest(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	jsonMode := jsonPrescan(c.Args, "-n", "--limit", "--min-size", "--exclude", "--depth", "--max-depth")
	s := ui.StyleOf(c.Term, false)

	pa := pathArgs{dir: "."}
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
			n, e := intFlag("invalid_limit", v)
			if e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			limit = n
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

	var minBytes int64
	if minInput != "" {
		n, ok := ParseSize(minInput)
		if !ok {
			return errf("invalid_size", minInput, "Use a size like 10M, 1G, 500K").emit(c.Stderr, s, jsonMode)
		}
		minBytes = n
	}
	maxDepth := -1
	if depthInput != "" {
		d, e := intFlag("invalid_depth", depthInput)
		if e != nil {
			return e.emit(c.Stderr, s, jsonMode)
		}
		maxDepth = d
	}

	home, pwd := callEnv(c)
	hidden := userHidden(c, home, pwd)
	root, fe := safePath(pa.dir, home, pwd, allowRoot, hidden)
	if fe != nil {
		return fe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	res := findWalk(walkOpts{
		root: root, hidden: hidden, follow: follow, xdev: xdev, maxDepth: maxDepth,
		noIgnore: noIgnore, excludes: excludes,
	}, func(e entry) bool { return e.isRegular && e.size >= minBytes })
	if res.rootErr {
		return rootUnreadable(pa.dir, home).emit(c.Stderr, s, jsonMode)
	}
	var rows []sizeRow
	for _, e := range res.entries {
		rows = append(rows, sizeRow{bytes: e.size, mtime: mtimeFmt(e.mtime), path: e.abs})
	}
	sortSizeRows(rows)
	if limit < len(rows) {
		rows = rows[:limit]
	}
	st := elapsed()

	switch {
	case jsonMode:
		renderSizeJSON(c.Stdout, rows, st, res.errors, "files")
	case nullMode:
		renderSizeNull(c.Stdout, rows)
	default:
		s = ui.StyleOf(c.Term, noColor)
		renderSizeText(c.Stdout, s, rows, human, "files", st, root, "")
		scanWarn(c.Stderr, s, res.errors, false)
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

// newestFileMtime is the newest regular file within ten levels of dir,
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
	jsonMode := jsonPrescan(c.Args, "-n", "--limit", "--depth", "--max-depth", "--min-size", "--exclude")
	s := ui.StyleOf(c.Term, false)

	pa := pathArgs{dir: "."}
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
			n, e := intFlag("invalid_limit", v)
			if e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			limit = n
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

	depth, fe := intFlag("invalid_depth", depthInput)
	if fe != nil {
		return fe.emit(c.Stderr, s, jsonMode)
	}
	if depth < 1 {
		depth = 1 // documented: 0 means 1 (FS-44)
	}
	var minBytes int64
	if minInput != "" {
		n, ok := ParseSize(minInput)
		if !ok {
			return errf("invalid_size", minInput, "Use a size like 10M, 1G").emit(c.Stderr, s, jsonMode)
		}
		minBytes = n
	}

	home, pwd := callEnv(c)
	hidden := userHidden(c, home, pwd)
	root, pe := safePath(pa.dir, home, pwd, allowRoot, hidden)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	res := duWalk(duOpts{root: root, hidden: hidden, xdev: xdev, maxDepth: depth, noIgnore: noIgnore, excludes: excludes})
	if res.rootErr {
		return rootUnreadable(pa.dir, home).emit(c.Stderr, s, jsonMode)
	}
	var rows []sizeRow
	for _, d := range res.dirs {
		if d.abs == root { // drop the root operand (du prints it last)
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
	jsonMode := jsonPrescan(c.Args, "--size", "--depth", "--max-depth", "-n", "--limit", "--exclude")
	s := ui.StyleOf(c.Term, false)

	pa := pathArgs{dir: "."}
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
			n, e := intFlag("invalid_limit", v)
			if e != nil {
				return e.emit(c.Stderr, s, jsonMode)
			}
			limit = n
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

	// Order matches 2.x: size, then limit, then depth.
	threshold, ok := ParseSize(sizeThreshold)
	if !ok || threshold > maxInt64-1023 { // (bytes+1023)/1024 must not overflow (FS-22)
		return errf("invalid_size", sizeThreshold, "Use a size like 20M, 1G, 500K").emit(c.Stderr, s, jsonMode)
	}
	thresholdKiB := (threshold + 1023) / 1024
	maxDepth := -1
	if depthInput != "" {
		depth, fe := intFlag("invalid_depth", depthInput)
		if fe != nil {
			return fe.emit(c.Stderr, s, jsonMode)
		}
		if depth > 0 { // 0 (and no flag) means unbounded (FS-45)
			maxDepth = depth
		}
	}

	home, pwd := callEnv(c)
	hidden := userHidden(c, home, pwd)
	root, pe := safePath(pa.dir, home, pwd, allowRoot, hidden)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}
	human := hm.resolve(jsonMode, nullMode)
	elapsed := scanTimer()

	res := duWalk(duOpts{root: root, hidden: hidden, xdev: xdev, maxDepth: maxDepth, noIgnore: noIgnore, excludes: excludes})
	if res.rootErr {
		return rootUnreadable(pa.dir, home).emit(c.Stderr, s, jsonMode)
	}
	var rows []sizeRow
	for _, d := range res.dirs {
		if d.abs == root {
			continue
		}
		if d.kib < thresholdKiB {
			continue
		}
		rows = append(rows, sizeRow{bytes: d.kib * 1024, path: d.abs})
	}
	// Size descending, ties by the `bytes\tpath` text in C byte order
	// descending (FS-28), then the limit; --with-mtime enrichment happens
	// afterwards, so mtime is never part of the sort key. A path is never
	// split once read (FS-09).
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
	if withMtime {
		for i := range rows {
			rows[i].mtime = newestFileMtime(rows[i].path)
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
	jsonMode := jsonPrescan(c.Args, "--min-depth", "--depth", "--max-depth", "--exclude")
	s := ui.StyleOf(c.Term, false)

	pa := pathArgs{dir: "."}
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

	minDepth := 0
	if minDepthInput != "" {
		d, e := intFlag("invalid_depth", minDepthInput)
		if e != nil {
			return e.emit(c.Stderr, s, jsonMode)
		}
		minDepth = d
	}
	maxDepth := -1
	if maxDepthInput != "" {
		d, e := intFlag("invalid_depth", maxDepthInput)
		if e != nil {
			return e.emit(c.Stderr, s, jsonMode)
		}
		maxDepth = d
	}

	home, pwd := callEnv(c)
	hidden := userHidden(c, home, pwd)
	root, pe := safePath(pa.dir, home, pwd, allowRoot, hidden)
	if pe != nil {
		return pe.emit(c.Stderr, s, jsonMode)
	}

	// --min-depth only filters the output: the prunes and excludes apply at
	// every level during the walk (FS-13).
	res := findWalk(walkOpts{
		root: root, hidden: hidden, xdev: xdev, maxDepth: maxDepth,
		noIgnore: noIgnore, excludes: excludes, rawOrder: true, needEmpty: true,
	}, func(e entry) bool {
		if e.depth < minDepth || !e.isDir || !e.empty {
			return false
		}
		return !(ignoreDotfiles && strings.HasPrefix(filepath.Base(e.abs), "."))
	})
	if res.rootErr {
		return rootUnreadable(pa.dir, home).emit(c.Stderr, s, jsonMode)
	}
	paths := make([]string, 0, len(res.entries))
	for _, e := range res.entries {
		paths = append(paths, e.abs)
	}

	switch {
	case jsonMode:
		var b strings.Builder
		b.WriteString(`{"data":[`)
		for i, p := range paths {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`"` + escFull(p) + `"`)
		}
		b.WriteString(`],"count":` + strconv.Itoa(len(paths)))
		b.WriteString(`,"path":"` + escFull(root) + `"`)
		b.WriteString(`,"errors":` + strconv.Itoa(res.errors) + "}\n")
		_, _ = io.WriteString(c.Stdout, b.String())
	case countOnly:
		_, _ = io.WriteString(c.Stdout, strconv.Itoa(len(paths))+"\n")
		scanWarn(c.Stderr, s, res.errors, false)
	case nullMode:
		var b strings.Builder
		for _, p := range paths {
			b.WriteString(p)
			b.WriteByte(0)
		}
		_, _ = io.WriteString(c.Stdout, b.String())
		scanWarn(c.Stderr, s, res.errors, false)
	default:
		// One path per line; nothing at all when none was found (FS-18).
		var b strings.Builder
		for _, p := range paths {
			b.WriteString(p)
			b.WriteByte('\n')
		}
		_, _ = io.WriteString(c.Stdout, b.String())
		scanWarn(c.Stderr, s, res.errors, false)
	}
	return 0
}
