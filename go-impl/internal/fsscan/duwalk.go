// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// duDir is one directory du reported: its absolute path and its cumulative
// size in KiB (du -k), with the depth relative to the root.
type duDir struct {
	abs   string
	kib   int64
	depth int
}

// duOpts drive duWalk, the native replacement for `du -k` (architecture.md
// 7): the GNU du semantics on every platform (FS-24, FS-27), hardlinks
// counted once, newline paths excluded as 2.x's `--exclude='*\n*'` did
// while tab paths are kept (FS-06, FS-09).
type duOpts struct {
	root     string
	xdev     bool
	maxDepth int      // du -d; <0 means unbounded
	noIgnore bool     // drop the default prunes
	excludes []string // user --exclude globs (du --exclude)
}

// duResult is what one du walk produced.
type duResult struct {
	dirs   []duDir
	errors int
}

// duWalk aggregates disk usage once, like `du -k`, and reports every
// directory within maxDepth (the root included; the caller drops it). An
// excluded entry (a default prune name, a user glob, or a newline in the
// path) is neither counted nor descended, as GNU du's --exclude does.
func duWalk(o duOpts) duResult {
	res := duResult{}
	var rootDev uint64
	if st, err := os.Stat(o.root); err == nil {
		rootDev = deviceOf(st)
	}
	var excludes []string
	if !o.noIgnore {
		excludes = append(excludes, defaultPrune...)
	}
	excludes = append(excludes, o.excludes...)
	excludes = append(excludes, "*\n*") // du --exclude='*\n*'

	seen := map[[2]uint64]bool{} // (dev,ino) of hardlinked files, counted once

	// blocks returns the cumulative 512-byte block count of the subtree at
	// abs (depth), recording each directory within maxDepth.
	var blocks func(abs, rel string, depth int, info os.FileInfo) int64
	blocks = func(abs, rel string, depth int, info os.FileInfo) int64 {
		total := blocksOf(info)
		if !info.IsDir() {
			return total
		}
		names, err := readDirNames(abs)
		if err != nil {
			res.errors++
		}
		for _, name := range names {
			if excluded(excludes, filepath.Join(abs, name), name) {
				continue
			}
			child := filepath.Join(abs, name)
			ci, cerr := os.Lstat(child)
			if cerr != nil {
				continue
			}
			if o.xdev && ci.IsDir() && deviceOf(ci) != rootDev {
				continue
			}
			if id, ok := inodeOf(ci); ok && !ci.IsDir() {
				if seen[id] {
					continue // a hardlink already counted (du counts once)
				}
				seen[id] = true
			}
			crel := name
			if rel != "" {
				crel = rel + "/" + name
			}
			total += blocks(child, crel, depth+1, ci)
		}
		if o.maxDepth < 0 || depth <= o.maxDepth {
			res.dirs = append(res.dirs, duDir{abs: abs, kib: (total + 1) / 2, depth: depth})
		}
		return total
	}
	info, err := os.Lstat(o.root)
	if err != nil {
		res.errors++
		return res
	}
	blocks(o.root, "", 0, info)
	return res
}

// excluded reports whether du's --exclude would skip an entry: a pattern
// without '/' matches the base name, one with '/' matches the whole path,
// both with fnmatch where '*' crosses '/' (no FNM_PATHNAME), as GNU du does.
func excluded(patterns []string, abs, base string) bool {
	for _, p := range patterns {
		if p == "" {
			continue
		}
		if strings.Contains(p, "/") {
			if pathguard.MatchGlobSegment(p, abs) {
				return true
			}
		} else if pathguard.MatchGlobSegment(p, base) {
			return true
		}
	}
	return false
}

// blocksOf is st_blocks (512-byte units); 0 when the platform hides it.
func blocksOf(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks)
	}
	return 0
}

// inodeOf is the (dev,ino) pair of an entry, for hardlink dedup.
func inodeOf(info os.FileInfo) ([2]uint64, bool) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return [2]uint64{uint64(st.Dev), uint64(st.Ino)}, true
	}
	return [2]uint64{}, false
}
