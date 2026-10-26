// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

// --host ALIAS on the agent functions (plan S5). A function opts in with
// one line at its registration, t.Handle(name, hoster.Wrap(name, spec, h)):
// without --host the wrapper calls h unchanged (byte-identical output);
// with it, the call runs on the registered host (see gash.go), its output
// is the local shape plus "host":"ALIAS" (AddHost), and the rails of
// rails.go apply to the path arguments before anything connects.
//
// --host may be repeated: the hosts run in parallel and the outputs are
// combined (CombineHosts).

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/runner"
)

// sharedValues are the value options of the shared output layer
// (internal/llmfmt): every spec knows them, so their values are never
// read as --host or as a path.
var sharedValues = []string{"--format", "--width", "--max-lines", "--head", "--tail", "-j", "--jobs", "--timeout"}

// Spec says how a function runs on a host.
type Spec struct {
	// Values are the function's options that take a value.
	Values []string
	// PathFrom is the index (among the positionals) of the first one that
	// is a remote path; every positional from there is one. -1: none.
	PathFrom int
	// PathMax is how many positionals from PathFrom are paths; 0: all.
	PathMax int
	// PathOpts are the value options whose (first) value is a remote path.
	PathOpts []string
	// Pairs are the options that take two values (llm_wait --file-match
	// FILE RE).
	Pairs []string
	// Also gives more forms of a path argument for the rails to check as
	// well (llm_read PATH:A-B is checked as PATH too); nil: none.
	Also func(arg string) []string
	// Write marks an agent_* function: its paths get CheckWrite (with
	// --force read from the arguments) instead of CheckRead.
	Write bool
	// Keep leaves the output as it is: it is the user's data (llm_config
	// prints the file), so no "host" key is added.
	Keep bool
	// Fallback builds the POSIX sh side of the call for args; nil, or ok
	// false, when only the gash binary can answer them.
	Fallback func(c *invoke.Call, nonce string, args []string) (plan *Plan, ok bool)
}

// Plan is a POSIX sh fallback for one call.
type Plan struct {
	// Err refuses the arguments before anything connects, exactly as the
	// local function refuses them.
	Err *contract.Envelope
	// Script is the sh body. It runs in a subshell after the dispatch
	// lines; its exit status is the call's unless Finish says otherwise.
	Script string
	// Finish turns what the body printed into the local output: stdout,
	// stderr and the exit status. nil passes them through.
	Finish func(h Host, stdout, stderr []byte, status int) ([]byte, []byte, int)
}

// Hoster wraps handlers with --host, through the module's process seam.
type Hoster struct {
	// Run starts ssh.
	Run runner.Runner
	// Exe returns the path of the running binary (nil: os.Executable).
	Exe func() (string, error)
}

// HostOut is what one host answered.
type HostOut struct {
	Host           Host
	Stdout, Stderr []byte
	RC             int
	// Err is a transport failure (unreachable, no binary); the call
	// fails with it.
	Err *contract.Envelope
}

// Wrap returns h with --host ALIAS.
func (hs Hoster) Wrap(name string, spec Spec, h invoke.Handler) invoke.Handler {
	return func(c *invoke.Call) int {
		aliases, rest, found, missing := splitHost(c.Args, spec.values(), spec.Pairs)
		if !found {
			return h(c)
		}
		for _, a := range c.Args {
			if a == "--" {
				break
			}
			if a == "--help" || a == "-h" {
				return h(c)
			}
		}
		if missing {
			return c.Fail(contract.New("missing_value", "--host", "--host needs a host alias"))
		}
		hosts := make([]Host, 0, len(aliases))
		for _, a := range aliases {
			host, e := Resolve(c.Env.Get, a)
			if e != nil {
				return c.Fail(*e)
			}
			hosts = append(hosts, host)
		}
		// The arguments first, as the local function checks them (its
		// fallback plan refuses what it refuses), then the path rails.
		nonce := NewNonce()
		var plan *Plan
		if spec.Fallback != nil {
			if p, ok := spec.Fallback(c, nonce, rest); ok {
				if p.Err != nil {
					return c.Fail(*p.Err)
				}
				plan = p
			}
		}
		if e := spec.check(rest); e != nil {
			return c.Fail(*e)
		}
		outs := hs.RunHosts(c, name, hosts, rest, nonce, plan)
		return writeOuts(c, spec, outs)
	}
}

// RunHosts runs name ARGS on every host (in parallel, input order kept),
// with the fallback plan built for nonce (nil: binary only).
func (hs Hoster) RunHosts(c *invoke.Call, name string, hosts []Host, args []string, nonce string, plan *Plan) []HostOut {
	bin, haveBin := LocalBinary(hs.Exe)
	t := Transport{Run: hs.Run, Env: c.Env}
	return llmfmt.Map(c.Ctx, llmfmt.NetworkJobs, 0, hosts, func(ctx context.Context, h Host) HostOut {
		return runOne(ctx, t, bin, haveBin, name, h, args, nonce, plan)
	})
}

func runOne(ctx context.Context, t Transport, bin Binary, haveBin bool, name string, h Host, args []string, nonce string, plan *Plan) HostOut {
	body := ""
	if plan != nil {
		body = plan.Script
	}
	r := t.Exec(ctx, h, nonce, DispatchScript(nonce, bin, haveBin, name, args, body), nil, 0)
	if r.Tag == "ship" {
		f, err := os.Open(bin.Path)
		if err != nil {
			return HostOut{Host: h, Err: unavailable(name, h, "the local binary cannot be read: "+err.Error())}
		}
		r = t.Exec(ctx, h, nonce, ShipScript(nonce, bin, name, args, body), f, 0)
		_ = f.Close()
	}
	if e := r.Failure(h); e != nil {
		return HostOut{Host: h, Err: e}
	}
	switch r.Tag {
	case "rc":
		return HostOut{Host: h, Stdout: r.Stdout, Stderr: r.Stderr, RC: r.Status}
	case "fb":
		out := HostOut{Host: h, Stdout: r.Stdout, Stderr: r.Stderr, RC: r.Status}
		if plan.Finish != nil {
			out.Stdout, out.Stderr, out.RC = plan.Finish(h, r.Stdout, r.Stderr, r.Status)
		}
		return out
	case "shipfail":
		return HostOut{Host: h, Err: unavailable(name, h, "gash could not be installed in ~/"+bin.Dir+" there, and these arguments have no POSIX fallback")}
	case "nobin":
		why := "no gash binary for its platform (" + r.Detail + ")"
		if bin.Platform == "" || !haveBin {
			why = "no gash binary can be shipped from this machine"
		}
		return HostOut{Host: h, Err: unavailable(name, h, why+", and these arguments have no POSIX fallback")}
	}
	e := contract.New("remote_protocol_error", h.Alias+": unexpected trailer "+strconv.Quote(r.Tag), "Report this to the Gash maintainers")
	return HostOut{Host: h, Err: &e}
}

func unavailable(name string, h Host, why string) *contract.Envelope {
	e := contract.New("remote_binary_unavailable", name+" on "+h.Alias+": "+why,
		"Run it with arguments the fallback covers (see the function's --help), or install gash on the host")
	return &e
}

// writeOuts writes the answers: one host as it answered plus "host";
// several combined (CombineHosts). A transport failure, or the first host
// (in input order) whose status is not 0, fails the call with its error.
func writeOuts(c *invoke.Call, spec Spec, outs []HostOut) int {
	for _, o := range outs {
		if o.Err != nil {
			return c.Fail(*o.Err)
		}
	}
	for _, o := range outs {
		if o.RC != 0 {
			_, _ = c.Stdout.Write(o.Stdout)
			_, _ = c.Stderr.Write(o.Stderr)
			return o.RC
		}
	}
	if len(outs) == 1 {
		o := outs[0]
		out := o.Stdout
		if !spec.Keep {
			out = AddHost(out, o.Host.Alias)
		}
		_, _ = c.Stdout.Write(out)
		_, _ = c.Stderr.Write(o.Stderr)
		return 0
	}
	_, _ = c.Stdout.Write(CombineHosts(outs, spec.Keep))
	for _, o := range outs {
		_, _ = c.Stderr.Write(o.Stderr)
	}
	return 0
}

// SplitHost takes every "--host ALIAS" out of args, skipping the values of
// the value options (so a value "--host" is never one). A "--" ends the
// options: it and every word after it are kept as they are (the command of
// llm_docker_exec). missing reports a --host with no value after it.
func SplitHost(args, values []string) (aliases, rest []string, found, missing bool) {
	return splitHost(args, values, nil)
}

func splitHost(args, values, pairs []string) (aliases, rest []string, found, missing bool) {
	rest = make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return aliases, append(rest, args[i:]...), found, missing
		case a == "--host":
			found = true
			next := args[i+1:]
			if len(next) == 0 {
				missing = true
				continue
			}
			aliases = append(aliases, next[0])
			i++
		case contains(values, a) || contains(pairs, a):
			n := 1
			if contains(pairs, a) {
				n = 2
			}
			n = min(n, len(args)-i-1)
			rest = append(rest, args[i:i+1+n]...)
			i += n
		default:
			rest = append(rest, a)
		}
	}
	return aliases, rest, found, missing
}

func (s Spec) values() []string { return append(append([]string{}, sharedValues...), s.Values...) }

// Paths are the remote path arguments of args (without --host), each
// followed by its Also forms.
func (s Spec) Paths(args []string) []string {
	values := s.values()
	var out []string
	add := func(p string) {
		out = append(out, p)
		if s.Also != nil {
			out = append(out, s.Also(p)...)
		}
	}
	pos := 0
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if contains(values, a) || contains(s.Pairs, a) {
			if i+1 < len(args) && contains(s.PathOpts, a) {
				add(args[i+1])
			}
			i++
			if contains(s.Pairs, a) {
				i++
			}
			continue
		}
		if strings.HasPrefix(a, "-") && a != "-" {
			continue
		}
		if s.PathFrom >= 0 && pos >= s.PathFrom && (s.PathMax == 0 || pos < s.PathFrom+s.PathMax) {
			add(a)
		}
		pos++
	}
	return out
}

// check applies the rails to every path argument. A "-" (stdin) is
// refused: the local stdin is never sent to a host.
func (s Spec) check(args []string) *contract.Envelope {
	force := contains(args, "--force")
	for _, p := range s.Paths(args) {
		var e *contract.Envelope
		switch {
		case p == "-":
			env := contract.New("unexpected_argument", "-", "With --host, name a file on the host: the local stdin is not sent")
			e = &env
		case s.Write:
			e = CheckWrite(p, force)
		default:
			e = CheckRead(p)
		}
		if e != nil {
			return e
		}
	}
	return nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// AddHost adds "host":"ALIAS" to an output, as the first key:
//
//   - a JSON object gets it at the top level;
//   - a JSON array whose elements are all objects: every element gets it
//     (the shape stays an array, and the rows of several hosts can be
//     concatenated);
//   - JSON Lines (one object per line): every line gets it;
//   - anything else (text, TSV, aligned tables, arrays of scalars, JSON
//     scalars) is left byte for byte as the local call prints it.
//
// An object that already has a top-level "host" key keeps it as it is.
func AddHost(out []byte, alias string) []byte {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return out
	}
	key, _ := json.Marshal(alias)
	switch trimmed[0] {
	case '{':
		if json.Valid(trimmed) {
			return insertKey(out, bytes.IndexByte(out, '{'), key)
		}
		return addHostLines(out, key)
	case '[':
		if json.Valid(trimmed) {
			return addHostArray(out, key)
		}
	}
	return out
}

// insertKey inserts "host":KEY after the { at out[at], unless the object
// there already has a top-level "host". An indented object gets the key on
// its own line with the indentation of the line after the {.
func insertKey(out []byte, at int, key []byte) []byte {
	if hasTopKey(out[at:], "host") {
		return out
	}
	j := at + 1
	for j < len(out) && (out[j] == ' ' || out[j] == '\t' || out[j] == '\n' || out[j] == '\r') {
		j++
	}
	var ins []byte
	switch {
	case j < len(out) && out[j] == '}':
		ins = append([]byte(`"host":`), key...)
	case bytes.IndexByte(out[at+1:j], '\n') >= 0:
		ws := out[at+1 : j]
		indent := ws[bytes.LastIndexByte(ws, '\n')+1:]
		ins = append(append(append([]byte("\n"), indent...), `"host": `...), key...)
		ins = append(ins, ',')
	default:
		ins = append(append([]byte(`"host":`), key...), ',')
	}
	return slices.Concat(out[:at+1], ins, out[at+1:])
}

// hasTopKey reports whether the JSON object at the start of b has key at
// its top level.
func hasTopKey(b []byte, key string) bool {
	dec := json.NewDecoder(bytes.NewReader(b))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return false
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return false
		}
		if k, ok := t.(string); ok && k == key {
			return true
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return false
		}
	}
	return false
}

// addHostArray inserts the key into every element of a JSON array of
// objects; an array with another element is returned unchanged.
func addHostArray(out []byte, key []byte) []byte {
	start := bytes.IndexByte(out, '[')
	dec := json.NewDecoder(bytes.NewReader(out[start:]))
	if _, err := dec.Token(); err != nil {
		return out
	}
	var at []int
	for dec.More() {
		off := int(dec.InputOffset())
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return out
		}
		if len(raw) == 0 || raw[0] != '{' {
			return out
		}
		// The element starts at its first {, after the separator.
		at = append(at, start+off+bytes.IndexByte(out[start+off:], '{'))
	}
	for i := len(at) - 1; i >= 0; i-- {
		out = insertKey(out, at[i], key)
	}
	return out
}

// addHostLines handles JSON Lines: every non-empty line one object, else
// the output is returned unchanged.
func addHostLines(out []byte, key []byte) []byte {
	lines := bytes.SplitAfter(out, []byte("\n"))
	var b bytes.Buffer
	for _, l := range lines {
		t := bytes.TrimSpace(l)
		if len(t) == 0 {
			b.Write(l)
			continue
		}
		if t[0] != '{' || !json.Valid(t) {
			return out
		}
		b.Write(insertKey(l, bytes.IndexByte(l, '{'), key))
	}
	return b.Bytes()
}

// CombineHosts joins the answers of several hosts, in input order, each
// with its "host" key: JSON objects become one array of them, arrays of
// objects one array of all their rows, JSON Lines all their lines; text
// becomes one block per host after a "==> ALIAS <==" line. keep leaves the
// outputs undecorated (blocks for text and data alike).
func CombineHosts(outs []HostOut, keep bool) []byte {
	kind := ""
	for _, o := range outs {
		k := jsonKind(o.Stdout)
		if keep || k == "" || (kind != "" && k != kind) {
			kind = "text"
			break
		}
		kind = k
	}
	var b bytes.Buffer
	switch kind {
	case "object", "array":
		b.WriteByte('[')
		n := 0
		for _, o := range outs {
			d := bytes.TrimSpace(AddHost(o.Stdout, o.Host.Alias))
			if kind == "array" {
				d = bytes.TrimSpace(d[1 : len(d)-1])
			}
			if len(d) == 0 {
				continue
			}
			if n > 0 {
				b.WriteByte(',')
			}
			b.Write(d)
			n++
		}
		b.WriteString("]\n")
	case "lines":
		for _, o := range outs {
			b.Write(AddHost(o.Stdout, o.Host.Alias))
		}
	default:
		for _, o := range outs {
			b.WriteString("==> " + o.Host.Alias + " <==\n")
			b.Write(o.Stdout)
			if len(o.Stdout) > 0 && o.Stdout[len(o.Stdout)-1] != '\n' {
				b.WriteByte('\n')
			}
		}
	}
	return b.Bytes()
}

// jsonKind classifies an output for CombineHosts: "object", "array" (of
// objects), "lines" (JSON Lines) or "" (anything else).
func jsonKind(out []byte) string {
	t := bytes.TrimSpace(out)
	if len(t) == 0 || !(t[0] == '{' || t[0] == '[') {
		return ""
	}
	if json.Valid(t) {
		if t[0] == '{' {
			return "object"
		}
		var elems []json.RawMessage
		if json.Unmarshal(t, &elems) != nil {
			return ""
		}
		for _, e := range elems {
			if len(e) == 0 || e[0] != '{' {
				return ""
			}
		}
		return "array"
	}
	for _, l := range bytes.Split(t, []byte("\n")) {
		l = bytes.TrimSpace(l)
		if len(l) > 0 && (l[0] != '{' || !json.Valid(l)) {
			return ""
		}
	}
	return "lines"
}
