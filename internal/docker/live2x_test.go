// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

const repoRoot = "../../.."

// absRoot is the checkout, resolved before any test changes directory.
var absRoot = func() string {
	p, err := filepath.Abs(repoRoot)
	if err != nil {
		panic(err)
	}
	return p
}()

// tools are the programs the 2.x code and the mocks run, linked into the
// scene's tool directory: everything but docker, so a scene never
// reaches the host's Docker.
var tools = []string{"bash", "env", "awk", "grep", "sort", "wc", "find", "dirname", "basename", "realpath", "cut",
	"tr", "sed", "cat", "jq", "mktemp", "rm", "head", "od", "chmod", "printf", "ls"}

// scene is a work directory (cwd and $PWD of both runs), a HOME, a stub
// directory first on PATH (the docker, docker-compose and curl mocks of
// tests/mocks/bin, or test stubs) and a tool directory, rebuilt for
// every run at the same paths.
type scene struct {
	t     *testing.T
	base  string
	files map[string]string // work-relative path -> content
	stubs map[string]string // stub name -> script ("" links the mock)
	env   []string
	stdin string
	prep  func(work string)
	// mode
	color bool
	utf8  bool
}

func newScene(t *testing.T) *scene {
	t.Helper()
	if _, err := os.Stat(filepath.Join(absRoot, "lib/modules/docker-compose.sh")); err != nil {
		t.Skip("the 2.x sources are not in this tree")
	}
	for _, n := range tools {
		if _, err := exec.LookPath(n); err != nil {
			t.Skip(n + " not available")
		}
	}
	return &scene{t: t, base: t.TempDir(), files: map[string]string{},
		stubs: map[string]string{"docker": "", "docker-compose": "", "curl": ""}}
}

func (s *scene) work() string  { return filepath.Join(s.base, "work") }
func (s *scene) home() string  { return filepath.Join(s.base, "home") }
func (s *scene) stubD() string { return filepath.Join(s.base, "stub") }
func (s *scene) toolD() string { return filepath.Join(s.base, "tools") }
func (s *scene) log() string   { return filepath.Join(s.base, "docker.log") }

// noDocker removes the docker and docker-compose mocks.
func (s *scene) noDocker() *scene {
	delete(s.stubs, "docker")
	delete(s.stubs, "docker-compose")
	return s
}

// environ is the environment of both runs: the defaults, then s.env, a
// later entry replacing an earlier one.
func (s *scene) environ() []string {
	lc := "C"
	if s.utf8 {
		lc = "C.UTF-8"
	}
	env := []string{"HOME=" + s.home(), "PATH=" + s.stubD() + ":" + s.toolD(), "LC_ALL=" + lc, "TERM=xterm-256color",
		"PWD=" + s.work(), "MOCK_DOCKER_LOG=" + s.log(), "MOCK_CURL_LOG=" + filepath.Join(s.base, "curl.log"),
		"TMPDIR=" + filepath.Join(s.base, "tmp")}
	vals := map[string]string{}
	for _, kv := range append(env, s.env...) {
		k, v, _ := strings.Cut(kv, "=")
		vals[k] = v
	}
	out := make([]string, 0, len(vals))
	for k, v := range vals {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func (s *scene) getenv(name string) string {
	for _, kv := range s.environ() {
		if k, v, _ := strings.Cut(kv, "="); k == name {
			return v
		}
	}
	return ""
}

func (s *scene) build() {
	s.t.Helper()
	if wd, err := os.Getwd(); err != nil || strings.HasPrefix(wd, s.base+"/") {
		require.NoError(s.t, os.Chdir(s.base))
	}
	for _, d := range []string{s.work(), s.home(), s.stubD(), s.toolD(), filepath.Join(s.base, "tmp")} {
		require.NoError(s.t, os.RemoveAll(d))
		require.NoError(s.t, os.MkdirAll(d, 0o755))
	}
	for _, f := range []string{s.log(), filepath.Join(s.base, "curl.log")} {
		require.NoError(s.t, os.RemoveAll(f))
	}
	for _, n := range tools {
		p, err := exec.LookPath(n)
		require.NoError(s.t, err)
		require.NoError(s.t, os.Symlink(p, filepath.Join(s.toolD(), n)))
	}
	for n, script := range s.stubs {
		dst := filepath.Join(s.stubD(), n)
		if script == "" {
			require.NoError(s.t, os.Symlink(filepath.Join(absRoot, "tests/mocks/bin", n), dst))
			continue
		}
		require.NoError(s.t, os.WriteFile(dst, []byte(script), 0o755))
	}
	for rel, content := range s.files {
		p := filepath.Join(s.work(), rel)
		require.NoError(s.t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(s.t, os.WriteFile(p, []byte(content), 0o644))
	}
	if s.prep != nil {
		s.prep(s.work())
	}
}

type outcome struct {
	Out, Err string
	RC       int
	Docker   string   // the docker mock log
	Requests []string // "METHOD URL" of every registry request
}

// run2x runs the 2.x function in the work directory: lib/core and the
// two modules sourced in a Bash without a controlling terminal. On a
// color run stdout is a file that __gash_use_color takes for the
// terminal (a command substitution's pipe is not).
func (s *scene) run2x(fn string, args ...string) outcome {
	s.t.Helper()
	s.build()
	script := `root=$1; shift
for f in output utils validation; do source "$root/lib/core/$f.sh"; done
source "$root/lib/modules/docker.sh"
source "$root/lib/modules/docker-compose.sh"
__gash_use_color() { __gash_color_env_ok || return 1; [[ /dev/stdout -ef "$__TTY_OUT" ]]; }
"$@"`
	outFile := filepath.Join(s.base, "stdout")
	cmd := exec.Command("bash", append([]string{"--noprofile", "--norc", "-c", script, "_", absRoot, fn}, args...)...)
	tty := "/nonexistent"
	if s.color {
		tty = outFile
	}
	cmd.Dir, cmd.Env = s.work(), append(s.environ(), "__TTY_OUT="+tty)
	cmd.Stdin = strings.NewReader(s.stdin)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} // no /dev/tty
	f, err := os.Create(outFile)
	require.NoError(s.t, err)
	var errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = f, &errb
	rc := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		require.True(s.t, errors.As(err, &ee), err)
		rc = ee.ExitCode()
	}
	require.NoError(s.t, f.Close())
	out, _ := os.ReadFile(outFile)
	dlog, _ := os.ReadFile(s.log())
	clog, _ := os.ReadFile(filepath.Join(s.base, "curl.log"))
	return outcome{Out: string(out), Err: noDiag(errb.String()), RC: rc, Docker: string(dlog), Requests: curlRequests(string(clog))}
}

var diagRe = regexp.MustCompile(regexp.QuoteMeta(absRoot) + `/lib/[^:\n]+: line [0-9]+: [^\n]*\n`)

// noDiag drops Bash's own diagnostics, which name a 2.x source line (the
// /dev/tty error of a question without a terminal): the binary does not
// print them (LEDGER DOCK-19).
func noDiag(s string) string { return diagRe.ReplaceAllString(s, "") }

// curlRequests are the requests of a curl mock log: HEAD for -I, the URL
// last.
func curlRequests(log string) []string {
	var out []string
	for _, l := range strings.Split(log, "\n") {
		if !strings.HasPrefix(l, "curl ") {
			continue
		}
		method := "GET"
		if strings.Contains(l, " -I ") {
			method = "HEAD"
		}
		out = append(out, method+" "+l[strings.LastIndexByte(l, ' ')+1:])
	}
	return out
}

// mockRegistry answers as the registry branches of tests/mocks/bin/curl,
// with the scene's MOCK_CURL_* knobs, and records each request.
type mockRegistry struct {
	mu   sync.Mutex
	env  func(string) string
	reqs []string
}

type timeoutErr struct{}

func (timeoutErr) Error() string   { return "mock: operation timed out" }
func (timeoutErr) Timeout() bool   { return true }
func (timeoutErr) Temporary() bool { return true }

var manifestPath = regexp.MustCompile(`/v2/.*/manifests/`)

func (r *mockRegistry) RoundTrip(req *http.Request) (*http.Response, error) {
	u := req.URL.String()
	r.mu.Lock()
	r.reqs = append(r.reqs, req.Method+" "+u)
	r.mu.Unlock()
	head := req.Method == http.MethodHead
	code, body := 200, ""
	h := http.Header{}
	env := r.env
	switch {
	case env("MOCK_CURL_ERROR") != "":
		code = 500
		if n, err := strconv.Atoi(env("MOCK_CURL_ERROR_CODE")); err == nil {
			code = n
		}
		body = `{"error":{"message":"Simulated error"}}`
	case env("MOCK_CURL_TIMEOUT") != "":
		return nil, timeoutErr{}
	case strings.Contains(u, "auth.docker.io/token"):
		switch {
		case env("MOCK_CURL_REGISTRY_AUTH_FAIL") == "1":
			code, body = 500, `{"error":"internal_error"}`
		case env("MOCK_CURL_REGISTRY_UNAUTHORIZED") == "1":
			code, body = 401, `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`
		default:
			body = `{"token":"mock-dockerhub-token-abc123","access_token":"mock-dockerhub-token-abc123"}`
		}
	case strings.Contains(u, "ghcr.io/token"):
		if env("MOCK_CURL_REGISTRY_AUTH_FAIL") == "1" {
			code, body = 500, `{"error":"internal_error"}`
		} else {
			body = `{"token":"mock-ghcr-token-xyz789"}`
		}
	case manifestPath.MatchString(u):
		switch {
		case env("MOCK_CURL_REGISTRY_TIMEOUT") == "1":
			return nil, timeoutErr{}
		case env("MOCK_CURL_REGISTRY_RATE_LIMITED") == "1":
			code, body = 429, `{"errors":[{"code":"TOOMANYREQUESTS","message":"rate limit exceeded"}]}`
		case env("MOCK_CURL_REGISTRY_NOT_FOUND") == "1":
			code, body = 404, `{"errors":[{"code":"MANIFEST_UNKNOWN","message":"manifest unknown"}]}`
		case env("MOCK_CURL_REGISTRY_UNAUTHORIZED") == "1":
			code, body = 401, `{"errors":[{"code":"UNAUTHORIZED","message":"authentication required"}]}`
		case head:
			h.Set("Content-Type", "application/vnd.docker.distribution.manifest.v2+json")
			h.Set("Docker-Content-Digest", "sha256:newdigest456abc123def456abc123def456abc123def456abc123def456newdig")
		default:
			body = `{"schemaVersion":2,"mediaType":"application/vnd.docker.distribution.manifest.v2+json"}`
		}
	default:
		code, body = 404, `{"error":"Unknown endpoint","url":"`+u+`"}`
	}
	if head {
		body = ""
	} else {
		body += "\n"
	}
	return &http.Response{StatusCode: code, Header: h, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

// runGo runs the handler in the work directory with the same scene;
// tm, when not nil, changes the call's terminal.
func (s *scene) runGo(fn string, tm func(*term.Term) *term.Term, args ...string) outcome {
	s.t.Helper()
	s.build()
	s.t.Chdir(s.work())
	reg := &mockRegistry{env: s.getenv}
	m := &Module{Run: runner.Exec{}, HTTP: &http.Client{Transport: reg}}
	tbl := invoke.NewTable()
	m.Register(tbl)
	e := invoke.EnvFrom(append(s.environ(), "GASH__HELP=0"))
	tt := term.Fixed(e, false, s.color, false)
	if tm != nil {
		tt = tm(tt)
	}
	in := filepath.Join(s.base, "stdin")
	require.NoError(s.t, os.WriteFile(in, []byte(s.stdin), 0o644))
	f, err := os.Open(in)
	require.NoError(s.t, err)
	defer func() { _ = f.Close() }()
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: f, Stdout: &out, Stderr: &errb, Term: tt}
	rc := tbl.Run(c)
	dlog, _ := os.ReadFile(s.log())
	// The 2.x mock logs the URL text; net/url escapes what curl would
	// glob (LEDGER DOCK-23, TestLedger_DOCK23): compare unescaped.
	var reqs []string
	for _, r := range reg.reqs {
		if u, err := url.PathUnescape(r); err == nil {
			r = u
		}
		reqs = append(reqs, r)
	}
	return outcome{Out: out.String(), Err: errb.String(), RC: rc, Docker: string(dlog), Requests: reqs}
}

// same runs fn the 2.x way and through the handler on fresh scenes and
// requires the same streams, status, docker calls and registry requests.
func (s *scene) same(fn string, args ...string) outcome {
	s.t.Helper()
	want := s.run2x(fn, args...)
	got := s.runGo(fn, nil, args...)
	require.Equal(s.t, want, got, "%s %q", fn, args)
	if testing.Verbose() {
		s.t.Logf("rc=%d\nstdout:\n%s\nstderr:\n%s\ndocker:\n%s\nrequests: %q", got.RC, got.Out, got.Err, got.Docker, got.Requests)
	}
	return got
}
