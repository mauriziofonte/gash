// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/proctab"
)

// procsCmdWidth is the default cut of the cmd field of the 3.4 llm_procs
// rows: command lines run to tens of kilobytes (a Java classpath), so
// they are cut unless --width says otherwise (plan R2).
const procsCmdWidth = 200

// procsListCap is the default row budget of a 3.4 listing with no target
// (no --pid, --name or --port): the first 100 rows, the rest counted in
// the metadata line.
const procsListCap = 100

// procsQuery is what a 3.4 llm_procs call asks (plan P2): explicit PIDs,
// or the processes of a port or a name, or all of them; live ones only;
// with the tree below each.
type procsQuery struct {
	pids  []int
	name  string
	port  string
	alive bool
	tree  bool
}

// active reports whether the call uses a 3.4 mode (and so the 3.4 row
// shape); without one llm_procs prints exactly what it printed before.
func (q procsQuery) active() bool { return q.pids != nil || q.alive || q.tree }

// takePIDs reads the values of --pid: every word up to the next option,
// at least one, each a plain integer (invalid_pid otherwise).
func takePIDs(c *invoke.Call, args []string) ([]int, int, bool) {
	n := 1
	var pids []int
	for n < len(args) && (args[n] == "" || args[n][0] != '-') {
		v := args[n]
		if !uintRE.MatchString(v) {
			fail(c, "invalid_pid", "Value '"+v+"' is not a process id", "Pass process ids as plain integers: --pid 1234 5678")
			return nil, 0, false
		}
		pids = append(pids, atoi(v))
		n++
	}
	if len(pids) == 0 {
		fail(c, "missing_value", args[0], "--pid needs one or more process ids")
		return nil, 0, false
	}
	return pids, n, true
}

// procTable is the process snapshot of a call: ps from the caller's PATH
// (the seam the tests fake), else a Linux /proc; ps_not_found without
// either.
func (m *Module) procTable(c *invoke.Call) (*proctab.Table, bool) {
	ps, _ := m.lookPath(c, "ps")
	t, err := proctab.Snapshot(c.Ctx, m.run, ps, append(c.Env.Child(), "LC_ALL=C"))
	if err != nil {
		fail(c, "ps_not_found", "Cannot list processes: no ps and no /proc",
			"Install procps (ps) to list processes. Skip this check")
		return nil, false
	}
	return t, true
}

// procs34 answers the 3.4 modes of llm_procs: [{pid,name,cmd,alive}] (and
// ppid and depth with --tree). Explicit --pid targets come in the given
// order, dead ones as alive:false rows; the other listings come by PID
// and leave out the caller's own chain (this process, its ancestors and
// their same-command-line subshells), so --name never finds the shell
// that asked.
func (m *Module) procs34(c *invoke.Call, o *llmfmt.Options, q procsQuery) int {
	var re *regexp.Regexp
	if q.pids == nil && q.port == "" && q.name != "" {
		var err error
		if re, err = regexp.Compile(q.name); err != nil {
			return fail(c, "invalid_regex", q.name, "--name takes a regular expression matched against the full command line")
		}
	}
	t, ok := m.procTable(c)
	if !ok {
		return 1
	}
	self := t.Callers(m.pid())
	var roots []int
	explicit := true
	switch {
	case q.pids != nil:
		roots = q.pids
	case q.port != "":
		roots = m.portPIDs(c, q.port)
	case re != nil:
		for _, p := range t.Match(re.MatchString, self) {
			roots = append(roots, p.PID)
		}
		explicit = false
	default:
		for _, p := range t.Procs() {
			if !self[p.PID] {
				roots = append(roots, p.PID)
			}
		}
		explicit = false
		if !o.Capped() {
			o.Head = procsListCap
		}
	}
	type row struct {
		node  proctab.Node
		found bool
	}
	var rows []row
	seen := map[int]bool{}
	for _, pid := range roots {
		if seen[pid] && explicit {
			continue
		}
		p, found := t.Lookup(pid)
		if !found {
			seen[pid] = true
			rows = append(rows, row{node: proctab.Node{Proc: proctab.Proc{PID: pid}}})
			continue
		}
		if !q.tree {
			seen[pid] = true
			rows = append(rows, row{node: proctab.Node{Proc: p}, found: true})
			continue
		}
		for _, n := range t.Tree(pid, self, seen) {
			rows = append(rows, row{node: n, found: true})
		}
	}
	cols := []string{"pid", "name", "cmd", "alive"}
	if q.tree {
		cols = append(cols, "ppid", "depth")
	}
	tbl := llmfmt.Table{Cols: cols}
	for _, r := range rows {
		alive := r.found && r.node.Alive()
		if q.alive && !alive {
			continue
		}
		cmd := r.node.Cmd
		if o.Width == 0 {
			cmd = llmfmt.Cut(cmd, procsCmdWidth)
		}
		v := []any{r.node.PID, r.node.Name, cmd, alive}
		if q.tree {
			var ppid any
			if r.found {
				ppid = r.node.PPID
			}
			v = append(v, ppid, r.node.Depth)
		}
		tbl.Rows = append(tbl.Rows, v)
	}
	o.Write(c, o.FormatOr(llmfmt.JSON), tbl, nil)
	return 0
}

// portPIDs are the processes with a socket on port (lsof -i :PORT -t), by
// PID; none without lsof.
func (m *Module) portPIDs(c *invoke.Call, port string) []int {
	lsof, ok := m.lookPath(c, "lsof")
	if !ok {
		return nil
	}
	set := map[int]bool{}
	for _, w := range readWords(m.output(c, nil, lsof, "-i", ":"+port, "-t")) {
		if digitsRE.MatchString(w) && len(w) <= 9 {
			set[atoi(w)] = true
		}
	}
	out := make([]int, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}

// procsLegacyTable writes the rows of a 2.x mode (the no-filter listing,
// --name, --port) in a --format or under a budget: the same keys, in the
// same order, numbers as the JSON numbers ps printed.
func procsLegacyTable(c *invoke.Call, o *llmfmt.Options, items []string) int {
	var cols []string
	var rows [][]any
	for _, it := range items {
		var obj map[string]json.RawMessage
		dec := json.NewDecoder(strings.NewReader(it))
		if err := dec.Decode(&obj); err != nil {
			continue
		}
		if cols == nil {
			cols = objectKeys(it)
		}
		r := make([]any, len(cols))
		for i, k := range cols {
			raw := obj[k]
			var s string
			if json.Unmarshal(raw, &s) == nil {
				r[i] = s
			} else if len(raw) > 0 {
				r[i] = json.Number(string(raw))
			}
		}
		rows = append(rows, r)
	}
	if cols == nil {
		cols = []string{"pid", "name"}
	}
	o.Write(c, o.FormatOr(llmfmt.JSON), llmfmt.Table{Cols: cols, Rows: rows}, nil)
	return 0
}

// objectKeys are the keys of a flat JSON object, in their order.
func objectKeys(obj string) []string {
	dec := json.NewDecoder(strings.NewReader(obj))
	var keys []string
	if _, err := dec.Token(); err != nil {
		return nil
	}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return keys
		}
		k, _ := tok.(string)
		keys = append(keys, k)
		var skip json.RawMessage
		if dec.Decode(&skip) != nil {
			return keys
		}
	}
	return keys
}
