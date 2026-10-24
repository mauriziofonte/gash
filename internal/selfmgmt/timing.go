// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// doctorTiming is gash_doctor --timing (__gash_doctor_timing): a new
// interactive shell loads Gash from the install directory with
// GASH_TIMING=1 and hands back __GASH_LOAD_TIMES (its own output dropped,
// SSH key unlocking marked done), shown as a table of milliseconds; then
// whole new shells are timed, bash -i with the user's ~/.bashrc and bash
// alone, the median of 3 each. The ~/.bashrc runs three times, which an
// info line says before anything starts (3.0: SELF-15 fixed; 2.x said
// nothing). The shells are the fixed runner.BashDoctor* uses.
func (m *Module) doctorTiming(c *invoke.Call, s ui.Style) int {
	p := s.Printer(c.Stderr)
	// 2.x read $EPOCHREALTIME of the calling shell: Bash 5.0 or newer.
	bv := c.Env.Get("BASH_VERSION")
	if major, _ := bashVersinfo(bv); bv != "" && major < 5 {
		p.Error("gash_doctor --timing needs Bash 5.0 or newer ($EPOCHREALTIME); this is " + bv + ".")
		return 1
	}
	p.Info("--timing starts your shell 3 times (your ~/.bashrc runs each time).")
	dir := installDir(c)
	bash := c.Env.Get("BASH")
	if bash == "" {
		bash = "bash"
	}
	ctx := ctxOf(c)

	env := runner.NewEnv().AllowAll(c.Env.Child()).
		Set("GASH_TIMING", "1").Set("GASH_HEADLESS", "").Set("GASH_SSH_AUTOUNLOCK_RAN", "1")
	res, _ := m.run.RunBash(ctx, runner.Bash{Use: runner.BashDoctorLoad, Bash: bash, Args: []string{dir},
		Options: runner.Options{Env: env.Environ(), Stderr: io.Discard}})
	out := runner.CommandSubst(res.Stdout)
	if out == "" {
		p.Error("No timings: a new shell did not load " + dir + "/gash.sh.")
		return 1
	}

	version := c.Env.Get("GASH_VERSION")
	if version == "" {
		version = "unknown"
	}
	s.Heading(c.Stdout, "--note", "Gash "+version+", bash "+bv+", in a new interactive shell", "Startup timing")
	// Milliseconds right-aligned, the total bold, slow steps (5 ms or
	// more) in the warning color.
	rows := []string{"ms\tstep\t"}
	for _, line := range strings.Split(out, "\n") {
		us, label := readWords(line)
		if us == "" || strings.Trim(us, "0123456789") != "" {
			continue
		}
		n, err := strconv.ParseInt(us, 10, 64)
		if err != nil {
			continue
		}
		style := ""
		if strings.HasSuffix(label, "(total)") {
			style = "bold"
		}
		if n >= 5000 {
			style = "warning"
		}
		rows = append(rows, usToMS(n)+"\t"+label+"\t"+style)
	}
	s.Table(c.Stdout, rows, "--no-header", "--indent", "2", "--right", "1", "--min", "1:7", "--cell-color", "1:3")

	// Whole shells, median of three runs each.
	whole := runner.NewEnv().AllowAll(c.Env.Child()).Set("GASH_SSH_AUTOUNLOCK_RAN", "1").Environ()
	bare := c.Env.Child()
	var wholeUS, bareUS [3]int64
	for i := range 3 {
		wholeUS[i] = m.timeShell(c, runner.BashDoctorShell, bash, whole)
		bareUS[i] = m.timeShell(c, runner.BashDoctorBare, bash, bare)
	}
	_, _ = io.WriteString(c.Stdout, "\n  New shell with your ~/.bashrc: "+usToMS(median3(wholeUS))+
		" ms (bash alone: "+usToMS(median3(bareUS))+" ms; median of 3)\n")
	_, _ = io.WriteString(c.Stdout, "  GASH_TIMING=1 when a shell starts keeps its own figures in __GASH_LOAD_TIMES.\n")
	return 0
}

// timeShell runs one whole-shell timing, stdin, stdout and stderr on
// /dev/null, and returns how long it took in microseconds.
func (m *Module) timeShell(c *invoke.Call, use runner.BashUse, bash string, env []string) int64 {
	t0 := time.Now()
	_, _ = m.run.RunBash(ctxOf(c), runner.Bash{Use: use, Bash: bash,
		Options: runner.Options{Env: env, Stdout: io.Discard, Stderr: io.Discard}})
	return time.Since(t0).Microseconds()
}

// readWords is `read -r first rest` with the default IFS: the first word
// and the rest of the line, blanks around both removed.
func readWords(line string) (first, rest string) {
	line = strings.Trim(line, " \t")
	i := strings.IndexAny(line, " \t")
	if i < 0 {
		return line, ""
	}
	return line[:i], strings.Trim(line[i:], " \t")
}

// usToMS is __gash_us_to_ms: microseconds as milliseconds with one
// decimal, truncated.
func usToMS(us int64) string {
	return strconv.FormatInt(us/1000, 10) + "." + strconv.FormatInt(us%1000/100, 10)
}

// median3 is __gash_median3.
func median3(v [3]int64) int64 {
	lo := min(v[0], v[1], v[2])
	hi := max(v[0], v[1], v[2])
	return v[0] + v[1] + v[2] - lo - hi
}
