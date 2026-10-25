// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// msRE is a timing field, for comparing outputs of two runs.
var msRE = regexp.MustCompile(`"ms":[0-9.]+`)

// netRun runs a network function in a temporary directory with the base
// environment plus extra, and no proxy from the test's own environment.
func netRun(t *testing.T, extra map[string]string, fn string, args ...string) result {
	t.Helper()
	env := baseEnv(physTemp(t), extra)
	return runGo(t, New(), physTemp(t), env, fn, args...)
}

// netRows decodes a JSON array output.
func netRows(t *testing.T, r result) []map[string]any {
	t.Helper()
	require.Equal(t, 0, r.RC, r.Stderr)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.Stdout), &rows), r.Stdout)
	return rows
}

// errCode is the error code of an envelope on stderr.
func errCode(t *testing.T, r result) string {
	t.Helper()
	require.Equal(t, 1, r.RC, r.Stdout)
	var e struct{ Error, Action string }
	require.NoError(t, json.Unmarshal([]byte(r.Stderr), &e), r.Stderr)
	return e.Error + "/" + e.Action
}

// httpFake is the test server of llm_http.
func httpFake(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "hello\nworld ERROR one\nERROR two\n")
	})
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Add("Set-Cookie", "sid=s3cr3tvalue; Path=/; HttpOnly")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "done", "auth": r.Header.Get("Authorization"), "key": r.Header.Get("X-Api-Key"),
			"host": r.Host, "ua": r.Header.Get("User-Agent"), "token": "ghp_abcdefghijklmnopqrstuvwxyz0123",
			"items": []any{map[string]any{"id": 1}, map[string]any{"id": 2.5}},
		})
	})
	mux.HandleFunc("/r1", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/r2", http.StatusMovedPermanently) })
	mux.HandleFunc("/r2", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/json", http.StatusFound) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	mux.HandleFunc("/slow", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(3 * time.Second):
		case <-r.Context().Done():
		}
	})
	mux.HandleFunc("/big", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"a":"`+strings.Repeat("x", 5000)+`"}`)
	})
	mux.HandleFunc("/long", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, strings.Repeat("a", 1000)+"NEEDLE"+strings.Repeat("b", 1000)+"\n")
	})
	mux.HandleFunc("/method", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method)
		_, _ = io.WriteString(w, r.Method)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestHTTPDefaultRow(t *testing.T) {
	srv := httpFake(t)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/ok", srv.URL+"/nope"))
	require.Len(t, rows, 2)
	keys := func(m map[string]any) []string {
		var k []string
		for x := range m {
			k = append(k, x)
		}
		return k
	}
	require.ElementsMatch(t, []string{"url", "status", "ms", "timing", "bytes", "redirects", "ok", "error"}, keys(rows[0]))
	require.Equal(t, srv.URL+"/ok", rows[0]["url"])
	require.EqualValues(t, 200, rows[0]["status"])
	require.EqualValues(t, 32, rows[0]["bytes"])
	require.Equal(t, true, rows[0]["ok"])
	require.Nil(t, rows[0]["error"])
	require.Equal(t, []any{}, rows[0]["redirects"])
	timing := rows[0]["timing"].(map[string]any)
	require.Nil(t, timing["tls"])
	require.NotNil(t, timing["connect"])
	require.NotNil(t, timing["ttfb"])
	require.EqualValues(t, 404, rows[1]["status"])
	require.Equal(t, false, rows[1]["ok"])
	require.Nil(t, rows[1]["error"])
}

func TestHTTPStatusOnlyAndExpect(t *testing.T) {
	srv := httpFake(t)
	r := netRun(t, nil, "llm_http", srv.URL+"/ok", srv.URL+"/nope", "--status-only", "--expect", "200,4xx")
	require.Regexp(t, `^\[\{"url":"[^"]+/ok","status":200,"ms":[0-9.]+,"ok":true,"error":null\},\{"url":"[^"]+/nope","status":404,"ms":[0-9.]+,"ok":true,"error":null\}\]\n$`, r.Stdout)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/ok", "--status-only", "--expect", "3xx"))
	require.Equal(t, false, rows[0]["ok"])
	for _, bad := range []string{"600", "2x", "20", "abc", "200,", "6xx"} {
		require.Equal(t, "invalid_expect/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--expect", bad)), bad)
	}
}

func TestHTTPRedirects(t *testing.T) {
	srv := httpFake(t)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/r1"))
	require.EqualValues(t, 301, rows[0]["status"])
	require.Equal(t, []any{map[string]any{"status": 301.0, "location": srv.URL + "/r2"}}, rows[0]["redirects"])
	require.Equal(t, true, rows[0]["ok"])

	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/r1", "--follow", "--json-path", "status"))
	require.EqualValues(t, 200, rows[0]["status"])
	require.Equal(t, []any{
		map[string]any{"status": 301.0, "location": srv.URL + "/r2"},
		map[string]any{"status": 302.0, "location": srv.URL + "/json"},
	}, rows[0]["redirects"])
	require.Equal(t, "done", rows[0]["value"])

	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/loop", "--follow"))
	require.Equal(t, "too_many_redirects", rows[0]["error"])
	require.Len(t, rows[0]["redirects"], 10)
	require.Equal(t, false, rows[0]["ok"])
}

func TestHTTPHeadersJSONPathGrep(t *testing.T) {
	srv := httpFake(t)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/json", srv.URL+"/ok",
		"--headers", "Content-Type,set-cookie,x-none", "--json-path", "items[-1].id", "--grep", "ERROR|done"))
	require.Equal(t, map[string]any{"content-type": "application/json", "set-cookie": "sid=***; Path=/; HttpOnly", "x-none": nil}, rows[0]["headers"])
	require.Equal(t, 2.5, rows[0]["value"])
	require.Contains(t, rows[0]["match"], `"status":"done"`)
	require.EqualValues(t, 1, rows[0]["matches"])
	require.Equal(t, true, rows[0]["ok"])
	require.Nil(t, rows[1]["value"])
	require.Equal(t, "invalid_json", rows[1]["error"])
	require.Equal(t, "world ERROR one", rows[1]["match"])
	require.EqualValues(t, 2, rows[1]["matches"])
	require.Equal(t, false, rows[1]["ok"])

	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/json", "--json-path", "items[5]"))
	require.Equal(t, "json_path_not_found", rows[0]["error"])
	require.Equal(t, false, rows[0]["ok"])

	// Known token formats are masked in values, as llm_env masks them.
	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/json", "--json-path", "token"))
	require.Equal(t, "***", rows[0]["value"])

	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/ok", "--grep", "nomatch"))
	require.Nil(t, rows[0]["match"])
	require.EqualValues(t, 0, rows[0]["matches"])
	require.Equal(t, false, rows[0]["ok"])

	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/long", "--grep", "NEEDLE"))
	m := rows[0]["match"].(string)
	require.Contains(t, m, "NEEDLE")
	require.True(t, strings.HasPrefix(m, "…") && strings.HasSuffix(m, "…"), m)
	require.LessOrEqual(t, len(m), httpMatchMax+len("……"))
}

func TestHTTPBodyMax(t *testing.T) {
	srv := httpFake(t)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/big", "--json-path", "a", "--body-max", "100"))
	require.Equal(t, "body_max_exceeded", rows[0]["error"])
	require.EqualValues(t, 5008, rows[0]["bytes"])
	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/big", "--grep", `^\{"a"`, "--body-max", "100"))
	require.EqualValues(t, 1, rows[0]["matches"])
	for _, bad := range []string{"0", "-1", "x", "1000000000"} {
		require.Equal(t, "invalid_body_max/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--body-max", bad)), bad)
	}
}

func TestHTTPMethods(t *testing.T) {
	srv := httpFake(t)
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/method", "--method", "head", "--headers", "x-method"))
	require.Equal(t, map[string]any{"x-method": "HEAD"}, rows[0]["headers"])
	require.EqualValues(t, 0, rows[0]["bytes"])
	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL+"/method", "--method", "OPTIONS", "--grep", "OPTIONS"))
	require.EqualValues(t, 1, rows[0]["matches"])
	for _, m := range []string{"POST", "put", "PATCH", "DELETE", "PURGE", "TRACE", "CONNECT", "G ET"} {
		require.Equal(t, "http_method_blocked/FATAL", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--method", m)), m)
	}
	require.Equal(t, "conflicting_options/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--method", "HEAD", "--grep", "x")))
	require.Equal(t, "conflicting_options/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--status-only", "--json-path", "a")))
}

func TestHTTPRequestHeadersAndResolve(t *testing.T) {
	srv := httpFake(t)
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	// --resolve pins a name that does not exist to the test server.
	rows := netRows(t, netRun(t, nil, "llm_http", "http://svc.invalid:"+port+"/json", "--resolve", "svc.invalid:127.0.0.1",
		"--header", "Host: vhost.example", "--header", "User-Agent: probe/1", "--json-path", "host"))
	require.Equal(t, "vhost.example", rows[0]["value"], rows)
	rows = netRows(t, netRun(t, nil, "llm_http", "http://svc.invalid:"+port+"/json", "--resolve", "svc.invalid:"+port+":127.0.0.1", "--json-path", "ua"))
	require.Regexp(t, `^gash/`, rows[0]["value"])
	// A pin for another port does not apply (unit test: the lookup would
	// leave the machine).
	sp := &httpSpec{pins: []httpPin{{host: "svc.invalid", port: "1", ip: "127.0.0.1"}}}
	_, pinned := sp.pinFor("svc.invalid", port)
	require.False(t, pinned)
	ip, pinned := sp.pinFor("SVC.invalid", "1")
	require.True(t, pinned)
	require.Equal(t, "127.0.0.1", ip)
	for _, bad := range []string{"svc", "svc:", ":1.2.3.4", "svc:notanip", "svc:99999:1.2.3.4", "svc:80:nope"} {
		require.Equal(t, "invalid_resolve/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--resolve", bad)), bad)
	}
	for _, bad := range []string{"NoColon", "Bad Name: x", ": x", "X: a\nb"} {
		require.Equal(t, "invalid_header/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--header", bad)), bad)
	}
	require.Equal(t, "invalid_header/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--headers", "a,,b")))
}

// TestHTTPSecretsNeverInArgvNorOutput: credentials go through
// --auth-env only, reach the server, and never reach the output.
func TestHTTPSecretsNeverInArgvNorOutput(t *testing.T) {
	srv := httpFake(t)
	for _, h := range []string{"Authorization: Bearer xsecret1", "cookie: a=bsecret2", "X-Api-Key: ksecret3", "X-Auth-Token: tsecret4", "Proxy-Authorization: xsecret5"} {
		r := netRun(t, nil, "llm_http", srv.URL, "--header", h)
		require.Equal(t, "secret_in_argv/RETRY", errCode(t, r), h)
		require.NotContains(t, r.Stderr, strings.SplitN(h, ": ", 2)[1])
	}
	r := netRun(t, nil, "llm_http", "http://user:pa55word@127.0.0.1/")
	require.Equal(t, "secret_in_argv/RETRY", errCode(t, r))
	require.NotContains(t, r.Stderr, "pa55word")

	env := map[string]string{"API_AUTH": "Authorization: Bearer topsecret-1", "API_KEY_HDR": "X-Api-Key: keyvalue-2", "BARE": "Bearer bare-3"}
	r = netRun(t, env, "llm_http", srv.URL+"/json", "--auth-env", "API_AUTH", "--auth-env", "API_KEY_HDR", "--json-path", ".", "--headers", "set-cookie")
	rows := netRows(t, r)
	v := rows[0]["value"].(map[string]any)
	require.Equal(t, "***", v["auth"], "the server got the header, the echo is masked")
	require.Equal(t, "***", v["key"])
	for _, s := range []string{"topsecret-1", "keyvalue-2", "s3cr3tvalue"} {
		require.NotContains(t, r.Stdout+r.Stderr, s)
	}
	rows = netRows(t, netRun(t, env, "llm_http", srv.URL+"/json", "--auth-env", "BARE", "--json-path", "auth"))
	require.Equal(t, "***", rows[0]["value"])

	require.Equal(t, "auth_env_unset/STOP", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--auth-env", "NOPE")))
	require.Equal(t, "invalid_auth_env/RETRY", errCode(t, netRun(t, nil, "llm_http", srv.URL, "--auth-env", "1BAD")))
	r = netRun(t, map[string]string{"ML": "Authorization: a\nb"}, "llm_http", srv.URL, "--auth-env", "ML")
	require.Equal(t, "invalid_auth_env/RETRY", errCode(t, r))
	require.NotContains(t, r.Stderr, "Authorization: a")
	require.Equal(t, "conflicting_options/RETRY", errCode(t, netRun(t, env, "llm_http", srv.URL, "--auth-env", "API_AUTH", "--insecure")))
}

// TestHTTPAuthNotForwardedAcrossHosts: a redirect to another host drops
// the --auth-env header.
func TestHTTPAuthNotForwardedAcrossHosts(t *testing.T) {
	var got atomic.Value
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Get("X-Api-Key") + "|" + r.Header.Get("Authorization"))
	}))
	defer other.Close()
	first := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, other.URL+"/landing", http.StatusFound)
	}))
	defer first.Close()
	env := map[string]string{"K": "X-Api-Key: kk-secret", "A": "Bearer aa-secret"}
	rows := netRows(t, netRun(t, env, "llm_http", first.URL, "--follow", "--auth-env", "K", "--auth-env", "A"))
	require.EqualValues(t, 200, rows[0]["status"])
	require.Equal(t, "|", got.Load())
}

func TestHTTPSAndRoots(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "secure\n") }))
	defer srv.Close()
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL))
	require.Equal(t, "certificate_error", rows[0]["error"])
	require.Nil(t, rows[0]["status"])
	rows = netRows(t, netRun(t, nil, "llm_http", srv.URL, "--insecure", "--grep", "secure"))
	require.EqualValues(t, 200, rows[0]["status"])
	require.NotNil(t, rows[0]["timing"].(map[string]any)["tls"])
	ca := filepath.Join(physTemp(t), "ca.pem")
	require.NoError(t, os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0o644))
	rows = netRows(t, netRun(t, map[string]string{"SSL_CERT_FILE": ca}, "llm_http", srv.URL))
	require.Nil(t, rows[0]["error"])
	require.Equal(t, true, rows[0]["ok"])
}

// TestHTTPProxyFromCallEnv: the proxy comes from the call's environment
// (HTTP_PROXY), loopback targets and NO_PROXY hosts excepted.
func TestHTTPProxyFromCallEnv(t *testing.T) {
	var seen atomic.Value
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.Store(r.URL.String())
		_, _ = io.WriteString(w, "via proxy\n")
	}))
	defer proxy.Close()
	env := map[string]string{"HTTP_PROXY": proxy.URL}
	rows := netRows(t, netRun(t, env, "llm_http", "http://api.example.test/x", "--grep", "via proxy"))
	require.EqualValues(t, 1, rows[0]["matches"], rows)
	require.Equal(t, "http://api.example.test/x", seen.Load())
	// A loopback target never goes through the proxy.
	srv := httpFake(t)
	rows = netRows(t, netRun(t, env, "llm_http", srv.URL+"/ok", "--grep", "via proxy"))
	require.EqualValues(t, 0, rows[0]["matches"])
	require.EqualValues(t, 200, rows[0]["status"])
}

func TestUseProxy(t *testing.T) {
	for _, c := range []struct {
		host, port, no string
		want           bool
	}{
		{"example.com", "80", "", true},
		{"localhost", "80", "", false},
		{"127.0.0.1", "80", "", false},
		{"::1", "80", "", false},
		{"example.com", "80", "*", false},
		{"api.example.com", "80", "example.com", false},
		{"example.com", "80", "example.com", false},
		{"notexample.com", "80", "example.com", true},
		{"api.example.com", "80", ".example.com", false},
		{"example.com", "80", ".example.com", true},
		{"10.1.2.3", "80", "10.0.0.0/8", false},
		{"11.1.2.3", "80", "10.0.0.0/8", true},
		{"example.com", "443", "example.com:80", true},
		{"example.com", "80", "example.com:80", false},
		{"192.0.2.1", "80", "192.0.2.1", false},
		{"Example.COM.", "80", "x.org, example.com", false},
	} {
		require.Equal(t, c.want, useProxy(c.host, c.port, c.no), "%+v", c)
	}
}

func TestHTTPTimeoutAndErrorsAreFields(t *testing.T) {
	srv := httpFake(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	closed := ln.Addr().String()
	require.NoError(t, ln.Close())
	start := time.Now()
	rows := netRows(t, netRun(t, nil, "llm_http", srv.URL+"/slow", "http://"+closed+"/", srv.URL+"/ok", "--timeout", "0.5", "--status-only"))
	require.Less(t, time.Since(start), 2500*time.Millisecond)
	require.Equal(t, "timeout", rows[0]["error"])
	require.Equal(t, "connection_refused", rows[1]["error"])
	require.Nil(t, rows[1]["status"])
	require.Equal(t, true, rows[2]["ok"])
}

// TestHTTPParallelSameAsSerial: -j 8 prints the rows of -j 1, in input
// order (timing fields aside).
func TestHTTPParallelSameAsSerial(t *testing.T) {
	srv := httpFake(t)
	var urls []string
	for i := 0; i < 12; i++ {
		urls = append(urls, fmt.Sprintf("%s/ok?n=%d", srv.URL, i), srv.URL+"/nope")
	}
	norm := func(r result) string { return msRE.ReplaceAllString(r.Stdout, `"ms":0`) }
	serial := netRun(t, nil, "llm_http", append(urls, "-j", "1", "--status-only")...)
	parallel := netRun(t, nil, "llm_http", append(urls, "-j", "8", "--status-only")...)
	require.Equal(t, norm(serial), norm(parallel))
	require.Len(t, netRows(t, parallel), 24)
}

func TestHTTPArgumentErrors(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "missing_target/RETRY"},
		{[]string{"--follow"}, "missing_target/RETRY"},
		{[]string{"example.com"}, "invalid_target/RETRY"},
		{[]string{"ftp://example.com/"}, "invalid_target/RETRY"},
		{[]string{"http://"}, "invalid_target/RETRY"},
		{[]string{"http://x/", "--bogus"}, "unknown_option/RETRY"},
		{[]string{"http://x/", "--method"}, "missing_value/RETRY"},
		{[]string{"http://x/", "--json-path", "a..b"}, "invalid_json_path/RETRY"},
		{[]string{"http://x/", "--grep", "("}, "invalid_regex/RETRY"},
		{[]string{"http://x/", "--format", "xml"}, "invalid_format/RETRY"},
		{[]string{"http://x/", "-j", "0"}, "invalid_jobs/RETRY"},
		{[]string{"http://x/", "--timeout", "0"}, "invalid_timeout/RETRY"},
		{[]string{"http://x/", "--follow=1"}, "unknown_option/RETRY"},
	} {
		require.Equal(t, c.want, errCode(t, netRun(t, nil, "llm_http", c.args...)), c.args)
	}
	r := netRun(t, nil, "llm_http", "--help")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Stdout, "llm_http URL...")
}

func TestHTTPFormats(t *testing.T) {
	srv := httpFake(t)
	r := netRun(t, nil, "llm_http", srv.URL+"/ok", srv.URL+"/nope", "--status-only", "--format", "tsv")
	lines := strings.Split(strings.TrimSuffix(r.Stdout, "\n"), "\n")
	require.Equal(t, "url\tstatus\tms\tok\terror", lines[0])
	require.Regexp(t, `^http://[^\t]+/ok\t200\t[0-9.]+\ttrue\t\\N$`, lines[1])
	r = netRun(t, nil, "llm_http", srv.URL+"/ok", srv.URL+"/nope", srv.URL+"/ok", "--status-only", "--head", "1")
	require.Len(t, netRows(t, r), 1)
	require.Equal(t, `{"truncated":2}`+"\n", r.Stderr)
}
