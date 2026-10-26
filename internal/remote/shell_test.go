// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// TestShTierIsClassifyPath: the sh tier of a resolved path is the Go one,
// macOS /private aliases and per-user temporary trees included.
func TestShTierIsClassifyPath(t *testing.T) {
	paths := []string{
		"/", "/etc", "/etc/nginx/nginx.conf", "/etc/ssh/sshd_config", "/etc/passwd", "/var/www/app/x.php",
		"/var/tmp/x", "/tmp/x", "/usr/local/bin/x", "/usr/bin/env", "/root/x", "/home/u/app/x", "/home/u/.ssh/config",
		"/home/u/.SSH/x", "/srv/app/config.php", "/boot", "/bootx", "/lib64/x", "/opt/x",
		"/private/etc/hosts", "/private/etc/ssh/sshd_config", "/private/var/www/x", "/private/tmp/x", "/private",
		"/privateetc/x", "/var/folders/ab/T/x", "/private/var/folders/ab/T/x", "/var/folders/ab/.ssh/x",
	}
	var script strings.Builder
	script.WriteString(ShTier())
	for _, p := range paths {
		script.WriteString("gash_tier " + Quote(p) + "\n")
	}
	out, err := exec.Command("sh", "-c", script.String()).Output()
	require.NoError(t, err)
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	require.Len(t, got, len(paths))
	for i, p := range paths {
		require.Equal(t, classifyResolved(p), got[i], p)
	}
}

// TestClassifyLocalUnaliases: on macOS /etc, /var and /tmp resolve into
// /private, which must not take a write out of its tier.
func TestClassifyLocalUnaliases(t *testing.T) {
	for p, want := range map[string]string{
		"/private/etc/hosts":             "ask:/etc",
		"/private/etc/ssh/sshd_config":   "deny:/etc/ssh",
		"/private/var/lib/x":             "ask:/var",
		"/private/tmp/x":                 "allow",
		"/private/var/folders/ab/T/x":    "allow",
		"/private/var/folders/ab/.ssh/x": "ask:.ssh",
		"/privateetc/x":                  "allow",
	} {
		require.Equal(t, want, ClassifyLocal(p), p)
	}
	require.Equal(t, "/etc/hosts", UnaliasResolved("/private/etc/hosts"))
	require.Equal(t, "/private", UnaliasResolved("/private"))
}

func TestShResolveAndFrames(t *testing.T) {
	// A physical directory: on macOS the temporary root and /home are
	// symlinks, which gash_resolve follows.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	out, err := exec.Command("sh", "-c", ShResolve()+`cd "$1" && HOME=$1/u gash_resolve '~/a/./b'; gash_resolve "$1/x/../y"`, "sh", dir).Output()
	require.NoError(t, err)
	want := dir + "/u/a/b\n"
	require.Equal(t, want, string(out[:min(len(want), len(out))]))
	require.True(t, strings.HasSuffix(string(out), "/y\n"), string(out))

	errOut := []byte("noise\nN err write_failed key some details here\nN done 1\n")
	code, key, details, ok := ErrFrame(errOut, "N")
	require.True(t, ok)
	require.Equal(t, []string{"write_failed", "key", "some details here"}, []string{code, key, details})
	rest, ok := Frame(errOut, "N", "done")
	require.True(t, ok)
	require.Equal(t, "1", rest)
	_, _, _, ok = ErrFrame([]byte("x\n"), "N")
	require.False(t, ok)
}

// TestShResolveWithoutGNU runs gash_resolve where there is no realpath -m
// and readlink has no -f (macOS, BusyBox): dangling links, link chains,
// a loop, and .. in the part that does not exist resolve as realpath -m
// resolves them.
func TestShResolveWithoutGNU(t *testing.T) {
	readlink, err := exec.LookPath("readlink")
	if err != nil {
		t.Skip("no readlink")
	}
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	stub := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(stub, "realpath"), []byte("#!/bin/sh\nexit 1\n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(stub, "readlink"), []byte("#!/bin/sh\n[ \"$1\" = -f ] && exit 1\nexec "+readlink+" \"$@\"\n"), 0o755))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "app"), 0o755))
	for link, target := range map[string]string{
		"app/k": "server.key", "l1": "l2", "l2": "app/x", "dl": "nodir/sub", "al": "/nonexistent-gash/y",
		"lp": "lp", "dlink": "app", "up": "app/../app/q",
	} {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, link)))
	}
	cases := []struct{ in, want string }{
		{"app/k", dir + "/app/server.key"},
		{"l1", dir + "/app/x"},
		{"dl/f", dir + "/nodir/sub/f"},
		{"al", "/nonexistent-gash/y"},
		{"dlink/new.txt", dir + "/app/new.txt"},
		{"nodir/../app/z", dir + "/app/z"},
		{"up", dir + "/app/q"},
		{"lp", dir + "/lp"},
	}
	var script strings.Builder
	script.WriteString(ShResolve() + "cd " + Quote(dir) + "\n")
	for _, c := range cases {
		script.WriteString("gash_resolve " + Quote(c.in) + "\n")
	}
	cmd := exec.Command("sh", "-c", script.String())
	cmd.Env = append(os.Environ(), "PATH="+stub+":"+os.Getenv("PATH"))
	out, err := cmd.Output()
	require.NoError(t, err)
	got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	require.Len(t, got, len(cases))
	gnu := exec.Command("realpath", "-m", "/").Run() == nil
	for i, c := range cases {
		require.Equal(t, c.want, got[i], c.in)
		if gnu && c.in != "lp" {
			real, err := exec.Command("realpath", "-m", "--", filepath.Join(dir, c.in)).Output()
			require.NoError(t, err)
			require.Equal(t, strings.TrimSpace(string(real)), got[i], "realpath -m %s", c.in)
		}
	}
}

// shells are the POSIX shells the host scripts are tested under: dash
// (Debian's sh, close to BusyBox ash) and bash in POSIX mode.
func shells(t *testing.T) [][]string {
	t.Helper()
	var out [][]string
	if p, err := exec.LookPath("dash"); err == nil {
		out = append(out, []string{p})
	}
	if p, err := exec.LookPath("bash"); err == nil {
		out = append(out, []string{p, "--posix"})
	}
	if len(out) == 0 {
		t.Skip("neither dash nor bash")
	}
	return out
}

// TestShRailsWithoutTr: gash_secret, gash_exception and the .ssh check of
// gash_tier lowercased the path with tr; without tr they compared an
// empty string and let every secret through. They run no program now:
// under dash and bash --posix with a PATH that holds nothing, they match
// as the Go rails do.
func TestShRailsWithoutTr(t *testing.T) {
	for _, f := range []string{ShSecret(), ShException(), ShTier()} {
		require.NotContains(t, f, "tr ", "no tr in the rails")
	}
	names := []string{
		".env", "app/.env", "app/.ENV.local", "x.env", ".env.example", ".Env.Example", "config/secrets.yml", "id_rsa", "ID_RSA.pub",
		"keys/server.PEM", "credentials.json", "CredentialsController.php", "home/u/.ssh/config", "home/u/.SSH/known_hosts",
		"a/.config/gcloud/x", "a/.Config/GCloud/x", "notes.txt", "login data", "Login Data", "LOGIN DATA", "terraform.tfstate.backup",
		"README.md", "x.tfvars.json", "ssh_host_rsa_key", "SSH_HOST_RSA_KEY.pub", "app.env.example", "dir/", "Kubeconfig", "NTUSER.DAT",
		"a b/.aws/credentials", "x/.Gnupg/y",
	}
	tiers := []string{"/home/u/.ssh/config", "/home/u/.SSH/x", "/srv/.Ssh/k", "/srv/app/x", "/etc/hosts", "/tmp/.ssh"}
	empty := t.TempDir()
	for _, sh := range shells(t) {
		var script strings.Builder
		script.WriteString(ShSecret() + ShException() + ShTier())
		script.WriteString(`for p in "$@"; do b=${p%/}; b=${b##*/}; s=0; e=0; gash_secret "$p" && s=1; gash_exception "$b" && e=1; echo "$s$e"; done` + "\n")
		for _, p := range tiers {
			script.WriteString("gash_tier " + Quote(p) + "\n")
		}
		cmd := exec.Command(sh[0], append(append(sh[1:], "-c", script.String(), "sh"), names...)...)
		cmd.Env = []string{"PATH=" + empty, "LC_ALL=C"}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "%v: %s", sh, out)
		got := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
		require.Len(t, got, len(names)+len(tiers), "%v: %s", sh, out)
		for i, n := range names {
			b := strings.TrimSuffix(n, "/")
			b = b[strings.LastIndexByte(b, '/')+1:]
			want := bit(pathguard.SecretName(n)) + bit(pathguard.SecretException(b))
			require.Equal(t, want, got[i], "%v %q", sh, n)
		}
		for i, p := range tiers {
			require.Equal(t, classifyResolved(p), got[len(names)+i], "%v %q", sh, p)
		}
	}
}

func bit(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// TestShResolveWithoutReadlink: without realpath -m and without readlink,
// gash_resolve gave a symlink's own path, so the rails judged a link named
// "settings" that points at .env as "settings" and the call read or wrote
// .env through it. It answers the "missing readlink" frame and status 1
// now, with nothing on stdout; paths without a link still resolve.
func TestShResolveWithoutReadlink(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(dir, "app"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("A=1\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "app", "f"), nil, 0o600))
	for link, target := range map[string]string{"settings": ".env", "dl": "nowhere/x", "dlink": "app"} {
		require.NoError(t, os.Symlink(target, filepath.Join(dir, link)))
	}
	empty := t.TempDir()
	for _, sh := range shells(t) {
		for _, c := range []struct{ in, out, errOut string }{
			{"app/f", "ok " + dir + "/app/f\n", ""},
			{"app/new/../g", "ok " + dir + "/app/g\n", ""},
			{"dlink/x", "ok " + dir + "/app/x\n", ""},
			{"settings", "status 1\n", "NONCE missing readlink\n"},
			{"dl", "status 1\n", "NONCE missing readlink\n"},
			{"dl/y", "status 1\n", "NONCE missing readlink\n"},
		} {
			script := "N=NONCE\n" + ShResolve() + "cd " + Quote(dir) + "\n" +
				`if r=$(gash_resolve "$1"); then echo "ok $r"; else echo "status $?${r:+ stdout $r}"; fi` + "\n"
			cmd := exec.Command(sh[0], append(append(sh[1:], "-c", script, "sh"), c.in)...)
			cmd.Env = []string{"PATH=" + empty, "LC_ALL=C"}
			var out, errOut strings.Builder
			cmd.Stdout, cmd.Stderr = &out, &errOut
			require.NoError(t, cmd.Run(), "%v %s", sh, c.in)
			require.Equal(t, c.out, out.String(), "%v %s", sh, c.in)
			require.Equal(t, c.errOut, errOut.String(), "%v %s", sh, c.in)
		}
	}
}
