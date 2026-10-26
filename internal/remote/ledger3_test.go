// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

// The proving tests of the 3.0 REM fixes. They need no 2.x sources: the
// handler runs against the logging ssh, scp and rsync stubs of the scene.

// newGoScene is newScene without the check for the 2.x sources (the
// stubs need bash).
func newGoScene(t *testing.T, gashEnv string) *scene {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	return &scene{t: t, root: absRoot, base: t.TempDir(), gash: gashEnv, stubs: []string{"ssh", "scp", "rsync"},
		files: map[string]string{}}
}

func tty(input string) func(*term.Term) *term.Term {
	return func(t *term.Term) *term.Term { return t.WithTTY(input) }
}

// envelope is the code and the details of the one error envelope a
// headless run printed: since 3.4.2 the remote_* errors are envelopes in
// headless mode, the Error: line for a person.
func envelope(t *testing.T, stderr string) (code, details string) {
	t.Helper()
	require.Equal(t, 1, strings.Count(stderr, "\n"), "one line: %q", stderr)
	var e struct {
		Error, Details, Action, Hint string
	}
	require.NoError(t, json.Unmarshal([]byte(stderr), &e), stderr)
	require.NotEmpty(t, e.Action)
	require.NotEmpty(t, e.Hint)
	return e.Error, e.Details
}

// 3.0: REM-01 fixed. An unknown option or a fourth positional is refused
// before the host is resolved; --exclude PAT is two words.
func TestLedger_REM01(t *testing.T) {
	s := newGoScene(t, boxEnv)
	r := s.runGo(headless, "remote_sync_push", nil, "box", "./src", "/tmp/build", "--delete", "-n")
	require.Equal(t, 1, r.RC)
	code, details := envelope(t, r.Err)
	require.Equal(t, "unknown_option", code)
	require.Equal(t, "Unknown option: -n (usage: "+syncPushSpec.usage+")", details)
	require.Empty(t, r.Log)
	r = s.runGo(plain, "remote_sync_push", nil, "box", "./src", "/tmp/build", "--delete", "-n")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: Unknown option: -n (usage: "+syncPushSpec.usage+")\n", r.Err)

	r = s.runGo(headless, "remote_sync_push", nil, "box", "./src", "/tmp/build", "--delete", "--exclude", ".env")
	require.Equal(t, 0, r.RC, r.Err)
	a := argv(r.Log, "rsync")[0]
	require.Contains(t, a, "--exclude=.env")
	require.Contains(t, a, "--delete")
	require.Equal(t, []string{"--", "./src", "deploy@box.example:/tmp/build"}, a[len(a)-3:])

	for _, typo := range []string{"--delte", "--forse", "--dryrun"} {
		r = s.runGo(headless, "remote_sync_pull", nil, "box", "/srv/d", "./d", "--delete", typo)
		require.Equal(t, 1, r.RC, typo)
		require.Contains(t, r.Err, "Unknown option: "+typo, typo)
		require.Empty(t, r.Log, typo)
	}

	r = s.runGo(headless, "remote_push", nil, "box", "./x", "/tmp/x", "--dry-run")
	require.Equal(t, 1, r.RC)
	code, details = envelope(t, r.Err)
	require.Equal(t, "unknown_option", code)
	require.Equal(t, "Unknown option: --dry-run (usage: "+pushSpec.usage+")", details)
	require.Empty(t, r.Log, "scp is not called")

	r = s.runGo(headless, "remote_pull", nil, "box", "/tmp/x", "./x", "-r", "extra")
	require.Equal(t, 1, r.RC)
	code, details = envelope(t, r.Err)
	require.Equal(t, "unexpected_argument", code)
	require.Equal(t, "Unexpected argument: extra (usage: "+pullSpec.usage+")", details)
	r = s.runGo(plain, "remote_pull", nil, "box", "/tmp/x", "./x", "-r", "extra")
	require.Equal(t, "Error: Unexpected argument: extra (usage: "+pullSpec.usage+")\n", r.Err)

	r = s.runGo(headless, "remote_sync_push", nil, "box", "./s", "/tmp/d", "--exclude")
	require.Equal(t, 1, r.RC)
	code, details = envelope(t, r.Err)
	require.Equal(t, "missing_value", code)
	require.Contains(t, details, "--exclude needs a pattern")

	// Before the host is resolved.
	r = s.runGo(headless, "remote_push", nil, "nohost", "./x", "/tmp/x", "-z")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Unknown option: -z")
	require.NotContains(t, r.Err, "Unknown host alias")

	// The scp flags still work, short and long, in any order.
	r = s.runGo(headless, "remote_push", nil, "box", "d", "/tmp/d", "--preserve", "-r", "--no-progress")
	require.Equal(t, 0, r.RC, r.Err)
	require.Equal(t, [][]string{{"-o", "ConnectTimeout=10", "-o", "BatchMode=yes", "-r", "-p", "-q", "--", "d", "deploy@box.example:/tmp/d"}}, argv(r.Log, "scp"))
}

// 3.0: REM-02 fixed. A target is strict user@host, and ssh gets -- right
// before it, in the 255 probe too.
func TestLedger_REM02(t *testing.T) {
	for v, ok := range map[string]bool{
		"-oProxyCommand=x@h": false, "u@-h": false, "-u@h": false, "u@h:22": false,
		"deploy@box.example": true, "u@[::1]": true, "u_1.x@10.0.0.1": true, "u@[1.2.3.4]": false, "u@[]": false,
	} {
		// remote_* and the HOST: load share one rule (F-07).
		require.Equal(t, ok, config.ValidHostTarget(v), "%q", v)
	}

	s := newGoScene(t, boxEnv)
	s.files["ssh.status"] = "255 255"
	r := s.runGo(plain, "remote_exec", nil, "box", "true")
	require.Equal(t, 1, r.RC)
	calls := argv(r.Log, "ssh")
	require.Len(t, calls, 2)
	require.Equal(t, []string{"--", "deploy@box.example", "unset HISTFILE HISTSIZE; true"}, calls[0][len(calls[0])-3:])
	require.Equal(t, []string{"--", "deploy@box.example", "exit 0"}, calls[1][len(calls[1])-3:])

	// An option-like HOST: entry is skipped: not listed, not used.
	s = newGoScene(t, "HOST:opt=-oProxyCommand=x@h\n")
	r = s.runGo(plain, "remote_hosts", nil)
	require.NotContains(t, r.Out, "ProxyCommand")
	require.Contains(t, r.Err, "No remote hosts configured")
	r = s.runGo(headless, "remote_exec", nil, "opt", "true")
	require.Equal(t, 1, r.RC)
	code, details := envelope(t, r.Err)
	require.Equal(t, "unknown_host", code)
	require.Contains(t, details, "Unknown host alias 'opt'.")
	require.Contains(t, details, "Skipped: the HOST target of 'opt' must be user@host.")
	require.Empty(t, r.Log)
	r = s.runGo(plain, "remote_exec", nil, "opt", "true")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Warning: HOST target of 'opt' must be user@host (skipping)\n")
	require.Contains(t, r.Err, "Error: Unknown host alias 'opt' and not a valid user@host target.")
	require.Empty(t, r.Log)

	// Interactively, an option-like raw target is refused.
	r = newGoScene(t, "").runGo(plain, "remote_exec", nil, "-oX=y@h", "true")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Error: Unknown host alias '-oX=y@h' and not a valid user@host target.\n", r.Err)
	require.Empty(t, r.Log)
}

// 3.0: REM-08 fixed. A HOST target u@h:22 is skipped with the hint to put
// the port in ~/.ssh/config; scp never sees it.
func TestLedger_REM08(t *testing.T) {
	require.False(t, config.ValidHostTarget("u@h:22"))
	s := newGoScene(t, "HOST:x=u@h:22\n")
	r := s.runGo(headless, "remote_push", nil, "x", "./f", "/tmp/f")
	require.Equal(t, 1, r.RC)
	code, details := envelope(t, r.Err)
	require.Equal(t, "unknown_host", code)
	require.Contains(t, details, "Skipped: the HOST target of 'x' must be user@host (put the port in a Host entry of ~/.ssh/config).")
	require.Empty(t, r.Log)
	r = s.runGo(plain, "remote_push", nil, "x", "./f", "/tmp/f")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Warning: HOST target of 'x' must be user@host (skipping)\nHint: put the port in a Host entry of ~/.ssh/config\n")
	require.Empty(t, r.Log)
}

// 3.0: REM-03 fixed. A remote path the remote side may expand is
// deny:metacharacters, even with --force; rsync always gets -s.
func TestLedger_REM03(t *testing.T) {
	for _, p := range []string{"/e?c/shadow", "/e[t]c/shadow", "$HOME", "${HOME}", "*", "./*", "$(echo ~)", "`pwd`",
		"/tmp/{a,b}", "/tmp/a;b", "/tmp/a&b", "/tmp/a|b", "/tmp/<a", "/tmp/a>", "/tmp/!a", "/tmp/'a'", `/tmp/"a"`,
		`/tmp/a\b`, "/tmp/a\nb", "/tmp/~x", "~/a~b", "~~"} {
		require.Equal(t, "deny:metacharacters", classifyPath(p), "%q", p)
		require.False(t, safeForDelete(p), "%q", p)
	}
	for _, p := range []string{"~", "~/x", "~bob", "~bob/x", "/tmp/a b", "/tmp/a-b_c.d", "/tmp/a#b"} {
		require.NotEqual(t, "deny:metacharacters", classifyPath(p), "%q", p)
	}

	s := newGoScene(t, boxEnv)
	for _, extra := range [][]string{nil, {"--force"}} {
		args := append([]string{"box", "./src", "$HOME", "--delete"}, extra...)
		r := s.runGo(headless, "remote_sync_push", nil, args...)
		require.Equal(t, 1, r.RC, extra)
		code, details := envelope(t, r.Err)
		require.Equal(t, "invalid_path", code)
		require.Equal(t, "Remote path '$HOME' contains characters the remote side may expand. Refusing sync-push.", details)
		require.Empty(t, r.Log, "rsync is not called")
	}
	r := s.runGo(headless, "remote_pull", nil, "box", "/etc/*", "./x", "--force")
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Refusing pull (source path access).")

	r = s.runGo(headless, "remote_sync_push", nil, "box", "./src", "/tmp/build")
	require.Equal(t, 0, r.RC, r.Err)
	require.Equal(t, []string{"-az", "-s", "-e"}, argv(r.Log, "rsync")[0][:3])
}

// rsyncSplit splits an rsync -e string as rsync does (main.c do_cmd):
// words separated by spaces, single or double quotes keep spaces, a
// doubled quote inside quotes is a literal quote, a backslash is not
// special. ok is false for an unterminated quote.
func rsyncSplit(cmd string) (words []string, ok bool) {
	f := 0
	for f < len(cmd) {
		if cmd[f] == ' ' {
			f++
			continue
		}
		var w strings.Builder
		var inQuote byte
		for f < len(cmd) && (cmd[f] != ' ' || inQuote != 0) {
			c := cmd[f]
			if c == '\'' || c == '"' {
				if inQuote == 0 {
					inQuote = c
					f++
					continue
				}
				if c == inQuote {
					if f+1 < len(cmd) && cmd[f+1] == inQuote {
						w.WriteByte(c)
						f += 2
						continue
					}
					inQuote = 0
					f++
					continue
				}
			}
			w.WriteByte(c)
			f++
		}
		if inQuote != 0 {
			return nil, false
		}
		words = append(words, w.String())
	}
	return words, true
}

// 3.0: REM-05 fixed. The ControlPath option is one rsync word for a HOME
// with spaces and quotes.
func TestLedger_REM05(t *testing.T) {
	for _, home := range []string{"/home/u", "/home/John Doe", "/home/it's me", `/home/a "b" c`, "/home/''"} {
		words, ok := rsyncSplit(rsyncRSH(home))
		require.True(t, ok, home)
		require.Equal(t, "ssh", words[0])
		var paths []string
		for i, w := range words {
			if strings.HasPrefix(w, "ControlPath=") {
				paths = append(paths, w)
				require.Equal(t, "-o", words[i-1])
			}
		}
		require.Equal(t, []string{"ControlPath=" + home + "/.ssh/cm-%C"}, paths, home)
		require.Len(t, words, 15, home)
	}

	s := newGoScene(t, boxEnv)
	s.homeName = "John Doe's"
	r := s.runGo(headless, "remote_sync_push", nil, "box", "./s", "/tmp/d")
	require.Equal(t, 0, r.RC, r.Err)
	words, ok := rsyncSplit(argv(r.Log, "rsync")[0][3])
	require.True(t, ok)
	require.Contains(t, words, "ControlPath="+s.home()+"/.ssh/cm-%C")
}

// 3.0: REM-06 fixed. remote_sync_pull --delete checks the local
// destination in local words.
func TestLedger_REM06(t *testing.T) {
	s := newGoScene(t, boxEnv)
	r := s.runGo(headless, "remote_sync_pull", nil, "box", "/srv", "./", "--delete")
	require.Equal(t, 1, r.RC)
	code, details := envelope(t, r.Err)
	require.Equal(t, "sensitive_path", code)
	require.Equal(t, "rsync --delete targeting the local destination './' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the current directory; never your home itself or .ssh). Refused in headless mode without --force.", details)
	require.Empty(t, r.Log)

	r = s.runGo(plain, "remote_sync_pull", tty("n\n"), "box", "/srv", "./", "--delete")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "rsync --delete targeting the local destination './' can remove local files. Proceed? [y/N] Info: Operation cancelled.\n", r.Err)

	// The push keeps the remote words.
	r = s.runGo(headless, "remote_sync_push", nil, "box", "./s", "~", "--delete")
	code, details = envelope(t, r.Err)
	require.Equal(t, "sensitive_path", code)
	require.Contains(t, details, "rsync --delete targeting '~' is outside the safe set (below /tmp, /var/tmp, /home/<user>/ or the remote home;")
}

// 3.0: REM-07 fixed. A missing ~/.ssh is created with mode 0700; an
// existing one keeps its mode.
func TestLedger_REM07(t *testing.T) {
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	s := newGoScene(t, boxEnv)
	r := s.runGo(plain, "remote_exec", nil, "box", "true")
	require.Equal(t, 0, r.RC, r.Err)
	require.Contains(t, r.Tree, "home/.ssh drwx------")

	s.prep = func(home string) { require.NoError(t, os.Mkdir(filepath.Join(home, ".ssh"), 0o755)) }
	r = s.runGo(plain, "remote_exec", nil, "box", "true")
	require.Equal(t, 0, r.RC, r.Err)
	require.Contains(t, r.Tree, "home/.ssh drwxr-xr-x")
}

// 3.0: REM-10 fixed. The root spelled "/.", "//." or "/./" is the root.
func TestLedger_REM10_fix(t *testing.T) {
	for _, p := range []string{"/.", "//.", "/./", "/./.", "///./"} {
		require.Equal(t, "/", normalizePath(p), "%q", p)
		require.Equal(t, "deny:/", classifyPath(p), "%q", p)
	}
	require.Equal(t, ".", normalizePath("."))
	require.Equal(t, "x", normalizePath("x/."))
	require.Equal(t, "allow", classifyPath("./"))
}

// LEDGER REM-10 (2.x): the 2.x normalization made "/." and "//." the
// relative ".", which classifies as allow.
func TestLedger_REM10_2x(t *testing.T) {
	got := bash2x(t, `for p in /. //.; do printf '%s|%s\n' "$(__remote_normalize_path "$p")" "$(__remote_classify_path "$p")"; done`)
	require.Equal(t, ".|allow\n.|allow\n", got)
}

// 3.0: REM-09 fixed. The security.md deny row is the Go table, and the
// registered help (the source of docs/remote/reference.md) states the
// flag rules, GASH_ENV_FILE, the 255 probe and the ~/.ssh creation.
func TestLedger_REM09(t *testing.T) {
	sec, err := os.ReadFile(filepath.Join(absRoot, "docs/security.md"))
	require.NoError(t, err)
	var row []string
	for _, p := range denyPrefixes {
		row = append(row, "`"+p+"`")
	}
	require.Contains(t, string(sec), "| Deny | `/` itself; "+strings.Join(row, ", ")+" and everything below each |")
	require.NotContains(t, string(sec), "/lib*")
	require.Contains(t, string(sec), "(`deny:metacharacters`)")

	for _, sp := range []transfer{pushSpec, pullSpec, syncPushSpec, syncPullSpec} {
		f, ok := registry.Lookup(sp.name)
		require.True(t, ok, sp.name)
		require.Equal(t, sp.usage, f.Fallback.Usage, sp.name)
		require.True(t, strings.HasPrefix(f.Help.Body, "USAGE\n  "+sp.usage+"\n"), sp.name)
		require.Contains(t, f.Help.Body, "An unknown option or a fourth", sp.name)
		require.Contains(t, f.Help.Body, "GASH_ENV_FILE", sp.name)
	}
	require.Contains(t, syncPushSpec.usage, "--exclude PAT|--exclude=PAT")
	hosts, _ := registry.Lookup("remote_hosts")
	require.Contains(t, hosts.Help.Body, "GASH_ENV_FILE")
	rex, _ := registry.Lookup("remote_exec")
	require.Contains(t, rex.Help.Body, "a probe")
	require.Contains(t, rex.Help.Body, "the status is 255")
	require.Contains(t, rex.Help.Body, "~/.ssh")
	require.Contains(t, rex.Help.Body, "mode 0700")
	ref, err := os.ReadFile(filepath.Join(absRoot, "docs/remote/reference.md"))
	require.NoError(t, err)
	require.Contains(t, string(ref), syncPullSpec.usage)
}

// TestFollowup_F08: with openrsync (macOS /usr/bin/rsync), which has no -s,
// no --info and splits -e on blanks without quotes, the rsync functions
// give it an argv it accepts that still keeps the remote path one word of
// the remote shell; Samba's rsync keeps -s (3.0: F-08 fixed).
func TestFollowup_F08(t *testing.T) {
	s := newGoScene(t, boxEnv)
	s.files["rsync.version"] = "openrsync: protocol version 29\n"
	r := s.runGo(headless, "remote_sync_push", nil, "box", "./s", "/tmp/my dir")
	require.Equal(t, 0, r.RC, r.Err)
	calls := argv(r.Log, "rsync")
	require.Len(t, calls, 1)
	a := calls[0]
	require.Equal(t, []string{"-az", "-e"}, a[:2])
	require.NotContains(t, a, "-s")
	require.Equal(t, []string{"--progress", "--", "./s", "deploy@box.example:'/tmp/my dir'"}, a[3:])
	// openrsync's split of -e: blanks only; no word holds a quote.
	words := strings.Fields(a[2])
	require.Contains(t, words, "ControlPath="+s.home()+"/.ssh/cm-%C")
	require.Equal(t, "ssh", words[0])
	require.NotContains(t, a[2], "'")

	// A HOME with a blank: the ControlPath names ssh's own ~.
	s.homeName = "John Doe"
	r = s.runGo(headless, "remote_sync_pull", nil, "box", "~/my dir/", "./d", "--no-progress", "--exclude", "a b")
	require.Equal(t, 0, r.RC, r.Err)
	a = argv(r.Log, "rsync")[0]
	require.Contains(t, strings.Fields(a[2]), "ControlPath=~/.ssh/cm-%C")
	require.NotContains(t, a, "--progress")
	require.Equal(t, []string{"--exclude=a b", "--", "deploy@box.example:~/'my dir/'", "./d"}, a[3:])

	// The remote shell reads each quoted path as one word, ~ expanded.
	for p, want := range map[string]string{
		"/tmp/my dir": "/tmp/my dir", "~/my dir/": "/r/my dir/", "~": "/r", "~/": "/r/", "rel\tx": "rel\tx",
	} {
		cmd := exec.Command("sh", "-c", `printf '[%s]\n' `+remoteShellWord(p))
		cmd.Env = []string{"HOME=/r", "PATH=" + os.Getenv("PATH")}
		out, err := cmd.Output()
		require.NoError(t, err, p)
		require.Equal(t, "["+want+"]\n", string(out), p)
	}

	// Samba's rsync: -s and --info=progress2, the path as it is.
	s = newGoScene(t, boxEnv)
	s.files["rsync.version"] = "rsync  version 3.2.7  protocol version 31\n"
	r = s.runGo(headless, "remote_sync_push", nil, "box", "./s", "/tmp/my dir")
	require.Equal(t, 0, r.RC, r.Err)
	a = argv(r.Log, "rsync")[0]
	require.Equal(t, []string{"-az", "-s", "-e"}, a[:3])
	require.Equal(t, []string{"--info=progress2", "--", "./s", "deploy@box.example:/tmp/my dir"}, a[4:])
}
