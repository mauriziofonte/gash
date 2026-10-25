// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

const showOut = `Id=nginx.service
LoadState=loaded
ActiveState=active
SubState=running
StateChangeTimestamp=Tue 2026-10-20 10:00:00 UTC
MainPID=812
NRestarts=2

Id=nosuch.service
LoadState=not-found
ActiveState=inactive
SubState=dead
StateChangeTimestamp=n/a
MainPID=0
NRestarts=0
`

// serviceModule fakes systemctl and journalctl.
func serviceModule(t *testing.T, show runner.Expect, journal map[string]string) (*Module, *runner.Fake) {
	m := New()
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "systemctl" || name == "journalctl" }
	f := runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		if c.Argv[0] == "/bin/systemctl" {
			return show, true
		}
		if c.Argv[0] == "/bin/journalctl" {
			return runner.Expect{Stdout: journal[c.Argv[2]]}, true
		}
		return runner.Expect{}, false
	})
	m.run = f
	return m, f
}

func TestService(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	long := strings.Repeat("z", 400)
	m, f := serviceModule(t, runner.Expect{Stdout: showOut}, map[string]string{
		"nginx": "2026-10-25T10:00:00+0000 nginx[812]: started\n2026-10-25T10:00:01+0000 nginx[812]: " + long + "\n",
	})
	r := runGo(t, m, d, env, "llm_service", "nginx", "nosuch", "--log", "5")
	require.Equal(t, 0, r.RC, r.Stderr)
	want := `[{"unit":"nginx","load":"loaded","active":"active","sub":"running","since":"2026-10-20 10:00:00 UTC","pid":812,"restarts":2,` +
		`"log":["2026-10-25T10:00:00+0000 nginx[812]: started","2026-10-25T10:00:01+0000 nginx[812]: ` + long[:300-len("2026-10-25T10:00:01+0000 nginx[812]: ")-1] + `…"]},` +
		`{"unit":"nosuch","load":"not-found","active":"inactive","sub":"dead","since":null,"pid":0,"restarts":0,"log":[]}]` + "\n"
	require.Equal(t, want, r.Stdout)
	calls := f.Calls()
	require.Equal(t, []string{"/bin/systemctl", "show", "--no-pager", "--property=" + serviceProps, "--", "nginx", "nosuch"}, calls[0].Argv)
	require.Len(t, calls, 2, "no journal for a unit that is not found")
	require.Equal(t, []string{"/bin/journalctl", "-u", "nginx", "-n", "5", "--no-pager", "-q", "-o", "short-iso", "--no-hostname"}, calls[1].Argv)

	// --log 0 reads no journal; --format tsv.
	m, f = serviceModule(t, runner.Expect{Stdout: showOut}, nil)
	r = runGo(t, m, d, env, "llm_service", "nginx", "nosuch", "--log", "0", "--format", "tsv")
	require.Equal(t, "unit\tload\tactive\tsub\tsince\tpid\trestarts\tlog\n"+
		"nginx\tloaded\tactive\trunning\t2026-10-20 10:00:00 UTC\t812\t2\t[]\n"+
		"nosuch\tnot-found\tinactive\tdead\t\\N\t0\t0\t[]\n", r.Stdout)
	require.Len(t, f.Calls(), 1)
}

func TestServiceErrors(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	m, _ := serviceModule(t, runner.Expect{Status: 1, Stderr: "System has not been booted with systemd as init system (PID 1). Can't operate.\nFailed to connect to bus\n"}, nil)
	r := runGo(t, m, d, env, "llm_service", "nginx")
	require.Equal(t, `{"error":"systemd_unavailable","details":"System has not been booted with systemd as init system (PID 1). Can't operate.","action":"CONTINUE","recoverable":true,"hint":"systemd is not running here (a container, WSL without systemd). Skip this check"}`+"\n", r.Stderr)
	require.Equal(t, 1, r.RC)

	m = New()
	m.lookPathFn = func(string) (string, bool) { return "", false }
	r = runGo(t, m, d, env, "llm_service", "nginx")
	require.Contains(t, r.Stderr, `"error":"systemd_unavailable","details":"systemctl is not installed","action":"CONTINUE"`)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{nil, "missing_unit"},
		{[]string{"--log", "5"}, "missing_unit"},
		{[]string{"x", "--log"}, "missing_value"},
		{[]string{"x", "--log", "-1"}, "invalid_log"},
		{[]string{"x", "--user"}, "unknown_option"},
		{[]string{"x", "--format", "csv"}, "invalid_format"},
	} {
		r := runGo(t, New(), d, env, "llm_service", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}

func TestParseUnitShow(t *testing.T) {
	got := parseUnitShow([]byte(showOut))
	require.Equal(t, []unitState{
		{load: "loaded", active: "active", sub: "running", since: "2026-10-20 10:00:00 UTC", pid: "812", restarts: "2"},
		{load: "not-found", active: "inactive", sub: "dead", pid: "0", restarts: "0"},
	}, got)
	require.Equal(t, "2026-10-20 10:00:00 CEST", systemdTime("Tue 2026-10-20 10:00:00 CEST"))
	require.Equal(t, "", systemdTime(""))
	require.Nil(t, intOrNull("x"))
	require.Nil(t, intOrNull(""))
	require.Equal(t, 7, intOrNull("7"))
}
