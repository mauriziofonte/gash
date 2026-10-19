// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The test binary doubles as a client process: with FAKE_CLIENT=URL it
// fetches URL through the proxy of FAKE_PROXY, trusting FAKE_CA, and
// prints the status and the body.
func TestMain(m *testing.M) {
	if u := os.Getenv("FAKE_CLIENT"); u != "" {
		os.Exit(client(u))
	}
	os.Exit(m.Run())
}

func client(target string) int {
	pool := x509.NewCertPool()
	ca, _ := os.ReadFile(os.Getenv("FAKE_CA"))
	pool.AppendCertsFromPEM(ca)
	proxy, _ := url.Parse(os.Getenv("FAKE_PROXY"))
	c := &http.Client{Transport: &http.Transport{Proxy: http.ProxyURL(proxy), TLSClientConfig: &tls.Config{RootCAs: pool}}}
	resp, err := c.Post(target, "application/json", strings.NewReader(os.Getenv("FAKE_BODY")))
	if err != nil {
		fmt.Println("error:", err)
		return 1
	}
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("%d %s", resp.StatusCode, b)
	return 0
}

// mockCurl runs tests/mocks/bin/curl as a 2.x caller did: the status, the
// body and the header file it wrote.
func mockCurl(t *testing.T, env map[string]string, method, rawURL, body string) (string, string, string) {
	t.Helper()
	mock, err := filepath.Abs("../../../tests/mocks/bin/curl")
	require.NoError(t, err)
	if _, err := os.Stat(mock); err != nil {
		t.Skip("tests/mocks/bin/curl is not in this tree")
	}
	dir := t.TempDir()
	args := []string{"-sS", "-D", dir + "/h", "-w", "%{http_code}", "-o", dir + "/o"}
	if method == "HEAD" {
		args = append(args, "-I")
	}
	if body != "" {
		args = append(args, "-d", body)
	}
	cmd := exec.Command(mock, append(args, rawURL)...)
	cmd.Env = []string{"PATH=/usr/bin:/bin"}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	out, err := cmd.Output()
	require.NoError(t, err)
	o, _ := os.ReadFile(dir + "/o")
	h, _ := os.ReadFile(dir + "/h")
	return strings.TrimSpace(string(out)), string(o), string(h)
}

var mockURLs = []string{
	"https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/nginx:pull",
	"https://ghcr.io/token?scope=repository:o/r:pull",
	"https://registry-1.docker.io/v2/library/nginx/manifests/latest",
	"https://ghcr.io/v2/o/r/manifests/1.0",
	"https://api.anthropic.com/v1/messages",
	"https://generativelanguage.googleapis.com/v1beta/models/gemini:generateContent?key=x",
	"https://example.com/unknown?a=b",
}

var mockBodies = []string{
	"",
	`{"messages":[{"content":"Context: {\"cwd\":\"/\"}\n\nhow to list files"}]}`,
	`{"messages":[{"content":"Context: {}\n\nWhat is bash"}]}`,
	`{"messages":[{"content":"Context: {}\n\nwrite a script"}]}`,
	`{"messages":[{"content":"Context: {}\n\n[PIPE INPUT] boom"}]}`,
	`{"messages":[{"content":"Context: {}\n\nhello"}]}`,
	`{"x":"Analyze this system enumeration"}`,
	`{"x":"Section: a. Initial findings: b"}`,
	`{"x":"initial server analysis ... User question"}`,
}

var mockEnvs = []map[string]string{
	{},
	{"MOCK_CURL_ERROR": "1"},
	{"MOCK_CURL_ERROR": "1", "MOCK_CURL_ERROR_CODE": "503"},
	{"MOCK_CURL_REGISTRY_AUTH_FAIL": "1"},
	{"MOCK_CURL_REGISTRY_UNAUTHORIZED": "1"},
	{"MOCK_CURL_REGISTRY_RATE_LIMITED": "1"},
	{"MOCK_CURL_REGISTRY_NOT_FOUND": "1"},
	{"MOCK_CURL_AI_RESPONSE": `{"content":[{"type":"text","text":"x"}]}`},
}

// The fake's answers are the mock's: status, body, content type; then
// the routed answers.
func TestRespondMatchesMock(t *testing.T) {
	for _, env := range mockEnvs {
		for _, u := range mockURLs {
			for _, method := range []string{"POST", "HEAD"} {
				bodies := mockBodies
				if method == "HEAD" || !strings.Contains(u, "api") {
					bodies = mockBodies[:1]
				}
				for _, body := range bodies {
					code, out, hdr := mockCurl(t, env, method, u, body)
					r := respond(env, request{method: method, url: u, body: body})
					require.Equal(t, code, fmt.Sprint(r.status), "%v %s %s", env, method, u)
					if method == "HEAD" {
						if strings.Contains(u, "/manifests/") && r.header[0][1] != "application/json" {
							// curl -I printed the headers the fake sends.
							require.Contains(t, out, "docker-content-digest: "+headerOf(r, "Docker-Content-Digest"))
						}
						require.Empty(t, r.body)
						continue
					}
					require.Equal(t, out, r.body, "%v %s %s %q", env, method, u, body)
					require.Contains(t, hdr, "content-type: "+headerOf(r, "Content-Type"))
				}
			}
		}
	}
	t.Run("routes", routesMatchMock)
}

func headerOf(r reply, name string) string {
	for _, h := range r.header {
		if h[0] == name {
			return h[1]
		}
	}
	return ""
}

// MOCK_CURL_AI_BUSY_ONCE: the first call per state file is busy, with
// retry-after; the timeouts hang.
func TestRespondKnobs(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	env := map[string]string{"MOCK_CURL_AI_BUSY_ONCE": "529", "MOCK_CURL_STATE": state}
	r := respond(env, request{method: "POST", url: "https://api.anthropic.com/v1/messages"})
	require.Equal(t, 529, r.status)
	require.Equal(t, "1", headerOf(r, "Retry-After"))
	require.FileExists(t, state)
	require.Equal(t, 200, respond(env, request{method: "POST", url: "https://api.anthropic.com/v1/messages"}).status)

	require.True(t, respond(map[string]string{"MOCK_CURL_TIMEOUT": "1"}, request{url: "https://x/"}).hang)
	require.True(t, respond(map[string]string{"MOCK_CURL_REGISTRY_TIMEOUT": "1"}, request{url: "https://r/v2/a/manifests/b"}).hang)
}

// startFake serves a fake on a free port; envOf replaces the fake's own
// environment when not nil.
func startFake(t *testing.T, envOf func(net.Conn) map[string]string) (*fake, string, string) {
	t.Helper()
	f, err := newFake()
	require.NoError(t, err)
	if envOf != nil {
		f.envOf = envOf
	}
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	go func() { _ = f.serve(ln) }()
	ca := filepath.Join(t.TempDir(), "ca.pem")
	require.NoError(t, os.WriteFile(ca, f.caPEM, 0o644))
	return f, "http://" + ln.Addr().String(), ca
}

// An HTTPS request through the proxy: the TLS is the fake's, trusted
// through its CA; the answer is the mock's; the log has the request
// without its credentials.
func TestProxy(t *testing.T) {
	log := filepath.Join(t.TempDir(), "log")
	f, proxy, _ := startFake(t, func(net.Conn) map[string]string { return map[string]string{"MOCK_CURL_LOG": log} })
	pool := x509.NewCertPool()
	require.True(t, pool.AppendCertsFromPEM(f.caPEM))
	pu, _ := url.Parse(proxy)
	c := &http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(pu), TLSClientConfig: &tls.Config{RootCAs: pool}}}

	req, _ := http.NewRequest("POST", "https://api.anthropic.com/v1/messages", strings.NewReader(`{"q":"Context: {}\n\nhow to x"}`))
	req.Header.Set("x-api-key", "sk-secret")
	req.Header.Add("Accept", "application/json")
	resp, err := c.Do(req)
	require.NoError(t, err)
	b, _ := io.ReadAll(resp.Body)
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(b), `\"type\":\"command\"`)

	resp, err = c.Head("https://registry-1.docker.io/v2/library/nginx/manifests/latest")
	require.NoError(t, err)
	require.Equal(t, "sha256:newdigest456abc123def456abc123def456abc123def456abc123def456newdig", resp.Header.Get("Docker-Content-Digest"))

	// Plain HTTP through the proxy.
	resp, err = c.Get("http://example.com/x")
	require.NoError(t, err)
	require.Equal(t, 404, resp.StatusCode)

	l, err := os.ReadFile(log)
	require.NoError(t, err)
	require.Equal(t, "curl -X POST -H \"Accept: application/json\" https://api.anthropic.com/v1/messages\n"+
		"body: {\"q\":\"Context: {}\\n\\nhow to x\"}\n"+
		"curl -X HEAD https://registry-1.docker.io/v2/library/nginx/manifests/latest\n"+
		"curl -X GET http://example.com/x\n", string(l))
	require.NotContains(t, string(l), "sk-secret")
}

// The knobs come from the fake's own environment, whatever the client's.
func TestOwnEnv(t *testing.T) {
	_, proxy, ca := startFake(t, nil)
	self, err := os.Executable()
	require.NoError(t, err)
	run := func() string {
		cmd := exec.Command(self)
		cmd.Env = []string{"FAKE_CLIENT=https://api.anthropic.com/v1/messages", "FAKE_PROXY=" + proxy, "FAKE_CA=" + ca, "MOCK_CURL_ERROR=1"}
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
		return string(out)
	}
	require.True(t, strings.HasPrefix(run(), "200 {\"content\""))
	t.Setenv("MOCK_CURL_ERROR", "1")
	t.Setenv("MOCK_CURL_ERROR_CODE", "503")
	require.Equal(t, "503 {\"error\":{\"message\":\"Simulated error\"}}\n", run())
}
