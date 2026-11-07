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
	"bytes"
	"context"
	"io"
	"os"
	"strconv"
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

// commands3x are the words that select a 3.x command: the public ones of
// registry.Commands and the internal ones.
var commands3x = func() map[string]bool {
	m := map[string]bool{"__compat": true, cobra.ShellCompRequestCmd: true, cobra.ShellCompNoDescRequestCmd: true}
	for _, c := range registry.Commands() {
		m[c] = true
	}
	return m
}()

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
	// 3.5: the completion of the command tree; the 3.x commands' words go
	// on to their cobra tree below.
	if p.completeTree() {
		return 0
	}
	// 3.5: a canonical command (gash files largest, gash git tag add,
	// gash doctor) runs its handler directly, as its flat name does.
	if len(p.Args) > 0 && !strings.HasPrefix(p.Args[0], "__") {
		if c, rest, ok := registry.Resolve(p.Args); ok {
			if status, done := p.runTree(c, rest); done {
				return status
			}
		} else if delegatePage(p.Args) {
			// gash wsl, gash win, gash runtime: the namespace page lists
			// the commands of the tree with those of the 3.x command.
			return p.call("gash", p.Args)
		}
	}
	if len(p.Args) == 0 || !IsCommand(p.Args[0]) {
		return p.call("gash", p.Args)
	}
	// A 2.x function takes its arguments raw (DisableFlagParsing): run it
	// without building the cobra tree, which costs more than most calls.
	if !commands3x[p.Args[0]] {
		if f, ok := registry.Lookup(p.Args[0]); ok && f.Delegated() {
			return p.call(f.Name, p.Args[1:])
		}
	}
	status := 0
	root := newRoot(&p, &status)
	root.SetArgs(p.Args)
	if err := root.ExecuteContext(p.Ctx); err != nil {
		return p.fail(usageEnvelope(err, p.Args))
	}
	return status
}

// delegatePage reports `gash NS` or `gash NS --help` for a namespace
// whose 3.x command shares it with commands of the tree.
func delegatePage(args []string) bool {
	ns, ok := registry.NamespaceOf(args[0])
	if !ok || ns.Delegate == "" || len(registry.CommandsUnder(ns.Name)) == 0 {
		return false
	}
	return len(args) == 1 || len(args) == 2 && (args[1] == "--help" || args[1] == "-h")
}

// runTree runs a command of the 3.5 tree: its catalog function, or the
// 3.x command it names (then done is false and p.Args is rewritten for
// the cobra tree).
func (p *Process) runTree(c *registry.Command, rest []string) (status int, done bool) {
	if c.Handler != "" {
		f, ok := registry.Lookup(c.Handler)
		if ok && f.Delegated() {
			return p.call(f.Name, append(append([]string(nil), c.Prefix...), rest...)), true
		}
		return p.fail(contract.New("shell_required", c.Name()+" runs in the shell that loaded Gash",
			"Run it from a shell that sourced ~/.gash/gash.sh")), true
	}
	switch c.Command3x {
	case "help":
		// gash help update, gash help hte site create: the --help of a
		// 3.x command, which its own tree prints.
		if len(rest) > 0 {
			if target, _, ok := registry.Resolve(rest); ok && target.Handler == "" && target.Command3x != "help" && target.Command3x != "all" {
				p.Args = append(append([]string{target.Command3x}, target.Prefix...), "--help")
				return 0, false
			}
			if _, _, ok := registry.Resolve(rest); !ok && len(rest) > 1 {
				if ns, ok := registry.NamespaceOf(rest[0]); ok && ns.Delegate != "" {
					p.Args = append(append([]string{ns.Delegate}, rest[1:]...), "--help")
					return 0, false
				}
			}
		}
		fallthrough
	case "all":
		if h, ok := p.Table.LookupCommand(c.Command3x); ok {
			return h(&invoke.Call{
				Ctx: p.Ctx, Name: c.Command3x, Args: rest, Env: p.Env,
				Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term, Handlers: p.Table,
			}), true
		}
		return p.fail(invoke.NotPorted(c.Name())), true
	}
	p.Args = append(append([]string{c.Command3x}, c.Prefix...), rest...)
	return 0, false
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
		hteCmd(p, status), delegateCmd(p, status, "logs", "Find and reclaim disk space from log files"),
		delegateCmd(p, status, "wsl", "Inspect and repair WSL and convert paths"),
		delegateCmd(p, status, "win", "Inspect the Windows host from WSL (read-only)"),
		delegateCmd(p, status, "runtime", "Resolve a php or node version to its binary, without nvm.sh"),
		delegateCmd(p, status, "run", "Run a command with its output shaped and its exit status kept"),
		delegateCmd(p, status, "agent-functions", "List the llm_* and agent_* functions for direct calls"),
		agentsHintCmd(p, status), compatCmd(p, status))
	return root
}

// delegateCmd builds the cobra command of a 3.x top-level command whose
// whole tree lives in its table handler (like hte): the arguments are
// handed to the handler raw, and completion is relayed through it
// (gash __complete NAME WORDS asks the handler, which answers a leading
// __complete).
func delegateCmd(p *Process, status *int, name, short string) *cobra.Command {
	return &cobra.Command{
		Use:                name,
		Short:              short,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			h, ok := p.Table.LookupCommand(name)
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out bytes.Buffer
			h(&invoke.Call{
				Ctx: p.Ctx, Name: name, Args: append(append([]string{cobra.ShellCompRequestCmd}, args...), toComplete),
				Env: p.Env, Stdin: p.Stdin, Stdout: &out, Stderr: io.Discard, Term: p.Term, Handlers: p.Table,
			})
			return parseCompletion(out.String())
		},
		RunE: func(_ *cobra.Command, args []string) error {
			if h, ok := p.Table.LookupCommand(name); ok {
				*status = h(&invoke.Call{
					Ctx: p.Ctx, Name: name, Args: args, Env: p.Env,
					Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term, Handlers: p.Table,
				})
				return nil
			}
			*status = p.fail(invoke.NotPorted("gash " + name))
			return nil
		},
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

// updateLong is the help of gash update (upgrade.md 3.6).
const updateLong = `Update the Gash install (~/.gash, or $GASH_DIR) to the highest release tag
of its origin, verified before anything changes:

  1. refuses a working tree with local modifications;
  2. fetches the tags and picks the highest release (never an older one
     than the checkout: a pre-release or a later commit stays as it is);
  3. checks the tag signature, or for an unsigned tag the Sigstore
     signature of its release with cosign (GASH_REQUIRE_SIGNED=1 refuses
     a tag with neither verified);
  4. downloads the release binary into
     ${XDG_DATA_HOME:-~/.local/share}/gash/versions/<version>/ and checks it
     against the hash the tagged tree pins (release/SHA256SUMS), else the
     release's checksums.txt (GASH_REQUIRE_SIGNED=1 accepts only a pinned
     binary); the binary must answer "gash version --json" with that version;
  5. checks out the tag and loads it in two new shells (headless and
     interactive); if either fails, the previous version is checked out
     again, its binary still installed.

The previous version and its binary are kept: "gash update --rollback"
returns to it (and a second rollback comes back). gash_upgrade runs the same
steps. GASH_RELEASE_URL replaces the release download URL (file:// works).

Exit status: 0 updated or nothing to do, 1 refused or failed (nothing
changed, or the previous version restored), 2 a bad argument.`

func updateCmd(p *Process, status *int) *cobra.Command {
	var rollback bool
	c := &cobra.Command{
		Use:   "update [--rollback]",
		Short: "Update Gash to the latest verified release",
		Long:  updateLong,
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			h, ok := p.Table.LookupCommand("update")
			if !ok {
				*status = p.fail(invoke.NotPorted("gash update"))
				return nil
			}
			var args []string
			if rollback {
				args = []string{"--rollback"}
			}
			*status = h(&invoke.Call{
				Ctx: p.Ctx, Name: "update", Args: args, Env: p.Env,
				Stdin: p.Stdin, Stdout: p.Stdout, Stderr: p.Stderr, Term: p.Term, Handlers: p.Table,
			})
			return nil
		},
	}
	c.Flags().BoolVar(&rollback, "rollback", false, "return to the version the last update left")
	return c
}

func hteCmd(p *Process, status *int) *cobra.Command {
	return &cobra.Command{
		Use:                "hte",
		Short:              "Manage the local Apache and PHP-FPM test environment",
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		// The hte tree lives in its handler: gash __complete hte WORDS asks
		// it (htecmd answers a leading __complete) and relays the answer.
		ValidArgsFunction: func(_ *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
			h, ok := p.Table.LookupCommand("hte")
			if !ok {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			var out bytes.Buffer
			h(&invoke.Call{
				Ctx: p.Ctx, Name: "hte", Args: append(append([]string{cobra.ShellCompRequestCmd}, args...), toComplete),
				Env: p.Env, Stdin: p.Stdin, Stdout: &out, Stderr: io.Discard, Term: p.Term, Handlers: p.Table,
			})
			return parseCompletion(out.String())
		},
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

// parseCompletion reads the answer of a cobra __complete request: one
// completion per line, then ":DIRECTIVE".
func parseCompletion(out string) ([]cobra.Completion, cobra.ShellCompDirective) {
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	directive := cobra.ShellCompDirectiveDefault
	var comps []cobra.Completion
	for _, l := range lines {
		if d, ok := strings.CutPrefix(l, ":"); ok {
			if n, err := strconv.Atoi(d); err == nil {
				directive = cobra.ShellCompDirective(n)
			}
			continue
		}
		if l != "" {
			comps = append(comps, l)
		}
	}
	return comps, directive
}
