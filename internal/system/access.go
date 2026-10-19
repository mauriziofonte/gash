// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import "golang.org/x/sys/unix"

// faccess is the Bash file test on path with the effective user and group
// (eaccess): mode 4 is -r, 1 is -x.
func faccess(path string, mode uint32) bool {
	return path != "" && unix.Faccessat(unix.AT_FDCWD, path, mode, unix.AT_EACCESS) == nil
}
