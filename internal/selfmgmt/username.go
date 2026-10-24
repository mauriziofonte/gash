// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const unknownUsername = "UNKNOWN_USERNAME"

// username is gash_username: whoami, then id -un, then the /etc/passwd
// entry of id -u, else UNKNOWN_USERNAME. An argument is refused (3.0:
// SELF-15 fixed; 2.x ignored it).
func (m *Module) username(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if badArgs(c, "gash_username") {
		return usageStatus
	}
	name := ""
	// if command -v whoami; then user_name=$(whoami 2>/dev/null) || true
	if _, ok := m.lookPath(c, "whoami"); ok {
		name = m.output(c, "whoami")
	}
	_, hasID := m.lookPath(c, "id")
	if name == "" && hasID {
		name = m.output(c, "id", "-un")
	}
	if name == "" && hasID {
		uid := m.output(c, "id", "-u")
		if uid != "" {
			if b, err := m.readFile(m.passwd); err == nil {
				name = passwdNames(string(b), uid)
			}
		}
	}
	if name == "" {
		name = unknownUsername
	}
	_, _ = c.Stdout.Write([]byte(name + "\n"))
	return 0
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
