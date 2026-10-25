// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build linux

package llm

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

// direntBufs are the getdents64 buffers of the pool walks, one per
// directory being read.
var direntBufs = sync.Pool{New: func() any {
	b := make([]byte, 32<<10)
	return &b
}}

// scanDir reads one directory of a pool walk and calls fn for each entry,
// in the order the file system returns them: getdents64 on the
// directory's descriptor, and with stat each entry's lstat by fstatat on
// it (no path walk per entry, no os.File per directory: a walk of tens of
// thousands of entries spends its time in the kernel). Without stat the
// type is the directory entry's (an lstat only where the file system does
// not give it). ok is false when the directory could not be opened or
// read to the end.
func scanDir(path string, stat bool, fn func(e scanEntry)) bool {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return false
	}
	defer func() { _ = unix.Close(fd) }()
	bp := direntBufs.Get().(*[]byte)
	defer direntBufs.Put(bp)
	buf := *bp
	var st unix.Stat_t
	for {
		n, err := unix.Getdents(fd, buf)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false
		}
		if n <= 0 {
			return true
		}
		// struct linux_dirent64: d_ino (8), d_off (8), d_reclen (2),
		// d_type (1), d_name (NUL-terminated).
		for off := 0; off < n; {
			if off+19 > n {
				return false
			}
			reclen := int(binary.NativeEndian.Uint16(buf[off+16:]))
			if reclen < 19 || off+reclen > n {
				return false
			}
			dtype, name := buf[off+18], buf[off+19:off+reclen]
			off += reclen
			if i := bytes.IndexByte(name, 0); i >= 0 {
				name = name[:i]
			}
			if string(name) == "." || string(name) == ".." {
				continue
			}
			e := scanEntry{name: string(name), typ: direntType(dtype)}
			if stat || e.typ == 0 {
				if unix.Fstatat(fd, e.name, &st, unix.AT_SYMLINK_NOFOLLOW) != nil {
					e.failed = true
				} else {
					e.typ, e.size, e.mtime = statType(st.Mode), st.Size, time.Unix(st.Mtim.Unix())
				}
			}
			fn(e)
		}
	}
}

// direntType is find's %y letter of a d_type, 0 for DT_UNKNOWN.
func direntType(t uint8) byte {
	switch t {
	case unix.DT_UNKNOWN:
		return 0
	case unix.DT_DIR:
		return 'd'
	case unix.DT_LNK:
		return 'l'
	case unix.DT_REG:
		return 'f'
	}
	return '?'
}

// statType is find's %y letter of an st_mode.
func statType(mode uint32) byte {
	switch mode & unix.S_IFMT {
	case unix.S_IFDIR:
		return 'd'
	case unix.S_IFLNK:
		return 'l'
	case unix.S_IFREG:
		return 'f'
	}
	return '?'
}
