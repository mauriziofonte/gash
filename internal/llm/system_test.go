// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

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
				// 3.0: LLM-06 fixed, LLM-11 fixed
				require.Equal(t, `[{"port":9,"proto":"tcp","state":"LISTEN","address":"[fe80::1]"},{"port":22,"proto":"tcp","state":"LISTEN","address":"[::]"},{"port":22,"proto":"tcp","state":"LISTEN","address":"0.0.0.0"},{"port":53,"proto":"tcp","state":"LISTEN","address":"127.0.0.53%lo"},{"port":68,"proto":"udp","state":"LISTEN","address":"0.0.0.0"},{"port":123,"proto":"udp","state":"LISTEN","address":"[fe80::1]"},{"port":8080,"proto":"tcp","state":"LISTEN","address":"a\"b\\c\u0001"},{"port":44312,"proto":"tcp","state":"ESTAB","address":"10.0.0.2"}]`+"\n", r.Stdout)
			}
		}
	}
}

// TestLedger_LLM11_2x: two sockets on one port and protocol are two
// records (2.x kept one).
func TestLedger_LLM11_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"ss": ssOut})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": filepath.Join(d, "log")})
	// 3.0: LLM-11 fixed
	r := runGo(t, m, d, env, "llm_ports", "--listen")
	require.Equal(t, 2, strings.Count(r.Stdout, `"port":22,`))
}

// TestLedger_LLM11: an IPv4 and an IPv6 listener on port 22 are two
// records; an exact duplicate is one; records sort by port, then protocol.
func TestLedger_LLM11(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"ss": `cat <<'EOF'
Netid State  Recv-Q Send-Q Local Address:Port Peer Address:Port
udp   UNCONN 0      0      0.0.0.0:22         0.0.0.0:*
tcp   LISTEN 0      4096   0.0.0.0:22         0.0.0.0:*
tcp   LISTEN 0      4096   [::]:22            [::]:*
tcp   LISTEN 0      4096   0.0.0.0:022        0.0.0.0:*
tcp   LISTEN 0      4096   0.0.0.0:21         0.0.0.0:*
EOF`})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t)})
	r := runGo(t, m, d, env, "llm_ports", "--listen")
	require.Equal(t, result{`[{"port":21,"proto":"tcp","state":"LISTEN","address":"0.0.0.0"},{"port":22,"proto":"tcp","state":"LISTEN","address":"0.0.0.0"},{"port":22,"proto":"tcp","state":"LISTEN","address":"[::]"},{"port":22,"proto":"udp","state":"LISTEN","address":"0.0.0.0"}]` + "\n", "", 0}, r)
}

// TestLedger_LLM06_2x_ports: llm_ports and llm_procs escape control
// characters as jq does (2.x kept them raw, only \ and " escaped).
func TestLedger_LLM06_2x_ports(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"ss": ssOut, "ps": `printf 'u\x01ser 7 0.0 0.1 1 2 ? S 00:00 0:00 /bin/a"b\x1bc\n'`})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": filepath.Join(d, "log")})
	// 3.0: LLM-06 fixed
	r := runGo(t, m, d, env, "llm_ports")
	require.Contains(t, r.Stdout, `"address":"a\"b\\c\u0001"`)
	r = runGo(t, m, d, env, "llm_procs")
	require.Equal(t, `[{"pid":7,"user":"u\u0001ser","cpu":0.0,"mem":0.1,"name":"/bin/a\"b\u001bc"}]`+"\n", r.Stdout)
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

// TestLedger_LLM12_2x: --port is a plain base-10 JSON number (2.x kept
// the leading zeros, a source text number token) and is range checked
// (2.x had no check).
func TestLedger_LLM12_2x(t *testing.T) {
	m := New()
	d := physTemp(t)
	bin := stubBin(t, map[string]string{
		"ps":   psStub,
		"lsof": `case "$2" in :80|:0080|:99999) echo 4242 ;; esac`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t)})
	// 3.0: LLM-12 fixed
	r := runGo(t, m, d, env, "llm_procs", "--port", "0080")
	require.Equal(t, `[{"pid":4242,"name":"nginx","port":80}]`+"\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_procs", "--port", "99999")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Stderr, `"error":"invalid_port"`)
}

// TestLedger_LLM12: llm_procs --port 0080 parses as strict JSON with port
// 80 (lsof asked for :80); --port 99999 is invalid_port/RETRY with nothing
// on stdout; 65535 is accepted.
func TestLedger_LLM12(t *testing.T) {
	m := New()
	d := physTemp(t)
	log := filepath.Join(d, "lsof.log")
	bin := stubBin(t, map[string]string{
		"ps":   psStub,
		"lsof": `printf '%s\n' "$2" >>"$LSOF_LOG"; case "$2" in :80) echo 4242 ;; esac`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "LSOF_LOG": log})
	r := runGo(t, m, d, env, "llm_procs", "--port", "0080")
	require.Equal(t, []any{map[string]any{"pid": float64(4242), "name": "nginx", "port": float64(80)}}, strictJSON(t, r.Stdout))
	r = runGo(t, m, d, env, "llm_procs", "--port", "65535")
	require.Equal(t, result{"[]\n", "", 0}, r)
	for _, p := range []string{"99999", "65536", "1234567890"} {
		r = runGo(t, m, d, env, "llm_procs", "--port", p)
		require.Equal(t, "", r.Stdout)
		require.Equal(t, 1, r.RC)
		var e struct{ Error, Action, Details string }
		require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
		require.Equal(t, "invalid_port", e.Error)
		require.Equal(t, "RETRY", e.Action)
		require.Contains(t, e.Details, p)
	}
	b, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, ":80\n:65535\n", string(b))
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

// TestLedger_LLM02_2x: 2.x anchored the URL mask at the start of the value
// and ended the password at its first / or @. 3.0: LLM-02 fixed, the same
// inputs are masked whole.
func TestLedger_LLM02_2x(t *testing.T) {
	require.Equal(t, "mysql://u:***@h/d", envRedact("mysql://u:p@ss@h/d"))
	require.Equal(t, "mysql://u:***@h/d", envRedact("mysql://u:pa/ss@h/d"))
	require.Equal(t, "x https://u:***@h/", envRedact("x https://u:pw@h/"))
}

// TestLedger_LLM02: every URL password is masked, to the last @ of its
// token, wherever the URL stands in the value; the separators are kept.
func TestLedger_LLM02(t *testing.T) {
	for in, want := range map[string]string{
		"mysql://u:p@ss@h/d":                 "mysql://u:***@h/d",
		"mysql://u:pa/ss@h/d":                "mysql://u:***@h/d",
		"x https://u:pw@h/":                  "x https://u:***@h/",
		"mysql://user:p@ss@host/db":          "mysql://user:***@host/db",
		"https://u:pw@h/":                    "https://u:***@h/",
		"redis://:pw@h:6379":                 "redis://:***@h:6379",
		"postgres://u@h/d":                   "postgres://u@h/d",
		"a+b.c-d://u:x@h":                    "a+b.c-d://u:***@h",
		"DSN=mysql://u:p@h":                  "DSN=mysql://u:***@h",
		"a\thttps://u:p@h  b\nftp://v:q/r@k": "a\thttps://u:***@h  b\nftp://v:***@k",
		"http://u:p@h/x?r=https://v:q@k":     "http://u:***@k",
		"no url here":                        "no url here",
		"https://host/a:b":                   "https://host/a:b",
	} {
		require.Equal(t, want, envRedact(in), in)
	}
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"MYURL_A": "mysql://user:p@ss@host/db", "MYURL_B": "see mysql://u:pa/ss@h/d"})
	r := runGo(t, m, d, env, "llm_env", "--filter", "MYURL")
	require.Equal(t, result{"{\n  \"MYURL_A\": \"mysql://user:***@host/db\",\n  \"MYURL_B\": \"see mysql://u:***@h/d\"\n}\n", "", 0}, r)
}

// TestLedger_LLM23_2x_env: llm_env values replace invalid UTF-8 with
// U+FFFD, as jq does (2.x passed the bytes through raw).
func TestLedger_LLM23_2x_env(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"BADV": "a\xff\xfeb"})
	r := runGo(t, m, d, env, "llm_env", "^BADV$")
	// 3.0: LLM-23 fixed
	require.Equal(t, "{\n  \"BADV\": \"a\uFFFD\uFFFDb\"\n}\n", r.Stdout)
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

// strictJSON fails unless out is JSON a strict parser (jq -e .) accepts:
// valid UTF-8 and no raw control character inside a string.
func strictJSON(t *testing.T, out string) any {
	t.Helper()
	require.True(t, utf8.ValidString(out), "invalid UTF-8 in %q", out)
	require.True(t, json.Valid([]byte(out)), "invalid JSON: %q", out)
	var v any
	require.NoError(t, json.Unmarshal([]byte(out), &v))
	return v
}

// TestLedger_LLM06: C0 controls and DEL in names and values are escaped on
// every llm_* JSON path, so the output parses and the values round-trip.
func TestLedger_LLM06(t *testing.T) {
	m := New()
	const hostile = "a\x01b\x1bc\x7fd"

	d := physTemp(t)
	mustWrite(t, filepath.Join(d, hostile), "x")
	r := runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_tree")
	require.Equal(t, 0, r.RC)
	tree := strictJSON(t, r.Stdout).(map[string]any)
	require.Equal(t, hostile, tree["children"].([]any)[0].(map[string]any)["name"])

	bin := stubBin(t, map[string]string{"ss": ssOut, "ps": `printf 'u\x01ser 7 0.0 0.1 1 2 ? S 00:00 0:00 /bin/a"b\x1bc\x7f\n'`})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": filepath.Join(d, "log"), "HOSTILE_V": hostile})
	r = runGo(t, m, d, env, "llm_ports")
	require.Contains(t, strictJSON(t, r.Stdout), map[string]any{"proto": "tcp", "address": "a\"b\\c\x01", "port": float64(8080), "state": "LISTEN"})
	r = runGo(t, m, d, env, "llm_procs")
	procs := strictJSON(t, r.Stdout).([]any)
	require.Equal(t, "u\x01ser", procs[0].(map[string]any)["user"])
	require.Equal(t, "/bin/a\"b\x1bc\x7f", procs[0].(map[string]any)["name"])

	r = runGo(t, m, d, env, "llm_env", "^HOSTILE_V$")
	require.Equal(t, map[string]any{"HOSTILE_V": hostile}, strictJSON(t, r.Stdout))
}

// TestLedger_LLM23: invalid UTF-8 becomes U+FFFD on every llm_* JSON path,
// the former raw ones (llm_tree, llm_env) and the jq ones (llm_git_status)
// alike.
func TestLedger_LLM23(t *testing.T) {
	m := New()
	d := physTemp(t)
	if rawNamesOK(t, d) {
		mustWrite(t, filepath.Join(d, "bad\xff\xfe"), "x")
		r := runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_tree")
		tree := strictJSON(t, r.Stdout).(map[string]any)
		require.Equal(t, "bad��", tree["children"].([]any)[0].(map[string]any)["name"])
	}

	r := runGo(t, m, d, baseEnv(physTemp(t), map[string]string{"BADV": "a\xff\xfeb"}), "llm_env", "^BADV$")
	require.Equal(t, map[string]any{"BADV": "a��b"}, strictJSON(t, r.Stdout))

	if _, err := lookPathTest("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := physTemp(t)
	repo := physTemp(t)
	git(t, repo, home, "init", "-b", "main")
	mustWrite(t, filepath.Join(repo, "f"), "x")
	git(t, repo, home, "add", "-A")
	git(t, repo, home, "commit", "-m", "one")
	git(t, repo, home, "checkout", "-b", "br\xff")
	r = runGo(t, m, repo, baseEnv(home, nil), "llm_git_status")
	require.Equal(t, 0, r.RC, r.Stderr)
	st := strictJSON(t, r.Stdout).(map[string]any)
	require.Equal(t, "br\uFFFD", st["branch"])
}
