// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

const unknownUsername = "UNKNOWN_USERNAME"

// username is gash_username: whoami, then id -un, then the /etc/passwd
// entry of id -u (of the effective uid without id, 3.4), else
// UNKNOWN_USERNAME with a warning that says why (3.4: silent). An argument
// is refused (3.0: SELF-15 fixed; 2.x ignored it).
func (m *Module) username(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if badArgs(c, "gash_username") {
		return usageStatus
	}
	name := ""
	// if command -v whoami; then user_name=$(whoami 2>/dev/null) || true
	p, hasWhoami := m.lookPath(c, "whoami")
	if hasWhoami {
		if n, found := m.whoamiFiles(p); found {
			name = n
		} else {
			name = m.output(c, "whoami")
		}
	}
	_, hasID := m.lookPath(c, "id")
	if name == "" && hasID {
		name = m.output(c, "id", "-un")
	}
	uid := ""
	if name == "" && hasID {
		uid = m.output(c, "id", "-u")
	}
	if name == "" && !hasID {
		uid = strconv.Itoa(os.Geteuid())
	}
	var passwdErr error
	if name == "" && uid != "" {
		var b []byte
		if b, passwdErr = m.readFile(m.passwd); passwdErr == nil {
			name = passwdNames(string(b), uid)
		}
	}
	if name == "" {
		name = unknownUsername
		usernameWarning(c, uid, hasWhoami, hasID, passwdErr != nil, m.passwd)
	}
	_, _ = c.Stdout.Write([]byte(name + "\n"))
	return 0
}

// usernameWarning says why gash_username prints UNKNOWN_USERNAME: what
// was missing or knew no name.
func usernameWarning(c *invoke.Call, uid string, hasWhoami, hasID, passwdUnreadable bool, passwd string) {
	var why []string
	for _, t := range []struct {
		name string
		has  bool
	}{{"whoami", hasWhoami}, {"id", hasID}} {
		if t.has {
			why = append(why, t.name+" knows no name for it")
		} else {
			why = append(why, t.name+" is not installed")
		}
	}
	switch {
	case uid == "":
		why = append(why, "id -u printed no uid")
	case passwdUnreadable:
		why = append(why, passwd+" cannot be read")
	default:
		why = append(why, passwd+" has no entry for it")
	}
	who := "the current user"
	if uid != "" {
		who = "uid " + uid
	}
	ui.NewPrinter(c.Stderr, c.Term, false).Warning("Cannot tell the name of " + who + ": " +
		strings.Join(why[:len(why)-1], ", ") + ", and " + why[len(why)-1] + ".")
}

// whoamiFiles answers what the system whoami at path prints without running
// it, when that answer is sure: on Linux, the coreutils program, with
// /etc/nsswitch.conf consulting the passwd file first and the file holding
// no NIS (+/-) lines, getpwuid(geteuid()) is the file's first entry of that
// uid. Otherwise found is false and whoami runs.
func (m *Module) whoamiFiles(path string) (name string, found bool) {
	if _, real := m.run.(runner.Exec); !real || runtime.GOOS != "linux" ||
		(path != "/usr/bin/whoami" && path != "/bin/whoami") {
		return "", false
	}
	nss, err := os.ReadFile("/etc/nsswitch.conf")
	if err != nil || !passwdFilesFirst(string(nss)) {
		return "", false
	}
	b, err := m.readFile(m.passwd)
	if err != nil {
		return "", false
	}
	uid := os.Geteuid()
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			return "", false
		}
		if f := strings.SplitN(line, ":", 4); len(f) == 4 && name == "" && f[0] != "" {
			if n, err := strconv.Atoi(f[2]); err == nil && n == uid {
				name = f[0]
			}
		}
	}
	return name, name != ""
}

// passwdFilesFirst reports whether the passwd database of an nsswitch.conf
// looks in the files first.
func passwdFilesFirst(conf string) bool {
	for _, line := range strings.Split(conf, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "passwd:" {
			return f[1] == "files"
		}
	}
	return false
}

// passwdNames is grep "^[^:]*:[^:]*:UID:" /etc/passwd | cut -d: -f1 inside
// $(...): the names of every entry whose third field is uid, one per line,
// trailing newlines removed.
func passwdNames(passwd, uid string) string {
	var names []string
	for _, line := range strings.Split(passwd, "\n") {
		f := strings.SplitN(line, ":", 4)
		if len(f) == 4 && f[2] == uid {
			names = append(names, f[0])
		}
	}
	return strings.TrimRight(strings.Join(names, "\n"), "\n")
}

// readFileOS reads a file when it is readable ([[ -r FILE ]]).
func readFileOS(name string) ([]byte, error) { return os.ReadFile(name) }
