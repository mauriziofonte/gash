// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// build is a test shortcut for Build with the permissive probe.
func build(t *testing.T, line, cwd string) (*Plan, error) {
	t.Helper()
	return Build(line, Settings{}, testEnv(cwd), permissiveProbe())
}

func assertRefuse(t *testing.T, line, cwd, want string) {
	t.Helper()
	_, err := build(t, line, cwd)
	if got := errCode(err); got != want {
		t.Errorf("llm_exec %q: got error %q, want %q", line, got, want)
	}
}

func assertOK(t *testing.T, line, cwd string) {
	t.Helper()
	if _, err := build(t, line, cwd); err != nil {
		t.Errorf("llm_exec %q: unexpected refusal %v", line, err)
	}
}

// TestRefuse_Constructs: llm_exec_security_spec.sh:30.
func TestRefuse_Constructs(t *testing.T) {
	cwd := t.TempDir()
	cases := map[string]string{
		"echo $HOME":          "construct_not_allowed",
		`echo "$(id)"`:        "construct_not_allowed",
		"echo `id`":           "construct_not_allowed",
		"echo x > out":        "construct_not_allowed",
		"cat < /etc/hostname": "construct_not_allowed",
		"sleep 5 &":           "construct_not_allowed",
		"(ls)":                "construct_not_allowed",
		"{rm,-rf,x}":          "construct_not_allowed",
		"FOO=1 ls":            "construct_not_allowed",
		"rm -f x":             "command_not_allowed",
		"/bin/rm x":           "command_not_allowed",
		"ls | xargs rm":       "command_not_allowed",
		"echo 'open":          "unterminated_quote",
	}
	for line, want := range cases {
		assertRefuse(t, line, cwd, want)
	}
	if _, err := build(t, "echo x > out", cwd); err == nil {
		t.Fatal("expected refusal")
	}
	if _, err := os.Stat(filepath.Join(cwd, "out")); err == nil {
		t.Error("out must not be created")
	}
}

// TestRefuse_WritingOptions: llm_exec_security_spec.sh:55.
func TestRefuse_WritingOptions(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "keep.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"find . -delete", "find . '-exec' rm {} ;", "find . -dele\\te",
		"sort -o out keep.txt", "sort -ro out keep.txt", "sed -i s/a/b/ keep.txt",
		"uniq keep.txt out", "date -s now", "git branch -D main", "git branch newname",
		"git -c core.pager=sh log", "git config user.name x", "git push", "git stash",
		"systemctl stop nginx", "docker rm x", "php -r 1", "crontab -r",
	}
	for _, line := range lines {
		assertRefuse(t, line, cwd, "option_not_allowed")
	}
	if _, err := os.Stat(filepath.Join(cwd, "out")); err == nil {
		t.Error("out must not be created")
	}
}

// TestGlobs: llm_exec_security_spec.sh:75.
func TestGlobs(t *testing.T) {
	cwd := t.TempDir()
	for _, n := range []string{"-delete", "a.log"} {
		if err := os.WriteFile(filepath.Join(cwd, n), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	assertRefuse(t, "find * -name x", cwd, "glob_matches_option")
	assertOK(t, "ls ./*", cwd)
	if err := os.WriteFile(filepath.Join(cwd, "prod.ENV"), []byte("S=1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	assertRefuse(t, "cat *.ENV", cwd, "secret_file_blocked")
}

// TestWalk_FollowSymlinks: llm_exec_walk_spec.sh:40.
func TestWalk_FollowSymlinks(t *testing.T) {
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"grep -R x p", "grep -rnR x p", "grep --dereference-recursive x p",
		"rg -L x p", "rg --follow x p", "find -L p", "find p -follow",
		"find p -files0-from list", "ls -RL p", "tree -l p", "du -L p", "fd -L x p",
	}
	for _, line := range lines {
		assertRefuse(t, line, cwd, "option_not_allowed")
	}
	assertOK(t, "ls -L p && echo ok", cwd)
}

// TestWalk_Roots: llm_exec_walk_spec.sh:57.
func TestWalk_Roots(t *testing.T) {
	cwd := t.TempDir()
	if err := os.MkdirAll(filepath.Join(cwd, "p", "d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/", filepath.Join(cwd, "p", "rootlink")); err != nil {
		t.Fatal(err)
	}
	lines := []string{
		"grep -r PASS /", "grep -r PASS /etc", "grep -d recurse PASS /", "rg PASS /",
		"rg -e PASS /etc", "find / -name x", "ls -R /", "du -sh /", "tree /", "fd x /",
		"fd x --search-path /", "diff -r / p", "ls -R p/rootlink",
		"cd / && grep -r PASS", "cd / && find . -name x", "cd / && rg PASS",
	}
	for _, line := range lines {
		assertRefuse(t, line, cwd, "forbidden_path")
	}
	// Allowed starts.
	assertOK(t, "find /etc -maxdepth 0", cwd)
	assertOK(t, "ls / >/dev/null", cwd)
}

// TestForbiddenPaths: llm_exec_security_spec.sh:212.
func TestForbiddenPaths(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "notes.txt"), []byte("see /proc/cpuinfo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink("/proc/self", filepath.Join(cwd, "me"))
	lines := []string{
		"head -c 50 /proc/self/environ", "cat /proc/self/cmdline", "ls /sys",
		"cat /root/.bashrc", "head /etc/shadow", "cat me/environ",
		"cd / && cat proc/self/environ", "cd / && ls proc", "cd /etc && cat shadow",
		"grep -f/proc/self/environ x", "ls --directory=/sys", "git -C /root status",
		"cat ~root/.profile", "wc -c /dev/sda",
	}
	for _, line := range lines {
		assertRefuse(t, line, cwd, "forbidden_path")
	}
	// /dev/null stays usable; patterns and plain text are not paths.
	assertOK(t, "cat /dev/null && echo ok", cwd)
	assertOK(t, "ls nothing 2>/dev/null || echo none", cwd)
	assertOK(t, "echo /proc/x", cwd)
	assertOK(t, "grep /proc/ notes.txt", cwd)
	assertOK(t, "cat ./notes.txt", cwd)
}

// TestForbiddenPaths_Details checks the forbidden envelope carries the
// original word, not the resolved path (llm_exec_security_spec.sh:212).
func TestForbiddenPaths_Details(t *testing.T) {
	cwd := t.TempDir()
	_, err := build(t, "cat /proc/self/environ", cwd)
	r, ok := err.(*Refusal)
	if !ok {
		t.Fatalf("expected refusal, got %v", err)
	}
	if r.Code != "forbidden_path" || r.Action != "FATAL" || r.Details != "/proc/self/environ" {
		t.Errorf("got %+v", r)
	}
}

// TestYqGetent: llm_exec_walk_spec.sh:176.
func TestYqGetent(t *testing.T) {
	cwd := t.TempDir()
	for _, line := range []string{
		`yq -n 'load("x.yml")'`, `yq '.a = load_str("f")' a.yml`, "yq -s .a a.yml", "yq --split-exp .a a.yml",
	} {
		assertRefuse(t, line, cwd, "option_not_allowed")
	}
	assertRefuse(t, "getent shadow", cwd, "forbidden_path")
	assertRefuse(t, "getent gshadow root", cwd, "forbidden_path")
}

// TestPsEnv: ps e variants are refused (llm_exec_walk_spec.sh:154).
func TestPsEnv(t *testing.T) {
	cwd := t.TempDir()
	assertRefuse(t, "ps eww", cwd, "option_not_allowed")
	assertRefuse(t, "ps auxe", cwd, "option_not_allowed")
	assertOK(t, "ps -o pid,etime -p 1", cwd)
}

// A refused command that changes files or processes points at the agent_*
// functions; any other refused command at the allowlist (3.4).
func TestRefuse_HintOfWriteCommands(t *testing.T) {
	cwd := t.TempDir()
	for line, want := range map[string]string{
		"rm -rf src":     "'rm' changes files or processes. Make the change with an agent_* function",
		"chmod +x a":     "'chmod' changes files or processes",
		"make build":     "Ask the user to add 'make' to GASH_LLM_EXEC_ALLOW",
		"terraform plan": "Ask the user to add 'terraform' to GASH_LLM_EXEC_ALLOW",
	} {
		_, err := build(t, line, cwd)
		r, ok := err.(*Refusal)
		if !ok || r.Code != "command_not_allowed" {
			t.Fatalf("llm_exec %q: got %v, want command_not_allowed", line, err)
		}
		if !strings.Contains(r.Hint, want) {
			t.Errorf("llm_exec %q: hint %q lacks %q", line, r.Hint, want)
		}
		if writeCommands[strings.Fields(line)[0]] && strings.Contains(r.Hint, "GASH_LLM_EXEC_ALLOW") {
			t.Errorf("llm_exec %q: the hint suggests allowing a write command", line)
		}
	}
}
