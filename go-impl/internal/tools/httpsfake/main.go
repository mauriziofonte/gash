// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command httpsfake is the network of a Go-mode spec run
// (go-impl/design/testing.md 2.4): an HTTP proxy on 127.0.0.1 that
// answers every request itself, with the canned responses of
// tests/mocks/bin/curl, which the binary no longer runs (it speaks HTTP
// with net/http). HTTPS requests reach it through HTTPS_PROXY (CONNECT);
// it terminates their TLS with a certificate for the requested host from
// its own CA, which the run trusts through SSL_CERT_FILE. Nothing leaves
// the machine.
//
// The mock's knobs (MOCK_CURL_ERROR, MOCK_CURL_AI_RESPONSE, ...) and the
// route file of MOCK_CURL_ROUTES (format at the top of the mock) are
// variables the test exported for curl; the fake reads them from the
// environment of the process at the other end of the connection
// (/proc/net/tcp and /proc/PID/environ, Linux only). An unreachable
// route refuses the CONNECT (502) when it covers the whole host, and
// otherwise drops the connection after the request. MOCK_CURL_LOG gets a
// curl-like line per request (method, Accept headers, URL; never another
// header, so no credential) and a "body:" line, as the mock wrote.
//
//	httpsfake -ca CA.pem -addr ADDR_FILE [-lo-up]
//
// It writes its CA certificate to CA.pem, then "127.0.0.1:PORT" to
// ADDR_FILE once it listens, and serves until killed. -lo-up first sets
// the loopback interface up (a fresh network namespace has it down).
package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

func main() {
	caFile := flag.String("ca", "", "write the CA certificate (PEM) here")
	addrFile := flag.String("addr", "", "write the listening address here once ready")
	loUp := flag.Bool("lo-up", false, "set the loopback interface up first")
	flag.Parse()
	if *caFile == "" || *addrFile == "" {
		fmt.Fprintln(os.Stderr, "usage: httpsfake -ca CA.pem -addr ADDR_FILE [-lo-up]")
		os.Exit(2)
	}
	if *loUp {
		if err := loopbackUp(); err != nil {
			fmt.Fprintln(os.Stderr, "httpsfake: loopback:", err)
		}
	}
	f, err := newFake()
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*caFile, f.caPEM, 0o644); err != nil {
		fatal(err)
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		fatal(err)
	}
	tmp := *addrFile + ".tmp"
	if err := os.WriteFile(tmp, []byte(ln.Addr().String()+"\n"), 0o644); err != nil {
		fatal(err)
	}
	if err := os.Rename(tmp, *addrFile); err != nil {
		fatal(err)
	}
	fatal(f.serve(ln))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "httpsfake:", err)
	os.Exit(1)
}

// fake is the proxy: its CA and the leaf certificates minted so far.
type fake struct {
	caCert *x509.Certificate
	caKey  *ecdsa.PrivateKey
	caPEM  []byte
	// envOf returns the environment of the client of a connection.
	envOf func(c net.Conn) map[string]string

	mu    sync.Mutex
	leafs map[string]*tls.Certificate
	logMu sync.Mutex
}

func newFake() (*fake, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Gash test fake CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(7 * 24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	return &fake{
		caCert: cert, caKey: key,
		caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		envOf: peerEnv,
		leafs: map[string]*tls.Certificate{},
	}, nil
}

// leaf returns a certificate for host signed by the CA.
func (f *fake) leaf(host string) (*tls.Certificate, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.leafs[host]; ok {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(7 * 24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if ip := net.ParseIP(host); ip != nil {
		tmpl.IPAddresses = []net.IP{ip}
	} else {
		tmpl.DNSNames = []string{host}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, f.caCert, &key.PublicKey, f.caKey)
	if err != nil {
		return nil, err
	}
	c := &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
	f.leafs[host] = c
	return c, nil
}

func (f *fake) serve(ln net.Listener) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go f.conn(c)
	}
}

// conn serves one client connection: plain proxy requests, or a CONNECT
// whose tunnel is terminated here.
func (f *fake) conn(c net.Conn) {
	defer func() { _ = c.Close() }()
	env := f.envOf(c)
	br := bufio.NewReader(c)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	if req.Method != http.MethodConnect {
		f.loop(c, br, env, req, "")
		return
	}
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		name, port = host, "443"
	}
	authority := name
	if port != "443" {
		authority = net.JoinHostPort(name, port)
	}
	if path := env["MOCK_CURL_ROUTES"]; path != "" && refusesTunnel(path, "https://"+authority) {
		// The host is unreachable: refuse the tunnel, as a proxy does.
		_, _ = io.WriteString(c, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		return
	}
	_, _ = io.WriteString(c, "HTTP/1.1 200 Connection established\r\n\r\n")
	cert, err := f.leaf(name)
	if err != nil {
		return
	}
	tc := tls.Server(&bufConn{Conn: c, r: br}, &tls.Config{Certificates: []tls.Certificate{*cert}, NextProtos: []string{"http/1.1"}})
	if err := tc.Handshake(); err != nil {
		return
	}
	tbr := bufio.NewReader(tc)
	first, err := http.ReadRequest(tbr)
	if err != nil {
		return
	}
	f.loop(tc, tbr, env, first, "https://"+authority)
}

// loop answers req and the requests that follow on the connection.
// origin is the scheme and host of a tunnel, "" for proxy requests (whose
// URL is absolute).
func (f *fake) loop(w net.Conn, br *bufio.Reader, env map[string]string, req *http.Request, origin string) {
	for {
		body, _ := io.ReadAll(req.Body)
		_ = req.Body.Close()
		url := req.URL.String()
		if origin != "" {
			url = origin + req.URL.RequestURI()
		}
		f.log(env, req, url, string(body))
		r := respond(env, request{method: req.Method, url: url, body: string(body)})
		if r.hang {
			// The client times out (curl's exit 28); wait for it to go.
			_, _ = io.Copy(io.Discard, br)
			return
		}
		if r.drop {
			// Unreachable: no answer at all.
			return
		}
		resp := &http.Response{
			StatusCode: r.status, ProtoMajor: 1, ProtoMinor: 1, Request: req,
			Header: http.Header{}, ContentLength: int64(len(r.body)),
			Body: io.NopCloser(strings.NewReader(r.body)),
		}
		for _, h := range r.header {
			resp.Header.Add(h[0], h[1])
		}
		if req.Method == http.MethodHead {
			resp.ContentLength = -1
			if r.length > 0 {
				resp.ContentLength = r.length
			}
			resp.Body = http.NoBody
		}
		var out bytes.Buffer
		if err := resp.Write(&out); err != nil {
			return
		}
		if _, err := w.Write(out.Bytes()); err != nil {
			return
		}
		if req.Close {
			return
		}
		next, err := http.ReadRequest(br)
		if err != nil {
			return
		}
		req = next
	}
}

// log appends the request to the MOCK_CURL_LOG of the client, as the
// mock did: "curl ..." then "body: ...".
func (f *fake) log(env map[string]string, req *http.Request, url, body string) {
	path := env["MOCK_CURL_LOG"]
	if path == "" {
		return
	}
	line := "curl -X " + req.Method
	for _, a := range req.Header.Values("Accept") {
		line += " -H " + strconv.Quote("Accept: "+a)
	}
	line += " " + url + "\n"
	if body != "" {
		line += "body: " + body + "\n"
	}
	f.logMu.Lock()
	defer f.logMu.Unlock()
	lf, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return
	}
	_, _ = io.WriteString(lf, line)
	_ = lf.Close()
}

// bufConn reads through the bufio.Reader that already holds the start of
// the TLS handshake.
type bufConn struct {
	net.Conn
	r *bufio.Reader
}

func (b *bufConn) Read(p []byte) (int, error) { return b.r.Read(p) }
