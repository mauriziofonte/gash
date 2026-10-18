// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package chrome

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// libPkgs is __gash_chs_lib_pkg: soname prefix to the package that ships
// it on Ubuntu 24.04 / Debian 13, in the order of the 2.x case.
var libPkgs = []struct{ prefixes, pkg string }{
	{"libnss3 libnssutil3 libsmime3", "libnss3"},
	{"libnspr4 libplc4 libplds4", "libnspr4"},
	{"libatk-bridge-2.0", "libatk-bridge2.0-0t64"},
	{"libatk-1.0", "libatk1.0-0t64"},
	{"libatspi", "libatspi2.0-0t64"},
	{"libcups", "libcups2t64"},
	{"libdrm", "libdrm2"},
	{"libgbm", "libgbm1"},
	{"libxkbcommon", "libxkbcommon0"},
	{"libXcomposite", "libxcomposite1"},
	{"libXdamage", "libxdamage1"},
	{"libXfixes", "libxfixes3"},
	{"libXrandr", "libxrandr2"},
	{"libXext", "libxext6"},
	{"libX11", "libx11-6"},
	{"libxcb", "libxcb1"},
	{"libasound", "libasound2t64"},
	{"libpango", "libpango-1.0-0"},
	{"libcairo", "libcairo2"},
	{"libdbus", "libdbus-1-3"},
	{"libexpat", "libexpat1"},
	{"libglib libgobject libgio", "libglib2.0-0t64"},
}

// LibToPkg is __gash_chs_lib_pkg SONAME: the package of a missing shared
// library, "?(SONAME)" when unknown.
func LibToPkg(soname string) string {
	for _, e := range libPkgs {
		for _, p := range strings.Fields(e.prefixes) {
			if strings.HasPrefix(soname, p) {
				return e.pkg
			}
		}
	}
	return "?(" + soname + ")"
}

// PkgName is __gash_chs_pkg_name PKG: a t64 package without its suffix
// when this system's apt knows only the older name.
func (m *Module) PkgName(ctx context.Context, env []string, pkg string) string {
	base, ok := strings.CutSuffix(pkg, "t64")
	if !ok {
		return pkg
	}
	if _, err := runner.LookPath("apt-cache", env, ""); err != nil {
		return pkg
	}
	show := func(p string) bool {
		res, err := m.run.Run(ctx, runner.Cmd{Path: "apt-cache", Args: []string{"show", p},
			Options: runner.Options{Env: env, Stdout: io.Discard, Stderr: io.Discard}})
		return err == nil && res.ExitCode == 0
	}
	if !show(pkg) && show(base) {
		return base
	}
	return pkg
}

var errTooManyRedirects = errors.New("too many redirects")

// httpClient is the client of every request: curl -L's redirect limit,
// the proxies and the CA of the environment.
func (m *Module) httpClient() *http.Client {
	if m.client != nil {
		return m.client
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = http.ProxyFromEnvironment
	return &http.Client{Transport: t, CheckRedirect: func(_ *http.Request, via []*http.Request) error {
		if len(via) > 50 {
			return errTooManyRedirects
		}
		return nil
	}}
}

// get is `curl -fsSL URL`: the body and whether curl succeeded (a status
// of 400 or more fails with no output; a cut transfer keeps what came).
func (m *Module) get(ctx context.Context, url string) ([]byte, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false
	}
	resp, err := m.httpClient().Do(req)
	if err != nil {
		return nil, false
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return nil, false
	}
	body, err := io.ReadAll(resp.Body)
	return body, err == nil
}

// locate is __gash_chs_locate VERSION: the download URL of that exact
// Chrome for Testing build (the per-version endpoint, the full index,
// then Playwright's mirror) and its size from a HEAD request.
func (m *Module) locate(ctx context.Context, v string) (string, int64) {
	body, _ := m.get(ctx, cftVersion+v+".json")
	url := jqRun(body, jqLinux64)
	if url == "" {
		body, _ = m.get(ctx, cftIndex)
		url = jqRun(body, jqKnownGood(v))
	}
	if url == "" {
		url = pwMirror + v + "/linux64/chrome-headless-shell-linux64.zip"
	}
	return url, ui.Downloader{Client: m.client}.RemoteSize(ctx, url)
}
