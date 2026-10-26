// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math"
	"net"
	"net/url"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// tlsTimeout is the default --timeout of one llm_tls target.
const tlsTimeout = 10 * time.Second

// tlsCert is one certificate of the chain after the leaf.
type tlsCert struct {
	Subject  string `json:"subject"`
	Issuer   string `json:"issuer"`
	NotAfter string `json:"not_after"`
}

var tlsCols = []string{"host", "protocol", "subject", "issuer", "san", "not_after", "days_left", "verified", "verify_error", "chain", "ms", "error"}

// tlsProbe is what llm_tls does with each target: the server name to
// send and verify, the CAs to verify against, the clock.
type tlsProbe struct {
	sni   string
	roots *x509.CertPool
	now   func() time.Time
}

// tlsCheck is llm_tls (plan N3): a TLS handshake with each HOST[:PORT]
// (default port 443; an https:// URL is taken too) and a summary of the
// certificate it presented. The handshake verifies the chain against the
// system CAs (or SSL_CERT_FILE) for the server name, the verified field;
// an expired or self-signed certificate is still described, with the
// reason in verify_error.
func (m *Module) tlsCheck(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(netFlags)
	p := &tlsProbe{now: time.Now}
	targets, ok := netArgs(c, o, "Use --sni NAME, -j N, --timeout S, --format F or HOST[:PORT] targets", func(args []string) (int, bool) {
		if args[0] != "--sni" {
			return 0, true
		}
		v, ok := needValue(c, args)
		if !ok {
			return 0, false
		}
		p.sni = v
		return 2, true
	})
	if !ok {
		return 1
	}
	if len(targets) == 0 {
		return fail(c, "missing_target", "llm_tls needs a HOST[:PORT]", "Pass one or more HOST[:PORT] targets, e.g. example.com mail.example.com:465")
	}
	addrs := make([][2]string, len(targets))
	for i, t := range targets {
		host, port, ok := tlsTarget(t)
		if !ok {
			return fail(c, "invalid_target", t, "Give HOST[:PORT] (port 1 to 65535, default 443) or an https:// URL")
		}
		addrs[i] = [2]string{host, port}
	}
	p.roots = m.rootCAs(c.Env)
	idx := make([]int, len(targets))
	for i := range idx {
		idx[i] = i
	}
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), o.TimeoutOr(tlsTimeout), idx, func(ctx context.Context, i int) []any {
		return p.check(ctx, targets[i], addrs[i][0], addrs[i][1])
	})
	o.Write(c, o.FormatOr(llmfmt.JSON), llmfmt.Table{Cols: tlsCols, Rows: rows}, nil)
	return 0
}

// tlsTarget splits a target into host and port.
func tlsTarget(t string) (string, string, bool) {
	if u, err := url.Parse(t); err == nil && u.Scheme != "" && u.Host != "" {
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return splitHostPort(net.JoinHostPort(u.Hostname(), port), "")
	}
	return splitHostPort(t, "443")
}

// check runs the handshake with one target and describes its leaf.
func (p *tlsProbe) check(ctx context.Context, target, host, port string) []any {
	row := make([]any, len(tlsCols))
	row[0] = target
	name := p.sni
	if name == "" {
		name = host
	}
	start := time.Now()
	var d net.Dialer
	raw, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, port))
	if err != nil {
		row[10], row[11] = llmfmt.Ms(time.Since(start)), netError(ctx, err)
		return row
	}
	// The handshake verifies the chain as a client does. A certificate that
	// does not verify ends it with the certificates the server presented,
	// which are then described with the reason instead of failing the row.
	now := p.now()
	conn := tls.Client(raw, &tls.Config{ServerName: name, RootCAs: p.roots, Time: func() time.Time { return now }})
	err = conn.HandshakeContext(ctx)
	row[10] = llmfmt.Ms(time.Since(start))
	st := conn.ConnectionState()
	_ = conn.Close()
	certs, verr := st.PeerCertificates, error(nil)
	var cve *tls.CertificateVerificationError
	switch {
	case errors.As(err, &cve):
		certs, verr = cve.UnverifiedCertificates, cve.Err
	case err != nil:
		// Connected, then no TLS: whatever the server did, the handshake
		// failed (unless the time ran out).
		row[11] = "tls_error"
		if netError(ctx, err) == "timeout" {
			row[11] = "timeout"
		}
		return row
	}
	if len(certs) == 0 {
		row[11] = "no_certificate"
		return row
	}
	leaf := certs[0]
	san := make([]string, 0, len(leaf.DNSNames)+len(leaf.IPAddresses))
	san = append(san, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		san = append(san, ip.String())
	}
	chain := make([]tlsCert, 0, len(certs)-1)
	for _, ct := range certs[1:] {
		chain = append(chain, tlsCert{Subject: certName(ct.Subject), Issuer: certName(ct.Issuer), NotAfter: ct.NotAfter.UTC().Format(time.RFC3339)})
	}
	row[1] = tls.VersionName(st.Version)
	row[2] = certName(leaf.Subject)
	row[3] = certName(leaf.Issuer)
	row[4] = san
	row[5] = leaf.NotAfter.UTC().Format(time.RFC3339)
	row[6] = int(math.Floor(leaf.NotAfter.Sub(now).Hours() / 24))
	row[7] = verr == nil
	row[8] = nullable(verifyError(verr, leaf, now))
	row[9] = chain
	return row
}

// certName is the common name of a certificate name, its whole
// distinguished name when it has none.
func certName(n pkix.Name) string {
	if n.CommonName != "" {
		return n.CommonName
	}
	return n.String()
}

// verifyError is the verify_error field: why the chain did not verify.
func verifyError(err error, leaf *x509.Certificate, now time.Time) string {
	if err == nil {
		return ""
	}
	var (
		invalid  x509.CertificateInvalidError
		hostname x509.HostnameError
		unknown  x509.UnknownAuthorityError
	)
	switch {
	case errors.As(err, &hostname):
		return "hostname_mismatch"
	case errors.As(err, &unknown):
		return "unknown_authority"
	case errors.As(err, &invalid) && invalid.Reason == x509.Expired:
		if now.Before(leaf.NotBefore) {
			return "not_yet_valid"
		}
		return "expired"
	case now.After(leaf.NotAfter):
		return "expired"
	case now.Before(leaf.NotBefore):
		return "not_yet_valid"
	}
	return "invalid"
}
