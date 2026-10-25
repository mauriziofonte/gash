// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// upgradeEnv is the environment of the fixture repositories and of the
// calls: a temporary HOME and GNUPGHOME, fixed identities and dates.
func upgradeEnv(d string) map[string]string {
	return map[string]string{
		"LANG": "C.UTF-8", "PATH": upgradePath, "HOME": d + "/home", "GNUPGHOME": d + "/gnupg",
		"GIT_CONFIG_NOSYSTEM": "1", "GIT_AUTHOR_NAME": "t", "GIT_AUTHOR_EMAIL": "t@t", "GIT_COMMITTER_NAME": "t",
		"GIT_COMMITTER_EMAIL": "t@t", "GIT_AUTHOR_DATE": "2024-05-01T12:00:00Z", "GIT_COMMITTER_DATE": "2024-05-01T12:00:00Z",
	}
}

// upgradePath is the PATH of the fixtures: the system's, then the directory
// of a gpg found elsewhere (Homebrew's on a macOS runner).
var upgradePath = func() string {
	p := "/usr/bin:/bin"
	if g, err := exec.LookPath("gpg"); err == nil {
		if dir := filepath.Dir(g); dir != "/usr/bin" && dir != "/bin" {
			p += ":" + dir
		}
	}
	return p
}()

// upgradeTempDir is a temporary directory with a short path: GNUPGHOME
// lives in it, and the gpg-agent sockets there must fit in sun_path even
// under a deep macOS $TMPDIR.
func upgradeTempDir(t *testing.T) string {
	t.Helper()
	d, err := os.MkdirTemp("", "gu")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(d) })
	return d
}

// Fixture builders: $D/o is the origin, $D/g the install.
const (
	upgradeOrigin = `git init -q $D/o; git -C $D/o commit -q --allow-empty -m one; `
	upgradeTwo    = upgradeOrigin + `git -C $D/o tag v1.0; git clone -q $D/o $D/g; ` +
		`git -C $D/g -c advice.detachedHead=false checkout -q v1.0; git -C $D/o commit -q --allow-empty -m two; `
	upgradeGPG = `mkdir -m 700 $D/gnupg; gpg --batch --pinentry-mode loopback --passphrase '' ` +
		`--quick-gen-key t@t ed25519 sign never 2>/dev/null; `
)

var upgradeCases = []struct {
	name, setup string
	vars        []string
	args        [][]string
}{
	{"not a checkout", "mkdir -p $D/g", nil, nil},
	{"missing", "", nil, nil},
	{"dirty", upgradeOrigin + "git clone -q $D/o $D/g; : >$D/g/new", nil, nil},
	{"no origin", upgradeOrigin + "mv $D/o $D/g", nil, nil},
	{"origin gone", upgradeOrigin + "git clone -q $D/o $D/g; rm -rf $D/o", nil, nil},
	{"no tags", upgradeOrigin + "git clone -q $D/o $D/g", nil, nil},
	{"up to date", upgradeOrigin + "git -C $D/o tag v1.0; git clone -q -b v1.0 $D/o $D/g 2>/dev/null", nil, nil},
	{"upgrade", upgradeTwo + "git -C $D/o tag v1.1", nil, [][]string{{}, {}}},
	{"untagged head", upgradeOrigin + "git -C $D/o tag v1.0; git -C $D/o commit -q --allow-empty -m two; git clone -q $D/o $D/g", nil, nil},
	{"tag order", upgradeTwo + "git -C $D/o tag 1.2; git -C $D/o tag v1.10; git -C $D/o tag v1.9; git -C $D/o tag v2.0-beta", nil, nil},
	{"rc install", upgradeOrigin + "git -C $D/o tag v2.0.0; git -C $D/o commit -q --allow-empty -m b; git -C $D/o tag v2.1.0; " +
		"git -C $D/o commit -q --allow-empty -m c; git -C $D/o tag v3.0.0-rc.1; git clone -q -b v3.0.0-rc.1 $D/o $D/g 2>/dev/null", nil, nil},
	{"unsigned required", upgradeTwo + "git -C $D/o tag v1.1", []string{"GASH_REQUIRE_SIGNED=1"}, nil},
	{"unsigned not required", upgradeTwo + "git -C $D/o tag v1.1", []string{"GASH_REQUIRE_SIGNED=0"}, nil},
	{"marker in message", upgradeTwo + `git -C $D/o tag -a v1.1 -m 'see -----BEGIN PGP SIGNATURE----- here'`, nil, nil},
	{"marker required", upgradeTwo + `git -C $D/o tag -a v1.1 -m 'see -----BEGIN SSH SIGNATURE----- here'`, []string{"GASH_REQUIRE_SIGNED=1"}, nil},
	{"signed", upgradeTwo + upgradeGPG + "git -C $D/o -c user.signingkey=t@t tag -s v1.1 -m rel", []string{"GASH_REQUIRE_SIGNED=1"}, nil},
	{"signed unknown key", upgradeTwo + upgradeGPG + "git -C $D/o -c user.signingkey=t@t tag -s v1.1 -m rel; mv $D/gnupg $D/gnupg.off; " +
		"mkdir -m 700 $D/gnupg", nil, nil},
	{"lightweight on signed commit", upgradeOrigin + "git -C $D/o tag v1.0; git clone -q -b v1.0 $D/o $D/g 2>/dev/null; " + upgradeGPG +
		"git -C $D/o -c user.signingkey=t@t commit -q -S --allow-empty -m signed; git -C $D/o tag v1.1", []string{"GASH_REQUIRE_SIGNED=1"}, nil},
	{"index locked", upgradeTwo + "git -C $D/o tag v1.1; : >$D/g/.git/index.lock", nil, nil},
	{"extra arguments", upgradeTwo + "git -C $D/o tag v1.1", nil, [][]string{{"--bogus", "x"}}},
	{"help", "", nil, [][]string{{"-h"}, {"--help"}}},
}

// upgradeSELF01 names the cases where 3.0 no longer matches 2.x: a
// checkout newer than the latest release is not moved back (SELF-01).
var upgradeSELF01 = map[string]bool{"untagged head": true, "rc install": true}

// upgradeSELF10 names the cases where a tag that only looks signed is now
// unsigned (SELF-10), with the line 3.0 prints and its status; the
// resulting checkout is still the one 2.x left.
var upgradeSELF10 = map[string]struct {
	line string
	rc   int
}{
	"marker in message":            {"Upgraded Gash from v1.0 to v1.1.", 0},
	"marker required":              {"v1.1 is not signed; install cosign", 1},
	"lightweight on signed commit": {"v1.1 is not signed; install cosign", 1},
	// 3.0.1: an unsigned tag may still carry a Sigstore release signature,
	// and the warning says how to check it (cosign is not on the fixtures'
	// PATH).
	"unsigned required": {"v1.1 is not signed; install cosign", 1},
}

// gash_upgrade does what 2.x does on the same fixtures: the same messages
// (git's own included), status and resulting checkout.
func TestUpgradeMatches2x(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	for _, mode := range []string{"off", "on"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ref, got := upgradeTempDir(t), upgradeTempDir(t)
			state := `git -C $D/g describe --tags --exact-match HEAD 2>/dev/null || git -C $D/g rev-parse --abbrev-ref HEAD 2>/dev/null || echo none`
			var script strings.Builder
			if mode == "off" {
				script.WriteString("__gash_use_color() { return 1; }\n")
			} else {
				script.WriteString("__gash_use_color() { return 0; }\n")
			}
			calls := 0
			for i, tc := range upgradeCases {
				for _, base := range []string{ref, got} {
					d := filepath.Join(base, strconv.Itoa(i))
					envBash(t, upgradeEnv(d), "D="+shq(d)+"; mkdir -p $D/home; "+tc.setup)
					t.Cleanup(func() { _ = exec.Command("gpgconf", "--homedir", d+"/gnupg", "--kill", "gpg-agent").Run() })
				}
				d := filepath.Join(ref, strconv.Itoa(i))
				args := tc.args
				if args == nil {
					args = [][]string{{}}
				}
				for _, a := range args {
					calls++
					script.WriteString("(")
					for k, v := range upgradeEnv(d) {
						script.WriteString("export " + k + "=" + shq(v) + "; ")
					}
					for _, v := range tc.vars {
						script.WriteString("export " + v + "; ")
					}
					script.WriteString("GASH_DIR=" + shq(d+"/g") + " gash_upgrade")
					for _, w := range a {
						script.WriteString(" " + shq(w))
					}
					script.WriteString(" 2>&1; printf '\\0%s\\0' $?)\n")
				}
			}
			out, _, _ := run2xIn(t, copy2x(t), []string{"LANG=C.UTF-8"}, script.String())
			fields := strings.Split(out, "\x00")
			require.Len(t, fields, 2*calls+1, out)
			n := 0
			for i, tc := range upgradeCases {
				dRef, d := filepath.Join(ref, strconv.Itoa(i)), filepath.Join(got, strconv.Itoa(i))
				args := tc.args
				if args == nil {
					args = [][]string{{}}
				}
				before := envBash(t, upgradeEnv(d), "D="+shq(d)+"; "+state)
				for _, a := range args {
					env := upgradeEnv(d)
					env["GASH_DIR"] = d + "/g"
					for _, v := range tc.vars {
						k, val, _ := strings.Cut(v, "=")
						env[k] = val
					}
					o, rc := runMerged(t, "gash_upgrade", env, mode == "on", a...)
					if tc.name == "help" {
						// The help page carries the 3.0 notes (SELF-01); the
						// help package tests its rendering.
						n++
						continue
					}
					if tc.name == "extra arguments" {
						// 3.0: SELF-15 fixed: the argument is refused before
						// any git command (2.x ignored it and upgraded).
						require.Contains(t, o, "Unknown option: --bogus (usage: gash_upgrade)", tc.name)
						require.Equal(t, 2, rc, tc.name)
					} else if want, ok := upgradeSELF10[tc.name]; ok {
						// 3.0: SELF-10 fixed: no "signed, but could not be
						// verified" for a tag without a signature block.
						require.Contains(t, o, want.line, tc.name)
						require.NotContains(t, o, "could not be verified", tc.name)
						require.Equal(t, want.rc, rc, tc.name)
					} else if upgradeSELF01[tc.name] {
						// 3.0: SELF-01 fixed: a checkout newer than the latest
						// release stays where it is (2.x moved it back).
						require.Contains(t, o, "is newer than the latest release", tc.name)
						require.Equal(t, 0, rc, tc.name)
					} else {
						require.Equal(t, strings.ReplaceAll(fields[2*n], dRef, d), o, "%s %q", tc.name, a)
						require.Equal(t, fields[2*n+1], strconv.Itoa(rc), "%s %q", tc.name, a)
					}
					n++
				}
				if upgradeSELF01[tc.name] || tc.name == "extra arguments" {
					require.Equal(t, before, envBash(t, upgradeEnv(d), "D="+shq(d)+"; "+state), tc.name)
					continue
				}
				require.Equal(t, envBash(t, upgradeEnv(dRef), "D="+shq(dRef)+"; "+state),
					envBash(t, upgradeEnv(d), "D="+shq(d)+"; "+state), tc.name)
			}
		})
	}
}

// envBash runs a fixture script with env and returns its stdout.
func envBash(t *testing.T, env map[string]string, script string) string {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.Output()
	var ee *exec.ExitError
	if err != nil && !errors.As(err, &ee) {
		require.NoError(t, err, script)
	}
	return string(out)
}

// upgradeFixture builds $D/o and $D/g with setup and returns the call's
// environment.
func upgradeFixture(t *testing.T, setup string) (string, map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	d := upgradeTempDir(t)
	envBash(t, upgradeEnv(d), "D="+shq(d)+"; mkdir -p $D/home; "+setup)
	env := upgradeEnv(d)
	env["GASH_DIR"] = d + "/g"
	return d, env
}

// LEDGER SELF-01: an install on a pre-release tag newer than the latest
// release, or on a commit after it, is left where it is with a "newer than"
// line; an older release still moves to the latest (3.0: SELF-01 fixed;
// 2.x compared for equality only and moved both back to v2.1.0).
func TestLedger_SELF01(t *testing.T) {
	mirror := upgradeOrigin + "git -C $D/o tag v2.0.0; git -C $D/o commit -q --allow-empty -m b; git -C $D/o tag v2.1.0; " +
		"git -C $D/o commit -q --allow-empty -m c; git -C $D/o tag v3.0.0-rc.1; git -C $D/o commit -q --allow-empty -m d; "
	for _, tc := range []struct{ from, clone, want, head string }{
		{"v3.0.0-rc.1", "git clone -q -b v3.0.0-rc.1 $D/o $D/g 2>/dev/null",
			"Info: This checkout (v3.0.0-rc.1) is newer than the latest release (v2.1.0); not downgrading.\n", "v3.0.0-rc.1"},
		{"untagged", "git clone -q $D/o $D/g",
			"Info: This checkout (v3.0.0-rc.1-1-g", ""},
		{"untagged after the release", "git clone -q $D/o $D/g; git -C $D/g -c advice.detachedHead=false checkout -q v2.1.0; " +
			"git -C $D/g tag -d v3.0.0-rc.1 >/dev/null; git -C $D/g commit -q --allow-empty -m local",
			"Info: This checkout (v2.1.0-1-g", ""},
		{"after the release, nearest tag not a version", "git clone -q $D/o $D/g; git -C $D/g -c advice.detachedHead=false checkout -q v2.1.0; " +
			"git -C $D/g commit -q --allow-empty -m local; git -C $D/g tag nightly",
			"Info: This checkout (nightly) is newer", ""},
		{"v2.0.0", "git clone -q -b v2.0.0 $D/o $D/g 2>/dev/null", "OK: Upgraded Gash from v2.0.0 to v2.1.0.\n", "v2.1.0"},
	} {
		d, env := upgradeFixture(t, mirror+tc.clone)
		before := envBash(t, upgradeEnv(d), "git -C "+shq(d+"/g")+" rev-parse HEAD")
		out, rc := runMerged(t, "gash_upgrade", env, false)
		require.Equal(t, 0, rc, tc.from)
		require.Contains(t, out, tc.want, tc.from)
		after := envBash(t, upgradeEnv(d), "git -C "+shq(d+"/g")+" rev-parse HEAD")
		if tc.head == "v2.1.0" {
			require.NotContains(t, out, "newer than", tc.from)
			require.Equal(t, "v2.1.0\n", envBash(t, upgradeEnv(d), "git -C "+shq(d+"/g")+" describe --tags --exact-match HEAD"), tc.from)
			continue
		}
		require.Contains(t, out, "is newer than the latest release (v2.1.0); not downgrading.\n", tc.from)
		require.NotContains(t, out, "Upgraded", tc.from)
		require.Equal(t, before, after, tc.from+": HEAD moved")
	}
}

// LEDGER SELF-10: a tag is signed only when it is an annotated tag whose
// object ends with a signature block, so an annotated tag whose message
// quotes the marker and a lightweight tag on a signed commit are "not
// signed" (3.0: SELF-10 fixed; rc.A matched the marker as text and said
// "signed, but the signature could not be verified").
func TestLedger_SELF10(t *testing.T) {
	_, env := upgradeFixture(t, upgradeTwo+`git -C $D/o tag -a v1.1 -m 'quoting -----BEGIN PGP SIGNATURE----- in text'`)
	out, rc := runMerged(t, "gash_upgrade", env, false)
	require.Equal(t, 0, rc)
	require.NotContains(t, out, "could not be verified")
	require.Contains(t, out, "OK: Upgraded Gash from v1.0 to v1.1.\n")

	_, env = upgradeFixture(t, upgradeTwo+`git -C $D/o tag -a v1.1 -m "$(printf 'see\n-----BEGIN SSH SIGNATURE-----\nno end')"`)
	env["GASH_REQUIRE_SIGNED"] = "1"
	out, rc = runMerged(t, "gash_upgrade", env, false)
	require.Equal(t, 1, rc)
	require.Contains(t, out, "Warning: v1.1 is not signed; install cosign to check its Sigstore release signature "+
		"(https://docs.sigstore.dev/cosign/system_config/installation/).\nError: GASH_REQUIRE_SIGNED=1 is set; not upgrading to v1.1.\n")

	if _, err := exec.LookPath("gpg"); err != nil {
		t.Skip("no gpg")
	}
	// gpg-agent's socket lives in the fixture's GNUPGHOME under TMPDIR; a
	// Unix socket path is limited to 107 bytes.
	if len(os.TempDir()) > 70 {
		t.Skip("TMPDIR is too long for a gpg-agent socket")
	}
	d, env := upgradeFixture(t, strings.Replace(upgradeTwo, "git -C $D/o commit -q --allow-empty -m two; ", "", 1)+upgradeGPG+
		"git -C $D/o -c user.signingkey=t@t commit -q -S --allow-empty -m signed; git -C $D/o tag v1.1")
	t.Cleanup(func() { _ = exec.Command("gpgconf", "--homedir", d+"/gnupg", "--kill", "gpg-agent").Run() })
	env["GASH_REQUIRE_SIGNED"] = "1"
	out, rc = runMerged(t, "gash_upgrade", env, false)
	require.Equal(t, 1, rc)
	require.Contains(t, out, "Warning: v1.1 is not signed; install cosign to check its Sigstore release signature "+
		"(https://docs.sigstore.dev/cosign/system_config/installation/).\n"+
		"Error: GASH_REQUIRE_SIGNED=1 is set; not upgrading to v1.1.\n")

	// A signed tag with the key still verifies.
	d, env = upgradeFixture(t, upgradeTwo+upgradeGPG+"git -C $D/o -c user.signingkey=t@t tag -s v1.1 -m rel")
	t.Cleanup(func() { _ = exec.Command("gpgconf", "--homedir", d+"/gnupg", "--kill", "gpg-agent").Run() })
	env["GASH_REQUIRE_SIGNED"] = "1"
	out, rc = runMerged(t, "gash_upgrade", env, false)
	require.Equal(t, 0, rc, out)
	require.Contains(t, out, "OK: Signature of v1.1 verified.\n")
}
