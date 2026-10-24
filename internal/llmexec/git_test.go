// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupRepo builds the git fixture repository of llm_exec_walk_spec.sh:82.
func setupRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git required")
	}
	home := t.TempDir()
	repo := filepath.Join(home, "r")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"),
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join(repo, "sub", ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	run("init", "-q")
	run("config", "user.email", "t@t")
	run("config", "user.name", "t")
	mustWrite(t, filepath.Join(repo, ".env"), "PASSWORD=env\n")
	mustWrite(t, filepath.Join(repo, ".ENV.local"), "PASSWORD=local\n")
	mustWrite(t, filepath.Join(repo, "sub", ".ssh", "notes"), "PASSWORD=key\n")
	mustWrite(t, filepath.Join(repo, "app.txt"), "PASSWORD=ok\n")
	run("add", "-A")
	run("commit", "-qm", "one")
	mustWrite(t, filepath.Join(repo, ".env"), "PASSWORD=env2\n")
	mustWrite(t, filepath.Join(repo, "app.txt"), "PASSWORD=ok2\n")
	run("commit", "-qam", "two")
	return repo
}

// TestGitContents: git commands that print file contents leave secret files
// out, with output identical to 2.x (llm_exec_walk_spec.sh:82).
func TestGitContents(t *testing.T) {
	repo := setupRepo(t)
	home := filepath.Dir(repo)
	lines := []string{
		"git grep PASSWORD", "git grep PASSWORD -- .", "git log -p",
		"git show", "git show HEAD~1", "git diff HEAD~1 HEAD", "git reflog -p",
	}
	for _, line := range lines {
		mine, mineExit, err := runMineCombined(t, line, repo, home)
		if err != nil {
			t.Errorf("%q: build refused: %v", line, err)
			continue
		}
		if !strings.Contains(mine, "PASSWORD=ok") {
			t.Errorf("%q: nothing found: %q", line, mine)
		}
		for _, leak := range []string{"PASSWORD=env", "PASSWORD=local", "PASSWORD=key"} {
			if strings.Contains(mine, leak) {
				t.Errorf("%q: leaked %s: %q", line, leak, mine)
			}
		}
		ref, refExit := run2x(t, line, repo, home)
		if mine != ref || mineExit != refExit {
			t.Errorf("%q:\n  mine = %q (%d)\n  2.x  = %q (%d)", line, mine, mineExit, ref, refExit)
		}
	}
	// Listings keep every commit; a REV:PATH is checked like a file name.
	count, _, _ := runMineCombined(t, "git log --oneline", repo, home)
	if n := len(strings.Split(strings.TrimRight(count, "\n"), "\n")); n != 2 {
		t.Errorf("git log --oneline: %d lines, want 2", n)
	}
	out, _, _ := runMineCombined(t, "git show HEAD:app.txt", repo, home)
	if strings.TrimSpace(out) != "PASSWORD=ok2" {
		t.Errorf("git show HEAD:app.txt = %q", out)
	}
}

// TestGitRefusals: build-time and runtime refusals around git contents.
func TestGitRefusals(t *testing.T) {
	repo := setupRepo(t)
	home := filepath.Dir(repo)

	// Build-time refusals (parser / allowlist).
	buildRefusals := map[string]string{
		"git show HEAD:.ENV.local":       "secret_file_blocked",
		"git log -L1,1:.ENV.local":       "secret_file_blocked",
		"git --literal-pathspecs log -p": "option_not_allowed",
		"git grep --no-index PASS":       "option_not_allowed",
		"git cat-file --batch":           "option_not_allowed",
		"git stash show -p":              "option_not_allowed",
		"git remote show origin":         "option_not_allowed",
	}
	for line, want := range buildRefusals {
		assertRefuse(t, line, repo, want)
	}

	// A blob named by id is refused at runtime by the child git wrapper.
	blob := gitRevParse(t, repo, "HEAD:.env")
	for _, line := range []string{"git show " + blob, "git cat-file -p " + blob} {
		out, exit, err := runMineCombined(t, line, repo, home)
		if err != nil {
			t.Fatalf("%q build: %v", line, err)
		}
		if exit == 0 || !strings.Contains(out, "option_not_allowed") {
			t.Errorf("%q: expected runtime option_not_allowed, got %q (exit %d)", line, out, exit)
		}
	}
}

// TestLedger_EXEC01_2x: 2.x let an attached short-option value skip the
// secret-file check, so a secret reached through -f<file> was read while
// the separated -f <file> was refused. 3.0: EXEC-01 fixed, both are refused.
func TestLedger_EXEC01_2x(t *testing.T) {
	cwd := t.TempDir()
	// "kubeconfig" is secret by exact name; prefixed with "-f" its base name
	// matches no secret pattern, which is how the attached form slipped by.
	mustWrite(t, filepath.Join(cwd, "kubeconfig"), "x\n")
	mustWrite(t, filepath.Join(cwd, "notes.txt"), "x\n")
	assertRefuse(t, "grep -fkubeconfig notes.txt", cwd, "secret_file_blocked")
	assertRefuse(t, "grep -f kubeconfig notes.txt", cwd, "secret_file_blocked")
}

// TestLedger_EXEC01: an attached short-option value, alone or at the end of
// a cluster, gets both the secret-file check (secret_file_blocked, FATAL)
// and the forbidden-path check (forbidden_path, FATAL), for a command the
// walk tables list and for one they do not. Plain clusters still run.
func TestLedger_EXEC01(t *testing.T) {
	cwd := t.TempDir()
	mustWrite(t, filepath.Join(cwd, "kubeconfig"), "x\n")
	mustWrite(t, filepath.Join(cwd, ".env"), "x\n")
	mustWrite(t, filepath.Join(cwd, "notes.txt"), "x\n")
	for line, want := range map[string]string{
		"grep -fkubeconfig notes.txt":   "secret_file_blocked",
		"grep -if.env notes.txt":        "secret_file_blocked",
		"grep -ivf.env notes.txt":       "secret_file_blocked",
		"rg -f.env x notes.txt":         "secret_file_blocked",
		"jq -nrf.env":                   "secret_file_blocked",
		"grep -f/etc/shadow notes.txt":  "forbidden_path",
		"grep -if/etc/shadow notes.txt": "forbidden_path",
		"grep -vf/proc/1/environ x":     "forbidden_path",
		"cat -A/root/x":                 "forbidden_path",
	} {
		assertRefuse(t, line, cwd, want)
	}
	plan, err := build(t, "grep -f/etc/shadow notes.txt", cwd)
	if r, ok := err.(*Refusal); plan != nil || !ok || r.Details != "/etc/shadow" || r.Action != "FATAL" {
		t.Errorf("grep -f/etc/shadow: got %v, want forbidden_path with details /etc/shadow", err)
	}
	plan, err = build(t, "grep -ivfkubeconfig notes.txt", cwd)
	if r, ok := err.(*Refusal); plan != nil || !ok || r.Code != "secret_file_blocked" || r.Details != "kubeconfig" || r.Action != "FATAL" {
		t.Errorf("grep -ivfkubeconfig: got %v, want secret_file_blocked with details kubeconfig", err)
	}
	for _, line := range []string{"grep -in x notes.txt", "ls -la", "head -n1 notes.txt", "grep -C2 x notes.txt"} {
		assertOK(t, line, cwd)
	}
}

// TestLedger_EXEC02_2x: `git stash list -p` is allowed and applies no secret
// pathspecs, so it leaks (LEDGER EXEC-02, reproduced in rc.A).
func TestLedger_EXEC02_2x(t *testing.T) {
	repo := setupRepo(t)
	if _, err := Build("git stash list -p", Settings{}, testEnv(repo), permissiveProbe()); err != nil {
		t.Errorf("EXEC-02: git stash list -p must not be refused in rc.A, got %v", err)
	}
}

func gitRevParse(t *testing.T, repo, rev string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", rev)
	cmd.Dir = repo
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}
