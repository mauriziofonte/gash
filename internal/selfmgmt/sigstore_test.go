// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
	"github.com/mauriziofonte/gash/internal/ui"
)

// cosignStub records its arguments in $D/args and checks the files it is
// given exist; it exits with the status in $D/rc, printing an error line
// first when that is not 0.
const cosignStub = `#!/bin/sh
printf '%s\n' "$@" >"$STUB_DIR/args"
for a; do last=$a; done
[ -r "$last" ] || exit 9
rc=$(cat "$STUB_DIR/rc")
[ "$rc" = 0 ] || echo "Error: no matching CertificateIdentity found" >&2
exit "$rc"
`

// The tag of an unsigned release is accepted under GASH_REQUIRE_SIGNED=1
// only when cosign verifies its release's bundle for the tag's workflow,
// ref and commit; without GASH_REQUIRE_SIGNED a missing bundle or cosign is
// silent and an invalid bundle a warning.
func TestVerifyTag_Sigstore(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	d := t.TempDir()
	repo, rel := d+"/repo", d+"/rel"
	require.NoError(t, os.MkdirAll(rel, 0o755))
	env := []string{"HOME=" + d, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
	bashRun(t, d, env, "git init -q repo; git -C repo commit -q --allow-empty -m one; git -C repo tag -a v3.0.1 -m rel")
	commit := bashRun(t, d, env, "git -C repo rev-parse 'v3.0.1^{commit}'")
	require.NoError(t, os.WriteFile(d+"/cosign", []byte(cosignStub), 0o755))
	require.NoError(t, os.WriteFile(d+"/rc", []byte("0"), 0o644))

	run := func(cosign bool, vars ...string) (bool, string) {
		t.Helper()
		m := New()
		m.lookPathFn = func(name string) (string, bool) {
			if name == "cosign" && cosign {
				return d + "/cosign", true
			}
			return "", false
		}
		e := map[string]string{"HOME": d, "PATH": os.Getenv("PATH"), "STUB_DIR": d, "GASH_RELEASE_URL": "file://" + rel,
			"GIT_CONFIG_NOSYSTEM": "1"}
		for _, v := range vars {
			k, val, _ := strings.Cut(v, "=")
			e[k] = val
		}
		var buf bytes.Buffer
		c := &invoke.Call{Env: invoke.EnvMap(e), Term: term.Fixed(invoke.EnvMap(e), false, false, false)}
		ok := m.verifyTag(c, ui.StyleOf(c.Term, false).Printer(&buf), repo, "v3.0.1")
		return ok, buf.String()
	}

	// No bundle in the release.
	ok, out := run(true)
	require.True(t, ok)
	require.Empty(t, out)
	ok, out = run(true, "GASH_REQUIRE_SIGNED=1")
	require.False(t, ok)
	require.Equal(t, "Warning: v3.0.1 is not signed, and its release has no Sigstore signature (checksums.txt.sigstore.json).\n", out)

	require.NoError(t, os.WriteFile(rel+"/checksums.txt", []byte("x  gash_3.0.1_linux_amd64.tar.gz\n"), 0o644))
	require.NoError(t, os.WriteFile(rel+"/checksums.txt.sigstore.json", []byte("{}"), 0o644))

	// No cosign.
	ok, out = run(false)
	require.True(t, ok)
	require.Empty(t, out)
	ok, out = run(false, "GASH_REQUIRE_SIGNED=1")
	require.False(t, ok)
	require.Contains(t, out, "Warning: v3.0.1 is not signed; install cosign to check its Sigstore release signature")

	// Verified: cosign is asked for exactly this workflow, tag and commit.
	ok, out = run(true, "GASH_REQUIRE_SIGNED=1")
	require.True(t, ok)
	require.Equal(t, "OK: Release signature of v3.0.1 verified (Sigstore).\n", out)
	args, err := os.ReadFile(d + "/args")
	require.NoError(t, err)
	a := strings.Split(strings.TrimSpace(string(args)), "\n")
	require.Equal(t, []string{"verify-blob", "--bundle"}, a[:2])
	require.True(t, strings.HasSuffix(a[2], "/checksums.txt.sigstore.json"), a[2])
	require.Equal(t, []string{
		"--certificate-identity", "https://github.com/mauriziofonte/gash/.github/workflows/release.yml@refs/tags/v3.0.1",
		"--certificate-oidc-issuer", "https://token.actions.githubusercontent.com",
		"--certificate-github-workflow-repository", "mauriziofonte/gash",
		"--certificate-github-workflow-ref", "refs/tags/v3.0.1",
		"--certificate-github-workflow-sha", commit,
	}, a[3:13])
	require.True(t, strings.HasSuffix(a[13], "/checksums.txt"), a[13])

	// Does not verify: a warning either way, a refusal only when required.
	require.NoError(t, os.WriteFile(d+"/rc", []byte("1"), 0o644))
	want := "Warning: The release signature of v3.0.1 does not verify: no matching CertificateIdentity found.\n"
	ok, out = run(true, "GASH_REQUIRE_SIGNED=1")
	require.False(t, ok)
	require.Equal(t, want, out)
	ok, out = run(true)
	require.True(t, ok)
	require.Equal(t, want, out)
}
