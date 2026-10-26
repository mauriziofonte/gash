// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const ssProcsOut = `printf '%s\n' "$*" >>"$STUB_LOG"
cat <<'EOF'
Netid State  Recv-Q Send-Q Local Address:Port  Peer Address:Port Process
tcp   LISTEN 0      511    0.0.0.0:8080        0.0.0.0:*         users:(("nginx",pid=812,fd=6),("nginx",pid=811,fd=6),("nginx",pid=812,fd=6))
tcp   LISTEN 0      511    [::]:8080           [::]:*            users:(("Web Content",pid=90,fd=7))
udp   UNCONN 0      0      0.0.0.0:8080        0.0.0.0:*
tcp   LISTEN 0      4096   0.0.0.0:22          0.0.0.0:*         users:(("sshd",pid=700,fd=3))
tcp   LISTEN 0      4096   127.0.0.1:08080     0.0.0.0:*         users:(("odd\"name",pid=5,fd=3))
EOF
[[ "$1" == *l* ]] || echo "tcp   ESTAB  0      0      10.0.0.2:8080       1.2.3.4:443       users:((\"nginx\",pid=812,fd=9))"`

func TestPortsByPortWithProcesses(t *testing.T) {
	m := New()
	d := physTemp(t)
	log := filepath.Join(d, "log")
	bin := stubBin(t, map[string]string{"ss": ssProcsOut})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t), "STUB_LOG": log})
	r := runGo(t, m, d, env, "llm_ports", "--listen", "--port", "8080")
	require.Equal(t, result{`[` +
		`{"port":8080,"proto":"tcp","state":"LISTEN","address":"0.0.0.0","procs":[{"pid":811,"name":"nginx"},{"pid":812,"name":"nginx"}]},` +
		`{"port":8080,"proto":"tcp","state":"LISTEN","address":"[::]","procs":[{"pid":90,"name":"Web Content"}]},` +
		`{"port":8080,"proto":"tcp","state":"LISTEN","address":"127.0.0.1","procs":[{"pid":5,"name":"odd\"name"}]},` +
		`{"port":8080,"proto":"udp","state":"LISTEN","address":"0.0.0.0","procs":[]}]` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_ports", "--port", "08080")
	require.Contains(t, r.Stdout, `{"port":8080,"proto":"tcp","state":"ESTAB","address":"10.0.0.2","procs":[{"pid":812,"name":"nginx"}]}`)
	r = runGo(t, m, d, env, "llm_ports", "--port", "9")
	require.Equal(t, result{"[]\n", "", 0}, r)
	calls, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, "-tunlp\n-tunap\n-tunap\n", string(calls))

	// Without --port the output is the 3.0 one, byte for byte.
	r = runGo(t, m, d, env, "llm_ports", "--listen")
	require.NotContains(t, r.Stdout, "procs")
	require.True(t, strings.HasPrefix(r.Stdout, `[{"port":22,"proto":"tcp","state":"LISTEN","address":"0.0.0.0"}`), r.Stdout)
}

func TestPortsByPortNetstat(t *testing.T) {
	m := New()
	m.goos = "linux" // the net-tools netstat; TestPortsBSD has macOS's
	d := physTemp(t)
	bin := stubBin(t, map[string]string{"netstat": `cat <<'EOF'
Active Internet connections (only servers)
Proto Recv-Q Send-Q Local Address           Foreign Address         State       PID/Program name
tcp        0      0 0.0.0.0:3306            0.0.0.0:*               LISTEN      1201/mysqld
tcp6       0      0 :::3306                 :::*                    LISTEN      -
udp        0      0 0.0.0.0:3306            0.0.0.0:*                           77/odd prog
EOF`})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t)})
	r := runGo(t, m, d, env, "llm_ports", "-l", "--port", "3306")
	require.Equal(t, result{`[` +
		`{"port":3306,"proto":"tcp","state":"LISTEN","address":"0.0.0.0","procs":[{"pid":1201,"name":"mysqld"}]},` +
		`{"port":3306,"proto":"tcp","state":"LISTEN","address":"::","procs":[]},` +
		`{"port":3306,"proto":"udp","state":"LISTEN","address":"0.0.0.0","procs":[{"pid":77,"name":"odd prog"}]}]` + "\n", "", 0}, r)
}

func TestPortsByPortErrors(t *testing.T) {
	m := New()
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"PATH": toolsPath(t)})
	code := func(args ...string) string {
		r := runGo(t, m, d, env, "llm_ports", args...)
		require.Equal(t, 1, r.RC, args)
		return r.Stderr
	}
	require.Contains(t, code("--port", "x"), `"error":"invalid_port"`)
	require.Contains(t, code("--port", "65536"), `"error":"invalid_port"`)
	require.Contains(t, code("--port", ""), `"error":"invalid_port"`)
	require.Contains(t, code("--port"), `"error":"missing_value"`)
	require.Contains(t, code("--port", "80", "x"), `"error":"unknown_option"`)
	require.Contains(t, code("--port", "80"), `"error":"ports_tool_not_found"`)
}

// The netstat of macOS 14 (netstat -an -p tcp, then -p udp): ADDR.PORT,
// tcp4/tcp6/tcp46, no state for udp.
const bsdNetstatTCP = `Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)    
tcp4       0      0  192.168.1.23.52614     17.57.146.24.5223      ESTABLISHED
tcp6       0      0  *.7000                 *.*                    LISTEN     
tcp4       0      0  *.7000                 *.*                    LISTEN     
tcp4       0      0  127.0.0.1.631          *.*                    LISTEN     
tcp6       0      0  ::1.631                *.*                    LISTEN     
tcp46      0      0  *.3306                 *.*                    LISTEN     
tcp4       0      0  127.0.0.1.3306         127.0.0.1.52710        ESTABLISHED
tcp4       0      0  127.0.0.1.8080         127.0.0.1.52711        TIME_WAIT  
`

const bsdNetstatUDP = `Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)    
udp4       0      0  *.5353                 *.*                               
udp6       0      0  *.5353                 *.*                               
udp4       0      0  192.168.1.23.123       *.*                               
udp4       0      0  127.0.0.1.53           127.0.0.1.60000                   
udp46      0      0  *.*                    *.*                               
`

// lsof -nP -F pcPn -i :3306 on macOS: mysqld's listener and an accepted
// connection, and a client whose remote port is 3306.
const bsdLsof3306 = "p501\ncmysqld\nf20\nPTCP\nn*:3306\nf31\nPTCP\nn127.0.0.1:3306->127.0.0.1:52710\n" +
	"p777\ncphp\nf5\nPTCP\nn127.0.0.1:52710->127.0.0.1:3306\n"

func TestParseNetstatBSD(t *testing.T) {
	out := []byte(bsdNetstatTCP + bsdNetstatUDP)
	require.Equal(t, [][4]string{
		{"7000", "tcp", "LISTEN", "*"}, {"7000", "tcp", "LISTEN", "*"}, {"631", "tcp", "LISTEN", "127.0.0.1"},
		{"631", "tcp", "LISTEN", "::1"}, {"3306", "tcp", "LISTEN", "*"},
		{"5353", "udp", "LISTEN", "*"}, {"5353", "udp", "LISTEN", "*"}, {"123", "udp", "LISTEN", "192.168.1.23"},
	}, parseNetstatBSD(out, true))
	all := parseNetstatBSD(out, false)
	require.Len(t, all, 12)
	require.Contains(t, all, [4]string{"52614", "tcp", "ESTABLISHED", "192.168.1.23"})
	require.Contains(t, all, [4]string{"53", "udp", "LISTEN", "127.0.0.1"})
	require.Equal(t, []lsofSock{
		{proc: portProc{501, "mysqld"}, proto: "tcp", addr: "*", port: "3306"},
		{proc: portProc{501, "mysqld"}, proto: "tcp", addr: "127.0.0.1", port: "3306"},
		{proc: portProc{777, "php"}, proto: "tcp", addr: "127.0.0.1", port: "52710"},
	}, parseLsofF([]byte(bsdLsof3306)))
}

// TestPortsBSD: on macOS llm_ports reads the BSD netstat (2.x and 3.4
// before: ADDR.PORT did not parse, so the result was [] with exit 0), and
// --port takes the processes from lsof.
func TestPortsBSD(t *testing.T) {
	m := New()
	m.goos = "darwin"
	d := physTemp(t)
	bin := stubBin(t, map[string]string{
		"netstat": `case "$*" in "-an -p tcp") cat <<'EOF'
` + bsdNetstatTCP + `EOF
;; "-an -p udp") cat <<'EOF'
` + bsdNetstatUDP + `EOF
;; *) exit 64 ;; esac`,
		"lsof": `[[ "$*" == "-nP -F pcPn -i :3306" ]] && printf '` + strings.ReplaceAll(bsdLsof3306, "\n", `\n`) + `'`,
	})
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin + ":" + toolsPath(t)})
	r := runGo(t, m, d, env, "llm_ports", "--listen")
	require.Equal(t, result{`[{"port":123,"proto":"udp","state":"LISTEN","address":"192.168.1.23"},` +
		`{"port":631,"proto":"tcp","state":"LISTEN","address":"127.0.0.1"},{"port":631,"proto":"tcp","state":"LISTEN","address":"::1"},` +
		`{"port":3306,"proto":"tcp","state":"LISTEN","address":"*"},{"port":5353,"proto":"udp","state":"LISTEN","address":"*"},` +
		`{"port":7000,"proto":"tcp","state":"LISTEN","address":"*"}]` + "\n", "", 0}, r)
	r = runGo(t, m, d, env, "llm_ports", "--port", "3306")
	require.Equal(t, result{`[{"port":3306,"proto":"tcp","state":"LISTEN","address":"*","procs":[{"pid":501,"name":"mysqld"}]},` +
		`{"port":3306,"proto":"tcp","state":"ESTABLISHED","address":"127.0.0.1","procs":[{"pid":501,"name":"mysqld"}]}]` + "\n", "", 0}, r)

	// Without lsof the rows come with no process.
	require.NoError(t, os.Remove(filepath.Join(bin, "lsof")))
	r = runGo(t, m, d, env, "llm_ports", "--port", "631")
	require.Equal(t, result{`[{"port":631,"proto":"tcp","state":"LISTEN","address":"127.0.0.1","procs":[]},` +
		`{"port":631,"proto":"tcp","state":"LISTEN","address":"::1","procs":[]}]` + "\n", "", 0}, r)
}

// TestPortsWithoutTools: neither ss nor netstat is reported through the
// prerequisites (ports_tool_not_found, with the package of this system).
func TestPortsWithoutTools(t *testing.T) {
	d := physTemp(t)
	path := toolsPath(t)
	pinDebian(t, path)
	env := baseEnv(physTemp(t), map[string]string{"PATH": path})
	want := `{"error":"ports_tool_not_found","details":"NEED needs ss, which is not installed","action":"CONTINUE","recoverable":true,` +
		`"hint":"Install iproute2: sudo apt-get install -y iproute2","missing":"ss","install":"sudo apt-get install -y iproute2"}` + "\n"
	r := runGo(t, New(), d, env, "llm_ports")
	require.Equal(t, result{"", strings.Replace(want, "NEED", "llm_ports", 1), 1}, r)
	r = runGo(t, New(), d, env, "llm_ports", "-l", "--port", "80")
	require.Equal(t, result{"", strings.Replace(want, "NEED", "llm_ports --port", 1), 1}, r)
}
