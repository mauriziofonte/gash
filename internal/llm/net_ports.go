// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/prereq"
)

// portProc is a process that holds a socket.
type portProc struct {
	pid  int
	name string
}

var (
	// ssUserRE is one ("name",pid=N,fd=M) entry of ss -p's users:(...).
	ssUserRE = lazyre.MustCompile(`\("((?:[^"\\]|\\.)*)",pid=([0-9]+)`)
	// netstatProgRE is netstat -p's PID/Program name column, at the end.
	netstatProgRE = lazyre.MustCompile(`\s([0-9]+)/(\S.*?)\s*$`)
)

// portsByPort is llm_ports --port N (plan N4): the sockets whose local
// port is N (listening ones only with --listen), each with the processes
// that hold it, from ss -p or netstat -p. Without root, the processes of
// other users are not shown (procs is then empty).
func (m *Module) portsByPort(c *invoke.Call, scope, port string) int {
	if !validUint(c, "port", port) {
		return 1
	}
	if n := numericText(port); len(n) > 5 || atoi(n) > 65535 {
		return fail(c, "invalid_port", "Value '"+port+"' is above 65535", "Provide a port number from 0 to 65535")
	}
	port = numericText(port)
	src, ok := m.portsSource(c, "llm_ports --port")
	if !ok {
		return 1
	}
	type rec struct {
		row   [4]string
		procs []portProc
	}
	var recs []rec
	index := map[[4]string]int{}
	rows, procs := src.read(scope, true)
	if src.kind == portsBSD {
		procs = src.lsofProcs(rows, port)
	}
	for i, r := range rows {
		if numericText(r[0]) != port {
			continue
		}
		key := [4]string{port, r[1], r[2], r[3]}
		j, seen := index[key]
		if !seen {
			j = len(recs)
			index[key] = j
			recs = append(recs, rec{row: key})
		}
		recs[j].procs = append(recs[j].procs, procs[i]...)
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].row[1] < recs[j].row[1] })
	b := []byte{'['}
	for i, r := range recs {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, `{"port":`+port+`,"proto":"`...)
		b = appendEscaped(b, r.row[1])
		b = append(b, `","state":"`...)
		b = appendEscaped(b, r.row[2])
		b = append(b, `","address":"`...)
		b = appendEscaped(b, r.row[3])
		b = append(b, `","procs":[`...)
		ps := r.procs
		sort.Slice(ps, func(i, j int) bool {
			if ps[i].pid != ps[j].pid {
				return ps[i].pid < ps[j].pid
			}
			return ps[i].name < ps[j].name
		})
		n := 0
		for k, p := range ps {
			if k > 0 && p == ps[k-1] {
				continue
			}
			if n > 0 {
				b = append(b, ',')
			}
			n++
			b = append(b, `{"pid":`+strconv.Itoa(p.pid)+`,"name":`...)
			b = contract.AppendString(b, p.name, contract.EscapeJQ)
			b = append(b, '}')
		}
		b = append(b, "]}"...)
	}
	b = append(b, "]\n"...)
	_, _ = c.Stdout.Write(b)
	return 0
}

// parseSSProcs reads ss -tun[al]p: the rows parseSS reads, and for each
// the processes of its users:(...) column.
func parseSSProcs(out []byte) ([][4]string, [][]portProc) {
	var rows [][4]string
	var procs [][]portProc
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	header := ""
	if len(lines) > 0 {
		header = lines[0]
	}
	for _, line := range lines[min(1, len(lines)):] {
		r := parseSS([]byte(header + "\n" + line))
		if len(r) == 0 {
			continue
		}
		var ps []portProc
		for _, mm := range ssUserRE.FindAllStringSubmatch(line, -1) {
			pid, _ := strconv.Atoi(mm[2])
			ps = append(ps, portProc{pid: pid, name: strings.ReplaceAll(mm[1], `\"`, `"`)})
		}
		rows = append(rows, r[0])
		procs = append(procs, ps)
	}
	return rows, procs
}

// parseNetstatProcs reads netstat -tun[al]p: the rows parseNetstat reads,
// and the process of its PID/Program name column.
func parseNetstatProcs(out []byte) ([][4]string, [][]portProc) {
	var rows [][4]string
	var procs [][]portProc
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		r := parseNetstat([]byte(line))
		if len(r) == 0 {
			continue
		}
		var ps []portProc
		if mm := netstatProgRE.FindStringSubmatch(line); mm != nil {
			pid, _ := strconv.Atoi(mm[1])
			ps = append(ps, portProc{pid: pid, name: mm[2]})
		}
		rows = append(rows, r[0])
		procs = append(procs, ps)
	}
	return rows, procs
}

// The programs llm_ports reads.
const (
	portsSS      = "ss"
	portsNetstat = "netstat" // net-tools, on Linux
	portsBSD     = "bsd"     // the netstat of macOS and the BSDs
)

// portsSource is the program llm_ports reads the sockets from: ss, else
// netstat, which prints ADDR.PORT and takes no -t or -u on macOS and the
// BSDs (where 2.x parsed no row and printed []).
type portsSource struct {
	m    *Module
	c    *invoke.Call
	path string
	kind string
}

// portsSource finds ss or netstat; without either the missing ss is
// reported for need (ports_tool_not_found).
func (m *Module) portsSource(c *invoke.Call, need string) (*portsSource, bool) {
	path, ok := m.locate(c, "ss")
	if !ok {
		prereq.Fail(c, "ss", need, true)
		return nil, false
	}
	kind := portsNetstat
	switch {
	case filepath.Base(path) == "ss":
		kind = portsSS
	case bsdNetstat(m.goos):
		kind = portsBSD
	}
	return &portsSource{m: m, c: c, path: path, kind: kind}, true
}

// bsdNetstat reports a system whose netstat is the BSD one.
func bsdNetstat(goos string) bool {
	switch goos {
	case "darwin", "freebsd", "openbsd", "netbsd", "dragonfly":
		return true
	}
	return false
}

// read lists the sockets of scope ("a" all, "l" listening): [port, proto,
// state, address] rows and, with procs, the processes of each (none from
// the BSD netstat: lsofProcs finds those).
func (s *portsSource) read(scope string, procs bool) ([][4]string, [][]portProc) {
	flags := "-tun" + scope
	if procs {
		flags += "p"
	}
	switch s.kind {
	case portsSS:
		out := s.m.output(s.c, nil, s.path, flags)
		if procs {
			return parseSSProcs(out)
		}
		return parseSS(out), nil
	case portsNetstat:
		out := s.m.output(s.c, nil, s.path, flags)
		if procs {
			return parseNetstatProcs(out)
		}
		return parseNetstat(out), nil
	}
	out := append(s.m.output(s.c, nil, s.path, "-an", "-p", "tcp"), s.m.output(s.c, nil, s.path, "-an", "-p", "udp")...)
	rows := parseNetstatBSD(out, scope == "l")
	return rows, make([][]portProc, len(rows))
}

// parseNetstatBSD reads netstat -an -p tcp and -p udp of macOS and the
// BSDs: "tcp4 0 0 127.0.0.1.631 *.* LISTEN", the port after the last dot
// of the address, the 4, 6 or 46 of the protocol dropped, udp reported as
// LISTEN (as parseNetstat does). listen keeps the listening tcp sockets
// and the unconnected udp ones, as ss -l and netstat -l do.
func parseNetstatBSD(out []byte, listen bool) [][4]string {
	var rows [][4]string
	for _, line := range strings.Split(strings.TrimSuffix(string(out), "\n"), "\n") {
		f := awkFields(line)
		p := field(f, 1)
		if !strings.HasPrefix(p, "tcp") && !strings.HasPrefix(p, "udp") {
			continue
		}
		proto := strings.TrimRight(p, "46")
		local := field(f, 4)
		i := strings.LastIndexByte(local, '.')
		if i < 0 {
			continue
		}
		addr, port := local[:i], local[i+1:]
		state := field(f, 6)
		if proto == "udp" {
			if listen && field(f, 5) != "*.*" {
				continue
			}
			state = "LISTEN"
		} else if listen && state != "LISTEN" {
			continue
		}
		if digitsRE.MatchString(port) {
			rows = append(rows, [4]string{port, proto, state, addr})
		}
	}
	return rows
}

// lsofProcs are the processes of each BSD row on port, from lsof -F (its
// fields: p the PID, c the command, P the protocol, n the address): a
// socket matches a row by protocol and local address. Without lsof none
// is shown, as ss shows none of another user's processes.
func (s *portsSource) lsofProcs(rows [][4]string, port string) [][]portProc {
	procs := make([][]portProc, len(rows))
	lsof, ok := s.m.locate(s.c, "lsof")
	if !ok {
		return procs
	}
	out := s.m.output(s.c, nil, lsof, "-nP", "-F", "pcPn", "-i", ":"+port)
	for _, sk := range parseLsofF(out) {
		if sk.port != port {
			continue
		}
		for i, r := range rows {
			if r[1] == sk.proto && strings.Trim(r[3], "[]") == sk.addr {
				procs[i] = append(procs[i], sk.proc)
			}
		}
	}
	return procs
}

// lsofSock is a socket of lsof -F: the process, the protocol (tcp, udp)
// and the local address and port.
type lsofSock struct {
	proc        portProc
	proto, addr string
	port        string
}

// parseLsofF reads lsof -F pcPn: a p line starts a process, c names it, P
// and n describe each of its sockets ("127.0.0.1:631", "[::1]:631",
// "*:3306", "10.0.0.2:8080->1.2.3.4:443").
func parseLsofF(out []byte) []lsofSock {
	var socks []lsofSock
	var cur portProc
	proto := ""
	for _, l := range strings.Split(string(out), "\n") {
		if l == "" {
			continue
		}
		v := l[1:]
		switch l[0] {
		case 'p':
			cur, proto = portProc{pid: atoiMax(v)}, ""
		case 'c':
			cur.name = v
		case 'P':
			proto = strings.ToLower(v)
		case 'n':
			local, _, _ := strings.Cut(v, "->")
			i := strings.LastIndexByte(local, ':')
			if i < 0 {
				continue
			}
			socks = append(socks, lsofSock{proc: cur, proto: proto, addr: strings.Trim(local[:i], "[]"), port: numericText(local[i+1:])})
		}
	}
	return socks
}

// atoiMax is a decimal of at most 9 digits, 0 otherwise.
func atoiMax(s string) int {
	if !uintRE.MatchString(s) {
		return 0
	}
	return atoi(s)
}
