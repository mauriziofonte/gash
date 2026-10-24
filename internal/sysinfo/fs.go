// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/platform"
)

// The file probes of 2.x, read as the invoking user through a Sysroot:
// test, cat, ls, Bash globs and find.

// files reads system paths for the collectors; u8 is a UTF-8 locale.
type files struct {
	sys platform.Sysroot
	u8  bool
}

// isFile is [[ -f P ]], isDir [[ -d P ]], exists [[ -e P ]] (all follow
// symlinks); isLink is [[ -L P ]].
func (f files) isFile(p string) bool { return f.sys.IsRegular(p) }
func (f files) isDir(p string) bool  { return f.sys.IsDir(p) }
func (f files) exists(p string) bool { return f.sys.Exists(p) }
func (f files) isLink(p string) bool {
	st, err := f.sys.Lstat(p)
	return err == nil && st.Mode()&fs.ModeSymlink != 0
}

// read is `cat P` (nothing when it cannot be read).
func (f files) read(p string) []byte {
	b, err := f.sys.ReadFile(p)
	if err != nil {
		return nil
	}
	return b
}

// ls is `ls DIR/` piped: the visible names in byte order (SYS-41).
func (f files) ls(dir string) []string {
	ents, err := f.sys.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// glob is Bash pathname expansion of an absolute pattern (`*` and `?`,
// no dotfiles), in byte order (SYS-41); nil when nothing matches, where
// Bash keeps the word. A trailing slash matches directories only and
// stays on the results.
func (f files) glob(pattern string) []string {
	dirOnly := strings.HasSuffix(pattern, "/")
	parts := strings.Split(strings.Trim(pattern, "/"), "/")
	cands := []string{""}
	for i, part := range parts {
		last := i == len(parts)-1
		var next []string
		for _, c := range cands {
			if !strings.ContainsAny(part, "*?") {
				next = append(next, c+"/"+part)
				continue
			}
			dir := c
			if dir == "" {
				dir = "/"
			}
			ents, err := f.sys.ReadDir(dir)
			if err != nil {
				continue
			}
			for _, e := range ents {
				if !strings.HasPrefix(e.Name(), ".") && fnmatch(part, e.Name()) {
					next = append(next, c+"/"+e.Name())
				}
			}
		}
		if !last {
			// Only directories lead on.
			kept := next[:0]
			for _, n := range next {
				if f.sys.IsDir(n) {
					kept = append(kept, n)
				}
			}
			next = kept
		}
		cands = next
	}
	var out []string
	for _, c := range cands {
		if _, err := f.sys.Lstat(c); err != nil {
			continue
		}
		if dirOnly {
			if !f.sys.IsDir(c) {
				continue
			}
			c += "/"
		}
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// words is the expansion of a glob word as Bash passes it on: the matches,
// or the pattern itself when nothing matches.
func (f files) words(pattern string) []string {
	if m := f.glob(pattern); m != nil {
		return m
	}
	return []string{pattern}
}

// lsPaths is `ls WORD...` piped, for words naming files: each one that
// exists, in byte order.
func (f files) lsPaths(words ...string) []string {
	var out []string
	for _, w := range words {
		if st, err := f.sys.Lstat(w); err == nil && !st.IsDir() {
			out = append(out, w)
		} else if err == nil && f.sys.IsDir(w) {
			for _, n := range f.ls(w) {
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// globIn is the Bash glob "PREFIX"PATTERN: the visible names of the
// directory PREFIX (taken literally) that match one-component PATTERN,
// in byte order, PREFIX joined; nil when nothing matches.
func (f files) globIn(prefix, pattern string) []string {
	dir := prefix
	if dir == "" {
		dir = "."
	}
	ents, err := f.sys.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), ".") && fnmatch(pattern, e.Name()) {
			out = append(out, prefix+e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// isBlock is [[ -b P ]].
func (f files) isBlock(p string) bool {
	st, err := f.sys.Stat(p)
	return err == nil && st.Mode()&fs.ModeDevice != 0 && st.Mode()&fs.ModeCharDevice == 0
}

// abs is a path a test operator gets: a relative one is the working
// directory's.
func (f files) abs(p string) string {
	if path.IsAbs(p) {
		return p
	}
	wd, err := os.Getwd()
	if err != nil {
		return "/" + p
	}
	return filepath.Join(wd, p)
}

// tail is `tail -n N FILE`, read from the end of a regular file.
func (f files) tail(p string, n int) []byte {
	r, err := f.sys.Resolve(p)
	if err != nil {
		return nil
	}
	fh, err := os.Open(f.sys.Path(r))
	if err != nil {
		return nil
	}
	defer func() { _ = fh.Close() }()
	st, err := fh.Stat()
	if err != nil {
		return nil
	}
	if !st.Mode().IsRegular() {
		b, _ := io.ReadAll(fh)
		return tailN(b, n)
	}
	size, chunk := st.Size(), int64(64<<10)
	for {
		off := max(size-chunk, 0)
		buf := make([]byte, size-off)
		if _, err := fh.ReadAt(buf, off); err != nil && err != io.EOF {
			return nil
		}
		if off == 0 || bytes.Count(buf, []byte{'\n'}) > n {
			return tailN(buf, n)
		}
		chunk *= 4
	}
}

// basenames is __sysinfo_basenames DIR SUFFIX [keep]: the names of
// DIR/*SUFFIX that exist, SUFFIX stripped unless keep.
func (f files) basenames(dir, suffix string, keep bool) []string {
	var out []string
	for _, p := range f.globIn(dir+"/", "*"+suffix) {
		if !f.sys.Exists(p) {
			continue
		}
		name := path.Base(p)
		if !keep {
			name = strings.TrimSuffix(name, suffix)
		}
		out = append(out, name)
	}
	return out
}

// query is a find expression: -mindepth, -maxdepth (-1 for none), -type
// (0, 'f' or 'd'), -name patterns joined by -o, -path.
type query struct {
	minDepth, maxDepth int
	typ                byte
	names              []string
	path               string
}

func (q query) match(p string, d fs.DirEntry, depth int) bool {
	if depth < q.minDepth {
		return false
	}
	switch q.typ {
	case 'f':
		if !d.Type().IsRegular() {
			return false
		}
	case 'd':
		if !d.IsDir() {
			return false
		}
	}
	if len(q.names) > 0 {
		ok := false
		for _, n := range q.names {
			if fnmatch(n, path.Base(p)) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return q.path == "" || fnmatch(q.path, p)
}

// find is `find ROOT EXPR` without -L: a lexical walk (byte order, SYS-41)
// that does not follow symlinks, the start point included.
func (f files) find(root string, q query) []string {
	st, err := f.sys.Lstat(root)
	if err != nil {
		return nil
	}
	if st.Mode()&fs.ModeSymlink != 0 || !st.IsDir() {
		if q.match(root, fs.FileInfoToDirEntry(st), 0) {
			return []string{root}
		}
		return nil
	}
	base := strings.Count(strings.TrimRight(root, "/"), "/")
	var out []string
	_ = f.sys.Walk(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		depth := strings.Count(strings.TrimRight(p, "/"), "/") - base
		if q.maxDepth >= 0 && depth > q.maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if q.match(p, d, depth) {
			out = append(out, p)
		}
		return nil
	})
	return out
}

// findAll is find over several start points, in order.
func (f files) findAll(roots []string, q query) []string {
	var out []string
	for _, r := range roots {
		out = append(out, f.find(r, q)...)
	}
	return out
}

// cat is `find ... -exec cat {} \;`: the files' bytes run together.
func (f files) cat(paths []string) []byte {
	var b []byte
	for _, p := range paths {
		b = append(b, f.read(p)...)
	}
	return b
}
