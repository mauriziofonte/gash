// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package handlers

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// fakeBash stands for the shells gash_doctor --timing starts: it logs its
// arguments (the script as <script>) and the timing variables, and the
// load probe prints the times in TIMES, on fd 3 as 2.x reads them and on
// stdout as the runner's probe does.
const fakeBash = `#!/bin/bash
log=""
for a in "$@"; do
    [[ "$a" == *$'\n'* ]] && a="<script>"
    log+="[$a]"
done
printf '%s %s|%s|%s\n' "$log" "${GASH_TIMING-unset}" "${GASH_HEADLESS-unset}" "${GASH_SSH_AUTOUNLOCK_RAN-unset}" >>"$FAKE_LOG"
if [[ " $* " == *" gash-timing "* && -n "$TIMES" ]]; then
    { printf '%s\n' "$TIMES" >&3; } 2>/dev/null
    printf '%s\n' "$TIMES"
fi
exit 0
`

// gash_doctor --timing prints what 2.x prints for the same load times
// (the whole-shell figures aside) and starts the same shells.
func TestDoctorTimingMatches2x(t *testing.T) {
	times := "  812 lib/compat/bootstrap.sh\n" +
		"1500 lib/core/output.sh\n" +
		"not-a-number lib/core/utils.sh\n" +
		"\n" +
		"5000 lib/modules/gash.sh\n" +
		"123456 gash init bash\n" +
		"99 user files: ~/.bash_aliases, ~/.bash_local\n" +
		"7 gash.sh (total)   \n" +
		"6400 bash_completion (total)\n" +
		"42\n"
	cases := []struct{ name, times string }{{"times", times}, {"empty", ""}, {"one", "0 gash.sh (total)"}}
	whole := regexp.MustCompile(`[0-9]+\.[0-9] ms`)
	for _, mode := range []string{"off", "on", "shadow"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			d := t.TempDir()
			fake := filepath.Join(d, "fakebash")
			require.NoError(t, os.WriteFile(fake, []byte(fakeBash), 0o755))
			var script strings.Builder
			if mode == "off" {
				script.WriteString("__gash_use_color() { return 1; }\n")
			} else {
				script.WriteString("__gash_use_color() { return 0; }\n")
			}
			args := []string{"--timing"}
			if mode == "shadow" {
				args = append(args, "--no-color")
			}
			for i, tc := range cases {
				script.WriteString("FAKE_LOG=" + shq(filepath.Join(d, "2x"+strconv.Itoa(i))) + " TIMES=" + shq(tc.times) +
					" BASH=" + shq(fake) + " GASH_DIR=" + shq(d+"/inst") + " gash_doctor " + strings.Join(args, " ") +
					" 2>&1; printf '\\0%s\\0%s\\0' $? \"$BASH_VERSION\"\n")
			}
			out, _, _ := run2xIn(t, copy2x(t), []string{"LANG=C.UTF-8", "COLUMNS=100"}, script.String())
			fields := strings.Split(out, "\x00")
			require.Len(t, fields, 3*len(cases)+1, out)
			for i, tc := range cases {
				logGo := filepath.Join(d, "go"+strconv.Itoa(i))
				// The 2.x shell exported GASH_SSH_AUTOUNLOCK_RAN when it loaded.
				env := map[string]string{
					"LANG": "C.UTF-8", "COLUMNS": "100", "PATH": "/usr/bin:/bin", "HOME": d, "GASH_DIR": d + "/inst",
					"GASH_SSH_AUTOUNLOCK_RAN": "1", "BASH": fake, "BASH_VERSION": fields[3*i+2], "GASH_VERSION": "test",
					"FAKE_LOG": logGo, "TIMES": tc.times,
				}
				o, rc := runMerged(t, "gash_doctor", env, mode != "off", args...)
				// 3.0: SELF-15 fixed: an info line first says how often the
				// user's ~/.bashrc runs (2.x said nothing).
				info := regexp.MustCompile(`(?m)^.*--timing starts your shell 3 times \(your ~/\.bashrc runs each time\)\..*\n`)
				require.Regexp(t, info, o, tc.name)
				o = info.ReplaceAllString(o, "")
				require.Equal(t, whole.ReplaceAllString(fields[3*i], "N ms"), whole.ReplaceAllString(o, "N ms"), tc.name)
				require.Equal(t, fields[3*i+1], strconv.Itoa(rc), tc.name)
				want, err := os.ReadFile(filepath.Join(d, "2x"+strconv.Itoa(i)))
				require.NoError(t, err)
				gotLog, err := os.ReadFile(logGo)
				require.NoError(t, err)
				require.Equal(t, string(want), string(gotLog), tc.name)
			}
		})
	}
}

// Without $EPOCHREALTIME (Bash before 5.0) the timing is refused, as 2.x
// refuses it, and no shell starts.
func TestDoctorTimingNeedsBash5(t *testing.T) {
	d := t.TempDir()
	fake := filepath.Join(d, "fakebash")
	require.NoError(t, os.WriteFile(fake, []byte(fakeBash), 0o755))
	env := map[string]string{"HOME": d, "BASH": fake, "BASH_VERSION": "4.4.23(1)-release", "FAKE_LOG": d + "/log"}
	out, rc := runMerged(t, "gash_doctor", env, false, "--timing")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: gash_doctor --timing needs Bash 5.0 or newer ($EPOCHREALTIME); this is 4.4.23(1)-release.\n", out)
	_, err := os.Stat(d + "/log")
	require.True(t, os.IsNotExist(err))
}
