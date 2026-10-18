// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"fmt"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

const syncUsage = "git_sync [-m MESSAGE] [--no-push] [--dry-run]"

// inProgress are the git-path markers of an operation in progress, in the
// 2.x order, with the word the message uses.
var inProgress = []struct{ path, what string }{
	{"MERGE_HEAD", "merge"},
	{"CHERRY_PICK_HEAD", "cherry-pick"},
	{"REVERT_HEAD", "revert"},
	{"rebase-merge", "rebase"},
	{"rebase-apply", "rebase"},
}

// sync is git_sync [-m MESSAGE] [--no-push] [--dry-run]: `git add --all`,
// `git commit --quiet -m MESSAGE` when there is something to commit, then
// `git push --quiet` to the upstream or `git push --quiet -u origin
// BRANCH`. No question is asked (LEDGER GIT-01) and --dry-run can promise
// a push that git refuses (GIT-07).
func (m *Module) sync(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	msg, push, dryRun := "chore: sync", true, false
	for i := 0; i < len(c.Args); i++ {
		switch a := c.Args[i]; a {
		case "-m", "--message":
			if i+1 >= len(c.Args) || c.Args[i+1] == "" {
				x.p.Error(a + " needs a message.")
				return 1
			}
			msg = c.Args[i+1]
			i++
		case "--no-push":
			push = false
		case "--dry-run":
			dryRun = true
		default:
			x.p.Error("Unknown option: " + a + " (usage: " + syncUsage + ")")
			return 1
		}
	}
	if !x.requireRepo() {
		return 1
	}

	branch, ok := x.gitOutQuiet("symbolic-ref", "--quiet", "--short", "HEAD")
	if !ok {
		x.p.Error("Detached HEAD: check out a branch first.")
		return 1
	}
	for _, st := range inProgress {
		p, _ := x.gitOut("rev-parse", "--git-path", st.path)
		if p != "" && exists(p) {
			x.p.Error("A " + st.what + " is in progress: finish or abort it first (git " + st.what + " --continue or --abort).")
			return 1
		}
	}

	upstream, ok := x.gitOutQuiet("rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if !ok {
		upstream = ""
	}
	remote := ""
	if push && upstream == "" {
		if !x.gitQuiet("remote", "get-url", "origin") {
			x.p.Error("Branch '" + branch + "' has no upstream and there is no 'origin' remote: use --no-push or add a remote.")
			return 1
		}
		remote = "origin"
	}

	changes, _ := x.gitOut("status", "--porcelain", "--untracked-files=all")

	if dryRun {
		if changes != "" {
			x.s.Heading(c.Stdout, "Would commit on "+branch+" with message \""+msg+"\":")
			x.git("status", "--short", "--untracked-files=all")
		} else {
			fmt.Fprintf(c.Stdout, "Nothing to commit on %s.\n", branch)
		}
		if push {
			if upstream != "" {
				fmt.Fprintf(c.Stdout, "Would push %s to %s.\n", branch, upstream)
			} else {
				fmt.Fprintf(c.Stdout, "Would push %s to origin/%s and set it as upstream.\n", branch, branch)
			}
		}
		return 0
	}

	if changes != "" {
		if !x.git("add", "--all").Success() {
			return 1
		}
		if !x.git("commit", "--quiet", "-m", msg).Success() {
			return 1
		}
		short, _ := x.gitOut("rev-parse", "--short", "HEAD")
		x.p.Success("Committed " + short + " on " + branch + ": " + msg)
	} else {
		x.p.Info("Nothing to commit on " + branch + ".")
	}

	if !push {
		return 0
	}
	if upstream != "" {
		if n, _ := x.gitOutQuiet("rev-list", "--count", "@{upstream}..HEAD"); n == "0" {
			x.p.Info(branch + " is already on " + upstream + ".")
			return 0
		}
		if !x.git("push", "--quiet").Success() {
			return 1
		}
		x.p.Success("Pushed " + branch + " to " + upstream + ".")
		return 0
	}
	if !x.git("push", "--quiet", "-u", remote, branch).Success() {
		return 1
	}
	x.p.Success("Pushed " + branch + " to " + remote + "/" + branch + " (upstream set).")
	return 0
}
