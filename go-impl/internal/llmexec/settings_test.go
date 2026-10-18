// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"strings"
	"testing"
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
	// -L (symlink-following walk) would normally be option_not_allowed; the
	// allow-listed command skips that rule.
	if _, err := Build("find -L p", s, testEnv(cwd), permissiveProbe()); err != nil {
		t.Errorf("allow-listed find -L p should skip walk rules, got %v", err)
	}
	// The forbidden-path check on an argument still applies.
	if _, err := Build("find -L /proc", s, testEnv(cwd), permissiveProbe()); errCode(err) != "forbidden_path" {
		t.Errorf("allow-listed find /proc must still be refused, got %v", err)
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
