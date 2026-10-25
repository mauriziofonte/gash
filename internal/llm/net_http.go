// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/netip"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/buildinfo"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/jsonpath"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
)

const (
	// httpTimeout is the default --timeout of one URL (redirects and the
	// body included).
	httpTimeout = 10 * time.Second
	// httpBodyMax is the default --body-max: the body bytes kept for
	// --grep and --json-path.
	httpBodyMax = 8 << 20
	// httpMaxRedirects is the most redirects --follow takes.
	httpMaxRedirects = 10
	// httpMatchMax is the longest --grep match line printed, in bytes.
	httpMatchMax = 300
)

// httpSpec is one llm_http call: what every URL gets.
type httpSpec struct {
	method     string
	header     [][2]string // --header, Host excepted
	host       string      // --header "Host: NAME"
	auth       [][2]string // --auth-env: never printed
	secrets    []string    // the --auth-env values, scrubbed from the output
	pins       []httpPin   // --resolve
	follow     bool
	statusOnly bool
	insecure   bool
	pick       []string // --headers, lower case
	path       []jsonpath.Step
	hasPath    bool
	grep       *regexp.Regexp
	bodyMax    int64
	expect     [][2]int // --expect ranges; nil = 200..399
	proxy      func(*http.Request) (*url.URL, error)
	roots      *x509.CertPool
}

// httpPin is one --resolve HOST[:PORT]:IP.
type httpPin struct {
	host, port, ip string
}

// httpHop is one redirect: the status of the 3xx answer and where it
// pointed.
type httpHop struct {
	Status   int    `json:"status"`
	Location string `json:"location"`
}

// httpTiming are the phases of a probe in ms: DNS lookup, TCP connect and
// TLS handshake (summed over the redirect hops; null when the phase did
// not happen), and the time to the first byte of the last answer.
type httpTiming struct {
	DNS     *float64 `json:"dns"`
	Connect *float64 `json:"connect"`
	TLS     *float64 `json:"tls"`
	TTFB    *float64 `json:"ttfb"`
}

var (
	headerTokenRE = lazyre.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
	headerLineRE  = lazyre.MustCompile("^([!#$%&'*+.^_`|~0-9A-Za-z-]+):[ \t]*(.*)$")
	expectRE      = lazyre.MustCompile(`^([1-5])(xx|[0-9][0-9])$`)
)

var errTooManyRedirects = errors.New("too many redirects")

// http is llm_http (plan N1): parallel HTTP probes, read-only.
func (m *Module) http(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(netFlags)
	s := &httpSpec{method: http.MethodGet, bodyMax: httpBodyMax}
	var authVars []string
	urls, ok := netArgs(c, o, "See llm_http --help for the options", func(args []string) (int, bool) {
		switch args[0] {
		case "--follow":
			s.follow = true
			return 1, true
		case "--status-only":
			s.statusOnly = true
			return 1, true
		case "-k", "--insecure":
			s.insecure = true
			return 1, true
		case "--method", "--header", "--auth-env", "--resolve", "--headers", "--json-path", "--grep", "--body-max", "--expect":
		default:
			return 0, true
		}
		v, ok := needValue(c, args)
		if !ok {
			return 0, false
		}
		if args[0] == "--auth-env" {
			authVars = append(authVars, v)
			return 2, true
		}
		return 2, s.option(c, args[0], v)
	})
	if !ok {
		return 1
	}
	if len(urls) == 0 {
		return fail(c, "missing_target", "llm_http needs a URL", "Pass one or more http:// or https:// URLs")
	}
	for _, u := range urls {
		if !httpURLOK(c, u) {
			return 1
		}
	}
	needsBody := s.grep != nil || s.hasPath
	switch {
	case needsBody && s.statusOnly:
		return fail(c, "conflicting_options", "--status-only with --grep or --json-path", "--grep and --json-path read the body; drop --status-only")
	case needsBody && s.method == http.MethodHead:
		return fail(c, "conflicting_options", "--method HEAD with --grep or --json-path", "A HEAD answer has no body; use GET")
	case s.insecure && len(authVars) > 0:
		return fail(c, "conflicting_options", "--insecure with --auth-env", "A credential is never sent to a host whose certificate is not verified")
	}
	for _, v := range authVars {
		if !s.authFromEnv(c, v) {
			return 1
		}
	}
	s.proxy = envProxy(c.Env)
	s.roots = m.rootCAs(c.Env)
	rows := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), o.TimeoutOr(httpTimeout), urls, s.probe)
	o.Write(c, o.FormatOr(llmfmt.JSON), llmfmt.Table{Cols: s.cols(), Rows: rows}, nil)
	return 0
}

// option parses one value option of llm_http.
func (s *httpSpec) option(c *invoke.Call, name, v string) bool {
	switch name {
	case "--method":
		switch up := strings.ToUpper(v); up {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			s.method = up
		default:
			fail(c, "http_method_blocked", v, "llm_http only reads (GET, HEAD, OPTIONS); a method that can change the server's state is a write, and writes belong to the agent_* family")
			return false
		}
	case "--header":
		mm := headerLineRE.FindStringSubmatch(v)
		if mm == nil || strings.ContainsAny(mm[2], "\r\n\x00") {
			fail(c, "invalid_header", v, `Give a header as "Name: value"`)
			return false
		}
		name, value := mm[1], strings.TrimRight(mm[2], " \t")
		if secretHeader(name) {
			fail(c, "secret_in_argv", name, "Never put a credential on the command line: export it (e.g. AUTH='"+name+": ...') and pass --auth-env AUTH")
			return false
		}
		if strings.EqualFold(name, "Host") {
			s.host = value
		} else {
			s.header = append(s.header, [2]string{name, value})
		}
	case "--headers":
		for _, h := range strings.Split(v, ",") {
			h = strings.ToLower(strings.TrimSpace(h))
			if !headerTokenRE.MatchString(h) {
				fail(c, "invalid_header", v, "Give header names separated by commas, e.g. --headers content-type,location")
				return false
			}
			if !containsString(s.pick, h) {
				s.pick = append(s.pick, h)
			}
		}
	case "--resolve":
		p, ok := parsePin(v)
		if !ok {
			fail(c, "invalid_resolve", v, "Give HOST:IP or HOST:PORT:IP, e.g. --resolve example.com:127.0.0.1")
			return false
		}
		s.pins = append(s.pins, p)
	case "--json-path":
		steps, err := jsonpath.Parse(v)
		if err != nil {
			fail(c, "invalid_json_path", v, `Use dotted keys and [index] steps, e.g. data.items[0].id or ["a.b"].c`)
			return false
		}
		s.path, s.hasPath = steps, true
	case "--grep":
		re, err := regexp.Compile(v)
		if err != nil {
			fail(c, "invalid_regex", v, "--grep takes a regular expression (RE2 syntax)")
			return false
		}
		s.grep = re
	case "--body-max":
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || len(v) > 9 || v != strconv.Itoa(n) {
			fail(c, "invalid_body_max", v, "Give a positive number of bytes (1 to 999999999)")
			return false
		}
		s.bodyMax = int64(n)
	case "--expect":
		s.expect = nil
		for _, e := range strings.Split(v, ",") {
			mm := expectRE.FindStringSubmatch(strings.TrimSpace(e))
			if mm == nil {
				fail(c, "invalid_expect", v, "Give status codes or classes separated by commas, e.g. 200, 2xx or 200,301")
				return false
			}
			lo, _ := strconv.Atoi(mm[1] + "00")
			hi := lo + 99
			if mm[2] != "xx" {
				lo, _ = strconv.Atoi(mm[1] + mm[2])
				hi = lo
			}
			s.expect = append(s.expect, [2]int{lo, hi})
		}
	}
	return true
}

// authFromEnv adds the header held by the exported variable name: its
// value is "Name: value", or a bare value meaning "Authorization: value".
// The value is never printed, not even in an error.
func (s *httpSpec) authFromEnv(c *invoke.Call, name string) bool {
	if !envNameRE.MatchString(name) {
		fail(c, "invalid_auth_env", name, "Give the name of an exported variable, e.g. --auth-env API_AUTH")
		return false
	}
	v := c.Env.Get(name)
	if v == "" {
		fail(c, "auth_env_unset", name, "Export "+name+` first, holding "Name: value" or the Authorization value (e.g. "Bearer ...")`)
		return false
	}
	if strings.ContainsAny(v, "\r\n\x00") {
		fail(c, "invalid_auth_env", name, name+" must hold one header line: \"Name: value\" or the Authorization value")
		return false
	}
	h := [2]string{"Authorization", strings.TrimSpace(v)}
	if mm := headerLineRE.FindStringSubmatch(v); mm != nil {
		h = [2]string{mm[1], strings.TrimRight(mm[2], " \t")}
	}
	s.auth = append(s.auth, h)
	// A server may echo the credential: it never reaches the output.
	for _, sec := range []string{h[1], strings.TrimSpace(strings.TrimPrefix(h[1], "Bearer "))} {
		if len(sec) >= 4 && !containsString(s.secrets, sec) {
			s.secrets = append(s.secrets, sec)
		}
	}
	return true
}

// httpURLOK refuses a URL that is not http(s) with a host, and one that
// carries a password (a secret on the command line).
func httpURLOK(c *invoke.Call, raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.Hostname() == "" {
		fail(c, "invalid_target", raw, "Give an absolute http:// or https:// URL")
		return false
	}
	if _, has := u.User.Password(); has {
		fail(c, "secret_in_argv", u.Redacted(), "Never put a password in the URL: export the Authorization header and pass --auth-env VAR")
		return false
	}
	return true
}

// secretHeader reports a request header name that carries a credential.
func secretHeader(name string) bool {
	n := strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return n == "COOKIE" || envSecretRE.MatchString(n)
}

// parsePin parses --resolve HOST:IP or HOST:PORT:IP (curl's form).
func parsePin(v string) (httpPin, bool) {
	host, rest, ok := strings.Cut(v, ":")
	if !ok || host == "" || strings.ContainsAny(host, " \t/[]") {
		return httpPin{}, false
	}
	p := httpPin{host: strings.ToLower(host)}
	if a, err := netip.ParseAddr(strings.Trim(rest, "[]")); err == nil {
		p.ip = a.String()
		return p, true
	}
	port, ip, ok := strings.Cut(rest, ":")
	if n, err := strconv.Atoi(port); !ok || err != nil || n < 1 || n > 65535 {
		return httpPin{}, false
	}
	a, err := netip.ParseAddr(strings.Trim(ip, "[]"))
	if err != nil {
		return httpPin{}, false
	}
	p.port, p.ip = port, a.String()
	return p, true
}

// pinFor returns the --resolve address of host:port, if any.
func (s *httpSpec) pinFor(host, port string) (string, bool) {
	host = strings.ToLower(host)
	for _, p := range s.pins {
		if p.host == host && (p.port == "" || p.port == port) {
			return p.ip, true
		}
	}
	return "", false
}

// cols are the columns of the call: the probe's own, then what the
// options asked for.
func (s *httpSpec) cols() []string {
	cols := []string{"url", "status", "ms"}
	if !s.statusOnly {
		cols = append(cols, "timing", "bytes", "redirects")
	}
	if len(s.pick) > 0 {
		cols = append(cols, "headers")
	}
	if s.hasPath {
		cols = append(cols, "value")
	}
	if s.grep != nil {
		cols = append(cols, "match", "matches")
	}
	return append(cols, "ok", "error")
}

// httpResult is what one probe found.
type httpResult struct {
	status    int
	ms        float64
	timing    httpTiming
	bytes     int64
	hops      []httpHop
	headers   map[string]any
	value     any
	match     string
	matches   int
	pathError string
	err       string
}

// probe requests one URL under ctx and returns its row.
func (s *httpSpec) probe(ctx context.Context, raw string) []any {
	r := s.fetch(ctx, raw)
	row := []any{raw, nil, r.ms}
	if r.status > 0 {
		row[1] = r.status
	}
	if !s.statusOnly {
		row = append(row, r.timing, r.bytes, r.hops)
	}
	if len(s.pick) > 0 {
		var h any
		if r.headers != nil {
			h = r.headers
		}
		row = append(row, h)
	}
	if s.hasPath {
		row = append(row, r.value)
	}
	if s.grep != nil {
		row = append(row, nullable(r.match), r.matches)
	}
	errField := r.err
	if errField == "" {
		errField = r.pathError
	}
	ok := r.err == "" && r.status > 0 && s.statusOK(r.status) &&
		(s.grep == nil || r.matches > 0) && (!s.hasPath || r.pathError == "")
	return append(row, ok, nullable(errField))
}

// statusOK applies --expect (default: 200 to 399).
func (s *httpSpec) statusOK(code int) bool {
	if s.expect == nil {
		return code >= 200 && code < 400
	}
	for _, e := range s.expect {
		if code >= e[0] && code <= e[1] {
			return true
		}
	}
	return false
}

// fetch runs the request of one URL, its redirects with --follow, and
// reads the body as the options need it.
func (s *httpSpec) fetch(ctx context.Context, raw string) (r httpResult) {
	r.hops = []httpHop{}
	start := time.Now()
	ph := &phases{}
	tr := s.transport()
	defer tr.CloseIdleConnections()
	origin, _ := url.Parse(raw)
	client := &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if !s.follow {
			return http.ErrUseLastResponse
		}
		if req.Response != nil {
			r.hops = append(r.hops, httpHop{Status: req.Response.StatusCode, Location: req.URL.String()})
		}
		if len(via) >= httpMaxRedirects {
			return errTooManyRedirects
		}
		if !strings.EqualFold(req.URL.Host, origin.Host) {
			// A credential never follows a redirect to another host.
			for _, h := range s.auth {
				req.Header.Del(h[0])
			}
		}
		return nil
	}}
	req, err := http.NewRequestWithContext(httptrace.WithClientTrace(ctx, ph.trace()), s.method, raw, nil)
	if err != nil {
		r.err = "transport_error"
		return r
	}
	for _, h := range s.header {
		req.Header.Add(h[0], h[1])
	}
	for _, h := range s.auth {
		req.Header.Add(h[0], h[1])
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "gash/"+buildinfo.Version)
	}
	if req.Header.Get("Accept") == "" {
		req.Header.Set("Accept", "*/*")
	}
	if s.host != "" {
		req.Host = s.host
	}
	resp, err := client.Do(req)
	defer func() {
		r.ms = llmfmt.Ms(time.Since(start))
		r.timing = ph.timing(start)
	}()
	if err != nil {
		if errors.Is(err, errTooManyRedirects) {
			r.err = "too_many_redirects"
		} else {
			r.err = netError(ctx, err)
		}
		return r
	}
	defer func() { _ = resp.Body.Close() }()
	r.status = resp.StatusCode
	if loc := resp.Header.Get("Location"); loc != "" && r.status >= 300 && r.status < 400 {
		if u, err := resp.Request.URL.Parse(loc); err == nil {
			loc = u.String()
		}
		r.hops = append(r.hops, httpHop{Status: r.status, Location: s.scrub(loc)})
	}
	if len(s.pick) > 0 {
		r.headers = map[string]any{}
		for _, name := range s.pick {
			var v any
			if vals := resp.Header.Values(name); len(vals) > 0 {
				v = s.scrub(maskHeader(name, vals))
			}
			r.headers[name] = v
		}
	}
	if s.statusOnly || s.method == http.MethodHead {
		return r
	}
	var kept bytes.Buffer
	var n int64
	if s.grep != nil || s.hasPath {
		n, err = io.CopyN(&kept, resp.Body, s.bodyMax)
		if errors.Is(err, io.EOF) {
			err = nil
		} else if err == nil {
			var more int64
			more, err = io.Copy(io.Discard, resp.Body)
			n += more
		}
	} else {
		n, err = io.Copy(io.Discard, resp.Body)
	}
	r.bytes = n
	if err != nil {
		r.err = netError(ctx, err)
		return r
	}
	if s.grep != nil {
		r.match, r.matches = grepBody(s.grep, kept.Bytes())
		r.match = s.scrub(r.match)
	}
	if s.hasPath {
		r.value, r.pathError = s.extract(kept.Bytes(), n > s.bodyMax)
	}
	return r
}

// transport is the HTTP transport of one URL: its own connections (a
// probe measures its own connect), the --resolve pins, the proxy of the
// caller's environment (never for a pinned host) and its CAs.
func (s *httpSpec) transport() *http.Transport {
	d := &net.Dialer{KeepAlive: -1}
	tc := &tls.Config{RootCAs: s.roots, InsecureSkipVerify: s.insecure} //nolint:gosec // --insecure is the caller's explicit choice
	return &http.Transport{
		Proxy: func(r *http.Request) (*url.URL, error) {
			port := r.URL.Port()
			if port == "" {
				port = map[string]string{"http": "80", "https": "443"}[r.URL.Scheme]
			}
			if _, pinned := s.pinFor(r.URL.Hostname(), port); pinned {
				return nil, nil
			}
			return s.proxy(r)
		},
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if host, port, err := net.SplitHostPort(addr); err == nil {
				if ip, ok := s.pinFor(host, port); ok {
					addr = net.JoinHostPort(ip, port)
				}
			}
			return d.DialContext(ctx, network, addr)
		},
		TLSClientConfig:     tc,
		ForceAttemptHTTP2:   true,
		MaxIdleConnsPerHost: 1,
		OnProxyConnectResponse: func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
			if resp.StatusCode != http.StatusOK {
				return errProxyRefused
			}
			return nil
		},
	}
}

var errProxyRefused = errors.New("proxy refused the tunnel")

// phases records the httptrace events of one probe.
type phases struct {
	mu                 sync.Mutex
	dnsStart, tlsStart time.Time
	connStart          map[string]time.Time
	dns, conn, tls     time.Duration
	sawDNS, sawConn    bool
	sawTLS             bool
	firstByte          time.Time
}

func (p *phases) trace() *httptrace.ClientTrace {
	lock := func(f func()) {
		p.mu.Lock()
		defer p.mu.Unlock()
		f()
	}
	return &httptrace.ClientTrace{
		DNSStart: func(httptrace.DNSStartInfo) { lock(func() { p.dnsStart = time.Now() }) },
		DNSDone: func(httptrace.DNSDoneInfo) {
			lock(func() { p.dns += time.Since(p.dnsStart); p.sawDNS = true })
		},
		ConnectStart: func(_, addr string) {
			lock(func() {
				if p.connStart == nil {
					p.connStart = map[string]time.Time{}
				}
				p.connStart[addr] = time.Now()
			})
		},
		ConnectDone: func(_, addr string, err error) {
			lock(func() {
				if err == nil {
					p.conn += time.Since(p.connStart[addr])
					p.sawConn = true
				}
			})
		},
		TLSHandshakeStart: func() { lock(func() { p.tlsStart = time.Now() }) },
		TLSHandshakeDone: func(tls.ConnectionState, error) {
			lock(func() { p.tls += time.Since(p.tlsStart); p.sawTLS = true })
		},
		GotFirstResponseByte: func() { lock(func() { p.firstByte = time.Now() }) },
	}
}

func (p *phases) timing(start time.Time) httpTiming {
	p.mu.Lock()
	defer p.mu.Unlock()
	ms := func(saw bool, d time.Duration) *float64 {
		if !saw {
			return nil
		}
		v := llmfmt.Ms(d)
		return &v
	}
	return httpTiming{
		DNS: ms(p.sawDNS, p.dns), Connect: ms(p.sawConn, p.conn), TLS: ms(p.sawTLS, p.tls),
		TTFB: ms(!p.firstByte.IsZero(), p.firstByte.Sub(start)),
	}
}

// maskHeader is the printed value of a response header, before scrub: a
// credential is masked (a cookie keeps its name and attributes).
func maskHeader(name string, vals []string) string {
	out := make([]string, len(vals))
	for i, v := range vals {
		switch name {
		case "authorization", "proxy-authorization", "cookie":
			v = "***"
		case "set-cookie":
			first, attrs, _ := strings.Cut(v, ";")
			if k, _, ok := strings.Cut(first, "="); ok {
				first = strings.TrimSpace(k) + "=***"
			}
			if attrs != "" {
				first += ";" + attrs
			}
			v = first
		}
		out[i] = v
	}
	return strings.Join(out, ", ")
}

// grepBody returns the first line of body that re matches (a window
// around the match when the line is long, marked with …) and how many
// lines match.
func grepBody(re *regexp.Regexp, body []byte) (string, int) {
	first, n := "", 0
	for len(body) > 0 {
		line := body
		if i := bytes.IndexByte(body, '\n'); i >= 0 {
			line, body = body[:i], body[i+1:]
		} else {
			body = nil
		}
		line = bytes.TrimSuffix(line, []byte("\r"))
		loc := re.FindIndex(line)
		if loc == nil {
			continue
		}
		if n == 0 {
			first = matchWindow(line, loc[0])
		}
		n++
	}
	return first, n
}

// matchWindow cuts a long line to httpMatchMax bytes around the match
// start at, on rune boundaries.
func matchWindow(line []byte, at int) string {
	if len(line) <= httpMatchMax {
		return string(line)
	}
	from := max(0, at-httpMatchMax/3)
	for from > 0 && from < len(line) && line[from]&0xC0 == 0x80 {
		from--
	}
	to := min(len(line), from+httpMatchMax)
	for to < len(line) && line[to]&0xC0 == 0x80 {
		to--
	}
	s := string(line[from:to])
	if from > 0 {
		s = "…" + s
	}
	if to < len(line) {
		s += "…"
	}
	return s
}

// extract applies --json-path to the body: the value (secrets masked) or
// the error field.
func (s *httpSpec) extract(body []byte, cut bool) (any, string) {
	if cut {
		return nil, "body_max_exceeded"
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, "invalid_json"
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, "invalid_json"
	}
	got, _, ok := jsonpath.Get(v, s.path)
	if !ok {
		return nil, "json_path_not_found"
	}
	return s.scrubJSON(got), ""
}

// scrub masks in a printed string the --auth-env values, then the known
// token formats and URL passwords, as llm_env masks them.
func (s *httpSpec) scrub(v string) string {
	for _, sec := range s.secrets {
		v = strings.ReplaceAll(v, sec, "***")
	}
	return envRedact(v)
}

// scrubJSON scrubs every string of a decoded JSON value.
func (s *httpSpec) scrubJSON(v any) any {
	switch x := v.(type) {
	case string:
		return s.scrub(x)
	case []any:
		for i := range x {
			x[i] = s.scrubJSON(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = s.scrubJSON(x[k])
		}
	}
	return v
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
