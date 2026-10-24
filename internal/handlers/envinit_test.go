// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// envInitCases are gash_env_init calls in a case directory $D: the setup
// (run by Bash), the variables of the call (GASH_DIR=- leaves it unset)
// and the arguments. Every case has $D/home (HOME) and the template
// $D/gash/.gash_env.template.
var envInitCases = []struct {
	name, setup string
	vars        map[string]string
	args        []string
}{
	{"fresh", "", nil, nil},
	{"exists", ": >$D/home/.gash_env", nil, nil},
	{"force", "echo old >$D/home/.gash_env; chmod 644 $D/home/.gash_env", nil, []string{"--force"}},
	{"bogus fresh", "", nil, []string{"--bogus"}},
	{"bogus exists", ": >$D/home/.gash_env", nil, []string{"--bogus"}},
	{"force second", ": >$D/home/.gash_env", nil, []string{"x", "--force"}},
	{"force twice", "", nil, []string{"--force", "--force"}},
	{"no template", "", map[string]string{"GASH_DIR": "$D/nowhere"}, nil},
	{"template dir", "rm $D/gash/.gash_env.template; mkdir $D/gash/.gash_env.template", nil, nil},
	{"no GASH_DIR", "", map[string]string{"GASH_DIR": "-"}, nil},
	{"env file", "", map[string]string{"GASH_ENV_FILE": "$D/conf.env"}, nil},
	{"env file empty", "", map[string]string{"GASH_ENV_FILE": ""}, nil},
	{"env file exists", ": >$D/conf.env", map[string]string{"GASH_ENV_FILE": "$D/conf.env"}, nil},
	{"target dir", "mkdir $D/home/.gash_env", nil, nil},
	{"target dir force", "mkdir $D/home/.gash_env", nil, []string{"--force"}},
	{"dangling link", "ln -s $D/none $D/home/.gash_env", nil, nil},
	{"link exists", "echo old >$D/real; ln -s $D/real $D/home/.gash_env", nil, nil},
	{"link force", "echo old >$D/real; chmod 644 $D/real; ln -s $D/real $D/home/.gash_env", nil, []string{"--force"}},
	{"no parent", "", map[string]string{"GASH_ENV_FILE": "$D/missing/conf.env"}, nil},
	{"same file", "", map[string]string{"GASH_ENV_FILE": "$D/gash/.gash_env.template"}, []string{"--force"}},
	{"help", "", nil, []string{"-h"}},
	{"help long", ": >$D/home/.gash_env", nil, []string{"--help"}},
}

// envInitSELF15 names the cases whose arguments 3.0 refuses (SELF-15),
// with the refusal: rc 2 and nothing written (2.x ignored the words).
var envInitSELF15 = map[string]string{
	"bogus fresh":  "Unknown option: --bogus",
	"bogus exists": "Unknown option: --bogus",
	"force second": "Unexpected argument: x",
}

// envInitState lists the case directory after the call: every path with
// its type, mode and content.
const envInitState = `cd "$D" && find . -mindepth 1 | LC_ALL=C sort | while read -r p; do ` +
	`if [[ -L $p ]]; then echo "$p -> $(readlink "$p")"; elif [[ -d $p ]]; then echo "$p/ $(stat -c %a "$p")"; ` +
	`else echo "$p $(stat -c %a "$p") $(cat "$p")"; fi; done`

func envInitSetup(d, setup string) string {
	return "D=" + shq(d) + "; mkdir -p $D/home $D/gash; echo TEMPLATE >$D/gash/.gash_env.template; " + setup
}

// gash_env_init does what 2.x does: the same messages, status and files,
// colors off and on.
func TestEnvInitMatches2x(t *testing.T) {
	for _, mode := range []string{"off", "on"} {
		t.Run(mode, func(t *testing.T) {
			ref, got := t.TempDir(), t.TempDir()
			// When a case's target is a directory, gash_env_init chmods it
			// to 0600 (as 2.x does), stripping the search bit; a non-root
			// runner then cannot traverse or remove it. Restore owner rwX
			// before t.TempDir's RemoveAll (cleanups run last-in-first-out).
			for _, d := range []string{ref, got} {
				d := d
				t.Cleanup(func() { _ = exec.Command("chmod", "-R", "u+rwX", d).Run() })
			}
			var script strings.Builder
			script.WriteString("unalias -a\n") // the setup's mkdir, cp, rm as written
			if mode == "on" {
				script.WriteString("__gash_use_color() { return 0; }; ")
			} else {
				script.WriteString("__gash_use_color() { return 1; }; ")
			}
			for i, tc := range envInitCases {
				d := filepath.Join(ref, strconv.Itoa(i))
				script.WriteString(envInitSetup(d, tc.setup) + "\n(")
				vars := map[string]string{"HOME": "$D/home", "GASH_DIR": "$D/gash"}
				for k, v := range tc.vars {
					vars[k] = v
				}
				var assign strings.Builder
				for k, v := range vars {
					if v == "-" {
						script.WriteString("unset " + k + "; ")
					} else {
						assign.WriteString(k + "=\"" + v + "\" ")
					}
				}
				script.WriteString(assign.String() + "gash_env_init")
				for _, a := range tc.args {
					script.WriteString(" " + shq(a))
				}
				script.WriteString(") 2>&1; printf '\\0%s\\0' $?; (" + envInitState + "); printf '\\0'\n")
			}
			out, _, _ := run2xIn(t, copy2x(t), []string{"LANG=C.UTF-8"}, script.String())
			fields := strings.Split(out, "\x00")
			require.Len(t, fields, 3*len(envInitCases)+1)
			for i, tc := range envInitCases {
				d := filepath.Join(got, strconv.Itoa(i))
				bash(t, envInitSetup(d, tc.setup))
				env := map[string]string{"LANG": "C.UTF-8", "PATH": "/usr/bin:/bin", "HOME": d + "/home", "GASH_DIR": d + "/gash"}
				for k, v := range tc.vars {
					if v == "-" {
						delete(env, k)
					} else {
						env[k] = strings.ReplaceAll(v, "$D", d)
					}
				}
				o, e, rc := run(t, "gash_env_init", env, mode == "on", tc.args...)
				state := bash(t, "D="+shq(d)+"; "+envInitState)
				norm := func(s, dir string) string { return strings.ReplaceAll(s, dir, "$D") }
				want := filepath.Join(ref, strconv.Itoa(i))
				if refusal, ok := envInitSELF15[tc.name]; ok {
					// 3.0: SELF-15 fixed: refused before anything is written.
					require.Contains(t, o+e, refusal+" (usage: gash_env_init [--force])", tc.name)
					require.Equal(t, 2, rc, tc.name)
					untouched := filepath.Join(t.TempDir(), "x")
					bash(t, envInitSetup(untouched, tc.setup))
					require.Equal(t, norm(bash(t, "D="+shq(untouched)+"; "+envInitState), untouched), norm(state, d), tc.name)
					continue
				}
				require.Equal(t, norm(fields[3*i], want), norm(o+e, d), tc.name)
				require.Equal(t, fields[3*i+1], strconv.Itoa(rc), tc.name)
				require.Equal(t, norm(fields[3*i+2], want), norm(state, d), tc.name)
			}
		})
	}
}

// bash runs a setup or listing script and returns its stdout.
func bash(t *testing.T, script string) string {
	t.Helper()
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C.UTF-8"}
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	require.NoError(t, err, script)
	return string(out)
}
