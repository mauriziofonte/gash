// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// transfer describes one scp or rsync function: its name, which side the
// remote path is on, and the operation word of its safety messages.
type transfer struct {
	name      string
	srcName   string // "local source" or "remote source"
	dstName   string
	operation string
	pull      bool // the remote path is SRC
}

var (
	pushSpec     = transfer{"remote_push", "local source", "remote destination", "push", false}
	pullSpec     = transfer{"remote_pull", "remote source", "local destination", "pull (source path access)", true}
	syncPushSpec = transfer{"remote_sync_push", "local source", "remote destination", "sync-push", false}
	syncPullSpec = transfer{"remote_sync_pull", "remote source", "local destination", "sync-pull (source path access)", true}
)

// flags are the words after the three positionals. Unknown words are
// ignored, a fourth positional too (LEDGER REM-01).
type flags struct {
	recursive, preserve, noProgress, force, delete, dryRun bool
	excludes                                               []string
}

func scpFlags(words []string) flags {
	var f flags
	for _, a := range words {
		switch a {
		case "-r", "--recursive":
			f.recursive = true
		case "-p", "--preserve":
			f.preserve = true
		case "--no-progress":
			f.noProgress = true
		case "--force":
			f.force = true
		}
	}
	return f
}

func rsyncFlags(words []string) flags {
	var f flags
	for _, a := range words {
		switch {
		case a == "--delete":
			f.delete = true
		case a == "--dry-run":
			f.dryRun = true
		case a == "--no-progress":
			f.noProgress = true
		case a == "--force":
			f.force = true
		case strings.HasPrefix(a, "--exclude="):
			f.excludes = append(f.excludes, strings.TrimPrefix(a, "--exclude="))
		}
	}
	return f
}

// positionals checks the three positionals, as __gash_require_arg does, in
// order: target, source, destination.
func (x *call) positionals(t transfer) (string, string, string, bool) {
	usage := t.name + " <alias> <src> <dst>"
	input, src, dst := x.Arg(0), x.Arg(1), x.Arg(2)
	ok := x.requireArg(input, "host alias/target", usage) &&
		x.requireArg(src, t.srcName, usage) &&
		x.requireArg(dst, t.dstName, usage)
	return input, src, dst, ok
}

// scp is remote_push and remote_pull: `scp -o ConnectTimeout=10 -o
// BatchMode=yes [-r] [-p] [-q] -- SRC DST`, the remote side as
// TARGET:PATH, after the tier check of the remote path.
func (m *Module) scp(c *invoke.Call, t transfer) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	input, src, dst, ok := x.positionals(t)
	if !ok {
		return 1
	}
	f := scpFlags(c.Args[3:])
	target, ok := x.resolveTarget(input)
	if !ok {
		return 1
	}
	remotePath := dst
	if t.pull {
		remotePath = src
	}
	if !x.checkSafety(remotePath, t.operation, f.force) {
		return 1
	}
	if !x.have("scp") {
		x.p.Error("scp is not installed.")
		return 1
	}
	args := []string{"-o", "ConnectTimeout=10", "-o", "BatchMode=yes"}
	if f.recursive {
		args = append(args, "-r")
	}
	if f.preserve {
		args = append(args, "-p")
	}
	if f.noProgress {
		args = append(args, "-q")
	}
	if t.pull {
		args = append(args, "--", target+":"+src, dst)
	} else {
		args = append(args, "--", src, target+":"+dst)
	}
	return x.run(c.Stdin, c.Stdout, c.Stderr, "scp", args...).ExitCode
}

// rsync is remote_sync_push and remote_sync_pull: `rsync -az -e "<ssh
// string>" [--info=progress2] [--delete] [--dry-run] [--exclude=PAT...]
// -- SRC DST`, after the tier check of the remote path and, with
// --delete, the delete check of the destination (local for a pull,
// LEDGER REM-06).
func (m *Module) rsync(c *invoke.Call, t transfer) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	input, src, dst, ok := x.positionals(t)
	if !ok {
		return 1
	}
	f := rsyncFlags(c.Args[3:])
	target, ok := x.resolveTarget(input)
	if !ok {
		return 1
	}
	remotePath := dst
	if t.pull {
		remotePath = src
	}
	if !x.checkSafety(remotePath, t.operation, f.force) {
		return 1
	}
	if f.delete && !x.checkDeleteSafety(dst, f.force) {
		return 1
	}
	if !x.have("rsync") {
		x.p.Error("rsync is not installed.")
		return 1
	}
	args := []string{"-az", "-e", rsyncRSH(x.home())}
	if !f.noProgress {
		args = append(args, "--info=progress2")
	}
	if f.delete {
		args = append(args, "--delete")
	}
	if f.dryRun {
		args = append(args, "--dry-run")
	}
	for _, pat := range f.excludes {
		args = append(args, "--exclude="+pat)
	}
	if t.pull {
		args = append(args, "--", target+":"+src, dst)
	} else {
		args = append(args, "--", src, target+":"+dst)
	}
	return x.run(c.Stdin, c.Stdout, c.Stderr, "rsync", args...).ExitCode
}

func (m *Module) push(c *invoke.Call) int     { return m.scp(c, pushSpec) }
func (m *Module) pull(c *invoke.Call) int     { return m.scp(c, pullSpec) }
func (m *Module) syncPush(c *invoke.Call) int { return m.rsync(c, syncPushSpec) }
func (m *Module) syncPull(c *invoke.Call) int { return m.rsync(c, syncPullSpec) }
