// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package htecmd is the command line of gash hte (design/hte.md 9): a
// cobra tree with ordinary flags over the hte.Engine API. Every command
// takes --json (one compact JSON object on stdout); without it the data
// goes to stdout as text and the messages (OK, warnings, hints) to
// stderr. An error is one envelope of the contract on stderr and exit
// status 1; doctor exits 1 when a check fails; a command that did part of
// its work prints that part, then the envelope (a partial failure never
// exits 0). A required flag missing on a terminal is asked for; headless
// or without a terminal it is missing_value.
//
// It runs as the handler of the 3.x command `hte` (Run), with the hidden
// commands the system calls: `__root` (the root step, under sudo),
// `__unit start|stop` (the systemd unit and the LaunchAgent) and
// `__autostart` (the shell autostart of gash init bash).
package htecmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// Options are the seams of the command line; the zero value runs against
// the real process.
type Options struct {
	// Engine builds the engine of a call; nil is hte.ProcessEngine with
	// the call's environment.
	Engine func(c *invoke.Call, interactive bool) (*hte.Engine, error)
	// RootEnv is the root step's view of the system; nil is
	// rootops.ProcessEnv.
	RootEnv func() rootops.Env
}

// Run is the handler of `gash hte ARGS...` on the real process.
func Run(c *invoke.Call) int { return Handler(Options{})(c) }

// Register binds gash hte to the table.
func Register(t *invoke.Table) { t.HandleCommand("hte", Run) }

// Handler returns the handler of `gash hte` with opt's seams.
func Handler(opt Options) invoke.Handler {
	return func(c *invoke.Call) int {
		if c.Ctx == nil {
			c.Ctx = context.Background()
		}
		r := &run{opt: opt, c: c}
		// hte runs under a stand-in for the gash root, so that help and
		// hints name the whole command (gash hte site create).
		top := &cobra.Command{Use: "gash", SilenceErrors: true, SilenceUsage: true}
		top.CompletionOptions.DisableDefaultCmd = true
		top.SetHelpCommand(&cobra.Command{Use: "__help", Hidden: true})
		top.AddCommand(r.tree())
		args := append([]string{"hte"}, c.Args...)
		// gash __complete hte WORDS... reaches here as Args __complete
		// WORDS...: cobra's completion request is a command of the root.
		if len(c.Args) > 0 && c.Args[0] == cobra.ShellCompRequestCmd {
			args = append([]string{cobra.ShellCompRequestCmd, "hte"}, c.Args[1:]...)
		}
		top.SetArgs(args)
		top.SetIn(c.Stdin)
		top.SetOut(c.Stdout)
		top.SetErr(c.Stderr)
		if cmd, err := top.ExecuteContextC(c.Ctx); err != nil {
			return r.envelope(usageEnvelope(err, cmd))
		}
		return r.status
	}
}

// run is one call: its flags and exit status.
type run struct {
	opt    Options
	c      *invoke.Call
	asJSON bool
	status int
	eng    *hte.Engine
}

// interactive reports a terminal to ask on: not headless, stdin and
// stderr terminals. sudo may then prompt too.
func (r *run) interactive() bool {
	t := r.c.Term
	return t != nil && !t.Headless() && t.StdinTTY() && t.StderrTTY()
}

func (r *run) engine() (*hte.Engine, error) {
	if r.eng != nil {
		return r.eng, nil
	}
	build := r.opt.Engine
	if build == nil {
		build = func(c *invoke.Call, interactive bool) (*hte.Engine, error) {
			e, err := hte.ProcessEngine(interactive)
			if err != nil {
				return nil, err
			}
			e.Getenv = c.Env.Get
			return e, nil
		}
	}
	e, err := build(r.c, r.interactive())
	if err != nil {
		return nil, err
	}
	e.BeforeSudo = r.beforeSudo
	r.eng = e
	return e, nil
}

// beforeSudo shows the root steps before sudo starts: on stderr, so that
// the user sees them above sudo's password prompt. With --json and no
// terminal nobody reads them there: the result carries them.
func (r *run) beforeSudo(steps []string) {
	if r.asJSON && !r.interactive() {
		return
	}
	r.say().Info("sudo runs these steps as root:")
	for i, s := range steps {
		fmt.Fprintf(r.c.Stderr, "  %d. %s\n", i+1, s)
	}
}

func (r *run) style() ui.Style { return ui.StyleOf(r.c.Term, false) }

// say is the printer of the messages, on stderr.
func (r *run) say() ui.Printer { return r.style().Printer(r.c.Stderr) }

// warn prints warnings in text mode (the JSON carries them).
func (r *run) warn(warnings []string) {
	for _, w := range warnings {
		r.say().Warning(w)
	}
}

// envelope writes the envelope of e and returns 1.
func (r *run) envelope(e contract.Envelope) int {
	_ = e.Write(r.c.Stderr)
	r.status = 1
	return 1
}

// Envelope is the contract envelope of an error of the engine: its code
// and the code's action; any other error is internal_error.
func Envelope(err error) contract.Envelope {
	if e, ok := hteerr.As(err); ok {
		return contract.Envelope{Code: string(e.Code), Details: e.Details, Action: contract.Action(e.Action()), Hint: e.Hint}
	}
	return contract.New("internal_error", err.Error(), "run gash hte doctor")
}

// writeJSON writes v as one compact JSON line.
func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func isNil(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// result prints what a command returned: the JSON object, or the text of
// human; then the envelope of err, if any. A result that came with an
// error is printed first: the caller sees what was done.
func result[T any](r *run, v T, err error, human func(T)) {
	if !isNil(v) {
		if r.asJSON {
			writeJSON(r.c.Stdout, v)
		} else {
			human(v)
		}
	}
	if err != nil {
		r.envelope(Envelope(err))
	}
}

// do runs fn on the engine and prints its result with human.
func do[T any](r *run, fn func(context.Context, *hte.Engine) (T, error), human func(T)) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		e, err := r.engine()
		if err != nil {
			r.envelope(Envelope(err))
			return nil
		}
		v, err := fn(cmd.Context(), e)
		result(r, v, err, human)
		return nil
	}
}

// fail ends a command with err before the engine runs.
func (r *run) fail(err error) error {
	r.envelope(Envelope(err))
	return nil
}

// required makes sure *v is set: on a terminal it asks for it (label on
// stderr, one line from stdin), else it is missing_value.
func (r *run) required(flag, label string, v *string) error {
	if *v != "" {
		return nil
	}
	if r.interactive() {
		_, _ = io.WriteString(r.c.Stderr, label+": ")
		line, ok := ui.ReadLine(r.c.Stdin)
		if line = strings.TrimSpace(line); ok && line != "" {
			*v = line
			return nil
		}
	}
	return hteerr.Newf(hteerr.MissingValue, "pass --"+flag, "--%s is required", flag)
}

// pair reads a --NAME/--no-NAME pair: nil when neither is given,
// conflicting_options when both are.
func pair(cmd *cobra.Command, name string) (*bool, error) {
	on, off := cmd.Flags().Changed(name), cmd.Flags().Changed("no-"+name)
	switch {
	case on && off:
		return nil, hteerr.Newf(hteerr.ConflictingOptions, "pass one of them", "--%s and --no-%s contradict each other", name, name)
	case on:
		v := true
		return &v, nil
	case off:
		v := false
		return &v, nil
	}
	return nil, nil
}

// usageEnvelope maps a cobra or pflag error to the envelope of its kind,
// as the command line of the other 3.x commands does.
func usageEnvelope(err error, cmd *cobra.Command) contract.Envelope {
	msg := err.Error()
	code := "unexpected_argument"
	switch {
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		code = "unknown_option"
	case strings.HasPrefix(msg, "flag needs an argument"):
		code = "missing_value"
	case missingArg(msg):
		code = "missing_value"
	}
	path := "gash hte"
	if cmd != nil && cmd.HasParent() {
		path = cmd.CommandPath()
	}
	return contract.New(code, msg, "Run: "+path+" --help")
}

// missingArg reports cobra's error for too few positional arguments
// ("accepts 2 arg(s), received 1").
func missingArg(msg string) bool {
	var want, got int
	n, _ := fmt.Sscanf(msg, "accepts %d arg(s), received %d", &want, &got)
	return n == 2 && got < want
}

// group is a command that only holds subcommands: without one it prints
// its help; an unknown one is unexpected_argument.
func group(use, short string, subs ...*cobra.Command) *cobra.Command {
	c := &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	c.AddCommand(subs...)
	return c
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
