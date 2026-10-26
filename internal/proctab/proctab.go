// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package proctab is a snapshot of the process table for the agent
// functions that look at processes (llm_procs --pid/--alive/--tree,
// llm_wait --gone) and the one that signals them (agent_kill), plan P1
// and P2: one ps listing (or /proc on Linux, cheap enough for a poll),
// the caller's own chain to leave out, name matching and the tree below a
// process. It never signals anything.
package proctab

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"
)

// Proc is one process.
type Proc struct {
	PID, PPID, UID int
	// State is ps's stat column (R, S, D, Z, T, I, ... and modifiers).
	State string
	// Name is the command name (ps comm), Cmd the full command line (ps
	// args; a kernel thread is its [name]).
	Name, Cmd string
}

// Alive reports whether p still runs: a zombie (exited, not yet reaped)
// does not.
func (p Proc) Alive() bool { return !strings.HasPrefix(p.State, "Z") }

// Table is a snapshot of the processes, sorted by PID.
type Table struct {
	procs    []Proc
	index    map[int]int
	children map[int][]int
}

// New builds a Table from procs (any order; a repeated PID keeps the
// first).
func New(procs []Proc) *Table {
	t := &Table{index: map[int]int{}, children: map[int][]int{}}
	for _, p := range procs {
		if _, dup := t.index[p.PID]; dup {
			continue
		}
		t.index[p.PID] = -1
		t.procs = append(t.procs, p)
	}
	sort.Slice(t.procs, func(i, j int) bool { return t.procs[i].PID < t.procs[j].PID })
	for i, p := range t.procs {
		t.index[p.PID] = i
		if p.PPID != p.PID {
			t.children[p.PPID] = append(t.children[p.PPID], p.PID)
		}
	}
	return t
}

// Procs are the processes, by PID.
func (t *Table) Procs() []Proc { return t.procs }

// Lookup returns the process pid.
func (t *Table) Lookup(pid int) (Proc, bool) {
	i, ok := t.index[pid]
	if !ok {
		return Proc{}, false
	}
	return t.procs[i], true
}

// Ancestors is self and its parents, up to but not including PID 1 (a
// process missing from the table ends the chain).
func (t *Table) Ancestors(self int) map[int]bool {
	chain := map[int]bool{}
	p := self
	for i := 0; i < 64 && p > 1; i++ {
		if chain[p] {
			break
		}
		chain[p] = true
		pr, ok := t.Lookup(p)
		if !ok {
			break
		}
		p = pr.PPID
	}
	return chain
}

// Callers is the caller's own chain, which no listing shows and no kill
// reaches: self (the gash process), its ancestors up to but not
// including PID 1, the children of self (the ps it ran), and the children
// of an ancestor with the ancestor's own command line (the subshells a
// shell forks for $(...)), as llm_procs --name leaves them out.
func (t *Table) Callers(self int) map[int]bool {
	chain := t.Ancestors(self)
	out := map[int]bool{}
	for pid := range chain {
		out[pid] = true
		parent, _ := t.Lookup(pid)
		for _, c := range t.children[pid] {
			if pid == self {
				out[c] = true
				continue
			}
			if child, ok := t.Lookup(c); ok && child.Cmd == parent.Cmd {
				out[c] = true
			}
		}
	}
	return out
}

// Match returns the processes whose command line match accepts, by PID,
// leaving out those in skip.
func (t *Table) Match(match func(cmd string) bool, skip map[int]bool) []Proc {
	var out []Proc
	for _, p := range t.procs {
		if !skip[p.PID] && match(p.Cmd) {
			out = append(out, p)
		}
	}
	return out
}

// Node is a process in a tree listing, with its depth below the root (0
// for the root).
type Node struct {
	Proc
	Depth int
}

// Tree returns root and every process below it, depth first, children by
// PID, leaving out those in skip (and what is below them) and those in
// seen (a process already listed under another root), which it updates.
// A root that is not in the table gives no node.
func (t *Table) Tree(root int, skip, seen map[int]bool) []Node {
	r, ok := t.Lookup(root)
	if !ok || seen[root] {
		return nil
	}
	var out []Node
	var walk func(p Proc, depth int)
	walk = func(p Proc, depth int) {
		seen[p.PID] = true
		out = append(out, Node{Proc: p, Depth: depth})
		for _, c := range t.children[p.PID] {
			if skip[c] || seen[c] {
				continue
			}
			if cp, ok := t.Lookup(c); ok {
				walk(cp, depth+1)
			}
		}
	}
	walk(r, 0)
	return out
}

// PSMain and PSComm are the two ps listings a Table is read from: the
// command line comes last in the first and the name last in the second,
// so a blank inside either never shifts a column (Linux and macOS ps both
// take them; a pipe gives the command line at full width).
var (
	PSMain = []string{"-A", "-o", "pid=,ppid=,uid=,stat=,args="}
	PSComm = []string{"-A", "-o", "pid=,comm="}
)

// ErrNoPS is a ps that failed or printed nothing.
var ErrNoPS = errors.New("ps listed no process")

// Snapshot reads the Table with ps (its path, "" when not installed)
// through r with env, else from /proc; ErrNoPS when neither works.
func Snapshot(ctx context.Context, r runner.Runner, ps string, env []string) (*Table, error) {
	if ps != "" {
		if t, err := ReadPS(ctx, r, ps, env); err == nil {
			return t, nil
		}
	}
	if HasProcFS() {
		if t, err := ReadProcFS("/proc"); err == nil {
			return t, nil
		}
	}
	return nil, ErrNoPS
}

// ReadPS reads a Table with the two ps listings, run through r with env.
func ReadPS(ctx context.Context, r runner.Runner, ps string, env []string) (*Table, error) {
	run := func(args []string) ([]byte, error) {
		res, err := r.Run(ctx, runner.Cmd{Path: ps, Args: args, Options: runner.Options{Env: env}})
		if err != nil {
			return nil, err
		}
		if res.ExitCode != 0 || len(res.Stdout) == 0 {
			return nil, ErrNoPS
		}
		return res.Stdout, nil
	}
	main, err := run(PSMain)
	if err != nil {
		return nil, err
	}
	comm, err := run(PSComm)
	if err != nil {
		return nil, err
	}
	return ParsePS(main, comm), nil
}

// ParsePS builds a Table from the PSMain and PSComm listings; a line that
// does not parse is skipped, a process missing from the second listing
// (it started in between) gets the first word of its command line as its
// name.
func ParsePS(main, comm []byte) *Table {
	names := map[int]string{}
	for _, l := range lines(comm) {
		pid, rest, ok := cutInt(l)
		if !ok {
			continue
		}
		names[pid] = strings.TrimSpace(rest)
	}
	var procs []Proc
	for _, l := range lines(main) {
		pid, rest, ok := cutInt(l)
		if !ok {
			continue
		}
		ppid, rest, ok := cutInt(rest)
		if !ok {
			continue
		}
		uid, rest, ok := cutInt(rest)
		if !ok {
			continue
		}
		rest = strings.TrimLeft(rest, " \t")
		state := rest
		cmd := ""
		if i := strings.IndexAny(rest, " \t"); i >= 0 {
			state, cmd = rest[:i], strings.TrimLeft(rest[i:], " \t")
		}
		name, ok := names[pid]
		if !ok {
			name = filepath.Base(firstWord(cmd))
		}
		procs = append(procs, Proc{PID: pid, PPID: ppid, UID: uid, State: state, Name: name, Cmd: cmd})
	}
	return New(procs)
}

// cutInt reads the leading blank-separated decimal of s.
func cutInt(s string) (int, string, bool) {
	s = strings.TrimLeft(s, " \t")
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 || i > 10 || (i < len(s) && s[i] != ' ' && s[i] != '\t') {
		return 0, s, false
	}
	n, err := strconv.Atoi(s[:i])
	if err != nil {
		return 0, s, false
	}
	return n, s[i:], true
}

func firstWord(s string) string {
	if i := strings.IndexAny(s, " \t"); i >= 0 {
		return s[:i]
	}
	return s
}

func lines(b []byte) []string {
	s := strings.TrimRight(string(b), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// ReadProcFS reads a Table from a Linux /proc at root: stat for the
// state, parent and name, the directory's owner for the uid, cmdline for
// the command line ([name] when empty, as ps shows a kernel thread). A
// process that ends while it is read is left out.
func ReadProcFS(root string) (*Table, error) {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var procs []Proc
	for _, e := range ents {
		pid, err := strconv.Atoi(e.Name())
		if err != nil || pid <= 0 {
			continue
		}
		p, ok := ReadProc(root, pid, true)
		if ok {
			procs = append(procs, p)
		}
	}
	if len(procs) == 0 {
		return nil, ErrNoPS
	}
	return New(procs), nil
}

// ReadProc reads one process from a Linux /proc at root, with its command
// line when cmdline is true; false when it does not exist.
func ReadProc(root string, pid int, cmdline bool) (Proc, bool) {
	dir := filepath.Join(root, strconv.Itoa(pid))
	stat, err := os.ReadFile(filepath.Join(dir, "stat"))
	if err != nil {
		return Proc{}, false
	}
	p, ok := parseStat(stat)
	if !ok || p.PID != pid {
		return Proc{}, false
	}
	p.UID = ownerUID(dir)
	if cmdline {
		p.Cmd = ReadCmdline(root, pid)
		if p.Cmd == "" {
			p.Cmd = "[" + p.Name + "]"
		}
	}
	return p, true
}

// ReadCmdline is the command line of pid from a Linux /proc at root, its
// NUL-separated words joined with blanks ("" for a kernel thread or a
// process that is gone).
func ReadCmdline(root string, pid int) string {
	b, err := os.ReadFile(filepath.Join(root, strconv.Itoa(pid), "cmdline"))
	if err != nil {
		return ""
	}
	b = bytes.TrimRight(b, "\x00")
	return string(bytes.ReplaceAll(b, []byte{0}, []byte{' '}))
}

// parseStat reads "pid (comm) state ppid ..." of /proc/PID/stat; comm may
// hold blanks and parentheses, so it ends at the last ")".
func parseStat(b []byte) (Proc, bool) {
	s := string(b)
	open := strings.IndexByte(s, '(')
	end := strings.LastIndexByte(s, ')')
	if open < 0 || end < open {
		return Proc{}, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(s[:open]))
	if err != nil {
		return Proc{}, false
	}
	f := strings.Fields(s[end+1:])
	if len(f) < 2 {
		return Proc{}, false
	}
	ppid, err := strconv.Atoi(f[1])
	if err != nil {
		return Proc{}, false
	}
	return Proc{PID: pid, PPID: ppid, State: f[0], Name: s[open+1 : end]}, true
}
