// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// The proving tests of the 3.0 DOCK fixes. They need no 2.x sources:
// the handler runs with a runner fake (fakeDocker) or the testdata
// mocks (newGoScene).

// dockerPath is a PATH directory with docker and docker-compose files
// (the fake runner never starts them).
func dockerPath(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for _, n := range []string{"docker", "docker-compose"} {
		require.NoError(t, os.WriteFile(filepath.Join(bin, n), nil, 0o755))
	}
	return bin
}

// fakeDocker answers every docker start: Compose v2 present, `config`
// prints config (fails when it is ""), `image inspect` prints the digest
// line of its reference from digests, anything else succeeds silently.
func fakeDocker(t *testing.T, config string, digests map[string]string) *runner.Fake {
	f := runner.NewFake(t)
	f.Respond(func(c runner.Call) (runner.Expect, bool) {
		a := c.Argv
		switch {
		case slices.Contains(a, "config"):
			if config == "" {
				return runner.Expect{Status: 1}, true
			}
			return runner.Expect{Stdout: config}, true
		case len(a) > 2 && a[1] == "image" && a[2] == "inspect":
			return runner.Expect{Stdout: digests[a[len(a)-1]]}, true
		}
		return runner.Expect{}, true
	})
	return f
}

// runUnit runs fn through the table with run as the process seam and rt
// as the registry, env first in the environment and docker on PATH.
func runUnit(t *testing.T, run runner.Runner, rt http.RoundTripper, env []string, tm func(*term.Term) *term.Term,
	fn string, args ...string) (string, string, int) {
	t.Helper()
	m := &Module{Run: run}
	if rt != nil {
		m.HTTP = &http.Client{Transport: rt}
	}
	tbl := invoke.NewTable()
	m.Register(tbl)
	e := invoke.EnvFrom(append(append([]string{}, env...), "PATH="+dockerPath(t), "GASH__HELP=0"))
	tt := term.Fixed(e, false, false, false)
	if tm != nil {
		tt = tm(tt)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Ctx: context.Background(), Name: fn, Args: args, Env: e, Stdin: strings.NewReader(""),
		Stdout: &out, Stderr: &errb, Term: tt}
	rc := tbl.Run(c)
	return out.String(), errb.String(), rc
}

// project writes the files of a compose project and returns its
// directory (physical path).
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	for rel, content := range files {
		p := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	}
	return dir
}

// composeCalls are the argv of the compose starts of f, with their dir.
func composeCalls(f *runner.Fake) []runner.Call {
	var out []runner.Call
	for _, c := range f.Calls() {
		if len(c.Argv) > 1 && c.Argv[1] == "compose" && !slices.Contains(c.Argv, "version") {
			out = append(out, c)
		}
	}
	return out
}

func argvs(calls []runner.Call) []string {
	var out []string
	for _, c := range calls {
		out = append(out, strings.Join(c.Argv, " "))
	}
	return out
}

// LEDGER DOCK-01 (3.0 fix): an empty PATH argument is refused, never the
// current directory, before any docker call.
func TestLedger_DOCK01(t *testing.T) {
	dir := project(t, map[string]string{"compose.yml": "services:\n  cwdsvc:\n    image: nginx:latest\n"})
	t.Chdir(dir)
	for _, tc := range []struct {
		fn   string
		args []string
		err  string
	}{
		{"docker_compose_check", []string{"", "--json"},
			`{"error":"path_not_found","action":"RETRY","recoverable":true,"hint":"Pass the directory that holds the compose file (omit it for the current directory)"}` + "\n"},
		{"docker_compose_check", []string{""}, "Error: Cannot resolve path: ''\n"},
		{"docker_compose_upgrade", []string{"", "--yes"}, "Error: Cannot resolve path: ''\n"},
		{"docker_compose_scan", []string{"", "--json"},
			`{"error":"path_not_found","action":"RETRY","recoverable":true,"hint":"Pass the directory that holds the compose file (omit it for the current directory)"}` + "\n"},
	} {
		f := runner.NewFake(t)
		out, errOut, rc := runUnit(t, f, nil, nil, nil, tc.fn, tc.args...)
		require.Equal(t, 1, rc, tc.fn)
		require.Empty(t, out, tc.fn)
		require.Equal(t, tc.err, errOut, tc.fn)
		require.Empty(t, f.Calls(), tc.fn)
	}
	// Omitting the argument still means the current directory.
	out, _, rc := runUnit(t, fakeDocker(t, "", nil), mockEnv(), nil, nil, "docker_compose_check", "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, dir, jsonOf(t, out)["path"])
}

// LEDGER DOCK-04 (3.0 fix): no -f in any compose argv, every compose
// call in the project directory, so the override files apply.
func TestLedger_DOCK04(t *testing.T) {
	dir := project(t, map[string]string{
		"docker-compose.yml":          "services:\n  web:\n    image: nginx:latest\n",
		"docker-compose.override.yml": "services:\n  web:\n    image: nginx:mainline\n",
	})
	merged := `{"services":{"web":{"image":"nginx:mainline","ports":[{"target":80}]}}}`
	f := fakeDocker(t, merged, nil)
	out, _, rc := runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--dry-run")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "web (nginx:mainline)")
	require.Contains(t, out, "  docker compose --project-directory "+dir+" pull -- web\n")
	require.NotContains(t, out, "-f ")

	f = fakeDocker(t, merged, nil)
	_, _, rc = runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--yes")
	require.Equal(t, 0, rc)
	calls := composeCalls(f)
	require.Equal(t, []string{
		"docker compose --project-directory " + dir + " config --format json",
		"docker compose --project-directory " + dir + " pull -- web",
		"docker compose --project-directory " + dir + " stop -- web",
		"docker compose --project-directory " + dir + " up -d --remove-orphans -- web",
		"docker compose --project-directory " + dir + " ps",
	}, argvs(calls))
	for _, c := range calls {
		require.Equal(t, dir, c.Dir, c.Argv)
		require.NotContains(t, c.Argv, "-f")
	}

	f = fakeDocker(t, merged, nil)
	_, _, rc = runUnit(t, f, mockEnv(), nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	for _, c := range composeCalls(f) {
		require.Equal(t, dir, c.Dir)
		require.NotContains(t, c.Argv, "-f")
	}
}

// LEDGER DOCK-05 (3.0 fix): a file without image services is refused
// before the plan and before any compose call.
func TestLedger_DOCK05(t *testing.T) {
	dir := project(t, map[string]string{"compose.yml": "services:\n  x:\n    build: .\n"})
	f := fakeDocker(t, `{"services":{"x":{"build":{"context":"."}}}}`, nil)
	out, errOut, rc := runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--yes")
	require.Equal(t, 1, rc)
	require.Empty(t, out)
	require.Equal(t, "Error: No services with images found in "+dir+"/compose.yml\n", errOut)
	require.Equal(t, []string{"docker compose --project-directory " + dir + " config --format json"}, argvs(composeCalls(f)))
	// The line parser (no Compose v2) too.
	f = fakeDocker(t, "", nil)
	_, errOut, rc = runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--dry-run")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: No services with images found in "+dir+"/compose.yml\n", errOut)
}

// counter is a registry that counts the requests it gets.
type counter struct {
	mu   sync.Mutex
	n    int
	next http.RoundTripper
}

func (c *counter) RoundTrip(r *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.n++
	c.mu.Unlock()
	return c.next.RoundTrip(r)
}

// LEDGER DOCK-06 (3.0 fix): image:tag@sha256:... is pinned to its digest
// and the registry is never asked.
func TestLedger_DOCK06(t *testing.T) {
	ref := "nginx:1.25@sha256:" + strings.Repeat("a", 64)
	i := normalizeImage(ref)
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), i.digest)
	require.Equal(t, "docker.io", i.registry)
	require.Equal(t, "library/nginx", i.name)
	dir := project(t, map[string]string{"compose.yml": "services:\n  w:\n    image: " + ref + "\n"})
	reg := &counter{next: mockEnv()}
	out, _, rc := runUnit(t, fakeDocker(t, "", nil), reg, nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	svc := jsonOf(t, out)["services"].([]any)[0].(map[string]any)
	require.Equal(t, "PINNED", svc["status"])
	require.Equal(t, false, svc["upgradeable"])
	require.Equal(t, "sha256:"+strings.Repeat("a", 64), svc["local_digest"])
	require.Equal(t, svc["local_digest"], svc["remote_digest"])
	require.Equal(t, 0, reg.n)
	// dcup2 lists it as pinned.
	out, _, rc = runUnit(t, fakeDocker(t, "", nil), reg, nil, nil, "docker_compose_upgrade", dir, "--dry-run")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "pinned version")
	require.Contains(t, out, "No services to upgrade (all pinned).")
}

// mockDigest is the digest the mock registry answers (not hex, so never
// a local digest); hexDigest a valid one.
const (
	mockDigest = "sha256:newdigest456abc123def456abc123def456abc123def456abc123def456newdig"
	hexDigest  = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// hexRegistry is the mock registry answering hexDigest.
func hexRegistry() roundTrip {
	return func(r *http.Request) (*http.Response, error) {
		resp, err := mockEnv().RoundTrip(r)
		if err == nil && resp.Header.Get("Docker-Content-Digest") != "" {
			resp.Header.Set("Docker-Content-Digest", hexDigest)
		}
		return resp, err
	}
}

// LEDGER DOCK-07 (3.0 fix): docker.io, index.docker.io and
// registry-1.docker.io are Docker Hub, and single names get library/.
func TestLedger_DOCK07(t *testing.T) {
	for _, ref := range []string{"alpine", "docker.io/alpine", "docker.io/library/alpine", "index.docker.io/library/alpine",
		"index.docker.io/alpine", "registry-1.docker.io/library/alpine"} {
		i := normalizeImage(ref)
		require.Equal(t, "docker.io", i.registry, ref)
		require.Equal(t, "library/alpine", i.name, ref)
		require.Equal(t, "latest", i.tag, ref)
		reg := mockEnv()
		d, e := newUnit(t, nil, nil, reg).remoteDigest(i)
		require.Equal(t, "", e, ref)
		require.Equal(t, mockDigest, d, ref)
		require.Equal(t, []string{
			"GET https://auth.docker.io/token?service=registry.docker.io&scope=repository:library/alpine:pull",
			"HEAD https://registry-1.docker.io/v2/library/alpine/manifests/latest",
		}, reg.reqs, ref)
	}
	dir := project(t, map[string]string{"compose.yml": "services:\n  a:\n    image: docker.io/alpine\n  b:\n    image: index.docker.io/library/alpine\n"})
	digests := map[string]string{"docker.io/alpine": "alpine@" + hexDigest + "\n", "index.docker.io/library/alpine": "alpine@" + hexDigest + "\n"}
	out, _, rc := runUnit(t, fakeDocker(t, "", digests), hexRegistry(), nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	for _, s := range jsonOf(t, out)["services"].([]any) {
		require.Equal(t, "CURRENT", s.(map[string]any)["status"], s)
	}
}

// scanJSON is the docker_compose_scan --json shape (the schema of the
// fixed output): every key present, of its type, nothing else.
type scanJSON struct {
	BasePath     *string `json:"base_path"`
	Depth        *int    `json:"depth"`
	ComposeFiles []struct {
		Path     *string  `json:"path"`
		File     *string  `json:"file"`
		Services []string `json:"services"`
	} `json:"compose_files"`
	TotalFound *int `json:"total_found"`
}

func validScan(t *testing.T, s string) scanJSON {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(s))
	dec.DisallowUnknownFields()
	var v scanJSON
	require.NoError(t, dec.Decode(&v), s)
	require.NotNil(t, v.BasePath, s)
	require.NotNil(t, v.Depth, s)
	require.NotNil(t, v.ComposeFiles, s)
	require.NotNil(t, v.TotalFound, s)
	require.Equal(t, len(v.ComposeFiles), *v.TotalFound, s)
	for _, f := range v.ComposeFiles {
		require.NotNil(t, f.Path, s)
		require.NotNil(t, f.File, s)
		require.NotNil(t, f.Services, s)
	}
	return v
}

// LEDGER DOCK-09 (3.0 fix): the scan JSON is valid and complete: names
// encoded, [] for a file without image services, the depth an integer
// (also in the empty shape); an invalid depth is refused.
func TestLedger_DOCK09(t *testing.T) {
	dir := project(t, map[string]string{
		"a/compose.yml": "services:\n  'a\"b':\n    image: nginx:latest\n",
		"b/compose.yml": "services:\n  x:\n    build: .\n",
	})
	f := runner.NewFake(t) // no Compose: the line parser
	out, _, rc := runUnit(t, f, nil, []string{"PATH=/nonexistent"}, nil, "docker_compose_scan", dir, "--depth", "05", "--json")
	require.Equal(t, 0, rc)
	v := validScan(t, out)
	require.Equal(t, 5, *v.Depth)
	require.Contains(t, out, `"depth":5,`)
	require.Equal(t, []string{`a"b`}, v.ComposeFiles[0].Services)
	require.Equal(t, []string{}, v.ComposeFiles[1].Services)

	empty := project(t, nil)
	out, _, rc = runUnit(t, f, nil, []string{"PATH=/nonexistent"}, nil, "docker_compose_scan", empty, "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, `{"base_path":"`+empty+`","depth":3,"compose_files":[],"total_found":0}`+"\n", out)
	validScan(t, out)

	for _, bad := range []string{"abc", "-1", "1.5", ""} {
		out, errOut, rc := runUnit(t, f, nil, nil, nil, "docker_compose_scan", dir, "--depth", bad, "--json")
		require.Equal(t, 1, rc, bad)
		require.Empty(t, out, bad)
		details := `"details":"` + bad + `",`
		if bad == "" {
			details = "" // an empty details is left out of the envelope
		}
		require.Equal(t, `{"error":"invalid_depth",`+details+`"action":"RETRY","recoverable":true,"hint":"Use --depth N with N a non-negative integer"}`+"\n", errOut, bad)
	}
	_, errOut, rc := runUnit(t, f, nil, nil, nil, "docker_compose_scan", dir, "--depth", "abc")
	require.Equal(t, 1, rc)
	require.Equal(t, "Error: Invalid --depth: abc (use a non-negative integer)\n", errOut)
}

// LEDGER DOCK-10 (3.0 fix): scan refuses unknown options, and --help is
// help only as the first word.
func TestLedger_DOCK10(t *testing.T) {
	dir := project(t, nil)
	t.Chdir(dir)
	for _, tc := range []struct {
		args []string
		err  string
	}{
		{[]string{"--bogus"}, "Error: Unknown option: --bogus\n"},
		{[]string{".", "--help"}, "Error: Unknown option: --help\n"},
		{[]string{"-x", "--json"}, `{"error":"unknown_option","details":"-x","action":"RETRY","recoverable":true,"hint":"Use a directory, --depth N, --json or --no-color"}` + "\n"},
		{[]string{"--bogus", "--json"}, `{"error":"unknown_option","details":"--bogus","action":"RETRY","recoverable":true,"hint":"Use a directory, --depth N, --json or --no-color"}` + "\n"},
	} {
		out, errOut, rc := runUnit(t, runner.NewFake(t), nil, nil, nil, "docker_compose_scan", tc.args...)
		require.Equal(t, 1, rc, tc.args)
		require.Empty(t, out, tc.args)
		require.Equal(t, tc.err, errOut, tc.args)
	}
	out, _, rc := runUnit(t, runner.NewFake(t), nil, nil, nil, "docker_compose_scan", "--help")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "USAGE")
}

// LEDGER DOCK-13 (3.0 fix): interpolation with Compose's rules: the
// operators, the environment over DIR/.env, $VAR, Compose's dotenv.
func TestLedger_DOCK13(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, ".env")
	require.NoError(t, os.WriteFile(env, []byte("EMPTY=\nTAG=env-file\n  SPACED=1\nexport EXP=1\nINL=x # c\n"+
		"SQ='a $TAG'\nDQ=\"v\\tx ${INL}\"\nREF=${INL}y\nML=\"l1\nl2\"\nAFTER=ok\nQ=\"a'\n"), 0o644))
	vectors := []struct{ in, want string }{
		{"redis:${EMPTY-dflt}/${TAG}/$TAG", "redis:/shell/shell"},
		{"${EMPTY:-dflt}", "dflt"},
		{"${SPACED:-no}", "1"},
		{"${EXP:-no}", "1"},
		{"${Q}", ""}, // an unterminated quote at the end: no definition
		{"${INL}", "x"},
		{"${A:-}x", "x"},
		{"${A:-a:-b}", "a:-b"},
		{"${A-${INL}}", "x"},
		{"${A:+x}", ""},
		{"${INL:+set}", "set"},
		{"${EMPTY+set}", "set"},
		{"${EMPTY:+set}", ""},
		{"$$INL", "$INL"},
		{"${SQ}", "a $TAG"},
		{"${DQ}", "v\tx x"},
		{"${REF}", "xy"},
		{"${ML}", "l1\nl2"},
		{"${AFTER}", "ok"},
		{"${TAG:?needed}", "shell"},
		{"a$", "a$"},
	}
	u := newUnit(t, []string{"TAG=shell"}, nil, nil)
	for _, v := range vectors {
		require.Equal(t, v.want, u.resolveEnvVars(v.in, env), v.in)
	}
	require.Empty(t, u.err.String())
	// A malformed or failing template is left as it is, with a warning.
	for _, in := range []string{"x${}", "${1A}", "${NOPE:?needed}", "${UNCLOSED"} {
		u := newUnit(t, nil, nil, nil)
		require.Equal(t, in, u.resolveEnvVars(in, env), in)
		require.Contains(t, u.err.String(), "Warning: Cannot interpolate "+in, in)
	}

	// The same vectors through Compose itself, when Compose v2 is here.
	if out, err := exec.Command("docker", "compose", "version").Output(); err != nil || !bytes.Contains(out, []byte("v2")) {
		t.Log("Compose v2 not available: the vectors are not compared with docker compose config")
		return
	}
	var yml strings.Builder
	yml.WriteString("services:\n")
	for i, v := range vectors {
		if strings.Contains(v.want, "\n") {
			continue
		}
		yml.WriteString("  s" + string(rune('a'+i)) + ":\n    image: '" + strings.ReplaceAll(v.in, "'", "''") + "'\n")
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(yml.String()), 0o644))
	cmd := exec.Command("docker", "compose", "--project-directory", dir, "config", "--format", "json")
	cmd.Dir, cmd.Env = dir, append(os.Environ(), "TAG=shell")
	out, err := cmd.Output()
	require.NoError(t, err)
	svcs, ok := configServices(out)
	require.True(t, ok)
	for _, s := range svcs {
		i := int(s.name[1] - 'a')
		require.Equal(t, vectors[i].want, s.image, vectors[i].in)
	}
}

// LEDGER DOCK-14 (3.0 fix): GITHUB_TOKEN is exchanged at ghcr.io/token,
// never sent as it is; a refused token falls back to anonymous access.
func TestLedger_DOCK14(t *testing.T) {
	ghcr := func(reqs *[]string) roundTrip {
		return func(r *http.Request) (*http.Response, error) {
			user, pass, basic := r.BasicAuth()
			*reqs = append(*reqs, r.Method+" "+r.URL.String()+" "+r.Header.Get("Authorization"))
			switch {
			case r.URL.Path == "/token" && basic && user == "gash" && pass == "real-pat":
				return answer(200, nil, `{"token":"exchanged"}`), nil
			case r.URL.Path == "/token" && basic:
				return answer(401, nil, ""), nil
			case r.URL.Path == "/token":
				return answer(200, nil, `{"token":"anon"}`), nil
			}
			switch r.Header.Get("Authorization") {
			case "Bearer exchanged", "Bearer anon":
				return answer(200, http.Header{"Docker-Content-Digest": {hexDigest}}, ""), nil
			}
			return answer(403, nil, ""), nil // a raw PAT is refused
		}
	}
	tokenURL := "https://ghcr.io/token?service=ghcr.io&scope=repository:org/app:pull"
	manifest := "https://ghcr.io/v2/org/app/manifests/latest"

	var reqs []string
	d, e := newUnit(t, []string{"GITHUB_TOKEN=real-pat"}, nil, ghcr(&reqs)).remoteDigest(normalizeImage("ghcr.io/org/app"))
	require.Equal(t, "", e)
	require.Equal(t, hexDigest, d)
	require.Equal(t, []string{"GET " + tokenURL + " Basic Z2FzaDpyZWFsLXBhdA==", "HEAD " + manifest + " Bearer exchanged"}, reqs)
	for _, r := range reqs {
		require.NotContains(t, r, "Bearer real-pat")
	}

	reqs = nil
	d, e = newUnit(t, []string{"GITHUB_TOKEN=bogus"}, nil, ghcr(&reqs)).remoteDigest(normalizeImage("ghcr.io/org/app"))
	require.Equal(t, "", e)
	require.Equal(t, hexDigest, d)
	require.Equal(t, []string{"GET " + tokenURL + " Basic Z2FzaDpib2d1cw==", "GET " + tokenURL + " ", "HEAD " + manifest + " Bearer anon"}, reqs)

	reqs = nil
	_, e = newUnit(t, nil, nil, ghcr(&reqs)).remoteDigest(normalizeImage("ghcr.io/org/app"))
	require.Equal(t, "", e)
	require.Equal(t, []string{"GET " + tokenURL + " ", "HEAD " + manifest + " Bearer anon"}, reqs)

	// The whole check: CURRENT.
	dir := project(t, map[string]string{"compose.yml": "services:\n  a:\n    image: ghcr.io/org/app\n"})
	reqs = nil
	out, _, rc := runUnit(t, fakeDocker(t, "", map[string]string{"ghcr.io/org/app": "ghcr.io/org/app@" + hexDigest + "\n"}),
		ghcr(&reqs), []string{"GITHUB_TOKEN=real-pat"}, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, "CURRENT", jsonOf(t, out)["services"].([]any)[0].(map[string]any)["status"])
}

// LEDGER DOCK-15 (3.0 fix): another registry's Bearer challenge is
// answered with an anonymous token; localhost registries are plain HTTP.
func TestLedger_DOCK15(t *testing.T) {
	var reqs []string
	rt := roundTrip(func(r *http.Request) (*http.Response, error) {
		reqs = append(reqs, r.Method+" "+r.URL.String()+" auth="+r.Header.Get("Authorization"))
		switch {
		case r.URL.Host == "public.ecr.aws" && r.URL.Path == "/token":
			return answer(200, nil, `{"token":"ecr-anon"}`), nil
		case r.Header.Get("Authorization") == "Bearer ecr-anon":
			return answer(200, http.Header{"Docker-Content-Digest": {hexDigest}}, ""), nil
		case r.URL.Host == "public.ecr.aws":
			return answer(401, http.Header{"Www-Authenticate": {`Bearer realm="https://public.ecr.aws/token",service="public.ecr.aws",scope="aws"`}}, ""), nil
		}
		return answer(200, http.Header{"Docker-Content-Digest": {hexDigest}}, ""), nil
	})
	d, e := newUnit(t, nil, nil, rt).remoteDigest(normalizeImage("public.ecr.aws/docker/library/alpine:latest"))
	require.Equal(t, "", e)
	require.Equal(t, hexDigest, d)
	_, _ = newUnit(t, nil, nil, rt).remoteDigest(normalizeImage("localhost:5000/app:v1"))
	require.Equal(t, []string{
		"HEAD https://public.ecr.aws/v2/docker/library/alpine/manifests/latest auth=",
		"GET https://public.ecr.aws/token?scope=aws&service=public.ecr.aws auth=",
		"HEAD https://public.ecr.aws/v2/docker/library/alpine/manifests/latest auth=Bearer ecr-anon",
		"HEAD http://localhost:5000/v2/app/manifests/v1 auth=",
	}, reqs)

	dir := project(t, map[string]string{"compose.yml": "services:\n  a:\n    image: public.ecr.aws/docker/library/alpine:latest\n"})
	out, _, rc := runUnit(t, fakeDocker(t, "", map[string]string{"public.ecr.aws/docker/library/alpine:latest": "public.ecr.aws/docker/library/alpine@" + hexDigest + "\n"}),
		rt, nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, "CURRENT", jsonOf(t, out)["services"].([]any)[0].(map[string]any)["status"])
}

// LEDGER DOCK-16 (3.0 fix): the local digest comes from `docker image
// inspect`, the reference after "--".
func TestLedger_DOCK16(t *testing.T) {
	dir := project(t, map[string]string{"compose.yml": "services:\n  redis:\n    container_name: redis\n    image: redis\n"})
	f := fakeDocker(t, "", nil)
	_, _, rc := runUnit(t, f, mockEnv(), nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	var inspects []string
	for _, c := range f.Calls() {
		if slices.Contains(c.Argv, "inspect") {
			inspects = append(inspects, strings.Join(c.Argv, " "))
		}
	}
	require.Equal(t, []string{`docker image inspect --format {{range .RepoDigests}}{{.}}{{"\n"}}{{end}} -- redis`}, inspects)
}

// LEDGER DOCK-31 (3.0 fix): "--" precedes every word taken from the
// compose file, and the line parser refuses a name that starts with '-'.
func TestLedger_DOCK31(t *testing.T) {
	dir := project(t, map[string]string{"compose.yml": "services:\n  w:\n    image: nginx:latest\n  v:\n    image: redis:latest\n"})
	f := fakeDocker(t, "", nil)
	_, _, rc := runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--yes")
	require.Equal(t, 0, rc)
	for _, c := range composeCalls(f) {
		if i := slices.Index(c.Argv, "w"); i >= 0 {
			require.Equal(t, "--", c.Argv[i-1], c.Argv)
			require.Equal(t, []string{"w", "v"}, c.Argv[i:])
		}
	}
	f = fakeDocker(t, "", nil)
	_, _, _ = runUnit(t, f, mockEnv(), nil, nil, "docker_compose_check", dir, "--json")
	for _, c := range f.Calls() {
		if slices.Contains(c.Argv, "inspect") {
			require.Equal(t, "--", c.Argv[len(c.Argv)-2], c.Argv)
		}
	}

	bad := project(t, map[string]string{"compose.yml": "services:\n  --help:\n    image: nginx:latest\n"})
	for _, fn := range []string{"docker_compose_upgrade", "docker_compose_check"} {
		f = fakeDocker(t, "", nil)
		out, errOut, rc := runUnit(t, f, nil, nil, nil, fn, bad, "--dry-run")
		if fn == "docker_compose_check" {
			out, errOut, rc = runUnit(t, f, nil, nil, nil, fn, bad)
		}
		require.Equal(t, 1, rc, fn)
		require.Empty(t, out, fn)
		require.Equal(t, "Error: Invalid service name '--help' in "+bad+"/compose.yml\n", errOut, fn)
	}
	_, errOut, rc := runUnit(t, fakeDocker(t, "", nil), nil, nil, nil, "docker_compose_check", bad, "--json")
	require.Equal(t, 1, rc)
	require.Equal(t, `{"error":"invalid_service_name","details":"--help","action":"CONTINUE","recoverable":true,"hint":"Rename the service in `+
		bad+`/compose.yml: a name cannot start with '-'"}`+"\n", errOut)
}

// LEDGER DOCK-18 (3.0 fix): a declined upgrade returns 1 and pulls
// nothing.
func TestLedger_DOCK18(t *testing.T) {
	dir := project(t, map[string]string{"compose.yml": "services:\n  w:\n    image: nginx:latest\n"})
	f := fakeDocker(t, "", nil)
	_, errOut, rc := runUnit(t, f, nil, nil, func(t *term.Term) *term.Term { return t.WithTTY("n\n") }, "docker_compose_upgrade", dir)
	require.Equal(t, 1, rc)
	require.Contains(t, errOut, "Proceed with upgrade? [y/N] Info: Upgrade cancelled.\n")
	for _, c := range f.Calls() {
		require.NotContains(t, c.Argv, "pull")
	}
	f = fakeDocker(t, "", nil)
	_, _, rc = runUnit(t, f, nil, nil, func(t *term.Term) *term.Term { return t.WithTTY("y\n") }, "docker_compose_upgrade", dir)
	require.Equal(t, 0, rc)
	require.Contains(t, argvs(composeCalls(f)), "docker compose --project-directory "+dir+" pull -- w")
}

// LEDGER DOCK-20 (3.0 fix): a failing listing is an error before any
// action; stop and start refuse unknown arguments.
func TestLedger_DOCK20(t *testing.T) {
	s := newGoScene(t)
	s.stubs["docker"] = failingDocker
	for _, tc := range []struct {
		fn   string
		args []string
	}{{"docker_stop_all", nil}, {"docker_start_all", nil}, {"docker_prune_all", []string{"--dry-run"}}, {"docker_prune_all", []string{"-y"}}} {
		r := s.runGo(tc.fn, nil, tc.args...)
		require.Equal(t, 1, r.RC, tc.fn)
		require.Equal(t, "Error: Cannot list Docker objects: Cannot connect to the Docker daemon\n", r.Err, tc.fn)
		for _, verb := range []string{" stop", " start", " rm", " rmi", "system prune"} {
			require.NotContains(t, r.Docker, "docker"+verb, tc.fn)
		}
	}
	for _, fn := range []string{"docker_stop_all", "docker_start_all"} {
		r := s.runGo(fn, nil, "--bogus")
		require.Equal(t, 1, r.RC)
		require.Equal(t, "Error: Unknown option: --bogus (usage: "+fn+" [-y])\n", r.Err)
		require.Empty(t, r.Docker)
	}
}

// LEDGER DOCK-21 (3.0 fix): headless without -y prune says that -y is
// needed, and removes nothing.
func TestLedger_DOCK21(t *testing.T) {
	s := newGoScene(t)
	s.env = []string{"GASH_HEADLESS=1"}
	r := s.runGo("docker_prune_all", nil)
	require.Equal(t, 1, r.RC)
	require.Equal(t, "Info: To remove: 0 container(s), 2 image(s), 1 volume(s), 0 network(s), plus the build cache.\n"+
		"Warning: This cannot be undone: volume data is lost.\nError: No terminal to ask for confirmation: pass -y to go ahead.\n", r.Err)
	require.NotContains(t, r.Docker, " rm ")
	require.NotContains(t, r.Docker, "system prune")
	// Without a terminal, outside headless mode, the same.
	s.env = nil
	r = s.runGo("docker_prune_all", nil)
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Error: No terminal to ask for confirmation: pass -y to go ahead.\n")
	// An interactive "no" keeps Aborted.
	r = s.runGo("docker_prune_all", func(t *term.Term) *term.Term { return t.WithTTY("n\n") })
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Info: Aborted.\n")
	require.NotContains(t, r.Docker, "system prune")
}

// LEDGER DOCK-22 (operator decision, 3.0): stop and start list the
// containers and ask Y/n; headless or without a terminal they need -y.
func TestLedger_DOCK22(t *testing.T) {
	s := newGoScene(t)
	s.stubs["docker"] = "#!/bin/sh\necho \"docker $*\" >>\"$MOCK_DOCKER_LOG\"\ncase $1 in ps) printf 'a1\\tweb\\nb2\\tdb\\n' ;; esac\n"
	s.env = []string{"GASH_HEADLESS=1"}
	for _, fn := range []string{"docker_stop_all", "docker_start_all"} {
		verb := strings.TrimSuffix(strings.TrimPrefix(fn, "docker_"), "_all")
		r := s.runGo(fn, nil)
		require.Equal(t, 1, r.RC, fn)
		require.Equal(t, "Will "+verb+" 2 container(s): web db\n", r.Out)
		require.Equal(t, "Error: No terminal to ask for confirmation: pass -y to go ahead.\n", r.Err)
		require.NotContains(t, r.Docker, "docker "+verb+" ")

		r = s.runGo(fn, nil, "-y")
		require.Equal(t, 0, r.RC, fn)
		require.Equal(t, 1, strings.Count(r.Docker, "docker "+verb+" "))
		require.Contains(t, r.Docker, "docker "+verb+" a1 b2\n")
	}
	s.env = nil
	r := s.runGo("docker_stop_all", func(t *term.Term) *term.Term { return t.WithTTY("n\n") })
	require.Equal(t, 1, r.RC)
	require.Contains(t, r.Err, "Do you want to continue? [Y/n] Info: Aborted.\n")
	require.NotContains(t, r.Docker, "docker stop ")
	r = s.runGo("docker_stop_all", func(t *term.Term) *term.Term { return t.WithTTY("\n") })
	require.Equal(t, 0, r.RC)
	require.Contains(t, r.Docker, "docker stop a1 b2\n")
}

// LEDGER DOCK-28 (3.0 fix): a quoted value ends at its quote, and " #"
// is a comment only outside quotes.
func TestLedger_DOCK28(t *testing.T) {
	require.Equal(t, []service{{"w", "nginx:latest #x"}}, lineParser([]byte("services:\n  w:\n    image: \"nginx:latest #x\"\n")))
	require.Equal(t, []service{{"w", "nginx:latest"}}, lineParser([]byte("services:\n  w:\n    image: nginx:latest #x\n")))
	require.Equal(t, []service{{"w", "nginx:1"}}, lineParser([]byte("services:\n  w:\n    image: 'nginx:1' # c\n")))
}

// LEDGER DOCK-29 (3.0 fix): compose.yaml wins over docker-compose.yml,
// as in Compose, and no compose argv names a file.
func TestLedger_DOCK29(t *testing.T) {
	dir := project(t, map[string]string{
		"docker-compose.yml": "services:\n  old:\n    image: nginx:latest\n",
		"compose.yaml":       "services:\n  new:\n    image: nginx:latest\n",
	})
	f := fakeDocker(t, "", nil)
	out, _, rc := runUnit(t, f, mockEnv(), nil, nil, "docker_compose_check", dir, "--json")
	require.Equal(t, 0, rc)
	require.Equal(t, "compose.yaml", jsonOf(t, out)["compose_file"])
	f = fakeDocker(t, "", nil)
	out, _, rc = runUnit(t, f, nil, nil, nil, "docker_compose_upgrade", dir, "--dry-run")
	require.Equal(t, 0, rc)
	require.Contains(t, out, "OK: new (nginx:latest)")
	for _, c := range f.Calls() {
		require.NotContains(t, c.Argv, "-f")
	}
}

// DOCK-17: one help text per compose function. NAME -h prints the
// registered help (the page of `gash help NAME`); its usage line names
// every option the parser accepts, and the check help names every status
// the code can emit.
func TestLedger_DOCK17(t *testing.T) {
	usage := map[string][]string{
		"docker_compose_check":   {"[PATH]", "--json", "--no-color"},
		"docker_compose_scan":    {"[BASEDIR]", "-d", "--depth N", "-j", "--json", "--no-color"},
		"docker_compose_upgrade": {"[PATH]", "--dry-run", "-f", "--force", "-y", "--yes", "--no-color"},
		"docker_stop_all":        {"[-y]"},
		"docker_start_all":       {"[-y]"},
	}
	for fn, words := range usage {
		f, ok := registry.Lookup(fn)
		require.True(t, ok, fn)
		body := f.Help.Body
		_, rest, found := strings.Cut(body, "USAGE\n")
		require.True(t, found, fn)
		line, _, _ := strings.Cut(rest, "\n")
		for _, w := range words {
			require.Contains(t, line, w, fn)
		}
		if strings.HasPrefix(fn, "docker_compose_") {
			var want bytes.Buffer
			help.Display(&want, false, f)
			for _, arg := range []string{"-h", "--help"} {
				out, errOut, rc := runUnit(t, runner.NewFake(t), nil, nil, nil, fn, arg)
				require.Equal(t, 0, rc, fn)
				require.Empty(t, errOut, fn)
				require.Equal(t, want.String(), out, fn)
			}
		}
	}
	f, _ := registry.Lookup("docker_compose_check")
	for _, st := range []string{"CURRENT", "UPDATE", "NOT_PULLED", "PINNED", "ERROR"} {
		require.Contains(t, f.Help.Body, "  "+st+" ", st)
	}
	require.NotContains(t, f.Help.Body, "UP-TO-DATE")
	require.Contains(t, f.Help.Body, "exit status is 0 whenever the report prints")
	require.Contains(t, f.Help.Body, "the others anonymously")
	for _, code := range []string{"dns_error", "connection_refused", "timeout", "ssl_error", "transport_error",
		"unauthorized", "forbidden", "not_found", "rate_limited", "server_error", "http_NNN", "malformed_token",
		"no_digest_in_response", "registry_unreachable", "invalid_reference"} {
		require.Contains(t, f.Help.Body, code)
	}
	u, _ := registry.Lookup("docker_compose_upgrade")
	require.Contains(t, u.Help.Body, "override files")
}

// DOCK-03: the completion word lists name the flags the functions accept
// (testdata/script/dock03_completion.txtar drives __gash_complete).
func TestLedger_DOCK03(t *testing.T) {
	for fn, words := range map[string]string{
		"docker_compose_upgrade": "--dry-run --force -f --yes -y --no-color --help",
		"docker_compose_scan":    "-d --depth -j --json --no-color --help",
		"docker_compose_check":   "--json --no-color --help",
		"docker_prune_all":       "-y --yes --dry-run --help",
		"docker_stop_all":        "-y --yes --help",
		"docker_start_all":       "-y --yes --help",
		"git_repair":             "-y --yes --dry-run --help",
		"git_sync":               "-m --message --no-push --dry-run -y --yes --help",
		"git_add_tag":            "-y --yes --dry-run --help",
		"git_delete_tag":         "-y --yes --dry-run --help",
		"git_apply_patch":        "-y --yes --dry-run --help",
		"remote_push":            "-r --recursive -p --preserve --no-progress --force --help",
		"remote_pull":            "-r --recursive -p --preserve --no-progress --force --help",
		"remote_sync_push":       "--delete --dry-run --no-progress --force --exclude= --help",
		"remote_sync_pull":       "--delete --dry-run --no-progress --force --exclude= --help",
		"remote_exec":            "--script --help",
	} {
		f, ok := registry.Lookup(fn)
		require.True(t, ok, fn)
		require.Contains(t, f.Completion, `compgen -W "`+words+`"`, fn)
	}
}
