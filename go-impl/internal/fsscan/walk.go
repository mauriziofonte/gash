// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
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

// walkOpts drive findWalk, the native replacement for the 2.x `find`
// invocations (architecture.md 7): one behavior on every platform, so the
// result never depends on which find is installed (MUST NOT 8, FS-26).
type walkOpts struct {
	root     string   // absolute scan root (safe_path)
	follow   bool     // -L: follow symlinks (FS-02 reproduced: no reguard)
	xdev     bool     // -xdev: stay on the root's device
	maxDepth int      // -maxdepth; <0 means unbounded
	minDepth int      // -mindepth; 0 means none
	noIgnore bool     // --no-ignore: drop the default prunes
	excludes []string // user --exclude globs, fnmatch on the absolute path
	skipTab  bool     // ! -path '*\t*'
	skipNL   bool     // ! -path '*\n*' (always true for every scanner)
	rawOrder bool     // children in GNU find's (fts) order, not sorted
}

// entry is one filesystem entry the walk yields, as find's -printf saw it.
type entry struct {
	abs       string // %p: the path find built (absolute, root-rooted)
	rel       string // %P: path relative to root ("" for the root itself)
	depth     int    // root is depth 0
	size      int64  // %s: st_size (of the target under -L)
	mtime     time.Time
	isDir     bool
	isRegular bool
}

// walkResult holds what one walk produced.
type walkResult struct {
	entries []entry
	errors  int // unreadable directories (FS-15)
}

// findWalk reproduces a 2.x `find` traversal: pre-order, directory entries
// in a deterministic (sorted) order (architecture.md 7), honoring the
// default prunes, user excludes, -maxdepth, -mindepth, -xdev and -L, and
// leaving out tab/newline paths as the scanner's -path tests did. It never
// consults the path guard below the root, so -L and --allow-root walk into
// protected trees exactly as 2.x did (FS-02, FS-03).
func findWalk(o walkOpts, keep func(e entry) bool) walkResult {
	res := walkResult{}
	var rootDev uint64
	if o.xdev {
		if st, err := os.Stat(o.root); err == nil {
			rootDev = deviceOf(st)
		}
	}
	pruneNames := map[string]bool{}
	if !o.noIgnore {
		for _, n := range defaultPrune {
			pruneNames[n] = true
		}
	}
	var visit func(abs, rel string, depth int, info os.FileInfo)
	visit = func(abs, rel string, depth int, info os.FileInfo) {
		isDir := info.IsDir()
		e := entry{
			abs: abs, rel: rel, depth: depth,
			size: info.Size(), mtime: info.ModTime(),
			isDir: isDir, isRegular: info.Mode().IsRegular(),
		}
		// The prune/exclude predicates fire at depth >= minDepth only, so a
		// prune above --min-depth does not stop the descent (FS-13).
		if depth >= o.minDepth {
			if isDir && pruneNames[filepath.Base(abs)] {
				return
			}
			for _, ex := range o.excludes {
				if ex != "" && pathguard.MatchGlobSegment(ex, abs) {
					return
				}
			}
		}
		yieldable := depth >= o.minDepth &&
			!(o.skipNL && strings.ContainsRune(abs, '\n')) &&
			!(o.skipTab && strings.ContainsRune(abs, '\t'))
		if yieldable && keep(e) {
			res.entries = append(res.entries, e)
		}
		if !isDir {
			return
		}
		if o.maxDepth >= 0 && depth >= o.maxDepth {
			return
		}
		var names []string
		var err error
		if o.rawOrder {
			names, err = readDirFTS(abs)
		} else {
			names, err = readDirNames(abs)
		}
		if err != nil {
			res.errors++
			return
		}
		for _, name := range names {
			child := filepath.Join(abs, name)
			crel := name
			if rel != "" {
				crel = rel + "/" + name
			}
			ci, cerr := statEntry(child, o.follow)
			if cerr != nil {
				continue
			}
			if o.xdev && ci.IsDir() && deviceOf(ci) != rootDev {
				continue
			}
			visit(child, crel, depth+1, ci)
		}
	}
	info, err := statEntry(o.root, o.follow)
	if err != nil {
		res.errors++
		return res
	}
	visit(o.root, "", 0, info)
	return res
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

// statEntry stats a path the way the walk reached it: Lstat normally, Stat
// under -L (follow). Under -L a dangling link or a loop fails and the
// entry is skipped (counted elsewhere as find would).
func statEntry(path string, follow bool) (os.FileInfo, error) {
	if follow {
		if st, err := os.Stat(path); err == nil {
			return st, nil
		}
	}
	return os.Lstat(path)
}

// deviceOf is the st_dev of an entry, for -xdev.
func deviceOf(info os.FileInfo) uint64 {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(st.Dev)
	}
	return 0
}
