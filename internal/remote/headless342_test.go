// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestHeadlessEnvelopes: headless, every refusal of remote_exec,
// remote_push, remote_pull and the remote_sync_* functions is one error
// envelope line on stderr with its code, and nothing is started; for a
// person the Error: line stays (3.4.2).
func TestHeadlessEnvelopes(t *testing.T) {
	s := newGoScene(t, boxEnv)
	script := filepath.Join(t.TempDir(), "empty.sh")
	require.NoError(t, os.WriteFile(script, []byte("\n\n"), 0o644))
	for _, c := range []struct {
		fn, code, details string
		args              []string
	}{
		{"remote_exec", "unknown_host", "Unknown host alias 'unknown-alias'.", []string{"unknown-alias", "true"}},
		{"remote_push", "unknown_host", "Unknown host alias 'nohost'.", []string{"nohost", "./x", "/tmp/x"}},
		{"remote_exec", "missing_host", "Missing host alias/target.", nil},
		{"remote_exec", "empty_command", "Missing remote command.", []string{"box"}},
		{"remote_exec", "unexpected_argument", "SINGLE quoted string", []string{"box", "ls", "-la"}},
		{"remote_exec", "missing_file", "Missing script file.", []string{"box", "--script"}},
		{"remote_exec", "path_unreadable", "Script file not readable: /nonexistent/s.sh", []string{"box", "--script", "/nonexistent/s.sh"}},
		{"remote_exec", "empty_command", "Script is empty: " + script, []string{"box", "--script", script}},
		{"remote_pull", "missing_value", "Missing local destination.", []string{"box", "/tmp/x"}},
		{"remote_push", "write_path_denied", "Remote path '/etc/shadow' is in the DENY list", []string{"box", "./x", "/etc/shadow", "--force"}},
		{"remote_pull", "forbidden_path", "Remote path '/etc/shadow' is in the DENY list", []string{"box", "/etc/shadow", "./x"}},
		{"remote_sync_pull", "forbidden_path", "Remote path '/proc' is in the DENY list", []string{"box", "/proc", "./x"}},
		{"remote_push", "sensitive_path", "Remote path '/etc/app.conf' is sensitive", []string{"box", "./x", "/etc/app.conf"}},
		{"remote_pull", "invalid_path", "contains characters the remote side may expand", []string{"box", "/tmp/*", "./x"}},
	} {
		r := s.runGo(headless, c.fn, nil, c.args...)
		require.Equal(t, 1, r.RC, "%s %q", c.fn, c.args)
		code, details := envelope(t, r.Err)
		require.Equal(t, c.code, code, "%s %q", c.fn, c.args)
		require.Contains(t, details, c.details, "%s %q", c.fn, c.args)
		require.Empty(t, r.Out)
		require.Empty(t, r.Log, "nothing is started: %s %q", c.fn, c.args)

		// A person gets the Error: line (an unknown alias may be a raw
		// user@host there, a sensitive path is a question).
		if c.code != "unknown_host" && c.code != "sensitive_path" {
			r = s.runGo(plain, c.fn, nil, c.args...)
			require.True(t, strings.HasPrefix(r.Err, "Error: "), "%s %q: %q", c.fn, c.args, r.Err)
			require.Contains(t, r.Err, c.details)
		}
	}

	// A connection that fails (ssh 255, and the probe fails too) is
	// host_unreachable; a remote command that exited 255 over a healthy
	// connection is its own status, with no envelope.
	s.files["ssh.status"] = "255 255"
	r := s.runGo(headless, "remote_exec", nil, "box", "true")
	require.Equal(t, 1, r.RC)
	code, details := envelope(t, r.Err)
	require.Equal(t, "host_unreachable", code)
	require.Equal(t, "Failed to connect to deploy@box.example. Verify SSH connectivity and key configuration.", details)
	s.files["ssh.status"] = "255 0"
	r = s.runGo(headless, "remote_exec", nil, "box", "exit 255")
	require.Equal(t, 255, r.RC)
	require.Empty(t, r.Err)
	r = s.runGo(plain, "remote_exec", nil, "box", "exit 255")
	require.Equal(t, 255, r.RC)
	require.Contains(t, r.Err, "Error: Remote command exited 255")
}
