// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/netip"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// llm_dns speaks DNS itself (RFC 1035, a UDP query with an EDNS0 OPT
// record, TCP when the answer is truncated): net.Resolver has no CAA and
// no SOA lookup and cannot report NXDOMAIN apart from an empty answer, and
// golang.org/x/net/dns/dnsmessage is a module this repository does not
// otherwise need.

const (
	// dnsTimeout is the default --timeout of one name (all its types).
	dnsTimeout = 5 * time.Second
	// dnsTries is how many times a UDP query is sent before giving up.
	dnsTries = 3
	// dnsUDPSize is the EDNS0 payload size advertised (the DNS flag day
	// 2020 value, safe from fragmentation).
	dnsUDPSize = 1232
)

// dnsTypes are the record types llm_dns asks for.
var dnsTypes = map[string]uint16{"A": 1, "NS": 2, "CNAME": 5, "SOA": 6, "MX": 15, "TXT": 16, "AAAA": 28, "CAA": 257}

// dnsMX, dnsCAA and dnsSOA are the structured answers.
type dnsMX struct {
	Host string `json:"host"`
	Pref int    `json:"pref"`
}

type dnsCAA struct {
	Flag  int    `json:"flag"`
	Tag   string `json:"tag"`
	Value string `json:"value"`
}

type dnsSOA struct {
	MName   string `json:"mname"`
	RName   string `json:"rname"`
	Serial  uint32 `json:"serial"`
	Refresh uint32 `json:"refresh"`
	Retry   uint32 `json:"retry"`
	Expire  uint32 `json:"expire"`
	Minimum uint32 `json:"minimum"`
}

// dns is llm_dns (plan N2): the records of each NAME, for each --type
// (default A,AAAA), asked of --server or the first nameservers of
// /etc/resolv.conf.
func (m *Module) dns(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(netFlags)
	types := []string{"A", "AAAA"}
	server := ""
	names, ok := netArgs(c, o, "Use --type T,T, --server IP, -j N, --timeout S, --format F or NAME targets", func(args []string) (int, bool) {
		if args[0] != "--type" && args[0] != "--server" {
			return 0, true
		}
		v, ok := needValue(c, args)
		if !ok {
			return 0, false
		}
		if args[0] == "--server" {
			host, port, ok := splitHostPort(v, "53")
			if _, err := netip.ParseAddr(host); !ok || err != nil {
				fail(c, "invalid_server", v, "Give the resolver as an IP address, optionally with a port: 1.1.1.1, 127.0.0.1:5353, [2606:4700::1111]:53")
				return 0, false
			}
			server = net.JoinHostPort(host, port)
			return 2, true
		}
		types = types[:0]
		for _, t := range strings.Split(v, ",") {
			t = strings.ToUpper(strings.TrimSpace(t))
			if _, known := dnsTypes[t]; !known {
				fail(c, "invalid_type", v, "Use record types from A, AAAA, MX, TXT, NS, CAA, CNAME, SOA, separated by commas")
				return 0, false
			}
			if !containsString(types, t) {
				types = append(types, t)
			}
		}
		return 2, true
	})
	if !ok {
		return 1
	}
	if len(names) == 0 {
		return fail(c, "missing_target", "llm_dns needs a NAME", "Pass one or more domain names, e.g. example.com _dmarc.example.com")
	}
	for _, n := range names {
		if _, ok := dnsName(n); !ok {
			return fail(c, "invalid_target", n, "Give a domain name: labels of 1 to 63 characters, at most 253 in all")
		}
	}
	servers := []string{server}
	if server == "" {
		servers = m.nameservers()
	}
	cols := append(append([]string{"name"}, types...), "ms", "error")
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), o.TimeoutOr(dnsTimeout), names, func(ctx context.Context, n string) []any {
		return dnsRow(ctx, servers, n, types)
	})
	o.Write(c, o.FormatOr(llmfmt.JSON), llmfmt.Table{Cols: cols, Rows: rows}, nil)
	return 0
}

// nameservers are the first three nameservers of /etc/resolv.conf, the
// local resolver when it names none.
func (m *Module) nameservers() []string {
	var out []string
	data, _ := m.readFile("/etc/resolv.conf")
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() && len(out) < 3 {
		f := strings.Fields(sc.Text())
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		if a, err := netip.ParseAddr(f[1]); err == nil {
			out = append(out, net.JoinHostPort(a.String(), "53"))
		}
	}
	if len(out) == 0 {
		out = []string{"127.0.0.1:53"}
	}
	return out
}

// dnsName is n as a fully qualified query name, and whether it is a
// valid one ("." is the root).
func dnsName(n string) (string, bool) {
	t := strings.TrimSuffix(n, ".")
	if t == "" {
		return ".", n == "."
	}
	if len(t) > 253 {
		return "", false
	}
	for _, l := range strings.Split(t, ".") {
		if len(l) == 0 || len(l) > 63 {
			return "", false
		}
		for i := 0; i < len(l); i++ {
			if l[i] <= ' ' || l[i] == 0x7f {
				return "", false
			}
		}
	}
	return t + ".", true
}

// dnsRow looks up every type of one name at once and builds its row:
// name, one column per type (null when that lookup failed), ms, error
// (the first failure, in type order; nxdomain when the name does not
// exist, its types then empty).
func dnsRow(ctx context.Context, servers []string, name string, types []string) []any {
	fqdn, _ := dnsName(name)
	start := time.Now()
	vals := make([]any, len(types))
	errs := make([]string, len(types))
	var wg sync.WaitGroup
	for i, t := range types {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vals[i], errs[i] = dnsLookup(ctx, servers, fqdn, dnsTypes[t])
		}()
	}
	wg.Wait()
	row := append([]any{name}, vals...)
	row = append(row, llmfmt.Ms(time.Since(start)))
	errField := ""
	for _, e := range errs {
		if e != "" {
			errField = e
			break
		}
	}
	return append(row, nullable(errField))
}

// dnsLookup asks for one type and returns the sorted answer (empty, not
// nil, for NOERROR without records and for NXDOMAIN) or the error field.
func dnsLookup(ctx context.Context, servers []string, fqdn string, qtype uint16) (any, string) {
	// A random ID (crypto/rand: an off-path spoofer must not guess it).
	var idb [2]byte
	_, _ = rand.Read(idb[:])
	id := binary.BigEndian.Uint16(idb[:])
	q := dnsQuery(id, fqdn, qtype)
	resp, err := dnsExchange(ctx, servers, q, id)
	if err != nil {
		return nil, netError(ctx, err)
	}
	msg, err := parseDNS(resp)
	if err != nil {
		return nil, "bad_response"
	}
	switch msg.rcode {
	case 0:
	case 3:
		return dnsAnswer(qtype, nil), "nxdomain"
	case 2:
		return nil, "servfail"
	case 5:
		return nil, "refused"
	default:
		return nil, "rcode_" + strconv.Itoa(msg.rcode)
	}
	var rrs []dnsRR
	for _, rr := range msg.answers {
		if rr.typ == qtype {
			rrs = append(rrs, rr)
		}
	}
	v := dnsAnswer(qtype, rrs)
	if v == nil {
		return nil, "bad_response"
	}
	return v, ""
}

// dnsQuery is a recursive query for one name and type, with an EDNS0
// OPT record.
func dnsQuery(id uint16, fqdn string, qtype uint16) []byte {
	b := make([]byte, 12, 64+len(fqdn))
	binary.BigEndian.PutUint16(b, id)
	b[2] = 0x01 // RD
	b[5] = 1    // QDCOUNT
	b[11] = 1   // ARCOUNT: the OPT record
	if fqdn != "." {
		for _, l := range strings.Split(strings.TrimSuffix(fqdn, "."), ".") {
			b = append(b, byte(len(l)))
			b = append(b, l...)
		}
	}
	b = append(b, 0)
	b = binary.BigEndian.AppendUint16(b, qtype)
	b = binary.BigEndian.AppendUint16(b, 1) // IN
	// OPT: root owner, type 41, class = UDP payload size, TTL 0, no data.
	b = append(b, 0, 0, 41)
	b = binary.BigEndian.AppendUint16(b, dnsUDPSize)
	return append(b, 0, 0, 0, 0, 0, 0)
}

// dnsExchange sends q over UDP (dnsTries times, rotating the servers,
// each try with its share of the time left) and returns the answer that
// carries id; a truncated answer is asked again over TCP.
func dnsExchange(ctx context.Context, servers []string, q []byte, id uint16) ([]byte, error) {
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(dnsTimeout)
	}
	var last error
	for try := 0; try < dnsTries; try++ {
		server := servers[try%len(servers)]
		left := time.Until(deadline)
		if left <= 0 {
			break
		}
		resp, err := dnsUDP(ctx, server, q, id, time.Now().Add(left/time.Duration(dnsTries-try)))
		if err == nil {
			if resp[2]&0x02 != 0 { // TC
				return dnsTCP(ctx, server, q, id)
			}
			return resp, nil
		}
		last = err
		var ne net.Error
		if ctx.Err() != nil || (len(servers) == 1 && !(errors.As(err, &ne) && ne.Timeout())) {
			break
		}
	}
	if last == nil {
		last = context.DeadlineExceeded
	}
	return nil, last
}

// dnsUDP sends q once and waits until until for the answer with id.
func dnsUDP(ctx context.Context, server string, q []byte, id uint16, until time.Time) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", server)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	_ = conn.SetDeadline(until)
	if _, err := conn.Write(q); err != nil {
		return nil, err
	}
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, err
		}
		if n >= 12 && binary.BigEndian.Uint16(buf) == id && buf[2]&0x80 != 0 {
			return buf[:n], nil
		}
	}
}

// dnsTCP sends q over TCP (two-byte length prefix) and reads the answer.
func dnsTCP(ctx context.Context, server string, q []byte, id uint16) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", server)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = conn.SetDeadline(time.Now()) })
	defer stop()
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if _, err := conn.Write(append(binary.BigEndian.AppendUint16(nil, uint16(len(q))), q...)); err != nil {
		return nil, err
	}
	var size [2]byte
	if _, err := io.ReadFull(conn, size[:]); err != nil {
		return nil, err
	}
	resp := make([]byte, binary.BigEndian.Uint16(size[:]))
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	if len(resp) < 12 || binary.BigEndian.Uint16(resp) != id {
		return nil, errBadDNS
	}
	return resp, nil
}

var errBadDNS = errors.New("malformed DNS message")

// dnsMsg is the part of an answer llm_dns reads.
type dnsMsg struct {
	rcode   int
	answers []dnsRR
}

// dnsRR is one answer record: its type and where its data sits in the
// message (names in the data may point anywhere in it).
type dnsRR struct {
	typ      uint16
	msg      []byte
	off, end int
}

// parseDNS reads the header, skips the questions and collects the answer
// records.
func parseDNS(msg []byte) (dnsMsg, error) {
	if len(msg) < 12 {
		return dnsMsg{}, errBadDNS
	}
	m := dnsMsg{rcode: int(msg[3] & 0x0f)}
	qd, an := int(binary.BigEndian.Uint16(msg[4:])), int(binary.BigEndian.Uint16(msg[6:]))
	off := 12
	for i := 0; i < qd; i++ {
		_, next, err := dnsReadName(msg, off)
		if err != nil || next+4 > len(msg) {
			return dnsMsg{}, errBadDNS
		}
		off = next + 4
	}
	for i := 0; i < an; i++ {
		_, next, err := dnsReadName(msg, off)
		if err != nil || next+10 > len(msg) {
			return dnsMsg{}, errBadDNS
		}
		typ := binary.BigEndian.Uint16(msg[next:])
		rdlen := int(binary.BigEndian.Uint16(msg[next+8:]))
		start := next + 10
		if start+rdlen > len(msg) {
			return dnsMsg{}, errBadDNS
		}
		m.answers = append(m.answers, dnsRR{typ: typ, msg: msg, off: start, end: start + rdlen})
		off = start + rdlen
	}
	return m, nil
}

// dnsReadName reads the (possibly compressed) name at off and returns it
// without the final dot and the offset after it.
func dnsReadName(msg []byte, off int) (string, int, error) {
	var labels []string
	next, jumps, size := -1, 0, 0
	for {
		if off >= len(msg) {
			return "", 0, errBadDNS
		}
		l := int(msg[off])
		switch {
		case l == 0:
			if next < 0 {
				next = off + 1
			}
			return strings.Join(labels, "."), next, nil
		case l&0xC0 == 0xC0:
			if off+1 >= len(msg) || jumps > 32 {
				return "", 0, errBadDNS
			}
			if next < 0 {
				next = off + 2
			}
			off = int(binary.BigEndian.Uint16(msg[off:]) & 0x3FFF)
			jumps++
		case l&0xC0 != 0:
			return "", 0, errBadDNS
		default:
			if off+1+l > len(msg) || size+l+1 > 255 {
				return "", 0, errBadDNS
			}
			labels = append(labels, string(msg[off+1:off+1+l]))
			size += l + 1
			off += 1 + l
		}
	}
}

// dnsAnswer decodes the records of one type into the sorted JSON value of
// its column; nil when a record is malformed.
func dnsAnswer(qtype uint16, rrs []dnsRR) any {
	switch qtype {
	case 15:
		out := []dnsMX{}
		for _, rr := range rrs {
			if rr.end-rr.off < 3 {
				return nil
			}
			host, _, err := dnsReadName(rr.msg, rr.off+2)
			if err != nil {
				return nil
			}
			out = append(out, dnsMX{Host: host, Pref: int(binary.BigEndian.Uint16(rr.msg[rr.off:]))})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Pref != out[j].Pref {
				return out[i].Pref < out[j].Pref
			}
			return out[i].Host < out[j].Host
		})
		return out
	case 257:
		out := []dnsCAA{}
		for _, rr := range rrs {
			d := rr.msg[rr.off:rr.end]
			if len(d) < 2 || 2+int(d[1]) > len(d) {
				return nil
			}
			out = append(out, dnsCAA{Flag: int(d[0]), Tag: string(d[2 : 2+d[1]]), Value: string(d[2+d[1]:])})
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].Tag != out[j].Tag {
				return out[i].Tag < out[j].Tag
			}
			return out[i].Value < out[j].Value
		})
		return out
	case 6:
		out := []dnsSOA{}
		for _, rr := range rrs {
			mname, off, err := dnsReadName(rr.msg, rr.off)
			if err != nil {
				return nil
			}
			rname, off, err := dnsReadName(rr.msg, off)
			if err != nil || off+20 > rr.end {
				return nil
			}
			u := func(i int) uint32 { return binary.BigEndian.Uint32(rr.msg[off+4*i:]) }
			out = append(out, dnsSOA{MName: mname, RName: rname, Serial: u(0), Refresh: u(1), Retry: u(2), Expire: u(3), Minimum: u(4)})
		}
		return out
	}
	out := []string{}
	for _, rr := range rrs {
		d := rr.msg[rr.off:rr.end]
		switch qtype {
		case 1, 28:
			a, ok := netip.AddrFromSlice(d)
			if !ok || (qtype == 1) != (len(d) == 4) {
				return nil
			}
			out = append(out, a.String())
		case 2, 5:
			name, _, err := dnsReadName(rr.msg, rr.off)
			if err != nil {
				return nil
			}
			out = append(out, name)
		case 16:
			var s strings.Builder
			for len(d) > 0 {
				l := int(d[0])
				if 1+l > len(d) {
					return nil
				}
				s.Write(d[1 : 1+l])
				d = d[1+l:]
			}
			out = append(out, s.String())
		}
	}
	sort.Strings(out)
	return out
}
