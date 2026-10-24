// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// listTags is git_list_tags: `git tag -l` and the second field of every
// `git ls-remote --tags --refs origin` line with the first refs/tags/
// removed, each as a one-column table. --refs leaves out the peeled ^{}
// lines, and a failing ls-remote is an error with status 1 (LEDGER
// GIT-02). Extra arguments are ignored.
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

	res = x.cmd(nil, c.Stderr, "git", "ls-remote", "--tags", "--refs", "origin")
	if !res.Success() {
		x.p.Error("Cannot list the tags of origin.")
		return 1
	}
	x.s.Heading(c.Stdout, "Remote tags:")
	x.s.Table(c.Stdout, ui.ReadRows(strings.NewReader(remoteTagLines(res.Stdout))), "--no-header", "--color", "1:accent")
	return 0
}

const (
	addTagUsage    = "git_add_tag <tag_name> [message] [-y|--yes] [--dry-run]"
	deleteTagUsage = "git_delete_tag <tag_name> [-y|--yes] [--dry-run]"
)

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

// gateArgs parses the words of git_add_tag, git_delete_tag and
// git_apply_patch: -y/--yes
// and --dry-run anywhere, -- ends the flags, every other word is a
// positional (an option-like one is then refused as a tag name, LEDGER
// GIT-04).
func gateArgs(args []string) (pos []string, yes, dryRun bool) {
	flags := true
	for _, a := range args {
		switch {
		case flags && a == "--":
			flags = false
		case flags && (a == "-y" || a == "--yes"):
			yes = true
		case flags && a == "--dry-run":
			dryRun = true
		default:
			pos = append(pos, a)
		}
	}
	return pos, yes, dryRun
}

// arg is the nth positional, "" when there is none.
func arg(pos []string, n int) string {
	if n < len(pos) {
		return pos[n]
	}
	return ""
}

// addTag is git_add_tag TAG [MESSAGE] [-y|--yes] [--dry-run]: exact
// local and remote checks (an unreachable origin is an error before any
// change, LEDGER GIT-03), then, after the confirmation (GIT-01), `git
// tag -a TAG -m MESSAGE` and `git push origin refs/tags/TAG`.
func (m *Module) addTag(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	pos, yes, dryRun := gateArgs(c.Args)
	if !x.requireRepoWithRemote() {
		return 1
	}
	tag := arg(pos, 0)
	if !x.requireArg(tag, "tag name", addTagUsage) {
		return 1
	}
	msg := arg(pos, 1)
	if msg == "" {
		msg = "Release " + tag
	}
	if !x.checkTagName(tag) {
		return 1
	}
	if x.localTag(tag) {
		x.p.Error("Tag '" + tag + "' already exists.")
		return 1
	}
	remote, ok := x.remoteTag(tag)
	if !ok {
		return 1
	}
	if remote {
		x.p.Error("Tag '" + tag + "' already exists.")
		return 1
	}
	url := x.originURL()
	if dryRun {
		fmt.Fprintf(c.Stdout, "Would create tag '%s' and push refs/tags/%s to origin (%s).\n", tag, tag, url)
		return 0
	}
	if !x.confirm(yes, "push tag '"+tag+"'", "Will push refs/tags/"+tag+" to origin ("+url+")") {
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

// deleteTag is git_delete_tag TAG [-y|--yes] [--dry-run]: the local tag
// (`git tag -d`) and the remote one (`git push origin --delete
// refs/tags/TAG`), each when it exists, after the confirmation (LEDGER
// GIT-01); an unreachable origin is an error before any change (GIT-03).
// Extra words are ignored.
func (m *Module) deleteTag(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	pos, yes, dryRun := gateArgs(c.Args)
	if !x.requireRepoWithRemote() {
		return 1
	}
	tag := arg(pos, 0)
	if !x.requireArg(tag, "tag name", deleteTagUsage) {
		return 1
	}
	if !x.checkTagName(tag) {
		return 1
	}
	local := x.localTag(tag)
	remote, ok := x.remoteTag(tag)
	if !ok {
		return 1
	}
	if !local && !remote {
		x.p.Error("Tag '" + tag + "' does not exist.")
		return 1
	}
	where := "locally"
	switch url := x.originURL(); {
	case local && remote:
		where = "locally and on origin (" + url + ")"
	case remote:
		where = "on origin (" + url + ")"
	}
	if dryRun {
		fmt.Fprintf(c.Stdout, "Would delete refs/tags/%s %s.\n", tag, where)
		return 0
	}
	if !x.confirm(yes, "delete tag '"+tag+"'", "Will delete refs/tags/"+tag+" "+where) {
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

// checkTagName is __gash_git_check_tag_name, which also refuses a name
// starting with '-', so no tag name reaches git in option position
// (LEDGER GIT-04).
func (x *call) checkTagName(tag string) bool {
	if !strings.HasPrefix(tag, "-") && x.cmd(x.Stdout, io.Discard, "git", "check-ref-format", "refs/tags/"+tag).Success() {
		return true
	}
	x.p.Error("Invalid tag name: '" + tag + "'.")
	return false
}

// localTag is __gash_git_local_tag.
func (x *call) localTag(tag string) bool {
	return x.gitQuiet("rev-parse", "-q", "--verify", "refs/tags/"+tag)
}

// remoteTag is __gash_git_remote_tag with the ls-remote status read
// exactly (LEDGER GIT-03): 0 present, 2 absent, anything else `Error:
// Cannot reach origin (URL): nothing was created or deleted.` and ok
// false.
func (x *call) remoteTag(tag string) (present, ok bool) {
	switch x.cmd(io.Discard, io.Discard, "git", "ls-remote", "--exit-code", "--tags", "origin", "refs/tags/"+tag).ExitCode {
	case 0:
		return true, true
	case 2:
		return false, true
	}
	x.p.Error("Cannot reach origin (" + x.originURL() + "): nothing was created or deleted.")
	return false, false
}

// originURL is `git remote get-url origin`.
func (x *call) originURL() string {
	u, _ := x.gitOutQuiet("remote", "get-url", "origin")
	return u
}
