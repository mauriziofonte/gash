// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package logs is `gash logs`: it finds log files that are eating disk and
// reclaims the space, for a human (an eye-candy report and a confirmation)
// and for an agent (--json and --yes). A rotated or compressed copy is
// deleted; a live log is truncated in place, so a running daemon keeps its
// open file. Root-owned logs under /var/log are handled with one sudo when
// --sudo is given; without it they are counted and left.
//
//	gash logs scan [PATH...] [--min-size N] [--older-than DUR] [--json]
//	gash logs clean [PATH...] [--truncate] [--older-than DUR] [--min-size N]
//	               [--sudo] [--dry-run] [--yes] [--json]
//
// With no PATH, the scan covers the current directory, the common user log
// directories and /var/log.
package logs

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Options are the seams; the zero value runs against the real process.
type Options struct {
	Run runner.Runner    // nil: runner.Exec{}
	Now func() time.Time // nil: time.Now
}

// Run is the handler of `gash logs ARGS...`.
func Run(c *invoke.Call) int { return Handler(Options{})(c) }

// Register binds gash logs to the table.
func Register(t *invoke.Table) { t.HandleCommand("logs", Run) }

// Handler returns the handler of `gash logs` with opt's seams.
func Handler(opt Options) invoke.Handler {
	return func(c *invoke.Call) int {
		if c.Ctx == nil {
			c.Ctx = context.Background()
		}
		if opt.Run == nil {
			opt.Run = runner.Exec{}
		}
		if opt.Now == nil {
			opt.Now = time.Now
		}
		r := &run{opt: opt, c: c}
		top := &cobra.Command{Use: "gash", SilenceErrors: true, SilenceUsage: true}
		top.CompletionOptions.DisableDefaultCmd = true
		top.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
		top.AddCommand(r.tree())
		args := append([]string{"logs"}, c.Args...)
		if len(c.Args) > 0 && c.Args[0] == cobra.ShellCompRequestCmd {
			args = append([]string{cobra.ShellCompRequestCmd, "logs"}, c.Args[1:]...)
		}
		top.SetArgs(args)
		top.SetIn(c.Stdin)
		top.SetOut(c.Stdout)
		top.SetErr(c.Stderr)
		if _, err := top.ExecuteContextC(c.Ctx); err != nil {
			return r.fail(usageEnvelope(err))
		}
		return r.status
	}
}

type run struct {
	opt    Options
	c      *invoke.Call
	status int
	// flags
	asJSON    bool
	minSize   string
	olderThan string
	truncate  bool
	useSudo   bool
	dryRun    bool
	yes       bool
	depth     int
	// truncateBin is the truncate rootReady found (gtruncate on macOS).
	truncateBin string
}

func (r *run) tree() *cobra.Command {
	logs := &cobra.Command{
		Use: "logs", Short: "Find and reclaim disk space from log files", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	scan := &cobra.Command{
		Use: "scan [PATH...]", Short: "Report the log files eating disk, grouped by category",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return r.doScan(cmd, args) },
	}
	clean := &cobra.Command{
		Use: "clean [PATH...]", Short: "Reclaim space: delete rotated logs, optionally truncate live ones",
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return r.doClean(cmd, args) },
	}
	for _, c := range []*cobra.Command{scan, clean} {
		c.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")
		c.Flags().StringVar(&r.minSize, "min-size", "", "ignore files smaller than this (e.g. 1M, 512K)")
		c.Flags().StringVar(&r.olderThan, "older-than", "", "ignore files modified more recently than this (e.g. 7d, 48h)")
		c.Flags().IntVar(&r.depth, "depth", 0, "limit how deep each root is walked (0: no limit)")
	}
	clean.Flags().BoolVar(&r.truncate, "truncate", false, "also truncate live logs in place (keeps the file for running daemons)")
	clean.Flags().BoolVar(&r.useSudo, "sudo", false, "reclaim root-owned logs too, with one sudo")
	clean.Flags().BoolVar(&r.dryRun, "dry-run", false, "show what would be freed, change nothing")
	clean.Flags().BoolVarP(&r.yes, "yes", "y", false, "do not ask for confirmation")
	logs.AddCommand(scan, clean)
	return logs
}

func (r *run) scanOpts() (scanOptions, *contract.Envelope) {
	o := scanOptions{maxDepth: r.depth, now: r.opt.Now()}
	if r.minSize != "" {
		n, ok := parseSize(r.minSize)
		if !ok {
			e := contract.New("invalid_size", "not a size: "+r.minSize, "use bytes or a suffix: 1M, 512K, 2G")
			return o, &e
		}
		o.minSize = n
	}
	if r.olderThan != "" {
		d, ok := parseDuration(r.olderThan)
		if !ok {
			e := contract.New("invalid_duration", "not a duration: "+r.olderThan, "use a suffix: 30m, 48h, 7d")
			return o, &e
		}
		o.olderThan = d
	}
	return o, nil
}

func (r *run) roots(args []string) []string {
	if len(args) > 0 {
		out := make([]string, 0, len(args))
		for _, a := range args {
			out = append(out, expandHome(a, r.c.Env.Get("HOME")))
		}
		return out
	}
	return defaultRoots(r.c.Env.Get("HOME"))
}

func (r *run) doScan(_ *cobra.Command, args []string) error {
	o, e := r.scanOpts()
	if e != nil {
		r.fail(*e)
		return nil
	}
	rep := scan(r.roots(args), o)
	if r.asJSON {
		writeJSON(r.c.Stdout, rep)
		return nil
	}
	r.renderReport(rep)
	return nil
}

func (r *run) doClean(_ *cobra.Command, args []string) error {
	o, e := r.scanOpts()
	if e != nil {
		r.fail(*e)
		return nil
	}
	rep := scan(r.roots(args), o)
	ops := plan(rep, r.truncate)
	if len(ops) == 0 {
		return r.cleanResult(rep, Result{}, true)
	}
	if !r.rootReady(ops) {
		return nil
	}
	// The confirmation: a human sees the report and the total; headless or
	// without --yes on a pipe it must pass --yes.
	if !r.dryRun && !r.yes {
		if !r.confirmClean(rep, ops) {
			r.say().Info("Nothing changed.")
			r.status = 1
			return nil
		}
	}
	cl := cleaner{run: r.opt.Run, dryRun: r.dryRun, useSudo: r.useSudo, truncate: r.truncateBin}
	res := cl.apply(r.c.Ctx, ops)
	return r.cleanResult(rep, res, false)
}

// rootReady checks, before anything is asked or changed, the programs the
// sudo batches run: sudo, and truncate when a root-owned live log is
// truncated (3.4: a missing one was "N files could not be removed.", after
// the other files were changed).
func (r *run) rootReady(ops []op) bool {
	if !r.useSudo || r.dryRun {
		return true
	}
	need := map[string]bool{}
	for _, o := range ops {
		if o.needRoot {
			need["sudo"] = true
			need["truncate"] = need["truncate"] || o.Truncate
		}
	}
	for _, t := range [][2]string{{"sudo", "gash logs clean --sudo"}, {"truncate", "gash logs clean --sudo --truncate"}} {
		if !need[t[0]] {
			continue
		}
		path, ok := prereq.Locate(r.c, t[0])
		if !ok {
			r.status = prereq.Fail(r.c, t[0], t[1], r.asJSON)
			return false
		}
		if t[0] == "truncate" {
			r.truncateBin = path
		}
	}
	return true
}

// confirmClean shows the plan and asks, unless there is no terminal.
func (r *run) confirmClean(rep *Report, ops []op) bool {
	if r.c.Term.Headless() || !r.interactive() {
		r.fail(contract.New("confirmation_required",
			"logs clean changes files and there is no terminal to confirm on",
			"pass --yes to run it unattended, or --dry-run to preview"))
		return false
	}
	r.renderPlan(rep, ops)
	var freed int64
	del, trunc := 0, 0
	for _, o := range ops {
		freed += o.Size
		if o.Truncate {
			trunc++
		} else {
			del++
		}
	}
	prompt := "Reclaim " + ui.HumanBytes(freed) + " (" + plural(del, "delete", "deletes") + ", " + plural(trunc, "truncate", "truncates") + ")?"
	return r.style().Confirm(r.c.Term, r.c.Stderr, prompt)
}

func (r *run) cleanResult(rep *Report, res Result, nothing bool) error {
	if r.asJSON {
		writeJSON(r.c.Stdout, cleanJSON(rep, res, r.dryRun))
		return nil
	}
	if nothing {
		r.say().Info("No reclaimable logs found.")
		return nil
	}
	r.renderClean(res)
	return nil
}

func (r *run) interactive() bool {
	t := r.c.Term
	return t != nil && !t.Headless() && t.StdinTTY() && t.StderrTTY()
}

func (r *run) style() ui.Style { return ui.StyleOf(r.c.Term, false) }
func (r *run) say() ui.Printer { return r.style().Printer(r.c.Stderr) }

func (r *run) fail(e contract.Envelope) int {
	_ = e.Write(r.c.Stderr)
	r.status = 1
	return 1
}

// usageEnvelope maps a cobra/pflag error to the contract, as the other 3.x
// commands do.
func usageEnvelope(err error) contract.Envelope {
	msg := err.Error()
	code := "unexpected_argument"
	switch {
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		code = "unknown_option"
	case strings.HasPrefix(msg, "flag needs an argument"):
		code = "missing_value"
	}
	return contract.New(code, msg, "Run: gash logs --help")
}

// defaultRoots are scanned when the user names no path: the current
// directory (project logs), the common user log directories and the
// system log directory.
func defaultRoots(home string) []string {
	cwd, _ := os.Getwd()
	roots := []string{}
	if cwd != "" {
		roots = append(roots, cwd)
	}
	if home != "" {
		for _, d := range []string{".npm/_logs", ".pm2/logs", ".cache/pip/log", ".local/state"} {
			p := filepath.Join(home, d)
			if st, err := os.Stat(p); err == nil && st.IsDir() {
				roots = append(roots, p)
			}
		}
	}
	if st, err := os.Stat("/var/log"); err == nil && st.IsDir() {
		roots = append(roots, "/var/log")
	}
	return roots
}

func expandHome(p, home string) string {
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") && home != "" {
		return filepath.Join(home, p[2:])
	}
	return p
}

// parseSize reads a byte count with an optional K/M/G/T suffix (powers of
// 1024); a bare number is bytes.
func parseSize(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	// An optional trailing "b"/"B" ("5MB", "512kB") is dropped first.
	if n := len(s); n > 0 && (s[n-1] == 'b' || s[n-1] == 'B') {
		s = s[:n-1]
	}
	mult := int64(1)
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'k', 'K':
			mult, s = 1<<10, s[:n-1]
		case 'm', 'M':
			mult, s = 1<<20, s[:n-1]
		case 'g', 'G':
			mult, s = 1<<30, s[:n-1]
		case 't', 'T':
			mult, s = 1<<40, s[:n-1]
		}
	}
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n * mult, true
}

// parseDuration reads a Go duration, plus a "d" (day) suffix Go does not.
func parseDuration(s string) (time.Duration, bool) {
	s = strings.TrimSpace(s)
	if n, ok := strings.CutSuffix(s, "d"); ok {
		days, err := strconv.ParseFloat(n, 64)
		if err != nil || days < 0 {
			return 0, false
		}
		return time.Duration(days * 24 * float64(time.Hour)), true
	}
	d, err := time.ParseDuration(s)
	if err != nil || d < 0 {
		return 0, false
	}
	return d, true
}

func plural(n int, one, many string) string {
	if n == 1 {
		return strconv.Itoa(n) + " " + one
	}
	return strconv.Itoa(n) + " " + many
}
