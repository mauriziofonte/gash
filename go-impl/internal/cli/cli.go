// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package cli is the command line of the binary: a cobra tree built from
// the registry.
//
//   - `gash`, `gash TOPIC`, `gash --no-color ...`, `gash migrate` and any
//     word that is not a command run the gash card handler with every
//     argument, as the 2.x gash function received them.
//   - `gash NAME ARGS...` for a 2.x function: a cobra command with
//     DisableFlagParsing, the arguments handed raw to the handler.
//   - The 3.x commands (version, init, update, hte, hidden __compat and
//     __complete) use ordinary cobra flags.
package cli

import (
	"context"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/shellinit"
	"github.com/mauriziofonte/gash/internal/term"
)

// Process is what the command line runs against: the arguments after the
// program name, the caller's environment and streams.
type Process struct {
	Ctx    context.Context
	Args   []string
	Env    invoke.Env
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Term   *term.Term
	Table  *invoke.Table
	// Executable returns the path of the running binary (os.Executable).
	Executable func() (string, error)
}

// commands3x are the words that select a 3.x command.
var commands3x = map[string]bool{
	"version": true, "init": true, "update": true, "hte": true,
	"__compat": true, cobra.ShellCompRequestCmd: true, cobra.ShellCompNoDescRequestCmd: true,
}

// IsCommand reports whether word selects a command of the binary rather
// than a topic of the gash card.
func IsCommand(word string) bool {
	if commands3x[word] {
		return true
	}
	f, ok := registry.Lookup(word)
	return ok && f.Delegated()
}

// Run runs the command line and returns the exit status.
func Run(p Process) int {
	if p.Ctx == nil {
		p.Ctx = context.Background()
	}
	if p.Executable == nil {
		p.Executable = os.Executable
	}
	if len(p.Args) == 0 || !IsCommand(p.Args[0]) {
		return p.call("gash", p.Args)
	}
	status := 0
	root := newRoot(&p, &status)
	root.SetArgs(p.Args)
	if err := root.ExecuteContext(p.Ctx); err != nil {
		return p.fail(usageEnvelope(err, p.Args))
	}
	return status
}

// call runs a 2.x function of the table.
func (p *Process) call(name string, args []string) int {
	return p.Table.Run(&invoke.Call{
		Ctx: p.Ctx, Name: name, Args: args, Env: p.Env,
		Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term,
	})
}

func (p *Process) fail(e contract.Envelope) int {
	_ = e.Write(p.Stderr)
	return 1
}

// newRoot builds the cobra tree. Each RunE stores the exit status in
// *status and returns nil: a non-nil error from Execute is always a
// command-line error of a 3.x command.
func newRoot(p *Process, status *int) *cobra.Command {
	root := &cobra.Command{
		Use:                "gash",
		Short:              "Bash for humans and AI agents",
		SilenceErrors:      true,
		SilenceUsage:       true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			*status = p.call("gash", args)
			return nil
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
	root.SetIn(p.Stdin)
	root.SetOut(p.Stdout)
	root.SetErr(p.Stderr)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return err })

	for i := range registry.Functions() {
		f := &registry.Functions()[i]
		if !f.Delegated() {
			continue
		}
		name := f.Name
		root.AddCommand(&cobra.Command{
			Use:                name,
			Short:              f.Help.Short,
			DisableFlagParsing: true,
			Args:               cobra.ArbitraryArgs,
			RunE: func(_ *cobra.Command, args []string) error {
				*status = p.call(name, args)
				return nil
			},
		})
	}
	root.AddCommand(versionCmd(p, status), initCmd(p, status), updateCmd(p, status),
		hteCmd(p, status), compatCmd(p, status))
	return root
}

// notPorted is a 3.x command placeholder until its milestone lands.
func notPorted(p *Process, status *int, name string) func(*cobra.Command, []string) error {
	return func(*cobra.Command, []string) error {
		*status = p.fail(invoke.NotPorted(name))
		return nil
	}
}

func initCmd(p *Process, status *int) *cobra.Command {
	return &cobra.Command{
		Use:       "init bash",
		Short:     "Print the interactive Bash layer for gash.sh to evaluate",
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs: []string{"bash"},
		RunE: func(_ *cobra.Command, _ []string) error {
			c := &invoke.Call{
				Ctx: p.Ctx, Name: "init", Env: p.Env,
				Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr,
				Term: p.Term, Handlers: p.Table,
			}
			script, err := shellinit.New().Emit(c)
			if err != nil {
				*status = p.fail(contract.New("init_failed", err.Error(), ""))
				return nil
			}
			_, _ = io.WriteString(p.Stdout, script)
			return nil
		},
	}
}

func updateCmd(p *Process, status *int) *cobra.Command {
	c := &cobra.Command{
		Use:   "update",
		Short: "Update Gash to the latest verified release",
		Args:  cobra.NoArgs,
		RunE:  notPorted(p, status, "gash update"),
	}
	c.Flags().Bool("rollback", false, "return to the previous installed version")
	return c
}

func hteCmd(p *Process, status *int) *cobra.Command {
	return &cobra.Command{
		Use:                "hte",
		Short:              "Manage the local Apache and PHP-FPM test environment",
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			if h, ok := p.Table.LookupCommand("hte"); ok {
				*status = h(&invoke.Call{
					Ctx: p.Ctx, Name: "hte", Args: args, Env: p.Env,
					Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term, Handlers: p.Table,
				})
				return nil
			}
			*status = p.fail(invoke.NotPorted("gash hte"))
			return nil
		},
	}
}

// compatCmd is the hidden plumbing of the generated Bash: the operation
// named by the leading words, its arguments unparsed.
func compatCmd(p *Process, status *int) *cobra.Command {
	return &cobra.Command{
		Use:                "__compat",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			h, op, rest, ok := p.Table.LookupCompat(args)
			if !ok {
				*status = p.fail(invoke.NotPorted("__compat " + strings.Join(first(args, 2), " ")))
				return nil
			}
			*status = h(&invoke.Call{
				Ctx: p.Ctx, Name: "__compat " + op, Args: rest, Env: p.Env,
				Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term, Handlers: p.Table,
			})
			return nil
		},
	}
}

func first(s []string, n int) []string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// usageEnvelope maps a cobra or pflag error of a 3.x command to the
// envelope of its kind.
func usageEnvelope(err error, args []string) contract.Envelope {
	msg := err.Error()
	code := "unexpected_argument"
	switch {
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		code = "unknown_option"
	case strings.HasPrefix(msg, "flag needs an argument"):
		code = "missing_value"
	}
	hint := ""
	if len(args) > 0 {
		hint = "Run: gash " + args[0] + " --help"
	}
	return contract.New(code, msg, hint)
}
