// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"io/fs"
	"os"
	"strconv"
	"syscall"
)

// CheckPerms is __gash_check_env_perms without its headless gate: the
// warnings for a file that is not private (any group or other permission
// bit) or not owned by euid. Symlinks are followed. A path that is not a
// regular file gives no warning.
func CheckPerms(path, home string, euid int) []Warning {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return nil
	}
	return permWarnings(fi, ShownPath(path, home), euid)
}

// permWarnings formats the two checks as 2.x does. The mode is printed as
// GNU stat -c %a prints it (octal, no leading zeros, setuid, setgid and
// sticky bits included); 2.x tried GNU stat first and BSD stat -f %Lp
// second, 3.x always gives the GNU form.
func permWarnings(fi fs.FileInfo, shown string, euid int) []Warning {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	var out []Warning
	mode := uint32(st.Mode) & 0o7777
	if mode&0o077 != 0 {
		perms := strconv.FormatUint(uint64(mode), 8)
		out = append(out, Warning{Text: shown + " is accessible by other users (mode " + perms + "). Run: chmod 600 " + shown})
	}
	if owner := int(st.Uid); owner != euid {
		out = append(out, Warning{Text: shown + " is owned by uid " + strconv.Itoa(owner) + ", not by you (uid " + strconv.Itoa(euid) + "). Gash will still read it."})
	}
	return out
}
