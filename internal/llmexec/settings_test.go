// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/pathguard"
)

// TestSettings_Allow: GASH_LLM_EXEC_ALLOW adds extra commands that skip the
// per-command and walk rules (EXEC-05) but still get the secret/path checks;
// a command not listed is still refused (llm_exec_security_spec.sh:117).
func TestSettings_Allow(t *testing.T) {
	cwd := t.TempDir()
	env := testEnv(cwd)
	// seq is not on the allowlist.
	if _, err := Build("seq 2", Settings{}, env, permissiveProbe()); errCode(err) != "command_not_allowed" {
		t.Errorf("seq without allow: got %v", err)
	}
	// With it allowed, seq runs; a non-allowed command in the same line is refused.
	s := Settings{ExecAllow: "seq"}
	if _, err := Build("seq 2 | wc -l", s, env, permissiveProbe()); err != nil {
		t.Errorf("seq allowed: unexpected %v", err)
	}
	if _, err := Build("seq 2 | xargs echo", s, env, permissiveProbe()); errCode(err) != "command_not_allowed" {
		t.Errorf("xargs still refused: got %v", err)
	}
}

// TestLedger_EXEC05_2x: a GASH_LLM_EXEC_ALLOW command skips the per-command
// and walk rules (so a symlink-following/root walk is allowed) but still gets
// the per-word secret and path checks (LEDGER EXEC-05, reproduced in rc.A).
func TestLedger_EXEC05_2x(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(cwd+"/p", 0o755); err != nil {
		t.Fatal(err)
	}
	s := Settings{ExecAllow: "find"}
	// 3.0: EXEC-05 fixed. -L (a symlink-following walk) is option_not_allowed
	// for an allow-listed find too (2.x skipped the walk rules).
	if _, err := Build("find -L p", s, testEnv(cwd), permissiveProbe()); errCode(err) != "option_not_allowed" {
		t.Errorf("allow-listed find -L p must keep the walk rules, got %v", err)
	}
	// The forbidden-path check on an argument still applies.
	if _, err := Build("find -L /proc", s, testEnv(cwd), permissiveProbe()); errCode(err) == "" {
		t.Errorf("allow-listed find /proc must still be refused, got %v", err)
	}
}

// TestLedger_EXEC05: a command in GASH_LLM_EXEC_ALLOW skips the per-command
// option rules but keeps the walk rules (no symlink-following walk, no
// walk root at / or above a protected tree) and the secret/path checks.
func TestLedger_EXEC05(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(cwd+"/p", 0o755); err != nil {
		t.Fatal(err)
	}
	s := Settings{ExecAllow: "find grep"}
	env := testEnv(cwd)
	for line, want := range map[string]string{
		// Option rules lifted: -delete and -exec are refused for the
		// plain allowlist, accepted when the operator names find.
		"find p -name x -delete": "",
		"find p -exec true ;":    "",
		// Walk rules kept.
		"find -L p":      "option_not_allowed",
		"find p -follow": "option_not_allowed",
		"find /":         "forbidden_path",
		"grep -R x p":    "option_not_allowed",
		"grep -r x /":    "forbidden_path",
		// Per-word checks kept.
		"find /proc":     "forbidden_path",
		"grep x .env":    "secret_file_blocked",
		"find p -name x": "",
	} {
		_, err := Build(line, s, env, permissiveProbe())
		if got := errCode(err); got != want {
			t.Errorf("Build(%q) = %v, want %q", line, err, want)
		}
	}
	// Without the allow-list entry, the option rules apply.
	if _, err := Build("find p -name x -delete", Settings{}, env, permissiveProbe()); errCode(err) == "" {
		t.Errorf("find -delete must be refused without GASH_LLM_EXEC_ALLOW")
	}
}

// TestSettings_Unrestricted: unrestricted mode skips the parser and allowlist
// and passes the raw line, but the blocklist still applies; the child keeps
// the caller's environment with BASH_ENV/ENV cleared.
func TestSettings_Unrestricted(t *testing.T) {
	cwd := t.TempDir()
	env := testEnv(cwd)
	env.Vars = map[string]string{"PATH": os.Getenv("PATH"), "HOME": cwd, "MY_TOKEN": "sekrit", "BASH_ENV": "/x", "ENV": "/y"}
	s := Settings{Exec: "unrestricted"}

	plan, err := Build("seq 2 | xargs echo", s, env, permissiveProbe())
	if err != nil {
		t.Fatalf("unrestricted should allow: %v", err)
	}
	// Raw line passed to bash -c (not a canonical script).
	if script := plan.Argv[len(plan.Argv)-1]; !strings.Contains(script, "seq 2 | xargs echo") {
		t.Errorf("raw line not passed: %q", script)
	}
	// Caller env kept, BASH_ENV and ENV blanked.
	var sawToken, sawBashEnv, sawEnv bool
	for _, kv := range plan.Env {
		switch {
		case kv == "MY_TOKEN=sekrit":
			sawToken = true
		case kv == "BASH_ENV=":
			sawBashEnv = true
		case kv == "ENV=":
			sawEnv = true
		}
	}
	if !sawToken || !sawBashEnv || !sawEnv {
		t.Errorf("env: token=%v bashenv=%v env=%v (%v)", sawToken, sawBashEnv, sawEnv, plan.Env)
	}
	// The blocklist still applies.
	if _, err := Build("rm -rf /", s, env, permissiveProbe()); errCode(err) != "dangerous_command_blocked" {
		t.Errorf("blocklist in unrestricted: got %v", err)
	}
}

// TestChildEnv_Allowlist: the restricted child gets only the allowlisted
// variables plus PWD and the git variables; a caller secret never reaches it
// (llm_exec_walk_spec.sh:154).
func TestChildEnv_Allowlist(t *testing.T) {
	cwd := t.TempDir()
	env := testEnv(cwd)
	env.Vars = map[string]string{
		"PATH": "/bin", "HOME": cwd, "LC_TIME": "C.UTF-8",
		"MY_API_TOKEN": "sekrit", "GH_PAT": "ghp_x", "RIPGREP_CONFIG_PATH": "/x", "POSIXLY_CORRECT": "1",
	}
	plan, err := Build("ls", Settings{}, env, permissiveProbe())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, kv := range plan.Env {
		i := strings.IndexByte(kv, '=')
		got[kv[:i]] = kv[i+1:]
	}
	for _, leak := range []string{"MY_API_TOKEN", "GH_PAT", "RIPGREP_CONFIG_PATH", "POSIXLY_CORRECT"} {
		if _, ok := got[leak]; ok {
			t.Errorf("%s leaked into child env", leak)
		}
	}
	if got["HOME"] != cwd || got["LC_TIME"] != "C.UTF-8" || got["PWD"] != cwd ||
		got["GIT_OPTIONAL_LOCKS"] != "0" || got["GIT_TERMINAL_PROMPT"] != "0" {
		t.Errorf("child env missing expected entries: %v", got)
	}
}

// TestSandbox_Unavailable and the bwrap argv (llm_exec_security_spec.sh:137).
func TestSandbox(t *testing.T) {
	cwd := t.TempDir()
	env := testEnv(cwd)
	// No bwrap: refused before anything runs.
	if _, err := Build("echo hi", Settings{Sandbox: "1"}, env, fakeProbe{sed: true, gawk: true, diff: true, gnu: true, bwrap: false}); errCode(err) != "sandbox_unavailable" {
		t.Errorf("no bwrap: got %v", err)
	}
	// bwrap present: argv is wrapped with the read-only sandbox flags.
	plan, err := Build("echo hi", Settings{Sandbox: "1"}, env, fakeProbe{sed: true, gawk: true, diff: true, gnu: true, bwrap: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(plan.Argv, " ")
	for _, flag := range []string{"bwrap", "--ro-bind", "--unshare-all", "--die-with-parent", "--proc"} {
		if !strings.Contains(joined, flag) {
			t.Errorf("sandbox argv missing %q: %v", flag, plan.Argv)
		}
	}
	if plan.Argv[0] != "bwrap" {
		t.Errorf("argv[0] = %q, want bwrap", plan.Argv[0])
	}
}

// wslFS is the real filesystem plus a /run/WSL directory, as under WSL.
type wslFS struct{ pathguard.OSFS }

func (f wslFS) Lstat(name string) (fs.FileInfo, error) {
	if name == WSLInteropDir {
		return os.Lstat(os.TempDir())
	}
	return f.OSFS.Lstat(name)
}

// TestSandbox_WSLInterop: under WSL the sandbox hides the interop sockets
// and never passes WSL_INTEROP, so a Windows program cannot start from it.
func TestSandbox_WSLInterop(t *testing.T) {
	cwd := t.TempDir()
	probe := fakeProbe{sed: true, gawk: true, diff: true, gnu: true, bwrap: true}
	for _, mode := range []string{"", "unrestricted"} {
		env := testEnv(cwd)
		env.FS = wslFS{}
		env.Vars["WSL_INTEROP"] = "/run/WSL/42_interop"
		plan, err := Build("echo hi", Settings{Exec: mode, Sandbox: "1"}, env, probe)
		if err != nil {
			t.Fatal(err)
		}
		joined := strings.Join(plan.Wrap, " ")
		if !strings.Contains(joined, "--tmpfs "+WSLInteropDir) {
			t.Errorf("mode %q: sandbox does not hide %s: %v", mode, WSLInteropDir, plan.Wrap)
		}
		for _, kv := range plan.Env {
			if strings.HasPrefix(kv, "WSL_INTEROP=") {
				t.Errorf("mode %q: WSL_INTEROP reaches the sandbox", mode)
			}
		}
	}
	// Without the sandbox nothing changes; on a machine without /run/WSL
	// the sandbox argv has no tmpfs.
	env := testEnv(cwd)
	env.Vars["WSL_INTEROP"] = "/run/WSL/42_interop"
	plan, err := Build("echo hi", Settings{Exec: "unrestricted"}, env, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Env, "WSL_INTEROP=/run/WSL/42_interop") {
		t.Errorf("unrestricted without sandbox lost WSL_INTEROP: %v", plan.Env)
	}
	env.FS = nil
	if plan, _ = Build("echo hi", Settings{Sandbox: "1"}, env, probe); strings.Contains(strings.Join(plan.Wrap, " "), "--tmpfs") && !isDir(pathguard.OSFS{}, WSLInteropDir) {
		t.Errorf("tmpfs without %s: %v", WSLInteropDir, plan.Wrap)
	}
}
