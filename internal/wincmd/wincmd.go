// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package wincmd is `gash win`: read-only inspection of the Windows that
// hosts the WSL distribution, for a human (eye-candy tables and marks) and
// for an agent (--json with a documented, stable shape). Every subcommand
// goes through internal/winbridge, so it finds Windows programs by absolute
// path (works with appendWindowsPath off), bounds each call with a timeout,
// and reports a broken or disabled interop as interop_unavailable rather
// than hanging. Nothing here writes to Windows: the scripts hold only read
// verbs, proven by a test. Writes live in `gash win fix`, behind UAC.
//
//	gash win info|disk|net|updates|defender|policy|hosts|doctor [--json]
//	gash win reg KEY [--value NAME] [--json]
//	gash win events [--log L] [--level LVL] [--since DUR] [-n N] [--json]
//	gash win services|procs|apps [FILTER] [--json]
//	gash win ports [--port N] [--json]
//	gash win file PATH [--json]
package wincmd

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
	"github.com/mauriziofonte/gash/internal/winbridge"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// Options are the seams; the zero value runs against the real system.
type Options struct {
	// Bridge builds the Windows bridge for a call; nil builds the real one
	// from a fresh WSL detection.
	Bridge func(c *invoke.Call) *winbridge.Bridge
}

// Run is the handler of `gash win ARGS...`.
func Run(c *invoke.Call) int { return Handler(Options{})(c) }

// Register binds gash win to the table.
func Register(t *invoke.Table) { t.HandleCommand("win", Run) }

// Handler returns the handler of `gash win` with opt's seams.
func Handler(opt Options) invoke.Handler {
	return func(c *invoke.Call) int {
		if c.Ctx == nil {
			c.Ctx = context.Background()
		}
		if opt.Bridge == nil {
			opt.Bridge = func(c *invoke.Call) *winbridge.Bridge {
				sr := platform.Real()
				facts := wsl.Detect(sr, c.Env.Get)
				return winbridge.New(facts, sr, runner.Exec{}, c.Env.Get)
			}
		}
		r := &run{opt: opt, c: c}
		top := &cobra.Command{Use: "gash", SilenceErrors: true, SilenceUsage: true}
		top.CompletionOptions.DisableDefaultCmd = true
		top.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
		top.AddCommand(r.tree())
		args := append([]string{"win"}, c.Args...)
		if len(c.Args) > 0 && c.Args[0] == cobra.ShellCompRequestCmd {
			args = append([]string{cobra.ShellCompRequestCmd, "win"}, c.Args[1:]...)
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
	br     *winbridge.Bridge
	// flags
	asJSON bool
	value  string
	log    string
	level  string
	since  string
	max    int
	port   int
	dryRun bool
	yes    bool
}

func (r *run) tree() *cobra.Command {
	win := &cobra.Command{Use: "win", Short: "Inspect the Windows host from WSL (read-only)", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() }}

	simple := func(use, short string, fn func() error) *cobra.Command {
		c := &cobra.Command{Use: use, Short: short, Args: cobra.NoArgs, RunE: func(*cobra.Command, []string) error { return fn() }}
		c.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")
		return c
	}
	filtered := func(use, short string, fn func(string) error) *cobra.Command {
		c := &cobra.Command{Use: use, Short: short, Args: cobra.MaximumNArgs(1),
			RunE: func(_ *cobra.Command, a []string) error { return fn(arg0(a)) }}
		c.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")
		return c
	}

	info := simple("info", "Windows build, edition, host, user and elevation", r.doInfo)
	disk := simple("disk", "Fixed drives with size and free space", r.doDisk)
	net := simple("net", "Network adapters, DNS and proxy", r.doNet)
	updates := simple("updates", "Installed hotfixes and pending reboot", r.doUpdates)
	defender := simple("defender", "Microsoft Defender status", r.doDefender)
	policy := simple("policy", "Where policy is set (registry roots) and the device join", r.doPolicy)
	hosts := simple("hosts", "The Windows hosts file", r.doHosts)
	doctor := simple("doctor", "A composed health check of the Windows host", r.doDoctor)

	services := filtered("services [FILTER]", "Windows services (optionally filtered by name)", r.doServices)
	procs := filtered("procs [FILTER]", "Top processes by memory (optionally filtered)", r.doProcs)
	apps := filtered("apps [FILTER]", "Installed programs (optionally filtered)", r.doApps)

	reg := &cobra.Command{Use: "reg KEY", Short: "Query a registry key (HKLM\\…, HKCU\\…)", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, a []string) error { return r.doReg(a[0]) }}
	reg.Flags().StringVar(&r.value, "value", "", "read only this value")
	reg.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")

	events := &cobra.Command{Use: "events", Short: "Windows event log entries", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return r.doEvents() }}
	events.Flags().StringVar(&r.log, "log", "System", "the log name (System, Application, Security, …)")
	events.Flags().StringVar(&r.level, "level", "", "error, warning, information or critical")
	events.Flags().StringVar(&r.since, "since", "", "only entries newer than this (e.g. 1h, 24h, 7d)")
	events.Flags().IntVarP(&r.max, "limit", "n", 50, "maximum entries")
	events.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")

	ports := &cobra.Command{Use: "ports", Short: "Listening and established TCP ports with the owning process", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return r.doPorts() }}
	ports.Flags().IntVar(&r.port, "port", 0, "only this local port")
	ports.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")

	file := &cobra.Command{Use: "file PATH", Short: "A file's metadata, owner, ACL and signature", Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, a []string) error { return r.doFile(a[0]) }}
	file.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")
	file.Flags().IntVarP(&r.max, "limit", "n", 0, "")

	win.AddCommand(info, disk, net, updates, defender, policy, hosts, doctor,
		services, procs, apps, reg, events, ports, file, r.fixCmd())
	return win
}

// fixCmd is `gash win fix`: the one write path, human-only and behind UAC.
// With no action it lists the allowlist.
func (r *run) fixCmd() *cobra.Command {
	fix := &cobra.Command{Use: "fix [ACTION] [ARGS...]",
		Short: "Apply an elevated Windows correction (human-only, asks for UAC)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, a []string) error {
			if len(a) == 0 {
				return r.fixList(cmd)
			}
			return r.doFix(a[0], a[1:])
		}}
	fix.Flags().BoolVar(&r.dryRun, "dry-run", false, "print the exact elevated command without running it")
	fix.Flags().BoolVar(&r.yes, "yes", false, "skip the in-shell question (Windows still asks for UAC)")
	fix.Flags().BoolVar(&r.asJSON, "json", false, "with --dry-run, emit the command as JSON")
	return fix
}

// fixList prints the available corrections.
func (r *run) fixList(*cobra.Command) error {
	if r.asJSON {
		var out []map[string]string
		for _, a := range fixActions {
			out = append(out, map[string]string{"action": a.name, "usage": "gash win fix " + a.usage, "description": a.short})
		}
		return r.emitJSON(map[string]any{"actions": out})
	}
	s := r.style()
	s.Section(r.c.Stdout, "gash win fix — elevated Windows corrections")
	rows := []string{"action\tdescription\t"}
	for _, a := range fixActions {
		rows = append(rows, a.usage+"\t"+a.short)
	}
	s.Table(r.c.Stdout, rows, "--gap", "2")
	r.say().Note("each action asks for a Windows UAC prompt; add --dry-run to see the exact command")
	return nil
}

// bridge builds the Windows bridge once.
func (r *run) bridge() *winbridge.Bridge {
	if r.br == nil {
		r.br = r.opt.Bridge(r.c)
	}
	return r.br
}

func (r *run) style() ui.Style { return ui.StyleOf(r.c.Term, false) }
func (r *run) say() ui.Printer { return r.style().Printer(r.c.Stderr) }

func (r *run) fail(e contract.Envelope) int {
	_ = e.Write(r.c.Stderr)
	r.status = 1
	return 1
}

// winErr turns a bridge error into the contract envelope and reports it.
func (r *run) winErr(e *winbridge.Error) error {
	r.fail(contract.New(e.Code, e.Details, e.Hint))
	return nil
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func arg0(a []string) string {
	if len(a) > 0 {
		return a[0]
	}
	return ""
}

func usageEnvelope(err error) contract.Envelope {
	msg := err.Error()
	code := "unexpected_argument"
	switch {
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		code = "unknown_option"
	case strings.HasPrefix(msg, "flag needs an argument"):
		code = "missing_value"
	}
	return contract.New(code, msg, "Run: gash win --help")
}
