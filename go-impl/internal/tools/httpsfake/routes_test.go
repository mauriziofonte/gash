// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// mockPath is tests/mocks/bin/curl; the test skips without it.
func mockPath(t *testing.T) string {
	t.Helper()
	mock, err := filepath.Abs("../../../../tests/mocks/bin/curl")
	require.NoError(t, err)
	if _, err := os.Stat(mock); err != nil {
		t.Skip("tests/mocks/bin/curl is not in this tree")
	}
	return mock
}

// runMock runs the mock with args: stdout, stderr and the exit status.
func runMock(t *testing.T, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(mockPath(t), args...)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	rc := 0
	if err != nil {
		var ee *exec.ExitError
		require.True(t, errors.As(err, &ee), err)
		rc = ee.ExitCode()
	}
	return out.String(), errb.String(), rc
}

// The glob of a route is Bash's [[ STRING == PATTERN ]].
func TestGlobMatchesBash(t *testing.T) {
	pats := []string{
		"*", "https://example.test/f", "*example.test*", "https://*/x", "https://h/?", "*[0-9].json",
		"*[!a-z]", "*[^a-z]", "[]x]*", "[a-]*", "*[[:digit:]]*", "*[[:alpha:]]", `*\*`, `a\?b`, "[abc",
		"*.json", "https://h/*/manifests/*", "**", "[!]]", "?", "",
	}
	strs := []string{
		"https://example.test/f", "https://example.test/g", "https://h/x", "https://h/a/x", "https://h/y",
		"https://h/yz", "v141.0.json", "abc", "ABC", "]x", "-", "a*", "a?b", "axb", "[abc", "x.json",
		"https://h/v2/a/b/manifests/c", "", "]", "é",
	}
	var script strings.Builder
	script.WriteString("set -f\n")
	for _, p := range pats {
		for _, s := range strs {
			script.WriteString("p=" + shq(p) + "; [[ " + shq(s) + " == $p ]] && echo 1 || echo 0\n")
		}
	}
	cmd := exec.Command("bash", "-c", script.String())
	cmd.Env = []string{"LC_ALL=C.UTF-8", "PATH=/usr/bin:/bin"}
	out, err := cmd.Output()
	require.NoError(t, err)
	want := strings.Fields(string(out))
	i := 0
	for _, p := range pats {
		for _, s := range strs {
			require.Equal(t, want[i] == "1", globMatch(p, s), "[[ %q == %q ]]", s, p)
			i++
		}
	}
}

func shq(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func TestGlobPrefix(t *testing.T) {
	for _, c := range []struct {
		pat        string
		could, all bool
	}{
		{"*", true, true},
		{"https://h/*", true, true},
		{"https://h*", true, true},
		{"https://h/x*", true, false},
		{"https://h/", true, false},
		{"*/x", true, false},
		{"https://g/*", false, false},
		{"https://h", false, false},
		{"https://[gh]/**", true, true},
	} {
		could, all := globPrefix(c.pat, "https://h/")
		require.Equal(t, c.could, could, c.pat)
		require.Equal(t, c.all, all, c.pat)
	}
}

const routeFile = `# answers of a test
* https://down.test/* unreachable
GET https://example.test/f 200 hello
HEAD https://example.test/size 200
  Content-Length: 104857600
POST */api 201 {"ok":true}
  Content-Type: application/json
  X-Two:  spaced value
GET https://example.test/zip 200 @zip
GET https://example.test/gone 404 {"error":"gone"}
* https://half.test/x unreachable
GET https://example.test/[0-9]* 200 digits   and  spaces
* * 500
`

// writeRoutes writes the route file and its @zip next to it.
func writeRoutes(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "zip"), bytes.Repeat([]byte{0}, 2048), 0o644))
	path := filepath.Join(dir, "routes")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

// routesMatchMock is the routed half of TestRespondMatchesMock: the
// fake's answers to MOCK_CURL_ROUTES are the mock's (status, body,
// headers, unreachable).
func routesMatchMock(t *testing.T) {
	path := writeRoutes(t, routeFile)
	env := map[string]string{"MOCK_CURL_ROUTES": path}
	for _, req := range []struct{ method, url string }{
		{"GET", "https://example.test/f"}, {"HEAD", "https://example.test/f"},
		{"HEAD", "https://example.test/size"}, {"GET", "https://example.test/size"},
		{"POST", "https://x.test/api"}, {"GET", "https://x.test/api"},
		{"GET", "https://example.test/zip"}, {"HEAD", "https://example.test/zip"},
		{"GET", "https://example.test/gone"}, {"GET", "https://down.test/a"}, {"POST", "https://half.test/x"},
		{"GET", "https://example.test/1"}, {"DELETE", "https://example.test/f"},
	} {
		dir := t.TempDir()
		args := []string{"-sS", "-D", dir + "/h", "-w", "%{http_code}", "-o", dir + "/o"}
		switch req.method {
		case "HEAD":
			args = append(args, "-I")
		case "POST":
			args = append(args, "-d", "{}")
		case "GET":
		default:
			args = append(args, "-X", req.method)
		}
		code, errOut, rc := runMock(t, env, append(args, req.url)...)
		r := respond(env, request{method: req.method, url: req.url})
		if r.drop {
			require.Equal(t, 6, rc, req)
			require.Equal(t, "000", code)
			require.Equal(t, "curl: (6) Could not resolve host: "+strings.Split(req.url, "/")[2]+"\n", errOut)
			continue
		}
		require.Equal(t, 0, rc, "%v %s", req, errOut)
		require.Equal(t, strconv.Itoa(r.status), code, req)
		o, _ := os.ReadFile(dir + "/o")
		h, _ := os.ReadFile(dir + "/h")
		want := "Content-Length: " + strconv.FormatInt(r.length, 10)
		if req.method == "HEAD" {
			require.Empty(t, r.body)
			require.Equal(t, string(h), string(o), "curl -I prints the headers")
		} else {
			require.Equal(t, string(o), r.body, req)
			require.Equal(t, int64(len(o)), r.length)
		}
		hdr := strings.Split(strings.TrimSuffix(string(h), "\r\n\r\n"), "\r\n")
		require.Equal(t, "HTTP/1.1 "+strconv.Itoa(r.status)+" "+http.StatusText(r.status), hdr[0])
		var got []string
		for _, l := range hdr[1:] {
			if strings.HasPrefix(l, "Content-Length: ") {
				require.Equal(t, want, l, req)
				continue
			}
			got = append(got, l)
		}
		var exp []string
		for _, kv := range r.header {
			exp = append(exp, kv[0]+": "+kv[1])
		}
		require.Equal(t, exp, got, req)
	}
	// No route matches: the canned answers, in both.
	r := respond(map[string]string{"MOCK_CURL_ROUTES": writeRoutes(t, "GET https://only.test/ 200 x\n")},
		request{method: "POST", url: "https://example.com/u"})
	require.Equal(t, 404, r.status)
	require.Contains(t, r.body, "Unknown endpoint")
	// A file that is not there has no routes.
	require.Equal(t, 404, respond(map[string]string{"MOCK_CURL_ROUTES": "/nonexistent/routes"}, request{method: "GET", url: "https://x/"}).status)
}

// curl's semantics in the mock: -f, -S, -I, -o, unreachable.
func TestMockCurlRoutes(t *testing.T) {
	path := writeRoutes(t, "GET https://example.test/f 200 hello\nHEAD * 200\n  Content-Length: 5\n* https://down.test* unreachable\n* * 404 {\"nope\":1}\n")
	env := map[string]string{"MOCK_CURL_ROUTES": path}
	dir := t.TempDir()

	out, errOut, rc := runMock(t, env, "-fsIL", "https://example.test/f")
	require.Equal(t, []any{"HTTP/1.1 200 OK\r\nContent-Length: 5\r\n\r\n", "", 0}, []any{out, errOut, rc})
	_, _, rc = runMock(t, env, "-fsSL", "-o", dir+"/f", "https://example.test/f")
	require.Equal(t, 0, rc)
	b, _ := os.ReadFile(dir + "/f")
	require.Equal(t, "hello", string(b))

	// -f: exit 22, no output and no file; the message with -S or without -s.
	out, errOut, rc = runMock(t, env, "-fsSL", "-o", dir+"/g", "https://example.test/g")
	require.Equal(t, []any{"", "curl: (22) The requested URL returned error: 404\n", 22}, []any{out, errOut, rc})
	require.NoFileExists(t, dir+"/g")
	out, errOut, rc = runMock(t, env, "-fsL", "https://example.test/g")
	require.Equal(t, []any{"", "", 22}, []any{out, errOut, rc})
	_, errOut, _ = runMock(t, env, "-f", "https://example.test/g")
	require.Equal(t, "curl: (22) The requested URL returned error: 404\n", errOut)
	// Without -f the body comes out, exit 0.
	out, _, rc = runMock(t, env, "-s", "https://example.test/g")
	require.Equal(t, []any{`{"nope":1}`, 0}, []any{out, rc})

	out, errOut, rc = runMock(t, env, "-sS", "-w", `%{http_code}\n`, "https://down.test:8443/x")
	require.Equal(t, []any{"000\n", "curl: (6) Could not resolve host: down.test\n", 6}, []any{out, errOut, rc})
	_, errOut, rc = runMock(t, env, "-fs", "https://down.test/")
	require.Equal(t, []any{"", 6}, []any{errOut, rc})

	// Bad files stop the mock; the fake answers 500 with the same message.
	for content, msg := range map[string]string{
		"GET x\n":                      "MOCK_CURL_ROUTES line 1: expected METHOD URL-PATTERN STATUS [BODY]",
		"GET * 200\n\nGET * 2000\n":    "MOCK_CURL_ROUTES line 3: STATUS is a three-digit code or unreachable",
		"GET * 200\n  no colon\n":      "MOCK_CURL_ROUTES line 2: a header is Name: value",
		"GET * 200 @missing\n":         "MOCK_CURL_ROUTES line 1: cannot read " + dir + "/missing",
		"# c\n  * x 1\nGET * 200 ok\n": "",
	} {
		p := filepath.Join(dir, "bad")
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
		_, errOut, rc = runMock(t, map[string]string{"MOCK_CURL_ROUTES": p}, "-s", "https://x.test/")
		r := respond(map[string]string{"MOCK_CURL_ROUTES": p}, request{method: "GET", url: "https://x.test/"})
		if msg == "" {
			// An indented line under a comment belongs to no route.
			require.Equal(t, 0, rc, errOut)
			require.Equal(t, 200, r.status)
			continue
		}
		require.Equal(t, 2, rc, content)
		require.Equal(t, "mock curl: "+msg+"\n", errOut)
		require.Equal(t, 500, r.status)
		require.Equal(t, "httpsfake: "+msg+"\n", r.body)
	}
	_, errOut, rc = runMock(t, map[string]string{"MOCK_CURL_ROUTES": "routes"}, "-s", "https://x.test/")
	require.Equal(t, []any{"mock curl: MOCK_CURL_ROUTES must be an absolute path\n", 2}, []any{errOut, rc})
	require.Equal(t, 500, respond(map[string]string{"MOCK_CURL_ROUTES": "routes"}, request{method: "GET", url: "https://x.test/"}).status)
}

// Through the proxy: a routed answer; a HEAD with its Content-Length; an
// unreachable host refuses the tunnel; an unreachable path drops the
// connection.
func TestProxyRoutes(t *testing.T) {
	path := writeRoutes(t, routeFile)
	f, proxy, _ := startFake(t, func(net.Conn) map[string]string { return map[string]string{"MOCK_CURL_ROUTES": path} })
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(f.caPEM))
	pu, _ := url.Parse(proxy)
	var connect []int
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: pool},
		OnProxyConnectResponse: func(_ context.Context, _ *url.URL, _ *http.Request, resp *http.Response) error {
			connect = append(connect, resp.StatusCode)
			return nil
		},
	}}

	resp, err := c.Get("https://example.test/zip")
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	require.Equal(t, 200, resp.StatusCode)
	require.Len(t, b, 2048)

	resp, err = c.Head("https://example.test/size")
	require.NoError(t, err)
	require.Equal(t, int64(104857600), resp.ContentLength)

	resp, err = c.Get("https://example.test/gone")
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)

	// Plain HTTP: the https route does not match, the catch-all does.
	resp, err = c.Get("http://example.test/f")
	require.NoError(t, err)
	require.Equal(t, 500, resp.StatusCode)

	connect = nil
	_, err = c.Get("https://down.test/a")
	require.Error(t, err)
	require.Equal(t, []int{502}, connect)

	// half.test/x is unreachable, but POST */api may answer on that host.
	connect = nil
	_, err = c.Post("https://half.test/x", "text/plain", strings.NewReader("x"))
	require.ErrorIs(t, err, io.EOF)
	require.Equal(t, []int{200}, connect)
}
