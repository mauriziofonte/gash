// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io/fs"
	"os"
	"strings"
)

// The llm_* walks are their own 2.x code (find pipelines in fs.sh), so this
// package has its own walker instead of sharing one with the files module.

// findEntry is one file a find(1) walk visits.
type findEntry struct {
	path  string      // as find prints it: the start point joined with the names
	rel   string      // %P: path below the start point ("" for the start point)
	depth int         // 0 for the start point
	mode  fs.FileMode // lstat type bits (find without -L)
	size  int64       // lstat st_size (%s)
}

// typeLetter is find's %y for the types the llm walks select.
func (e findEntry) typeLetter() byte {
	switch {
	case e.mode.IsDir():
		return 'd'
	case e.mode&fs.ModeSymlink != 0:
		return 'l'
	case e.mode.IsRegular():
		return 'f'
	}
	return '?'
}

// prunedNames are the -name ... -prune tests of llm_tree.
var prunedNames = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true, "__pycache__": true, ".cache": true,
}

// findJoin is how find names a child of dir: "/" + name below "/", and
// dir + "/" + name elsewhere.
func findJoin(dir, name string) string {
	if strings.HasSuffix(dir, "/") {
		return dir + name
	}
	return dir + "/" + name
}

// startName is the base name find's -name tests on the start point: the
// last component with trailing slashes removed, "/" for "/".
func startName(p string) string {
	t := strings.TrimRight(p, "/")
	if t == "" {
		if p == "" {
			return ""
		}
		return "/"
	}
	if i := strings.LastIndexByte(t, '/'); i >= 0 {
		return t[i+1:]
	}
	return t
}

// findWalker walks like GNU find without -L: pre-order, each directory's
// entries in the order the file system returns them (find does not sort),
// unreadable directories skipped silently (2>/dev/null).
type findWalker struct {
	maxDepth int // -maxdepth; < 0 for none
	// visit is called for every entry find evaluates (depth >= 1, and the
	// start point when withStart); it returns whether to descend into a
	// directory (false is -prune).
	visit     func(e findEntry) bool
	withStart bool
	// stop, once set by visit, ends the walk (head closed the pipe).
	stop bool
}

func (w *findWalker) walk(start string) {
	st, err := os.Lstat(start)
	if err != nil {
		return
	}
	e := findEntry{path: start, mode: st.Mode(), size: st.Size()}
	if w.withStart && !w.visit(e) {
		return
	}
	if st.IsDir() && (w.maxDepth < 0 || w.maxDepth > 0) {
		w.dir(start, "", 1)
	}
}

func (w *findWalker) dir(path, rel string, depth int) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	ents, _ := f.ReadDir(-1)
	_ = f.Close()
	for _, d := range ents {
		if w.stop {
			return
		}
		name := d.Name()
		info, err := d.Info()
		if err != nil {
			continue
		}
		e := findEntry{path: findJoin(path, name), depth: depth, mode: info.Mode(), size: info.Size()}
		if rel == "" {
			e.rel = name
		} else {
			e.rel = rel + "/" + name
		}
		if !w.visit(e) {
			continue
		}
		if e.mode.IsDir() && (w.maxDepth < 0 || depth < w.maxDepth) {
			w.dir(e.path, e.rel, depth+1)
		}
	}
}
