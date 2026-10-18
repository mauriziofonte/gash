// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// The 2.x curl limits: --connect-timeout 10 --max-time 20, and the
// redirect limit of curl -L.
const (
	connectTimeout = 10 * time.Second
	callTimeout    = 20 * time.Second
	maxRedirects   = 50
)

var errTooManyRedirects = errors.New("too many redirects")

// manifestAccept are the Accept values of a manifest request, the
// multi-arch index types first (a pull by tag records the index digest).
var manifestAccept = []string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
	"application/vnd.docker.distribution.manifest.v2+json",
}

func (x *call) client() *http.Client {
	if x.m.HTTP != nil {
		return x.m.HTTP
	}
	return &http.Client{
		// The proxy and CA variables of the process, which is the
		// caller's exported environment (LEDGER DOCK-23).
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: connectTimeout}).DialContext,
			TLSHandshakeTimeout: connectTimeout,
			ForceAttemptHTTP2:   true,
		},
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) > maxRedirects {
				return errTooManyRedirects
			}
			return nil
		},
	}
}

// reply is the final answer of a registry call.
type reply struct {
	body   []byte
	header http.Header
}

// registryCall is __gash_registry_curl: one request (redirects
// followed), "" and the final answer on a 2xx status, else the error
// class of the 2.x vocabulary, with net/http errors in place of curl's
// exit codes (LEDGER DOCK-24).
func (x *call) registryCall(method, rawURL string, header http.Header) (reply, string) {
	ctx, cancel := context.WithTimeout(x.ctx(), callTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, rawURL, nil)
	if err != nil {
		return reply{}, "transport_error"
	}
	for k, vs := range header {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	resp, err := x.client().Do(req)
	if err != nil {
		return reply{}, errorClass(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return reply{}, errorClass(err)
	}
	code := resp.StatusCode
	switch {
	case code >= 200 && code <= 299:
		return reply{body: body, header: resp.Header}, ""
	case code == 401:
		return reply{}, "unauthorized"
	case code == 403:
		return reply{}, "forbidden"
	case code == 404:
		return reply{}, "not_found"
	case code == 429:
		return reply{}, "rate_limited"
	case code >= 500 && code <= 599:
		return reply{}, "server_error"
	}
	return reply{}, fmt.Sprintf("http_%03d", code)
}

// errorClass maps a net/http failure onto the curl error words: a DNS
// failure, a refused connection, any deadline, a TLS or certificate
// failure, and transport_error for every other one.
func errorClass(err error) string {
	var dns *net.DNSError
	if errors.As(err, &dns) {
		return "dns_error"
	}
	if errors.Is(err, syscall.ECONNREFUSED) {
		return "connection_refused"
	}
	var ne net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &ne) && ne.Timeout() {
		return "timeout"
	}
	var (
		rec      tls.RecordHeaderError
		alert    tls.AlertError
		verify   *tls.CertificateVerificationError
		unknown  x509.UnknownAuthorityError
		hostname x509.HostnameError
		invalid  x509.CertificateInvalidError
	)
	if errors.As(err, &rec) || errors.As(err, &alert) || errors.As(err, &verify) || errors.As(err, &unknown) ||
		errors.As(err, &hostname) || errors.As(err, &invalid) {
		return "ssl_error"
	}
	return "transport_error"
}

// queryEscape writes a repository name into a query string as 2.x's URL
// text had it: raw, but for the bytes a URL cannot carry there (blanks
// and controls, non-ASCII, and " # % < > [ \\ ] ^ ` { | }), escaped.
func queryEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c <= ' ' || c >= 0x7f || strings.IndexByte("\"#%<>[\\]^`{|}", c) >= 0 {
			fmt.Fprintf(&b, "%%%02X", c)
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

// tokenFrom is `jq -r '.token // .access_token // empty'` on a token
// answer: "" when it is not JSON, not an object, or has neither key
// (2.x without jq cut the text instead; the jq rule is kept).
func tokenFrom(body []byte) string {
	dec := json.NewDecoder(bytes.NewReader(bytes.ReplaceAll(body, []byte{0}, nil)))
	dec.UseNumber()
	var outs []string
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return ""
		}
		if v == nil {
			continue
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		for _, k := range []string{"token", "access_token"} {
			if t := obj[k]; t != nil && t != false {
				outs = append(outs, jqRaw(t))
				break
			}
		}
	}
	return strings.TrimRight(strings.Join(outs, "\n"), "\n")
}

// jqRaw is a value as jq -r prints it.
func jqRaw(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	if n, ok := v.(json.Number); ok {
		return n.String()
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	return string(out)
}

// token is __gash_dockerhub_token and __gash_ghcr_token: the bearer of a
// manifest request, or the error class.
func (x *call) token(rawURL string) (string, string) {
	r, e := x.registryCall(http.MethodGet, rawURL, http.Header{"Accept": {"*/*"}})
	if e != "" {
		return "", e
	}
	if t := tokenFrom(r.body); t != "" {
		return t, ""
	}
	return "", "malformed_token"
}

// remoteDigest is __gash_get_remote_digest REGISTRY NAME TAG: Docker Hub
// with an anonymous token, ghcr.io with GITHUB_TOKEN as the bearer when
// it is exported (LEDGER DOCK-14, kept) or an anonymous token, any other
// registry with one plain HEAD (DOCK-15, kept). The digest is the
// Docker-Content-Digest of the final answer (DOCK-25).
func (x *call) remoteDigest(img image) (string, string) {
	if strings.HasPrefix(img.tag, "@sha256:") {
		return img.tag[1:], ""
	}
	h := http.Header{"Accept": manifestAccept}
	host := img.registry
	switch img.registry {
	case "docker.io":
		t, e := x.token("https://auth.docker.io/token?service=registry.docker.io&scope=repository:" + queryEscape(img.name) + ":pull")
		if e != "" {
			return "", e
		}
		h.Set("Authorization", "Bearer "+t)
		host = "registry-1.docker.io"
	case "ghcr.io":
		t := x.exported.Get("GITHUB_TOKEN")
		if t == "" {
			var e string
			if t, e = x.token("https://ghcr.io/token?service=ghcr.io&scope=repository:" + queryEscape(img.name) + ":pull"); e != "" {
				return "", e
			}
		}
		h.Set("Authorization", "Bearer "+t)
	}
	u := url.URL{Scheme: "https", Host: host, Path: "/v2/" + img.name + "/manifests/" + img.tag}
	r, e := x.registryCall(http.MethodHead, u.String(), h)
	if e != "" {
		return "", e
	}
	// grep -i docker-content-digest: | awk '{print $2}' | tr -d '\r\n'
	var digest strings.Builder
	for _, v := range r.header.Values("Docker-Content-Digest") {
		if f := strings.FieldsFunc(v, func(r rune) bool { return r == ' ' || r == '\t' }); len(f) > 0 {
			digest.WriteString(strings.NewReplacer("\r", "", "\n", "").Replace(f[0]))
		}
	}
	if digest.Len() == 0 {
		return "", "no_digest_in_response"
	}
	return digest.String(), ""
}
