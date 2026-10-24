// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/system/handoff"
	"github.com/mauriziofonte/gash/internal/ui"
)

// requireWSL is __gash_require_wsl: /proc/version names Microsoft.
func (m *Module) requireWSL(c *invoke.Call) bool {
	b, err := m.sys.ReadFile("/proc/version")
	if err == nil && strings.Contains(strings.ToLower(string(b)), "microsoft") {
		return true
	}
	ui.NewPrinter(c.Stderr, c.Term, false).Error("This command requires WSL (Windows Subsystem for Linux).")
	return false
}

// winDir is the working directory of __gash_wsl_cmd: `cd /mnt/c || cd /`.
func (m *Module) winDir() string {
	if st, err := os.Stat("/mnt/c"); err == nil && st.IsDir() && faccess("/mnt/c", 1) {
		return "/mnt/c"
	}
	return "/"
}

func (m *Module) wslRestart(c *invoke.Call) int  { return m.wslStopVM(c) }
func (m *Module) wslShutdown(c *invoke.Call) int { return m.wslStopVM(c) }

// wslStopVM is __gash_wsl_stop_vm NAME [-y]: the options, the WSL check
// and the question; the stub override then saves the history (`history
// -a`, the hand-off) and the resumed call shuts the VM down with
// `cmd.exe /C wsl --shutdown` from /mnt/c.
func (m *Module) wslStopVM(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	assumeYes := false
	for _, a := range c.Args {
		if a != "-y" && a != "--yes" {
			p.Error("Unknown option: " + a + " (usage: " + c.Name + " [-y])")
			return 1
		}
		assumeYes = true
	}
	if !m.requireWSL(c) {
		return 1
	}
	if !handoff.Resumed(c.Env) {
		ch := handoff.Open(c.Env, m.open)
		if !assumeYes && !ui.StyleOf(c.Term, false).Confirm(c.Term, c.Stderr,
			"Shut down WSL: every distro, Docker Desktop and running servers stop?") {
			ch.Close()
			p.Info("Aborted.")
			return 1
		}
		if ch.Active() {
			ch.Send(handoff.Go)
			return 0
		}
	}
	p.Info("History saved. Shutting down WSL (it starts again with the next terminal)...")
	return m.wslCmd(c, "wsl", "--shutdown")
}

// wslCmd is __gash_wsl_cmd: cmd.exe /C ARGS from a Windows directory, on
// the caller's terminal.
func (m *Module) wslCmd(c *invoke.Call, args ...string) int {
	res, err := m.run.Run(ctx(c), runner.Cmd{Path: "cmd.exe", Args: append([]string{"/C"}, args...),
		Options: runner.Options{Env: c.Env.Child(), Dir: m.winDir(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
	if se, ok := err.(*runner.StartError); ok {
		write(c.Stderr, "bash: "+se.Error()+"\n")
		return se.Status
	}
	return res.ExitCode
}

// wslExplorer is wsl_explorer [PATH]: Explorer on the Windows form of
// PATH (wslpath -w, else PATH as given); status 0 whatever explorer.exe
// returns.
func (m *Module) wslExplorer(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if !m.requireWSL(c) {
		return 1
	}
	if len(c.Args) > 1 {
		ui.NewPrinter(c.Stderr, c.Term, false).Error(unexpectedArg(c.Args[1]))
		return 1
	}
	target := c.Arg(0)
	if target == "" {
		target = "."
	}
	if _, err := os.Stat(target); err != nil {
		ui.NewPrinter(c.Stderr, c.Term, false).Error("No such file or directory: " + target)
		return 1
	}
	win, st := m.output(c, "wslpath", "-w", target)
	if st != 0 {
		win = target
	}
	_ = m.passthrough(c, "explorer.exe", win)
	return 0
}

// wslTaskmanager is wsl_taskmanager: `(__gash_wsl_cmd taskmgr &)`, a
// background start that is not waited for.
func (m *Module) wslTaskmanager(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if !m.requireWSL(c) {
		return 1
	}
	cmd := runner.Cmd{Path: "cmd.exe", Args: []string{"/C", "taskmgr"},
		Options: runner.Options{Env: c.Env.Child(), Dir: m.winDir(), Stdout: osFile(c.Stdout), Stderr: osFile(c.Stderr)}}
	if err := m.run.Start(ctx(c), cmd); err != nil {
		if se, ok := err.(*runner.StartError); ok {
			write(c.Stderr, "bash: "+se.Error()+"\n")
		}
	}
	return 0
}

// osFile passes a terminal stream to a background child; anything else
// (a test buffer) becomes /dev/null.
func osFile(w io.Writer) io.Writer {
	if f, ok := w.(*os.File); ok {
		return f
	}
	return nil
}

// memSnapshot is __gash_wsl_mem_snapshot: "MemFree|SReclaimable|blocks",
// the first two in MiB from /proc/meminfo, blocks the free blocks of
// order 7 and up per zone of /proc/buddyinfo.
func memSnapshot(sys platform.Sysroot) string {
	mib := func(meminfo, key string) string {
		for _, l := range strings.Split(meminfo, "\n") {
			if strings.HasPrefix(l, key+":") {
				return strconv.FormatInt(int64(awkNumber(field(awkFields(l), 1))/1024), 10) + " MiB"
			}
		}
		return ""
	}
	mi, _ := sys.ReadFile("/proc/meminfo")
	var blocks []string
	if bi, err := sys.ReadFile("/proc/buddyinfo"); err == nil {
		for _, l := range strings.Split(strings.TrimSuffix(string(bi), "\n"), "\n") {
			f := awkFields(l)
			n := 0.0
			for i := 11; i < len(f); i++ {
				n += awkNumber(f[i]) * float64(int64(1)<<(i-11))
			}
			blocks = append(blocks, field(f, 3)+"="+strconv.FormatInt(int64(n), 10))
		}
	}
	return mib(string(mi), "MemFree") + "|" + mib(string(mi), "SReclaimable") + "|" + strings.Join(blocks, " ")
}

// dmesgStamp is the sed -E 's/^\[([^]]+)\].*/\1/' of the last failure.
var dmesgStamp = regexp.MustCompile(`^\[([^]]+)\].*`)

const order7 = "page allocation failure: order:7"

// wslCompact is wsl_compact [--status] [-y]: the order-7 failures of
// dmesg, the memory snapshot, then the constant sudo script of SYS-46,
// the before/after table and the interop check. On a terminal it runs
// unprompted; headless it needs -y (SYS-45).
func (m *Module) wslCompact(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.NewPrinter(c.Stderr, c.Term, false)
	statusOnly, yes := false, false
	for _, a := range c.Args {
		switch a {
		case "--status":
			statusOnly = true
		case "-y", "--yes":
			yes = true
		default:
			p.Error("Unknown option: " + a + " (usage: wsl_compact [--status] [-y])")
			return 1
		}
	}
	if !m.requireWSL(c) {
		return 1
	}
	if !statusOnly && !yes && c.Term.Headless() {
		p.Error("wsl_compact needs root: pass -y to run it unattended.")
		return 1
	}
	s := ui.StyleOf(c.Term, false)
	plain, dmesgRC := m.output(c, "dmesg")
	fails := 0
	for _, l := range strings.Split(plain, "\n") {
		if strings.Contains(l, order7) {
			fails++
		}
	}
	stamped, _ := m.output(c, "dmesg", "-T")
	last := ""
	for _, l := range strings.Split(stamped, "\n") {
		if strings.Contains(l, order7) {
			last = l
		}
	}
	last = dmesgStamp.ReplaceAllString(last, "$1")
	value := strconv.Itoa(fails)
	if last != "" {
		value += " (last: " + last + ")"
	}
	if dmesgRC != 0 {
		// kernel.dmesg_restrict: no count at all, not 0 (SYS-45).
		value = "unknown (dmesg not readable)"
	}
	s.KV(c.Stdout, "--width", "0", "order-7 allocation failures since the VM started:", value)

	before := strings.SplitN(memSnapshot(m.sys), "|", 3)
	if statusOnly {
		s.KV(c.Stdout, "--width", "22", "MemFree", before[0], "SReclaimable (slab)", before[1], "free >=512KiB blocks", before[2])
		return 0
	}
	if !m.sudoReady(c, p) {
		return 1
	}
	p.Info("Dropping the page cache and slab, then compacting memory (sudo)...")
	res, err := m.run.RunScript(ctx(c), runner.Script{Name: runner.ScriptWSLCompact,
		Options: runner.Options{Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
	if se, ok := err.(*runner.StartError); ok {
		write(c.Stderr, "bash: "+se.Error()+"\n")
	}
	if err != nil || res.ExitCode != 0 {
		p.Error("drop_caches/compact_memory failed.")
		return 1
	}
	after := strings.SplitN(memSnapshot(m.sys), "|", 3)
	var args []string
	if !c.Term.UseColor() {
		args = append(args, "--no-header")
	}
	args = append(args, "--gap", "1", "--min", "1:22", "--min", "2:26")
	s.Table(c.Stdout, []string{
		"\tbefore\tafter",
		"MemFree\t" + before[0] + "\t" + after[0],
		"SReclaimable (slab)\t" + before[1] + "\t" + after[1],
		"free >=512KiB blocks\t" + before[2] + "\t" + after[2],
	}, args...)

	const winCmd = "/mnt/c/Windows/System32/cmd.exe"
	if !faccess(m.sys.Path(winCmd), 1) {
		return 0
	}
	res, err = m.run.Run(ctx(c), runner.Cmd{Path: m.sys.Path(winCmd), Args: []string{"/c", "exit", "0"},
		Options: runner.Options{Env: c.Env.Child(), Stdout: io.Discard, Stderr: io.Discard, Timeout: 20 * time.Second}})
	if err == nil && res.TimeoutStatus() == 0 {
		p.Success("Windows interop works.")
		return 0
	}
	p.Error("Windows interop still fails. Last resort, from Windows: wsl --shutdown")
	return 1
}
