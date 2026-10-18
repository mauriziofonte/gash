// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// runAnswering is runMerged with a controlling terminal that types the
// lines of answers, one per question.
func runAnswering(t *testing.T, name string, env map[string]string, color bool, answers string, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdout: &out, Stderr: &out,
		Term: term.Fixed(e, false, color, false).WithTTYAnswers(strings.SplitAfter(answers, "\n")...)}
	rc := Table().Run(c)
	return out.String(), rc
}

// A fake install in $D/home/.gash, the profiles' Gash blocks.
const (
	uninstallFake  = `mkdir -p $D/home/.gash/lib/core; : >$D/home/.gash/gash.sh; echo 'source ~/.gash/gash.sh' >$D/home/.gashrc; `
	uninstallBlock = `printf '# before\n# >>> GASH START >>>\nif [ -f ~/.gashrc ]; then\n  source ~/.gashrc\nfi\n# <<< GASH END <<<\n# after\n' `
	uninstall1x    = `printf '# before\n# Load Gash Bash\n# This loads the Gash customized Bash.\nif [ -f "$HOME/.gashrc" ]; then\n  source "$HOME/.gashrc"\nfi\n# after\n' `
)

var uninstallCases = []struct {
	name, setup, answers string
	vars                 map[string]string
	args                 []string
}{
	{"not installed", "", "y\n", map[string]string{"GASH_DIR": "$D/none"}, nil},
	{"not gash", "mkdir -p $D/other", "y\n", map[string]string{"GASH_DIR": "$D/other"}, nil},
	{"home", "mkdir -p $D/home/lib/core; : >$D/home/gash.sh", "y\n", map[string]string{"GASH_DIR": "$D/home"}, nil},
	{"cancel", uninstallFake + uninstallBlock + ">$D/home/.bashrc", "n\n", nil, nil},
	{"no answer", uninstallFake, "", nil, nil},
	{"confirm", uninstallFake + uninstallBlock + ">$D/home/.bashrc; echo K=v >$D/home/.gash_env", "y\ny\n", nil, nil},
	{"keep env", uninstallFake + uninstallBlock + ">$D/home/.bashrc; echo K=v >$D/home/.gash_env", "y\nn\n", nil, nil},
	{"1.x block", uninstallFake + uninstall1x + ">$D/home/.bashrc", "y\n", nil, nil},
	{"1.x block at end", uninstallFake + `printf 'a\n# Load Gash Bash\nsource ~/.gashrc\nb\n' >$D/home/.bash_profile`, "y\n", nil, nil},
	{"two blocks", uninstallFake + uninstallBlock + ">$D/home/.profile; " + uninstallBlock + ">>$D/home/.profile", "y\n", nil, nil},
	{"no end marker", uninstallFake + `printf 'a\n# >>> GASH START >>>\nsource ~/.gashrc\nb\n' >$D/home/.bashrc`, "y\n", nil, nil},
	{"unmarked", uninstallFake + `echo '. /x/.gashrc' >$D/home/.bashrc; echo 'source ~/.gashrc' >$D/home/.profile`, "y\n", nil, nil},
	{"no block", uninstallFake + `echo 'alias x=y' >$D/home/.bashrc`, "y\n", nil, nil},
	{"all profiles", uninstallFake + uninstallBlock + ">$D/home/.bashrc; " + uninstall1x + ">$D/home/.bash_profile; " +
		uninstallBlock + ">$D/home/.profile; chmod 640 $D/home/.profile", "y\n", nil, nil},
	{"linked profile", uninstallFake + "mkdir $D/dot; " + uninstallBlock + ">$D/dot/bashrc; chmod 600 $D/dot/bashrc; " +
		"ln -s ../dot/bashrc $D/home/.bashrc", "y\n", nil, nil},
	{"env file elsewhere", uninstallFake + "echo K=v >$D/conf.env", "y\ny\n", map[string]string{"GASH_ENV_FILE": "$D/conf.env"}, nil},
	{"env file in home", uninstallFake + "mkdir $D/home/cfg; echo K=v >$D/home/cfg/env", "y\nn\n",
		map[string]string{"GASH_ENV_FILE": "$D/home/cfg/env"}, nil},
	{"linked install", "mkdir -p $D/real/lib/core $D/home; : >$D/real/gash.sh; ln -s ../real $D/home/.gash", "y\n", nil, nil},
	{"custom dir", "mkdir -p $D/opt/g/lib/core $D/home; : >$D/opt/g/gash.sh", "y\n", map[string]string{"GASH_DIR": "$D/opt/g"}, nil},
	{"backup is a dir", uninstallFake + uninstallBlock + ">$D/home/.profile; mkdir $D/home/.profile.gash-uninstall.bak", "y\n", nil, nil},
	{"no gashrc", "mkdir -p $D/home/.gash/lib/core; : >$D/home/.gash/gash.sh", "y\n", nil, nil},
	{"headless", uninstallFake, "y\n", map[string]string{"GASH_HEADLESS": "1"}, nil},
	{"extra arguments", uninstallFake, "y\n", nil, []string{"--bogus", "x"}},
	{"help", uninstallFake, "y\n", nil, []string{"-h"}},
}

// uninstallState lists the case directory: every path with its type and
// mode, link targets and file contents.
const uninstallState = `cd "$D" && find . -mindepth 1 | LC_ALL=C sort | while read -r p; do ` +
	`if [[ -L $p ]]; then echo "$p -> $(readlink "$p")"; elif [[ -d $p ]]; then echo "$p/"; ` +
	`else echo "$p $(stat -c %a "$p")"; sed 's/^/  | /' "$p"; fi; done`

// gash_uninstall does what 2.x does: the same messages and questions, the
// same answers taken, status and files left.
func TestUninstallMatches2x(t *testing.T) {
	for _, mode := range []string{"off", "on"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ref, got := t.TempDir(), t.TempDir()
			var script strings.Builder
			if mode == "off" {
				script.WriteString("__gash_use_color() { return 1; }\n")
			} else {
				script.WriteString(`__gash_use_color() { [[ "${GASH_HEADLESS-}" != 1 ]]; }` + "\n")
			}
			// The terminal's answers come from fd 7.
			script.WriteString(`needs_confirm_prompt() { local r; [[ "${GASH_HEADLESS-}" == "1" ]] && return 1; ` +
				`__gash_ask_render "${1-}" "y/N"; read -r r <&7 || return 1; [[ "$r" =~ ^[Yy]$ ]]; }` + "\n")
			for i, tc := range uninstallCases {
				for _, base := range []string{ref, got} {
					d := filepath.Join(base, strconv.Itoa(i))
					require.NoError(t, os.MkdirAll(d, 0o755))
					bash(t, "D="+shq(d)+"; "+tc.setup)
				}
				d := filepath.Join(ref, strconv.Itoa(i))
				answers := filepath.Join(ref, "answers"+strconv.Itoa(i))
				require.NoError(t, os.WriteFile(answers, []byte(tc.answers), 0o644))
				// GASH_DIR as the stub leaves it: unset unless the case sets it
				// (gash.sh set it to the 2.x copy, which must stay).
				script.WriteString("(unset GASH_DIR; D=" + shq(d) + "; HOME=$D/home; ")
				for k, v := range tc.vars {
					script.WriteString(k + "=\"" + v + "\"; ")
				}
				script.WriteString("gash_uninstall")
				for _, a := range tc.args {
					script.WriteString(" " + shq(a))
				}
				script.WriteString(" 7<" + shq(answers) + " 2>&1; printf '\\0%s\\0' $?)\n")
			}
			out, _, _ := run2xIn(t, copy2x(t), []string{"LANG=C.UTF-8"}, script.String())
			fields := strings.Split(out, "\x00")
			require.Len(t, fields, 2*len(uninstallCases)+1, out)
			for i, tc := range uninstallCases {
				dRef, d := filepath.Join(ref, strconv.Itoa(i)), filepath.Join(got, strconv.Itoa(i))
				env := map[string]string{"LANG": "C.UTF-8", "PATH": "/usr/bin:/bin", "HOME": d + "/home"}
				for k, v := range tc.vars {
					env[k] = strings.ReplaceAll(v, "$D", d)
				}
				o, rc := runAnswering(t, "gash_uninstall", env, mode == "on", tc.answers, tc.args...)
				require.Equal(t, strings.ReplaceAll(fields[2*i], dRef, d), o, tc.name)
				require.Equal(t, fields[2*i+1], strconv.Itoa(rc), tc.name)
				require.Equal(t, strings.ReplaceAll(bash(t, "D="+shq(dRef)+"; "+uninstallState), dRef, d),
					bash(t, "D="+shq(d)+"; "+uninstallState), tc.name)
			}
		})
	}
}

// uninstallFixture runs setup in a new case directory and returns it with
// the call's environment.
func uninstallFixture(t *testing.T, setup string) (string, map[string]string) {
	t.Helper()
	d := t.TempDir()
	bash(t, "D="+shq(d)+"; "+setup)
	return d, map[string]string{"LANG": "C.UTF-8", "PATH": "/usr/bin:/bin", "HOME": d + "/home"}
}

// LEDGER SELF-02, reproduced in rc.A: a confirmed uninstall removes a
// ~/.gashrc holding the user's own lines without a copy, and the profile
// backups sed made are gone at the end.
func TestLedger_SELF02_2x(t *testing.T) {
	d, env := uninstallFixture(t, uninstallFake+"echo \"alias mine='echo mine'\" >>$D/home/.gashrc; "+uninstallBlock+">$D/home/.bashrc")
	_, rc := runAnswering(t, "gash_uninstall", env, false, "y\n")
	require.Equal(t, 0, rc)
	for _, gone := range []string{".gashrc", ".gashrc.gash-uninstall.bak", ".bashrc.gash-uninstall.bak", ".gash"} {
		_, err := os.Lstat(filepath.Join(d, "home", gone))
		require.True(t, os.IsNotExist(err), gone)
	}
	b, err := os.ReadFile(filepath.Join(d, "home/.bashrc"))
	require.NoError(t, err)
	require.Equal(t, "# before\n# after\n", string(b))
}

// LEDGER SELF-04, reproduced in rc.A: a linked ~/.bashrc becomes an edited
// regular file, and the link's target keeps the Gash block.
func TestLedger_SELF04_2x(t *testing.T) {
	d, env := uninstallFixture(t, uninstallFake+"mkdir $D/dot; "+uninstallBlock+">$D/dot/bashrc; ln -s ../dot/bashrc $D/home/.bashrc")
	_, rc := runAnswering(t, "gash_uninstall", env, false, "y\n")
	require.Equal(t, 0, rc)
	st, err := os.Lstat(filepath.Join(d, "home/.bashrc"))
	require.NoError(t, err)
	require.True(t, st.Mode().IsRegular(), "the link was replaced")
	b, err := os.ReadFile(filepath.Join(d, "home/.bashrc"))
	require.NoError(t, err)
	require.Equal(t, "# before\n# after\n", string(b))
	b, err = os.ReadFile(filepath.Join(d, "dot/bashrc"))
	require.NoError(t, err)
	require.Contains(t, string(b), "# >>> GASH START >>>")
}
