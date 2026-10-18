// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const logRule = "*******************************************************\n"

// dumpRevisions is git_dump_revisions FILE: for every short hash of `git
// log --pretty=format:%h FILE` (newest first, numbered from 001), the
// file at that commit in FILE.NNN.HASH and its log message in
// FILE.NNN.HASH.logmsg. The status is 0 whatever git reports, FILE is not
// preceded by -- and is read as ./FILE in the commit (LEDGER GIT-05). The
// caller's $commit is never touched (GIT-09).
func (m *Module) dumpRevisions(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	file := c.Arg(0)
	if !x.requireArg(file, "filename", "git_dump_revisions <filename>") {
		return 1
	}
	if !x.requireRepo() {
		return 1
	}
	if !isFile(file) {
		x.p.Error("File '" + file + "' does not exist in the repository.")
		return 1
	}

	hashes, _ := x.gitOut("log", "--pretty=format:%h", file)
	// for commit in $(...): unquoted, split on blanks and newlines (hashes
	// hold no glob characters).
	for i, commit := range strings.Fields(hashes) {
		index := i + 1
		out := file + "." + fmt.Sprintf("%03d", index) + "." + commit
		log := out + ".logmsg"
		x.p.Info("Saving version " + strconv.Itoa(index) + " to file " + out + " for commit " + commit)

		// echo RULE >"$log"; git log -1 ... >>"$log"; echo RULE >>"$log"
		if f, ok := redirect(log, false); ok {
			_, _ = io.WriteString(f, logRule)
			_ = f.Close()
		}
		if f, ok := redirect(log, true); ok {
			x.cmd(f, c.Stderr, "git", "log", "-1", "--pretty=format:%s%nAuthored by %an at %ai%n%n%b%n", commit)
			_ = f.Close()
		}
		if f, ok := redirect(log, true); ok {
			_, _ = io.WriteString(f, logRule)
			_ = f.Close()
		}
		// git show "$commit:./$file" >"$out"
		if f, ok := redirect(out, false); ok {
			x.cmd(f, c.Stderr, "git", "show", commit+":./"+file)
			_ = f.Close()
		}
	}
	return 0
}

// redirect opens path as the shell's > (truncate) or >> (append) does:
// created with mode 0666 less the umask, symlinks followed. When the
// open fails the shell skips the command; Bash's diagnostic, which names
// a 2.x script line, is not printed (LEDGER DOCK-19).
func redirect(path string, appendTo bool) (*os.File, bool) {
	flag := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if appendTo {
		flag = os.O_WRONLY | os.O_CREATE | os.O_APPEND
	}
	f, err := os.OpenFile(path, flag, 0o666)
	if err != nil {
		return nil, false
	}
	return f, true
}
