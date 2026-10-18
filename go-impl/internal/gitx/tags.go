// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// listTags is git_list_tags: `git tag -l` and the second field of every
// `git ls-remote --tags origin` line with the first refs/tags/ removed,
// each as a one-column table. Peeled ^{} lines are listed and a failing
// ls-remote still returns 0 (LEDGER GIT-02). Extra arguments are ignored.
func (m *Module) listTags(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	if !x.requireRepoWithRemote() {
		return 1
	}

	x.s.Heading(c.Stdout, "Local tags:")
	res := x.cmd(nil, c.Stderr, "git", "tag", "-l")
	x.s.Table(c.Stdout, ui.ReadRows(bytes.NewReader(res.Stdout)), "--no-header", "--color", "1:accent")

	x.s.Heading(c.Stdout, "Remote tags:")
	res = x.cmd(nil, c.Stderr, "git", "ls-remote", "--tags", "origin")
	x.s.Table(c.Stdout, ui.ReadRows(strings.NewReader(remoteTagLines(res.Stdout))), "--no-header", "--color", "1:accent")
	return 0
}

// remoteTagLines is `awk '{print $2}' | sed 's|refs/tags/||'`: one line
// per input line (awk's default field splitting on blanks, tabs and
// newlines, an empty line for a line with fewer than two fields), the
// first refs/tags/ of each removed.
func remoteTagLines(out []byte) string {
	if len(out) == 0 {
		return ""
	}
	s := string(out)
	s = strings.TrimSuffix(s, "\n")
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		f := awkFields(line)
		field := ""
		if len(f) >= 2 {
			field = f[1]
		}
		b.WriteString(strings.Replace(field, "refs/tags/", "", 1))
		b.WriteByte('\n')
	}
	return b.String()
}

// awkFields splits a record as awk's default FS does: runs of blanks and
// tabs separate fields, leading and trailing ones are ignored.
func awkFields(line string) []string {
	return strings.FieldsFunc(line, func(r rune) bool { return r == ' ' || r == '\t' })
}

// addTag is git_add_tag TAG [MESSAGE]: exact local and remote checks,
// `git tag -a TAG -m MESSAGE`, then `git push origin refs/tags/TAG`,
// without a question (LEDGER GIT-01); an unreachable origin reads as "tag
// absent" (GIT-03) and option-like names reach git tag (GIT-04).
func (m *Module) addTag(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	if !x.requireRepoWithRemote() {
		return 1
	}
	if !x.requireArg(c.Arg(0), "tag name", "git_add_tag <tag_name> [message]") {
		return 1
	}
	tag := c.Arg(0)
	msg := c.Arg(1)
	if msg == "" {
		msg = "Release " + tag
	}
	if !x.checkTagName(tag) {
		return 1
	}
	if x.localTag(tag) || x.remoteTag(tag) {
		x.p.Error("Tag '" + tag + "' already exists.")
		return 1
	}
	if !x.git("tag", "-a", tag, "-m", msg).Success() {
		x.p.Error("Could not create tag '" + tag + "'.")
		return 1
	}
	x.p.Success("Tag '" + tag + "' created.")
	if !x.git("push", "origin", "refs/tags/"+tag).Success() {
		x.p.Error("Tag '" + tag + "' was created locally but could not be pushed.")
		return 1
	}
	x.p.Success("Tag '" + tag + "' pushed to remote.")
	return 0
}

// deleteTag is git_delete_tag TAG: the local tag (`git tag -d`) and the
// remote one (`git push origin --delete refs/tags/TAG`), each when it
// exists, without a question (LEDGER GIT-01, GIT-03). Extra words are
// ignored.
func (m *Module) deleteTag(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	if !x.requireRepoWithRemote() {
		return 1
	}
	if !x.requireArg(c.Arg(0), "tag name", "git_delete_tag <tag_name>") {
		return 1
	}
	tag := c.Arg(0)
	if !x.checkTagName(tag) {
		return 1
	}
	local, remote := x.localTag(tag), x.remoteTag(tag)
	if !local && !remote {
		x.p.Error("Tag '" + tag + "' does not exist.")
		return 1
	}
	rc := 0
	if local {
		if x.cmd(io.Discard, c.Stderr, "git", "tag", "-d", tag).Success() {
			x.p.Success("Local tag '" + tag + "' deleted.")
		} else {
			rc = 1
		}
	}
	if remote {
		if x.git("push", "origin", "--delete", "refs/tags/"+tag).Success() {
			x.p.Success("Remote tag '" + tag + "' deleted.")
		} else {
			x.p.Error("Could not delete remote tag '" + tag + "'.")
			rc = 1
		}
	}
	return rc
}

// checkTagName is __gash_git_check_tag_name.
func (x *call) checkTagName(tag string) bool {
	if x.cmd(x.Stdout, io.Discard, "git", "check-ref-format", "refs/tags/"+tag).Success() {
		return true
	}
	x.p.Error("Invalid tag name: '" + tag + "'.")
	return false
}

// localTag is __gash_git_local_tag.
func (x *call) localTag(tag string) bool {
	return x.gitQuiet("rev-parse", "-q", "--verify", "refs/tags/"+tag)
}

// remoteTag is __gash_git_remote_tag: any non-zero ls-remote status reads
// as absent (LEDGER GIT-03).
func (x *call) remoteTag(tag string) bool {
	return x.gitQuiet("ls-remote", "--exit-code", "--tags", "origin", "refs/tags/"+tag)
}
