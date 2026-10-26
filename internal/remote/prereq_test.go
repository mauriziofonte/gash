// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The 3.4 prerequisites of the remote functions: a client missing here is
// the prerequisite report before anything connects; a program missing on
// the host is named, with the host.

func TestWrapWithoutSSH(t *testing.T) {
	defer prereq.SetSystem(debian("ssh"))()
	s := newHostScene(t)
	_, errOut, rc := s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web", "--host", "db", "x")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"prerequisite_missing","details":"llm_x --host needs ssh, which is not installed","action":"STOP","recoverable":true,`+
		`"hint":"Install openssh-client: sudo apt-get install -y openssh-client","missing":"ssh","install":"sudo apt-get install -y openssh-client"}`+"\n", errOut,
		"once for the call, not host_unreachable per host")
	require.Zero(t, s.ssh.N())
	// The rails still answer first.
	_, errOut, _ = s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web", "/proc/1")
	require.Equal(t, "forbidden_path/FATAL", code(t, errOut))
}

// TestTransportWithoutSSH: the backstop under the entry checks. ssh that
// cannot start was host_unreachable (RETRY) "check the host is up".
func TestTransportWithoutSSH(t *testing.T) {
	defer prereq.SetSystem(debian("ssh"))()
	tr := Transport{Run: runner.Exec{}, Env: invoke.EnvMap(map[string]string{"HOME": t.TempDir(), "PATH": t.TempDir()})}
	r := tr.Exec(context.Background(), Host{Alias: "web", Target: "u@web"}, "N", "true", nil, 0)
	e := r.Failure(Host{Alias: "web", Target: "u@web"})
	require.NotNil(t, e)
	require.Equal(t, "prerequisite_missing", e.Code)
	require.Equal(t, "ssh is not installed", e.Details)
	require.False(t, r.Unreachable())

	// A 127 from the remote login shell ("sh: not found") is not a dead
	// connection either: the remote side ended early.
	r = Result{SSH: 127, Stderr: []byte("bash: sh: command not found\n")}
	require.Equal(t, "remote_protocol_error", r.Failure(Host{Alias: "web"}).Code)
}

// remotePath is a remote PATH with only the named programs (and sh, which
// runs the script).
func remotePath(t *testing.T, tools ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range append([]string{"sh"}, tools...) {
		p, err := exec.LookPath(name)
		require.NoError(t, err, name)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	return dir
}

// TestDispatchMissingUploadTools: without uname the platform read "()",
// without wc (or tr) the upload failed as "could not be installed": the
// program is named now, and a fallback still answers.
func TestDispatchMissingUploadTools(t *testing.T) {
	if !shipsHere() {
		t.Skip("no binary is shipped from this platform")
	}
	all := []string{"uname", "mkdir", "cat", "wc", "chmod", "mv", "rm", "rmdir"}
	for _, missing := range []string{"uname", "wc", "mkdir"} {
		s := newHostScene(t)
		s.ssh.Path = remotePath(t, slices.DeleteFunc(slices.Clone(all), func(n string) bool { return n == missing })...)
		_, errOut, rc := s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web", "x")
		require.Equal(t, 1, rc, missing)
		require.Equal(t, `{"error":"prerequisite_missing","details":"llm_x on web needs `+missing+`, which is not installed there","action":"STOP","recoverable":true,`+
			`"hint":"Install `+missing+` on web (usually the coreutils package), then retry","missing":"`+missing+`","host":"web"}`+"\n", errOut)
		require.Equal(t, 1, s.ssh.N(), "nothing shipped")

		spec := Spec{PathFrom: 0, Fallback: func(*invoke.Call, string, []string) (*Plan, bool) {
			return &Plan{Script: `printf '{"fallback":true}\n'`}, true
		}}
		out, errOut, rc := s.call(s.hoster().Wrap("llm_x", spec, local), "--host", "web", "x")
		require.Zero(t, rc, errOut)
		require.Equal(t, `{"host":"web","fallback":true}`+"\n", out, "the fallback answers")
	}
	// Without tr the binary ships: the size check needs no tr.
	s := newHostScene(t)
	s.ssh.Path = remotePath(t, all...)
	out, errOut, rc := s.call(s.hoster().Wrap("llm_x", Spec{PathFrom: 0}, local), "--host", "web", "a")
	require.Zero(t, rc, errOut)
	require.Equal(t, `{"host":"web","fn":"llm_x","argc":1}`+"\n", out)
	require.Equal(t, 2, s.ssh.N())
}

// TestFallbackMissingTool: a fallback that lacks a program on the host
// answers with the "missing" frame, which is the prerequisite envelope
// with the host, not an empty result.
func TestFallbackMissingTool(t *testing.T) {
	s := newHostScene(t)
	s.stubUname()
	spec := Spec{PathFrom: 0, Fallback: func(_ *invoke.Call, nonce string, _ []string) (*Plan, bool) {
		return &Plan{Script: "N=" + Quote(nonce) + "\n" + ShTools() + "gash_tools sh gash-no-such-tool\nprintf '[]\\n'",
			Finish: func(h Host, out, errOut []byte, st int) ([]byte, []byte, int) { return out, nil, st }}, true
	}}
	_, errOut, rc := s.call(s.hoster().Wrap("llm_find", spec, local), "--host", "web", "x")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"prerequisite_missing","details":"llm_find on web needs gash-no-such-tool, which is not installed there","action":"STOP","recoverable":true,`+
		`"hint":"Install gash-no-such-tool on web, then retry","missing":"gash-no-such-tool","host":"web"}`+"\n", errOut)
}

func TestAttributeHost(t *testing.T) {
	prereqLine := `{"error":"prerequisite_missing","details":"llm_procs --port needs lsof, which is not installed","action":"STOP","recoverable":true,"hint":"Install lsof: sudo dnf install -y lsof","missing":"lsof","install":"sudo dnf install -y lsof"}`
	for _, tc := range []struct{ in, want string }{
		{prereqLine + "\n", `{"error":"prerequisite_missing","details":"llm_procs --port needs lsof, which is not installed on web","action":"STOP","recoverable":true,` +
			`"hint":"On web: Install lsof: sudo dnf install -y lsof","missing":"lsof","install":"sudo dnf install -y lsof","host":"web"}` + "\n"},
		{"warning: x\n" + `{"error":"git_not_found","details":"Git is not installed","action":"CONTINUE","recoverable":true,"hint":"Install git"}` + "\n",
			"warning: x\n" + `{"error":"git_not_found","details":"Git is not installed on web","action":"CONTINUE","recoverable":true,"hint":"On web: Install git","host":"web"}` + "\n"},
		{`{"error":"ports_tool_not_found","details":"Neither ss nor netstat is installed","action":"CONTINUE","recoverable":true}`,
			`{"error":"ports_tool_not_found","details":"Neither ss nor netstat is installed on web","action":"CONTINUE","recoverable":true,"host":"web"}` + "\n"},
		// Not a missing program, a host already named, not an envelope.
		{`{"error":"path_not_found","details":"/x","action":"RETRY","recoverable":true}` + "\n", ""},
		{`{"error":"git_not_found","details":"x","action":"CONTINUE","recoverable":true,"host":"db"}` + "\n", ""},
		{"git: not found\n", ""},
		{`{"error":"git_not_found","action":"CONTINUE","recoverable":true,"n":1}`, ""},
		{"", ""},
	} {
		want := tc.want
		if want == "" {
			want = tc.in
		}
		require.Equal(t, want, string(AttributeHost([]byte(tc.in), "web")), tc.in)
	}

	// Through the wrapper: the cached binary on the host lacks git.
	s := newHostScene(t)
	require.NoError(t, os.WriteFile(s.exe, []byte("#!/bin/sh\necho '"+prereqLine+"' >&2; exit 1\n"), 0o755))
	b, _ := LocalBinary(func() (string, error) { return s.exe, nil })
	require.NoError(t, os.MkdirAll(filepath.Join(s.rhome, b.Dir), 0o700))
	data, _ := os.ReadFile(s.exe)
	require.NoError(t, os.WriteFile(filepath.Join(s.rhome, b.Dir, "gash"), data, 0o700))
	_, errOut, rc := s.call(s.hoster().Wrap("llm_procs", Spec{PathFrom: -1}, local), "--host", "web", "--port", "80")
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, `"details":"llm_procs --port needs lsof, which is not installed on web"`)
	require.Contains(t, errOut, `"host":"web"}`)
}
