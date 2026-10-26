// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package sysinfo

import (
	"strconv"

	"golang.org/x/sys/unix"
)

// Uname is the kernel's name, release and machine (uname -srm).
func Uname() (sysname, release, machine string, ok bool) {
	var u unix.Utsname
	if err := unix.Uname(&u); err != nil {
		return "", "", "", false
	}
	return unix.ByteSliceToString(u.Sysname[:]), unix.ByteSliceToString(u.Release[:]), unix.ByteSliceToString(u.Machine[:]), true
}

// Statfs is the size of the filesystem that holds path.
func Statfs(path string) (FS, bool) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return FS{}, false
	}
	bs := uint64(st.Bsize) //nolint:gosec // a block size is positive
	var dev unix.Stat_t
	id := ""
	if unix.Stat(path, &dev) == nil {
		id = strconv.FormatUint(uint64(dev.Dev), 10) //nolint:unconvert // Dev is int32 on darwin
	}
	return FS{Dev: id, Total: st.Blocks * bs, Free: st.Bfree * bs, Avail: st.Bavail * bs}, true
}
