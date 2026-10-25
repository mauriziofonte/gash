// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/llmfmt"
)

func TestParsePSAgent(t *testing.T) {
	out := `{"Names":"web","Image":"nginx:1.27","State":"running","Status":"Up 2 hours (healthy)","Ports":"0.0.0.0:8080->80/tcp, :::8080->80/tcp"}
not json
{"Names":"db","Image":"mysql:8","State":"exited","Status":"Exited (0) 3 days ago","Ports":""}

{"Names":"api","Image":"app","Status":"Up 5 minutes (health: starting)","Ports":"127.0.0.1:9000->9000/tcp"}
{"Names":"old","Image":"x","Status":"Up 1 hour (Paused)"}
`
	require.Equal(t, []Container{
		{Name: "api", Image: "app", State: "running", Health: "starting", Ports: "127.0.0.1:9000->9000/tcp", Up: "Up 5 minutes"},
		{Name: "db", Image: "mysql:8", State: "exited", Up: "Exited (0) 3 days ago"},
		{Name: "old", Image: "x", State: "paused", Up: "Up 1 hour (Paused)"},
		{Name: "web", Image: "nginx:1.27", State: "running", Health: "healthy", Ports: "8080->80/tcp", Up: "Up 2 hours"},
	}, ParsePS([]byte(out)))
	require.Empty(t, ParsePS(nil))

	for status, want := range map[string]string{
		"Restarting (1) 5 seconds ago": "restarting", "Created": "created", "Dead": "dead", "Removal In Progress": "removal in progress",
	} {
		require.Equal(t, want, stateOf(status), status)
	}
	require.Equal(t, "8080->80/tcp, 443/tcp, 10.0.0.1:53->53/udp", compactPorts("0.0.0.0:8080->80/tcp, [::]:8080->80/tcp, 443/tcp, 10.0.0.1:53->53/udp"))
	require.Equal(t, "", compactPorts(""))
}

func TestFailure(t *testing.T) {
	for stderr, want := range map[string]string{
		"Error response from daemon: No such container: web":                                                "container_not_found",
		"Error response from daemon: container 4f2a is not running":                                         "container_not_running",
		"Error response from daemon: Container 4f2a is paused, unpause the container before exec":           "container_not_running",
		"Cannot connect to the Docker daemon at unix:///var/run/docker.sock. Is the docker daemon running?": "docker_unavailable",
		"permission denied while trying to connect to the Docker daemon socket":                             "docker_unavailable",
		"OCI runtime exec failed: exec failed: unable to start container process":                           "docker_error",
	} {
		class := Failure([]byte(stderr))
		require.Equal(t, want, class, stderr)
		require.NotEmpty(t, FailureHint(class))
	}
	require.Equal(t, "second", FirstLine([]byte("\n  \n second \nthird\n")))
	require.Equal(t, "", FirstLine(nil))
}

func TestExecArgsAndShape(t *testing.T) {
	require.Equal(t, []string{"exec", "web", "ls", "-la", "a b"}, ExecArgs("web", "", []string{"ls", "-la", "a b"}))
	require.Equal(t, []string{"exec", "--user=www-data", "web", "id"}, ExecArgs("web", "www-data", []string{"id"}))

	o := llmfmt.New(llmfmt.BudgetFlags)
	require.Equal(t, ExecOut{RC: 2, Out: "a\nb", Err: "e\n"}, ShapeExec(o, 2, []byte("a\nb"), []byte("e\n")))
	require.Equal(t, ExecOut{Out: "bad �"}, ShapeExec(o, 0, []byte("bad \xff"), nil))

	o.Head, o.Tail, o.Width = 1, 1, 4
	got := ShapeExec(o, 0, []byte("line1\nline2\nline3\nline4\n"), []byte("x\n"))
	require.Equal(t, ExecOut{Out: "lin…\n[... 2 lines omitted]\nlin…\n", Err: "x\n", Truncated: 2}, got)
}
