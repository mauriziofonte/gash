// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package wslcmd is `gash wsl`: it tells a human and an agent the truth
// about the WSL distribution they are in — the version, the Windows
// interop and its socket, the mounted drives, the configuration in force —
// converts paths between the Linux and Windows forms, and repairs the
// interop of a reattached shell. Everything is read-only toward Windows
// and reads only files (no process starts), so it is fast and never hangs.
//
//	gash wsl info   [--json]
//	gash wsl doctor [--json]
//	gash wsl path PATH [--windows|--mixed|--linux] [--json]
//	gash wsl config [FILE] [--json]
//	gash wsl interop [--export] [--json]
package wslcmd

import (
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/ui"
	"github.com/mauriziofonte/gash/internal/wsl"
)

// Options are the seams; the zero value runs against the real system.
type Options struct {
	Sysroot platform.Sysroot
	// Getenv reads the caller's environment; nil uses the call's.
	Getenv func(string) string
}

// Run is the handler of `gash wsl ARGS...`.
func Run(c *invoke.Call) int { return Handler(Options{})(c) }

// Register binds gash wsl to the table.
func Register(t *invoke.Table) { t.HandleCommand("wsl", Run) }

// Handler returns the handler of `gash wsl` with opt's seams.
func Handler(opt Options) invoke.Handler {
	return func(c *invoke.Call) int {
		if c.Ctx == nil {
			c.Ctx = context.Background()
		}
		if opt.Getenv == nil {
			opt.Getenv = c.Env.Get
		}
		r := &run{opt: opt, c: c}
		top := &cobra.Command{Use: "gash", SilenceErrors: true, SilenceUsage: true}
		top.CompletionOptions.DisableDefaultCmd = true
		top.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
		top.AddCommand(r.tree())
		args := append([]string{"wsl"}, c.Args...)
		if len(c.Args) > 0 && c.Args[0] == cobra.ShellCompRequestCmd {
			args = append([]string{cobra.ShellCompRequestCmd, "wsl"}, c.Args[1:]...)
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
	opt      Options
	c        *invoke.Call
	status   int
	asJSON   bool
	toWin    bool
	toMixed  bool
	toLinux  bool
	doExport bool
	facts    *wsl.Facts
}

func (r *run) tree() *cobra.Command {
	wslCmd := &cobra.Command{
		Use: "wsl", Short: "Inspect and repair the WSL distribution and convert paths", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	info := &cobra.Command{Use: "info", Short: "A quick summary of the WSL environment", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return r.doInfo() }}
	doctor := &cobra.Command{Use: "doctor", Short: "Check WSL, interop, drives, networking and configuration", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return r.doDoctor() }}
	path := &cobra.Command{Use: "path PATH", Short: "Convert a path between its Linux and Windows forms", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, a []string) error { return r.doPath(a) }}
	path.Flags().BoolVar(&r.toWin, "windows", false, "force the Windows form (C:\\Users\\me)")
	path.Flags().BoolVar(&r.toMixed, "mixed", false, "force the Windows form with forward slashes (C:/Users/me)")
	path.Flags().BoolVar(&r.toLinux, "linux", false, "force the Linux form (/mnt/c/Users/me)")
	config := &cobra.Command{Use: "config [FILE]", Short: "Show the effective wsl.conf and .wslconfig settings", Args: cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, a []string) error { return r.doConfig(a) }}
	interop := &cobra.Command{Use: "interop", Short: "Report the Windows interop of this shell", Args: cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error { return r.doInterop() }}
	interop.Flags().BoolVar(&r.doExport, "export", false, "print only `export WSL_INTEROP=…` for eval")
	for _, c := range []*cobra.Command{info, doctor, path, config, interop} {
		c.Flags().BoolVar(&r.asJSON, "json", false, "one JSON object on stdout")
	}
	wslCmd.AddCommand(info, doctor, path, config, interop)
	return wslCmd
}

// detect returns the facts, built once.
func (r *run) detect() wsl.Facts {
	if r.facts == nil {
		f := wsl.Detect(r.opt.Sysroot, r.opt.Getenv)
		r.facts = &f
	}
	return *r.facts
}

func (r *run) style() ui.Style { return ui.StyleOf(r.c.Term, false) }
func (r *run) say() ui.Printer { return r.style().Printer(r.c.Stderr) }

func (r *run) fail(e contract.Envelope) int {
	_ = e.Write(r.c.Stderr)
	r.status = 1
	return 1
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

// notWSL writes the "not WSL" message (text or JSON) and reports it.
func (r *run) notWSL() bool {
	if r.detect().WSL {
		return false
	}
	if r.asJSON {
		writeJSON(r.c.Stdout, map[string]any{"wsl": false})
		return true
	}
	r.say().Error("Not running under WSL (the Windows Subsystem for Linux).")
	return true
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
	return contract.New(code, msg, "Run: gash wsl --help")
}
