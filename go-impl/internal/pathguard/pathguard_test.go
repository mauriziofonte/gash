// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"os/exec"
	"path/filepath"
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
	// Look-alike names are allowed and returned unchanged.
	for _, p := range []string{"/rootfs/x", "/devices", "/etc/passwdx", "/procfs"} {
		got, err := g.ValidatePath(p)
		require.NoErrorf(t, err, "%q should be allowed", p)
		require.Equal(t, p, got)
	}
}

// TestLedger_LLM10_2x pins the forbidden-list/secret gaps rc.A reproduces: the
// shadow/gshadow backups, /etc/security/opasswd and several secret-looking
// names are NOT refused, exactly as 2.1.0 (LEDGER LLM-10).
func TestLedger_LLM10_2x(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	// Not in the forbidden list, so ValidatePath returns them.
	for _, p := range []string{"/etc/shadow-", "/etc/gshadow-", "/etc/security/opasswd"} {
		got, err := g.ValidatePath(p)
		require.NoErrorf(t, err, "LLM-10: %q must not be forbidden in rc.A", p)
		require.Equal(t, p, got)
	}
	// Not matched as secrets in rc.A.
	for _, name := range []string{"ssh_host_rsa_key", ".envrc", "terraform.tfvars.json", "auth.json", "credentials_old"} {
		require.Falsef(t, SecretName(name), "LLM-10: %q must not be a secret in rc.A", name)
	}
}

func TestValidatePath_Tilde(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	got, err := g.ValidatePath("~")
	require.NoError(t, err)
	require.Equal(t, "/home/u", got)
	got, err = g.ValidatePath("~/x")
	require.NoError(t, err)
	require.Equal(t, "/home/u/x", got)
}

// TestLedger_LLM25_2x pins ~user as textual, not real user expansion:
// "~root/x" becomes "$HOME" + "root/x" (LEDGER LLM-25, kept in rc.A).
func TestLedger_LLM25_2x(t *testing.T) {
	g := guardAt("/tmp", "/home/u")
	got, err := g.ValidatePath("~root/x")
	require.NoError(t, err)
	require.Equal(t, "/home/uroot/x", got)
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

// TestLedger_portability_realpath replicates llm_portability_spec.sh:48 — the
// walk (what rc.A always uses) equals realpath -m and realpath -m -s on a
// fixture tree with relative, absolute and looping symlinks.
func TestLedger_portability_realpath(t *testing.T) {
	rp, err := exec.LookPath("realpath")
	if err != nil {
		t.Skip("realpath not available")
	}
	dir := t.TempDir()
	mk := func(p string) { require.NoError(t, os.MkdirAll(filepath.Join(dir, p), 0o755)) }
	mk("a/b")
	sym := func(target, name string) { require.NoError(t, os.Symlink(target, filepath.Join(dir, name))) }
	sym("a/b", "l")
	sym("../..", "a/b/up")
	sym("loop2", "loop1")
	sym("loop1", "loop2")
	sym("/root", "rootlink")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".env"), []byte("S=1\n"), 0o600))
	sym(".env", "settings.json")

	g := guardAt(dir, dir)
	cases := []string{"l", "l/../x", "a/b/up/x", "/etc//x/./y/../z", ".", "a/./b/", "/", "a/new line", "missing/x"}
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
	// A symlink loop fails, as in 2.x (! __llm_realpath loop1).
	_, err = g.Realpath(filepath.Join(dir, "loop1"), false)
	require.Error(t, err)
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
}
