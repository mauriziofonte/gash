// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/compatgen"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
	"github.com/mauriziofonte/gash/internal/term"
)

// runMerged is run with stdout and stderr in one stream, as 2>&1 gives
// them.
func runMerged(t *testing.T, name string, env map[string]string, tty bool, args ...string) (string, int) {
	t.Helper()
	var out bytes.Buffer
	e := invoke.EnvMap(env)
	c := &invoke.Call{Name: name, Args: args, Env: e, Stdout: &out, Stderr: &out, Term: term.Fixed(e, false, tty, false)}
	rc := Table().Run(c)
	return out.String(), rc
}

// doctor2xFiles are the files the 2.x doctor checks.
var doctor2xFiles = []string{
	"gash.sh", "lib/core/output.sh", "lib/core/help.sh", "lib/core/utils.sh", "lib/core/validation.sh", "lib/core/config.sh",
	"lib/shell/terminal.sh", "lib/shell/history.sh", "lib/shell/prompt.sh", "lib/shell/banner.sh", "lib/shell/toolchains.sh",
	"lib/aliases/_loader.sh",
	"lib/modules/gash.sh", "lib/modules/git.sh", "lib/modules/ssh.sh", "lib/modules/files.sh", "lib/modules/docker.sh",
	"lib/modules/docker-compose.sh", "lib/modules/system.sh", "lib/modules/llm.sh", "lib/modules/ai.sh",
	"lib/modules/sysinfo.sh", "lib/modules/remote.sh", "lib/modules/node.sh", "lib/modules/php.sh",
}

// doctorInstall makes an install directory that holds the files of both
// file lists, and a bin directory with fake git and jq (and stat, which
// the 2.x doctor runs).
func doctorInstall(t *testing.T, d string) {
	t.Helper()
	files := append(append([]string{}, doctor2xFiles...), selfmgmt.DoctorCoreFiles...)
	for _, m := range selfmgmt.DoctorModules {
		files = append(files, "lib/modules/"+m+".sh")
	}
	for _, f := range files {
		p := filepath.Join(d, "inst", f)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, nil, 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(d, "bin"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(d, "home"), 0o755))
	for _, tool := range []string{"git", "jq"} {
		require.NoError(t, os.WriteFile(filepath.Join(d, "bin", tool), []byte("#!/bin/sh\n"), 0o755))
	}
	stat := "/usr/bin/stat"
	if _, err := os.Stat(stat); err != nil {
		stat = "/bin/stat"
	}
	require.NoError(t, os.Symlink(stat, filepath.Join(d, "bin", "stat")))
}

// doctorCases are gash_doctor calls: a shell prefix run in a subshell
// before the call ($D is the case directory) and the arguments.
var doctorCases = []struct {
	name, prefix string
	args         []string
}{
	{"plain", "", nil},
	{"env 600", "echo x >$D/home/.gash_env; chmod 600 $D/home/.gash_env", nil},
	{"env 400", "echo x >$D/home/.gash_env; chmod 400 $D/home/.gash_env", nil},
	{"env 644", "echo x >$D/home/.gash_env; chmod 644 $D/home/.gash_env", nil},
	{"env 4600", "echo x >$D/home/.gash_env; chmod 4600 $D/home/.gash_env", nil},
	{"env dir", "mkdir -p $D/home/.gash_env", nil},
	{"env link", "echo x >$D/real; chmod 600 $D/real; ln -sf $D/real $D/link; GASH_ENV_FILE=$D/link", nil},
	{"env file var", "GASH_ENV_FILE=$D/nowhere/env", nil},
	{"settings", `export GASH_DB_TIMEOUT=5; GASH_PROMPT=0; GASH_LLM_EXEC=unrestricted; GASH_PATH_ADD=$'a\nb'; ` +
		`GASH_UMASK='x\y'; GASH_HISTORY_SYNC=; GASH_NOT_KNOWN=1`, nil},
	{"load errors", `__GASH_LOAD_ERRORS=("lib/modules/broken.sh: exit 2" $'two\nlines' 'back\slash' '')`, nil},
	{"no version", "unset GASH_VERSION", nil},
	{"no-color", "", []string{"--no-color"}},
	{"bogus", "", []string{"--bogus", "x"}},
	{"help", "", []string{"-h"}},
	{"help long", "", []string{"--help"}},
}

// doctorStubLoop is the gash_doctor stub's encoding of the shell state
// (compat/stubs.tmpl), printed for the Go side.
const doctorStubLoop = `__bs='\' __nl=$'\n' __errs="" __set=""
for __v in ${__GASH_LOAD_ERRORS[@]+"${__GASH_LOAD_ERRORS[@]}"}; do
    __v=${__v//"$__bs"/"$__bs$__bs"}
    __errs+=${__v//"$__nl"/"${__bs}n"}$__nl
done
for __n in ${__GASH_SETTINGS_KNOWN-}; do
    [[ -n "${!__n+x}" ]] || continue
    __v=${!__n}
    __v=${__v//"$__bs"/"$__bs$__bs"}
    __set+=$__n$'\t'${__v//"$__nl"/"${__bs}n"}$__nl
done
printf '%s\0%s\0%s\0%s\0%s\0' "$__errs" "$__set" "${GASH_ENV_FILE-}" "${GASH_VERSION-}" "$BASH_VERSION"
`

// withoutFileLists drops the sections whose file lists differ by design
// (LEDGER SELF-17) and the 3.x binary line.
func withoutFileLists(out string) string {
	lines := strings.SplitAfter(out, "\n")
	var keep []string
	skip := false
	for _, l := range lines {
		switch {
		case strings.Contains(l, "Core Files:"):
			skip = true
		case strings.Contains(l, "Configuration:"):
			skip = false
		}
		if !skip && !strings.Contains(l, "  gash dev (") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "")
}

// gash_doctor prints what 2.x prints, the file lists aside: the header,
// the runtime lines, the settings file and settings of the shell, the
// tools, the summary and the status, colors off, on and shadowed.
func TestDoctorMatches2x(t *testing.T) {
	for _, mode := range []string{"off", "on", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			ref, got := t.TempDir(), t.TempDir()
			var script strings.Builder
			script.WriteString("unalias -a\n")
			if mode == "off" {
				script.WriteString("__gash_use_color() { return 1; }\n")
			} else {
				script.WriteString("__gash_use_color() { return 0; }\n")
			}
			for i, tc := range doctorCases {
				d := filepath.Join(ref, strconv.Itoa(i))
				doctorInstall(t, d)
				doctorInstall(t, filepath.Join(got, strconv.Itoa(i)))
				args := tc.args
				if mode == "shadow" {
					args = append([]string{"--no-color"}, args...)
				}
				script.WriteString("(D=" + shq(d) + "; HOME=$D/home; " + tc.prefix + "\n" + doctorStubLoop +
					"GASH_DIR=$D/inst PATH=$D/bin gash_doctor")
				for _, a := range args {
					script.WriteString(" " + shq(a))
				}
				script.WriteString(" 2>&1; printf '\\0%s\\0' $?)\n")
			}
			out, _, _ := run2xIn(t, copy2x(t), []string{"LANG=C.UTF-8", "COLUMNS=100"}, script.String())
			fields := strings.Split(out, "\x00")
			require.Len(t, fields, 7*len(doctorCases)+1, out)
			for i, tc := range doctorCases {
				f := fields[7*i : 7*i+7]
				d := filepath.Join(got, strconv.Itoa(i))
				norm := func(s string) string { return strings.ReplaceAll(s, filepath.Join(ref, strconv.Itoa(i)), d) }
				bash(t, "D="+shq(d)+"; "+tc.prefix)
				env := map[string]string{
					"LANG": "C.UTF-8", "COLUMNS": "100", "HOME": d + "/home", "GASH_DIR": d + "/inst", "PATH": d + "/bin",
					"BASH_VERSION": f[4], selfmgmt.LoadErrorsVar: f[0], selfmgmt.SettingsVar: f[1],
				}
				if f[2] != "" {
					env["GASH_ENV_FILE"] = norm(f[2])
				}
				if f[3] != "" {
					env["GASH_VERSION"] = f[3]
				}
				args := tc.args
				if mode == "shadow" {
					args = append([]string{"--no-color"}, args...)
				}
				o, rc := runMerged(t, "gash_doctor", env, mode != "off", args...)
				if strings.HasPrefix(tc.name, "help") {
					// The help page carries the 3.0 notes (SELF-06, SELF-17).
					continue
				}
				if tc.name == "bogus" {
					// 3.0: SELF-06 fixed: an unknown option is refused before
					// any check (TestLedger_SELF06).
					require.Equal(t, 2, rc, tc.name)
					continue
				}
				require.Equal(t, withoutFileLists(norm(f[5])), withoutFileLists(o), tc.name)
				// 3.0: SELF-06 fixed: issues make the status 1 (2.x: 0).
				want := f[6]
				if strings.Contains(o, " issue(s) found.") {
					want = "1"
				}
				require.Equal(t, want, strconv.Itoa(rc), tc.name)
			}
		})
	}
}

// LEDGER SELF-17: the doctor checks the files the 3.x tree ships, every
// generated one among them, and the binary; a staged tree passes, and a
// missing shim is its MISSING line.
func TestLedger_SELF17(t *testing.T) {
	defer prereq.SetSystem(&prereq.System{
		OS:      platform.OS{GOOS: "linux", ID: "debian", VersionID: "12", Name: "Debian GNU/Linux 12 (bookworm)"},
		Manager: prereq.Apt, Root: true,
		LookPath:   func(name string, _ []string) (string, bool) { return "/usr/bin/" + name, true },
		RunProbe:   func(string, []string, []string) bool { return true },
		RunVersion: func(string, []string) string { return "GNU bash, version 5.2.15(1)-release" },
	})()
	d := t.TempDir()
	for _, f := range selfmgmt.DoctorCoreFiles {
		p := filepath.Join(d, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("3.0.0\n"), 0o644))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(d, "lib/modules"), 0o755))
	for _, m := range selfmgmt.DoctorModules {
		require.NoError(t, os.WriteFile(filepath.Join(d, "lib/modules", m+".sh"), nil, 0o644))
	}
	env := map[string]string{"HOME": t.TempDir(), "GASH_DIR": d, "PATH": t.TempDir(), "BASH_VERSION": "5.2.21(1)-release"}
	out, rc := runMerged(t, "gash_doctor", env, false)
	require.Equal(t, 0, rc)
	require.Regexp(t, `(?m)^OK:   gash dev \(.+\)$`, out)
	require.True(t, strings.HasSuffix(out, "\nOK: All checks passed.\n"), out)

	require.NoError(t, os.Remove(filepath.Join(d, "lib/core/utils.sh")))
	require.NoError(t, os.Remove(filepath.Join(d, "lib/modules/node.sh")))
	out, rc = runMerged(t, "gash_doctor", env, false)
	require.Equal(t, 1, rc) // 3.0: SELF-06 fixed
	require.Contains(t, out, "\nError:   lib/core/utils.sh — MISSING\n")
	require.Contains(t, out, "\nWarning:   node — not found\n")
	require.True(t, strings.HasSuffix(out, "\nWarning: 2 issue(s) found.\n"), out)
}

// The doctor's lists name every file the generator ships, and the module
// shims are the committed ones.
func TestDoctorChecksTheShippedFiles(t *testing.T) {
	files, err := compatgen.Generate(false)
	require.NoError(t, err)
	checked := map[string]bool{}
	for _, f := range selfmgmt.DoctorCoreFiles {
		checked[f] = true
	}
	for _, m := range selfmgmt.DoctorModules {
		checked["lib/modules/"+m+".sh"] = true
	}
	for _, f := range files {
		// The cold definitions are checked with the shims.
		require.True(t, checked[f.Path] || strings.HasPrefix(f.Path, compatgen.ColdDir), f.Path)
	}
	require.ElementsMatch(t, compatgen.CommittedModules, selfmgmt.DoctorModules)
	for _, f := range []string{"VERSION", "quotes/list.txt", ".gash_env.template"} {
		require.True(t, checked[f], f)
		_, err := os.Stat(filepath.Join(repoRoot, f))
		require.NoError(t, err, f)
	}
}

// LEDGER SELF-06: a failed check makes the status 1, and an unknown option
// is refused with status 2 before any check runs (3.0: SELF-06 fixed; 2.x
// returned 0 and ignored the option).
func TestLedger_SELF06(t *testing.T) {
	env := map[string]string{"HOME": t.TempDir(), "GASH_DIR": filepath.Join(t.TempDir(), "none"), "PATH": t.TempDir()}
	out, rc := runMerged(t, "gash_doctor", env, false)
	require.Equal(t, 1, rc)
	require.Regexp(t, regexp.MustCompile(`\nWarning: [0-9]+ issue\(s\) found\.\n$`), out)
	require.Contains(t, out, "\nError:   gash.sh — MISSING\n")

	out, rc = runMerged(t, "gash_doctor", env, false, "--bogus")
	require.Equal(t, 2, rc)
	require.Equal(t, "Error: Unknown option: --bogus (usage: gash_doctor [--timing] [--no-color] [--prereqs] [--json] [--install [--yes] [--all] [TOOL...]])\n", out)
}
