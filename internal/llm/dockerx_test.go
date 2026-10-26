// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

const dockerPSOut = `{"Command":"\"nginx\"","ID":"a1","Image":"nginx:1.27","Names":"web","Ports":"0.0.0.0:8080->80/tcp, :::8080->80/tcp","State":"running","Status":"Up 2 hours (healthy)"}
{"ID":"b2","Image":"mariadb:11","Names":"db","Ports":"127.0.0.1:3306->3306/tcp","State":"exited","Status":"Exited (1) 3 minutes ago"}
not json
{"ID":"c3","Image":"redis","Names":"cache","Ports":"","Status":"Up 5 seconds (health: starting)"}
`

// dockerModule fakes docker with answer.
func dockerModule(t *testing.T, answer func(argv []string) runner.Expect) (*Module, *runner.Fake) {
	m := New()
	m.lookPathFn = func(name string) (string, bool) { return "/bin/" + name, name == "docker" }
	f := runner.NewFake(t).Respond(func(c runner.Call) (runner.Expect, bool) {
		if c.Argv[0] != "/bin/docker" {
			return runner.Expect{Status: 1}, true // tool probes
		}
		return answer(c.Argv[1:]), true
	})
	m.run = f
	return m, f
}

func TestDockerPS(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), map[string]string{"GITHUB_TOKEN": "secret", "DOCKER_HOST": "unix:///x.sock"})
	m, f := dockerModule(t, func(argv []string) runner.Expect {
		return runner.Expect{Stdout: dockerPSOut}
	})
	r := runGo(t, m, d, env, "llm_docker_ps")
	require.Equal(t, result{`[{"name":"cache","image":"redis","state":"running","health":"starting","ports":"","up":"Up 5 seconds"},` +
		`{"name":"db","image":"mariadb:11","state":"exited","health":"","ports":"127.0.0.1:3306->3306/tcp","up":"Exited (1) 3 minutes ago"},` +
		`{"name":"web","image":"nginx:1.27","state":"running","health":"healthy","ports":"8080->80/tcp","up":"Up 2 hours"}]` + "\n", "", 0}, r)
	c := f.Calls()[0]
	require.Equal(t, []string{"/bin/docker", "ps", "-a", "--no-trunc", "--format", "{{json .}}"}, c.Argv)
	require.Contains(t, c.Env, "DOCKER_HOST=unix:///x.sock")
	for _, kv := range c.Env {
		require.False(t, strings.HasPrefix(kv, "GITHUB_TOKEN="), "a secret reached docker")
	}
	m, _ = dockerModule(t, func([]string) runner.Expect { return runner.Expect{Stdout: dockerPSOut} })
	r = runGo(t, m, d, env, "llm_docker_ps", "--running", "--format", "tsv")
	require.Equal(t, "name\timage\tstate\thealth\tports\tup\ncache\tredis\trunning\tstarting\t\tUp 5 seconds\nweb\tnginx:1.27\trunning\thealthy\t8080->80/tcp\tUp 2 hours\n", r.Stdout)

	// Daemon down: docker_unavailable; no docker: docker_not_found (both CONTINUE).
	m, _ = dockerModule(t, func([]string) runner.Expect {
		return runner.Expect{Status: 1, Stderr: "Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?\n"}
	})
	r = runGo(t, m, d, env, "llm_docker_ps")
	require.Contains(t, r.Stderr, `"error":"docker_unavailable","details":"Cannot connect to the Docker daemon`)
	require.Contains(t, r.Stderr, `"action":"CONTINUE"`)
	m = New()
	m.lookPathFn = func(string) (string, bool) { return "", false }
	r = runGo(t, m, d, env, "llm_docker_ps")
	require.Contains(t, r.Stderr, `"error":"docker_not_found","details":"docker is not installed","action":"CONTINUE"`)
	r = runGo(t, New(), d, env, "llm_docker_ps", "--all")
	require.Contains(t, r.Stderr, `"error":"unknown_option","details":"--all"`)
}

func TestDockerLogs(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	m, f := dockerModule(t, func(argv []string) runner.Expect {
		return runner.Expect{Stdout: "GET / 200\nGET /x 500 error\n", Stderr: "ERROR boom " + strings.Repeat("e", 400) + "\n"}
	})
	r := runGo(t, m, d, env, "llm_docker_logs", "web", "--tail", "50", "--since", "10m", "--grep", "(?i)error")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"ctr":"web","read":3,"log":["GET /x 500 error","ERROR boom `+strings.Repeat("e", 300-len("ERROR boom ")-1)+`…"]}`+"\n", r.Stdout)
	require.Equal(t, []string{"/bin/docker", "logs", "--tail", "50", "--since", "10m", "--", "web"}, f.Calls()[0].Argv)

	m, f = dockerModule(t, func([]string) runner.Expect { return runner.Expect{} })
	r = runGo(t, m, d, env, "llm_docker_logs", "web", "--width", "5")
	require.Equal(t, `{"ctr":"web","read":0,"log":[]}`+"\n", r.Stdout)
	require.Equal(t, []string{"/bin/docker", "logs", "--tail", "100", "--", "web"}, f.Calls()[0].Argv)

	m, _ = dockerModule(t, func([]string) runner.Expect {
		return runner.Expect{Status: 1, Stderr: "Error response from daemon: No such container: nope\n"}
	})
	r = runGo(t, m, d, env, "llm_docker_logs", "nope")
	require.Contains(t, r.Stderr, `"error":"container_not_found","details":"Error response from daemon: No such container: nope","action":"RETRY"`)

	for _, tc := range []struct {
		args []string
		code string
	}{
		{nil, "missing_container"},
		{[]string{"a", "b"}, "unexpected_argument"},
		{[]string{"a", "--since", "-1h"}, "invalid_since"},
		{[]string{"a", "--grep", "("}, "invalid_regex"},
		{[]string{"a", "--tail", "x"}, "invalid_tail"},
		{[]string{"a", "--follow"}, "unknown_option"},
	} {
		r := runGo(t, New(), d, env, "llm_docker_logs", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}

func TestDockerExec(t *testing.T) {
	d := physTemp(t)
	env := baseEnv(physTemp(t), nil)
	var argv []string
	m, _ := dockerModule(t, func(a []string) runner.Expect {
		argv = a
		return runner.Expect{Stdout: "/app/a.php\n/app/b.php\n/app/c.php\n/app/d.php\n", Stderr: "grep: /app/x: Permission denied\n", Status: 2}
	})
	// The three-level example of the plan, with no quoting at all.
	r := runGo(t, m, d, env, "llm_docker_exec", "ctr", "--user", "node", "--head", "2", "--", "grep", "-rl", `say "hi" | it's`, "/app")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, `{"rc":2,"out":"/app/a.php\n/app/b.php\n[... 2 lines omitted]\n","err":"grep: /app/x: Permission denied\n","truncated":2}`+"\n", r.Stdout)
	want := append([]string{"exec", "--user=node", "ctr", "grep"}, pathguard.ExecSecretExcludes("grep")...)
	require.Equal(t, append(want, "-rl", `say "hi" | it's`, "/app"), argv)

	// sed and awk run sandboxed; other commands as given.
	for _, tc := range []struct{ cmd, want []string }{
		{[]string{"sed", "-n", "1,5p", "/etc/hosts"}, []string{"sed", "--sandbox", "-n", "1,5p", "/etc/hosts"}},
		{[]string{"awk", "{print $1}", "/etc/hosts"}, []string{"gawk", "--sandbox", "{print $1}", "/etc/hosts"}},
		{[]string{"cat", "/app/composer.json"}, []string{"cat", "/app/composer.json"}},
		{[]string{"ls", "-la", "$HOME"}, []string{"ls", "-la", "$HOME"}},
	} {
		m, _ := dockerModule(t, func(a []string) runner.Expect { argv = a; return runner.Expect{Stdout: "ok"} })
		args := append([]string{"ctr", "--"}, tc.cmd...)
		r := runGo(t, m, d, env, "llm_docker_exec", args...)
		require.Equal(t, `{"rc":0,"out":"ok","err":""}`+"\n", r.Stdout, tc.cmd)
		require.Equal(t, append([]string{"exec", "ctr"}, tc.want...), argv, tc.cmd)
	}

	// The llm_exec rails refuse before docker runs.
	for _, tc := range []struct {
		cmd  []string
		code string
	}{
		{[]string{"rm", "-rf", "/app"}, "command_not_allowed"},
		{[]string{"sh", "-c", "id"}, "command_not_allowed"},
		{[]string{"git", "log"}, "command_not_allowed"},
		{[]string{"cat", "/app/.env"}, "secret_file_blocked"},
		{[]string{"cat", "/etc/shadow"}, "dangerous_command_blocked"},
		{[]string{"find", "/app", "-delete"}, "option_not_allowed"},
		{[]string{"sed", "-i", "s/a/b/", "/app/x"}, "option_not_allowed"},
		{[]string{"dd", "if=/dev/zero"}, "command_not_allowed"},
	} {
		m, f := dockerModule(t, func([]string) runner.Expect { return runner.Expect{} })
		r := runGo(t, m, d, env, "llm_docker_exec", append([]string{"ctr", "--"}, tc.cmd...)...)
		require.Equal(t, 1, r.RC, tc.cmd)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.cmd)
		for _, c := range f.Calls() {
			require.NotEqual(t, "/bin/docker", c.Argv[0], tc.cmd)
		}
	}

	// GASH_LLM_EXEC_ALLOW names commands that run as given.
	home := physTemp(t)
	mustWrite(t, home+"/.gash_env", "GASH_LLM_EXEC_ALLOW=\"make\"\n")
	m, _ = dockerModule(t, func(a []string) runner.Expect { argv = a; return runner.Expect{} })
	r = runGo(t, m, d, baseEnv(home, nil), "llm_docker_exec", "ctr", "--", "make", "test")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, []string{"exec", "ctr", "make", "test"}, argv)

	// Container errors.
	for _, tc := range [][2]string{
		{"Error response from daemon: No such container: ctr", "container_not_found"},
		{"Error response from daemon: container abc is not running", "container_not_running"},
	} {
		m, _ := dockerModule(t, func([]string) runner.Expect { return runner.Expect{Status: 1, Stderr: tc[0] + "\n"} })
		r := runGo(t, m, d, env, "llm_docker_exec", "ctr", "--", "ls")
		require.Equal(t, 1, r.RC)
		require.Contains(t, r.Stderr, `"error":"`+tc[1]+`"`)
	}

	for _, tc := range []struct {
		args []string
		code string
	}{
		{nil, "missing_container"},
		{[]string{"ctr"}, "missing_command"},
		{[]string{"ctr", "--"}, "missing_command"},
		{[]string{"ctr", "ls"}, "unexpected_argument"},
		{[]string{"--", "ls"}, "missing_container"},
		{[]string{"ctr", "--user"}, "missing_value"},
		{[]string{"ctr", "--user", "-x", "--", "ls"}, "invalid_user"},
		{[]string{"ctr", "--privileged", "--", "ls"}, "unknown_option"},
		{[]string{"ctr", "--head", "0", "--", "ls"}, "invalid_head"},
	} {
		r := runGo(t, New(), d, env, "llm_docker_exec", tc.args...)
		require.Equal(t, 1, r.RC, tc.args)
		require.Contains(t, r.Stderr, `"error":"`+tc.code+`"`, tc.args)
	}
}
