// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/proctab"
	"github.com/mauriziofonte/gash/internal/registry"
)

// killCmdWidth cuts the command lines a dry run reports, as llm_procs.
const killCmdWidth = 200

// killer holds the seams of agent_kill: the process table, the calling
// process and the signal itself (the tests fake all three).
type killer struct {
	table  func(c *invoke.Call) (*proctab.Table, error)
	getpid func() int
	signal func(pid int, sig syscall.Signal) error
}

// newKiller wires agent_kill to the system: ps through the module's runner
// (or /proc), kill(2).
func (m *Module) newKiller() *killer {
	return &killer{
		table: func(c *invoke.Call) (*proctab.Table, error) {
			ps, _ := prereq.Locate(c, "ps")
			return proctab.Snapshot(c.Ctx, m.run, ps, append(c.Env.Child(), "LC_ALL=C"), proctab.Root())
		},
		getpid: m.getpid,
		signal: syscall.Kill,
	}
}

// kill is agent_kill NAME|PID... [--signal S] [--dry-run] (plan P2, A1):
// the write half of llm_procs. A PID target is that process; a NAME target
// is a regular expression on the full command line, as llm_procs --name
// matches it, leaving out the caller's own chain. Only the caller's own
// user's processes are signaled, never PID 1, never the caller or one of
// its ancestors: an explicit PID breaking those rules is kill_refused
// (FATAL), a NAME simply never matches them. Nothing matched is success
// with killed:0 (pkill exits 1 there).
func (m *Module) kill(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	return m.newKiller().run(c)
}

// killTarget is one command-line target: a PID or a NAME pattern.
type killTarget struct {
	pid  int
	name *regexp.Regexp
}

func (k *killer) run(c *invoke.Call) int {
	o := llmfmt.New(llmfmt.FormatFlag)
	sig, sigName := syscall.SIGTERM, "TERM"
	dry := false
	var targets []killTarget
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch {
		case a == "--signal" || a == "-s":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a signal: TERM, KILL, HUP, INT, a number")
			}
			s, name, ok := parseSignal(args[1])
			if !ok {
				return fail(c, "invalid_signal", args[1], "Give a signal name (TERM, KILL, HUP, INT, USR1, ...) or its number (1-64); 0 is a liveness check: use llm_procs --pid")
			}
			sig, sigName = s, name
			args = args[2:]
			continue
		case a == "--dry-run":
			dry = true
		case a == "--":
			for _, t := range args[1:] {
				kt, ok := parseKillTarget(c, t)
				if !ok {
					return 1
				}
				targets = append(targets, kt)
			}
			args = nil
			continue
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
		default:
			kt, ok := parseKillTarget(c, a)
			if !ok {
				return 1
			}
			targets = append(targets, kt)
		}
		args = args[1:]
	}
	if len(targets) == 0 {
		return fail(c, "missing_target", "", "Name the processes: a PID, or a NAME matched against the full command line (preview with llm_procs --pid or --name); there is no default \"all\"")
	}

	tab, err := k.table(c)
	if err != nil {
		// agent_kill checks every target in the process table before
		// signaling: without one nothing is signaled.
		return prereq.Fail(c, "ps", "agent_kill", true)
	}
	self := k.getpid()
	uid := -1
	if p, ok := tab.Lookup(self); ok {
		uid = p.UID
	}
	if uid < 0 {
		return fail(c, "ps_not_found", "The calling process is not in the process table", "Retry; agent_kill needs the caller's own row to know its user")
	}
	ancestors := tab.Ancestors(self)
	callers := tab.Callers(self)

	var hit []proctab.Proc
	seen := map[int]bool{}
	add := func(p proctab.Proc) {
		if !seen[p.PID] && p.Alive() {
			seen[p.PID] = true
			hit = append(hit, p)
		}
	}
	for _, t := range targets {
		if t.name != nil {
			for _, p := range tab.Match(t.name.MatchString, callers) {
				if p.UID == uid && p.PID > 1 {
					add(p)
				}
			}
			continue
		}
		p, ok := tab.Lookup(t.pid)
		switch {
		case t.pid == 1:
			return fail(c, "kill_refused", "1", "PID 1 (init) is never signaled")
		case ancestors[t.pid]:
			what := "an ancestor of this call (the shell or agent that runs it)"
			if t.pid == self {
				what = "this gash process"
			}
			return fail(c, "kill_refused", strconv.Itoa(t.pid)+" is "+what, "agent_kill never signals the caller or its ancestors")
		case !ok:
			continue // gone already: nothing to kill
		case p.UID != uid:
			return fail(c, "kill_refused", strconv.Itoa(t.pid)+" belongs to uid "+strconv.Itoa(p.UID), "agent_kill signals only your own user's processes; ask the user")
		}
		add(p)
	}

	if dry {
		return writeKillReport(c, o, hit, len(hit), sigName, true)
	}
	var killed []proctab.Proc
	var failed []string
	for _, p := range hit {
		err := k.signal(p.PID, sig)
		switch {
		case err == nil:
			killed = append(killed, p)
		case errors.Is(err, syscall.ESRCH):
			// It exited between the listing and the signal.
		default:
			failed = append(failed, strconv.Itoa(p.PID)+": "+err.Error())
		}
	}
	rc := writeKillReport(c, o, killed, len(killed), sigName, false)
	if len(failed) > 0 {
		return fail(c, "kill_failed", strings.Join(failed, "; "), "The other targets were signaled (see stdout); ask the user about these")
	}
	return rc
}

// parseKillTarget reads one target: digits are a PID, anything else a
// NAME pattern, which must not match an empty command line (".*" would be
// "everything").
func parseKillTarget(c *invoke.Call, a string) (killTarget, bool) {
	if a != "" && strings.Trim(a, "0123456789") == "" {
		pid, err := strconv.Atoi(a)
		if err != nil || pid < 1 || len(a) > 9 {
			fail(c, "invalid_pid", a, "A PID is a positive integer")
			return killTarget{}, false
		}
		return killTarget{pid: pid}, true
	}
	re, err := regexp.Compile(a)
	if err != nil {
		fail(c, "invalid_regex", a, "A NAME is a regular expression matched against the full command line, as llm_procs --name")
		return killTarget{}, false
	}
	if re.MatchString("") {
		fail(c, "pattern_too_broad", a, "A NAME must match part of a command line, not every process: name the program (e.g. 'php artisan serve')")
		return killTarget{}, false
	}
	return killTarget{name: re}, true
}

// parseSignal reads --signal: a name with or without SIG, any case, or a
// number; the name returned is without SIG.
func parseSignal(v string) (syscall.Signal, string, bool) {
	if n, err := strconv.Atoi(v); err == nil {
		if n < 1 || n > 64 {
			return 0, "", false
		}
		name := unix.SignalName(syscall.Signal(n))
		if name == "" {
			return 0, "", false
		}
		return syscall.Signal(n), strings.TrimPrefix(name, "SIG"), true
	}
	name := strings.TrimPrefix(strings.ToUpper(v), "SIG")
	if name == "" {
		return 0, "", false
	}
	s := unix.SignalNum("SIG" + name)
	if s == 0 {
		return 0, "", false
	}
	return s, name, true
}

// writeKillReport prints {"killed":N,"pids":[...]}; a dry run adds the
// command lines and "dry_run":true,"written":false. Other formats give one
// row per process with the counts on stderr.
func writeKillReport(c *invoke.Call, o *llmfmt.Options, procs []proctab.Proc, n int, sig string, dry bool) int {
	if o.Format != llmfmt.Default && o.Format != llmfmt.JSON {
		rows := make([][]any, 0, len(procs))
		for _, p := range procs {
			rows = append(rows, []any{p.PID, p.Name, llmfmt.Cut(p.Cmd, killCmdWidth)})
		}
		meta := map[string]any{"killed": n, "signal": sig}
		if dry {
			meta["dry_run"], meta["written"] = true, false
		}
		o.Write(c, o.Format, llmfmt.Table{Cols: []string{"pid", "name", "cmd"}, Rows: rows}, meta)
		return 0
	}
	var b strings.Builder
	b.WriteString(`{"killed":` + strconv.Itoa(n) + `,"pids":[`)
	for i, p := range procs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(p.PID))
	}
	b.WriteByte(']')
	if dry {
		b.WriteString(`,"cmds":[`)
		for i, p := range procs {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(llmfmt.Quote(llmfmt.Cut(p.Cmd, killCmdWidth)))
		}
		b.WriteString(`],"signal":` + llmfmt.Quote(sig) + `,"dry_run":true,"written":false`)
	}
	b.WriteString("}\n")
	_, _ = c.Stdout.Write([]byte(b.String()))
	return 0
}
