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
// exit codes (LEDGER DOCK-24). The headers of an HTTP error answer are
// kept, for its WWW-Authenticate challenge.
func (x *call) registryCall(method, rawURL string, header http.Header) (reply, string) {
	return x.registryCallAuth(method, rawURL, header, "", "")
}

// registryCallAuth is registryCall with HTTP basic credentials when user
// is not empty.
func (x *call) registryCallAuth(method, rawURL string, header http.Header, user, pass string) (reply, string) {
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
	if user != "" {
		req.SetBasicAuth(user, pass)
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
	r := reply{body: body, header: resp.Header}
	switch {
	case code >= 200 && code <= 299:
		return r, ""
	case code == 401:
		return r, "unauthorized"
	case code == 403:
		return r, "forbidden"
	case code == 404:
		return r, "not_found"
	case code == 429:
		return r, "rate_limited"
	case code >= 500 && code <= 599:
		return r, "server_error"
	}
	return r, fmt.Sprintf("http_%03d", code)
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
// manifest request, or the error class; with user, the token is asked
// with those basic credentials (a GITHUB_TOKEN exchange).
func (x *call) token(rawURL, user, pass string) (string, string) {
	r, e := x.registryCallAuth(http.MethodGet, rawURL, http.Header{"Accept": {"*/*"}}, user, pass)
	if e != "" {
		return "", e
	}
	if t := tokenFrom(r.body); t != "" {
		return t, ""
	}
	return "", "malformed_token"
}

// denied is an answer that another credential may change.
func denied(e string) bool { return e == "unauthorized" || e == "forbidden" }

// remoteDigest is __gash_get_remote_digest: the digest of the manifest
// of img on its registry, by one HEAD (LEDGER DOCK-25), or the error
// class. Docker Hub gets an anonymous token; ghcr.io a token exchanged
// for the exported GITHUB_TOKEN (never sent as it is), or an anonymous
// one when there is none or it is refused (DOCK-14); any other registry
// gets a plain HEAD and, on a Bearer challenge, an anonymous token from
// its realm (DOCK-15). No credential store is read.
func (x *call) remoteDigest(img image) (string, string) {
	if img.digest != "" {
		return img.digest, ""
	}
	if img.bad {
		return "", "invalid_reference"
	}
	scheme := img.scheme
	if scheme == "" {
		scheme = "https"
	}
	u := url.URL{Scheme: scheme, Host: img.host, Path: "/v2/" + img.name + "/manifests/" + img.tag}
	head := func(bearer string) (reply, string) {
		h := http.Header{"Accept": manifestAccept}
		if bearer != "" {
			h.Set("Authorization", "Bearer "+bearer)
		}
		return x.registryCall(http.MethodHead, u.String(), h)
	}
	var r reply
	var e string
	switch img.registry {
	case "docker.io":
		t, te := x.token("https://auth.docker.io/token?service=registry.docker.io&scope=repository:"+queryEscape(img.name)+":pull", "", "")
		if te != "" {
			return "", te
		}
		r, e = head(t)
	case "ghcr.io":
		tokenURL := "https://ghcr.io/token?service=ghcr.io&scope=repository:" + queryEscape(img.name) + ":pull"
		e = "unauthorized"
		if pat := x.exported.Get("GITHUB_TOKEN"); pat != "" {
			t, te := x.token(tokenURL, "gash", pat)
			switch {
			case te == "":
				r, e = head(t)
			case !denied(te):
				return "", te
			}
		}
		if denied(e) {
			t, te := x.token(tokenURL, "", "")
			if te != "" {
				return "", te
			}
			r, e = head(t)
		}
	default:
		r, e = head("")
		if e == "unauthorized" {
			if tokenURL, ok := challengeURL(r.header, img.name); ok {
				t, te := x.token(tokenURL, "", "")
				if te != "" {
					return "", te
				}
				r, e = head(t)
			}
		}
	}
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

// challengeURL is the token URL of a `WWW-Authenticate: Bearer
// realm="...",service="...",scope="..."` challenge: the realm with its
// service and scope (repository:NAME:pull when the challenge names none).
func challengeURL(h http.Header, repo string) (string, bool) {
	for _, v := range h.Values("Www-Authenticate") {
		scheme, rest, _ := strings.Cut(strings.TrimSpace(v), " ")
		if !strings.EqualFold(scheme, "bearer") {
			continue
		}
		params := challengeParams(rest)
		realm, err := url.Parse(params["realm"])
		if err != nil || params["realm"] == "" || (realm.Scheme != "https" && realm.Scheme != "http") {
			continue
		}
		q := realm.Query()
		if s := params["service"]; s != "" {
			q.Set("service", s)
		}
		scope := params["scope"]
		if scope == "" {
			scope = "repository:" + repo + ":pull"
		}
		q.Set("scope", scope)
		realm.RawQuery = q.Encode()
		return realm.String(), true
	}
	return "", false
}

// challengeParams reads the key="value" (or key=token) list of a
// challenge.
func challengeParams(s string) map[string]string {
	out := map[string]string{}
	for s != "" {
		s = strings.TrimLeft(s, " \t,")
		k, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		k = strings.ToLower(strings.TrimSpace(k))
		var v string
		if strings.HasPrefix(rest, `"`) {
			var b strings.Builder
			i := 1
			for ; i < len(rest) && rest[i] != '"'; i++ {
				if rest[i] == '\\' && i+1 < len(rest) {
					i++
				}
				b.WriteByte(rest[i])
			}
			v, s = b.String(), rest[min(i+1, len(rest)):]
		} else {
			v, s, _ = strings.Cut(rest, ",")
			v = strings.TrimSpace(v)
		}
		out[k] = v
	}
	return out
}
