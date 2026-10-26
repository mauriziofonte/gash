// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// defaultPrune is __GASH_FS_DEFAULT_PRUNE: directory names skipped unless
// --no-ignore, matched on the base name (files.sh:34-40).
var defaultPrune = []string{"node_modules", "vendor", ".git", "__pycache__", ".cache"}

// walkOpts drive findWalk, the native walk every scanner uses
// (architecture.md 7): one behavior on every platform, so the result never
// depends on which find or du is installed (MUST NOT 8, FS-26).
type walkOpts struct {
	root      string   // absolute, physical scan root (safePath)
	follow    bool     // -L: follow symlinks (never into protected trees, FS-02)
	xdev      bool     // stay on the root's device
	maxDepth  int      // deepest level reported and descended; <0 means unbounded
	noIgnore  bool     // --no-ignore: drop the default prunes
	excludes  []string // user --exclude globs (FS-12)
	rawOrder  bool     // children in readdir (fts) order, not sorted
	needEmpty bool     // also read directories at maxDepth, to know if they are empty
	hidden    []string // the user's PROTECT:all trees (userHidden), left out like protected ones
}

// entry is one filesystem entry the walk reports.
type entry struct {
	abs       string // the path below the root, as the walk built it
	rel       string // path relative to the root ("" for the root itself)
	depth     int    // root is depth 0
	size      int64  // st_size (of the target under -L)
	mtime     time.Time
	isDir     bool
	isRegular bool
	empty     bool // a directory that holds no entry at all (FS-16)
}

// walkResult holds what one walk produced.
type walkResult struct {
	entries []entry
	// errors is the number of entries whose open or stat failed, each
	// counted once, plus the symlink loops met under -L (FS-15).
	errors int
	// rootErr reports that the root itself could not be read (FS-14).
	rootErr bool
}

// skipped is the one exclusion rule of the five scanners, applied to every
// entry below the root (depth >= 1, FS-13) during the walk: a path that
// holds a tab or a newline (FS-01), a default prune directory, or a user
// --exclude match (FS-12). A skipped entry is not reported, not descended
// and adds to no total.
func skipped(o *walkOpts, rel, name string, isDir bool) bool {
	if strings.ContainsAny(rel, "\t\n") {
		return true
	}
	if isDir && !o.noIgnore {
		for _, n := range defaultPrune {
			if name == n {
				return true
			}
		}
	}
	for _, ex := range o.excludes {
		if excludeMatch(ex, rel, name) {
			return true
		}
	}
	return false
}

// excludeMatch is the --exclude rule (FS-12): GLOB matches the entry's base
// name or its path relative to the scan root, with fnmatch semantics where
// '*', '?' and a bracket never match '/' (FNM_PATHNAME). The root's own
// path is never part of the match.
func excludeMatch(pat, rel, name string) bool {
	if pat == "" {
		return false
	}
	return matchPathname(pat, name) || matchPathname(pat, rel)
}

// matchPathname is fnmatch with FNM_PATHNAME: the pattern and the name have
// the same number of '/'-separated components and each pair matches.
func matchPathname(pat, name string) bool {
	ps, ns := strings.Split(pat, "/"), strings.Split(name, "/")
	if len(ps) != len(ns) {
		return false
	}
	for i := range ps {
		if !pathguard.MatchGlobSegment(ps[i], ns[i]) {
			return false
		}
	}
	return true
}

// findWalk walks the tree pre-order, children in sorted order (or readdir
// order with rawOrder), and reports every entry keep accepts, the root
// included. Below the root it applies skipped, -xdev, -L with loop
// detection, and the protected-tree rail: an entry whose physical path is
// protected (FS-02, FS-03) or in one of the user's PROTECT:all trees
// (3.4.1) is skipped silently, before its target is stat'ed, and is not
// counted in errors.
func findWalk(o walkOpts, keep func(e entry) bool) walkResult {
	res := walkResult{}
	var rootDev uint64
	rootInfo, err := os.Stat(o.root)
	if err != nil {
		res.errors++
		res.rootErr = true
		return res
	}
	rootDev = deviceOf(rootInfo)
	ancestors := map[[2]uint64]bool{}

	var visit func(abs, phys, rel string, depth int, info os.FileInfo)
	visit = func(abs, phys, rel string, depth int, info os.FileInfo) {
		isDir := info.IsDir()
		e := entry{
			abs: abs, rel: rel, depth: depth,
			size: info.Size(), mtime: info.ModTime(),
			isDir: isDir, isRegular: info.Mode().IsRegular(),
		}
		descend := isDir && (o.maxDepth < 0 || depth < o.maxDepth)
		var names []string
		if isDir && (descend || o.needEmpty) {
			var rerr error
			if o.rawOrder {
				names, rerr = readDirFTS(abs)
			} else {
				names, rerr = readDirNames(abs)
			}
			if rerr != nil {
				res.errors++
				if depth == 0 {
					res.rootErr = true
				}
				descend = false
			} else {
				e.empty = len(names) == 0
			}
		}
		if keep(e) {
			res.entries = append(res.entries, e)
		}
		if !descend {
			return
		}
		id, hasID := inodeOf(info)
		if hasID {
			ancestors[id] = true
			defer delete(ancestors, id)
		}
		for _, name := range names {
			crel := name
			if rel != "" {
				crel = rel + "/" + name
			}
			child := filepath.Join(abs, name)
			cphys := filepath.Join(phys, name)
			if protected(cphys) || pathguard.UnderTrees(cphys, o.hidden) {
				continue
			}
			ci, lerr := os.Lstat(child)
			if lerr != nil {
				res.errors++
				continue
			}
			if o.follow && ci.Mode()&os.ModeSymlink != 0 {
				target, rerr := filepath.EvalSymlinks(child)
				if rerr == nil && (protected(target) || pathguard.UnderTrees(target, o.hidden)) {
					continue
				}
				if st, serr := os.Stat(child); serr == nil {
					ci, cphys = st, target
				} else if !errors.Is(serr, fs.ErrNotExist) {
					res.errors++ // a symlink loop: find -L reports it
				}
			}
			if skipped(&o, crel, name, ci.IsDir()) {
				continue
			}
			if o.xdev && ci.IsDir() && deviceOf(ci) != rootDev {
				continue
			}
			if ci.IsDir() {
				if cid, ok := inodeOf(ci); ok && ancestors[cid] {
					res.errors++ // a directory loop through a followed link
					continue
				}
			}
			visit(child, cphys, crel, depth+1, ci)
		}
	}
	visit(o.root, o.root, "", 0, rootInfo)
	return res
}

// protectedList is the scanners' protected set: __gash_fs_forbidden's list
// plus root's home where it is not /root (macOS: /var/root), the same table
// for the PATH check and for the walk (FS-02, FS-03).
func protectedList() []string {
	list := scannerForbidden
	if h := pathguard.RootHome(); len(h) > 1 && h[0] == '/' && h != "/root" {
		list = append(append([]string{}, list...), h)
	}
	return list
}

// protected reports whether a physical path is, or is below, a protected
// location (whole components), also with a leading /private removed
// (macOS's /private/var/root).
func protected(p string) bool {
	for _, cand := range []string{p, strings.TrimPrefix(p, "/private")} {
		for _, f := range protectedList() {
			if cand == f || strings.HasPrefix(cand, f+"/") {
				return true
			}
		}
	}
	return false
}

// readDirNames returns the names of a directory, sorted (deterministic
// order, architecture.md 7).
func readDirNames(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// The gnulib fts limits GNU find runs under: a directory is read in batches
// of at most ftsMaxReaddir entries, and a batch of more than
// ftsInodeSortThreshold entries is sorted by inode number unless the file
// system is tmpfs, NFS or CIFS (fts.c, fts_build and
// dirent_inode_sort_may_be_useful).
const (
	ftsMaxReaddir         = 100000
	ftsInodeSortThreshold = 10000
	magicTmpfs            = 0x01021994
	magicNFS              = 0x6969
	magicCIFS             = 0xFF534D42
)

// readDirFTS returns a directory's names in the order GNU find visits them:
// readdir order, with fts's inode sort of large batches. dirs_list_empty
// prints in this order (MATRIX: readdir order, unsorted).
func readDirFTS(dir string) ([]string, error) {
	f, err := os.Open(dir)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	names, err := f.Readdirnames(-1)
	if err != nil {
		return nil, err
	}
	if len(names) <= ftsInodeSortThreshold || !inodeSortUseful(dir) {
		return names, nil
	}
	for start := 0; start < len(names); start += ftsMaxReaddir {
		end := min(start+ftsMaxReaddir, len(names))
		batch := names[start:end]
		if len(batch) <= ftsInodeSortThreshold {
			continue
		}
		ino := make(map[string]uint64, len(batch))
		for _, n := range batch {
			if info, e := os.Lstat(filepath.Join(dir, n)); e == nil {
				if st, ok := info.Sys().(*syscall.Stat_t); ok {
					ino[n] = uint64(st.Ino)
				}
			}
		}
		sort.SliceStable(batch, func(i, j int) bool { return ino[batch[i]] < ino[batch[j]] })
	}
	return names, nil
}

// inodeSortUseful is gnulib's dirent_inode_sort_may_be_useful: false on
// tmpfs, NFS and CIFS (Linux only, where statfs gives the type), else true.
func inodeSortUseful(dir string) bool {
	if runtime.GOOS != "linux" {
		return true
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return true
	}
	switch uint32(st.Type) {
	case magicTmpfs, magicNFS, magicCIFS:
		return false
	}
	return true
}

// deviceOf is the st_dev of an entry, for -xdev.
func deviceOf(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}
