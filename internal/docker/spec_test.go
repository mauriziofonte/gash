// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// unit is a call for the helpers: env as the caller's, run as the
// process seam, rt as the registry.
type unit struct {
	*call
	out, err *bytes.Buffer
}

func newUnit(t *testing.T, env []string, run runner.Runner, rt http.RoundTripper) unit {
	t.Helper()
	if run == nil {
		run = runner.NewFake(t)
	}
	m := &Module{Run: run}
	if rt != nil {
		m.HTTP = &http.Client{Transport: rt}
	}
	// The first of duplicate names wins: env before the defaults.
	e := invoke.EnvFrom(append(append([]string{}, env...), "PATH="+t.TempDir()))
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Env: e, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb,
		Term: term.Fixed(e, false, false, false)}
	return unit{call: m.begin(c), out: &out, err: &errb}
}

// mockEnv is a registry that answers as tests/mocks/bin/curl with knobs.
func mockEnv(knobs ...string) *mockRegistry {
	m := map[string]string{}
	for _, k := range knobs {
		n, v, _ := strings.Cut(k, "=")
		m[n] = v
	}
	return &mockRegistry{env: func(n string) string { return m[n] }}
}

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "compose.yml")
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

// docker_compose_spec.sh:11-104 and :713, the parser (YAML subset; the
// config JSON path with a docker fake).
func TestSpecParseServices(t *testing.T) {
	out := lineParser([]byte("version: \"3\"\nservices:\n  web:\n    image: nginx:latest\n    ports:\n      - \"80:80\"\n" +
		"  db:\n    image: postgres:15\n    environment:\n      - POSTGRES_PASSWORD=secret\n  redis:\n    image: redis:7-alpine\n"))
	require.Equal(t, "web|nginx:latest\ndb|postgres:15\nredis|redis:7-alpine\n", out)

	out = lineParser([]byte("services:\n  app:\n    image: \"ghcr.io/org/app:v1.0\"\n  api:\n    image: 'my-registry.io/api:latest'\n"))
	require.Equal(t, "app|ghcr.io/org/app:v1.0\napi|my-registry.io/api:latest\n", out)

	out = lineParser([]byte(strings.Join([]string{"services:", "    web:   # front", "        image: \"nginx:1.25\"   # pinned",
		"        labels:", "            image: not-this", "    \"db\":", "        build: .", "    cache:",
		"        image: redis:latest", "volumes:", "    data:", "        image: nope"}, "\n") + "\n"))
	require.Equal(t, "web|nginx:1.25\ncache|redis:latest\n", out)

	out = lineParser([]byte("services:\n  app:\n    build: .\n    ports:\n      - \"3000:3000\"\n  db:\n    image: postgres:15\n"))
	require.Equal(t, "db|postgres:15\n", out)

	out = lineParser([]byte("services:\n  # Comment before service\n  web:\n    image: nginx:latest # inline comment\n" +
		"    ports:\n      - \"80:80\"\n  db:\n    # Comment inside service\n    image: postgres:15\n  cache:\n" +
		"    image: redis  # No tag\n  custom:\n    image: \"my.registry.io:5000/namespace/app:v1.0.0\"\n"))
	require.Equal(t, "web|nginx:latest\ndb|postgres:15\ncache|redis\ncustom|my.registry.io:5000/namespace/app:v1.0.0\n", out)

	// Compose v2: the services of `config --format json`, never jq.
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), nil, 0o755))
	file := writeFile(t, "services:\n  web:\n    image: ${TAG}\n")
	f := runner.NewFake(t,
		runner.Expect{Argv: []string{"docker", "compose", "version"}},
		runner.Expect{Argv: []string{"docker", "compose", "-f", file, "--project-directory", filepath.Dir(file), "config", "--format", "json"},
			Stdout: `{"services":{"web":{"image":"nginx:1.27"},"app":{"build":{"context":"."}}}}` + "\n"})
	u := newUnit(t, []string{"PATH=" + dir}, f, nil)
	require.Equal(t, "web|nginx:1.27", u.parseServices(file))
	f.Verify()
}

// docker_compose_spec.sh:115-160 and :746.
func TestSpecResolveEnvVars(t *testing.T) {
	u := newUnit(t, nil, nil, nil)
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(env, []byte("TAG=v2.0\nREGISTRY=myregistry.io\n"), 0o644))
	require.Equal(t, "myregistry.io/app:v2.0", u.resolveEnvVars("${REGISTRY}/app:${TAG}", env))
	require.NoError(t, os.WriteFile(env, []byte("\n"), 0o644))
	require.Equal(t, "nginx:latest", u.resolveEnvVars("nginx:${TAG:-latest}", env))
	require.Equal(t, "nginx:1.25", u.resolveEnvVars("${IMAGE:-nginx}:1.25", env))
	require.NoError(t, os.WriteFile(env, []byte("TAG=\"v1.0.0\"\nIMAGE='nginx'\n"), 0o644))
	require.Equal(t, "nginx:v1.0.0", u.resolveEnvVars("${IMAGE}:${TAG}", env))
	require.Equal(t, "nginx:latest", u.resolveEnvVars("nginx:${TAG:-latest}", "/nonexistent/.env"))
}

// docker_compose_spec.sh:167-268 and :1123.
func TestSpecNormalizeAndUpgradeable(t *testing.T) {
	for ref, want := range map[string]string{
		"nginx": "docker.io|library/nginx|latest", "nginx:1.25": "docker.io|library/nginx|1.25",
		"redis:7-alpine": "docker.io|library/redis|7-alpine", "ollama/ollama:latest": "docker.io|ollama/ollama|latest",
		"bitnami/postgresql:15": "docker.io|bitnami/postgresql|15", "ghcr.io/open-webui/open-webui:main": "ghcr.io|open-webui/open-webui|main",
		"nginx@sha256:abc123def456": "docker.io|library/nginx|@sha256:abc123def456", "registry.example.com/myapp:v1": "registry.example.com|myapp|v1",
		"my.private.registry:5000/org/app:latest": "my.private.registry:5000|org/app|latest",
		"localhost:5000/myapp:latest":             "localhost:5000|myapp|latest", "localhost/myapp:v1": "localhost|myapp|v1",
	} {
		i := normalizeImage(ref)
		require.Equal(t, want, i.registry+"|"+i.name+"|"+i.tag, ref)
	}
	for _, tag := range []string{"latest", "main", "dev", "edge", "nightly", "15", "1.25", "8.1", "7-alpine", "1.25-slim"} {
		require.True(t, isUpgradeable(tag), tag)
	}
	for _, tag := range []string{"1.25.3", "v2.0.1", "@sha256:abc123", "1.25.3-alpine"} {
		require.False(t, isUpgradeable(tag), tag)
	}
}

// docker_compose_spec.sh:276-326.
func TestSpecComposeCmd(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker"), nil, 0o755))
	f := runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}, Stdout: "Docker Compose version v2.24.0\n"})
	require.Equal(t, []string{"docker", "compose"}, newUnit(t, []string{"PATH=" + dir}, f, nil).composeCmd())
	f.Verify()

	require.NoError(t, os.WriteFile(filepath.Join(dir, "docker-compose"), nil, 0o755))
	f = runner.NewFake(t, runner.Expect{Argv: []string{"docker", "compose", "version"}, Status: 1})
	require.Equal(t, []string{"docker-compose"}, newUnit(t, []string{"PATH=" + dir}, f, nil).composeCmd())
	f.Verify()

	u := newUnit(t, []string{"PATH=/nonexistent"}, nil, nil)
	require.False(t, u.requireCompose())
	require.Equal(t, "Error: Docker Compose not found. Install docker-compose or Docker Desktop.\n", u.err.String())
}

// docker_compose_spec.sh:785-865: status mapping of the registry call.
func TestSpecRegistryCall(t *testing.T) {
	tokenURL := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/nginx:pull"
	r, e := newUnit(t, nil, nil, mockEnv()).registryCall(http.MethodGet, tokenURL, nil)
	require.Equal(t, "", e)
	require.NotEmpty(t, r.body)
	_, e = newUnit(t, nil, nil, mockEnv("MOCK_CURL_REGISTRY_UNAUTHORIZED=1")).registryCall(http.MethodGet, tokenURL, nil)
	require.Equal(t, "unauthorized", e)
	_, e = newUnit(t, nil, nil, mockEnv("MOCK_CURL_REGISTRY_RATE_LIMITED=1")).registryCall(http.MethodHead,
		"https://registry-1.docker.io/v2/library/nginx/manifests/latest", nil)
	require.Equal(t, "rate_limited", e)
	_, e = newUnit(t, nil, nil, mockEnv("MOCK_CURL_TIMEOUT=1")).registryCall(http.MethodGet, tokenURL, nil)
	require.Equal(t, "timeout", e)
	_, e = newUnit(t, nil, nil, mockEnv("MOCK_CURL_ERROR=1")).registryCall(http.MethodGet, tokenURL, nil)
	require.Equal(t, "server_error", e)
}

// docker_compose_spec.sh:872-1037: tokens and digests.
func TestSpecTokensAndDigests(t *testing.T) {
	hub := "https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/nginx:pull"
	tok, e := newUnit(t, nil, nil, mockEnv()).token(hub)
	require.Equal(t, "mock-dockerhub-token-abc123", tok)
	require.Equal(t, "", e)
	tok, e = newUnit(t, nil, nil, mockEnv("MOCK_CURL_REGISTRY_AUTH_FAIL=1")).token(hub)
	require.Equal(t, "", tok)
	require.Equal(t, "server_error", e)
	tok, _ = newUnit(t, nil, nil, mockEnv()).token("https://ghcr.io/token?service=ghcr.io&scope=repository:open-webui/open-webui:pull")
	require.Equal(t, "mock-ghcr-token-xyz789", tok)

	// GITHUB_TOKEN is the bearer itself: no token request.
	reg := mockEnv()
	d, _ := newUnit(t, []string{"GITHUB_TOKEN=my-custom-gh-token"}, nil, reg).remoteDigest(image{"ghcr.io", "some/image", "latest"})
	require.True(t, strings.HasPrefix(d, "sha256:"))
	require.Equal(t, []string{"HEAD https://ghcr.io/v2/some/image/manifests/latest"}, reg.reqs)

	d, e = newUnit(t, nil, nil, mockEnv()).remoteDigest(image{"docker.io", "library/nginx", "latest"})
	require.Equal(t, "sha256:newdigest456abc123def456abc123def456abc123def456abc123def456newdig", d)
	require.Equal(t, "", e)
	d, _ = newUnit(t, nil, nil, mockEnv()).remoteDigest(image{"ghcr.io", "open-webui/open-webui", "main"})
	require.True(t, strings.HasPrefix(d, "sha256:"))
	for _, knob := range []string{"MOCK_CURL_REGISTRY_AUTH_FAIL=1", "MOCK_CURL_REGISTRY_RATE_LIMITED=1"} {
		d, e = newUnit(t, nil, nil, mockEnv(knob)).remoteDigest(image{"docker.io", "library/nginx", "latest"})
		require.Equal(t, "", d)
		require.NotEqual(t, "", e)
	}
	d, _ = newUnit(t, nil, nil, nil).remoteDigest(image{"docker.io", "library/nginx", "@sha256:abc123def456"})
	require.Equal(t, "sha256:abc123def456", d)

	// The multi-arch index types are asked first, on every registry.
	var accepts []string
	rec := roundTrip(func(req *http.Request) (*http.Response, error) {
		if req.Method == http.MethodHead {
			accepts = append(accepts, req.URL.Host+" "+strings.Join(req.Header.Values("Accept"), ","))
		}
		return mockEnv().RoundTrip(req)
	})
	_, _ = newUnit(t, nil, nil, rec).remoteDigest(image{"docker.io", "library/nginx", "latest"})
	_, _ = newUnit(t, nil, nil, rec).remoteDigest(image{"registry.example.com", "team/app", "latest"})
	require.Equal(t, []string{"registry-1.docker.io " + strings.Join(manifestAccept, ","),
		"registry.example.com " + strings.Join(manifestAccept, ",")}, accepts)
	require.Equal(t, "application/vnd.oci.image.index.v1+json", manifestAccept[0])
	require.Equal(t, "application/vnd.docker.distribution.manifest.list.v2+json", manifestAccept[1])
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func answer(code int, header http.Header, body string) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{StatusCode: code, Header: header, Body: io.NopCloser(strings.NewReader(body))}
}
