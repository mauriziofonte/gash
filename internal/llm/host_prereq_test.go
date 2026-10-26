// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// The 3.4 prerequisites of the remote llm_* functions: a program the host
// lacks is named with the host instead of an empty answer, the host rails
// hold without tr, and a missing local ssh is one prerequisite report.

// fallbackTools are the programs the --host fallbacks run.
var fallbackTools = []string{"realpath", "readlink", "find", "tr", "sort", "head", "grep", "wc", "ls", "cat"}

// hostPath is a remote PATH with sh and tools, without the missing ones,
// after the fixture's stub directory (its uname, and in bsd mode its
// realpath and find).
func hostPath(t *testing.T, f *hostFixture, tools []string, missing ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range append([]string{"sh"}, tools...) {
		if slices.Contains(missing, name) {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dir, name)); err == nil {
			continue
		}
		p, err := exec.LookPath(name)
		require.NoError(t, err, name)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	stub, _, _ := strings.Cut(f.ssh.Path, ":")
	return stub + ":" + dir
}

// missingOnWeb is the envelope of a program web lacks.
func missingOnWeb(fn, tool, pkg string) string {
	return `{"error":"prerequisite_missing","details":"` + fn + ` on web needs ` + tool + `, which is not installed there","action":"STOP","recoverable":true,` +
		`"hint":"Install ` + tool + ` on web (usually the ` + pkg + ` package), then retry","missing":"` + tool + `","host":"web"}` + "\n"
}

// TestHostFallbacksNameMissingTools: without find, sort, head, grep or ls
// on the host the fallbacks answered an empty list (2>/dev/null), and
// llm_config skipped its --max-bytes cap without ls.
func TestHostFallbacksNameMissingTools(t *testing.T) {
	for _, tc := range []struct {
		bsd     bool
		missing string
		pkg     string
		fn      string
		args    []string
	}{
		{false, "find", "findutils", "llm_find", []string{"*.php"}},
		{false, "sort", "coreutils", "llm_find", []string{"*.php", "src"}},
		{false, "head", "coreutils", "llm_find", []string{"*"}},
		{false, "find", "findutils", "llm_tree", []string{"src"}},
		{true, "ls", "coreutils", "llm_tree", []string{"--stats"}},
		{true, "readlink", "coreutils", "llm_tree", []string{"."}},
		{false, "grep", "grep", "llm_grep", []string{"TODO"}},
		{false, "find", "findutils", "llm_grep", []string{"TODO", "src"}},
		{false, "wc", "coreutils", "llm_grep", []string{"TODO", "a.txt"}},
		{false, "ls", "coreutils", "llm_config", []string{"conf/app.json", "--max-bytes", "3"}},
		{false, "cat", "coreutils", "llm_config", []string{"conf/app.yaml"}},
	} {
		f := newHostFixtureOn(t, tc.bsd)
		f.tree(t)
		f.ssh.Path = hostPath(t, f, fallbackTools, tc.missing)
		r := runGo(t, f.m, f.dir, f.env, tc.fn, append(tc.args, "--host", "web")...)
		require.Equal(t, 1, r.RC, "%s %q without %s: %s", tc.fn, tc.args, tc.missing, r.Stdout)
		require.Equal(t, missingOnWeb(tc.fn, tc.missing, tc.pkg), r.Stderr, "%s %q without %s", tc.fn, tc.args, tc.missing)
		require.Empty(t, r.Stdout)
	}
	// With every program there, the restricted PATH answers as before.
	f := newHostFixture(t)
	f.tree(t)
	f.ssh.Path = hostPath(t, f, fallbackTools)
	f.same(t, "llm_find", "*.php")
	f.same(t, "llm_grep", "TODO", "src")
}

// TestHostRailsWithoutTr: gash_secret compared an empty string when the
// host had no tr, so llm_config read a symlink to .env and sent it, and
// the llm_grep walk searched secret files. Under dash and bash --posix,
// without tr, the secret is refused and the walk needs tr for its order.
func TestHostRailsWithoutTr(t *testing.T) {
	for _, shell := range [][]string{{"sh"}, {"bash", "--posix"}} {
		if _, err := exec.LookPath(shell[0]); err != nil {
			continue
		}
		f := newHostFixture(t)
		f.tree(t)
		f.ssh.Shell = shell
		f.ssh.Path = hostPath(t, f, append(fallbackTools, shell[0]), "tr")
		r := runGo(t, f.m, f.dir, f.env, "llm_config", "env-link", "--host", "web")
		require.Equal(t, 1, r.RC, shell)
		require.Contains(t, r.Stderr, `"error":"secret_file_blocked"`, shell)
		require.NotContains(t, r.Stdout+r.Stderr, "TODO=secret", shell)

		r = runGo(t, f.m, f.dir, f.env, "llm_grep", "TODO", ".", "--host", "web")
		require.Equal(t, 1, r.RC, shell)
		require.Equal(t, missingOnWeb("llm_grep", "tr", "coreutils"), r.Stderr, shell)
		require.NotContains(t, r.Stdout, "secret", shell)
	}
}

// TestHostRailsWithoutReadlink: without realpath -m and without readlink
// on the host gash_resolve gave a symlink's own path, so llm_config read
// env-link (a link to .env) as a file named env-link and sent .env. The
// link is refused with readlink named now; a path without a link still
// answers.
func TestHostRailsWithoutReadlink(t *testing.T) {
	f := newHostFixtureOn(t, true)
	f.tree(t)
	f.ssh.Path = hostPath(t, f, fallbackTools, "readlink")
	for _, args := range [][]string{{"llm_config", "env-link"}, {"llm_grep", "TODO", "env-link"}} {
		r := runGo(t, f.m, f.dir, f.env, args[0], append(args[1:], "--host", "web")...)
		require.Equal(t, 1, r.RC, args)
		require.Equal(t, missingOnWeb(args[0], "readlink", "coreutils"), r.Stderr, args)
		require.Empty(t, r.Stdout, args)
	}
	r := runGo(t, f.m, f.dir, f.env, "llm_config", "conf/app.json", "--host", "web")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "{\"a\": 1}\n", r.Stdout)
}

// TestHostFallbacksUnderBashPosix: the fallbacks give the local answer
// under bash --posix too (the parity tests run them under sh).
func TestHostFallbacksUnderBashPosix(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("no bash")
	}
	f := newHostFixture(t)
	f.ssh.Shell = []string{"bash", "--posix"}
	testHostFind(t, f)
	f = newHostFixture(t)
	f.ssh.Shell = []string{"bash", "--posix"}
	testHostGrep(t, f)
}

// debian is a pinned Debian 12 without the named programs.
func debian(missing ...string) *prereq.System {
	return &prereq.System{OS: platform.OS{GOOS: "linux", ID: "debian", Name: "Debian GNU/Linux 12 (bookworm)"}, Manager: prereq.Apt, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) {
			return "/usr/bin/" + name, !slices.Contains(missing, name)
		}}
}

// sshMissing is the prerequisite envelope of a missing local ssh.
func sshMissing(need string) string {
	return `{"error":"prerequisite_missing","details":"` + need + ` needs ssh, which is not installed","action":"STOP","recoverable":true,` +
		`"hint":"Install openssh-client: sudo apt-get install -y openssh-client","missing":"ssh","install":"sudo apt-get install -y openssh-client"}` + "\n"
}

// TestRemoteWithoutLocalSSH: without ssh here every --host call, llm_same
// and llm_remote_run gave host_unreachable (RETRY) "check the host is
// up", and the llm_remote_run fan-out exited 0 with the code in each row.
func TestRemoteWithoutLocalSSH(t *testing.T) {
	defer prereq.SetSystem(debian("ssh"))()
	f := newHostFixture(t)
	f.tree(t)
	r := runGo(t, f.m, f.dir, f.env, "llm_find", "*.php", "--host", "web")
	require.Equal(t, 1, r.RC)
	require.Equal(t, sshMissing("llm_find --host"), r.Stderr)
	require.Zero(t, f.ssh.N())

	rf := newRunFixture(t)
	r = rf.run(t, "web", "db", "--", "uptime")
	require.Equal(t, 1, r.RC, "the whole call fails, once")
	require.Equal(t, sshMissing("llm_remote_run"), r.Stderr)
	require.Empty(t, r.Stdout)
	r = rf.run(t, "web", "--step", "true")
	require.Equal(t, sshMissing("llm_remote_run"), r.Stderr)
	require.Zero(t, rf.ssh.N())

	sf := newSameFixture(t)
	_, r = sf.run(t, "app", "web:srv/app")
	require.Equal(t, 1, r.RC)
	require.Equal(t, sshMissing("llm_same"), r.Stderr)
	require.Zero(t, sf.ssh.N())
}

// TestRemoteRunWithoutBash: without bash on the host each line's rc was
// 127 with "bash: not found", read as the command's own result.
func TestRemoteRunWithoutBash(t *testing.T) {
	f := newRunFixture(t)
	dir := t.TempDir()
	for _, name := range []string{"sh", "grep", "date", "cat"} {
		p, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	f.ssh.Path = dir
	r := f.run(t, "web", "db", "--", "uptime")
	require.Equal(t, 0, r.RC, r.Stderr)
	got := rows(t, r.Stdout)
	require.Len(t, got, 2)
	for i, h := range []string{"web", "db"} {
		require.Nil(t, got[i]["rc"], "the line never ran")
		require.Equal(t, "prerequisite_missing", got[i]["error"])
		require.Equal(t, "llm_remote_run on "+h+" needs bash, which is not installed there", got[i]["err"])
	}
	r = f.run(t, "web", "--step", "uptime", "--step", "true")
	require.Equal(t, 1, r.RC)
	require.Equal(t, missingOnWeb("llm_remote_run", "bash", "bash"), r.Stderr)
}

// TestSameWithoutFind: without find on the host the remote tree hashed to
// nothing and every local file was reported as a difference.
func TestSameWithoutFind(t *testing.T) {
	f := newSameFixture(t)
	stub, _, _ := strings.Cut(f.ssh.Path, ":")
	dir := t.TempDir()
	for _, name := range []string{"sh", "sha256sum", "realpath", "readlink"} {
		p, err := exec.LookPath(name)
		require.NoError(t, err)
		require.NoError(t, os.Symlink(p, filepath.Join(dir, name)))
	}
	f.ssh.Path = stub + ":" + dir
	_, r := f.run(t, "app", "web:srv/app")
	require.Equal(t, 1, r.RC)
	require.Equal(t, missingOnWeb("llm_same", "find", "findutils"), r.Stderr)

	_, r = f.run(t, "app", "web:srv/app", "--container", "api")
	require.Equal(t, 1, r.RC)
	require.Equal(t, `{"error":"prerequisite_missing","details":"llm_same on web needs find, which is not installed in container api","action":"STOP","recoverable":true,`+
		`"hint":"Install find in container api on web (usually the findutils package), then retry","missing":"find","host":"web","container":"api"}`+"\n", r.Stderr)

	// A file needs no find.
	out, r := f.run(t, "app/index.php", "web:srv/app/index.php")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.True(t, out.Same)

	// docker is the host's own.
	f.ssh.Path = dir
	_, r = f.run(t, "app", "web:srv/app", "--container", "api")
	require.Equal(t, 1, r.RC)
	require.Equal(t, missingOnWeb("llm_same", "docker", "docker"), r.Stderr)
}
