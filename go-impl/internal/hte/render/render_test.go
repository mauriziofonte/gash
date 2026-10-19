// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package render

import (
	"bytes"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

var update = flag.Bool("update", false, "rewrite the golden files under testdata/hte/golden")

const goldenRoot = "../../../testdata/hte/golden"

func paths(home, runtime string) state.Paths {
	return state.Paths{
		ConfigDir:  home + "/.config/gash/hte",
		StateDir:   home + "/.local/state/gash/hte",
		DataDir:    home + "/.local/share/gash/hte",
		RuntimeDir: runtime,
	}
}

func modules(dir string, builtin ...string) []Module {
	var out []Module
	for _, m := range BaseModules {
		if slices.Contains(builtin, m) {
			continue
		}
		out = append(out, Module{Name: m, Path: dir + "/mod_" + m + ".so"})
	}
	return out
}

// site returns a managed site with the HTE-55 defaults, changed by mut.
func site(domain string, mut func(*state.Site)) state.Site {
	s := state.Site{
		Domain:  domain,
		Aliases: []string{"www." + domain},
		Docroot: "/home/me/src/" + domain + "/public",
		PHP:     "8.3",
		Profile: string(profiles.Dev),
		HTTPS:   true,
		HTTP2:   true,
		Enabled: true,
		Kind:    state.KindManaged,
	}
	if mut != nil {
		mut(&s)
	}
	return s
}

// debianStandard is Debian/Ubuntu with standard ports and IPv6 loopback:
// every site variant.
func debianStandard() Input {
	p := paths("/home/me", "/run/user/1000/gash-hte")
	return Input{
		GenDir: p.RenderDir() + "/gen-7",
		Paths:  p,
		Apache: Apache{
			Binary:    "/usr/sbin/apache2",
			Modules:   modules("/usr/lib/apache2/modules", "log_config", "logio", "unixd"),
			MimeTypes: "/etc/mime.types",
		},
		Ports:  state.StandardPorts,
		Listen: []string{state.LoopbackIPv4, state.LoopbackIPv6},
		PHP: map[string]PHPVersion{
			"8.3": debianPHP("8.3"),
			"8.4": debianPHP("8.4"),
		},
		Sites: []state.Site{
			site("shop.test", func(s *state.Site) {
				s.Docroot = "/home/me/src/my shop/public"
				s.PHPIni = map[string]string{"memory_limit": "512M", "display_errors": "On", "auto_prepend_file": "",
					"session.name": "S;ID", "opcache.enable": "0"}
			}),
			site("forced.test", func(s *state.Site) {
				s.PHP, s.Profile, s.ForceHTTPS = "8.4", string(profiles.Staging), true
				s.Aliases = []string{"www.forced.test", "api.forced.test"}
				s.ExtraHTTPPorts, s.ExtraHTTPSPorts = []int{3000}, []int{3443}
			}),
			site("bypass.test", func(s *state.Site) {
				s.Profile, s.ForceHTTPS, s.ForceHTTPSBypassLoopback, s.HTTP2 = string(profiles.Hardened), true, true, false
				s.Aliases = []string{}
			}),
			site("plain.test", func(s *state.Site) { s.HTTPS, s.Aliases = false, []string{} }),
			{
				Domain: "proxy.test", Aliases: []string{}, Kind: state.KindRaw, Enabled: true,
				RawConf:        "<VirtualHost 127.0.0.1:80 127.0.0.1:8081>\n    ServerName proxy.test\n    ProxyPass / http://127.0.0.1:3000/\n</VirtualHost>",
				ExtraHTTPPorts: []int{8081, 3000},
			},
			site("off.test", func(s *state.Site) { s.Enabled, s.PHP = false, "8.4" }),
		},
		Overlays: map[string][]byte{
			p.ApacheOverlay():            []byte("LogLevel info\n"),
			p.SiteOverlay("forced.test"): []byte("Header set X-Forced 1\n"),
		},
	}
}

func debianPHP(v string) PHPVersion {
	return PHPVersion{FPM: "/usr/sbin/php-fpm" + v, INI: "/etc/php/" + v + "/fpm/php.ini", ScanDir: "/etc/php/" + v + "/fpm/conf.d"}
}

func withSession(dir string) PHPVersion {
	f := debianPHP("8.3")
	f.SessionDir = dir
	return f
}

// fedoraHigh is Fedora with high ports, IPv4 loopback only, every module
// loadable, and a per-version session directory (HTE-41).
func fedoraHigh() Input {
	p := paths("/home/dev", "/run/user/1001/gash-hte")
	return Input{
		GenDir: p.RenderDir() + "/gen-1",
		Paths:  p,
		Apache: Apache{Binary: "/usr/sbin/httpd", Modules: modules("/usr/lib64/httpd/modules"), MimeTypes: "/etc/mime.types"},
		Ports:  state.HighPorts,
		Listen: []string{state.LoopbackIPv4},
		PHP: map[string]PHPVersion{"8.3": {FPM: "/usr/sbin/php-fpm", INI: "/etc/php.ini", ScanDir: "/etc/php.d",
			SessionDir: p.StateDir + "/php/8.3/sessions"}},
		Sites: []state.Site{
			site("app.test", func(s *state.Site) {
				s.Docroot, s.Profile, s.ForceHTTPS = "/home/dev/app/public", string(profiles.Staging), true
			}),
			site("lab.test", func(s *state.Site) {
				s.Docroot, s.Profile, s.Aliases = "/home/dev/lab", string(profiles.Hardened), []string{}
			}),
			site("dev.test", func(s *state.Site) { s.Docroot = "/home/dev/dev" }),
		},
	}
}

// macosPF is Homebrew on macOS: high ports behind the pf redirect, so the
// redirect target is port 443; the runtime directory falls back to /tmp.
func macosPF() Input {
	p := paths("/Users/me", "/tmp/gash-hte-501")
	return Input{
		GenDir: p.RenderDir() + "/gen-3",
		Paths:  p,
		Apache: Apache{Binary: "/opt/homebrew/bin/httpd", Modules: modules("/opt/homebrew/lib/httpd/modules"),
			MimeTypes: "/opt/homebrew/etc/httpd/mime.types"},
		Ports:             state.HighPorts,
		RedirectHTTPSPort: 443,
		Listen:            []string{state.LoopbackIPv4, state.LoopbackIPv6},
		PHP:               map[string]PHPVersion{"8.4": {FPM: "/opt/homebrew/opt/php/sbin/php-fpm", INI: "/opt/homebrew/etc/php/8.4/php.ini"}},
		Sites: []state.Site{
			site("mac.test", func(s *state.Site) {
				s.Docroot, s.PHP, s.ForceHTTPS = "/Users/me/Sites/mac", "8.4", true
			}),
		},
	}
}

var goldenCases = []struct {
	name string
	in   func() Input
}{
	{"debian-standard", debianStandard},
	{"fedora-high", fedoraHigh},
	{"macos-pf", macosPF},
}

func mustRender(t *testing.T, in Input) *Generation {
	t.Helper()
	g, err := Render(in)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	return g
}

func TestGolden(t *testing.T) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			g := mustRender(t, tc.in())
			dir := filepath.Join(goldenRoot, tc.name)
			if *update {
				if err := os.RemoveAll(dir); err != nil {
					t.Fatal(err)
				}
				for _, f := range g.Files {
					p := filepath.Join(dir, f.Path)
					if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, f.Data, 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			var onDisk []string
			err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(dir, p)
				onDisk = append(onDisk, filepath.ToSlash(rel))
				return nil
			})
			if err != nil {
				t.Fatalf("golden files missing (run go test -update): %v", err)
			}
			var rendered []string
			for _, f := range g.Files {
				rendered = append(rendered, f.Path)
			}
			if diff := cmp.Diff(onDisk, rendered); diff != "" {
				t.Fatalf("golden file set differs (-golden +rendered):\n%s", diff)
			}
			for _, f := range g.Files {
				want, err := os.ReadFile(filepath.Join(dir, f.Path))
				if err != nil {
					t.Fatal(err)
				}
				if diff := cmp.Diff(string(want), string(f.Data)); diff != "" {
					t.Errorf("%s differs (-golden +rendered):\n%s", f.Path, diff)
				}
			}
		})
	}
}

// managedVhosts returns the rendered vhosts of managed sites.
func managedVhosts(in Input, g *Generation) map[string]string {
	out := map[string]string{}
	for _, s := range in.Sites {
		if s.Kind == state.KindManaged && s.Enabled {
			out[s.Domain] = string(g.File(siteConfRel(s.Domain)))
		}
	}
	return out
}

func pools(g *Generation) map[string]string {
	out := map[string]string{}
	for _, f := range g.Files {
		if strings.Contains(f.Path, "/pools/") {
			out[f.Path] = string(f.Data)
		}
	}
	return out
}

func allCases(t *testing.T, fn func(t *testing.T, in Input, g *Generation)) {
	for _, tc := range goldenCases {
		t.Run(tc.name, func(t *testing.T) {
			in := tc.in()
			fn(t, in, mustRender(t, in))
		})
	}
}

func TestQuoteApacheArg(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"/srv/www", `"/srv/www"`},
		{"/home/me/my shop", `"/home/me/my shop"`},
		{`/a"b`, `"/a\"b"`},
		{`/a\b`, `"/a\\b"`},
		{`/a\`, `"/a\\"`},
		{"", `""`},
	} {
		got, err := QuoteApacheArg(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("QuoteApacheArg(%q) = %q, %v; want %q", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"/a\nInclude /etc/shadow", "/a\r", "/a\x00", "/a\tb", "\xff", "/a\u0085"} {
		if _, err := QuoteApacheArg(bad); err == nil {
			t.Errorf("QuoteApacheArg(%q) accepted", bad)
		}
	}
}

// apacheWord reads one argument the way ap_getword_conf (server/util.c)
// does, for a double-quoted word: \\ and \" are unescaped, any other
// backslash is literal.
func apacheWord(s string) (word, rest string, ok bool) {
	if !strings.HasPrefix(s, `"`) {
		return "", s, false
	}
	var b strings.Builder
	i := 1
	for ; i < len(s) && s[i] != '"'; i++ {
		if s[i] == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\') {
			i++
		}
		b.WriteByte(s[i])
	}
	if i >= len(s) {
		return "", s, false
	}
	return b.String(), s[i+1:], true
}

func FuzzQuoteApacheArg(f *testing.F) {
	for _, s := range []string{"/srv", `a"b`, `a\b`, `\`, `\"`, `"\\"`, "a b", "${HOME}", "x\ny", "é"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		q, err := QuoteApacheArg(s)
		quotable := utf8.ValidString(s) && !strings.ContainsFunc(s, unicode.IsControl)
		if err != nil {
			if quotable {
				t.Fatalf("refused quotable %q: %v", s, err)
			}
			return
		}
		if !quotable {
			t.Fatalf("accepted %q", s)
		}
		word, rest, ok := apacheWord(q)
		if !ok || rest != "" || word != s {
			t.Fatalf("%q quoted as %s reads back as %q (rest %q, ok %v)", s, q, word, rest, ok)
		}
		if strings.HasSuffix(q, `\`) || strings.ContainsAny(q, "\r\n") {
			t.Fatalf("%s could continue or break the line", q)
		}
	})
}

func TestQuoteINI(t *testing.T) {
	if got, err := quoteINI("/home/me/my shop"); err != nil || got != `"/home/me/my shop"` {
		t.Fatalf("got %q, %v", got, err)
	}
	for _, bad := range []string{`/a"b`, `/a\b`, "/a${HOME}", "/a\nb", "\xff"} {
		if _, err := quoteINI(bad); err == nil {
			t.Errorf("quoteINI(%q) accepted", bad)
		}
	}
}

func code(err error) hteerr.Code { return hteerr.CodeOf(err) }

func TestValidateRefuses(t *testing.T) {
	for _, tc := range []struct {
		name string
		mut  func(*Input)
		want hteerr.Code
	}{
		{"relative gen dir", func(in *Input) { in.GenDir = "render/gen-1" }, hteerr.PlatformUnsupported},
		{"dollar in home", func(in *Input) { in.Paths.ConfigDir = "/home/$me/.config/gash/hte" }, hteerr.PlatformUnsupported},
		{"quote in runtime", func(in *Input) { in.Paths.RuntimeDir = `/run/user/1000/"x` }, hteerr.PlatformUnsupported},
		{"semicolon in state", func(in *Input) { in.Paths.StateDir = "/home/a;b/.local/state/gash/hte" }, hteerr.PlatformUnsupported},
		{"glob in module", func(in *Input) { in.Apache.Modules[0].Path = "/usr/lib/apache2/modules/*.so" }, hteerr.PlatformUnsupported},
		{"no mime types", func(in *Input) { in.Apache.MimeTypes = "" }, hteerr.PlatformUnsupported},
		{"wildcard listen", func(in *Input) { in.Listen = []string{"0.0.0.0"} }, hteerr.InvalidState},
		{"no ipv4 loopback", func(in *Input) { in.Listen = []string{state.LoopbackIPv6} }, hteerr.InvalidState},
		{"same ports", func(in *Input) { in.Ports = state.Ports{HTTP: 80, HTTPS: 80} }, hteerr.InvalidState},
		{"bad redirect port", func(in *Input) { in.RedirectHTTPSPort = 70000 }, hteerr.InvalidState},
		{"php not installed", func(in *Input) { delete(in.PHP, "8.4") }, hteerr.PHPVersionNotInstalled},
		{"duplicate domain", func(in *Input) { in.Sites = append(in.Sites, in.Sites[0]) }, hteerr.SiteExists},
		{"shared alias", func(in *Input) {
			in.Sites = append(in.Sites, site("other.test", func(s *state.Site) { s.Aliases = []string{"www.shop.test"} }))
		}, hteerr.SiteExists},
		{"docroot with quote", func(in *Input) { in.Sites[0].Docroot = `/srv/a"b` }, hteerr.InvalidDocroot},
		{"docroot with newline", func(in *Input) { in.Sites[0].Docroot = "/srv/a\nInclude /etc/shadow" }, hteerr.InvalidDocroot},
		{"relative docroot", func(in *Input) { in.Sites[0].Docroot = "public" }, hteerr.InvalidDocroot},
		{"session dir with dollar", func(in *Input) { in.PHP["8.3"] = withSession("/var/$x") }, hteerr.PlatformUnsupported},
		{"session dir with colon", func(in *Input) { in.PHP["8.3"] = withSession("/home/a:b/sessions") }, hteerr.PlatformUnsupported},
		{"scan dir with colon", func(in *Input) {
			f := in.PHP["8.3"]
			f.ScanDir = "/etc/php/8.3/fpm/conf.d:/tmp"
			in.PHP["8.3"] = f
		}, hteerr.PlatformUnsupported},
		{"config dir with colon", func(in *Input) { in.Paths.ConfigDir = "/home/a:b/.config/gash/hte" }, hteerr.PlatformUnsupported},
		{"no fpm binary", func(in *Input) { in.PHP["8.3"] = PHPVersion{INI: "/etc/php/8.3/fpm/php.ini"} }, hteerr.PHPVersionNotInstalled},
		{"relative apache", func(in *Input) { in.Apache.Binary = "apache2" }, hteerr.ApacheNotFound},
		{"php_ini key", func(in *Input) { in.Sites[0].PHPIni = map[string]string{"memory limit": "1G"} }, hteerr.InvalidState},
		{"php_ini value newline", func(in *Input) { in.Sites[0].PHPIni = map[string]string{"x": "a\nb"} }, hteerr.InvalidState},
		{"php_ini value quote", func(in *Input) { in.Sites[0].PHPIni = map[string]string{"x": `a"b`} }, hteerr.InvalidState},
		{"php_ini value backslash", func(in *Input) { in.Sites[0].PHPIni = map[string]string{"x": `a\b`} }, hteerr.InvalidState},
		{"php_ini value expansion", func(in *Input) { in.Sites[0].PHPIni = map[string]string{"x": "${HOME}"} }, hteerr.InvalidState},
		{"forced without https", func(in *Input) { in.Sites[1].HTTPS = false }, hteerr.ConflictingOptions},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := debianStandard()
			tc.mut(&in)
			g, err := Render(in)
			if code(err) != tc.want {
				t.Fatalf("got %v (%v), want %s", err, g != nil, tc.want)
			}
		})
	}
}

// A disabled site needs no installed PHP: nothing of it is rendered.
func TestDisabledSiteIsNotRendered(t *testing.T) {
	in := debianStandard()
	in.Sites[5].PHP = "7.4" // off.test, not installed
	g := mustRender(t, in)
	for _, f := range g.Files {
		if strings.Contains(f.Path, "off.test") {
			t.Fatalf("disabled site rendered: %s", f.Path)
		}
	}
	if bytes.Contains(g.File("httpd.conf"), []byte("off.test")) {
		t.Fatal("httpd.conf includes the disabled site")
	}
}

func TestRawSiteVerbatim(t *testing.T) {
	in := debianStandard()
	g := mustRender(t, in)
	if got, want := string(g.File("sites/proxy.test.conf")), in.Sites[4].RawConf+"\n"; got != want {
		t.Fatalf("raw conf = %q, want %q", got, want)
	}
	in.Sites[4].PHP, in.Sites[4].Profile, in.Sites[4].Docroot = "8.3", "dev", "/srv/proxy"
	g = mustRender(t, in)
	if g.File("fpm/8.3/pools/proxy.test.conf") == nil {
		t.Fatal("a raw site with php gets no pool")
	}
}

func TestLedger_HTE07(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for _, s := range in.Sites {
			if !s.Enabled || !s.HasPool() {
				continue
			}
			pool := string(g.File(poolRel(s.PHP, s.Domain)))
			for _, line := range strings.Split(pool, "\n") {
				if strings.Contains(line, s.Docroot) && !strings.HasPrefix(line, "php_admin_value[open_basedir] = ") {
					t.Errorf("%s: a path under the document root: %s", s.Domain, line)
				}
			}
			logs := in.Paths.SiteLogDir(s.Domain)
			for _, want := range []string{
				`slowlog = "` + logs + `/php-slow.log"`,
				`php_admin_value[error_log] = "` + logs + `/php-error.log"`,
			} {
				if !strings.Contains(pool, want+"\n") {
					t.Errorf("%s: missing %s", s.Domain, want)
				}
			}
		}
		for _, v := range g.Versions() {
			want := `error_log = "` + in.Paths.LogsDir() + "/php-fpm-" + v + `.log"`
			if !strings.Contains(string(g.File(fpmConfRel(v))), want+"\n") {
				t.Errorf("php-fpm %s: missing %s", v, want)
			}
		}
	})
}

func TestLedger_HTE08(t *testing.T) {
	g := mustRender(t, debianStandard())
	vhost := string(g.File("sites/shop.test.conf"))
	for _, want := range []string{
		`DocumentRoot "/home/me/src/my shop/public"`,
		`<Directory "/home/me/src/my shop/public/">`,
	} {
		if strings.Count(vhost, want) != 2 {
			t.Errorf("want %s in both vhosts", want)
		}
	}
	if !strings.Contains(string(g.File("fpm/8.3/pools/shop.test.conf")), "\n; PHP settings\n") {
		t.Fatal("pool not rendered")
	}
	// mod_proxy_fcgi percent-encodes SCRIPT_FILENAME ("my%20shop"): every
	// vhost with a PHP handler gives PHP-FPM the decoded path, once per
	// VirtualHost; raw vhosts are left as they are.
	unescape := `    ProxyFCGISetEnvIf "true" SCRIPT_FILENAME "%{unescape:%{reqenv:SCRIPT_FILENAME}}"` + "\n"
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for _, s := range in.Sites {
			v := string(g.File("sites/" + s.Domain + ".conf"))
			want := 0
			if s.Kind == state.KindManaged && s.HasPool() {
				want = strings.Count(v, "<VirtualHost ")
			}
			if got := strings.Count(v, unescape); got != want {
				t.Errorf("%s: %d unescape lines, want %d", s.Domain, got, want)
			}
		}
	})
	// Every docroot character that Apache or FPM would interpret is
	// refused before rendering (TestValidateRefuses covers " and newline).
	for _, c := range []string{"$", "*", "?", "[", "]", ";", ":", `"`, `\`, "\t"} {
		in := debianStandard()
		in.Sites[0].Docroot = "/srv/a" + c + "b"
		if _, err := Render(in); code(err) != hteerr.InvalidDocroot {
			t.Errorf("docroot with %q: %v", c, err)
		}
	}
	// Every config line holding a path quotes it.
	unquoted := regexp.MustCompile(`(?m)^\s*(DocumentRoot|ErrorLog|CustomLog|SSLCertificateFile|SSLCertificateKeyFile|IncludeOptional|SetHandler|PidFile|ServerRoot|DefaultRuntimeDir|TypesConfig|LoadModule \S+) [^"]`)
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for _, f := range g.Files {
			if f.Path == "sites/proxy.test.conf" {
				continue
			}
			if m := unquoted.FindString(string(f.Data)); m != "" {
				t.Errorf("%s: unquoted path: %q", f.Path, m)
			}
		}
	})
}

func TestLedger_HTE10(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for d, v := range managedVhosts(in, g) {
			if strings.Contains(v, "R=301") {
				t.Errorf("%s: 301 redirect", d)
			}
		}
	})
	v := string(mustRender(t, debianStandard()).File("sites/forced.test.conf"))
	if !strings.Contains(v, "    RewriteRule ^/(.*) https://%{HTTP_HOST}%{REQUEST_URI} [R=307,L]\n") {
		t.Fatalf("forced vhost without the 307 rule:\n%s", v)
	}
	v = string(mustRender(t, fedoraHigh()).File("sites/app.test.conf"))
	if !strings.Contains(v, "    RewriteRule ^/(.*) https://%{SERVER_NAME}:8443%{REQUEST_URI} [R=307,L]\n") {
		t.Fatalf("high-port forced vhost does not redirect to 8443:\n%s", v)
	}
	v = string(mustRender(t, macosPF()).File("sites/mac.test.conf"))
	if !strings.Contains(v, "https://%{HTTP_HOST}%{REQUEST_URI} [R=307,L]") {
		t.Fatalf("pf-redirected vhost does not redirect to 443:\n%s", v)
	}
}

// Overlays are included, never written: changing any field of a site
// changes rendered files only, inside the generation directory.
func TestLedger_HTE11(t *testing.T) {
	root := t.TempDir()
	in := debianStandard()
	in.Paths = paths(root, root+"/run")
	in.Paths.StateDir = root + "/state"
	layout := Layout{Dir: in.Paths.RenderDir()}
	overlays := map[string][]byte{
		in.Paths.SiteOverlay("shop.test"): []byte("Header set X-Shop 1\n"),
		in.Paths.ApacheOverlay():          []byte("LogLevel info\n"),
		in.Paths.PHPOverlay("8.3"):        []byte("memory_limit = 768M\n"),
	}
	for p, data := range overlays {
		must(t, os.MkdirAll(filepath.Dir(p), 0o700))
		must(t, os.WriteFile(p, data, 0o600))
	}
	mods := []func(*state.Site){
		func(s *state.Site) { s.Aliases = []string{"m.shop.test"} },
		func(s *state.Site) { s.Docroot = "/srv/shop" },
		func(s *state.Site) { s.PHP = "8.4" },
		func(s *state.Site) { s.Profile = string(profiles.Hardened) },
		func(s *state.Site) { s.HTTPS = false },
		func(s *state.Site) { s.HTTPS, s.ForceHTTPS = true, true },
		func(s *state.Site) { s.ForceHTTPS, s.ForceHTTPSBypassLoopback = true, true },
		func(s *state.Site) { s.HTTP2 = false },
		func(s *state.Site) { s.PHPIni = map[string]string{"upload_max_filesize": "1G"} },
		func(s *state.Site) { s.Enabled = false },
	}
	for i, mod := range mods {
		mod(&in.Sites[0])
		n, err := layout.Next()
		must(t, err)
		in.GenDir = layout.GenDir(n)
		in.Overlays, err = ReadOverlays(in.Paths, in.Sites)
		must(t, err)
		g := mustRender(t, in)
		_, err = layout.Write(n, g)
		must(t, err)
		must(t, layout.Swap(n))
		for p, data := range overlays {
			got, err := os.ReadFile(p)
			must(t, err)
			if !bytes.Equal(got, data) {
				t.Fatalf("modification %d changed %s", i, p)
			}
		}
		if s := in.Sites[0]; s.Enabled {
			vhost := string(g.File(siteConfRel(s.Domain)))
			want := 1
			if s.HTTPS {
				want = 2
			}
			if got := strings.Count(vhost, `IncludeOptional "`+in.abs(siteOverlayRel(s.Domain))+`"`); got != want {
				t.Fatalf("modification %d: site overlay included %d times, want %d", i, got, want)
			}
			if got := strings.Count(vhost, `# Site overlay: "`+in.Paths.SiteOverlay(s.Domain)+`"`); got != want {
				t.Fatalf("modification %d: site overlay named %d times, want %d", i, got, want)
			}
			if !bytes.Equal(g.File(siteOverlayRel(s.Domain)), overlays[in.Paths.SiteOverlay(s.Domain)]) {
				t.Fatalf("modification %d: no copy of the site overlay", i)
			}
		} else if g.File(siteOverlayRel(in.Sites[0].Domain)) != nil {
			t.Fatalf("modification %d: a disabled site's overlay was copied", i)
		}
		httpd := string(g.File("httpd.conf"))
		if !strings.Contains(httpd, `IncludeOptional "`+in.abs(globalOverlayRel)+`"`) ||
			!bytes.Equal(g.File(globalOverlayRel), overlays[in.Paths.ApacheOverlay()]) {
			t.Fatalf("modification %d: the global overlay copy is not included", i)
		}
	}
}

// TestOverlayCopies: a generation holds a copy of each overlay that
// exists and includes the copies, never the user's files; a missing
// overlay has no copy, a raw site none at all (LEDGER HTE-71).
func TestOverlayCopies(t *testing.T) {
	in := debianStandard()
	in.Overlays = map[string][]byte{
		in.Paths.SiteOverlay("forced.test"): []byte("Listen 127.0.0.1:3001\n"),
		in.Paths.SiteOverlay("proxy.test"):  []byte("Header set X-Raw 1\n"),
		in.Paths.SiteOverlay("off.test"):    []byte("Header set X-Off 1\n"),
	}
	g := mustRender(t, in)
	var copies []string
	for _, f := range g.Files {
		if strings.HasPrefix(f.Path, "overlays/") {
			copies = append(copies, f.Path)
		}
	}
	if !slices.Equal(copies, []string{"overlays/sites/forced.test.apache.conf"}) {
		t.Fatalf("copies %v", copies)
	}
	for _, f := range g.Files {
		for _, user := range []string{in.Paths.ApacheOverlay(), in.Paths.SiteOverlay("forced.test"), in.Paths.SiteOverlay("shop.test")} {
			if strings.Contains(string(f.Data), "IncludeOptional \""+user+"\"") {
				t.Errorf("%s includes the user's %s", f.Path, user)
			}
		}
	}
	if strings.Contains(string(g.File("sites/proxy.test.conf")), "IncludeOptional") {
		t.Error("the raw site includes an overlay")
	}
}

// ReadOverlays reads the overlays that exist, refuses one that is not a
// regular file, and never writes them.
func TestReadOverlays(t *testing.T) {
	root := t.TempDir()
	p := paths(root, root+"/run")
	sites := []state.Site{site("a.test", nil), site("b.test", nil), site("off.test", func(s *state.Site) { s.Enabled = false }),
		{Domain: "raw.test", Kind: state.KindRaw, Enabled: true}}
	must(t, os.MkdirAll(p.SitesDir(), 0o700))
	must(t, os.WriteFile(p.SiteOverlay("a.test"), []byte("Header set X-A 1\n"), 0o400))
	must(t, os.WriteFile(p.SiteOverlay("off.test"), []byte("x\n"), 0o600))
	must(t, os.WriteFile(p.SiteOverlay("raw.test"), []byte("x\n"), 0o600))
	must(t, os.WriteFile(p.ApacheOverlay(), nil, 0o600))
	got, err := ReadOverlays(p, sites)
	must(t, err)
	want := map[string][]byte{p.ApacheOverlay(): {}, p.SiteOverlay("a.test"): []byte("Header set X-A 1\n")}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
	must(t, os.Mkdir(p.SiteOverlay("b.test"), 0o700))
	if _, err := ReadOverlays(p, sites); err == nil || !strings.Contains(err.Error(), "not a regular file") {
		t.Fatalf("a directory overlay: %v", err)
	}
}

func TestLedger_HTE13(t *testing.T) {
	in := debianStandard()
	in.Sites = in.Sites[:3]
	before := mustRender(t, in)
	in.Sites = in.Sites[1:]
	after := mustRender(t, in)
	for _, s := range in.Sites {
		for _, rel := range []string{siteConfRel(s.Domain), poolRel(s.PHP, s.Domain)} {
			if b, a := before.File(rel), after.File(rel); a == nil || !bytes.Equal(a, b) {
				t.Errorf("%s changed after removing another site", rel)
			}
		}
	}
	for _, f := range after.Files {
		if regexp.MustCompile(`\d{3}-`).MatchString(f.Path) {
			t.Errorf("numbered file %s", f.Path)
		}
	}
	// Include order is the sorted domain list.
	httpd := string(before.File("httpd.conf"))
	if !(strings.Index(httpd, "bypass.test") < strings.Index(httpd, "forced.test") &&
		strings.Index(httpd, "forced.test") < strings.Index(httpd, "shop.test")) {
		t.Fatalf("includes are not sorted:\n%s", httpd)
	}
}

func TestLedger_HTE15(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for d, v := range managedVhosts(in, g) {
			for _, bad := range []string{"SSLCipherSuite", "SSLProtocol", "SSLHonorCipherOrder"} {
				if strings.Contains(v, bad) {
					t.Errorf("%s: %s in a managed vhost", d, bad)
				}
			}
		}
		httpd := string(g.File("httpd.conf"))
		for _, want := range []string{
			"SSLProtocol all -SSLv3 -TLSv1 -TLSv1.1\n",
			"SSLCipherSuite HIGH:!aNULL\n",
			"SSLHonorCipherOrder off\n",
			`SSLSessionCache "shmcb:` + in.Paths.RuntimeDir + `/ssl_scache(512000)"` + "\n",
			"SSLSessionCacheTimeout 300\n",
		} {
			if !strings.Contains(httpd, want) {
				t.Errorf("httpd.conf lacks %q", want)
			}
		}
	})
}

func TestLedger_HTE16(t *testing.T) {
	legacy := regexp.MustCompile(`(?im)^\s*(order\s|allow\s+from|deny\s+from)`)
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for d, v := range managedVhosts(in, g) {
			if m := legacy.FindString(v); m != "" {
				t.Errorf("%s: %q", d, m)
			}
			if !strings.Contains(v, "Require all granted") {
				t.Errorf("%s: no Require all granted", d)
			}
		}
	})
}

func TestLedger_HTE18(t *testing.T) {
	owner := regexp.MustCompile(`(?m)^\s*(user|group|listen\.owner|listen\.group)\s*=`)
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for p, pool := range pools(g) {
			if m := owner.FindString(pool); m != "" {
				t.Errorf("%s: %q", p, m)
			}
			if !strings.Contains(pool, "\nlisten.mode = 0660\n") {
				t.Errorf("%s: no listen.mode = 0660", p)
			}
		}
	})
}

func TestLedger_HTE34(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		shuffled := in
		shuffled.Sites = slices.Clone(in.Sites)
		slices.Reverse(shuffled.Sites)
		for range 3 {
			if diff := cmp.Diff(g, mustRender(t, shuffled)); diff != "" {
				t.Fatalf("rendering is not deterministic:\n%s", diff)
			}
		}
		date := regexp.MustCompile(`20\d\d-\d\d-\d\d`)
		for _, f := range g.Files {
			if date.Match(f.Data) {
				t.Errorf("%s holds a date", f.Path)
			}
		}
		for d, v := range managedVhosts(in, g) {
			if !strings.Contains(v, "    # Managed by gash hte. Edit the site with `gash hte site modify`, or add directives to the overlay named below.\n") {
				t.Errorf("%s: no managed header", d)
			}
		}
		for p, pool := range pools(g) {
			if !regexp.MustCompile(`^\[[a-z0-9.-]+\]\n; Managed by gash hte\. Profile: (dev|staging|hardened)\n`).MatchString(pool) {
				t.Errorf("%s: no managed header", p)
			}
		}
	})
}

func TestLedger_HTE35(t *testing.T) {
	label := strings.Repeat("a", 63)
	domain := label + "." + label + "." + label + "." + strings.Repeat("b", 61)
	if len(domain) != 253 {
		t.Fatalf("domain is %d bytes", len(domain))
	}
	run := "/run/user/1000/" + strings.Repeat("r", 60-len("/run/user/1000/"))
	if len(run) != 60 {
		t.Fatalf("runtime dir is %d bytes", len(run))
	}
	sock := SocketPath(run, "8.3", domain)
	if len(sock) > MaxSocketPath || strings.Contains(sock, "aaa") {
		t.Fatalf("socket %s (%d bytes)", sock, len(sock))
	}
	if !regexp.MustCompile(`/8\.3-[0-9a-f]{10}\.sock$`).MatchString(sock) {
		t.Fatalf("socket name %s", sock)
	}
	in := debianStandard()
	in.Paths.RuntimeDir = run
	in.Sites = []state.Site{site(domain, func(s *state.Site) { s.Aliases = []string{} })}
	g := mustRender(t, in)
	if !strings.Contains(string(g.File(siteConfRel(domain))), `SetHandler "proxy:unix:`+sock+`|fcgi://localhost/"`) {
		t.Fatal("vhost does not name the socket")
	}
	if !strings.Contains(string(g.File(poolRel("8.3", domain))), "\nlisten = \""+sock+"\"\n") {
		t.Fatal("pool does not name the socket")
	}
	in.Paths.RuntimeDir = "/" + strings.Repeat("x", 90)
	if _, err := Render(in); code(err) != hteerr.PlatformUnsupported {
		t.Fatalf("long runtime dir: %v", err)
	}
}

func TestLedger_HTE36(t *testing.T) {
	listen := regexp.MustCompile(`(?m)^Listen (.*)$`)
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		got := listen.FindAllStringSubmatch(string(g.File("httpd.conf")), -1)
		h, hs := extraPorts(in.Sites)
		if len(got) != (2+len(h)+len(hs))*len(in.Listen) {
			t.Fatalf("Listen lines: %v", got)
		}
		for _, m := range got {
			if !regexp.MustCompile(`^(127\.0\.0\.1|\[::1\]):\d+( https)?$`).MatchString(m[1]) {
				t.Errorf("non-loopback Listen %s", m[1])
			}
		}
	})
}

func TestLedger_HTE37(t *testing.T) {
	v6 := string(mustRender(t, debianStandard()).File("sites/shop.test.conf"))
	for _, want := range []string{"<VirtualHost 127.0.0.1:80 [::1]:80>", "<VirtualHost 127.0.0.1:443 [::1]:443>"} {
		if !strings.Contains(v6, want) {
			t.Errorf("IPv6 platform: missing %s", want)
		}
	}
	v4 := string(mustRender(t, fedoraHigh()).File("sites/app.test.conf"))
	for _, want := range []string{"<VirtualHost 127.0.0.1:8080>", "<VirtualHost 127.0.0.1:8443>"} {
		if !strings.Contains(v4, want) {
			t.Errorf("IPv4-only platform: missing %s", want)
		}
	}
	if strings.Contains(v4, "[::1]") {
		t.Error("IPv4-only platform binds [::1]")
	}
}

func TestLedger_HTE38(t *testing.T) {
	g := mustRender(t, debianStandard())
	if !strings.Contains(string(g.File("httpd.conf")), "\nProtocols h2 h2c http/1.1\n") {
		t.Error("server Protocols")
	}
	on := strings.Split(string(g.File("sites/shop.test.conf")), "<VirtualHost 127.0.0.1:443")
	if !strings.Contains(on[0], "    Protocols h2c http/1.1\n") || !strings.Contains(on[1], "    Protocols h2 http/1.1\n") {
		t.Error("http2 = true")
	}
	off := string(g.File("sites/bypass.test.conf"))
	if strings.Count(off, "    Protocols http/1.1\n") != 2 || strings.Contains(off, "h2") {
		t.Error("http2 = false")
	}
	plain := string(g.File("sites/plain.test.conf"))
	if strings.Count(plain, "    Protocols h2c http/1.1\n") != 1 || strings.Contains(plain, "SSLEngine") {
		t.Error("http-only site")
	}
}

func TestLedger_HTE41(t *testing.T) {
	for p, pool := range pools(mustRender(t, debianStandard())) {
		if strings.Contains(pool, "session.save_path") {
			t.Errorf("Debian %s sets session.save_path", p)
		}
	}
	in := fedoraHigh()
	g := mustRender(t, in)
	dir := in.PHP["8.3"].SessionDir
	for _, s := range in.Sites {
		pool := string(g.File(poolRel(s.PHP, s.Domain)))
		if !strings.Contains(pool, "\nphp_admin_value[session.save_path] = \""+dir+"\"\n") {
			t.Errorf("%s: no session.save_path", s.Domain)
		}
		basedir := regexp.MustCompile(`(?m)^php_admin_value\[open_basedir\] = "(.*)"$`).FindStringSubmatch(pool)
		switch profiles.Name(s.Profile) {
		case profiles.Dev:
			if basedir != nil {
				t.Errorf("%s: dev sets open_basedir", s.Domain)
			}
		default:
			if basedir == nil || !strings.HasSuffix(basedir[1], ":"+dir) {
				t.Errorf("%s: session dir not in open_basedir: %v", s.Domain, basedir)
			}
		}
	}
}

func TestLedger_HTE42(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for _, s := range in.Sites {
			if !s.Enabled || !s.HasPool() || s.Profile == string(profiles.Dev) {
				continue
			}
			want := `php_admin_value[open_basedir] = "` + s.Docroot + ":" + profiles.DefaultOpenBasedir
			if d := in.PHP[s.PHP].SessionDir; d != "" {
				want += ":" + d
			}
			if !strings.Contains(string(g.File(poolRel(s.PHP, s.Domain))), want+"\"\n") {
				t.Errorf("%s: want %s", s.Domain, want)
			}
		}
	})
}

func TestLedger_HTE43(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for p, pool := range pools(g) {
			if !strings.Contains(pool, "\nslowlog = ") || strings.Contains(pool, "request_slowlog_timeout") {
				t.Errorf("%s: slowlog lines differ from hte-cli", p)
			}
		}
	})
}

func TestLedger_HTE44(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		ps := pools(g)
		if len(ps) == 0 {
			t.Fatal("no pools")
		}
		for p, pool := range ps {
			if strings.Count(pool, "\nsecurity.limit_extensions = .php\n") != 1 {
				t.Errorf("%s: no security.limit_extensions", p)
			}
		}
	})
}

func TestLedger_HTE48(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		for _, f := range g.Files {
			if f.Path != "sites/proxy.test.conf" && bytes.Contains(f.Data, []byte("${")) {
				t.Errorf("%s holds ${", f.Path)
			}
		}
		if !strings.Contains(string(g.File("httpd.conf")), "ErrorLog \""+in.Paths.LogsDir()+"/error.log\"\nLogLevel warn\n") {
			t.Error("global error log")
		}
		for d, v := range managedVhosts(in, g) {
			logs := in.Paths.SiteLogDir(d)
			want := []string{
				`ErrorLog "` + logs + `/error.log"`,
				`CustomLog "` + logs + `/access.log" combined`,
				`CustomLog "` + logs + `/bytes.log" "%{%s}t %I .\n%{%s}t %O ."`,
			}
			for _, s := range in.Sites {
				if s.Domain == d && s.HTTPS {
					want = append(want,
						`CustomLog "`+logs+`/ssl-access.log" combined`,
						`CustomLog "`+logs+`/ssl-bytes.log" "%t %h %{SSL_PROTOCOL}x %{SSL_CIPHER}x \"%r\" %b"`)
				}
			}
			for _, w := range want {
				if !strings.Contains(v, "    "+w+"\n") {
					t.Errorf("%s: missing %s", d, w)
				}
			}
		}
	})
}

func TestLedger_HTE52(t *testing.T) {
	g := mustRender(t, debianStandard())
	bypass := []string{
		"    RewriteCond %{REMOTE_ADDR} !^127\\.0\\.0\\.1$\n",
		"    RewriteCond %{REMOTE_ADDR} !^::1$\n",
	}
	forced := string(g.File("sites/forced.test.conf"))
	withBypass := string(g.File("sites/bypass.test.conf"))
	for _, c := range bypass {
		if strings.Contains(forced, c) {
			t.Errorf("bypass off: %q present", c)
		}
		if !strings.Contains(withBypass, c) {
			t.Errorf("bypass on: %q missing", c)
		}
	}
	for d, v := range map[string]string{"forced": forced, "bypass": withBypass} {
		if !strings.Contains(v, "    RewriteEngine On\n    RewriteCond %{HTTPS} off\n") {
			t.Errorf("%s: no rewrite block", d)
		}
		https := v[strings.Index(v, "<VirtualHost 127.0.0.1:443"):]
		if strings.Contains(https, "Rewrite") {
			t.Errorf("%s: rewrite in the HTTPS vhost", d)
		}
	}
	for _, d := range []string{"shop.test", "plain.test"} {
		if strings.Contains(string(g.File(siteConfRel(d))), "Rewrite") {
			t.Errorf("%s: not forced but has a rewrite", d)
		}
	}
}

func TestLedger_HTE59(t *testing.T) {
	allCases(t, func(t *testing.T, in Input, g *Generation) {
		httpd := string(g.File("httpd.conf"))
		for _, want := range []string{
			"<Directory \"/\">\n    AllowOverride None\n    Require all denied\n</Directory>\n",
			"<FilesMatch \"^\\.ht\">\n    Require all denied\n</FilesMatch>\n",
			"DirectoryIndex index.html index.cgi index.pl index.php index.xhtml index.htm\n",
			"ServerTokens OS\nServerSignature On\nTraceEnable Off\n",
			"TypesConfig \"" + in.Apache.MimeTypes + "\"\n",
			"AddDefaultCharset Off\n",
			"PidFile \"" + in.Paths.RuntimeDir + "/httpd.pid\"\n",
			"Mutex \"file:" + in.Paths.RuntimeDir + "\" default\n",
		} {
			if !strings.Contains(httpd, want) {
				t.Errorf("httpd.conf lacks %q", want)
			}
		}
		if got := strings.Count(httpd, "\nLoadModule "); got != len(in.Apache.Modules) {
			t.Errorf("%d LoadModule lines for %d modules", got, len(in.Apache.Modules))
		}
	})
	httpd := string(mustRender(t, debianStandard()).File("httpd.conf"))
	for _, builtin := range []string{"log_config_module", "logio_module", "unixd_module"} {
		if strings.Contains(httpd, builtin) {
			t.Errorf("Debian: LoadModule for built-in %s", builtin)
		}
	}
	if !strings.Contains(string(mustRender(t, fedoraHigh()).File("httpd.conf")), "LoadModule log_config_module \"/usr/lib64/httpd/modules/mod_log_config.so\"\n") {
		t.Error("Fedora: no LoadModule for log_config")
	}
}

// TestPoolPHPIni: a php_ini key the profile sets replaces the profile's
// lines with the same admin kind; other keys are php_value or php_flag; a
// boolean word is a bare flag, any other value is double-quoted. The global
// overlay is never in a pool.
// TestExtraListen: extra ports become Listen lines on loopback (ascending,
// once, TLS ones naming https) and addresses of the site's vhosts only.
func TestExtraListen(t *testing.T) {
	g := mustRender(t, debianStandard())
	httpd := string(g.File("httpd.conf"))
	want := "Listen 127.0.0.1:80\nListen [::1]:80\nListen 127.0.0.1:3000\nListen [::1]:3000\nListen 127.0.0.1:8081\nListen [::1]:8081\n" +
		"Listen 127.0.0.1:443\nListen [::1]:443\nListen 127.0.0.1:3443 https\nListen [::1]:3443 https\n"
	if !strings.Contains(httpd, want) {
		t.Fatalf("Listen lines:\n%s", httpd)
	}
	forced := string(g.File("sites/forced.test.conf"))
	if !strings.Contains(forced, "<VirtualHost 127.0.0.1:80 [::1]:80 127.0.0.1:3000 [::1]:3000>\n") ||
		!strings.Contains(forced, "<VirtualHost 127.0.0.1:443 [::1]:443 127.0.0.1:3443 [::1]:3443>\n") {
		t.Fatalf("forced vhost:\n%s", forced)
	}
	if strings.Contains(string(g.File("sites/shop.test.conf")), "3000") {
		t.Fatal("another site's extra port leaked")
	}
	// A disabled site's ports are not listened on; a port of the stack is refused.
	in := debianStandard()
	in.Sites[1].Enabled = false
	if strings.Contains(string(mustRender(t, in).File("httpd.conf")), ":3443") {
		t.Fatal("disabled site's port listened")
	}
	in = fedoraHigh()
	in.Sites[0].ExtraHTTPPorts = []int{8443}
	if _, err := Render(in); code(err) != hteerr.ConflictingOptions {
		t.Fatalf("stack port: %v", err)
	}
	in = debianStandard()
	in.Sites[0].ExtraHTTPSPorts = []int{3000}
	if _, err := Render(in); code(err) != hteerr.ConflictingOptions {
		t.Fatalf("protocol clash: %v", err)
	}
}

func TestPoolPHPIni(t *testing.T) {
	pool := string(mustRender(t, debianStandard()).File("fpm/8.3/pools/shop.test.conf"))
	for _, want := range []string{
		"\nphp_flag[display_errors] = on\n",
		"\nphp_admin_value[memory_limit] = \"512M\"\nphp_value[memory_limit] = \"512M\"\n",
	} {
		if !strings.Contains(pool, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, gone := range []string{"1024M", "php_flag[display_errors] = off", "Global overlay", "php.ini"} {
		if strings.Contains(pool, gone) {
			t.Errorf("pool still has %q", gone)
		}
	}
	want := `
; php_ini of /home/me/.config/gash/hte/sites/shop.test.toml
php_value[auto_prepend_file] = ""
php_value[opcache.enable] = "0"
php_value[session.name] = "S;ID"
`
	if !strings.HasSuffix(pool, want) {
		t.Fatalf("pool tail:\n%s", pool)
	}
	// hardened sets auto_prepend_file as an admin value: replaced in place.
	in := debianStandard()
	in.Sites[2].PHPIni = map[string]string{"auto_prepend_file": "/srv/prepend.php", "expose_php": "Yes"}
	hardened := string(mustRender(t, in).File("fpm/8.3/pools/bypass.test.conf"))
	if !strings.Contains(hardened, "\nphp_admin_value[auto_prepend_file] = \"/srv/prepend.php\"\nphp_admin_value[auto_append_file] = \"\"\nphp_admin_flag[expose_php] = yes\n") ||
		strings.Contains(hardened, "; php_ini of") {
		t.Fatalf("hardened pool:\n%s", hardened)
	}
	// The session directory line is replaced by a session.save_path value.
	fin := fedoraHigh()
	fin.Sites[2].PHPIni = map[string]string{"session.save_path": "/home/dev/sessions"}
	if p := string(mustRender(t, fin).File("fpm/8.3/pools/dev.test.conf")); !strings.Contains(p,
		"\nphp_admin_value[session.save_path] = \"/home/dev/sessions\"\n") || strings.Count(p, "session.save_path") != 2 {
		t.Fatalf("session override:\n%s", p)
	}
}

// TestManifest: the generation says how to run it: binaries, php.ini, the
// full PHP_INI_SCAN_DIR, PID files, sockets and site addresses.
func TestManifest(t *testing.T) {
	in := debianStandard()
	g := mustRender(t, in)
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, ManifestFile), g.File(ManifestFile), 0o600))
	m, err := ReadManifest(dir)
	must(t, err)
	if m.Apache.Binary != "/usr/sbin/apache2" || m.Apache.Config != "httpd.conf" || len(m.Apache.Listen) != 10 {
		t.Fatalf("apache: %+v", m.Apache)
	}
	f, ok := m.Master("8.3")
	if !ok || f.Binary != "/usr/sbin/php-fpm8.3" || f.INI != "/etc/php/8.3/fpm/php.ini" ||
		f.Env()[0] != "PHP_INI_SCAN_DIR=/etc/php/8.3/fpm/conf.d:/home/me/.config/gash/hte/php/8.3" ||
		f.Config != "fpm/8.3/php-fpm.conf" || len(f.Sockets) != 3 {
		t.Fatalf("8.3: %+v", f)
	}
	if diff := cmp.Diff([]string{"8.3", "8.4"}, m.Versions()); diff != "" {
		t.Fatal(diff)
	}
	if len(m.Sites) != 5 || m.Sites[0].Domain != "bypass.test" {
		t.Fatalf("sites: %+v", m.Sites)
	}
	if e := m.Sites[1]; e.Domain != "forced.test" || len(e.HTTP) != 4 || e.HTTPS[3] != "[::1]:3443" {
		t.Fatalf("forced.test endpoint: %+v", e)
	}
	// No system scan directory: only the overlay directory.
	if v := ScanDirEnv(in.Paths, PHPVersion{}, "8.4"); v != "/home/me/.config/gash/hte/php/8.4" {
		t.Fatal(v)
	}
	must(t, os.WriteFile(filepath.Join(dir, ManifestFile), []byte(`{"schema":9}`), 0o600))
	if _, err := ReadManifest(dir); code(err) != hteerr.InvalidState {
		t.Fatalf("foreign schema: %v", err)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
