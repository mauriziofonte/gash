// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
)

// serviceLog is the default --log of llm_service: the last 10 journal
// lines of each unit.
const serviceLog = 10

// serviceProps are the systemctl show properties llm_service reads.
const serviceProps = "Id,LoadState,ActiveState,SubState,StateChangeTimestamp,MainPID,NRestarts"

// unitState is what systemctl show says about one unit.
type unitState struct {
	load, active, sub, since string
	pid, restarts            string
}

// service is llm_service UNIT... (plan P3): the state of each unit and its
// last journal lines in one call, instead of systemctl is-active plus
// journalctl -u ... | cut. Read-only: it runs systemctl show and
// journalctl. Rows come in the order of the units given.
func (m *Module) service(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	logN := serviceLog
	var units []string
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch {
		case a == "--log":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if !validUint(c, "log", args[1]) {
				return 1
			}
			logN = atoi(args[1])
			args = args[2:]
			continue
		case strings.HasPrefix(a, "-"):
			return fail(c, "unknown_option", a, "Use UNIT... with --log N, --format F or --width N")
		}
		units = append(units, a)
		args = args[1:]
	}
	if len(units) == 0 {
		return fail(c, "missing_unit", "", "Name one or more units, e.g. llm_service nginx php8.4-fpm")
	}
	systemctl, ok := m.locate(c, "systemctl")
	if !ok {
		return prereq.Fail(c, "systemctl", "llm_service", true)
	}
	env := append(c.Env.Child(), "LC_ALL=C", "SYSTEMD_PAGER=", "SYSTEMD_COLORS=0")
	res, err := m.run.Run(c.Ctx, runner.Cmd{Path: systemctl, Args: append([]string{"show", "--no-pager", "--property=" + serviceProps, "--"}, units...),
		Options: runner.Options{Env: env}})
	if err != nil || res.ExitCode != 0 {
		return fail(c, "systemd_unavailable", firstLine(string(res.Stderr), "systemctl show failed"), "systemd is not running here (a container, WSL without systemd). Skip this check")
	}
	states := parseUnitShow(res.Stdout)
	if len(states) != len(units) {
		return fail(c, "systemd_unavailable", "systemctl show gave "+strconv.Itoa(len(states))+" units for "+strconv.Itoa(len(units)), "Skip this check")
	}
	logs := make([][]string, len(units))
	// notes say why a unit has no journal lines when the journal could
	// not be read (3.4: 2.x and 3.4 before printed "log":[] as if it were
	// empty); the column is there only when a row has one.
	notes := make([]string, len(units))
	if logN > 0 {
		type job struct {
			i    int
			unit string
		}
		var jobs []job
		for i, u := range units {
			if states[i].load != "not-found" {
				jobs = append(jobs, job{i, u})
			}
		}
		journalctl, found := m.locate(c, "journalctl")
		width := o.Width
		if width == 0 {
			width = waitLineWidth
		}
		type journal struct {
			lines []string
			note  string
		}
		got := llmfmt.Map(c.Ctx, llmfmt.LocalJobs(), 0, jobs, func(_ context.Context, j job) journal {
			if !found {
				return journal{note: "journalctl is not installed"}
			}
			r, err := m.run.Run(c.Ctx, runner.Cmd{Path: journalctl, Args: []string{"-u", j.unit, "-n", strconv.Itoa(logN), "--no-pager", "-q", "-o", "short-iso", "--no-hostname"},
				Options: runner.Options{Env: env}})
			if err != nil || r.ExitCode != 0 {
				return journal{note: firstLine(string(r.Stderr), "journalctl exited with status "+strconv.Itoa(r.ExitCode))}
			}
			ls := textLines(r.Stdout)
			for k, l := range ls {
				ls[k] = llmfmt.Cut(l, width)
			}
			return journal{lines: ls}
		})
		checked, readable := false, true
		for k, j := range jobs {
			logs[j.i], notes[j.i] = got[k].lines, got[k].note
			if notes[j.i] != "" || len(logs[j.i]) > 0 {
				continue
			}
			if !checked {
				checked, readable = true, m.journalReadable()
			}
			if !readable {
				notes[j.i] = "the system journal is not readable by this user (root and the systemd-journal group read it)"
			}
		}
	}
	cols := []string{"unit", "load", "active", "sub", "since", "pid", "restarts", "log"}
	noted := slices.ContainsFunc(notes, func(n string) bool { return n != "" })
	if noted {
		cols = append(cols, "log_note")
	}
	t := llmfmt.Table{Cols: cols}
	for i, u := range units {
		s := states[i]
		l := logs[i]
		if l == nil {
			l = []string{}
		}
		row := []any{u, s.load, s.active, s.sub, nullable(s.since), intOrNull(s.pid), intOrNull(s.restarts), l}
		if noted {
			row = append(row, nullable(notes[i]))
		}
		t.Rows = append(t.Rows, row)
	}
	o.Write(c, o.FormatOr(llmfmt.JSON), t, nil)
	return 0
}

// parseUnitShow reads systemctl show for several units: one block of
// KEY=VALUE lines per unit, blocks separated by an empty line.
func parseUnitShow(out []byte) []unitState {
	var states []unitState
	var cur *unitState
	for _, l := range strings.Split(string(out), "\n") {
		l = strings.TrimSuffix(l, "\r")
		if l == "" {
			cur = nil
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			continue
		}
		if cur == nil {
			states = append(states, unitState{})
			cur = &states[len(states)-1]
		}
		switch k {
		case "LoadState":
			cur.load = v
		case "ActiveState":
			cur.active = v
		case "SubState":
			cur.sub = v
		case "StateChangeTimestamp":
			cur.since = systemdTime(v)
		case "MainPID":
			cur.pid = v
		case "NRestarts":
			cur.restarts = v
		}
	}
	return states
}

// systemdTime is a systemd timestamp without its weekday ("Tue 2026-10-20
// 10:00:00 UTC" -> "2026-10-20 10:00:00 UTC"); "n/a" and "" are none.
func systemdTime(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || v == "n/a" {
		return ""
	}
	if i := strings.IndexByte(v, ' '); i == 3 {
		return v[4:]
	}
	return v
}

// intOrNull is a decimal as a JSON number, anything else as null.
func intOrNull(s string) any {
	if s == "" || len(s) > 18 || !digitsRE.MatchString(s) {
		return nil
	}
	n := 0
	for i := 0; i < len(s); i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

// journalReadable reports whether this user reads the system journal:
// false when a system journal file exists and does not open (a user
// neither root nor in the systemd-journal group, whose journalctl -q
// prints nothing and exits 0).
func (m *Module) journalReadable() bool {
	if m.journalOK != nil {
		return m.journalOK()
	}
	denied := false
	for _, pat := range []string{"/var/log/journal/*/system.journal", "/run/log/journal/*/system.journal"} {
		files, _ := filepath.Glob(pat)
		for _, p := range files {
			f, err := os.Open(p)
			if err == nil {
				_ = f.Close()
				return true
			}
			denied = denied || errors.Is(err, fs.ErrPermission)
		}
	}
	return !denied
}
