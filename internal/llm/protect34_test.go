// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// protectedDir is DIR with private/ (PROTECT:all), contracts/
// (PROTECT:content), public/, a link to private and a link into
// contracts; env reads the PROTECT: lines from its GASH_ENV_FILE.
func protectedDir(t *testing.T) (string, map[string]string) {
	t.Helper()
	d := physTemp(t)
	mustWrite(t, filepath.Join(d, "private", "a.txt"), "token private\n")
	mustWrite(t, filepath.Join(d, "contracts", "deal.txt"), "token contract\n")
	mustWrite(t, filepath.Join(d, "public", "a.txt"), "token public\n")
	require.NoError(t, os.Symlink(filepath.Join(d, "private"), filepath.Join(d, "plink")))
	require.NoError(t, os.Symlink(filepath.Join(d, "contracts", "deal.txt"), filepath.Join(d, "public", "c.txt")))
	env := baseEnv(physTemp(t), map[string]string{
		"GASH_ENV_FILE": envFile(t, "PROTECT:all="+d+"/private\nPROTECT:content="+d+"/contracts\n"),
	})
	return d, env
}

// TestProtectedPathsLocal: a PROTECT:all tree is refused and left out of
// every walk; a PROTECT:content tree is listed by name, its content never
// read or searched; both through links too.
func TestProtectedPathsLocal(t *testing.T) {
	d, env := protectedDir(t)
	m := New()
	code := func(r result) string {
		t.Helper()
		require.Equal(t, 1, r.RC, r.Stdout)
		return gitCode(t, r)
	}
	for _, p := range []string{"private/a.txt", "plink/a.txt", d + "/private"} {
		require.Equal(t, "forbidden_path/FATAL", code(runGo(t, m, d, env, "llm_read", p)), p)
	}
	for _, p := range []string{"contracts/deal.txt", "public/c.txt"} {
		require.Equal(t, "secret_file_blocked/FATAL", code(runGo(t, m, d, env, "llm_read", p)), p)
	}
	require.Equal(t, "forbidden_path/FATAL", code(runGo(t, m, d, env, "llm_tree", "private")))

	r := runGo(t, m, d, env, "llm_grep", "token", ".")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, "token public")
	require.NotContains(t, r.Stdout, "contract")
	require.NotContains(t, r.Stdout, "private")

	r = runGo(t, m, d, env, "llm_tree", ".", "--depth", "3")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, "deal.txt")
	require.NotContains(t, r.Stdout, "private")

	r = runGo(t, m, d, env, "llm_find", "*.txt", ".")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, "contracts/deal.txt")
	require.NotContains(t, r.Stdout, "private/")

	// llm_exec: by name, through a link, and a walk that would enter one.
	for line, want := range map[string]string{
		"cat private/a.txt":       "forbidden_path/FATAL",
		"ls -la private":          "forbidden_path/FATAL",
		"cat plink/a.txt":         "forbidden_path/FATAL",
		"cat contracts/deal.txt":  "secret_file_blocked/FATAL",
		"wc -l public/c.txt":      "secret_file_blocked/FATAL",
		"grep -r token .":         "forbidden_path/FATAL",
		"find . -name '*.txt'":    "forbidden_path/FATAL",
		"grep -r token contracts": "secret_file_blocked/FATAL",
	} {
		require.Equal(t, want, code(runGo(t, m, d, env, "llm_exec", line)), line)
	}
	r = runGo(t, m, d, env, "llm_exec", "grep -r token public")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "public/a.txt:token public\n", r.Stdout)

	// A content tree alone: a listing may walk it, a search may not.
	only := baseEnv(physTemp(t), map[string]string{"GASH_ENV_FILE": envFile(t, "PROTECT:content="+d+"/contracts\n")})
	r = runGo(t, m, d, only, "llm_exec", "find contracts -name '*.txt'")
	require.Equal(t, "secret_file_blocked/FATAL", code(r), "the root itself is in the tree")
	r = runGo(t, m, d, only, "llm_exec", "find . -name deal.txt")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Equal(t, "./contracts/deal.txt\n", r.Stdout)
	require.Equal(t, "forbidden_path/FATAL", code(runGo(t, m, d, only, "llm_exec", "grep -r token .")))

	// Without the file, nothing is protected.
	r = runGo(t, m, d, baseEnv(physTemp(t), nil), "llm_read", "private/a.txt")
	require.Equal(t, 0, r.RC, r.Stderr)
}

// TestProtectedPathsGit: git outputs leave a PROTECT:all path out and
// show a PROTECT:content path without its content.
func TestProtectedPathsGit(t *testing.T) {
	requireGit(t)
	home := physTemp(t)
	d := physTemp(t)
	git(t, d, home, "init", "-b", "main")
	for _, f := range []string{"private/a.txt", "contracts/deal.txt", "pub.txt"} {
		mustWrite(t, filepath.Join(d, f), "one\n")
	}
	git(t, d, home, "add", "-A")
	git(t, d, home, "commit", "-m", "base")
	for _, f := range []string{"private/a.txt", "contracts/deal.txt", "pub.txt"} {
		mustWrite(t, filepath.Join(d, f), "two secret words\n")
	}
	mustWrite(t, filepath.Join(d, "private", "new.txt"), "x\n")
	env := baseEnv(physTemp(t), map[string]string{
		"GASH_ENV_FILE": envFile(t, "PROTECT:all=~/w/private\nPROTECT:content="+d+"/contracts\n"),
	})
	// ~ is the caller's home: link it to the repository.
	require.NoError(t, os.Symlink(d, filepath.Join(env["HOME"], "w")))
	m := New()

	r := runGo(t, m, d, env, "llm_git_status")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.Contains(t, r.Stdout, "contracts/deal.txt")
	require.NotContains(t, r.Stdout, "private")
	for _, args := range [][]string{{"--files"}, {"--numstat"}, {}, {"--patch"}} {
		r = runGo(t, m, d, env, "llm_git_diff", args...)
		require.Equal(t, 0, r.RC, r.Stderr)
		require.Contains(t, r.Stdout, "pub.txt", args)
		require.NotContains(t, r.Stdout, "private", args)
		if len(args) == 1 && args[0] == "--patch" {
			require.Contains(t, r.Stdout, "[... secret file: patch omitted]")
			require.Equal(t, 1, strings.Count(r.Stdout, "two secret words"), "pub.txt only")
		}
	}
	git(t, d, home, "commit", "-qam", "second")
	r = runGo(t, m, d, env, "llm_git_show", "HEAD", "--patch")
	require.Equal(t, 0, r.RC, r.Stderr)
	require.NotContains(t, r.Stdout, "private")
	require.Equal(t, 1, strings.Count(r.Stdout, "two secret words"), r.Stdout)
	r = runGo(t, m, d, env, "llm_git_log", "--fields", "subject,files", "-n", "1")
	require.Equal(t, `[{"subject":"second","files":["contracts/deal.txt","pub.txt"]}]`+"\n", r.Stdout)
	r = runGo(t, m, d, env, "llm_git_show", "HEAD:private/a.txt")
	require.Equal(t, "forbidden_path/FATAL", gitCode(t, r))
	r = runGo(t, m, d, env, "llm_git_show", "HEAD:contracts/deal.txt")
	require.Equal(t, "secret_file_blocked/FATAL", gitCode(t, r))
}
