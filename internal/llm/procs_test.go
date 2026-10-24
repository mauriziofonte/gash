// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// llm34Helper is set in the child that runs llm_procs from inside the
// marked process chain of TestLedger_LLM34.
const llm34Helper = "GASH_TEST_LLM34_HELPER"

// TestLedger_LLM34: the binary forks no helper shells, so a NAME only the
// caller's chain matches gives [] on every run. 2.x could list its own
// exiting helper subshell with an empty name.
func TestLedger_LLM34(t *testing.T) {
	if os.Getenv(llm34Helper) == "1" {
		llm34Child(t)
		return
	}
	for _, tool := range []string{"bash", "ps", "pgrep", "sleep"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	exe, err := os.Executable()
	require.NoError(t, err)
	raw := make([]byte, 8)
	_, err = rand.Read(raw)
	require.NoError(t, err)
	marker := "gash-llm34-" + hex.EncodeToString(raw)
	home := physTemp(t)

	// The chain: this test, a marked bash, the command-substitution
	// subshell bash forks, and the child test binary (marked too).
	chain := func(runs int) []string {
		script := `i=0; while [ "$i" -lt "$2" ]; do r=$("$0" -test.run='^TestLedger_LLM34$' "$1"); printf '%s\n' "$r"; i=$((i+1)); done`
		cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script, exe, marker, fmt.Sprint(runs))
		cmd.Env = append(os.Environ(), llm34Helper+"=1", "GASH_TEST_LLM34_HOME="+home)
		cmd.Dir = home
		var out, errb bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errb
		require.NoError(t, cmd.Run(), errb.String())
		require.Empty(t, errb.String())
		return strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	}
	runs := chain(25)
	require.Len(t, runs, 25)
	for _, r := range runs {
		require.Equal(t, "[]", r)
	}

	// Control: a marked process outside the chain is listed.
	sib := exec.Command("bash", "--noprofile", "--norc", "-c", "sleep 30; true", "bash", marker)
	require.NoError(t, sib.Start())
	t.Cleanup(func() {
		_ = sib.Process.Kill()
		_ = sib.Wait()
	})
	want := fmt.Sprintf(`[{"pid":%d,"name":"bash"}]`, sib.Process.Pid)
	require.Equal(t, []string{want}, chain(1))
}

// llm34Child runs llm_procs --name MARKER and exits with its status before
// the test framework prints anything.
func llm34Child(t *testing.T) {
	marker := flag.Arg(0)
	if marker == "" {
		t.Fatal("no marker")
	}
	wd, err := os.Getwd()
	require.NoError(t, err)
	env := map[string]string{"PATH": os.Getenv("PATH"), "HOME": os.Getenv("GASH_TEST_LLM34_HOME"), "LC_ALL": "C", "GASH_HEADLESS": "1"}
	r := runGo(t, New(), wd, env, "llm_procs", "--name", marker)
	_, _ = os.Stdout.WriteString(r.Stdout)
	_, _ = os.Stderr.WriteString(r.Stderr)
	os.Exit(r.RC)
}
