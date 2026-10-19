// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// route is one canned answer of the test server.
type route struct {
	status int
	header map[string]string
	body   string
}

// server answers every request of the module, whatever its host, from
// routes keyed "METHOD URL"; a GET route answers HEAD too, anything else
// is a 404.
type server struct {
	mu     sync.Mutex
	routes map[string]route
	seen   []string
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key := "https://" + r.Host + r.URL.RequestURI()
	s.mu.Lock()
	s.seen = append(s.seen, r.Method+" "+key)
	rt, ok := s.routes[r.Method+" "+key]
	if !ok && r.Method == http.MethodHead {
		rt, ok = s.routes["GET "+key]
	}
	s.mu.Unlock()
	if !ok {
		rt = route{status: 404, body: "not found"}
	}
	for k, v := range rt.header {
		w.Header().Set(k, v)
	}
	if w.Header().Get("Content-Length") == "" {
		w.Header().Set("Content-Length", strconv.Itoa(len(rt.body)))
	}
	w.WriteHeader(rt.status)
	if r.Method != http.MethodHead {
		_, _ = w.Write([]byte(rt.body))
	}
}

// rewrite sends every request to the test server, keeping its Host.
type rewrite struct {
	base http.RoundTripper
	addr string
}

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = "https"
	req.URL.Host = r.addr
	return r.base.RoundTrip(req)
}

// down is a network that refuses every request.
type down struct{}

func (down) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("connection refused")
}

const (
	pwJSON      = `{"version":"1.56.0"}`
	browserJSON = `{"browsers":[{"name":"chromium","browserVersion":"1.0.0.0"},{"name":"chromium-headless-shell","revision":"1194","browserVersion":"141.0.7390.37"}]}`
	target      = "141.0.7390.37"
	zipURL      = "https://storage.example/141.0.7390.37/linux64/chrome-headless-shell-linux64.zip"
	browsersURL = "https://cdn.jsdelivr.net/npm/playwright-core@1.56.0/browsers.json"
	unpkgURL    = "https://unpkg.com/playwright-core@1.56.0/browsers.json"
)

// cftRoutes are the routes of chrome_spec: the three JSON documents and a
// 2048-byte archive.
func cftRoutes() map[string]route {
	return map[string]route{
		"GET " + npmLatest:                     {status: 200, body: pwJSON},
		"GET " + browsersURL:                   {status: 200, body: browserJSON},
		"GET " + cftVersion + target + ".json": {status: 200, body: `{"downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"` + zipURL + `"}]}}`},
		"GET " + zipURL:                        {status: 200, body: strings.Repeat("\x00", 2048)},
	}
}

// Stubs on PATH: uname, unzip and ldd answer from MOCK_* variables of the
// call's environment; sudo logs its argv and runs it.
const (
	unameStub = `#!/bin/sh
case "$1" in -m) echo "${MOCK_ARCH:-x86_64}" ;; *) echo "${MOCK_SYS:-Linux}" ;; esac
`
	unzipStub = `#!/bin/sh
case "$1" in
-Z1) echo "${MOCK_ZIPLIST:-chrome-headless-shell-linux64/chrome-headless-shell}" ;;
-q) mkdir -p "$4/chrome-headless-shell-linux64" &&
	printf '#!/bin/sh\necho "%s"\n' "${MOCK_VERSION:-Chrome-Headless-Shell 141.0.7390.37}" >"$4/chrome-headless-shell-linux64/chrome-headless-shell" &&
	chmod +x "$4/chrome-headless-shell-linux64/chrome-headless-shell" ;;
esac
`
	lddStub = `#!/bin/sh
[ -n "$MOCK_LDD" ] && printf '\tlibnss3.so => not found\n\tlibgbm.so.1 => not found\n\tlibasound.so.2 => not found\n\tlibnss3.so => not found\n'
exit 0
`
	sudoStub = `#!/bin/sh
printf 'sudo %s\n' "$*" >>"$SUDO_LOG"
exec "$@"
`
)

type fixture struct {
	dir, bin, root, link string
	env                  map[string]string
	srv                  *server
	m                    *Module
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	fx := &fixture{dir: dir, bin: filepath.Join(dir, "bin"), root: filepath.Join(dir, "opt", "chs")}
	fx.link = filepath.Join(fx.bin, "chs")
	for _, d := range []string{fx.bin, filepath.Join(dir, "opt"), filepath.Join(dir, "tmp")} {
		require.NoError(t, os.MkdirAll(d, 0o755))
	}
	for name, body := range map[string]string{"uname": unameStub, "unzip": unzipStub, "ldd": lddStub, "sudo": sudoStub} {
		require.NoError(t, os.WriteFile(filepath.Join(fx.bin, name), []byte(body), 0o755))
	}
	fx.env = map[string]string{
		"HOME": dir, "LANG": "C.UTF-8", "PATH": fx.bin + ":/usr/bin:/bin", "TMPDIR": filepath.Join(dir, "tmp"),
		"GASH_CHROME_ROOT": fx.root, "GASH_CHROME_LINK": fx.link, "SUDO_LOG": filepath.Join(dir, "sudo.log"),
	}
	fx.srv = &server{routes: cftRoutes()}
	ts := httptest.NewTLSServer(fx.srv)
	t.Cleanup(ts.Close)
	fx.m = &Module{run: runner.Exec{}, client: &http.Client{Transport: rewrite{ts.Client().Transport, ts.Listener.Addr().String()}}}
	return fx
}

type result struct{ out, err string }

// fetched hides the timing of the download summary.
var fetched = regexp.MustCompile(`Fetched (.*) in [0-9]+s \(.*\)`)

func (fx *fixture) call(t *testing.T, tt *term.Term, args ...string) (result, int) {
	t.Helper()
	e := invoke.EnvMap(fx.env)
	if tt == nil {
		tt = term.Fixed(e, false, false, false)
	}
	var out, errb bytes.Buffer
	c := &invoke.Call{Name: "chrome_headless_update", Args: args, Env: e, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb, Term: tt}
	tbl := invoke.NewTable()
	fx.m.Register(tbl)
	rc := tbl.Run(c)
	norm := func(s string) string {
		s = strings.ReplaceAll(s, fx.dir, "T")
		return fetched.ReplaceAllString(s, "Fetched $1 in Ns (S)")
	}
	return result{norm(out.String()), norm(errb.String())}, rc
}

// version makes ROOT/V with an executable that prints `Chrome-Headless-
// Shell SAYS`.
func (fx *fixture) version(t *testing.T, v, says string) string {
	t.Helper()
	bin := filepath.Join(fx.root, v, binRel)
	require.NoError(t, os.MkdirAll(filepath.Dir(bin), 0o755))
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\necho 'Chrome-Headless-Shell "+says+"'\n"), 0o755))
	return bin
}

// tree lists what is under ROOT, as find | sort prints it.
func (fx *fixture) tree(t *testing.T) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(fx.root, func(p string, _ os.FileInfo, err error) error {
		if err == nil && p != fx.root {
			out = append(out, strings.TrimPrefix(p, fx.root+"/"))
		}
		return nil
	})
	return out
}

func (fx *fixture) sudo(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(fx.env["SUDO_LOG"])
	if os.IsNotExist(err) {
		return ""
	}
	require.NoError(t, err)
	return strings.ReplaceAll(string(b), fx.dir, "T")
}

const (
	tasks   = "Resolving the latest Playwright release... done\nReading the pinned browser build... done\n"
	head    = "Latest Playwright: playwright-core 1.56.0 (chromium-headless-shell 141.0.7390.37, revision 1194)\n"
	newPlan = "The following NEW packages will be installed:\n  chrome-headless-shell 141.0.7390.37 (from playwright-core 1.56.0)\n" +
		"Need to get 2.0 KiB of archives from " + zipURL + "\n"
	located     = "Locating the Chrome for Testing download... done\n"
	fetchedNote = "Get: " + zipURL + " [2.0 KiB]\nFetched 2.0 KiB in Ns (S)\nUnpacking chrome-headless-shell 141.0.7390.37... done\n"
)

func TestDryRun(t *testing.T) {
	fx := newFixture(t)
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, result{head + "Installed: none\n" + newPlan, tasks + located}, r)
	require.Empty(t, fx.tree(t))
}

func TestUpgradePlan(t *testing.T) {
	fx := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fx.root, "139.0.7258.5", "chrome-headless-shell-linux64"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(fx.root, "x", "chrome-headless-shell-linux64"), 0o755))
	require.NoError(t, os.Symlink(fx.version(t, "140.0.7339.16", "140.0.7339.16"), fx.link))
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, result{head + "Installed: 140.0.7339.16\n" +
		"The following packages will be upgraded:\n  chrome-headless-shell (140.0.7339.16 => 141.0.7390.37, from playwright-core 1.56.0)\n" +
		"Kept for rollback: 140.0.7339.16\nThe following versions will be REMOVED:\n  139.0.7258.5\n" +
		"Need to get 2.0 KiB of archives from " + zipURL + "\n", tasks + located}, r)
}

func TestUpToDate(t *testing.T) {
	fx := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fx.root, target, "chrome-headless-shell-linux64"), 0o755))
	// A dangling link still names the version.
	require.NoError(t, os.Symlink(filepath.Join(fx.root, target, binRel), fx.link))
	r, rc := fx.call(t, nil)
	require.Equal(t, 0, rc)
	require.Equal(t, result{head + "Installed: 141.0.7390.37\nchrome-headless-shell is already the newest version (141.0.7390.37).\n", tasks}, r)
}

func TestInstall(t *testing.T) {
	fx := newFixture(t)
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 0, rc)
	installed := "T/opt/chs/141.0.7390.37/chrome-headless-shell-linux64/chrome-headless-shell"
	require.Equal(t, result{head + "Installed: none\n" + newPlan, tasks + located + fetchedNote +
		"Info: Verified: Chrome-Headless-Shell 141.0.7390.37\nInfo: Installing into T/opt/chs/141.0.7390.37 (sudo)...\n" +
		"OK: Installed: Chrome-Headless-Shell 141.0.7390.37 -> " + installed + "\n"}, r)
	require.Equal(t, []string{target, target + "/chrome-headless-shell-linux64", target + "/" + binRel}, fx.tree(t))
	dest, err := os.Readlink(fx.link)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(fx.root, target, binRel), dest)
	// The temporary directory is gone.
	ents, err := os.ReadDir(fx.env["TMPDIR"])
	require.NoError(t, err)
	require.Empty(t, ents)
}

func TestUpgradeInstall(t *testing.T) {
	fx := newFixture(t)
	require.NoError(t, os.MkdirAll(filepath.Join(fx.root, "139.0.7258.5", "chrome-headless-shell-linux64"), 0o755))
	require.NoError(t, os.Symlink(fx.version(t, "140.0.7339.16", "140.0.7339.16"), fx.link))
	r, rc := fx.call(t, nil, "--yes")
	require.Equal(t, 0, rc)
	require.Contains(t, r.err, "Info: Installing into T/opt/chs/141.0.7390.37 (sudo)...\nInfo: Removing chrome-headless-shell 139.0.7258.5...\n"+
		"OK: Installed: Chrome-Headless-Shell 141.0.7390.37 -> T/opt/chs/141.0.7390.37/"+binRel+"\n")
	require.Equal(t, []string{"140.0.7339.16", "140.0.7339.16/chrome-headless-shell-linux64", "140.0.7339.16/" + binRel,
		target, target + "/chrome-headless-shell-linux64", target + "/" + binRel}, fx.tree(t))
	require.Contains(t, fx.sudo(t), "sudo find T/opt/chs/139.0.7258.5 -depth -delete\n")
}

func TestMissingLibraries(t *testing.T) {
	fx := newFixture(t)
	fx.env["MOCK_LDD"] = "1"
	// An Ubuntu 22.04-like apt: only the names without t64 exist.
	require.NoError(t, os.WriteFile(filepath.Join(fx.bin, "apt-cache"), []byte("#!/bin/sh\ncase \"$2\" in *t64) exit 100 ;; *) exit 0 ;; esac\n"), 0o755))
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.Equal(t, tasks+located+fetchedNote+"Error: Missing shared libraries: libasound.so.2 libgbm.so.1 libnss3.so \n"+
		"Info: Install them, then run chrome_headless_update again:  sudo apt-get install libasound2 libgbm1 libnss3\n", r.err)
	require.Empty(t, fx.tree(t))
	require.Empty(t, fx.sudo(t))
}

func TestArchiveChecks(t *testing.T) {
	fx := newFixture(t)
	fx.env["MOCK_ZIPLIST"] = "other"
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.True(t, strings.HasSuffix(r.err, "Fetched 2.0 KiB in Ns (S)\nError: The archive does not contain "+binRel+".\n"), r.err)

	delete(fx.env, "MOCK_ZIPLIST")
	fx.env["MOCK_VERSION"] = "Chrome 1"
	r, rc = fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.True(t, strings.HasSuffix(r.err, fetchedNote+"Error: Version check failed: expected 141.0.7390.37, the binary says 'Chrome 1'.\n"), r.err)
	require.Empty(t, fx.tree(t))
	ents, err := os.ReadDir(fx.env["TMPDIR"])
	require.NoError(t, err)
	require.Empty(t, ents)
}

func TestDownloadLookup(t *testing.T) {
	// The per-version document names no linux64 build: the full index.
	fx := newFixture(t)
	fx.srv.routes["GET "+cftVersion+target+".json"] = route{status: 200, body: `{"downloads":{"chrome-headless-shell":[{"platform":"mac","url":"x"}]}}`}
	fx.srv.routes["GET "+cftIndex] = route{status: 200, body: `{"versions":[{"version":"141.0.7390.37","downloads":{"chrome-headless-shell":[{"platform":"linux64","url":"https://storage.example/kg.zip"}]}}]}`}
	fx.srv.routes["HEAD https://storage.example/kg.zip"] = route{status: 200, header: map[string]string{"Content-Length": "2048"}}
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Contains(t, r.out, "Need to get 2.0 KiB of archives from https://storage.example/kg.zip\n")

	// Neither: Playwright's mirror.
	fx = newFixture(t)
	delete(fx.srv.routes, "GET "+cftVersion+target+".json")
	mirror := pwMirror + target + "/linux64/chrome-headless-shell-linux64.zip"
	fx.srv.routes["HEAD "+mirror] = route{status: 200, header: map[string]string{"Content-Length": "104857600"}}
	r, rc = fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Contains(t, r.out, "Need to get 100.0 MiB of archives from "+mirror+"\n")

	// No size anywhere: the located URL answers the HEAD with no size
	// (Content-Length 0), so locate fails and nothing is downloaded. A 2.x
	// `curl -fsIL` reads even an error response's Content-Length, so a 404
	// that still carries a size would proceed instead (RemoteSize is faithful
	// to that); this drives the size genuinely to 0 to cover the no-download
	// branch.
	fx = newFixture(t)
	delete(fx.srv.routes, "GET "+zipURL)
	fx.srv.routes["HEAD "+zipURL] = route{status: 404}
	r, rc = fx.call(t, nil, "--dry-run")
	require.Equal(t, 1, rc)
	require.Equal(t, result{head + "Installed: none\n", tasks + "Locating the Chrome for Testing download... failed\n" +
		"Error: No download found for chrome-headless-shell 141.0.7390.37 (linux64). Nothing changed.\n"}, r)
}

func TestBrowsersJSON(t *testing.T) {
	// jsdelivr fails: unpkg.
	fx := newFixture(t)
	delete(fx.srv.routes, "GET "+browsersURL)
	fx.srv.routes["GET "+unpkgURL] = route{status: 200, body: browserJSON}
	r, rc := fx.call(t, nil, "--dry-run")
	require.Equal(t, 0, rc)
	require.Equal(t, head+"Installed: none\n"+newPlan, r.out)

	// No entry for the shell.
	fx = newFixture(t)
	fx.srv.routes["GET "+browsersURL] = route{status: 200, body: strings.Replace(browserJSON, `chromium-headless-shell"`, `other"`, 1)}
	r, rc = fx.call(t, nil, "--dry-run")
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Resolving the latest Playwright release... done\nReading the pinned browser build... failed\n" +
		"Error: No chromium-headless-shell entry in browsers.json of playwright-core 1.56.0. Nothing changed.\n"}, r)

	// An entry without a revision.
	fx = newFixture(t)
	fx.srv.routes["GET "+browsersURL] = route{status: 200, body: `{"browsers":[{"name":"chromium-headless-shell","browserVersion":"141.0.7390.37"}]}`}
	r, _ = fx.call(t, nil, "--dry-run")
	require.Contains(t, r.out, "(chromium-headless-shell 141.0.7390.37, revision ?)\n")
}

func TestUnreachable(t *testing.T) {
	fx := newFixture(t)
	fx.m.client = &http.Client{Transport: down{}}
	r, rc := fx.call(t, nil, "-y")
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Resolving the latest Playwright release... failed\n" +
		"Error: Unable to read playwright-core from the npm registry (network?). Nothing changed.\n"}, r)
}

func TestConfirm(t *testing.T) {
	fx := newFixture(t)
	r, rc := fx.call(t, nil)
	require.Equal(t, 1, rc)
	require.Equal(t, tasks+located+"Error: No terminal to ask for confirmation: pass -y to go ahead.\n", r.err)

	e := invoke.EnvMap(fx.env)
	r, rc = fx.call(t, term.Fixed(e, true, true, true).WithTTY("n\n"))
	require.Equal(t, 1, rc)
	require.Contains(t, r.err, "Aborted.")
	require.Empty(t, fx.tree(t))

	r, rc = fx.call(t, term.Fixed(e, true, true, true).WithTTY("\n"))
	require.Equal(t, 0, rc, r.err)
	require.Contains(t, r.err, "Installed: Chrome-Headless-Shell 141.0.7390.37")
}

func TestPlatform(t *testing.T) {
	fx := newFixture(t)
	fx.env["MOCK_ARCH"] = "aarch64"
	r, rc := fx.call(t, nil)
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Error: Chrome for Testing publishes chrome-headless-shell for Linux x86_64 only (this is Linux aarch64).\n"}, r)

	fx.env["MOCK_ARCH"] = "x86_64"
	fx.env["MOCK_SYS"] = "Darwin"
	r, rc = fx.call(t, nil)
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Error: Chrome for Testing publishes chrome-headless-shell for Linux x86_64 only (this is Darwin x86_64).\n"}, r)
	require.Empty(t, fx.srv.seen)
}

func TestOptionsAndTools(t *testing.T) {
	fx := newFixture(t)
	r, rc := fx.call(t, nil, "-y", "--bogus")
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Error: Unknown option: --bogus (usage: chrome_headless_update [-y] [--dry-run])\n"}, r)

	r, rc = fx.call(t, nil, "--help")
	require.Equal(t, 0, rc)
	require.Contains(t, r.out, "chrome_headless_update [-y] [--dry-run]")

	require.NoError(t, os.Remove(filepath.Join(fx.bin, "ldd")))
	fx.env["PATH"] = fx.bin
	r, rc = fx.call(t, nil)
	require.Equal(t, 1, rc)
	require.Equal(t, result{"", "Error: chrome_headless_update needs 'ldd'.\n"}, r)
	require.Empty(t, fx.srv.seen)
}

func TestLibToPkg(t *testing.T) {
	for lib, pkg := range map[string]string{
		"libnss3.so": "libnss3", "libsmime3.so": "libnss3", "libplds4.so": "libnspr4",
		"libatk-bridge-2.0.so.0": "libatk-bridge2.0-0t64", "libatk-1.0.so.0": "libatk1.0-0t64",
		"libgbm.so.1": "libgbm1", "libXrandr.so.2": "libxrandr2", "libX11-xcb.so.1": "libx11-6",
		"libasound.so.2": "libasound2t64", "libgio-2.0.so.0": "libglib2.0-0t64", "libfoo.so": "?(libfoo.so)",
	} {
		require.Equal(t, pkg, LibToPkg(lib), lib)
	}

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apt-cache"), []byte("#!/bin/sh\ncase \"$2\" in *t64) exit 100 ;; *) exit 0 ;; esac\n"), 0o755))
	m := &Module{run: runner.Exec{}}
	env := []string{"PATH=" + dir}
	require.Equal(t, "libasound2", m.PkgName(t.Context(), env, LibToPkg("libasound.so.2")))
	require.Equal(t, "libgbm1", m.PkgName(t.Context(), env, "libgbm1"))
	// Both names known, or no apt: the t64 name.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "apt-cache"), []byte("#!/bin/sh\nexit 0\n"), 0o755))
	require.Equal(t, "libcups2t64", m.PkgName(t.Context(), env, "libcups2t64"))
	require.Equal(t, "libcups2t64", m.PkgName(t.Context(), []string{"PATH=" + t.TempDir()}, "libcups2t64"))
}

func TestReadlinkF(t *testing.T) {
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "real", "v"), 0o755))
	require.NoError(t, os.Symlink("real", filepath.Join(dir, "rl")))
	require.NoError(t, os.Symlink(filepath.Join(dir, "rl", "v", "missing"), filepath.Join(dir, "l1")))
	require.NoError(t, os.Symlink("l1", filepath.Join(dir, "l2")))
	got, ok := readlinkF(filepath.Join(dir, "l2"))
	require.True(t, ok)
	require.Equal(t, filepath.Join(dir, "real", "v", "missing"), got)
	_, ok = readlinkF(filepath.Join(dir, "nodir", "x", "y"))
	require.False(t, ok)
	require.NoError(t, os.Symlink("loop", filepath.Join(dir, "loop")))
	_, ok = readlinkF(filepath.Join(dir, "loop"))
	require.False(t, ok)
}
