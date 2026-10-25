// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package invoke defines how a 2.x function runs in the binary: the Call
// a handler receives (arguments, environment, streams, terminal) and the
// Table that binds handlers to registry names.
//
// A module package exposes one function,
//
//	func Register(t *invoke.Table) {
//		t.Handle("gash_username", username)
//	}
//
// and internal/handlers calls it. Nothing registers itself in init().
package invoke

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/term"
)

// Env is a snapshot of the process environment, which holds the caller's
// shell variables the stub passed.
type Env struct {
	m map[string]string
}

// EnvFrom builds an Env from KEY=VALUE entries (os.Environ()). The first
// entry of a repeated key wins, as getenv does.
func EnvFrom(environ []string) Env {
	m := make(map[string]string, len(environ))
	for _, kv := range environ {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		if _, dup := m[k]; !dup {
			m[k] = v
		}
	}
	return Env{m: m}
}

// EnvMap builds an Env from a map, for tests.
func EnvMap(m map[string]string) Env {
	c := make(map[string]string, len(m))
	for k, v := range m {
		c[k] = v
	}
	return Env{m: c}
}

// Lookup returns the value of name and whether it is set.
func (e Env) Lookup(name string) (string, bool) {
	v, ok := e.m[name]
	return v, ok
}

// Get returns the value of name, or "" when unset.
func (e Env) Get(name string) string { return e.m[name] }

// Environ returns the snapshot as sorted KEY=VALUE entries.
func (e Env) Environ() []string {
	out := make([]string, 0, len(e.m))
	for k, v := range e.m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

// Child is the environment of a command the function runs, as a 2.x
// child saw it: the caller's exported environment. It leaves out the
// GASH__ protocol variables and the shell variables the stub exported for
// this call only (GASH__ADDED), so a setting or a token held in a shell
// variable never reaches a child. Sorted KEY=VALUE entries.
func (e Env) Child() []string {
	drop := map[string]bool{}
	for _, n := range strings.Fields(e.m["GASH__ADDED"]) {
		drop[n] = true
	}
	out := make([]string, 0, len(e.m))
	for k, v := range e.m {
		if !drop[k] && !strings.HasPrefix(k, "GASH__") {
			out = append(out, k+"="+v)
		}
	}
	sort.Strings(out)
	return out
}

// Call is one invocation of a 2.x function.
type Call struct {
	Ctx context.Context
	// Name is the registry name of the function.
	Name string
	// Args are the raw 2.x arguments, unparsed.
	Args   []string
	Env    Env
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	Term   *term.Term
	// Handlers is the table the call was dispatched from, so that a
	// handler can run another function (gash migrate runs gash_migrate).
	Handlers *Table
	// truncated counts the lines or rows an output budget cut (plan S2),
	// for the telemetry line (plan S10).
	truncated int
}

// NoteTruncated records that an output budget cut k lines or rows.
func (c *Call) NoteTruncated(k int) {
	if k > 0 {
		c.truncated += k
	}
}

// Truncated is what NoteTruncated recorded.
func (c *Call) Truncated() int { return c.truncated }

// Arg returns the i-th argument, or "" (Bash ${N-}).
func (c *Call) Arg(i int) string {
	if i < len(c.Args) {
		return c.Args[i]
	}
	return ""
}

// HelpLoaded reports whether the 2.x help registry would be loaded in the
// caller's shell: the generated gash.sh marks it, and the stub passes
// GASH__HELP=1 or 0. A direct invocation (no stub) has the registry.
func (c *Call) HelpLoaded() bool { return c.Env.Get("GASH__HELP") != "0" }

// Fail writes the envelope of code to stderr and returns 1.
func (c *Call) Fail(e contract.Envelope) int {
	_ = e.Write(c.Stderr)
	return 1
}

// Handler runs a call and returns its exit status.
type Handler func(c *Call) int

// Table binds handlers to registry function names, and to the hidden
// `gash __compat` operations the generated Bash calls.
type Table struct {
	fns    map[string]Handler
	compat map[string]Handler
	cmds   map[string]Handler
}

// NewTable returns an empty table.
func NewTable() *Table {
	return &Table{fns: map[string]Handler{}, compat: map[string]Handler{}, cmds: map[string]Handler{}}
}

// Handle binds h to the 2.x function name. It panics when name is not in
// the registry or already has a handler: both are programming errors that
// the handlers test catches.
func (t *Table) Handle(name string, h Handler) {
	if _, ok := registry.Lookup(name); !ok {
		panic(fmt.Sprintf("invoke: %q is not a registry function", name))
	}
	if _, dup := t.fns[name]; dup {
		panic(fmt.Sprintf("invoke: %q registered twice", name))
	}
	t.fns[name] = h
}

// HandleCompat binds h to a hidden compat operation, named by its words
// after `gash __compat` ("help", "output table", "env load"). The
// operation receives the remaining arguments unparsed.
func (t *Table) HandleCompat(op string, h Handler) {
	if _, dup := t.compat[op]; dup {
		panic(fmt.Sprintf("invoke: compat %q registered twice", op))
	}
	t.compat[op] = h
}

// Lookup returns the handler of a 2.x function.
func (t *Table) Lookup(name string) (Handler, bool) {
	h, ok := t.fns[name]
	return h, ok
}

// HandleCommand binds h to a 3.x top-level command word ("hte"). It panics
// when the word already has a handler: a programming error.
func (t *Table) HandleCommand(word string, h Handler) {
	if _, dup := t.cmds[word]; dup {
		panic(fmt.Sprintf("invoke: command %q registered twice", word))
	}
	t.cmds[word] = h
}

// LookupCommand returns the handler of a 3.x command word.
func (t *Table) LookupCommand(word string) (Handler, bool) {
	h, ok := t.cmds[word]
	return h, ok
}

// LookupCompat returns the handler of the longest compat operation that
// prefixes args, and the arguments after it.
func (t *Table) LookupCompat(args []string) (Handler, string, []string, bool) {
	for n := min(len(args), 3); n > 0; n-- {
		op := strings.Join(args[:n], " ")
		if h, ok := t.compat[op]; ok {
			return h, op, args[n:], true
		}
	}
	return nil, "", nil, false
}

// Names returns the names of the functions that have a handler, sorted.
func (t *Table) Names() []string {
	out := make([]string, 0, len(t.fns))
	for n := range t.fns {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// CompatOps returns the registered compat operations, sorted.
func (t *Table) CompatOps() []string {
	out := make([]string, 0, len(t.compat))
	for n := range t.compat {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Run dispatches name to its handler, or fails with the internal_error
// envelope of a function that has no Go handler yet (development builds
// during the port).
func (t *Table) Run(c *Call) int {
	c.Handlers = t
	h, ok := t.fns[c.Name]
	if !ok {
		return c.Fail(NotPorted(c.Name))
	}
	if Telemetered(c.Name) {
		return WithTelemetry(c, c.Name, func() int { return h(c) })
	}
	return h(c)
}

// NotPorted is the envelope of a function without a Go handler.
func NotPorted(name string) contract.Envelope {
	return contract.New("internal_error",
		name+" is not implemented in this gash build",
		"Gash 3.0 development build: this function has no Go handler yet")
}
