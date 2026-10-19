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

// LEDGER DOCK-01 (rc.A reproduced): an empty PATH argument is the
// current directory ($PWD) for check, upgrade and scan.
func TestLedger_DOCK01_2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  cwdsvc:\n    image: nginx:latest\n"
	r := s.same("docker_compose_check", "", "--json")
	require.Equal(t, 0, r.RC)
	require.Equal(t, s.work(), jsonOf(t, r.Out)["path"])
	r = s.same("docker_compose_upgrade", "", "--dry-run")
	require.Contains(t, r.Out, "pull cwdsvc")
	r = s.same("docker_compose_scan", "", "--json")
	require.Equal(t, s.work(), jsonOf(t, r.Out)["base_path"])
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
		want := "/fallback"
		if o, err := exec.Command("realpath", "-m", v).Output(); err == nil {
			want = strings.TrimSuffix(string(o), "\n")
		}
		require.Equal(t, want, u.resolveDir(v), v)
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

// LEDGER DOCK-04 (rc.A reproduced): every compose call names the
// discovered file with -f, so override files are ignored.
func TestLedger_DOCK04_2x(t *testing.T) {
	s := newScene(t)
	s.files["docker-compose.yml"] = "services:\n  web:\n    image: nginx:latest\n"
	s.files["docker-compose.override.yml"] = "services:\n  web:\n    image: nginx:mainline\n"
	r := s.same("docker_compose_upgrade", "--dry-run")
	require.Contains(t, r.Out, "web (nginx:latest)")
	proj := "-f " + s.work() + "/docker-compose.yml --project-directory " + s.work()
	require.Contains(t, r.Out, "  docker compose "+proj+" pull web\n")
	r = s.same("docker_compose_upgrade", "--yes")
	for _, sub := range []string{"config --format json", "pull web", "stop web", "up -d --remove-orphans web", "ps"} {
		require.Contains(t, r.Docker, "docker compose "+proj+" "+sub+"\n", sub)
	}
	require.NotContains(t, r.Docker, "override")
}

// LEDGER DOCK-05 (rc.A reproduced): a file with build-only services
// plans one empty service and runs compose with an empty argument.
func TestLedger_DOCK05_2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  x:\n    build: .\n"
	r := s.same("docker_compose_upgrade", "--dry-run")
	require.Contains(t, r.Out, "  OK:  ()\n")
	require.Contains(t, r.Out, "--project-directory "+s.work()+" pull \n")

	file := writeFile(t, "services:\n  x:\n    build: .\n")
	dir := filepath.Dir(file)
	bin := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "docker"), nil, 0o755))
	dc := []string{"docker", "compose", "-f", file, "--project-directory", dir}
	// One version probe each for requireCompose, the compose command and
	// the service parser.
	version := runner.Expect{Argv: []string{"docker", "compose", "version"}}
	f := runner.NewFake(t, version, version, version,
		runner.Expect{Argv: append(append([]string{}, dc...), "config", "--format", "json"), Status: 1},
		runner.Expect{Argv: append(append([]string{}, dc...), "pull", "")},
		runner.Expect{Argv: append(append([]string{}, dc...), "stop", "")},
		runner.Expect{Argv: append(append([]string{}, dc...), "up", "-d", "--remove-orphans", "")},
		runner.Expect{Argv: []string{"docker", "image", "prune", "-f"}},
		runner.Expect{Argv: append(append([]string{}, dc...), "ps")})
	u := newUnit(t, []string{"PATH=" + bin}, f, nil)
	u.Args = []string{dir, "--yes"}
	require.Equal(t, 0, u.m.upgrade(u.Call))
	f.Verify()
}

// LEDGER DOCK-06 (rc.A reproduced): image:tag@sha256:... keeps its tag,
// is asked to the registry and reported upgradeable.
func TestLedger_DOCK06_2x(t *testing.T) {
	ref := "nginx:1.25@sha256:" + strings.Repeat("a", 64)
	require.Equal(t, "docker.io|library/nginx|1.25\n", bash2x(t, nil, `__gash_normalize_image '`+ref+`'`))
	i := normalizeImage(ref)
	require.Equal(t, image{"docker.io", "library/nginx", "1.25"}, i)
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  w:\n    image: " + ref + "\n"
	r := s.same("docker_compose_check", "--json")
	require.Contains(t, r.Out, `"status":"UPDATE","upgradeable":true`)
	require.Contains(t, r.Requests, "HEAD https://registry-1.docker.io/v2/library/nginx/manifests/1.25")
}

// LEDGER DOCK-07 (rc.A reproduced): docker.io/<name> keeps the bare name
// and index.docker.io is a generic registry.
func TestLedger_DOCK07_2x(t *testing.T) {
	require.Equal(t, "docker.io|alpine|latest\nindex.docker.io|library/alpine|latest\n",
		bash2x(t, nil, `__gash_normalize_image docker.io/alpine; __gash_normalize_image index.docker.io/library/alpine`))
	require.Equal(t, image{"docker.io", "alpine", "latest"}, normalizeImage("docker.io/alpine"))
	require.Equal(t, image{"index.docker.io", "library/alpine", "latest"}, normalizeImage("index.docker.io/library/alpine"))
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  a:\n    image: docker.io/alpine\n  b:\n    image: index.docker.io/library/alpine\n"
	r := s.same("docker_compose_check", "--json")
	require.Equal(t, []string{
		"GET https://auth.docker.io/token?service=registry.docker.io&scope=repository:alpine:pull",
		"HEAD https://registry-1.docker.io/v2/alpine/manifests/latest",
		"HEAD https://index.docker.io/v2/library/alpine/manifests/latest",
	}, r.Requests)
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

// LEDGER DOCK-09 (rc.A reproduced): scan JSON with raw names, [""] and
// the depth text; a depth find refuses gives the empty shape, rc 0.
func TestLedger_DOCK09_2x(t *testing.T) {
	s := newScene(t).noDocker()
	s.files["a/compose.yml"] = "services:\n  'a\"b':\n    image: nginx:latest\n"
	s.files["b/compose.yml"] = "services:\n  x:\n    build: .\n"
	r := s.same("docker_compose_scan", "--depth", "05", "--json")
	require.Contains(t, r.Out, `"depth":05,`)
	require.Contains(t, r.Out, `"services":["a"b"]`)
	require.Contains(t, r.Out, `"services":[""]`)
	r = s.same("docker_compose_scan", "--depth", "abc", "--json")
	require.Equal(t, 0, r.RC)
	require.Equal(t, `{"base_path":"`+s.work()+`","compose_files":[],"total_found":0}`+"\n", r.Out)
}

// LEDGER DOCK-10 (rc.A reproduced): scan ignores unknown options, also
// --help after a path.
func TestLedger_DOCK10_2x(t *testing.T) {
	s := newScene(t).noDocker()
	for _, args := range [][]string{{"--bogus", "--json"}, {".", "--help"}, {"-x", "--json"}} {
		r := s.same("docker_compose_scan", args...)
		require.Equal(t, 0, r.RC)
		require.NotContains(t, r.Out, "USAGE")
	}
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

// LEDGER DOCK-13 (rc.A reproduced): the 2.x interpolation rules.
func TestLedger_DOCK13_2x(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(env, []byte("EMPTY=\nTAG=env-file\n  SPACED=1\nexport EXP=1\nQ=\"a'\nINL=x # c\n"), 0o644))
	vectors := []string{"redis:${EMPTY-dflt}/${TAG}/$TAG", "${SPACED:-no}", "${EXP:-no}", "${Q}", "${INL}", "${A:-}x", "${A:-a:-b}",
		"${A-${B}}", "x${}", "${1A}", "${A:+x}"}
	var script strings.Builder
	for _, v := range vectors {
		script.WriteString(`__gash_resolve_env_vars '` + v + `' '` + env + `'` + "\n")
	}
	want := strings.Split(strings.TrimSuffix(bash2x(t, []string{"TAG=shell"}, "export TAG=shell\n"+script.String()), "\n"), "\n")
	u := newUnit(t, []string{"TAG=shell"}, nil, nil)
	for i, v := range vectors {
		require.Equal(t, want[i], u.resolveEnvVars(v, env), v)
	}
	require.Equal(t, "redis:dflt/env-file/$TAG", want[0])
}

// LEDGER DOCK-14 (rc.A reproduced): an exported GITHUB_TOKEN is the raw
// bearer of the ghcr.io manifest request; no token is asked.
func TestLedger_DOCK14_2x(t *testing.T) {
	var auth []string
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		auth = append(auth, r.Method+" "+r.URL.String()+" "+r.Header.Get("Authorization"))
		return answer(403, nil, ""), nil
	})
	d, e := newUnit(t, []string{"GITHUB_TOKEN=bogus"}, nil, rt).remoteDigest(image{"ghcr.io", "org/app", "latest"})
	require.Equal(t, "", d)
	require.Equal(t, "forbidden", e)
	require.Equal(t, []string{"HEAD https://ghcr.io/v2/org/app/manifests/latest Bearer bogus"}, auth)
	require.Equal(t, "bogus\n", bash2x(t, nil, `GITHUB_TOKEN=bogus; __gash_ghcr_token org/app >/dev/null; echo "$__gash_auth_token"`))
}

// LEDGER DOCK-15 (rc.A reproduced): another registry gets one plain
// HEAD over https, and a bearer challenge is not answered.
func TestLedger_DOCK15_2x(t *testing.T) {
	var reqs []string
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		reqs = append(reqs, r.Method+" "+r.URL.String()+" auth="+r.Header.Get("Authorization"))
		return answer(401, http.Header{"Www-Authenticate": {`Bearer realm="https://public.ecr.aws/token"`}}, ""), nil
	})
	_, e := newUnit(t, nil, nil, rt).remoteDigest(image{"public.ecr.aws", "docker/library/alpine", "latest"})
	require.Equal(t, "unauthorized", e)
	_, _ = newUnit(t, nil, nil, rt).remoteDigest(image{"localhost:5000", "app", "v1"})
	require.Equal(t, []string{"HEAD https://public.ecr.aws/v2/docker/library/alpine/manifests/latest auth=",
		"HEAD https://localhost:5000/v2/app/manifests/v1 auth="}, reqs)
}

// LEDGER DOCK-16 (rc.A reproduced) and DOCK-31 (rc.A reproduced): the
// local digest comes from `docker inspect` without --type image or "--",
// and service names reach compose in option position.
func TestLedger_DOCK16_2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  redis:\n    container_name: redis\n    image: redis\n"
	r := s.same("docker_compose_check", "--json")
	require.Contains(t, r.Docker, "docker inspect --format {{range .RepoDigests}}{{.}}{{\"\\n\"}}{{end}} redis\n")
}

func TestLedger_DOCK31_2x(t *testing.T) {
	s := newScene(t)
	s.files["q/compose.yml"] = "services:\n  --help:\n    image: nginx:latest\n  w:\n    image: -x\n"
	r := s.same("docker_compose_upgrade", "q", "--dry-run")
	require.Contains(t, r.Out, "--project-directory "+s.work()+"/q pull --help w\n")
	r = s.same("docker_compose_check", "q", "--json")
	require.Contains(t, r.Docker, "{{end}} -x\n")
}

// LEDGER DOCK-18 (rc.A reproduced): a declined upgrade returns 0, and
// nothing is pulled.
func TestLedger_DOCK18_2x(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  w:\n    image: nginx:latest\n"
	r := s.same("docker_compose_upgrade")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Err, "Proceed with upgrade? [y/N] Info: Upgrade cancelled.\n")
	got := s.runGo("docker_compose_upgrade", func(t *term.Term) *term.Term { return t.WithTTY("n\n") })
	require.Equal(t, 0, got.RC)
	require.NotContains(t, got.Docker, " pull ")
	got = s.runGo("docker_compose_upgrade", func(t *term.Term) *term.Term { return t.WithTTY("y\n") })
	require.Equal(t, 0, got.RC)
	require.Contains(t, got.Docker, " pull w\n")
}

// LEDGER DOCK-19 (forced divergence): without a terminal the question
// and the refusal are printed, Bash's /dev/tty diagnostic is not.
func TestLedger_DOCK19(t *testing.T) {
	s := newScene(t)
	s.files["compose.yml"] = "services:\n  w:\n    image: nginx:latest\n"
	s.env = []string{"MOCK_DOCKER_CONTAINERS=1"}
	want := s.run2x("docker_compose_upgrade")
	got := s.runGo("docker_compose_upgrade", nil)
	require.Equal(t, want, got)
	require.Equal(t, "Proceed with upgrade? [y/N] Info: Upgrade cancelled.\n", got.Err)
	got = s.runGo("docker_prune_all", nil)
	require.NotContains(t, got.Err, "line 77")
	require.Contains(t, got.Err, "Remove all Docker containers, images, volumes, and networks? [y/N] Info: Aborted.\n")
}

// LEDGER DOCK-20 (rc.A reproduced): a failing daemon reads as no
// containers and zero counts; extra arguments are ignored.
func TestLedger_DOCK20_2x(t *testing.T) {
	s := newScene(t)
	s.stubs["docker"] = failingDocker
	r := s.same("docker_stop_all", "--bogus")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "Info: No running containers to stop.\n", r.Err)
	r = s.same("docker_start_all")
	require.Equal(t, "Info: No stopped containers to start.\n", r.Err)
	r = s.same("docker_prune_all", "--dry-run")
	require.Equal(t, 0, r.RC)
	require.Equal(t, "Info: To remove: 0 container(s), 0 image(s), 0 volume(s), 0 network(s), plus the build cache.\n", r.Err)
}

// LEDGER DOCK-21 (rc.A reproduced): headless without -y prune refuses
// with "Aborted." and never says that -y is needed.
func TestLedger_DOCK21_2x(t *testing.T) {
	s := newScene(t)
	s.env = []string{"GASH_HEADLESS=1"}
	r := s.same("docker_prune_all")
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Info: To remove: 0 container(s), 2 image(s), 1 volume(s), 0 network(s), plus the build cache.\n"+
		"Warning: This cannot be undone: volume data is lost.\nInfo: Aborted.\n", r.Err)
	require.NotContains(t, r.Docker, " rm ")
	require.NotContains(t, r.Docker, "system prune")
}

// LEDGER DOCK-22 (operator decision, rc.A keeps 2.x): stop and start act
// on every container without a question, headless too.
func TestLedger_DOCK22_2x(t *testing.T) {
	s := newScene(t)
	s.env = []string{"GASH_HEADLESS=1", "MOCK_DOCKER_CONTAINERS=1"}
	r := s.same("docker_stop_all")
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Docker, "docker stop abc123\n")
	r = s.same("docker_start_all", "-y")
	require.Contains(t, r.Docker, "docker start stopped123\n")
}

// LEDGER DOCK-23 (forced divergence): URLs are built with net/url (no
// globbing: one request, the braces escaped), no curl and no ~/.curlrc
// is involved, and the proxies come from the environment.
func TestLedger_DOCK23(t *testing.T) {
	reg := mockEnv()
	home := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(home, ".curlrc"), []byte("proxy = http://127.0.0.1:9\n"), 0o644))
	d, e := newUnit(t, []string{"HOME=" + home}, nil, reg).remoteDigest(normalizeImage("app{a,b}:latest"))
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
	_, e = x.remoteDigest(image{"docker.io", "library/x", "latest"})
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
	d, e := newUnit(t, nil, nil, rt).remoteDigest(image{"example.org", "app", "v1"})
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
	f := runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}},
		runner.Expect{Argv: []string{"docker", "compose", "-f", file, "--project-directory", filepath.Dir(file), "config", "--format", "json"},
			Stdout: `{"services":{"a":{"image":"redis:7"},"b":{"image":"nginx:latest"}}}`})
	require.Equal(t, "a|redis:7\nb|nginx:latest", newUnit(t, []string{"PATH=" + bin}, f, nil).parseServices(file))
	f.Verify()
	f = runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}, Status: 1})
	require.Equal(t, "b|*img\na|redis:7", newUnit(t, []string{"PATH=" + bin}, f, nil).parseServices(file))
	f.Verify()
	// Line-parser vectors: block style, quotes, " #", CRLF, tabs.
	require.Equal(t, "w|nginx:1\nx|'q'\ny|redis\n", lineParser([]byte("services:\r\n  w:\r\n    image: \"nginx:1\"\r\n"+
		"  x:\n    image: \"'q'\" # c\n  y:\n\timage: no\n    image:\tredis\t\n")))
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
	require.Equal(t, strings.Join([]string{root + "/B/compose.yml", root + "/a-b/compose.yml", root + "/a/compose.yml",
		root + "/a_b/compose.yml"}, "\n"), findCompose(root, "3"))
}

// LEDGER DOCK-28 (rc.A reproduced): the comment strip runs before the
// unquote.
func TestLedger_DOCK28_2x(t *testing.T) {
	require.Equal(t, "w|\"nginx:latest\n", lineParser([]byte("services:\n  w:\n    image: \"nginx:latest #x\"\n")))
	require.Equal(t, "w|nginx:latest\n", lineParser([]byte("services:\n  w:\n    image: nginx:latest #x\n")))
	f := writeFile(t, "services:\n  w:\n    image: \"nginx:latest #x\"\n")
	require.Equal(t, "w|\"nginx:latest\n", bash2x(t, nil, `__gash_compose_cmd() { return 1; }; __gash_parse_compose_services '`+f+`'`))
}

// LEDGER DOCK-29 (rc.A reproduced): docker-compose.yml wins over
// compose.yaml.
func TestLedger_DOCK29_2x(t *testing.T) {
	s := newScene(t)
	s.files["docker-compose.yml"] = "services:\n  old:\n    image: nginx:latest\n"
	s.files["compose.yaml"] = "services:\n  new:\n    image: nginx:latest\n"
	r := s.same("docker_compose_upgrade", "--dry-run")
	require.Contains(t, r.Out, "-f "+s.work()+"/docker-compose.yml --project-directory "+s.work()+" pull old\n")
	r = s.same("docker_compose_check", "--json")
	require.Equal(t, "docker-compose.yml", jsonOf(t, r.Out)["compose_file"])
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
