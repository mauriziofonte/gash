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

// processKill is process_kill: `pgrep -ix NAME` with NAME as pgrep's
// regex (SYS-01), the plan and the question unless -y, then TERM, a
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
	res, _ := m.run.Run(ctx(c), runner.Cmd{Path: "pgrep", Args: []string{"-ix", name},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	pids := m.dropSelf(strings.Fields(runner.CommandSubst(res.Stdout)))
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
	m.terminate(pids, func(pid string, killed bool) {
		if killed {
			p.Info("Process " + pid + " ignored SIGTERM; killed with SIGKILL.")
		} else {
			p.Info("Process " + pid + " terminated.")
		}
	})
	return 0
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

// terminate sends TERM to every PID, polls them with signal 0 every 0.1 s
// for up to 3 s, then KILLs the survivors. A PID whose probe fails (dead,
// or EPERM: SYS-02) counts as terminated. report gets each PID in order.
func (m *Module) terminate(pids []string, report func(pid string, killed bool)) {
	for _, pid := range pids {
		_ = m.signal(pid, syscall.SIGTERM)
	}
	for waited := 0; waited < 30; waited++ {
		alive := false
		for _, pid := range pids {
			if m.signal(pid, 0) == nil {
				alive = true
			}
		}
		if !alive {
			break
		}
		m.sleep(100 * time.Millisecond)
	}
	for _, pid := range pids {
		if m.signal(pid, 0) == nil {
			_ = m.signal(pid, syscall.SIGKILL)
			report(pid, true)
		} else {
			report(pid, false)
		}
	}
}

// portKill is port_kill: the lsof TCP-listener and UDP queries (a UDP
// client of PORT included, SYS-03) merged as `sort -un`, the plan, the
// question unless -y, then the kill sequence of process_kill.
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
	udp, _ := m.run.Run(ctx(c), runner.Cmd{Path: "lsof", Args: []string{"-t", "-nP", "-iUDP:" + port},
		Options: runner.Options{Env: c.Env.Child(), Stderr: io.Discard}})
	pids := m.dropSelf(sortUN(string(tcp.Stdout) + string(udp.Stdout)))
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
	m.terminate(pids, func(pid string, killed bool) {
		if killed {
			p.Info("Process " + pid + " on port " + port + " ignored SIGTERM; killed with SIGKILL.")
		} else {
			p.Info("Process " + pid + " on port " + port + " terminated.")
		}
	})
	return 0
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
