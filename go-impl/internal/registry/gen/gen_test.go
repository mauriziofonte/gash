// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const repoRoot = "../../../.."

func needs2x(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(repoRoot, "lib/core/help.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
}

// run2x sources the 2.x gash.sh of the checkout in a scrubbed headless
// (or interactive) shell with a temporary HOME and runs script.
func run2x(t *testing.T, interactive bool, script string) []byte {
	t.Helper()
	root, err := filepath.Abs(repoRoot)
	require.NoError(t, err)
	args := []string{"--noprofile", "--norc"}
	env := []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LANG=C.UTF-8", "TERM=dumb"}
	if interactive {
		args = append(args, "-i")
	} else {
		env = append(env, "GASH_HEADLESS=1")
	}
	args = append(args, "-c", `source "$1/gash.sh" >/dev/null 2>&1; `+script, "_", root)
	cmd := exec.Command("bash", args...)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	require.NoError(t, cmd.Run())
	return out.Bytes()
}

// The extracted help entries are byte for byte what 2.x registers.
func TestHelpMatchesThe2xRegistry(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	out := run2x(t, false, `for f in "${!__GASH_HELP_REGISTRY[@]}"; do
		printf '%s\0%s\0%s\0%s\0%s\0%s\0' "$f" "${__GASH_HELP_REGISTRY[$f]}" "${__GASH_HELP_SHORT[$f]-}" \
			"${__GASH_HELP_ALIASES[$f]-}" "${__GASH_HELP_MODULE[$f]-}" "${__GASH_HELP_SEE_ALSO[$f]-}"
	done`)
	fields := strings.Split(string(out), "\x00")
	fields = fields[:len(fields)-1]
	require.Equal(t, 0, len(fields)%6)
	seen := map[string]bool{}
	for i := 0; i < len(fields); i += 6 {
		name := fields[i]
		seen[name] = true
		h, ok := c.helps[name]
		require.True(t, ok, "2.x registers help for %s, gen does not", name)
		require.Equal(t, fields[i+1], h.body, name)
		require.Equal(t, fields[i+2], h.short, name)
		require.Equal(t, fields[i+3], h.aliases, name)
		require.Equal(t, fields[i+4], h.module, name)
		require.Equal(t, fields[i+5], h.seeAlso, name)
	}
	for name := range c.helps {
		require.True(t, seen[name], "gen registers help for %s, 2.x does not", name)
	}
}

// The public functions are those that `declare -F` lists after a 2.x load.
func TestFunctionsMatchDeclareF(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	fns, err := c.build()
	require.NoError(t, err)
	public := func(b []byte) []string {
		var out []string
		for _, l := range strings.Split(strings.TrimSpace(string(b)), "\n") {
			name := strings.TrimPrefix(l, "declare -f ")
			if !strings.HasPrefix(name, "_") {
				out = append(out, name)
			}
		}
		sort.Strings(out)
		return out
	}
	var headless, all []string
	for _, f := range fns {
		all = append(all, f.Name)
		if f.Headless {
			headless = append(headless, f.Name)
		}
	}
	require.Equal(t, headless, public(run2x(t, false, "declare -F")))
	// Interactive: every function but the nvm stub, which 2.x defines only
	// with GASH_NVM=lazy and an nvm install.
	got := public(run2x(t, true, "declare -F"))
	var want []string
	for _, n := range all {
		if n != "nvm" {
			want = append(want, n)
		}
	}
	require.Equal(t, want, got)
}

// Module short aliases are the aliases 2.x defines in headless mode.
func TestModuleAliasesMatch2x(t *testing.T) {
	needs2x(t)
	c, err := extract(repoRoot)
	require.NoError(t, err)
	var want []string
	for _, a := range c.aliases {
		if a.pack == "" && a.condition == "" {
			want = append(want, "alias "+a.name+"='"+a.function+"'")
		}
	}
	sort.Strings(want)
	var got []string
	for _, l := range strings.Split(strings.TrimSpace(string(run2x(t, false, "alias"))), "\n") {
		if !strings.HasPrefix(l, "alias hg=") { // conditional on Mercurial
			got = append(got, l)
		}
	}
	sort.Strings(got)
	require.Equal(t, want, got)
}
