// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// stubBin writes executable scripts into a new directory and returns it.
func stubBin(t *testing.T, scripts map[string]string) string {
	t.Helper()
	bin := physTemp(t)
	for name, body := range scripts {
		p := filepath.Join(bin, name)
		require.NoError(t, os.WriteFile(p, []byte("#!/usr/bin/env bash\n"+body+"\n"), 0o755))
	}
	return bin
}

// toolsPath is a directory with links to the text tools 2.x pipelines use,
// so a PATH can leave the others out.
func toolsPath(t *testing.T, tools ...string) string {
	t.Helper()
	bin := physTemp(t)
	for _, tool := range append([]string{"bash", "env", "awk", "sort", "head", "cat", "tr", "sed", "grep", "realpath", "readlink"}, tools...) {
		if p, err := lookPathTest(tool); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, tool))
		}
	}
	return bin
}

const ssOut = `printf '%s\n' "$*" >>"$STUB_LOG"
cat <<'EOF'
Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port
tcp   LISTEN 0      4096   [::]:22            [::]:*
tcp   LISTEN 0      4096   0.0.0.0:22         0.0.0.0:*
udp   UNCONN 0      0      0.0.0.0:68         0.0.0.0:*
tcp   LISTEN 0      4096   127.0.0.53%lo:053  0.0.0.0:*
tcp   LISTEN 0      4096   a"b\c` + "\x01" + `:8080 0.0.0.0:*
tcp   LISTEN 0      4096   *:*                0.0.0.0:*
tcp   LISTEN 0      4096   noport             0.0.0.0:*
udp   UNCONN 0      0      [fe80::1]:123      0.0.0.0:*
tcp   LISTEN 0      4096   [fe80::1]:9
EOF
[[ "$1" == *l* ]] || echo "tcp   ESTAB  0      0      10.0.0.2:44312     1.2.3.4:443"`

func TestPortsMatches2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	tools := toolsPath(t)
	log := filepath.Join(d, "log")
	for _, c := range []struct {
		name    string
		scripts map[string]string
	}{
		{"ss", map[string]string{"ss": ssOut}},
		{"ss without Netid", map[string]string{"ss": `cat <<'EOF'
State  Recv-Q Send-Q Local Address:Port Peer Address:Port
LISTEN 0      4096   [::]:22            [::]:*
ESTAB  0      0      10.0.0.2:44312     1.2.3.4:443
EOF`}},
		{"ss empty", map[string]string{"ss": `exit 1`}},
		{"netstat", map[string]string{"netstat": `cat <<'EOF'
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 0.0.0.0:22              0.0.0.0:*               LISTEN
tcp6       0      0 :::22                   :::*                    LISTEN
udp        0      0 0.0.0.0:68              0.0.0.0:*
udp6       0      0 :::5353                 :::*
tcp        0      0 10.0.0.2:44312          1.2.3.4:443             ESTABLISHED
EOF`}},
		{"none", nil},
	} {
		bin := stubBin(t, c.scripts)
		env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + tools, "STUB_LOG": log})
		for _, args := range [][]string{{}, {"--listen"}, {"-l"}, {"-l", "--listen"}, {"--bogus"}, {"-l", "x"}, {"x", "--help"}} {
			r := same(t, m, d, env, "llm_ports", args...)
			if c.name == "ss" && len(args) == 0 {
				require.Equal(t, `[{"port":9,"proto":"tcp","state":"LISTEN","address":"[fe80::1]"},{"port":22,"proto":"tcp","state":"LISTEN","address":"[::]"},{"port":53,"proto":"tcp","state":"LISTEN","address":"127.0.0.53%lo"},{"port":68,"proto":"udp","state":"LISTEN","address":"0.0.0.0"},{"port":123,"proto":"udp","state":"LISTEN","address":"[fe80::1]"},{"port":8080,"proto":"tcp","state":"LISTEN","address":"a\"b\\c`+"\x01"+`"},{"port":44312,"proto":"tcp","state":"ESTAB","address":"10.0.0.2"}]`+"\n", r.Stdout)
			}
		}
	}
}

// TestLedger_LLM11_2x: two sockets on one port and protocol are one record.
func TestLedger_LLM11_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"ss": ssOut})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": filepath.Join(d, "log")})
	r := same(t, m, d, env, "llm_ports", "--listen")
	require.Equal(t, 1, strings.Count(r.Stdout, `"port":22,`))
}

// TestLedger_LLM06_2x_ports: llm_ports and llm_procs keep control
// characters raw (only \ and " escaped).
func TestLedger_LLM06_2x_ports(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"ss": ssOut, "ps": `printf 'u\x01ser 7 0.0 0.1 1 2 ? S 00:00 0:00 /bin/a"b\x1bc\n'`})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": filepath.Join(d, "log")})
	r := same(t, m, d, env, "llm_ports")
	require.Contains(t, r.Stdout, `"address":"a\"b\\c`+"\x01"+`"`)
	r = same(t, m, d, env, "llm_procs")
	require.Equal(t, `[{"pid":7,"user":"u`+"\x01"+`ser","cpu":0.0,"mem":0.1,"name":"/bin/a\"b`+"\x1b"+`c"}]`+"\n", r.Stdout)
}

const psStub = `case "$*" in
  "aux --no-headers") for i in $(seq 1 25); do echo "user$i $i 1.5 0.$i 100 200 pts/0 S 10:00 0:00 /usr/bin/cmd$i --flag lc=$LC_ALL"; done ;;
  "-o ppid= -p "*) echo "    1" ;;
  "-o ppid=,args= -p 101") echo "  50 sleep 30" ;;
  "-o ppid=,args= -p 102") echo "  51   sleep   30  " ;;
  "-o ppid=,args= -p 103") ;;
  "-p 101 -o comm=") echo "sleep" ;;
  "-p 102 -o comm=") printf 'we"ird\\name\x01\n\n' ;;
  "-p 4242 -o comm=") echo "nginx" ;;
  "-p 4243 -o comm=") ;;
  *) echo "unexpected ps $*" >&2; exit 1 ;;
esac`

func TestProcsMatches2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{
		"ps":    psStub,
		"pgrep": `[[ "$1" == -f ]] || exit 3; case "$2" in sleep*) printf '101\n102\n103\nxyz\n' ;; *) exit 1 ;; esac`,
		"lsof":  `[[ "$1 $3" == "-i -t" ]] || exit 3; case "$2" in :80|:0080) printf '4242\n 4243 \nbad\n' ;; *) exit 1 ;; esac`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t, "seq")})
	for _, args := range [][]string{
		{},
		{"--name", "sleep 30"},
		{"--name", "nothing"},
		{"--name", ""},
		{"--port", "80"},
		{"--port", "0080"},
		{"--port", "81"},
		{"--port", ""},
		{"--name", "sleep", "--port", "80"},
		{"--port", "1x"},
		{"--port", "99999"},
		{"--port", "1234567890"},
		{"--port"},
		{"--name"},
		{"--bogus"},
		{"x"},
		{"--name", "x", "-h"},
	} {
		same(t, m, d, env, "llm_procs", args...)
	}
	// Without lsof, pgrep or ps: empty arrays.
	env["PATH"] = toolsPath(t)
	for _, args := range [][]string{{}, {"--name", "x"}, {"--port", "80"}} {
		same(t, m, d, env, "llm_procs", args...)
	}
}

// TestLedger_LLM12_2x: --port keeps leading zeros in the JSON (a source
// text number token, as 2.x printed it) and has no range check.
func TestLedger_LLM12_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{
		"ps":   psStub,
		"lsof": `case "$2" in :0080|:99999) echo 4242 ;; esac`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t)})
	r := same(t, m, d, env, "llm_procs", "--port", "0080")
	require.Equal(t, `[{"pid":4242,"name":"nginx","port":0080}]`+"\n", r.Stdout)
	r = same(t, m, d, env, "llm_procs", "--port", "99999")
	require.Equal(t, `[{"pid":4242,"name":"nginx","port":99999}]`+"\n", r.Stdout)
}

// TestProcsByNameLeavesOutSelf: this process, its ancestors and their
// same-command-line children are not listed.
func TestProcsByNameLeavesOutSelf(t *testing.T) {
	m := New()
	m.getpid = func() int { return 500 }
	f := runner.NewFake(t,
		runner.Expect{Argv: []string{"/bin/ps", "-o", "ppid=", "-p", "500"}, Stdout: "  400\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-o", "ppid=", "-p", "400"}, Stdout: "  1\n"},
		runner.Expect{Argv: []string{"/bin/pgrep", "-f", "x"}, Stdout: "500\n400\n401\n402\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-o", "ppid=,args=", "-p", "401"}, Stdout: "400 bash -c x\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-o", "args=", "-p", "400"}, Stdout: "bash -c x\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-o", "ppid=,args=", "-p", "402"}, Stdout: "400 x-server\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-o", "args=", "-p", "400"}, Stdout: "bash -c x\n"},
		runner.Expect{Argv: []string{"/bin/ps", "-p", "402", "-o", "comm="}, Stdout: "x-server\n"},
	)
	m.run = f
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": "/bin"})
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, true }
	r := runGo(t, m, d, env, "llm_procs", "--name", "x")
	require.Equal(t, result{`[{"pid":402,"name":"x-server"}]` + "\n", "", 0}, r)
	require.NoError(t, f.Verify())
}

func TestEnvMatches2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	t.Chdir(d) // bash then starts in d: its cd leaves OLDPWD=d
	wd := d
	home := physTemp(t)
	extra := map[string]string{
		"MYSQL_PWD": "a", "GH_PAT": "b", "PASSPHRASE": "c", "MY_JWT": "d", "SLACK_WEBHOOK_URL": "e",
		"SESSION_ID": "f", "BEARER": "g", "APP_SALT": "h", "ENCRYPTION_PASS": "i", "PRIVATE_THING": "j",
		"lower_token": "k", "PATH_KEYS": "l", "NOTES": "key AKIAABCDEFGHIJKLMNOP and ghp_abcdefghijklmnopqrstuvwxyz0123 end ghp_abcdefghijklmnopqrstuvwxyz0123",
		"API_BASE": "https://u:pw@example.com/x", "BAD_URL": "mysql://u:p@ss@h/d", "SLASH_URL": "mysql://u:pa/ss@h/d",
		"LATE_URL": "x https://u:pw@h/", "SAFE_VALUE": "plain", "PATCH_LEVEL": "3",
		"PEMDATA": "-----BEGIN OPENSSH PRIVATE KEY-----\nabc", "MULTI": "a\nb=c\td\"e\\f\x01g\x7f\xffh",
		"SK": "sk-ant-abcdefghijklmnopqrstuvwxyz", "SLACKY": "xoxb-1234567890-abc", "GOOG": "AIza" + strings.Repeat("x", 35),
		"GOOD_LONG": "github_pat_" + strings.Repeat("Z", 30), "UNICODE": "café ☃",
		"PWD": d, "OLDPWD": wd,
	}
	env := baseEnv(home, extra)
	goEnv := baseEnv(home, extra)
	goEnv["SHLVL"] = "1" // what bash -c exports
	for _, args := range [][]string{
		{},
		{"--filter", "^(DB_URL|GITHUB_TOKEN|NOTES|API_BASE)$"},
		{"-f", "URL"},
		{"URL"},
		{"--filter", "^NOPE$"},
		{"--filter", ""},
		{"--filter", "[[:upper:]]_[A-Z]"},
		{"--filter", "("},
		{"--filter"},
		{"--bogus"},
		{"a", "--filter", "PATH"},
		{"--filter", "x", "--help"},
	} {
		want := run2x(t, d, env, "llm_env", args...)
		got := runGo(t, m, d, goEnv, "llm_env", args...)
		require.Equal(t, want, got, "%q", args)
	}
	// The output is the caller's exported set: GASH__ names and the names
	// the stub added are not listed.
	goEnv["GASH__ADDED"] = "COLUMNS"
	goEnv["COLUMNS"] = "80"
	goEnv["GASH__HELP"] = "1"
	got := runGo(t, m, d, goEnv, "llm_env", "--filter", "COLUMNS|GASH__")
	require.Equal(t, "{\n}\n", got.Stdout)
	var v map[string]string
	require.NoError(t, json.Unmarshal([]byte(runGo(t, m, d, goEnv, "llm_env", "NOTES").Stdout), &v))
	require.Equal(t, "key *** and *** end ***", v["NOTES"])
}

// TestLedger_LLM02_2x: the URL mask is anchored and ends at the first / or
// @ of the password.
func TestLedger_LLM02_2x(t *testing.T) {
	require.Equal(t, "mysql://u:***@ss@h/d", envRedact("mysql://u:p@ss@h/d"))
	require.Equal(t, "mysql://u:pa/ss@h/d", envRedact("mysql://u:pa/ss@h/d"))
	require.Equal(t, "x https://u:pw@h/", envRedact("x https://u:pw@h/"))
}

// TestLedger_LLM23_2x_env: llm_env values pass invalid UTF-8 through raw.
func TestLedger_LLM23_2x_env(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"BADV": "a\xff\xfeb"})
	r := runGo(t, m, d, env, "llm_env", "^BADV$")
	require.Equal(t, "{\n  \"BADV\": \"a\xff\xfeb\"\n}\n", r.Stdout)
}

// TestLedger_LLM39: llm_env leaves out _, whatever its value (in Go mode
// the binary's path), as the 2.x Bash here does.
func TestLedger_LLM39(t *testing.T) {
	archive2x(t) // before runGo changes the directory
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"_": "/usr/local/bin/gash", "PWD": d})
	r := runGo(t, m, d, env, "llm_env")
	require.Equal(t, 0, r.RC)
	require.NotContains(t, r.Stdout, `"_"`)
	require.Contains(t, r.Stdout, `"PWD": `)
	require.Equal(t, result{"{\n}\n", "", 0}, runGo(t, m, d, env, "llm_env", "--filter", "^_$"))
	require.Equal(t, result{"{\n}\n", "", 0}, same(t, m, d, env, "llm_env", "--filter", "^_$"))
}
