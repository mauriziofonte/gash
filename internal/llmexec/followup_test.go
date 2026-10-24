// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFollowup_F05: inside llm_exec, grep, rg, diff and git grep search the
// secret exceptions (.env.example, id_rsa.pub, ...) as the path guard
// allows, and still skip the secret files (.env, .env.local, id_rsa).
func TestFollowup_F05(t *testing.T) {
	home := t.TempDir()
	cwd := filepath.Join(home, "p")
	searched := []string{"app.php", ".env.example", ".ENV.sample", ".env.dist", "id_rsa.pub", "id_ed25519.pub", "app.env.example"}
	skipped := []string{".env", ".env.local", ".env.examples", "id_rsa", "id_ed25519", "server.key", "credentials.json"}
	other := filepath.Join(home, "q")
	for _, d := range []string{cwd, other} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, n := range append(append([]string{}, searched...), skipped...) {
		mustWrite(t, filepath.Join(cwd, n), "needle "+n+"\n")
	}
	env := testEnv(cwd)
	env.Home = home
	env.Vars = map[string]string{"PATH": os.Getenv("PATH"), "HOME": home}
	run := func(line, dir string) string {
		t.Helper()
		plan, err := Build(line, Settings{}, env, osProbe{})
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		out, _ := runPlan(t, plan, dir)
		return out
	}
	// names is the set of base names the output mentions, one per line.
	check := func(line, out string) {
		t.Helper()
		got := map[string]bool{}
		for _, l := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
			for _, f := range strings.Fields(l) {
				got[filepath.Base(strings.TrimSuffix(f, ":"))] = true
			}
		}
		for _, n := range searched {
			if !got[n] {
				t.Errorf("%q: %s not searched; out=%q", line, n, out)
			}
		}
		for _, n := range skipped {
			if got[n] {
				t.Errorf("%q: secret %s searched; out=%q", line, n, out)
			}
		}
	}
	check("grep -rl needle .", run("grep -rl needle .", cwd))
	check("grep -l needle FILES", run("grep -l needle *.php .env.example .ENV.sample .env.dist id_rsa.pub id_ed25519.pub app.env.example", cwd))
	if _, err := exec.LookPath("rg"); err == nil {
		check("rg --hidden -l needle", run("rg --hidden -l needle", cwd))
	}

	// diff -r names the differing files it compares.
	for _, n := range append(append([]string{}, searched...), skipped...) {
		mustWrite(t, filepath.Join(other, n), "other "+n+"\n")
	}
	check("diff -rq p q", run("diff -rq p q", home))

	if _, err := exec.LookPath("git"); err == nil {
		g := func(args ...string) {
			cmd := exec.Command("git", args...)
			cmd.Dir = cwd
			cmd.Env = []string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null"}
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		g("init", "-q")
		g("-c", "user.email=t@t", "-c", "user.name=t", "add", "-A")
		g("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "one")
		check("git grep -l needle", run("git grep -l needle", cwd))
	}
}
