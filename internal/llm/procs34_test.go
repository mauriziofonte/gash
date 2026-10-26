// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/proctab"
	"github.com/mauriziofonte/gash/internal/runner"
)

// psTable is a ps listing: the gash process 200 runs from the shell 101,
// a subshell of the shell 100; 300 sleeps with a child and a zombie.
const psTableMain = `    1     0     0 Ss   /sbin/init
  100     1  1000 Ss   bash -c x; sleep 30
  101   100  1000 S    bash -c x; sleep 30
  200   101  1000 S    /usr/bin/gash llm_procs --name sleep
  201   200  1000 R    ps -A -o pid=,ppid=,uid=,stat=,args=
  300   100  1000 S    sleep 30
  301   300  1000 Z    [sleep] <defunct>
  302   300  1000 S    tail -f /var/log/app.log
  400     1     0 S    sleep 30 --long ` + "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx" + `
`

const psTableComm = `    1 init
  100 bash
  101 bash
  200 gash
  201 ps
  300 sleep
  301 sleep
  302 tail
  400 sleep
`

// procsModule is the module with a fake ps (and lsof) and the gash
// process as PID 200.
func procsModule(t *testing.T) *Module {
	m := New()
	m.getpid = func() int { return 200 }
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, true }
	m.run = runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		switch strings.Join(c.Argv, " ") {
		case "/bin/ps " + strings.Join(proctab.PSMain, " "):
			return runner.Expect{Stdout: psTableMain}, true
		case "/bin/ps " + strings.Join(proctab.PSComm, " "):
			return runner.Expect{Stdout: psTableComm}, true
		case "/bin/lsof -i :8080 -t":
			return runner.Expect{Stdout: "302\n300\n302\n"}, true
		}
		return runner.Expect{}, false
	})
	return m
}

func TestProcs34(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	long := "sleep 30 --long " + strings.Repeat("x", 200-len("sleep 30 --long ")-1) + "…"
	for _, tc := range []struct {
		args []string
		want string
	}{
		// Explicit PIDs: the given order, a gone one as alive:false, a
		// zombie is not alive, the caller's own chain is reported when asked.
		{[]string{"--pid", "302", "999", "301", "200"}, `[{"pid":302,"name":"tail","cmd":"tail -f /var/log/app.log","alive":true},` +
			`{"pid":999,"name":"","cmd":"","alive":false},{"pid":301,"name":"sleep","cmd":"[sleep] <defunct>","alive":false},` +
			`{"pid":200,"name":"gash","cmd":"/usr/bin/gash llm_procs --name sleep","alive":true}]`},
		{[]string{"--pid", "302", "--pid", "302", "999", "--alive"}, `[{"pid":302,"name":"tail","cmd":"tail -f /var/log/app.log","alive":true}]`},
		// --name: a regular expression on the full command line, by PID,
		// never the caller's shell (100, 101 both run "... sleep 30").
		{[]string{"--name", "sleep 3[0]", "--alive"}, `[{"pid":300,"name":"sleep","cmd":"sleep 30","alive":true},` +
			`{"pid":400,"name":"sleep","cmd":"` + long + `","alive":true}]`},
		{[]string{"--name", "sleep 30", "--alive", "--width", "12"}, `[{"pid":300,"name":"sleep","cmd":"sleep 30","alive":true},` +
			`{"pid":400,"name":"sleep","cmd":"sleep 30 --…","alive":true}]`},
		{[]string{"--name", "nothing-runs-this", "--alive"}, `[]`},
		// --tree: depth first, children by PID, with ppid and depth.
		{[]string{"--pid", "300", "--tree"}, `[{"pid":300,"name":"sleep","cmd":"sleep 30","alive":true,"ppid":100,"depth":0},` +
			`{"pid":301,"name":"sleep","cmd":"[sleep] <defunct>","alive":false,"ppid":300,"depth":1},` +
			`{"pid":302,"name":"tail","cmd":"tail -f /var/log/app.log","alive":true,"ppid":300,"depth":1}]`},
		{[]string{"--pid", "100", "--tree", "--alive", "--format", "tsv"}, "pid\tname\tcmd\talive\tppid\tdepth\n" +
			"100\tbash\tbash -c x; sleep 30\ttrue\t1\t0\n300\tsleep\tsleep 30\ttrue\t100\t1\n302\ttail\ttail -f /var/log/app.log\ttrue\t300\t2"},
		{[]string{"--pid", "999", "--tree"}, `[{"pid":999,"name":"","cmd":"","alive":false,"ppid":null,"depth":0}]`},
		// --port: the processes lsof names, by PID, once each; --pid wins.
		{[]string{"--port", "8080", "--alive"}, `[{"pid":300,"name":"sleep","cmd":"sleep 30","alive":true},{"pid":302,"name":"tail","cmd":"tail -f /var/log/app.log","alive":true}]`},
		{[]string{"--port", "8080", "--pid", "1", "--alive"}, `[{"pid":1,"name":"init","cmd":"/sbin/init","alive":true}]`},
	} {
		r := runGo(t, procsModule(t), d, env, "llm_procs", tc.args...)
		require.Equal(t, result{tc.want + "\n", "", 0}, r, tc.args)
	}

	// No target: every process but the caller's chain, by PID, first 100.
	r := runGo(t, procsModule(t), d, env, "llm_procs", "--alive", "--format", "tsv")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "pid\tname\tcmd\talive\n1\tinit\t/sbin/init\ttrue\n300\tsleep\tsleep 30\ttrue\n302\ttail\ttail -f /var/log/app.log\ttrue\n400\tsleep\t"+long+"\ttrue\n", r.Stdout)
	r = runGo(t, procsModule(t), d, env, "llm_procs", "--alive", "--head", "1")
	require.Equal(t, result{`[{"pid":1,"name":"init","cmd":"/sbin/init","alive":true}]` + "\n", `{"truncated":3}` + "\n", 0}, r)
}

func TestProcs34Errors(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	for _, tc := range []struct {
		args []string
		code string
	}{
		{[]string{"--pid"}, "missing_value"},
		{[]string{"--pid", "--alive"}, "missing_value"},
		{[]string{"--pid", "12x"}, "invalid_pid"},
		{[]string{"--pid", "1234567890"}, "invalid_pid"},
		{[]string{"--pid", ""}, "invalid_pid"},
		{[]string{"--name", "(", "--alive"}, "invalid_regex"},
		{[]string{"--tree", "x"}, "unknown_option"},
		{[]string{"--alive", "--format", "xml"}, "invalid_format"},
		{[]string{"--alive", "--head", "0"}, "invalid_head"},
		{[]string{"--alive", "--port", "70000"}, "invalid_port"},
	} {
		r := runGo(t, New(), d, env, "llm_procs", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Empty(t, r.Stdout, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
	// Without ps and without /proc: ps_not_found (CONTINUE); with /proc the
	// table comes from there.
	m := New()
	m.lookPathFn = func(string) (string, bool) { return "", false }
	r := runGo(t, m, d, env, "llm_procs", "--pid", strconv.Itoa(os.Getpid()))
	if proctab.HasProcFS() {
		require.Equal(t, 0, r.RC, r.Stderr)
		require.Contains(t, r.Stdout, `"alive":true`)
	} else {
		require.Contains(t, r.Stderr, `"error":"ps_not_found","details":"llm_procs --pid needs ps, which is not installed","action":"CONTINUE"`)
		require.Contains(t, r.Stderr, `"missing":"ps"`)
	}
}

// TestProcsLegacyFormats: the 2.x modes print what they printed before
// without the 3.4 options, and take --format and the budget.
func TestProcsLegacyFormats(t *testing.T) {
	d := physTemp(t)
	bin := stubBin(t, map[string]string{
		"ps":    psStub,
		"pgrep": `[[ "$1" == -f ]] || exit 3; case "$2" in sleep*) printf '101\n102\n103\nxyz\n' ;; *) exit 1 ;; esac`,
		"lsof":  `[[ "$1 $3" == "-i -t" ]] || exit 3; case "$2" in :80|:0080) printf '4242\n 4243 \nbad\n' ;; *) exit 1 ;; esac`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t, "seq")})
	r := runGo(t, New(), d, env, "llm_procs", "--port", "80")
	require.Equal(t, result{`[{"pid":4242,"name":"nginx","port":80},{"pid":4243,"name":"","port":80}]` + "\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_procs", "--port", "80", "--format", "tsv")
	require.Equal(t, result{"pid\tname\tport\n4242\tnginx\t80\n4243\t\t80\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_procs", "--format", "jsonl", "--head", "2")
	require.Equal(t, 0, r.RC)
	require.Equal(t, `{"pid":1,"user":"user1","cpu":1.5,"mem":0.1,"name":"/usr/bin/cmd1"}`+"\n"+
		`{"pid":2,"user":"user2","cpu":1.5,"mem":0.2,"name":"/usr/bin/cmd2"}`+"\n", r.Stdout)
	require.Equal(t, `{"truncated":18}`+"\n", r.Stderr)
	r = runGo(t, New(), d, env, "llm_procs", "--name", "nothing", "--format", "table")
	require.Equal(t, result{"pid  name\n", "", 0}, r)
}

// TestProcs34RealChain: through the real ps, a NAME only the caller's
// shell matches gives [], a child of that shell is listed, and --pid of a
// process that ended is alive:false.
func TestProcs34RealChain(t *testing.T) {
	for _, tool := range []string{"ps", "sleep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	marker := "gash-procs34-" + strconv.Itoa(os.Getpid())
	sleep := exec.Command("sleep", "30")
	require.NoError(t, sleep.Start())
	pid := sleep.Process.Pid
	t.Cleanup(func() {
		_ = sleep.Process.Kill()
		_ = sleep.Wait()
	})
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	// This test process is the caller's chain: its own command line never
	// matches.
	r := runGo(t, New(), d, env, "llm_procs", "--name", marker+"|"+"llm34-none", "--alive")
	require.Equal(t, result{"[]\n", "", 0}, r)
	r = runGo(t, New(), d, env, "llm_procs", "--pid", strconv.Itoa(pid))
	require.Equal(t, `[{"pid":`+strconv.Itoa(pid)+`,"name":"sleep","cmd":"sleep 30","alive":true}]`+"\n", r.Stdout)
	require.NoError(t, sleep.Process.Kill())
	_ = sleep.Wait()
	r = runGo(t, New(), d, env, "llm_procs", "--pid", strconv.Itoa(pid))
	require.Equal(t, `[{"pid":`+strconv.Itoa(pid)+`,"name":"","cmd":"","alive":false}]`+"\n", r.Stdout)
}
