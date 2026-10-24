// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// guardAt builds a guard rooted at dir on the real filesystem, with /root as
// root's home so the extra forbidden entry is not added (deterministic).
func guardAt(dir, home string) *Guard {
	return NewGuard(OSFS{}, dir, home, "/root")
}

func TestValidatePath_Traversal(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	cases := []string{"../../../etc/passwd", "..", "../x", "a/..", "/tmp/../etc", "a/../b"}
	for _, c := range cases {
		_, err := g.ValidatePath(c)
		r, ok := err.(*Refusal)
		require.Truef(t, ok, "%q: expected refusal, got %v", c, err)
		require.Equal(t, "path_traversal_blocked", r.Code, c)
		require.Equal(t, "FATAL", r.Action)
		require.Equal(t, "Path traversal (..) is not allowed for security reasons", r.Hint)
		require.Empty(t, r.Details)
	}
}

func TestValidatePath_NotTraversal(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	// Substrings that are not whole ".." components (core.sh comment).
	for _, c := range []string{"file..bak", "v1.2..3", "a..b"} {
		_, err := g.ValidatePath(c)
		require.NoErrorf(t, err, "%q should not be a traversal", c)
	}
}

// TestValidatePath_Forbidden covers the llm_spec vectors and the
// llm_exec_security_spec __llm_validate_path vector (whole trees, not
// look-alike names).
func TestValidatePath_Forbidden(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	forbidden := []string{
		"/proc", "/proc/self/environ", "/sys/kernel", "/dev/sda",
		"/root", "/root/.ssh", "/root/.bashrc",
		"/etc/shadow", "/etc/gshadow", "/etc/passwd", "/etc/sudoers", "/etc/sudoers.d/x",
	}
	for _, p := range forbidden {
		_, err := g.ValidatePath(p)
		r, ok := err.(*Refusal)
		require.Truef(t, ok, "%q: expected refusal, got %v", p, err)
		require.Equalf(t, "forbidden_path", r.Code, "%q", p)
		require.Equal(t, "FATAL", r.Action)
		require.Equal(t, "This path is protected and cannot be accessed", r.Hint)
		require.NotEmpty(t, r.Details) // the normalized path (resolved; e.g. /proc/self -> /proc/<pid>)
	}
	// Look-alike names are allowed. The returned path is realpath-normalized,
	// so comparing against that same normalization keeps the expectation
	// host-independent (macOS resolves /etc through the /private firmlink).
	for _, p := range []string{"/rootfs/x", "/devices", "/etc/passwdx", "/procfs"} {
		got, err := g.ValidatePath(p)
		require.NoErrorf(t, err, "%q should be allowed", p)
		want, err := g.Realpath(p, false)
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

// TestLedger_LLM10_2x: 2.1.0 left the shadow/gshadow backups and
// /etc/security/opasswd out of the forbidden list and did not match several
// secret-looking names. 3.0: LLM-10 fixed, they are refused; auth.json,
// which the fix recipe leaves out, is still not a secret.
func TestLedger_LLM10_2x(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	for _, p := range []string{"/etc/shadow-", "/etc/gshadow-", "/etc/security/opasswd"} {
		_, err := g.ValidatePath(p)
		var r *Refusal
		require.ErrorAsf(t, err, &r, "LLM-10: %q must be forbidden", p)
		require.Equal(t, "forbidden_path", r.Code)
	}
	for _, name := range []string{"ssh_host_rsa_key", ".envrc", "terraform.tfvars.json", "credentials_old"} {
		require.Truef(t, SecretName(name), "LLM-10: %q must be a secret", name)
	}
	require.False(t, SecretName("auth.json"))
}

// TestLedger_LLM10: the secret/forbidden matrix with the 3.0 additions, and
// the look-alikes that stay allowed.
func TestLedger_LLM10(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	for _, p := range []string{"/etc/shadow-", "/etc/gshadow-", "/etc/security/opasswd", "/etc/shadow", "/etc/sudoers.d/x"} {
		_, err := g.ValidatePath(p)
		var r *Refusal
		require.ErrorAsf(t, err, &r, "%q must be forbidden", p)
		require.Equal(t, "forbidden_path", r.Code)
		require.Equal(t, "FATAL", r.Action)
	}
	for _, p := range []string{"/etc/shadow--x", "/etc/security", "/etc/security/limits.conf"} {
		_, err := g.ValidatePath(p)
		require.NoErrorf(t, err, "%q must stay allowed", p)
	}
	for _, name := range []string{
		"ssh_host_rsa_key", "SSH_HOST_ED25519_KEY", "/etc/ssh/ssh_host_ecdsa_key", ".envrc", "x/.EnvRC",
		"terraform.tfvars.json", "prod.TFVARS.JSON", "credentials", "credentials.json", "credentials_old", "Credentials-prod", "Credentials.bak",
	} {
		require.Truef(t, SecretName(name), "%q must be a secret", name)
	}
	for _, name := range []string{"ssh_host_rsa_key.pub", "ssh_host_key", "envrc", "tfvars.json.txt", "my_credentials", "CredentialsController.php", "auth.json"} {
		require.Falsef(t, SecretName(name), "%q must not be a secret", name)
	}
}

func TestValidatePath_Tilde(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	// ~ expands to Home, then realpath-normalizes; compare against that same
	// normalization so the macOS /System/Volumes/Data firmlink on /home
	// does not make the expectation Linux-specific.
	got, err := g.ValidatePath("~")
	require.NoError(t, err)
	want, err := g.Realpath("/home/u", false)
	require.NoError(t, err)
	require.Equal(t, want, got)
	got, err = g.ValidatePath("~/x")
	require.NoError(t, err)
	want, err = g.Realpath("/home/u/x", false)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

// TestLedger_LLM25_2x pins ~user as textual, not real user expansion:
// "~root/x" becomes "$HOME" + "root/x" (LEDGER LLM-25, kept in rc.A).
func TestLedger_LLM25_2x(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	got, err := g.ValidatePath("~root/x")
	require.NoError(t, err)
	// ~root is textual: the leading ~ is dropped and Home prepended
	// ("/home/u" + "root/x" = "/home/uroot/x"), not expanded to root's real
	// home. The result is then realpath-normalized exactly as Realpath does;
	// comparing against that same normalization keeps the expectation
	// host-independent (macOS resolves /home through the /System/Volumes/Data
	// firmlink, which applies equally to both sides).
	want, err := g.Realpath("/home/uroot/x", false)
	require.NoError(t, err)
	require.Equal(t, want, got)
}

func TestValidatePath_Empty(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	got, err := g.ValidatePath("")
	require.NoError(t, err)
	require.Equal(t, ".", got)
}

func TestSecretName(t *testing.T) {
	// From the llm_exec_security and llm_spec vectors.
	secret := []string{
		".env", ".ENV", ".env.local", "prod.env", "id_rsa", "id_ed25519",
		"deploy_rsa", "server.PEM", ".pgpass", ".netrc", ".git-credentials",
		".npmrc", "kubeconfig", "terraform.tfstate", "secrets.yml",
		"/x/.ssh/config", "/x/.aws/config", "/x/.config/gcloud/creds.db",
	}
	for _, f := range secret {
		require.Truef(t, SecretName(f), "expected secret: %q", f)
	}
	notSecret := []string{
		".env.example", "id_rsa.pub", "README.md", "config.json",
		"package.json", "CredentialsController.php", "env.php",
	}
	for _, f := range notSecret {
		require.Falsef(t, SecretName(f), "false positive: %q", f)
	}
}

// TestIsSecretFile_SymlinkTarget covers the symlink-target case: a non-secret
// name pointing at a secret file is itself secret (llm_exec_security_spec).
func TestIsSecretFile_SymlinkTarget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("S=1\n"), 0o600))
	link := filepath.Join(dir, "settings.json")
	require.NoError(t, os.Symlink(filepath.Join(dir, ".env"), link))
	g := guardAt(dir, dir)
	require.False(t, SecretName(link), "settings.json is not secret by name")
	require.True(t, g.IsSecretFile(link), "settings.json resolves to .env, a secret")
}

// TestLedger_portability_realpath replicates llm_portability_spec.sh:48:
// Realpath equals realpath -m and realpath -m -s on a fixture tree with
// relative, absolute and looping symlinks. Unlike the 2.x walk the spec
// tests (a realpath without -m), a loop resolves as GNU -m resolves it.
func TestLedger_portability_realpath(t *testing.T) {
	rp, err := exec.LookPath("realpath")
	if err != nil {
		t.Skip("realpath not available")
	}
	// The oracle is GNU `realpath -m`; BSD realpath (stock macOS, no
	// coreutils) has neither -m nor -s, so there is nothing to compare
	// against here.
	if err := exec.Command(rp, "-m", "--", ".").Run(); err != nil {
		t.Skip("realpath has no GNU -m (BSD); oracle unavailable")
	}
	dir := t.TempDir()
	mk := func(p string) { require.NoError(t, os.MkdirAll(filepath.Join(dir, p), 0o755)) }
	mk("a/b")
	sym := func(target, name string) { require.NoError(t, os.Symlink(target, filepath.Join(dir, name))) }
	sym("a/b", "l")
	sym("../..", "a/b/up")
	sym("loop2", "loop1")
	sym("loop1", "loop2")
	sym("self", "self")
	sym("../"+filepath.Base(dir)+"/back", "back")
	sym("/root", "rootlink")
	sym("c1", "c0")
	for i := 1; i <= 60; i++ {
		sym("c"+strconv.Itoa(i+1), "c"+strconv.Itoa(i))
	}
	mk("c61")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("S=1\n"), 0o600))
	sym(".env", "settings.json")

	g := guardAt(dir, dir)
	cases := []string{"l", "l/../x", "a/b/up/x", "/etc//x/./y/../z", ".", "a/./b/", "/", "a/new line", "missing/x",
		"loop1", "loop1/x", "loop2/x/../y", "loop1/../l/x", "self", "self/self/x", "back", "back/x", "c0/x", "l/up/loop1/z"}
	for _, c := range cases {
		wantM := realpathRun(t, rp, dir, "-m", c)
		gotM, err := g.Realpath(absIn(dir, c), false)
		require.NoErrorf(t, err, "Realpath(%q)", c)
		require.Equalf(t, wantM, gotM, "realpath -m %q", c)

		wantS := realpathRun(t, rp, dir, "-m-s", c)
		gotS, err := g.Realpath(absIn(dir, c), true)
		require.NoErrorf(t, err, "Realpath -s (%q)", c)
		require.Equalf(t, wantS, gotS, "realpath -m -s %q", c)
	}
	// A link to itself/.. expands without end (GNU realpath -m never
	// returns): an error, so callers use the path as given.
	sym("grow/..", "grow")
	_, err = g.Realpath(filepath.Join(dir, "grow", "x"), false)
	require.ErrorIs(t, err, errSymlinkLoop)
	normalized, err := g.ValidatePath(filepath.Join(dir, "grow", "x"))
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "grow", "x"), normalized)
	// rootlink/x resolves to /root/x and is forbidden.
	_, err = g.ValidatePath(filepath.Join(dir, "rootlink", "x"))
	r, ok := err.(*Refusal)
	require.True(t, ok)
	require.Equal(t, "forbidden_path", r.Code)
	require.True(t, g.IsSecretFile(filepath.Join(dir, "settings.json")))
}

// absIn turns a relative case into one rooted at dir, matching how the spec
// evaluated the cases from inside the fixture directory.
func absIn(dir, c string) string {
	if filepath.IsAbs(c) {
		return c
	}
	return c // Guard.Cwd == dir makes relative paths resolve there
}

func realpathRun(t *testing.T, rp, dir, mode, arg string) string {
	t.Helper()
	var args []string
	switch mode {
	case "-m":
		args = []string{"-m", "--", arg}
	case "-m-s":
		args = []string{"-m", "-s", "--", arg}
	}
	cmd := exec.Command(rp, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	require.NoError(t, err)
	return string(out[:len(out)-1]) // trim trailing newline
}

func TestPathAboveForbidden(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	// Content mode: / and ancestors of any forbidden entry (incl. /etc).
	require.True(t, g.PathAboveForbidden("/", "content"))
	require.True(t, g.PathAboveForbidden("/etc", "content"))
	require.True(t, g.PathAboveForbidden("/", "names"))
	// Names mode: /etc is not above a non-/etc forbidden tree.
	require.False(t, g.PathAboveForbidden("/etc", "names"))
	// A narrow directory is not above anything.
	require.False(t, g.PathAboveForbidden("/tmp/project", "content"))
	require.False(t, g.PathAboveForbidden("/home/u", "names"))
}

func TestIcaseGlob(t *testing.T) {
	require.Equal(t, ".[eE][nN][vV]", IcaseGlob(".env"))
	require.Equal(t, "*.[pP][eE][mM]", IcaseGlob("*.pem"))
}

func TestRootHomeAppended(t *testing.T) {
	g := NewGuard(OSFS{}, "/tmp", "/home/u", "/var/root")
	_, err := g.ValidatePath("/var/root/x")
	r, ok := err.(*Refusal)
	require.True(t, ok)
	require.Equal(t, "forbidden_path", r.Code)
	// With rootHome == /root the extra entry is not added.
	g2 := NewGuard(OSFS{}, "/tmp", "/home/u", "/root")
	require.Len(t, g2.Forbidden, len(ForbiddenBasePaths))
	// core.sh appends only a home matching /?* : not "/", "" or a relative one.
	for _, h := range []string{"/", "", "root"} {
		require.Len(t, NewGuard(OSFS{}, "/tmp", "/home/u", h).Forbidden, len(ForbiddenBasePaths), h)
	}
}

func TestGlobPosixClasses(t *testing.T) {
	for _, c := range []struct {
		pat, name string
		want      bool
	}{
		{"[[:upper:]]*", "Abc", true},
		{"[[:upper:]]*", "abc", false},
		{"[![:lower:]]*", "Abc", true},
		{"[![:lower:]]*", "abc", false},
		{"[[:digit:]x]", "x", true},
		{"[[:digit:]x]", "7", true},
		{"[[:space:]]", "\t", true},
		{"[[:alpha:]]", "_", false},
		{"[[:punct:]]", "_", true},
		{"[[:bogus:]]", ":", false},
		{"[![:bogus:]]", "x", true},
		{"[x[:bogus:]]", "x", true},
	} {
		require.Equal(t, c.want, MatchGlobSegment(c.pat, c.name), "%s %s", c.pat, c.name)
	}
}

// TestUnderForbidden: the lexical per-entry test of a walk (LEDGER LLM-09).
func TestUnderForbidden(t *testing.T) {
	g := NewGuard(OSFS{}, "/", "/home/u", "/var/root")
	for _, p := range []string{"/proc", "/proc/1/environ", "/root/", "/etc/shadow", "/etc/shadow-", "/etc/sudoers.d/x",
		"/private/var/root/x", "/var/root", "proc/self"} {
		require.Truef(t, g.UnderForbidden(p), "%q is under a forbidden entry", p)
	}
	for _, p := range []string{"", "/", "/etc", "/etc/passwdx", "/rootfs", "/home/u/proc", "/var", "/private/var"} {
		require.Falsef(t, g.UnderForbidden(p), "%q is not under a forbidden entry", p)
	}
}
