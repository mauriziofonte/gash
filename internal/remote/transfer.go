// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// transfer describes one scp or rsync function: its name, its usage
// line, which side the remote path is on, and the operation word of its
// safety messages.
type transfer struct {
	name      string
	usage     string
	srcName   string // "local source" or "remote source"
	dstName   string
	operation string
	pull      bool // the remote path is SRC
}

const (
	scpOpts   = " [-r|--recursive] [-p|--preserve] [--no-progress] [--force]"
	rsyncOpts = " [--delete] [--exclude PAT|--exclude=PAT]... [--dry-run] [--no-progress] [--force]"
)

var (
	pushSpec = transfer{"remote_push", "remote_push <alias|user@host> <local-src> <remote-dst>" + scpOpts,
		"local source", "remote destination", "push", false}
	pullSpec = transfer{"remote_pull", "remote_pull <alias|user@host> <remote-src> <local-dst>" + scpOpts,
		"remote source", "local destination", "pull (source path access)", true}
	syncPushSpec = transfer{"remote_sync_push", "remote_sync_push <alias|user@host> <local-dir> <remote-dir>" + rsyncOpts,
		"local source", "remote destination", "sync-push", false}
	syncPullSpec = transfer{"remote_sync_pull", "remote_sync_pull <alias|user@host> <remote-dir> <local-dir>" + rsyncOpts,
		"remote source", "local destination", "sync-pull (source path access)", true}
)

// flags are the words after the three positionals.
type flags struct {
	recursive, preserve, noProgress, force, delete, dryRun bool
	excludes                                               []string
}

// parseFlags reads the words after the three positionals: the scp or the
// rsync flags, --exclude PAT (two words) as well as --exclude=PAT. An
// unknown option or a fourth positional is an error naming the usage
// line (LEDGER REM-01).
func (x *call) parseFlags(t transfer, words []string, rsync bool) (flags, bool) {
	var f flags
	for i := 0; i < len(words); i++ {
		a := words[i]
		switch {
		case !rsync && (a == "-r" || a == "--recursive"):
			f.recursive = true
		case !rsync && (a == "-p" || a == "--preserve"):
			f.preserve = true
		case a == "--no-progress":
			f.noProgress = true
		case a == "--force":
			f.force = true
		case rsync && a == "--delete":
			f.delete = true
		case rsync && a == "--dry-run":
			f.dryRun = true
		case rsync && strings.HasPrefix(a, "--exclude="):
			f.excludes = append(f.excludes, strings.TrimPrefix(a, "--exclude="))
		case rsync && a == "--exclude":
			if i+1 >= len(words) {
				x.p.Error("--exclude needs a pattern (usage: " + t.usage + ")")
				return f, false
			}
			i++
			f.excludes = append(f.excludes, words[i])
		case strings.HasPrefix(a, "-"):
			x.p.Error("Unknown option: " + a + " (usage: " + t.usage + ")")
			return f, false
		default:
			x.p.Error("Unexpected argument: " + a + " (usage: " + t.usage + ")")
			return f, false
		}
	}
	return f, true
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
	f, ok := x.parseFlags(t, c.Args[3:], false)
	if !ok {
		return 1
	}
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
	// scp starts ssh for the connection: both are looked up first.
	if !x.require(t.name, "scp", "ssh") {
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

// rsync is remote_sync_push and remote_sync_pull: `rsync -az -s -e "<ssh
// string>" [--info=progress2] [--delete] [--dry-run] [--exclude=PAT...]
// -- SRC DST`, after the tier check of the remote path and, with
// --delete, the delete check of the destination (the local one, in local
// words, for a pull: LEDGER REM-06).
func (m *Module) rsync(c *invoke.Call, t transfer) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	input, src, dst, ok := x.positionals(t)
	if !ok {
		return 1
	}
	f, ok := x.parseFlags(t, c.Args[3:], true)
	if !ok {
		return 1
	}
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
	if f.delete && !x.checkDeleteSafety(dst, f.force, t.pull) {
		return 1
	}
	// rsync starts ssh (-e) for the connection: both are looked up first.
	if !x.require(t.name, "rsync", "ssh") {
		return 1
	}
	openrs := x.openrsync()
	// -s (--protect-args): the remote shell never splits or expands the
	// remote path (LEDGER REM-03). openrsync has no -s (F-08): see
	// openrsyncArgs.
	args := []string{"-az", "-s", "-e", rsyncRSH(x.home())}
	if openrs {
		args = []string{"-az", "-e", openrsyncRSH(x.home())}
	}
	switch {
	case f.noProgress:
	case openrs:
		args = append(args, "--progress")
	default:
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
	remote := func(p string) string {
		if openrs {
			return target + ":" + remoteShellWord(p)
		}
		return target + ":" + p
	}
	if t.pull {
		args = append(args, "--", remote(src), dst)
	} else {
		args = append(args, "--", src, remote(dst))
	}
	return x.run(c.Stdin, c.Stdout, c.Stderr, "rsync", args...).ExitCode
}

// openrsync reports whether the rsync on PATH is openrsync, OpenBSD's
// rsync that macOS ships as /usr/bin/rsync: its --version prints
// "openrsync: protocol version N" (on stderr), where Samba's rsync prints
// "rsync  version ...".
func (x *call) openrsync() bool {
	var b strings.Builder
	x.run(nil, &b, &b, "rsync", "--version")
	return strings.Contains(b.String(), "openrsync")
}

// openrsync takes no -s (--protect-args) and no --info, and splits the -e
// string on blanks with no quoting (fargs.c), so the rsync functions give
// it another argv (F-08):
//
//   - -e: openrsyncRSH, the ssh options as plain words;
//   - --progress instead of --info=progress2;
//   - the remote path quoted for the remote shell (remoteShellWord).
//
// Without -s, rsync sends the remote path inside the command line that
// ssh hands to the remote user's shell, which splits it into words and
// expands it. The remote-path guard (REM-03) has already refused every
// character the shell would expand or that would end a quote ($, `, a
// quote, a backslash, glob characters, ;, &, |, <, >, (, ), !, a newline),
// so what is left to protect is blanks: the path goes inside single
// quotes, which the shell removes, and stays one word. A leading ~, ~/ or
// ~NAME/ stays outside the quotes, so the remote shell still expands it
// as it does for scp and ssh.

// openrsyncRSH is rsyncRSH for openrsync: no quotes (openrsync would pass
// them to ssh as they are). A HOME with a blank cannot be one word there,
// so the ControlPath then names the home as ssh's own "~".
func openrsyncRSH(home string) string {
	cp := "ControlPath=" + home + "/.ssh/cm-%C"
	if strings.ContainsAny(home, " \t") {
		cp = "ControlPath=~/.ssh/cm-%C"
	}
	return "ssh -o BatchMode=yes -o ConnectTimeout=10 -o ServerAliveInterval=15 -o ServerAliveCountMax=4 -o ControlMaster=auto -o " +
		cp + " -o ControlPersist=60s"
}

// remoteShellWord is the remote path p as one word of the remote shell:
// a leading tilde prefix (~, ~NAME, with the / after it) left for the
// shell to expand, the rest single-quoted (a single quote in it, which
// REM-03 never lets through, would close the quotes, be escaped with a
// backslash and reopen them).
func remoteShellWord(p string) string {
	head, rest := "", p
	if strings.HasPrefix(p, "~") {
		end := strings.IndexByte(p, '/')
		if end < 0 {
			end = len(p)
		}
		if name := p[1:end]; strings.Trim(name, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789._-") == "" {
			// The / stays unquoted too: a quoted one would make the
			// whole word the tilde prefix, which the shell leaves alone.
			end = min(end+1, len(p))
			head, rest = p[:end], p[end:]
		}
	}
	if rest == "" {
		return head
	}
	return head + "'" + strings.ReplaceAll(rest, "'", `'\''`) + "'"
}

func (m *Module) push(c *invoke.Call) int     { return m.scp(c, pushSpec) }
func (m *Module) pull(c *invoke.Call) int     { return m.scp(c, pullSpec) }
func (m *Module) syncPush(c *invoke.Call) int { return m.rsync(c, syncPushSpec) }
func (m *Module) syncPull(c *invoke.Call) int { return m.rsync(c, syncPullSpec) }
