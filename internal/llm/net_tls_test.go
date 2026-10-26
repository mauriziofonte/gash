// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// testCA mints a CA and a leaf for names, valid from notBefore to
// notAfter, and serves the leaf plus the CA (as the chain) on 127.0.0.1.
func tlsServerWithChain(t *testing.T, names []string, notBefore, notAfter time.Time) (addr, caFile string) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	caTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Test Root", Organization: []string{"Gash"}},
		NotBefore: time.Now().Add(-48 * time.Hour), NotAfter: time.Now().Add(365 * 24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	require.NoError(t, err)
	ca, err := x509.ParseCertificate(caDER)
	require.NoError(t, err)
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	leafTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: names[0]},
		NotBefore: notBefore, NotAfter: notAfter, DNSNames: names, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTmpl, ca, &key.PublicKey, caKey)
	require.NoError(t, err)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{leafDER, caDER}, PrivateKey: key}}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = c.(*tls.Conn).Handshake()
				_ = c.Close()
			}()
		}
	}()
	caFile = filepath.Join(physTemp(t), "ca.pem")
	require.NoError(t, os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644))
	return ln.Addr().String(), caFile
}

func TestTLSDescribesTheChain(t *testing.T) {
	notAfter := time.Now().Add(90*24*time.Hour + time.Hour).UTC().Truncate(time.Second)
	addr, ca := tlsServerWithChain(t, []string{"svc.test", "www.svc.test"}, time.Now().Add(-time.Hour), notAfter)
	env := map[string]string{"SSL_CERT_FILE": ca}
	rows := netRows(t, netRun(t, env, "llm_tls", addr, "--sni", "www.svc.test"))
	require.Len(t, rows, 1)
	r := rows[0]
	require.Equal(t, addr, r["host"])
	require.Equal(t, "TLS 1.3", r["protocol"])
	require.Equal(t, "svc.test", r["subject"])
	require.Equal(t, "Test Root", r["issuer"])
	require.Equal(t, []any{"svc.test", "www.svc.test", "127.0.0.1"}, r["san"])
	require.Equal(t, notAfter.Format(time.RFC3339), r["not_after"])
	require.EqualValues(t, 90, r["days_left"])
	require.Equal(t, true, r["verified"], r)
	require.Nil(t, r["verify_error"])
	require.Equal(t, []any{map[string]any{"subject": "Test Root", "issuer": "Test Root", "not_after": r["chain"].([]any)[0].(map[string]any)["not_after"]}}, r["chain"])
	require.Nil(t, r["error"])

	// The IP itself is a SAN; another name is not.
	rows = netRows(t, netRun(t, env, "llm_tls", addr, "https://"+addr+"/path"))
	require.Equal(t, true, rows[0]["verified"])
	require.Equal(t, "https://"+addr+"/path", rows[1]["host"])
	rows = netRows(t, netRun(t, env, "llm_tls", addr, "--sni", "other.test"))
	require.Equal(t, false, rows[0]["verified"])
	require.Equal(t, "hostname_mismatch", rows[0]["verify_error"])
	require.Equal(t, "TLS 1.3", rows[0]["protocol"]) // from the handshake the verification ended
	require.Len(t, rows[0]["chain"], 1)
	// Without the CA: described, not verified.
	rows = netRows(t, netRun(t, nil, "llm_tls", addr))
	require.Equal(t, "svc.test", rows[0]["subject"])
	require.Equal(t, "unknown_authority", rows[0]["verify_error"])
}

func TestTLSExpiredIsDescribed(t *testing.T) {
	addr, ca := tlsServerWithChain(t, []string{"old.test"}, time.Now().Add(-72*time.Hour), time.Now().Add(-49*time.Hour))
	rows := netRows(t, netRun(t, map[string]string{"SSL_CERT_FILE": ca}, "llm_tls", addr))
	require.Equal(t, false, rows[0]["verified"])
	require.Equal(t, "expired", rows[0]["verify_error"])
	require.EqualValues(t, -3, rows[0]["days_left"])
	addr, ca = tlsServerWithChain(t, []string{"new.test"}, time.Now().Add(48*time.Hour), time.Now().Add(96*time.Hour))
	rows = netRows(t, netRun(t, map[string]string{"SSL_CERT_FILE": ca}, "llm_tls", addr))
	require.Equal(t, "not_yet_valid", rows[0]["verify_error"])
}

func TestTLSHandshakeFailureIsAField(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "plain") }))
	defer plain.Close()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := ln.Addr().String()
	require.NoError(t, ln.Close())
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	good := srv.Listener.Addr().String()
	rows := netRows(t, netRun(t, nil, "llm_tls", plain.Listener.Addr().String(), closed, good, "--timeout", "2"))
	require.Equal(t, "tls_error", rows[0]["error"])
	require.Nil(t, rows[0]["subject"])
	require.Equal(t, "connection_refused", rows[1]["error"])
	require.Nil(t, rows[2]["error"])
	require.Equal(t, "O=Acme Co", rows[2]["subject"]) // httptest: no common name, the whole DN
}

func TestTLSVerifyErrorMapping(t *testing.T) {
	now := time.Now()
	leaf := &x509.Certificate{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)}
	require.Equal(t, "", verifyError(nil, leaf, now))
	require.Equal(t, "hostname_mismatch", verifyError(x509.HostnameError{Certificate: leaf, Host: "x"}, leaf, now))
	require.Equal(t, "unknown_authority", verifyError(x509.UnknownAuthorityError{}, leaf, now))
	require.Equal(t, "expired", verifyError(x509.CertificateInvalidError{Reason: x509.Expired}, leaf, now.Add(2*time.Hour)))
	require.Equal(t, "not_yet_valid", verifyError(x509.CertificateInvalidError{Reason: x509.Expired}, leaf, now.Add(-2*time.Hour)))
	require.Equal(t, "invalid", verifyError(x509.CertificateInvalidError{Reason: x509.NotAuthorizedToSign}, leaf, now))
	require.Equal(t, "expired", verifyError(context.DeadlineExceeded, leaf, now.Add(2*time.Hour)))
}

func TestTLSArgumentErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "missing_target/RETRY"},
		{[]string{"--sni", "x"}, "missing_target/RETRY"},
		{[]string{"host:0"}, "invalid_target/RETRY"},
		{[]string{"host:99999"}, "invalid_target/RETRY"},
		{[]string{"host:https"}, "invalid_target/RETRY"},
		{[]string{"a:b:c"}, "invalid_target/RETRY"},
		{[]string{"x", "--sni"}, "missing_value/RETRY"},
		{[]string{"x", "--bogus"}, "unknown_option/RETRY"},
	} {
		require.Equal(t, c.want, errCode(t, netRun(t, nil, "llm_tls", c.args...)), c.args)
	}
	for in, want := range map[string][2]string{
		"example.com": {"example.com", "443"}, "example.com:8443": {"example.com", "8443"},
		"[::1]": {"::1", "443"}, "::1": {"::1", "443"}, "[::1]:8443": {"::1", "8443"},
		"https://example.com/x": {"example.com", "443"}, "https://example.com:9443": {"example.com", "9443"},
	} {
		h, p, ok := tlsTarget(in)
		require.True(t, ok, in)
		require.Equal(t, want, [2]string{h, p}, in)
	}
}
