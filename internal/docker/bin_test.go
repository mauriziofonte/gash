// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker_test

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/app"
)

// TestMain lets the test binary stand in for the gash binary behind the
// generated shims (GASH_BIN): with DOCKER_TEST_AS_GASH=1 it is app.Main.
func TestMain(m *testing.M) {
	if os.Getenv("DOCKER_TEST_AS_GASH") == "1" {
		os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

// leakNames are the variables LEDGER DOCK-11 lists.
const leakNames = "registry name tag service_name image_ref arg f r svc img local_d remote_d stat upgr err file " +
	"__gash_auth_token __gash_remote_digest __gash_registry_curl_body __gash_registry_curl_http_code __gash_registry_curl_error"

// LEDGER DOCK-11 (forced divergence): the 2.x compose functions leave
// their loop variables and the registry token in the caller's shell;
// through the shim they run in the binary, and the caller's variables
// are the same before and after.
func TestLedger_DOCK11(t *testing.T) {
	for _, p := range []string{"bash", "jq", "awk", "grep"} {
		if _, err := exec.LookPath(p); err != nil {
			t.Skip(p + " not available")
		}
	}
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	compat, err := filepath.Abs("../../testdata/compat")
	require.NoError(t, err)
	if _, err := os.Stat(filepath.Join(root, "lib/modules/docker-compose.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	self, err := os.Executable()
	require.NoError(t, err)

	base := t.TempDir()
	work, stub := filepath.Join(base, "w"), filepath.Join(base, "stub")
	require.NoError(t, os.MkdirAll(work, 0o755))
	require.NoError(t, os.MkdirAll(stub, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(work, "compose.yml"),
		[]byte("services:\n  w:\n    image: nginx:latest\n"), 0o644))
	for _, n := range []string{"docker", "curl"} {
		require.NoError(t, os.Symlink(filepath.Join(root, "tests/mocks/bin", n), filepath.Join(stub, n)))
	}
	// The binary's registry calls go to a proxy that refuses them: no
	// network is reached.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxy := "http://" + ln.Addr().String()
	require.NoError(t, ln.Close())

	run := func(src string) (before, after, check string) {
		t.Helper()
		snap := `{ for __v in ` + leakNames + `; do declare -p "$__v" 2>/dev/null || echo "unset $__v"; done; compgen -v | grep -Ev '^(_|BASH_.*|FUNCNAME|PIPESTATUS|LINENO|SECONDS|RANDOM|SRANDOM|EPOCH.*|COLUMNS|LINES|OLDPWD)$'; }`
		cmd := exec.Command("bash", "--noprofile", "--norc", "-c", src+`
name=mine; file=keep
`+snap+` >"$1/before"
docker_compose_check . --json >"$1/check" 2>&1
docker_compose_upgrade . --dry-run >/dev/null 2>&1
docker_compose_scan . >/dev/null 2>&1
`+snap+` >"$1/after"`, "_", base)
		cmd.Dir = work
		cmd.Env = []string{"HOME=" + base, "PATH=" + stub + ":/usr/bin:/bin", "LC_ALL=C", "TERM=dumb",
			"GASH_BIN=" + self, "DOCKER_TEST_AS_GASH=1", "HTTPS_PROXY=" + proxy, "https_proxy=" + proxy}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		b, err := os.ReadFile(filepath.Join(base, "before"))
		require.NoError(t, err)
		a, err := os.ReadFile(filepath.Join(base, "after"))
		require.NoError(t, err)
		c, err := os.ReadFile(filepath.Join(base, "check"))
		require.NoError(t, err)
		return string(b), string(a), string(c)
	}

	before, after, check := run(`for f in output utils validation; do source "` + root + `/lib/core/$f.sh"; done
source "` + root + `/lib/modules/docker.sh"; source "` + root + `/lib/modules/docker-compose.sh"`)
	require.Contains(t, before, `declare -- name="mine"`)
	require.Contains(t, after, `declare -- name="library/nginx"`)
	require.Contains(t, after, `declare -- __gash_auth_token="mock-dockerhub-token-abc123"`)
	require.Contains(t, check, `"name":"w"`)

	before, after, check = run(`source "` + compat + `/lib/compat/bootstrap.sh"; source "` + compat + `/lib/modules/docker-compose.sh"`)
	require.Contains(t, before, `declare -- name="mine"`)
	require.Contains(t, before, `declare -- file="keep"`)
	require.False(t, strings.Contains(before, "__gash_auth_token=") || strings.Contains(after, "__gash_auth_token="))
	require.Equal(t, before, after)
	require.Contains(t, check, `"name":"w"`)
	require.Contains(t, check, `"error":"connection_refused"`)
}
