// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// pathVectors are the remote_spec.sh vectors and more: tables, prefixes,
// normalization, homes, .ssh, traversal and the strings 2.x compares
// literally (LEDGER REM-03).
var pathVectors = []string{
	// remote_spec.sh: DENY, ASK, ALLOW
	"/etc/shadow", "/etc/passwd", "/etc/ssh/sshd_config", "/boot/grub", "/usr/bin/bash", "/",
	"/tmp/../etc/shadow", "/etc/systemd/system/myapp.service", "/etc/crontab", "/etc/init.d/apache2",
	"/root/.bashrc", "/tmp/foo", "/var/tmp/foo", "/home/maurizio/project", "relative/path", "/opt/myapp",
	// normalization
	"//etc/shadow", "/etc//shadow", "/./etc/shadow", "/etc/./ssh/", "/etc/shadow/", "///usr//bin/sh", "//",
	"~/.ssh/authorized_keys", ".ssh/id_ed25519", "~root/.ssh/x", "/home/bob/.ssh/", "./.ssh", "~/notes.txt",
	"..config", "a/../../etc", "~", "~/", "~/project", "~bob", "~bob/", "~root", "~root/x", "~bob/.bashrc",
	"~/a//b/", "~//etc/shadow", "/.", "//.", "/./", ".", "./", "./x", "x/.", "x/./y", "/././etc/shadow",
	"", "..", "../x", "x/..", "/..", "a/b/..",
	// .ssh anywhere
	"/srv/git/.ssh/authorized_keys", "/Users/x/.ssh", "/Users/x/.SSH/authorized_keys", "/opt/app/.ssh",
	"/tmp/x/.ssh", "/var/tmp/.ssh/id_rsa", "/root/.ssh", "~/a/.ssh/b", "~bob/.ssh", "a/.ssh", "/boot/.ssh",
	"/home/u/.sshx", "/srv/git/ssh", ".SSH",
	// homes for --delete
	"/home/u", "/home/u/", "/home/u/.", "//home//u//", "/Users/u", "/Users/u/", "/root/", "/home/u/project",
	"/home/u/project/", "/tmp", "/tmp/x", "/var/tmp/x", "/var/tmp", "/Users/u/project", "/home", "/home/",
	// prefixes and near misses
	"/etc", "/etcx", "/etc/sudoers.d/x", "/etc/shadow-", "/etc/shadow-x", "/etc/gshadow-", "/etc/pam.d",
	"/etc/pam.dx", "/sys", "/proc/1/", "/dev/null", "/bin", "/sbinx", "/lib64/x", "/lib32", "/libx32",
	"/usr/lib", "/usr/lib64/x", "/usr/local/bin", "/usr/localx", "/usr", "/usr/share", "/var", "/var/www",
	"/varx", "/rootx", "/tmpx", "/var/tmpx",
	// literal strings the remote side may expand (REM-03)
	"/e?c/shadow", "/e[t]c/shadow", "$HOME", "${HOME}", "*", "./*", "$(echo ~)", "`pwd`", "/etc/*",
	// odd characters
	"/tmp/a b", "/tmp/\tx", "/tmp/é", "/ETC/shadow", "/etc/SHADOW",
}

// The tables give what the 2.x tables give, vector by vector. 3.0: REM-03
// and REM-10 fixed: the expandable strings are deny:metacharacters and
// "/." is deny:/ (TestLedger_REM03, TestLedger_REM10_fix); this 2.x
// comparison skips without the 2.x sources.
func TestClassifyMatches2x(t *testing.T) {
	var script strings.Builder
	for _, v := range pathVectors {
		script.WriteString("printf '%s|' \"$(__remote_normalize_path " + shq(v) + ")\" \"$(__remote_classify_path " + shq(v) + ")\"; ")
		script.WriteString("__remote_is_safe_for_delete " + shq(v) + " && echo safe || echo unsafe\n")
	}
	lines := strings.Split(strings.TrimSuffix(bash2x(t, script.String()), "\n"), "\n")
	require.Len(t, lines, len(pathVectors))
	for i, v := range pathVectors {
		safe := "unsafe"
		if safeForDelete(v) {
			safe = "safe"
		}
		require.Equal(t, lines[i], normalizePath(v)+"|"+classifyPath(v)+"|"+safe, "%q", v)
	}
}

// remote_spec.sh:11-66 and :142-190, INTERNAL vectors kept verbatim.
func TestSafetyTablesSpec(t *testing.T) {
	for _, p := range []string{"/etc/shadow", "/etc/passwd", "/etc/ssh/sshd_config", "/boot/grub", "/usr/bin/bash", "/"} {
		require.True(t, strings.HasPrefix(classifyPath(p), "deny:"), p)
	}
	require.Equal(t, "deny:path_traversal", classifyPath("/tmp/../etc/shadow"))
	for _, p := range []string{"/etc/systemd/system/myapp.service", "/etc/crontab", "/etc/init.d/apache2", "/root/.bashrc"} {
		require.True(t, strings.HasPrefix(classifyPath(p), "ask:"), p)
	}
	for _, p := range []string{"/tmp/foo", "/var/tmp/foo", "/home/maurizio/project", "relative/path", "/opt/myapp"} {
		require.Equal(t, "allow", classifyPath(p), p)
	}
	for _, p := range []string{"//etc/shadow", "/etc//shadow", "/./etc/shadow", "/etc/./ssh/", "/etc/shadow/", "///usr//bin/sh", "//"} {
		require.True(t, strings.HasPrefix(classifyPath(p), "deny:"), p)
	}
	for _, p := range []string{"~/.ssh/authorized_keys", ".ssh/id_ed25519", "~root/.ssh/x", "/home/bob/.ssh/", "./.ssh",
		"/srv/git/.ssh/authorized_keys", "/Users/x/.ssh", "/Users/x/.SSH/authorized_keys", "/opt/app/.ssh", "/tmp/x/.ssh",
		"/var/tmp/.ssh/id_rsa", "/root/.ssh", "~/a/.ssh/b", "~bob/.ssh", "a/.ssh"} {
		require.Equal(t, "ask:.ssh", classifyPath(p), p)
		require.False(t, safeForDelete(p), p)
	}
	require.Equal(t, "allow", classifyPath("~/notes.txt"))
	require.Equal(t, "allow", classifyPath("..config"))
	require.Equal(t, "deny:path_traversal", classifyPath("a/../../etc"))
	require.Equal(t, "deny:/boot", classifyPath("/boot/.ssh"))
	require.Equal(t, "allow", classifyPath("/home/u/.sshx"))
	require.Equal(t, "allow", classifyPath("/srv/git/ssh"))
	for _, p := range []string{"~", "~/", "~bob", "~bob/", "/home/u", "/home/u/", "/home/u/.", "//home//u//", "/Users/u", "/Users/u/", "/root", "/root/"} {
		require.False(t, safeForDelete(p), p)
	}
	for _, p := range []string{"~/project", "project", "/home/u/project", "/home/u/project/", "/tmp", "/tmp/x", "/var/tmp/x"} {
		require.True(t, safeForDelete(p), p)
	}
	require.Equal(t, "/root/x", normalizePath("~root/x"))
	require.Equal(t, "/home/bob/.bashrc", normalizePath("~bob/.bashrc"))
	require.Equal(t, "a/b", normalizePath("~/a//b/"))
	require.True(t, strings.HasPrefix(classifyPath("~root/x"), "ask:"))
}

// remote_spec.sh:69-104: raw targets interactively, aliases only
// headless.
func TestResolveTargetSpec(t *testing.T) {
	s := newScene(t, "HOST:hnas=root@hnas.example.com\n")
	r := s.same(plain, "remote_exec", "someone@example.com", "true")
	require.Contains(t, r.Log, "[someone@example.com]")
	r = s.same(headless, "remote_exec", "someone@example.com", "true")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Unknown host alias")
	require.Empty(t, r.Log)
	r = s.same(headless, "remote_exec", "hnas", "true")
	require.Contains(t, r.Log, "[root@hnas.example.com]")
}

// 3.0: REM-02 and REM-08 fixed: targets are strict user@host (no part
// starting with '-', no ':' outside a bracketed IPv6 literal), so the
// 2.x regex comparison below no longer applies (it skips without the 2.x
// sources).
func TestRawTarget(t *testing.T) {
	for v, ok := range map[string]bool{
		"deploy@box.example": true, "u@[::1]": true, "-oProxyCommand=x@h": false, "u@-h": false, "-u@h": false,
		"u@h:22": false, "u@h@x": false, "@h": false, "u@": false, "uh": false, "u @h": false, "u@h\t": false,
		"u@h\v": false, "u@h\r": false, "": false, "é@h": false,
	} {
		require.Equal(t, ok, validTarget(v), "%q", v)
	}
	// The 2.x regex, on the same vectors: it accepted the option-like and
	// host:port targets.
	vs := []string{"deploy@box.example", "-oProxyCommand=x@h", "u@h:22", "u@h@x", "@h", "u@", "u @h", "u@h\v", "é@h"}
	var script strings.Builder
	for _, v := range vs {
		script.WriteString(`[[ ` + shq(v) + ` =~ ^[^@[:space:]]+@[^@[:space:]]+$ ]] && echo y || echo n` + "\n")
	}
	got := strings.Fields(bash2x(t, script.String()))
	require.Equal(t, []string{"y", "y", "y", "n", "n", "n", "n", "n", "y"}, got)
}

func TestScriptArgs(t *testing.T) {
	require.Equal(t, "", scriptArgs(nil))
	require.Equal(t, ` 'a b' '~' '' '$(id)' 'it'\''s'`, scriptArgs([]string{"a b", "~", "", "$(id)", "it's"}))
}

// shq quotes s for Bash.
func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
