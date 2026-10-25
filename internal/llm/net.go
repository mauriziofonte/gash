// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

// The network probes of the 3.4 agent toolkit (plan N1-N4): llm_http,
// llm_dns, llm_tls, llm_tcp and llm_ports --port. They only read: an HTTP
// probe sends GET, HEAD or OPTIONS and nothing else (plan D4). Every
// function takes several targets, probes them in parallel with
// llmfmt.Map (-j, default llmfmt.NetworkJobs; --timeout per target) and
// prints one row per target in input order; what goes wrong with one
// target is the "error" field of its row, never a failed call.

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// netFlags are the shared options every network probe takes.
const netFlags = llmfmt.FormatFlag | llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag

// netArgs runs the option loop of a network probe: the shared options go
// to o, own(args) handles the function's options (it returns the words it
// consumed, 0 for a word that is not one of them, and false after writing
// an error), and the other words are the targets. A word that starts with
// "-" and nobody takes is unknown_option.
func netArgs(c *invoke.Call, o *llmfmt.Options, hint string, own func(args []string) (int, bool)) ([]string, bool) {
	var targets []string
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return nil, false
		}
		if n == 0 && own != nil {
			if n, ok = own(args); !ok {
				return nil, false
			}
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		if strings.HasPrefix(a, "-") && a != "-" {
			fail(c, "unknown_option", a, hint)
			return nil, false
		}
		targets = append(targets, a)
		args = args[1:]
	}
	return targets, true
}

// needValue returns args[1], or writes missing_value.
func needValue(c *invoke.Call, args []string) (string, bool) {
	if len(args) < 2 {
		fail(c, "missing_value", args[0], args[0]+" needs a value")
		return "", false
	}
	return args[1], true
}

// splitHostPort splits HOST[:PORT] (an IPv6 address in brackets when it
// has a port, bare otherwise); def is the port when none is given ("" =
// the port is required). ok is false for an empty host or a port outside
// 1..65535.
func splitHostPort(s, def string) (host, port string, ok bool) {
	h, p, err := net.SplitHostPort(s)
	switch {
	case err == nil:
	case def != "" && strings.Count(s, ":") != 1:
		// No port: a name, an IPv4 address, or a bare or bracketed IPv6.
		h, p = strings.TrimSuffix(strings.TrimPrefix(s, "["), "]"), def
	default:
		return "", "", false
	}
	if h == "" || strings.ContainsAny(h, " \t\r\n/[]") {
		return "", "", false
	}
	if strings.Contains(h, ":") {
		if _, err := netip.ParseAddr(h); err != nil {
			return "", "", false
		}
	}
	n, err := strconv.Atoi(p)
	if err != nil || n < 1 || n > 65535 || p != strconv.Itoa(n) {
		return "", "", false
	}
	return h, p, true
}

// netError is the error field of a target: a short stable word for what
// failed (AGENTS.md 3.9 lists them), from the error of a dial, a
// handshake or a request made under ctx.
func netError(ctx context.Context, err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		switch {
		case dnsErr.IsTimeout:
			return "timeout"
		case dnsErr.IsNotFound:
			return "dns_not_found"
		}
		return "dns_error"
	}
	var (
		unknownCA x509.UnknownAuthorityError
		hostname  x509.HostnameError
		invalid   x509.CertificateInvalidError
		verify    *tls.CertificateVerificationError
	)
	switch {
	case errors.As(err, &unknownCA), errors.As(err, &hostname), errors.As(err, &invalid), errors.As(err, &verify):
		return "certificate_error"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.ECONNRESET), errors.Is(err, syscall.EPIPE), errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "connection_reset"
	case errors.Is(err, syscall.EHOSTUNREACH), errors.Is(err, syscall.ENETUNREACH), errors.Is(err, syscall.EADDRNOTAVAIL), errors.Is(err, syscall.EAFNOSUPPORT):
		return "unreachable"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var rec tls.RecordHeaderError
	var alert tls.AlertError
	if errors.As(err, &rec) || errors.As(err, &alert) || strings.Contains(err.Error(), "tls: ") {
		return "tls_error"
	}
	return "transport_error"
}

// nullable is s as a row value: nil for "".
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// envProxy is the proxy of an HTTP request from the caller's environment
// (c.Env, never the process's): HTTPS_PROXY/https_proxy for https URLs,
// HTTP_PROXY/http_proxy for http ones, NO_PROXY/no_proxy exceptions, the
// rules of net/http's ProxyFromEnvironment (localhost and loopback
// addresses are never proxied; a proxy without a scheme is http://).
func envProxy(env invoke.Env) func(*http.Request) (*url.URL, error) {
	get := func(names ...string) string {
		for _, n := range names {
			if v := env.Get(n); v != "" {
				return v
			}
		}
		return ""
	}
	httpsProxy, httpProxy := get("HTTPS_PROXY", "https_proxy"), get("HTTP_PROXY", "http_proxy")
	noProxy := get("NO_PROXY", "no_proxy")
	return func(r *http.Request) (*url.URL, error) {
		p := httpProxy
		if r.URL.Scheme == "https" {
			p = httpsProxy
		}
		if p == "" || !useProxy(r.URL.Hostname(), r.URL.Port(), noProxy) {
			return nil, nil
		}
		u, err := url.Parse(p)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5") {
			if u, err = url.Parse("http://" + p); err != nil {
				return nil, errors.New("invalid proxy address")
			}
		}
		return u, nil
	}
}

// useProxy reports whether a request to host:port goes through the proxy
// given the NO_PROXY list.
func useProxy(host, port, noProxy string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return false
	}
	ip := net.ParseIP(host)
	if ip != nil && ip.IsLoopback() {
		return false
	}
	for _, e := range strings.FieldsFunc(strings.ToLower(noProxy), func(r rune) bool { return r == ',' || r == ' ' }) {
		if e == "*" {
			return false
		}
		if _, cidr, err := net.ParseCIDR(e); err == nil {
			if ip != nil && cidr.Contains(ip) {
				return false
			}
			continue
		}
		if h, p, err := net.SplitHostPort(e); err == nil {
			if p != port {
				continue
			}
			e = h
		}
		e = strings.TrimSuffix(strings.TrimPrefix(e, "["), "]")
		if eip := net.ParseIP(e); eip != nil {
			if ip != nil && eip.Equal(ip) {
				return false
			}
			continue
		}
		switch {
		case strings.HasPrefix(e, "*."):
			if strings.HasSuffix(host, e[1:]) {
				return false
			}
		case strings.HasPrefix(e, "."):
			if strings.HasSuffix(host, e) {
				return false
			}
		case host == e || strings.HasSuffix(host, "."+e):
			return false
		}
	}
	return true
}

// rootCAs is the CA pool of the caller's SSL_CERT_FILE and SSL_CERT_DIR
// (c.Env), nil (the system roots) when neither is set or nothing loads.
func (m *Module) rootCAs(env invoke.Env) *x509.CertPool {
	file, dirs := env.Get("SSL_CERT_FILE"), env.Get("SSL_CERT_DIR")
	if file == "" && dirs == "" {
		return nil
	}
	pool, loaded := x509.NewCertPool(), false
	add := func(p string) {
		if data, err := m.readFile(p); err == nil && pool.AppendCertsFromPEM(data) {
			loaded = true
		}
	}
	if file != "" {
		add(file)
	}
	for _, d := range filepath.SplitList(dirs) {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				add(filepath.Join(d, e.Name()))
			}
		}
	}
	if !loaded {
		return nil
	}
	return pool
}
