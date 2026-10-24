// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"os"
	"path/filepath"
	"syscall"
)

// duDir is one directory the walk reported: its absolute path and its
// cumulative size in KiB (du -k), with the depth relative to the root.
type duDir struct {
	abs   string
	kib   int64
	depth int
}

// duOpts drive duWalk, the native replacement for `du -k` (architecture.md
// 7): the GNU du semantics on every platform (FS-24, FS-27), hardlinks
// counted once.
type duOpts struct {
	root     string
	xdev     bool
	maxDepth int      // du -d; <0 means unbounded
	noIgnore bool     // drop the default prunes
	excludes []string // user --exclude globs (FS-12)
}

// duResult is what one du walk produced.
type duResult struct {
	dirs    []duDir
	errors  int  // entries whose open or stat failed, each once (FS-15)
	rootErr bool // the root itself could not be read (FS-14)
}

// duWalk aggregates disk usage once, like `du -k`, and reports every
// directory within maxDepth (the root included; the caller drops it). It
// applies the scanners' one exclusion rule (skipped: tab/newline paths,
// default prunes, --exclude; FS-01, FS-12, FS-13) and the protected-tree
// rail (FS-03): a skipped entry is neither counted nor descended.
func duWalk(o duOpts) duResult {
	res := duResult{}
	wo := walkOpts{noIgnore: o.noIgnore, excludes: o.excludes}
	rootInfo, err := os.Lstat(o.root)
	if err != nil {
		res.errors++
		res.rootErr = true
		return res
	}
	rootDev := deviceOf(rootInfo)
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
			if depth == 0 {
				res.rootErr = true
			}
		}
		for _, name := range names {
			child := filepath.Join(abs, name)
			if protected(child) {
				continue
			}
			crel := name
			if rel != "" {
				crel = rel + "/" + name
			}
			ci, cerr := os.Lstat(child)
			if cerr != nil {
				res.errors++
				continue
			}
			if skipped(&wo, crel, name, ci.IsDir()) {
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
			total += blocks(child, crel, depth+1, ci)
		}
		if o.maxDepth < 0 || depth <= o.maxDepth {
			res.dirs = append(res.dirs, duDir{abs: abs, kib: (total + 1) / 2, depth: depth})
		}
		return total
	}
	blocks(o.root, "", 0, rootInfo)
	return res
}

// blocksOf is st_blocks (512-byte units); 0 when the platform hides it.
func blocksOf(info os.FileInfo) int64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int64(st.Blocks)
	}
	return 0
}

// inodeOf is the (dev,ino) pair of an entry, for hardlink dedup and loop
// detection.
func inodeOf(info os.FileInfo) ([2]uint64, bool) {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return [2]uint64{uint64(st.Dev), uint64(st.Ino)}, true
	}
	return [2]uint64{}, false
}
