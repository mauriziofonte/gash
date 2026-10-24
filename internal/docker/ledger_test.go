// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// bash2x runs script with the 2.x docker modules loaded (no color) and
// returns its stdout.
func bash2x(t *testing.T, env []string, script string) string {
	t.Helper()
	newScene(t) // the skips
	cmd := exec.Command("bash", "--noprofile", "--norc", "-c", `root=$1; shift
for f in output utils validation; do source "$root/lib/core/$f.sh"; done
source "$root/lib/modules/docker.sh"; source "$root/lib/modules/docker-compose.sh"
__gash_use_color() { return 1; }
`+script, "_", absRoot)
	cmd.Env = append([]string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin", "LC_ALL=C"}, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	require.NoError(t, cmd.Run(), errb.String())
	return out.String()
}

func jsonOf(t *testing.T, s string) map[string]any {
	t.Helper()
	var v map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &v), s)
	return v
}

// LEDGER DOCK-02 (forced divergence): paths resolve natively with GNU
// realpath -m semantics, whatever realpath is installed.
func TestLedger_DOCK02(t *testing.T) {
	out, err := exec.Command("realpath", "--version").Output()
	if err != nil || !strings.Contains(string(out), "GNU coreutils") {
		t.Skip("GNU realpath not available")
	}
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "d/e"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(base, "d/e"), filepath.Join(base, "l")))
	require.NoError(t, os.Symlink("d/e/../..", filepath.Join(base, "rel")))
	require.NoError(t, os.Symlink("loop", filepath.Join(base, "loop")))
	t.Chdir(filepath.Join(base, "d"))
	u := newUnit(t, []string{"PWD=/fallback"}, nil, nil)
	for _, v := range []string{".", "e", "e/../x", "missing/a/../b", "../l", "../l/..", "../l/x/y", "../rel/d",
		"//" + base + "//d/./e/", base + "/l/../e", "/", "/..", "../loop", "../loop/x"} {
		// 3.0: DOCK-01 fixed: what realpath -m refuses is refused, never
		// the caller's $PWD.
		want, wantOK := "", false
		if o, err := exec.Command("realpath", "-m", v).Output(); err == nil {
			want, wantOK = strings.TrimSuffix(string(o), "\n"), true
		}
		got, ok := u.resolveDir(v)
		require.Equal(t, wantOK, ok, v)
		require.Equal(t, want, got, v)
	}

	// A realpath without -m first on PATH: 2.x falls back to $PWD, the
	// binary still names the target.
	s := newScene(t)
	s.stubs["realpath"] = "#!/bin/sh\nexit 64\n"
	s.files["compose.yml"] = "services:\n  cwdsvc:\n    image: nginx:latest\n"
	s.files["t/compose.yml"] = "services:\n  targetsvc:\n    image: nginx:latest\n"
	want := s.run2x("docker_compose_check", s.work()+"/t", "--json")
	require.Contains(t, want.Out, `"name":"cwdsvc"`)
	got := s.runGo("docker_compose_check", nil, s.work()+"/t", "--json")
	require.Equal(t, s.work()+"/t", jsonOf(t, got.Out)["path"])
	require.Contains(t, got.Out, `"name":"targetsvc"`)
}

// LEDGER DOCK-08 (forced divergence): each service is one struct and
// every string is escaped, so a '|' in an image injects nothing.
func TestLedger_DOCK08(t *testing.T) {
	s := newScene(t)
	s.files[".env"] = "T=a|L|R|S\",\"evil\":\"1|0|\n"
	s.files["compose.yml"] = "services:\n  svc:\n    image: \"redis:${T}\"\n"
	want := s.run2x("docker_compose_check", "--json")
	require.Contains(t, want.Out, `"evil":"1"`)
	got := s.runGo("docker_compose_check", nil, "--json")
	v := jsonOf(t, got.Out)
	svc := v["services"].([]any)[0].(map[string]any)
	var keys []string
	for k := range svc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"image", "local_digest", "name", "remote_digest", "status", "upgradeable"}, keys)
	require.Equal(t, `redis:a|L|R|S","evil":"1|0|`, svc["image"])
}

// LEDGER DOCK-12 (forced divergence): ${VAR} comes from DIR/.env, then
// the exported environment, then the default, replaced literally.
func TestLedger_DOCK12(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(env, []byte("AMP=a&b\n"), 0o644))
	// SHELLONLY is a shell variable the stub passes but did not export.
	u := newUnit(t, []string{"SHELLONLY=x", "GASH__ADDED=SHELLONLY", "EXPORTED=yes"}, nil, nil)
	require.Equal(t, "redis:a&b", u.resolveEnvVars("redis:${AMP}", env))
	require.Equal(t, "redis:[ab]", u.resolveEnvVars("redis:${TAG:-[ab]}", "/dev/null"))
	require.Equal(t, "nginx:", u.resolveEnvVars("nginx:${compose_file}", "/dev/null"))
	require.Equal(t, "nginx:", u.resolveEnvVars("nginx:${SHELLONLY}", "/dev/null"))
	require.Equal(t, "nginx:yes", u.resolveEnvVars("nginx:${EXPORTED}", "/dev/null"))
	require.Empty(t, u.err.String())
}

// LEDGER DOCK-19 (forced divergence): without a terminal the question
// and the refusal are printed, Bash's /dev/tty diagnostic is not.
func TestLedger_DOCK19(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  w:\n    image: nginx:latest\n"
	s.env = []string{"MOCK_DOCKER_CONTAINERS=1"}
	want := s.run2x("docker_compose_upgrade")
	got := s.runGo("docker_compose_upgrade", nil)
	require.Equal(t, want.Err, got.Err)
	require.Equal(t, 1, got.RC) // 3.0: DOCK-18 fixed (2.x returned 0)
	require.Equal(t, "Proceed with upgrade? [y/N] Info: Upgrade cancelled.\n", got.Err)
	got = s.runGo("docker_prune_all", nil)
	require.NotContains(t, got.Err, "line 77")
	// 3.0: DOCK-21 fixed: no terminal, no question.
	require.Contains(t, got.Err, "Error: No terminal to ask for confirmation: pass -y to go ahead.\n")
}

// LEDGER DOCK-23 (forced divergence): URLs are built with net/url (no
// globbing: one request, the braces escaped), no curl and no ~/.curlrc
// is involved, and the proxies come from the environment.
func TestLedger_DOCK23(t *testing.T) {
	reg := mockEnv()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".curlrc"), []byte("proxy = http://127.0.0.1:9\n"), 0o644))
	// 3.0: such a reference no longer parses (DOCK-06, DOCK-07: check
	// reports invalid_reference); the URL building is proved on the
	// parsed form directly.
	glob := image{registry: "docker.io", host: "registry-1.docker.io", scheme: "https", name: "library/app{a,b}", tag: "latest"}
	require.True(t, normalizeImage("app{a,b}:latest").bad)
	d, e := newUnit(t, []string{"HOME=" + home}, nil, reg).remoteDigest(glob)
	require.Equal(t, "", e)
	require.NotEmpty(t, d)
	require.Equal(t, []string{
		"GET https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/app%7Ba,b%7D:pull",
		"HEAD https://registry-1.docker.io/v2/library/app%7Ba,b%7D/manifests/latest",
	}, reg.reqs)
	// A CR or LF in the token fails in the transport, before sending.
	var sent []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent = append(sent, r.Method+" "+r.URL.Path)
		_, _ = w.Write([]byte(`{"token":"a\nb"}`))
	}))
	defer srv.Close()
	conf := srv.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	conf.ServerName = "example.com"
	x := newUnit(t, nil, nil, nil)
	x.m.HTTP = dialClient(srv.Listener.Addr().String(), conf)
	_, e = x.remoteDigest(normalizeImage("x"))
	require.Equal(t, "transport_error", e)
	require.Equal(t, []string{"GET /token"}, sent)
	// The default client takes its proxies from the environment.
	tr := newUnit(t, nil, nil, nil).client().Transport.(*http.Transport)
	require.Equal(t, reflect.ValueOf(http.ProxyFromEnvironment).Pointer(), reflect.ValueOf(tr.Proxy).Pointer())
}

// dialClient sends every request to addr.
func dialClient(addr string, tlsConf *tls.Config) *http.Client {
	return &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: tlsConf,
	}}
}

// LEDGER DOCK-24 (forced divergence): net/http failures map onto the
// 2.x words; every other transport failure is transport_error.
func TestLedger_DOCK24(t *testing.T) {
	call := func(c *http.Client, ctx context.Context, u string) string {
		x := newUnit(t, nil, nil, nil)
		x.m.HTTP = c
		x.Ctx = ctx
		_, e := x.registryCall(http.MethodHead, u, nil)
		return e
	}
	bg := context.Background()

	// A server that closes the connection.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	require.Equal(t, "transport_error", call(dialClient(ln.Addr().String(), nil), bg, "https://registry.test/v2/"))
	addr := ln.Addr().String()
	require.NoError(t, ln.Close())
	require.Equal(t, "connection_refused", call(dialClient(addr, nil), bg, "https://registry.test/v2/"))

	// An unresolvable host.
	require.Equal(t, "dns_error", call(&http.Client{Transport: &http.Transport{}}, bg, "https://registry.invalid/v2/"))

	// A server that never answers, with a short deadline.
	stall, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = stall.Close() }()
	go func() {
		for {
			c, err := stall.Accept()
			if err != nil {
				return
			}
			defer func() { _ = c.Close() }()
		}
	}()
	ctx, cancel := context.WithTimeout(bg, 200*time.Millisecond)
	defer cancel()
	require.Equal(t, "timeout", call(dialClient(stall.Addr().String(), nil), ctx, "https://registry.test/v2/"))

	// A self-signed certificate.
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	defer srv.Close()
	require.Equal(t, "ssl_error", call(dialClient(srv.Listener.Addr().String(), nil), bg, "https://registry.test/v2/"))

	// Statuses.
	for code, want := range map[int]string{200: "", 204: "", 302: "http_302", 401: "unauthorized", 403: "forbidden",
		404: "not_found", 418: "http_418", 429: "rate_limited", 500: "server_error", 599: "server_error", 600: "http_600"} {
		rt := roundTrip(func(*http.Request) (*http.Response, error) { return answer(code, nil, ""), nil })
		x := newUnit(t, nil, nil, rt)
		_, e := x.registryCall(http.MethodGet, "https://registry.test/", nil)
		require.Equal(t, want, e, code)
	}
	// Invalid URL text.
	require.Equal(t, "transport_error", call(nil, bg, "https://bad host/v2/"))
}

// LEDGER DOCK-25 (forced divergence): the digest is the final answer's,
// not the concatenation of every redirect hop's.
func TestLedger_DOCK25(t *testing.T) {
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host == "example.org" {
			return answer(307, http.Header{"Location": {"https://cdn.example.org/blob"},
				"Docker-Content-Digest": {"sha256:aa"}}, ""), nil
		}
		return answer(200, http.Header{"Docker-Content-Digest": {"sha256:bb"}}, ""), nil
	})
	d, e := newUnit(t, nil, nil, rt).remoteDigest(normalizeImage("example.org/app:v1"))
	require.Equal(t, "", e)
	require.Equal(t, "sha256:bb", d)
}

// LEDGER DOCK-26 (forced divergence): with Compose v2 the config JSON is
// parsed without jq, in its order; with Compose v1 the YAML-subset
// reader runs.
func TestLedger_DOCK26(t *testing.T) {
	file := writeFile(t, "x-img: &img nginx:latest\nservices:\n  b:\n    image: *img\n  a:\n    image: redis:7\n")
	bin := t.TempDir() // docker and docker-compose, no jq
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), nil, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker-compose"), nil, 0o755))
	// A scanned file is named with -f (a project is not: DOCK-04).
	f := runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}},
		runner.Expect{Argv: []string{"docker", "compose", "-f", file, "--project-directory", filepath.Dir(file), "config", "--format", "json"},
			Stdout: `{"services":{"a":{"image":"redis:7"},"b":{"image":"nginx:latest"}}}`})
	require.Equal(t, "a|redis:7\nb|nginx:latest\n", lines3(newUnit(t, []string{"PATH=" + bin}, f, nil).parseServices(file, false).services))
	f.Verify()
	f = runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}, Status: 1})
	require.Equal(t, "b|*img\na|redis:7\n", lines3(newUnit(t, []string{"PATH=" + bin}, f, nil).parseServices(file, false).services))
	f.Verify()
	// Line-parser vectors: block style, quotes, " #", CRLF, tabs.
	require.Equal(t, "w|nginx:1\nx|'q'\ny|redis\n", lines3(lineParser([]byte("services:\r\n  w:\r\n    image: \"nginx:1\"\r\n"+
		"  x:\n    image: \"'q'\" # c\n  y:\n\timage: no\n    image:\tredis\t\n"))))
}

// LEDGER DOCK-27 (forced divergence): scan lists the files in byte
// order, whatever the locale.
func TestLedger_DOCK27(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"a", "B", "a-b", "a_b"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, d), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, d, "compose.yml"), []byte("services: {}\n"), 0o644))
	}
	t.Setenv("LANG", "en_US.UTF-8")
	t.Setenv("LC_ALL", "en_US.UTF-8")
	require.Equal(t, []string{root + "/B/compose.yml", root + "/a-b/compose.yml", root + "/a/compose.yml",
		root + "/a_b/compose.yml"}, findCompose(root, 3))
}

// LEDGER DOCK-30 (rc.A reproduced): the table keeps --min 2:40 --max
// 2:38. On this checkout 2.x pads a cut cell to the column, so the rows
// stay aligned; the bytes are 2.x's either way.
func TestLedger_DOCK30_2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  p:\n    image: redis@sha256:" + strings.Repeat("1", 64) + "\n  w:\n    image: nginx:latest\n"
	r := s.same("docker_compose_check")
	require.Contains(t, r.Out, "p               redis@sha256:111111111111111111111111~   PINNED\n")
	require.Contains(t, r.Out, "w               nginx:latest                             UPDATE\n")
}

// The per-call environment view: the exported variables only.
func TestExportedView(t *testing.T) {
	e := invoke.EnvFrom([]string{"A=1", "B=2", "GASH__ADDED=B", "GASH__X=3"})
	x := (&Module{}).begin(&invoke.Call{Env: e, Term: term.Fixed(e, false, false, false)})
	require.Equal(t, "1", x.exported.Get("A"))
	require.Equal(t, "", x.exported.Get("B"))
	require.Equal(t, "", x.exported.Get("GASH__X"))
}
