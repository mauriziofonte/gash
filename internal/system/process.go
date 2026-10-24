// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// processFind is process_find: `ps aux | grep -i -- NAME | grep -v grep`
// (GNU BRE, SYS-38), as a PID/USER/%CPU/command table; no match is an
// error line with status 0 (SYS-04). The binary's own row is left out.
func (m *Module) processFind(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	name := c.Arg(0)
	if name == "" {
		p.Error("Missing process name. Usage: process_find <process_name>")
		return 1
	}
	res, _ := m.run.Run(ctx(c), runner.Cmd{Path: "ps", Args: []string{"aux"},
		Options: runner.Options{Env: c.Env.Child(), Stderr: c.Stderr}})
	ps := strings.ReplaceAll(string(res.Stdout), "\x00", "")
	var rows []string
	re, err := compileBRE(name)
	if gerr, ok := err.(*grepError); ok {
		write(c.Stderr, "grep: "+gerr.msg+"\n")
	} else if err != nil {
		p.Error("Invalid pattern: " + name)
		return 1
	} else {
		self := strconv.Itoa(m.self)
		for _, l := range grepData(c.Stderr, re, ps, "(standard input)", "") {
			f := awkFields(l)
			if strings.Contains(l, "grep") || field(f, 1) == self {
				continue
			}
			rows = append(rows, field(f, 1)+"\t"+field(f, 0)+"\t"+field(f, 2)+"\t"+field(f, 10))
		}
	}
	if len(rows) == 0 {
		p.Error("No process found with name '" + name + "'.")
		return 0
	}
	p.Info("Processes matching " + name + ":")
	ui.StyleOf(c.Term, false).Table(c.Stdout, rows, "--no-header", "--indent", "3", "--gap", "1",
		"--min", "1:8", "--min", "2:12", "--min", "3:4", "--color", "1:accent", "--color", "2:muted")
	return 0
}

// killArgs is the option loop of process_kill and port_kill: -y, --yes,
// -f and --force skip the question, another word starting with "-" is an
// error, the last other word wins (SYS-10).
func killArgs(c *invoke.Call, p ui.Printer) (force bool, operand string, ok bool) {
	for _, a := range c.Args {
		switch {
		case a == "-y" || a == "--yes" || a == "-f" || a == "--force":
			force = true
		case strings.HasPrefix(a, "-"):
			p.Error("Unknown option: " + a)
			return false, "", false
		default:
			operand = a
		}
	}
	return force, operand, true
}

// processKill is process_kill: the processes named NAME exactly, ignoring
// case (pgrepName), the plan and the question unless -y, then TERM, a
// three-second poll and KILL for the survivors.
func (m *Module) processKill(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	force, name, ok := killArgs(c, p)
	if !ok {
		return 1
	}
	if name == "" {
		p.Error("Missing process name. Usage: process_kill [-y] <process_name>")
		return 1
	}
	pids := m.dropSelf(m.pgrepName(c, name))
	if len(pids) == 0 {
		p.Error("No process found with exact name '" + name + "'. Use process_find (pf) to inspect candidates.")
		return 1
	}
	count := len(pids)
	if !force {
		if c.Env.Get("GASH_HEADLESS") == "1" || !hasTTY(c) {
			p.Error(strconv.Itoa(count) + " process(es) match '" + name + "' (PID " + strings.Join(pids, " ") +
				"). Refusing to kill without a terminal to confirm; pass -y.")
			return 1
		}
		p.Info("Matching '" + name + "':")
		var plan []string
		for _, pid := range pids {
			cmd, st := m.output(c, "ps", "-o", "args=", "-p", pid)
			if st != 0 {
				cmd = "?"
			}
			plan = append(plan, pid+"\t"+strings.ReplaceAll(ui.Sub(cmd, 0, 100), "\t", " "))
		}
		m.planTable(c, plan)
		q := "Kill it?"
		if count > 1 {
			q = "Kill all " + strconv.Itoa(count) + "?"
		}
		if !ui.StyleOf(c.Term, false).Confirm(c.Term, c.Stderr, q) {
			p.Info("Aborted.")
			return 1
		}
	}
	if !m.terminate(pids, func(pid string, outcome int) {
		switch outcome {
		case outKilled:
			p.Info("Process " + pid + " ignored SIGTERM; killed with SIGKILL.")
		case outDenied:
			p.Error(deniedMsg(pid))
		default:
			p.Info("Process " + pid + " terminated.")
		}
	}) {
		return 1
	}
	return 0
}

// commLen is the length of the kernel's process name (comm), which pgrep
// matches: longer names are truncated to it.
const commLen = 15

// pgrepName lists the PIDs whose name is NAME, ignoring case (LEDGER
// SYS-01: NAME is a name, not a pattern). Every ERE metacharacter is
// escaped for `pgrep -ix --`; a NAME longer than the 15-byte comm is
// looked up by its first 15 bytes and kept only where argv[0]'s base name
// (from /proc/PID/cmdline, else `ps -o args=`) equals NAME ignoring case.
func (m *Module) pgrepName(c *invoke.Call, name string) []string {
	key := name
	if len(key) > commLen {
		key = key[:commLen]
	}
	res, _ := m.run.Run(ctx(c), runner.Cmd{Path: "pgrep", Args: []string{"-ix", "--", escapeERE(key)},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	pids := strings.Fields(runner.CommandSubst(res.Stdout))
	if len(name) <= commLen {
		return pids
	}
	var out []string
	for _, pid := range pids {
		if strings.EqualFold(m.argv0Base(c, pid), name) {
			out = append(out, pid)
		}
	}
	return out
}

// argv0Base is the base name of PID's argv[0]: /proc/PID/cmdline, or the
// first word of `ps -o args= -p PID` where /proc is absent.
func (m *Module) argv0Base(c *invoke.Call, pid string) string {
	arg0 := ""
	if b, err := m.sys.ReadFile("/proc/" + pid + "/cmdline"); err == nil {
		arg0, _, _ = strings.Cut(string(b), "\x00")
	} else if out, st := m.output(c, "ps", "-o", "args=", "-p", pid); st == 0 {
		if f := strings.Fields(out); len(f) > 0 {
			arg0 = f[0]
		}
	}
	if i := strings.LastIndexByte(arg0, '/'); i >= 0 {
		arg0 = arg0[i+1:]
	}
	return arg0
}

// escapeERE backslash-escapes every POSIX ERE metacharacter of s.
func escapeERE(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(`\.[]()*+?{}|^$`, s[i]) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

// hasTTY is `{ : </dev/tty; } 2>/dev/null`: a controlling terminal opens.
func hasTTY(c *invoke.Call) bool {
	f, err := c.Term.OpenTTY()
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

// dropSelf removes the binary's own PID (a process 2.x did not have).
func (m *Module) dropSelf(pids []string) []string {
	self := strconv.Itoa(m.self)
	out := pids[:0:0]
	for _, p := range pids {
		if p != self {
			out = append(out, p)
		}
	}
	return out
}

// planTable prints the PID table shown before a kill question, on stderr:
// colors only where stderr may carry them (__gash_stderr_fancy), as the
// 2.x `__gash_table ... >&2` decided on its redirected stdout.
func (m *Module) planTable(c *invoke.Call, rows []string) {
	s := ui.StyleOf(c.Term, false)
	s.Color = s.StderrFancy()
	args := []string{"--no-header", "--indent", "2"}
	if s.Color {
		args = append(args, "--color", "1:accent")
	}
	s.Table(c.Stderr, rows, args...)
}

// signal is the kill builtin on one PID word: kill(2) for a positive PID,
// a failure for anything else (never a process group).
func (m *Module) signal(pid string, sig syscall.Signal) error {
	n, err := strconv.Atoi(pid)
	if err != nil || n <= 0 {
		return syscall.EINVAL
	}
	return m.kill(n, sig)
}

// Outcomes of terminate for one PID.
const (
	outTerminated = iota // gone after TERM (or already gone)
	outKilled            // still alive after the poll, sent KILL
	outDenied            // the user may not signal it (EPERM)
)

// terminate sends TERM to every PID, polls them with signal 0 every 0.1 s
// for up to 3 s, then KILLs the survivors. A PID that refuses a signal
// with EPERM is not terminated (LEDGER SYS-02): it is reported as denied
// and left out of the poll; ESRCH means it is gone. report gets each PID
// in order; terminate returns false when any PID was denied.
func (m *Module) terminate(pids []string, report func(pid string, outcome int)) bool {
	denied := map[string]bool{}
	for _, pid := range pids {
		if m.signal(pid, syscall.SIGTERM) == syscall.EPERM {
			denied[pid] = true
		}
	}
	probe := func(pid string) bool {
		err := m.signal(pid, 0)
		if err == syscall.EPERM {
			denied[pid] = true
			return false
		}
		return err == nil
	}
	for waited := 0; waited < 30; waited++ {
		alive := false
		for _, pid := range pids {
			if !denied[pid] && probe(pid) {
				alive = true
			}
		}
		if !alive {
			break
		}
		m.sleep(100 * time.Millisecond)
	}
	ok := true
	for _, pid := range pids {
		alive := !denied[pid] && probe(pid)
		if alive && m.signal(pid, syscall.SIGKILL) == syscall.EPERM {
			denied[pid] = true
		}
		switch {
		case denied[pid]:
			ok = false
			report(pid, outDenied)
		case alive:
			report(pid, outKilled)
		default:
			report(pid, outTerminated)
		}
	}
	return ok
}

// deniedMsg is the SYS-02 error of a process the user may not signal.
func deniedMsg(pid string) string { return "Process " + pid + ": permission denied." }

// portKill is port_kill: the lsof TCP listeners and the UDP sockets bound
// to PORT (udpListeners) merged as `sort -un`, the plan, the question
// unless -y, then the kill sequence of process_kill.
func (m *Module) portKill(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	force, port, ok := killArgs(c, p)
	if !ok {
		return 1
	}
	if port == "" {
		p.Error("Missing port. Usage: port_kill [-y] <port>")
		return 1
	}
	// Digits as a decimal number without wraparound (SYS-33).
	n, err := strconv.ParseUint(port, 10, 64)
	if !isDigits(port) || err != nil || n < 1 || n > 65535 {
		p.Error("Invalid port: " + port + " (expected 1-65535).")
		return 1
	}
	port = strconv.FormatUint(n, 10)
	if !found(c, "lsof") {
		p.Error("This function requires 'lsof', which is not available.")
		return 1
	}
	tcp, _ := m.run.Run(ctx(c), runner.Cmd{Path: "lsof", Args: []string{"-t", "-nP", "-iTCP:" + port, "-sTCP:LISTEN"},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	udp, _ := m.run.Run(ctx(c), runner.Cmd{Path: "lsof", Args: []string{"-nP", "-iUDP:" + port, "-F", "pn"},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	pids := m.dropSelf(sortUN(string(tcp.Stdout) + udpListeners(string(udp.Stdout), port)))
	if len(pids) == 0 {
		p.Error("Nothing is listening on port " + port + ".")
		return 1
	}
	p.Info("Listening on port " + port + ":")
	var plan []string
	for _, pid := range pids {
		cmd, st := m.output(c, "ps", "-o", "comm=", "-p", pid)
		if st != 0 || cmd == "" {
			cmd = "?"
		}
		plan = append(plan, pid+"\t"+strings.ReplaceAll(cmd, "\t", " "))
	}
	m.planTable(c, plan)
	if !force && !ui.StyleOf(c.Term, false).Confirm(c.Term, c.Stderr, "Terminate these processes?") {
		p.Info("Aborted.")
		return 1
	}
	if !m.terminate(pids, func(pid string, outcome int) {
		switch outcome {
		case outKilled:
			p.Info("Process " + pid + " on port " + port + " ignored SIGTERM; killed with SIGKILL.")
		case outDenied:
			p.Error(deniedMsg(pid))
		default:
			p.Info("Process " + pid + " on port " + port + " terminated.")
		}
	}) {
		return 1
	}
	return 0
}

// udpListeners reads `lsof -nP -iUDP:PORT -F pn` and lists, one per line,
// the PIDs of unconnected UDP sockets whose local port is PORT (LEDGER
// SYS-03): -iUDP:PORT also matches a connected client whose remote port is
// PORT ("LOCAL->REMOTE"), which is not a listener.
func udpListeners(out, port string) string {
	var b strings.Builder
	pid := ""
	listed := map[string]bool{}
	for _, l := range strings.Split(out, "\n") {
		if l == "" {
			continue
		}
		switch l[0] {
		case 'p':
			pid = l[1:]
		case 'n':
			name := l[1:]
			if pid == "" || listed[pid] || strings.Contains(name, "->") || !strings.HasSuffix(name, ":"+port) {
				continue
			}
			listed[pid] = true
			b.WriteString(pid + "\n")
		}
	}
	return b.String()
}

// sortUN is `sort -un` of lines of PIDs: numeric order, one line per
// number (the first of equal ones), as words for the `for pid in $pids`
// loops.
func sortUN(out string) []string {
	type line struct {
		text string
		n    float64
	}
	var ls []line
	for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if l == "" && out == "" {
			continue
		}
		ls = append(ls, line{l, leadingNumber(l)})
	}
	sort.SliceStable(ls, func(i, j int) bool { return ls[i].n < ls[j].n })
	var words []string
	for i, l := range ls {
		if i > 0 && l.n == ls[i-1].n {
			continue
		}
		words = append(words, strings.Fields(l.text)...)
	}
	return words
}

// leadingNumber is the number sort -n reads at the start of a line
// (blanks skipped, 0 without digits).
func leadingNumber(s string) float64 {
	s = strings.TrimLeft(s, " \t")
	j := 0
	if j < len(s) && s[j] == '-' {
		j++
	}
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		j++
	}
	v, err := strconv.ParseFloat(s[:j], 64)
	if err != nil {
		return 0
	}
	return v
}
