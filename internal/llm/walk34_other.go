// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build !linux

package llm

import "os"

// scanDir reads one directory of a pool walk and calls fn for each entry,
// in the order the file system returns them, with its lstat when stat is
// set (the type bits of the directory entry otherwise). ok is false when
// the directory could not be opened or read to the end.
func scanDir(path string, stat bool, fn func(e scanEntry)) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	ents, err := f.ReadDir(-1)
	_ = f.Close()
	for _, de := range ents {
		e := scanEntry{name: de.Name(), typ: dirEntryType(de.Type())}
		if stat {
			if fi, err := de.Info(); err != nil {
				e.failed = true
			} else {
				e.size, e.mtime, e.typ = fi.Size(), fi.ModTime(), dirEntryType(fi.Mode())
			}
		}
		fn(e)
	}
	return err == nil
}
