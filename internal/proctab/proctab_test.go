// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package proctab

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

const psMain = `    1     0     0 Ss   /sbin/init splash
  100     1  1000 Ss   bash -c x; sleep 30
  101   100  1000 S    bash -c x; sleep 30
  200   101  1000 S    /usr/bin/gash llm_procs --name sleep
  201   200  1000 R    ps -A -o pid=,ppid=,uid=,stat=,args=
  300   100  1000 S    sleep 30
  301   300  1000 Z    [sleep] <defunct>
  302   300  1000 S    tail -f /var/log/app.log
  400     1     0 S    sleep 30 root
  401   400     0 S    nested child
  bad line
  500     2     0 I<   [kworker/0:1H]
`

const psComm = `    1 systemd
  100 bash
  101 bash
  200 gash
  201 ps
  300 sleep
  301 sleep
  302 tail
  400 sleep
  401 nested
  500 kworker/0:1H
`

func TestParsePS(t *testing.T) {
	tb := ParsePS([]byte(psMain), []byte(psComm))
	require.Len(t, tb.Procs(), 11)
	p, ok := tb.Lookup(302)
	require.True(t, ok)
	require.Equal(t, Proc{PID: 302, PPID: 300, UID: 1000, State: "S", Name: "tail", Cmd: "tail -f /var/log/app.log"}, p)
	z, _ := tb.Lookup(301)
	require.False(t, z.Alive())
	require.True(t, p.Alive())
	k, _ := tb.Lookup(500)
	require.Equal(t, "[kworker/0:1H]", k.Cmd)
	require.Equal(t, "I<", k.State)
	_, ok = tb.Lookup(999)
	require.False(t, ok)

	// A process missing from the comm listing gets its first word.
	tb = ParsePS([]byte("  7 1 0 S /opt/x/bin/server --port 1\n"), nil)
	p, _ = tb.Lookup(7)
	require.Equal(t, "server", p.Name)
}

func TestCallersAndMatch(t *testing.T) {
	tb := ParsePS([]byte(psMain), []byte(psComm))
	self := tb.Callers(200)
	// gash, its ps, the shell, the subshell with the shell's command line;
	// PID 1 is never part of the chain.
	require.Equal(t, map[int]bool{200: true, 201: true, 101: true, 100: true}, self)
	got := tb.Match(func(cmd string) bool { return strings.Contains(cmd, "sleep 30") }, self)
	var pids []int
	for _, p := range got {
		pids = append(pids, p.PID)
	}
	require.Equal(t, []int{300, 400}, pids)
	// A self that is not in the table is still left out.
	require.Equal(t, map[int]bool{4242: true}, tb.Callers(4242))
}

func TestTree(t *testing.T) {
	tb := ParsePS([]byte(psMain), []byte(psComm))
	skip := tb.Callers(200)
	seen := map[int]bool{}
	nodes := tb.Tree(100, skip, seen)
	var got []string
	for _, n := range nodes {
		got = append(got, strings.Repeat(" ", n.Depth)+n.Name)
	}
	require.Equal(t, []string{"bash", " sleep", "  sleep", "  tail"}, got)
	// What a first root listed is not listed again; an unknown root gives nothing.
	require.Empty(t, tb.Tree(300, skip, seen))
	require.Empty(t, tb.Tree(999, skip, seen))
	all := tb.Tree(1, skip, map[int]bool{})
	require.Equal(t, 1, all[0].PID)
	for _, n := range all {
		require.False(t, skip[n.PID], n.PID)
	}
}

func TestParseStat(t *testing.T) {
	p, ok := parseStat([]byte("42 (we (ird) name) S 7 42 42 0 -1\n"))
	require.True(t, ok)
	require.Equal(t, Proc{PID: 42, PPID: 7, State: "S", Name: "we (ird) name"}, p)
	for _, bad := range []string{"", "42 x S 7", "x (a) S 7", "42 (a) S", "42 (a) S x"} {
		_, ok := parseStat([]byte(bad))
		require.False(t, ok, bad)
	}
}

func TestReadPS(t *testing.T) {
	f := runner.NewFake(t,
		runner.Expect{Argv: append([]string{"/bin/ps"}, PSMain...), EnvHas: []string{"LC_ALL=C"}, Stdout: psMain},
		runner.Expect{Argv: append([]string{"/bin/ps"}, PSComm...), Stdout: psComm},
	)
	tb, err := ReadPS(context.Background(), f, "/bin/ps", []string{"LC_ALL=C"})
	require.NoError(t, err)
	require.Len(t, tb.Procs(), 11)

	f = runner.NewFake(t, runner.Expect{Argv: append([]string{"/bin/ps"}, PSMain...), Status: 1})
	_, err = ReadPS(context.Background(), f, "/bin/ps", nil)
	require.ErrorIs(t, err, ErrNoPS)
}

func TestReadProcFS(t *testing.T) {
	if !HasProcFS() {
		t.Skip("no /proc")
	}
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("no sleep")
	}
	cmd := exec.Command(sleep, "30")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})
	tb, err := ReadProcFS("/proc")
	require.NoError(t, err)
	p, ok := tb.Lookup(cmd.Process.Pid)
	require.True(t, ok)
	require.Equal(t, os.Getpid(), p.PPID)
	require.Equal(t, "sleep", p.Name)
	require.Equal(t, sleep+" 30", p.Cmd)
	require.Equal(t, os.Geteuid(), p.UID)
	require.True(t, p.Alive())
	self := tb.Callers(os.Getpid())
	require.True(t, self[os.Getpid()])
	require.True(t, self[cmd.Process.Pid], "a child of self is part of the caller chain")

	// A fake /proc: a kernel thread and a vanished entry.
	root := t.TempDir()
	write := func(p, s string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, p), []byte(s), 0o644))
	}
	write("2/stat", "2 (kthreadd) S 0 0 0")
	write("2/cmdline", "")
	write("9/stat", "8 (liar) S 1 0 0")
	write("self/stat", "x")
	tb, err = ReadProcFS(root)
	require.NoError(t, err)
	require.Len(t, tb.Procs(), 1)
	k, _ := tb.Lookup(2)
	require.Equal(t, "[kthreadd]", k.Cmd)
	_, err = ReadProcFS(filepath.Join(root, "none"))
	require.Error(t, err)
}

// TestReadUsage: %CPU is the CPU time over the time since the process
// started, %MEM the resident size over MemTotal, in tenths cut as ps
// prints them; a process without its times keeps zeros.
func TestReadUsage(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, p), []byte(s), 0o644))
	}
	rss := strconv.Itoa(51200 * 1024 / os.Getpagesize()) // 50 MiB
	// 99.5 s of CPU (4975 ticks each of user and system) over the 995 s
	// since it started at tick 500.
	write("1/stat", "1 (in it) S 0 1 1 0 -1 0 0 0 0 0 4975 4975 0 0 20 0 1 0 500 1000 "+rss+"\n")
	write("1/cmdline", "/sbin/init\x00")
	write("7/stat", "7 (short) S 1 7 7\n")
	write("uptime", "1000.00 1.00\n")
	write("meminfo", "MemTotal:        1024000 kB\n")
	us, err := ReadUsage(root)
	require.NoError(t, err)
	require.Len(t, us, 2)
	require.Equal(t, 100, us[0].CPU)
	require.Equal(t, 50, us[0].Mem)
	require.Equal(t, "/sbin/init", us[0].Cmd)
	require.Equal(t, "in it", us[0].Name)
	require.Equal(t, Usage{Proc: Proc{PID: 7, PPID: 1, UID: us[1].UID, State: "S", Name: "short", Cmd: "[short]"}}, us[1])
	_, err = ReadUsage(filepath.Join(root, "none"))
	require.Error(t, err)
}
