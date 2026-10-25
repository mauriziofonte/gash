// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"encoding/binary"
	"io"
	"net"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// dnsRec is one answer record of the fake: its type and its data (a name
// in the data is written by dnsWireName, uncompressed).
type dnsRec struct {
	typ  uint16
	data []byte
}

// dnsZone is what the fake knows: records by lower-case name.
type dnsZone map[string][]dnsRec

// dnsWireName encodes a name as labels.
func dnsWireName(n string) []byte {
	var b []byte
	for _, l := range strings.Split(strings.TrimSuffix(n, "."), ".") {
		if l != "" {
			b = append(b, byte(len(l)))
			b = append(b, l...)
		}
	}
	return append(b, 0)
}

func u16(v uint16) []byte { return binary.BigEndian.AppendUint16(nil, v) }
func u32(v uint32) []byte { return binary.BigEndian.AppendUint32(nil, v) }

func txtData(parts ...string) []byte {
	var b []byte
	for _, p := range parts {
		b = append(b, byte(len(p)))
		b = append(b, p...)
	}
	return b
}

// dnsFake answers queries from a zone on 127.0.0.1, over UDP and TCP on
// the same port. "servfail." answers SERVFAIL, "big." is truncated over
// UDP (the full answer comes over TCP), "slow." never answers, a name
// that is not in the zone is NXDOMAIN. queries counts UDP queries.
type dnsFake struct {
	addr    string
	queries atomic.Int32
	tcp     atomic.Int32
}

func startDNSFake(t *testing.T, zone dnsZone) *dnsFake {
	t.Helper()
	var pc net.PacketConn
	var ln net.Listener
	for i := 0; i < 20; i++ {
		var err error
		pc, err = net.ListenPacket("udp4", "127.0.0.1:0")
		require.NoError(t, err)
		ln, err = net.Listen("tcp4", pc.LocalAddr().String())
		if err == nil {
			break
		}
		_ = pc.Close()
		pc = nil
	}
	require.NotNil(t, pc, "no free port for UDP and TCP")
	f := &dnsFake{addr: pc.LocalAddr().String()}
	t.Cleanup(func() { _ = pc.Close(); _ = ln.Close() })
	go func() {
		buf := make([]byte, 1500)
		for {
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			f.queries.Add(1)
			resp := f.answer(zone, buf[:n], true)
			if resp != nil {
				_, _ = pc.WriteTo(resp, from)
			}
		}
	}()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.tcp.Add(1)
			go func() {
				defer func() { _ = c.Close() }()
				var size [2]byte
				if _, err := io.ReadFull(c, size[:]); err != nil {
					return
				}
				q := make([]byte, binary.BigEndian.Uint16(size[:]))
				if _, err := io.ReadFull(c, q); err != nil {
					return
				}
				resp := f.answer(zone, q, false)
				_, _ = c.Write(append(u16(uint16(len(resp))), resp...))
			}()
		}
	}()
	return f
}

// answer builds the response to query q: header, the question, and the
// records of the asked type, owners compressed to the question name.
func (f *dnsFake) answer(zone dnsZone, q []byte, udp bool) []byte {
	name, off, err := dnsReadName(q, 12)
	if err != nil {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[off:])
	qend := off + 4
	name = strings.ToLower(name)
	if name == "slow" {
		return nil
	}
	var rrs []dnsRec
	rcode := byte(0)
	switch recs, ok := zone[name]; {
	case name == "servfail":
		rcode = 2
	case !ok:
		rcode = 3
	default:
		for _, r := range recs {
			if r.typ == qtype {
				rrs = append(rrs, r)
			}
		}
	}
	flags := byte(0x81) // QR, RD
	if udp && name == "big" {
		flags |= 0x02 // TC
		rrs = nil
	}
	b := append([]byte{q[0], q[1], flags, 0x80 | rcode}, u16(1)...)
	b = append(b, u16(uint16(len(rrs)))...)
	b = append(b, 0, 0, 0, 0)
	b = append(b, q[12:qend]...)
	for _, r := range rrs {
		b = append(b, 0xC0, 12) // owner: the question name
		b = append(b, u16(r.typ)...)
		b = append(b, u16(1)...)
		b = append(b, u32(300)...)
		b = append(b, u16(uint16(len(r.data)))...)
		b = append(b, r.data...)
	}
	return b
}

func testZone() dnsZone {
	mx := func(pref uint16, host string) dnsRec { return dnsRec{15, append(u16(pref), dnsWireName(host)...)} }
	soa := append(append(dnsWireName("ns1.example.test"), dnsWireName("hostmaster.example.test")...), append(append(append(append(u32(2026102501), u32(7200)...), u32(3600)...), u32(1209600)...), u32(300)...)...)
	caa := append([]byte{0, 5}, "issueletsencrypt.org"...)
	var big []dnsRec
	for i := 0; i < 60; i++ {
		big = append(big, dnsRec{1, []byte{10, 0, 0, byte(i)}})
	}
	return dnsZone{
		"example.test": {
			{1, []byte{192, 0, 2, 20}}, {1, []byte{192, 0, 2, 3}},
			{28, net.ParseIP("2001:db8::1").To16()},
			mx(20, "mx2.example.test"), mx(10, "mx1.example.test"), mx(10, "mx0.example.test"),
			{16, txtData("v=spf1 ", "-all")}, {16, txtData("b-verify")},
			{2, dnsWireName("ns2.example.test")}, {2, dnsWireName("ns1.example.test")},
			{6, soa}, {257, caa},
		},
		"www.example.test": {{5, dnsWireName("example.test")}},
		"empty.test":       {},
		"big":              big,
		// Compression in the data: "mail" then a pointer to the question.
		"ptr.test": {{15, append(u16(5), append([]byte{4, 'm', 'a', 'i', 'l'}, 0xC0, 12)...)}},
	}
}

func TestDNSRecordsOfEveryType(t *testing.T) {
	f := startDNSFake(t, testZone())
	r := netRun(t, nil, "llm_dns", "example.test", "--server", f.addr, "--type", "a,AAAA,MX,TXT,NS,CAA,CNAME,SOA")
	require.Regexp(t, `^\[\{"name":"example.test","A":\["192.0.2.20","192.0.2.3"\],"AAAA":\["2001:db8::1"\],`+
		`"MX":\[\{"host":"mx0.example.test","pref":10\},\{"host":"mx1.example.test","pref":10\},\{"host":"mx2.example.test","pref":20\}\],`+
		`"TXT":\["b-verify","v=spf1 -all"\],"NS":\["ns1.example.test","ns2.example.test"\],`+
		`"CAA":\[\{"flag":0,"tag":"issue","value":"letsencrypt.org"\}\],"CNAME":\[\],`+
		`"SOA":\[\{"mname":"ns1.example.test","rname":"hostmaster.example.test","serial":2026102501,"refresh":7200,"retry":3600,"expire":1209600,"minimum":300\}\],`+
		`"ms":[0-9.]+,"error":null\}\]`+"\n$", r.Stdout)
	require.Empty(t, r.Stderr)
}

func TestDNSErrorsAreFields(t *testing.T) {
	f := startDNSFake(t, testZone())
	rows := netRows(t, netRun(t, nil, "llm_dns", "www.example.test", "nope.test", "servfail", "empty.test", "ptr.test", "slow",
		"--server", f.addr, "--type", "CNAME,MX", "--timeout", "0.6"))
	require.Len(t, rows, 6)
	require.Equal(t, []any{"example.test"}, rows[0]["CNAME"])
	require.Equal(t, []any{}, rows[0]["MX"])
	require.Nil(t, rows[0]["error"])
	require.Equal(t, "nxdomain", rows[1]["error"])
	require.Equal(t, []any{}, rows[1]["MX"])
	require.Equal(t, "servfail", rows[2]["error"])
	require.Nil(t, rows[2]["MX"])
	require.Nil(t, rows[3]["error"])
	require.Equal(t, []any{map[string]any{"host": "mail.ptr.test", "pref": 5.0}}, rows[4]["MX"])
	require.Equal(t, "timeout", rows[5]["error"])
	require.Nil(t, rows[5]["CNAME"])
}

func TestDNSTruncatedRetriesOverTCP(t *testing.T) {
	f := startDNSFake(t, testZone())
	rows := netRows(t, netRun(t, nil, "llm_dns", "big", "--server", f.addr, "--type", "A"))
	require.Len(t, rows[0]["A"], 60)
	require.Nil(t, rows[0]["error"])
	require.EqualValues(t, 1, f.tcp.Load())
}

func TestDNSDefaultServerFromResolvConf(t *testing.T) {
	f := startDNSFake(t, testZone())
	_, port, _ := net.SplitHostPort(f.addr)
	m := New()
	m.readFile = func(p string) ([]byte, error) {
		require.Equal(t, "/etc/resolv.conf", p)
		return []byte("# test\nsearch example.test\nnameserver 127.0.0.1\nnameserver ::1\n"), nil
	}
	got := m.nameservers()
	require.Equal(t, []string{"127.0.0.1:53", "[::1]:53"}, got)
	m.readFile = func(string) ([]byte, error) { return nil, io.EOF }
	require.Equal(t, []string{"127.0.0.1:53"}, m.nameservers())
	// The lookup itself, through the rotation, against the fake.
	v, e := dnsLookupFor(t, []string{"127.0.0.1:1", "127.0.0.1:" + port}, "example.test.", 1)
	require.Equal(t, "", e)
	require.Equal(t, []string{"192.0.2.20", "192.0.2.3"}, v)
}

func dnsLookupFor(t *testing.T, servers []string, fqdn string, qtype uint16) (any, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return dnsLookup(ctx, servers, fqdn, qtype)
}

func TestDNSRetriesAfterLoss(t *testing.T) {
	// A server that drops the first query: the second try answers.
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = pc.Close() }()
	f := &dnsFake{}
	zone := testZone()
	go func() {
		buf := make([]byte, 1500)
		for n := 0; ; n++ {
			k, from, err := pc.ReadFrom(buf)
			if err != nil {
				return
			}
			if n > 0 {
				_, _ = pc.WriteTo(f.answer(zone, buf[:k], true), from)
			}
		}
	}()
	rows := netRows(t, netRun(t, nil, "llm_dns", "example.test", "--server", pc.LocalAddr().String(), "--type", "A", "--timeout", "1.5"))
	require.Nil(t, rows[0]["error"], rows)
	require.Len(t, rows[0]["A"], 2)
}

func TestDNSClosedPortIsConnectionRefused(t *testing.T) {
	pc, err := net.ListenPacket("udp4", "127.0.0.1:0")
	require.NoError(t, err)
	addr := pc.LocalAddr().String()
	require.NoError(t, pc.Close())
	rows := netRows(t, netRun(t, nil, "llm_dns", "example.test", "--server", addr, "--timeout", "1"))
	require.Contains(t, []any{"connection_refused", "timeout"}, rows[0]["error"])
	require.Nil(t, rows[0]["A"])
}

func TestDNSParallelSameAsSerial(t *testing.T) {
	f := startDNSFake(t, testZone())
	var names []string
	for i := 0; i < 10; i++ {
		names = append(names, "example.test", "nope"+strconv.Itoa(i)+".test", "www.example.test")
	}
	args := append(names, "--server", f.addr, "--type", "A,MX,TXT")
	norm := func(s string) string { return msRE.ReplaceAllString(s, `"ms":0`) }
	serial := netRun(t, nil, "llm_dns", append(args, "-j", "1")...)
	parallel := netRun(t, nil, "llm_dns", append(args, "-j", "8")...)
	require.Equal(t, norm(serial.Stdout), norm(parallel.Stdout))
	require.Len(t, netRows(t, parallel), 30)
}

func TestDNSArgumentErrors(t *testing.T) {
	long := strings.Repeat("a", 64) + ".test"
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "missing_target/RETRY"},
		{[]string{"--type", "A"}, "missing_target/RETRY"},
		{[]string{"x.test", "--type", "PTR"}, "invalid_type/RETRY"},
		{[]string{"x.test", "--type", "A,,MX"}, "invalid_type/RETRY"},
		{[]string{"x.test", "--server", "dns.google"}, "invalid_server/RETRY"},
		{[]string{"x.test", "--server", "1.1.1.1:99999"}, "invalid_server/RETRY"},
		{[]string{"x.test", "--server"}, "missing_value/RETRY"},
		{[]string{long}, "invalid_target/RETRY"},
		{[]string{"a..b"}, "invalid_target/RETRY"},
		{[]string{"a b"}, "invalid_target/RETRY"},
		{[]string{"x.test", "--bogus"}, "unknown_option/RETRY"},
	} {
		require.Equal(t, c.want, errCode(t, netRun(t, nil, "llm_dns", c.args...)), c.args)
	}
	fq, ok := dnsName("Example.TEST.")
	require.True(t, ok)
	require.Equal(t, "Example.TEST.", fq)
	fq, ok = dnsName(".")
	require.True(t, ok)
	require.Equal(t, ".", fq)
}

// TestDNSParserRejectsMalformed: a compression loop, a pointer past the
// end and short data are errors, never a hang or a panic.
func TestDNSParserRejectsMalformed(t *testing.T) {
	loop := append(make([]byte, 12), 0xC0, 12)
	_, _, err := dnsReadName(loop, 12)
	require.Error(t, err)
	_, _, err = dnsReadName([]byte{3, 'a', 'b'}, 0)
	require.Error(t, err)
	_, _, err = dnsReadName([]byte{0xC0}, 0)
	require.Error(t, err)
	_, _, err = dnsReadName([]byte{0x80, 0}, 0)
	require.Error(t, err)
	_, err = parseDNS([]byte{1, 2, 3})
	require.Error(t, err)
	hdr := []byte{0, 1, 0x81, 0x80, 0, 0, 0, 1, 0, 0, 0, 0}
	_, err = parseDNS(append(hdr, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 9, 1)) // rdlength past the end
	require.Error(t, err)
	msg := append(hdr, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 3, 1, 2, 3) // A with 3 bytes
	p, err := parseDNS(msg)
	require.NoError(t, err)
	require.Nil(t, dnsAnswer(1, p.answers))
	q := dnsQuery(0xBEEF, "example.test.", 15)
	require.Equal(t, []byte{0xBE, 0xEF, 0x01, 0, 0, 1, 0, 0, 0, 0, 0, 1}, q[:12])
	name, off, err := dnsReadName(q, 12)
	require.NoError(t, err)
	require.Equal(t, "example.test", name)
	require.Equal(t, uint16(15), binary.BigEndian.Uint16(q[off:]))
}
