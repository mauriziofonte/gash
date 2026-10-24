// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const logRule = "*******************************************************\n"

// dumpRevisions is git_dump_revisions FILE: for every short hash of `git
// log --pretty=format:%h -- PATH` (newest first, numbered from 001), the
// file at that commit in FILE.NNN.HASH and its log message in
// FILE.NNN.HASH.logmsg. PATH is FILE's work-tree path (`git ls-files
// --full-name --error-unmatch -- FILE`), read as `git show HASH:PATH`, so
// a file named like a branch and an absolute FILE work; any git failure
// is `Error: git failed on 'FILE'.` with status 1, and every output is
// written to a temporary file then renamed, so no empty file is left
// (LEDGER GIT-05). The caller's $commit is never touched (GIT-09).
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
	failed := func() int {
		x.p.Error("git failed on '" + file + "'.")
		return 1
	}
	path, ok := x.gitOut("ls-files", "--full-name", "--error-unmatch", "--", file)
	if !ok || path == "" || strings.Contains(path, "\n") {
		return failed()
	}
	hashes, ok := x.gitOut("log", "--pretty=format:%h", "--", file)
	if !ok {
		return failed()
	}
	for i, commit := range strings.Fields(hashes) {
		index := i + 1
		out := file + "." + fmt.Sprintf("%03d", index) + "." + commit
		log := out + ".logmsg"
		x.p.Info("Saving version " + strconv.Itoa(index) + " to file " + out + " for commit " + commit)

		var msg bytes.Buffer
		msg.WriteString(logRule)
		if !x.cmd(&msg, c.Stderr, "git", "log", "-1", "--pretty=format:%s%nAuthored by %an at %ai%n%n%b%n", commit).Success() {
			return failed()
		}
		msg.WriteString(logRule)
		var body bytes.Buffer
		if !x.cmd(&body, c.Stderr, "git", "show", commit+":"+path).Success() {
			return failed()
		}
		if err := writeRenamed(log, msg.Bytes()); err != nil {
			x.p.Error("Cannot write " + log + ": " + err.Error())
			return 1
		}
		if err := writeRenamed(out, body.Bytes()); err != nil {
			x.p.Error("Cannot write " + out + ": " + err.Error())
			return 1
		}
	}
	return 0
}

// writeRenamed writes data to a new temporary file next to path, created
// as a shell redirection creates one (mode 0666 less the umask), and
// renames it to path.
func writeRenamed(path string, data []byte) error {
	var f *os.File
	var err error
	for i := 0; ; i++ {
		f, err = os.OpenFile(path+".tmp-"+strconv.Itoa(os.Getpid())+"-"+strconv.Itoa(i), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o666)
		if !errors.Is(err, fs.ErrExist) || i == 100 {
			break
		}
	}
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}
