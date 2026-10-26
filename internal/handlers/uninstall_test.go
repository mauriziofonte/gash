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

// uninstall3x names the cases where 3.0 no longer matches 2.x. 3.0: SELF-04
// fixed: a linked profile stays a link, and a backup path taken by a
// directory leaves the profile unedited with an error (TestLedger_SELF04
// proves both). The help page carries the 3.0 notes on the backups kept
// (SELF-02).
var uninstall3x = map[string]bool{"linked profile": true, "backup is a dir": true, "help": true}

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
				if uninstall3x[tc.name] {
					// 3.0 differs from 2.x here on purpose (see uninstall3x).
					continue
				}
				if tc.name == "extra arguments" {
					// 3.0: SELF-15 fixed: the argument is refused before
					// anything is removed (2.x ignored it and uninstalled).
					require.Contains(t, o, "Unknown option: --bogus (usage: gash_uninstall)", tc.name)
					require.Equal(t, 2, rc, tc.name)
					require.DirExists(t, d+"/home/.gash", tc.name)
					continue
				}
				// 3.0: SELF-02 fixed. A ~/.gashrc with the user's lines is kept
				// as a backup and the profile backups are kept and listed, so
				// those lines, files and the backup-removal status are put back
				// in their 2.x shape; everything else must still match 2.x.
				refOut, refRC := undoSELF02Ref(strings.ReplaceAll(fields[2*i], dRef, d), fields[2*i+1])
				require.Equal(t, refOut, undoSELF02(o), tc.name)
				require.Equal(t, refRC, strconv.Itoa(rc), tc.name)
				bash(t, "D="+shq(d)+"; find \"$D\" -name '*.gash-uninstall.bak' ! -type d -delete")
				require.Equal(t, strings.ReplaceAll(bash(t, "D="+shq(dRef)+"; "+uninstallState), dRef, d),
					bash(t, "D="+shq(d)+"; "+uninstallState), tc.name)
			}
		})
	}
}

// undoSELF02 maps the 3.0 uninstall messages back to 2.x: the ~/.gashrc
// line says "Removing" and no "Backups kept" line is printed.
func undoSELF02(out string) string {
	out = strings.ReplaceAll(out, "Saved your ~/.gashrc as ~/.gashrc.gash-uninstall.bak", "Removing ~/.gashrc...")
	var b strings.Builder
	for _, line := range strings.SplitAfter(out, "\n") {
		if !strings.Contains(line, "Backups kept: ") {
			b.WriteString(line)
		}
	}
	return b.String()
}

// undoSELF02Ref drops what 2.x printed and returned when removing a profile
// backup failed (3.0 no longer removes the backups).
func undoSELF02Ref(out, rc string) (string, string) {
	var b strings.Builder
	dropped := false
	for _, line := range strings.SplitAfter(out, "\n") {
		if strings.HasPrefix(line, "rm: cannot remove '") && strings.Contains(line, ".gash-uninstall.bak': ") {
			dropped = true
			continue
		}
		b.WriteString(line)
	}
	if dropped {
		rc = "0"
	}
	return b.String(), rc
}

// uninstallFixture runs setup in a new case directory and returns it with
// the call's environment.
func uninstallFixture(t *testing.T, setup string) (string, map[string]string) {
	t.Helper()
	d := t.TempDir()
	bash(t, "D="+shq(d)+"; "+setup)
	return d, map[string]string{"LANG": "C.UTF-8", "PATH": "/usr/bin:/bin", "HOME": d + "/home"}
}

// LEDGER SELF-02: a confirmed uninstall keeps a ~/.gashrc holding the
// user's own lines as ~/.gashrc.gash-uninstall.bak and keeps the profile
// backups sed made (3.0: SELF-02 fixed; 2.x removed them all).
func TestLedger_SELF02(t *testing.T) {
	d, env := uninstallFixture(t, uninstallFake+"echo \"alias mine='echo mine'\" >>$D/home/.gashrc; "+uninstallBlock+">$D/home/.bashrc")
	out, rc := runAnswering(t, "gash_uninstall", env, false, "y\n")
	require.Equal(t, 0, rc)
	for _, gone := range []string{".gashrc", ".gash"} {
		_, err := os.Lstat(filepath.Join(d, "home", gone))
		require.True(t, os.IsNotExist(err), gone)
	}
	b, err := os.ReadFile(filepath.Join(d, "home/.gashrc.gash-uninstall.bak"))
	require.NoError(t, err)
	require.Equal(t, "source ~/.gash/gash.sh\nalias mine='echo mine'\n", string(b))
	_, err = os.Stat(filepath.Join(d, "home/.bashrc.gash-uninstall.bak"))
	require.NoError(t, err)
	require.Contains(t, out, "Info: Backups kept: ~/.bashrc.gash-uninstall.bak, ~/.gashrc.gash-uninstall.bak\n")
	b, err = os.ReadFile(filepath.Join(d, "home/.bashrc"))
	require.NoError(t, err)
	require.Equal(t, "# before\n# after\n", string(b))
}

// LEDGER SELF-04: a linked ~/.bashrc stays a link, and the file it points
// to loses the Gash block and keeps its mode (3.0: SELF-04 fixed; 2.x's
// GNU sed -i replaced the link by an edited regular file).
func TestLedger_SELF04(t *testing.T) {
	d, env := uninstallFixture(t, uninstallFake+"mkdir $D/dot; "+uninstallBlock+">$D/dot/bashrc; chmod 640 $D/dot/bashrc; ln -s ../dot/bashrc $D/home/.bashrc")
	_, rc := runAnswering(t, "gash_uninstall", env, false, "y\n")
	require.Equal(t, 0, rc)
	target, err := os.Readlink(filepath.Join(d, "home/.bashrc"))
	require.NoError(t, err, "the link was replaced")
	require.Equal(t, "../dot/bashrc", target)
	b, err := os.ReadFile(filepath.Join(d, "dot/bashrc"))
	require.NoError(t, err)
	require.Equal(t, "# before\n# after\n", string(b))
	st, err := os.Stat(filepath.Join(d, "dot/bashrc"))
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o640), st.Mode().Perm())
	b, err = os.ReadFile(filepath.Join(d, "home/.bashrc.gash-uninstall.bak"))
	require.NoError(t, err)
	require.Contains(t, string(b), "# >>> GASH START >>>")

	// A backup path taken by a directory: the profile is left as it was,
	// with an error. 3.4: the uninstall stops there, Gash still installed
	// (it went on, and the profile loaded a removed Gash).
	d, env = uninstallFixture(t, uninstallFake+uninstallBlock+">$D/home/.profile; mkdir $D/home/.profile.gash-uninstall.bak")
	out, rc := runAnswering(t, "gash_uninstall", env, false, "y\n")
	require.Equal(t, 1, rc)
	require.Contains(t, out, "Error: Failed to edit "+d+"/home/.profile; remove the Gash block yourself.\n"+
		"Error: Gash was not uninstalled: the Gash block is still in "+d+"/home/.profile.\n")
	b, err = os.ReadFile(filepath.Join(d, "home/.profile"))
	require.NoError(t, err)
	require.Contains(t, string(b), "# >>> GASH START >>>")
	require.DirExists(t, filepath.Join(d, "home/.gash"))
}
