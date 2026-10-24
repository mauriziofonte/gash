// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

const (
	meminfo   = "MemTotal:       16000000 kB\nMemFree:         2048000 kB\nMemAvailable:    9000000 kB\nSReclaimable:     512500 kB\n"
	buddyinfo = "Node 0, zone      DMA      1      1      0      1      2      1      1      0      1      1      3\n" +
		"Node 0, zone   Normal   1000    500    200    100     50     20     10      5      2      1      0\n"
	dmesgPlain   = "[  1.0] x: page allocation failure: order:7, mode\n[  2.0] y: page allocation failure: order:7, mode\n"
	dmesgStamped = "[Mon Oct  6 10:00:00 2026] x: page allocation failure: order:7, mode\n" +
		"[Mon Oct  6 11:00:00 2026] y: page allocation failure: order:7, mode\n[Mon Oct  6 12:00:00 2026] other\n"
)

// wslRoot gives m a fixture sysroot: a WSL kernel (or not), the memory
// files and, when cmd is true, an executable Windows cmd.exe.
func wslRoot(t *testing.T, m *Module, wsl, cmd bool) string {
	t.Helper()
	d := t.TempDir()
	version := "Linux version 6.8.0-45-generic (buildd@lcy02) #45-Ubuntu\n"
	if wsl {
		version = "Linux version 5.15.153.1-microsoft-standard-WSL2 (root@x) #1 SMP\n"
	}
	for name, body := range map[string]string{"proc/version": version, "proc/meminfo": meminfo, "proc/buddyinfo": buddyinfo} {
		require.NoError(t, os.MkdirAll(filepath.Join(d, filepath.Dir(name)), 0o700))
		require.NoError(t, os.WriteFile(filepath.Join(d, name), []byte(body), 0o600))
	}
	if cmd {
		p := filepath.Join(d, "mnt/c/Windows/System32/cmd.exe")
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o700))
		require.NoError(t, os.WriteFile(p, []byte("#!/bin/sh\n"), 0o700))
	}
	sr, err := platform.NewSysroot(d)
	require.NoError(t, err)
	m.sys = sr
	return d
}

func TestRequireWSL(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t))
	wslRoot(t, m, false, false)
	for _, name := range []string{"wsl_restart", "wsl_shutdown", "wsl_explorer", "wsl_taskmanager", "wsl_compact"} {
		r := call(m, name, nil, nil, "")
		require.Equal(t, result{"", "Error: This command requires WSL (Windows Subsystem for Linux).\n", 1}, r, name)
	}
}

func TestWSLStopVM(t *testing.T) {
	m, _, h := tm(t, runner.NewFake(t))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_restart", nil, nil, "", "-y", "now")
	require.Equal(t, result{"", "Error: Unknown option: now (usage: wsl_restart [-y])\n", 1}, r)
	r = call(m, "wsl_shutdown", nil, nil, "", "--force")
	require.Equal(t, result{"", "Error: Unknown option: --force (usage: wsl_shutdown [-y])\n", 1}, r)

	// Headless without -y: the refused question.
	r = call(m, "wsl_restart", map[string]string{"GASH_HEADLESS": "1", "GASH__HANDOFF": "3"}, nil, "")
	require.Equal(t, result{"", "Info: Aborted.\n", 1}, r)
	require.Empty(t, h.String())

	// The stub's first call: Go, nothing run.
	r = call(m, "wsl_restart", map[string]string{"GASH__HANDOFF": "3"}, ttyTerm(nil, "y\n"), "")
	require.Equal(t, 0, r.rc)
	require.Equal(t, "Shut down WSL: every distro, Docker Desktop and running servers stop? \x1b[1m[y/N]\x1b[0m ", r.err)
	require.Equal(t, "1", h.String())

	// The resumed call (history saved) shuts the VM down from Windows.
	dir := m.winDir()
	m, _, h = tm(t, runner.NewFake(t, runner.Expect{Argv: []string{"cmd.exe", "/C", "wsl", "--shutdown"}, Dir: &dir, Status: 2}))
	wslRoot(t, m, true, false)
	r = call(m, "wsl_shutdown", map[string]string{"GASH__RESUME": "1"}, nil, "", "-y")
	require.Equal(t, result{"", "Info: History saved. Shutting down WSL (it starts again with the next terminal)...\n", 2}, r)
	require.Empty(t, h.String())

	// A direct run with -y does it all at once.
	m, _, _ = tm(t, runner.NewFake(t, runner.Expect{Argv: []string{"cmd.exe", "/C", "wsl", "--shutdown"},
		Err: &runner.StartError{Name: "cmd.exe", Status: 127, Err: errors.New("not found")}}))
	wslRoot(t, m, true, false)
	r = call(m, "wsl_shutdown", nil, nil, "", "--yes")
	require.Equal(t, result{"", "Info: History saved. Shutting down WSL (it starts again with the next terminal)...\nbash: cmd.exe: command not found\n", 127}, r)
}

func TestWSLExplorer(t *testing.T) {
	d := t.TempDir()
	m, _, _ := tm(t, runner.NewFake(t,
		runner.Expect{Argv: []string{"wslpath", "-w", d}, Stdout: "\\\\wsl.localhost\\Ubuntu\\tmp\\x\n"},
		runner.Expect{Argv: []string{"explorer.exe", "\\\\wsl.localhost\\Ubuntu\\tmp\\x"}, Status: 1},
		runner.Expect{Argv: []string{"wslpath", "-w", "."}, Status: 1},
		runner.Expect{Argv: []string{"explorer.exe", "."}},
	))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_explorer", nil, nil, "", d)
	require.Equal(t, result{"", "", 0}, r)
	r = call(m, "wsl_explorer", nil, nil, "")
	require.Equal(t, result{"", "", 0}, r)
	r = call(m, "wsl_explorer", nil, nil, "", filepath.Join(d, "nope"))
	require.Equal(t, result{"", "Error: No such file or directory: " + filepath.Join(d, "nope") + "\n", 1}, r)
	// 3.0: SYS-10 fixed: a second positional is refused, nothing runs.
	r = call(m, "wsl_explorer", nil, nil, "", d, "x")
	require.Equal(t, result{"", "Error: Unexpected argument: x\n", 1}, r)
}

func TestWSLTaskmanager(t *testing.T) {
	m, _, _ := tm(t, nil)
	dir := m.winDir()
	m, _, _ = tm(t, runner.NewFake(t,
		runner.Expect{Background: true, Argv: []string{"cmd.exe", "/C", "taskmgr"}, Dir: &dir},
		runner.Expect{Background: true, Argv: []string{"cmd.exe", "/C", "taskmgr"}, Err: &runner.StartError{Name: "cmd.exe", Status: 127, Err: errors.New("not found")}},
	))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_taskmanager", nil, nil, "")
	require.Equal(t, result{"", "", 0}, r)
	r = call(m, "wsl_taskmanager", nil, nil, "")
	require.Equal(t, result{"", "bash: cmd.exe: command not found\n", 0}, r)
}

func dmesg(plain, stamped string, status int) []runner.Expect {
	return []runner.Expect{
		{Argv: []string{"dmesg"}, Stdout: plain, Status: status},
		{Argv: []string{"dmesg", "-T"}, Stdout: stamped, Status: status},
	}
}

func compact(status int) runner.Expect {
	return runner.Expect{Script: runner.ScriptWSLCompact,
		Argv:   []string{"sudo", "sh", "-c", "sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory"},
		Stderr: "sudo sh -c sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory\n", Status: status}
}

const compactTable = "                       before                     after\n" +
	"MemFree                2000 MiB                   2000 MiB\n" +
	"SReclaimable (slab)    500 MiB                    500 MiB\n" +
	"free >=512KiB blocks   DMA=30 Normal=13           DMA=30 Normal=13\n"

// The expectations are the output of 2.x wsl_compact on the same files.
func TestWSLCompact(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, dmesg(dmesgPlain, dmesgStamped, 0)...))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_compact", nil, nil, "", "--status")
	require.Equal(t, result{"order-7 allocation failures since the VM started: 2 (last: Mon Oct  6 11:00:00 2026)\n" +
		"MemFree                2000 MiB\nSReclaimable (slab)    500 MiB\nfree >=512KiB blocks   DMA=30 Normal=13\n", "", 0}, r)

	m, _, _ = tm(t, runner.NewFake(t, append(dmesg(dmesgPlain, dmesgStamped, 0), compact(0))...))
	wslRoot(t, m, true, false)
	r = call(m, "wsl_compact", nil, nil, "")
	require.Equal(t, "order-7 allocation failures since the VM started: 2 (last: Mon Oct  6 11:00:00 2026)\n"+compactTable, r.out)
	require.Equal(t, "Info: Dropping the page cache and slab, then compacting memory (sudo)...\n"+
		"sudo sh -c sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory\n", r.err)
	require.Equal(t, 0, r.rc)

	m, _, _ = tm(t, runner.NewFake(t, append(dmesg("", "", 1), compact(1))...))
	wslRoot(t, m, true, false)
	r = call(m, "wsl_compact", nil, nil, "")
	require.Equal(t, result{"order-7 allocation failures since the VM started: 0\n",
		"Info: Dropping the page cache and slab, then compacting memory (sudo)...\n" +
			"sudo sh -c sync && echo 3 > /proc/sys/vm/drop_caches && echo 1 > /proc/sys/vm/compact_memory\n" +
			"Error: drop_caches/compact_memory failed.\n", 1}, r)

	r = call(m, "wsl_compact", nil, nil, "", "--x")
	require.Equal(t, result{"", "Error: Unknown option: --x (usage: wsl_compact [--status])\n", 1}, r)
}

// The interop check runs Windows' cmd.exe with a 20 s limit when it is
// there.
func TestWSLCompactInterop(t *testing.T) {
	f := runner.NewFake(t, append(dmesg("", "", 0), compact(0))...)
	m, _, _ := tm(t, f)
	root := wslRoot(t, m, true, true)
	cmd := filepath.Join(root, "mnt/c/Windows/System32/cmd.exe")
	f.Expect(runner.Expect{Argv: []string{cmd, "/c", "exit", "0"}})
	r := call(m, "wsl_compact", nil, nil, "")
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.err, "OK: Windows interop works.\n")
	require.Equal(t, "20s", f.Calls()[3].Timeout.String())

	f = runner.NewFake(t, append(dmesg("", "", 0), compact(0), runner.Expect{Argv: []string{cmd, "/c", "exit", "0"}, TimedOut: true, Signal: 15})...)
	m, _, _ = tm(t, f)
	sr, err := platform.NewSysroot(root)
	require.NoError(t, err)
	m.sys = sr
	r = call(m, "wsl_compact", nil, nil, "")
	require.Equal(t, 1, r.rc)
	require.Contains(t, r.err, "Error: Windows interop still fails. Last resort, from Windows: wsl --shutdown\n")
}

// SYS-45: no question and no headless gate; a failing dmesg counts 0.
func TestLedger_SYS45(t *testing.T) {
	m, _, _ := tm(t, runner.NewFake(t, append(dmesg("", "", 1), compact(0))...))
	wslRoot(t, m, true, false)
	r := call(m, "wsl_compact", map[string]string{"GASH_HEADLESS": "1"}, nil, "")
	require.Equal(t, 0, r.rc)
	require.Equal(t, "order-7 allocation failures since the VM started: 0\n"+compactTable, r.out)
}

// SYS-46: the one sudo call is the registered constant script.
func TestLedger_SYS46(t *testing.T) {
	f := runner.NewFake(t, append(dmesg("", "", 0), compact(0))...)
	m, _, _ := tm(t, f)
	wslRoot(t, m, true, false)
	r := call(m, "wsl_compact", nil, nil, "")
	require.Equal(t, 0, r.rc)
	require.Equal(t, runner.ScriptWSLCompact, f.Calls()[2].Script)
}

func TestMemSnapshot(t *testing.T) {
	m, _, _ := tm(t, nil)
	wslRoot(t, m, true, false)
	require.Equal(t, "2000 MiB|500 MiB|DMA=30 Normal=13", memSnapshot(m.sys))
	// Missing files: empty fields, as awk on a missing file.
	sr, err := platform.NewSysroot(t.TempDir())
	require.NoError(t, err)
	require.Equal(t, "||", memSnapshot(sr))
}
