// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
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
	var out []byte
	parse := parseSSProcs
	if ss, ok := m.lookPath(c, "ss"); ok {
		out = m.output(c, nil, ss, "-tun"+scope+"p")
	} else if ns, ok := m.lookPath(c, "netstat"); ok {
		out, parse = m.output(c, nil, ns, "-tun"+scope+"p"), parseNetstatProcs
	} else {
		return fail(c, "ports_tool_not_found", "Neither ss nor netstat is installed",
			"Install iproute2 (ss) or net-tools (netstat) to list ports. Skip this check")
	}
	type rec struct {
		row   [4]string
		procs []portProc
	}
	var recs []rec
	index := map[[4]string]int{}
	rows, procs := parse(out)
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
