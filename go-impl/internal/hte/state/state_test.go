// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package state

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/platform"
)

func ptr[T any](v T) *T { return &v }

type fixture struct {
	t     *testing.T
	root  string // a temporary home
	env   Env
	store Store
	pub   string // an existing docroot
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	pub := filepath.Join(root, "src", "shop", "public")
	must(t, os.MkdirAll(pub, 0o755))
	dirs, err := platform.ResolveDirs(func(k string) string {
		return map[string]string{"HOME": root, "XDG_RUNTIME_DIR": "/run/user/1000"}[k]
	})
	must(t, err)
	return &fixture{t: t, root: root, pub: pub,
		env:   Env{Cwd: filepath.Join(root, "src", "shop"), Installed: []string{"7.4", "8.3", "8.1"}},
		store: Store{Paths: NewPaths(dirs, 1000)}}
}

func (f *fixture) create(fl Flags, existing ...Site) Site {
	f.t.Helper()
	s, err := NewSite(fl, f.env, existing)
	must(f.t, err)
	must(f.t, f.store.SaveSite(s))
	return s
}

func code(err error) hteerr.Code { return hteerr.CodeOf(err) }

func TestPaths(t *testing.T) {
	d := platform.Dirs{Home: "/home/me", ConfigHome: "/home/me/.config", StateHome: "/home/me/.local/state",
		DataHome: "/home/me/.local/share", RuntimeDir: "/run/user/1000"}
	p := NewPaths(d, 1000)
	got := []string{p.Settings(), p.SiteFile("shop.test"), p.SiteOverlay("shop.test"), p.ApacheOverlay(),
		p.PHPOverlay("8.3"), p.RenderDir(), p.SiteLogDir("shop.test"), p.CertFile("shop.test"), p.KeyFile("shop.test"),
		p.SessionDir("8.3"), p.Receipt(), p.Audit(), p.Lock(), p.RuntimeDir, p.Stage("20261018T100000Z")}
	want := []string{"/home/me/.config/gash/hte/hte.toml", "/home/me/.config/gash/hte/sites/shop.test.toml",
		"/home/me/.config/gash/hte/sites/shop.test.apache.conf", "/home/me/.config/gash/hte/apache.conf",
		"/home/me/.config/gash/hte/php/8.3/global.ini", "/home/me/.local/state/gash/hte/render",
		"/home/me/.local/state/gash/hte/logs/shop.test", "/home/me/.local/share/gash/hte/certs/shop.test/cert.pem",
		"/home/me/.local/share/gash/hte/certs/shop.test/key.pem", "/home/me/.local/state/gash/hte/php/8.3/sessions",
		"/home/me/.local/state/gash/hte/receipt.json", "/home/me/.local/state/gash/hte/audit.jsonl",
		"/home/me/.local/state/gash/hte/lock", "/run/user/1000/gash-hte", "/home/me/.local/state/gash/hte-stage-20261018T100000Z"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatal(diff)
	}
	d.RuntimeDir = ""
	if NewPaths(d, 1000).RuntimeDir != "/tmp/gash-hte-1000" {
		t.Fatal("runtime fallback")
	}
}

// TestLedger_HTE01: every combination of the HTTPS flag pairs gives the
// TOML values it names; force-HTTPS without HTTPS is refused.
func TestLedger_HTE01(t *testing.T) {
	f := newFixture(t)
	bools := []*bool{nil, ptr(true), ptr(false)}
	for _, https := range bools {
		for _, force := range bools {
			for _, bypass := range bools {
				fl := Flags{Domain: "a.test", Docroot: ptr(f.pub), HTTPS: https, ForceHTTPS: force, ForceHTTPSBypass: bypass}
				s, err := NewSite(fl, f.env, nil)
				wantHTTPS := https == nil || *https
				wantForce := force != nil && *force
				if !wantHTTPS && wantForce {
					if code(err) != hteerr.ConflictingOptions {
						t.Errorf("https=%v force=%v: %v", deref(https), deref(force), err)
					}
					continue
				}
				must(t, err)
				if s.HTTPS != wantHTTPS || s.ForceHTTPS != wantForce || s.ForceHTTPSBypassLoopback != (bypass != nil && *bypass) {
					t.Errorf("https=%v force=%v bypass=%v: got %+v", deref(https), deref(force), deref(bypass), s)
				}
			}
		}
	}
	// modify --no-https on a forced site needs --no-force-https.
	cur, err := NewSite(Flags{Domain: "a.test", Docroot: ptr(f.pub), ForceHTTPS: ptr(true)}, f.env, nil)
	must(t, err)
	if _, _, err := ModifySite(cur, Flags{Domain: "a.test", HTTPS: ptr(false)}, f.env, []Site{cur}); code(err) != hteerr.ConflictingOptions {
		t.Fatalf("modify --no-https: %v", err)
	}
	next, changed, err := ModifySite(cur, Flags{Domain: "a.test", HTTPS: ptr(false), ForceHTTPS: ptr(false)}, f.env, []Site{cur})
	if err != nil || !changed || next.HTTPS || next.ForceHTTPS {
		t.Fatalf("modify --no-https --no-force-https: %+v %v %v", next, changed, err)
	}
}

func deref(b *bool) any {
	if b == nil {
		return "absent"
	}
	return *b
}

// TestLedger_HTE02: each invalid value fails with its code, in argument
// order, and nothing is written.
func TestLedger_HTE02(t *testing.T) {
	f := newFixture(t)
	existing := f.create(Flags{Domain: "taken.test", Docroot: ptr(f.pub)})
	file := filepath.Join(f.root, "file")
	must(t, os.WriteFile(file, nil, 0o644))
	before := snapshot(t, f.root)
	tests := []struct {
		name string
		fl   Flags
		want hteerr.Code
	}{
		{"bad domain first", Flags{Domain: "bad_domain", Docroot: ptr("/nonexistent"), PHP: ptr("9.9"), Profile: ptr("x")}, hteerr.InvalidDomain},
		{"missing docroot", Flags{Domain: "a.test", Docroot: ptr("/nonexistent"), PHP: ptr("9.9"), Profile: ptr("x")}, hteerr.DocrootNotFound},
		{"docroot is a file", Flags{Domain: "a.test", Docroot: ptr(file)}, hteerr.InvalidDocroot},
		{"php before profile", Flags{Domain: "a.test", Docroot: ptr(f.pub), PHP: ptr("9.9"), Profile: ptr("x")}, hteerr.PHPVersionNotInstalled},
		{"php cli only", Flags{Domain: "a.test", Docroot: ptr(f.pub), PHP: ptr("8.4")}, hteerr.PHPVersionNotInstalled},
		{"profile", Flags{Domain: "a.test", Docroot: ptr(f.pub), Profile: ptr("custom")}, hteerr.InvalidProfile},
		{"profile case", Flags{Domain: "a.test", Docroot: ptr(f.pub), Profile: ptr("Dev")}, hteerr.InvalidProfile},
		{"bad alias", Flags{Domain: "a.test", Docroot: ptr(f.pub), Aliases: []string{"*.a.test"}}, hteerr.InvalidDomain},
		{"site exists last", Flags{Domain: "taken.test", Docroot: ptr(f.pub)}, hteerr.SiteExists},
		{"alias taken", Flags{Domain: "a.test", Docroot: ptr(f.pub), Aliases: []string{"www.taken.test"}}, hteerr.SiteExists},
		{"no domain", Flags{Docroot: ptr(f.pub)}, hteerr.MissingValue},
		{"no docroot", Flags{Domain: "a.test"}, hteerr.MissingValue},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewSite(tt.fl, f.env, []Site{existing})
			if code(err) != tt.want {
				t.Fatalf("got %v, want %s", err, tt.want)
			}
			e, _ := hteerr.As(err)
			if e.Action() != hteerr.Retry {
				t.Fatalf("action %s", e.Action())
			}
		})
	}
	if tests[3].want == hteerr.PHPVersionNotInstalled {
		_, err := NewSite(tests[3].fl, f.env, nil)
		if e, _ := hteerr.As(err); !strings.Contains(e.Hint, "7.4, 8.1, 8.3") {
			t.Fatalf("hint %q", e.Hint)
		}
	}
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("files changed:\n%s", diff)
	}
}

// TestLedger_HTE04: removing or loading a typo is site_not_found and both
// existing sites stay byte-identical.
func TestLedger_HTE04(t *testing.T) {
	f := newFixture(t)
	one := f.create(Flags{Domain: "one.test", Docroot: ptr(f.pub)})
	f.create(Flags{Domain: "two.test", Docroot: ptr(f.pub)}, one)
	before := snapshot(t, f.root)
	if err := f.store.RemoveSite("typo.test"); code(err) != hteerr.SiteNotFound {
		t.Fatalf("remove: %v", err)
	}
	if _, err := f.store.LoadSite("typo.test"); code(err) != hteerr.SiteNotFound {
		t.Fatalf("load: %v", err)
	}
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("files changed:\n%s", diff)
	}
}

// TestLedger_HTE05: a TOML whose domain differs from its file name is
// refused; LoadSites reports it and keeps the others.
func TestLedger_HTE05(t *testing.T) {
	f := newFixture(t)
	good := f.create(Flags{Domain: "good.test", Docroot: ptr(f.pub)})
	data, err := EncodeSite(good)
	must(t, err)
	must(t, os.WriteFile(f.store.Paths.SiteFile("other.test"), data, 0o600))
	if _, err := f.store.LoadSite("other.test"); code(err) != hteerr.InvalidState {
		t.Fatalf("mismatch: %v", err)
	}
	must(t, os.WriteFile(filepath.Join(f.store.Paths.SitesDir(), "Bad_Name.toml"), data, 0o600))
	sites, problems, err := f.store.LoadSites()
	must(t, err)
	if len(sites) != 1 || sites[0].Domain != "good.test" || len(problems) != 2 {
		t.Fatalf("sites %v problems %v", sites, problems)
	}
}

// TestLedger_HTE20: domains are lowercased; App.test and app.test are one site.
func TestLedger_HTE20(t *testing.T) {
	f := newFixture(t)
	s := f.create(Flags{Domain: "Shop.Test", Docroot: ptr(f.pub)})
	if s.Domain != "shop.test" || s.Aliases[0] != "www.shop.test" {
		t.Fatalf("%+v", s)
	}
	loaded, err := f.store.LoadSite("SHOP.test")
	if err != nil || loaded.Domain != "shop.test" {
		t.Fatalf("load: %v %v", loaded, err)
	}
	if _, err := NewSite(Flags{Domain: "shop.test", Docroot: ptr(f.pub)}, f.env, []Site{s}); code(err) != hteerr.SiteExists {
		t.Fatalf("duplicate: %v", err)
	}
	for _, d := range []string{"http://shop.test", "shop.test/", "shop.test:8080", "shop.test."} {
		if _, err := NormalizeDomain(d); code(err) != hteerr.InvalidDomain {
			t.Errorf("%q: %v", d, err)
		}
	}
}

// TestLedger_HTE21: IP addresses are not site names.
func TestLedger_HTE21(t *testing.T) {
	for _, d := range []string{"127.0.0.1", "::1", "[::1]", "10.0.1", "192.168.1.10"} {
		_, err := NormalizeDomain(d)
		if e, ok := hteerr.As(err); !ok || e.Code != hteerr.InvalidDomain || !strings.Contains(e.Hint, "hostname") {
			t.Errorf("%q: %v", d, err)
		}
	}
}

// TestLedger_HTE08 (state half): the refused characters and control
// characters in a document root; spaces are accepted.
func TestLedger_HTE08(t *testing.T) {
	f := newFixture(t)
	spaced := filepath.Join(f.root, "my docs")
	must(t, os.Mkdir(spaced, 0o755))
	if s, err := NewSite(Flags{Domain: "a.test", Docroot: ptr(spaced)}, f.env, nil); err != nil || s.Docroot != spaced {
		t.Fatalf("space: %v", err)
	}
	for _, c := range []string{"$", "*", "?", "[", "]", ";", ":", `"`, `\`, "\n", "\r", "\t", "\x00", "\x7f", "\u0085", "${HOME}"} {
		dir := filepath.Join(f.root, "x"+c+"y")
		_ = os.Mkdir(dir, 0o755) // some names cannot exist; the check runs first anyway
		_, err := NewSite(Flags{Domain: "a.test", Docroot: ptr(dir)}, f.env, nil)
		e, ok := hteerr.As(err)
		if !ok || e.Code != hteerr.InvalidDocroot || e.Hint != hintDocrootChars {
			t.Errorf("%q: %v", c, err)
			continue
		}
		// A refused character is named; a control character is called one.
		if r := []rune(c)[0]; strings.ContainsRune(docrootRefused, r) && !strings.Contains(e.Details, fmt.Sprintf("%q", r)) {
			t.Errorf("%q: the details do not name the character: %s", c, e.Details)
		}
	}
	if err := CheckDocrootText("/x/\xff"); code(err) != hteerr.InvalidDocroot {
		t.Errorf("invalid UTF-8: %v", err)
	}
}

// TestLedger_HTE29: trailing slashes are cleaned and a relative argument
// is resolved against the current directory.
func TestLedger_HTE29(t *testing.T) {
	f := newFixture(t)
	for arg, want := range map[string]string{
		f.pub + "/":                f.pub,
		f.pub + "//":               f.pub,
		f.pub + "/../public":       f.pub,
		"public":                   f.pub,
		"public/":                  f.pub,
		"./public":                 f.pub,
		"../shop/public":           f.pub,
		filepath.Join(f.root, "."): f.root,
	} {
		got, err := ResolveDocroot(arg, f.env.Cwd, nil)
		if err != nil || got != want {
			t.Errorf("%q: %q %v, want %q", arg, got, err, want)
		}
	}
	if _, err := ResolveDocroot("public", "", nil); code(err) != hteerr.InvalidDocroot {
		t.Errorf("no cwd: %v", err)
	}
}

// TestLedger_HTE47: the www. alias rules and extra aliases.
func TestLedger_HTE47(t *testing.T) {
	f := newFixture(t)
	tests := []struct {
		fl   Flags
		want []string
	}{
		{Flags{Domain: "x.test"}, []string{"www.x.test"}},
		{Flags{Domain: "x.test", NoWWW: true}, []string{}},
		{Flags{Domain: "x.test", Aliases: []string{"Other.Test", "api.x.test"}}, []string{"www.x.test", "other.test", "api.x.test"}},
		{Flags{Domain: "x.test", Aliases: []string{"www.x.test"}}, []string{"www.x.test"}},
		{Flags{Domain: "x.test", NoWWW: true, Aliases: []string{"www.x.test"}}, []string{"www.x.test"}},
		{Flags{Domain: "www.x.test"}, []string{}},
		{Flags{Domain: "www.x.test", Aliases: []string{"x.test"}}, []string{"x.test"}},
	}
	for _, tt := range tests {
		tt.fl.Docroot = ptr(f.pub)
		s, err := NewSite(tt.fl, f.env, nil)
		if err != nil {
			t.Errorf("%+v: %v", tt.fl, err)
			continue
		}
		if diff := cmp.Diff(tt.want, s.Aliases); diff != "" {
			t.Errorf("%+v: %s", tt.fl, diff)
		}
	}
	for _, aliases := range [][]string{{"x.test"}, {"a.test", "a.test"}, {""}, {"127.0.0.1"}} {
		if _, err := NewSite(Flags{Domain: "x.test", Docroot: ptr(f.pub), Aliases: aliases}, f.env, nil); code(err) != hteerr.InvalidDomain {
			t.Errorf("%q: %v", aliases, err)
		}
	}
	// modify recomputes the aliases only when an alias flag is given.
	cur, _ := NewSite(Flags{Domain: "x.test", Docroot: ptr(f.pub), Aliases: []string{"a.test"}}, f.env, nil)
	next, changed, err := ModifySite(cur, Flags{Domain: "x.test", Profile: ptr("staging")}, f.env, []Site{cur})
	if err != nil || !changed || !cmp.Equal(next.Aliases, cur.Aliases) || next.Profile != "staging" {
		t.Fatalf("%+v %v", next, err)
	}
	next, _, err = ModifySite(cur, Flags{Domain: "x.test", NoWWW: true}, f.env, []Site{cur})
	if err != nil || len(next.Aliases) != 0 {
		t.Fatalf("--no-www: %+v %v", next, err)
	}
}

// TestLedger_HTE55: create with only the required flags.
func TestLedger_HTE55(t *testing.T) {
	f := newFixture(t)
	s, err := NewSite(Flags{Domain: "c.test", Docroot: ptr(f.pub)}, f.env, nil)
	must(t, err)
	want := Site{Domain: "c.test", Aliases: []string{"www.c.test"}, Docroot: f.pub, PHP: "8.3", Profile: "dev",
		HTTPS: true, ForceHTTPS: false, ForceHTTPSBypassLoopback: false, HTTP2: true, Enabled: true, Kind: KindManaged,
		ExtraHTTPPorts: []int{}, ExtraHTTPSPorts: []int{}}
	if diff := cmp.Diff(want, s); diff != "" {
		t.Fatal(diff)
	}
	env := f.env
	env.DefaultPHP = "7.4"
	if s, _ := NewSite(Flags{Domain: "c.test", Docroot: ptr(f.pub)}, env, nil); s.PHP != "7.4" {
		t.Fatalf("default_php: %s", s.PHP)
	}
	env.DefaultPHP = "8.4" // configured but no longer installed: no silent substitute
	if _, err := NewSite(Flags{Domain: "c.test", Docroot: ptr(f.pub)}, env, nil); code(err) != hteerr.PHPVersionNotInstalled {
		t.Fatalf("stale default_php: %v", err)
	}
	env = f.env
	env.Installed = nil
	if _, err := NewSite(Flags{Domain: "c.test", Docroot: ptr(f.pub)}, env, nil); code(err) != hteerr.PHPVersionNotInstalled {
		t.Fatalf("no PHP: %v", err)
	}
	env.Installed = []string{"8.10", "8.9"}
	if v, _ := ResolvePHP(nil, env); v != "8.10" {
		t.Fatalf("numeric order: %s", v)
	}
}

// TestLedger_HTE60: a missing document root is never created.
func TestLedger_HTE60(t *testing.T) {
	f := newFixture(t)
	missing := filepath.Join(f.root, "nope", "public")
	_, err := NewSite(Flags{Domain: "a.test", Docroot: ptr(missing)}, f.env, nil)
	if e, ok := hteerr.As(err); !ok || e.Code != hteerr.DocrootNotFound || e.Hint != hintDocrootMake {
		t.Fatalf("%v", err)
	}
	if _, err := os.Stat(filepath.Join(f.root, "nope")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the document root was created")
	}
}

// TestLedger_HTE62 (lock half): a second mutation while the lock is held
// fails with lock_timeout once the fake clock passes 30 s.
func TestLedger_HTE62(t *testing.T) {
	f := newFixture(t)
	held, err := AcquireLock(f.store.Paths.Lock(), LockWait, nil)
	must(t, err)
	clk := &fakeClock{now: time.Unix(1_700_000_000, 0)}
	_, err = AcquireLock(f.store.Paths.Lock(), LockWait, clk)
	if e, ok := hteerr.As(err); !ok || e.Code != hteerr.LockTimeout || e.Action() != hteerr.Retry {
		t.Fatalf("%v", err)
	}
	if clk.slept < LockWait || clk.slept > LockWait+lockRetry {
		t.Fatalf("waited %s", clk.slept)
	}
	must(t, held.Unlock())
	again, err := AcquireLock(f.store.Paths.Lock(), LockWait, clk)
	must(t, err)
	must(t, again.Unlock())
	fi, err := os.Stat(f.store.Paths.Lock())
	if err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("lock file %v %v", fi, err)
	}
}

type fakeClock struct {
	now   time.Time
	slept time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Sleep(d time.Duration) {
	c.now = c.now.Add(d)
	c.slept += d
}

func TestSiteRoundTripAndStrictDecode(t *testing.T) {
	f := newFixture(t)
	s, err := NewSite(Flags{Domain: "shop.test", Docroot: ptr(f.pub)}, f.env, nil)
	must(t, err)
	s.PHPIni = map[string]string{"memory_limit": "512M", "error_reporting": "E_ALL & ~E_DEPRECATED"}
	must(t, f.store.SaveSite(s))
	got, err := f.store.LoadSite("shop.test")
	must(t, err)
	if diff := cmp.Diff(s, got); diff != "" {
		t.Fatal(diff)
	}
	fi, _ := os.Stat(f.store.Paths.SiteFile("shop.test"))
	di, _ := os.Stat(f.store.Paths.SitesDir())
	if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
		t.Fatalf("modes %v %v", fi.Mode(), di.Mode())
	}
	data, _ := os.ReadFile(f.store.Paths.SiteFile("shop.test"))
	text := string(data)
	bad := map[string]string{
		"unknown key":     text + "extra = 1\n",
		"missing key":     strings.Replace(text, "http2 = true\n", "", 1),
		"wrong type":      strings.Replace(text, "https = true", `https = "yes"`, 1),
		"bad kind":        strings.Replace(text, `kind = "managed"`, `kind = "proxy"`, 1),
		"bad profile":     strings.Replace(text, `profile = "dev"`, `profile = "custom"`, 1),
		"bad php":         strings.Replace(text, `php = "8.3"`, `php = "8"`, 1),
		"relative root":   strings.Replace(text, `docroot = "`+f.pub, `docroot = "src`, 1),
		"newline in root": strings.Replace(text, `docroot = "`+f.pub, `docroot = "`+f.pub+`\nHeader set X 1`, 1),
		"upper domain":    strings.Replace(text, `domain = "shop.test"`, `domain = "Shop.test"`, 1),
		"force no https":  strings.Replace(strings.Replace(text, "https = true", "https = false", 1), "force_https = false", "force_https = true", 1),
		"raw on managed":  strings.Replace(text, `raw_conf = ""`, `raw_conf = "x"`, 1),
		"bad ini key":     text + "\"bad key\" = \"1\"\n",
		"ini newline":     strings.Replace(text, `memory_limit = "512M"`, `memory_limit = "512M\nx"`, 1),
		"syntax":          text + "[",
		"bad port":        strings.Replace(text, "extra_http_ports = []", "extra_http_ports = [0]", 1),
		"https port only": strings.Replace(strings.Replace(text, "https = true", "https = false", 1), "extra_https_ports = []", "extra_https_ports = [9443]", 1),
	}
	for name, content := range bad {
		if content == text {
			t.Fatalf("%s: replacement did not apply", name)
		}
		_, err := DecodeSite([]byte(content), "x.toml")
		if e, ok := hteerr.As(err); !ok || e.Code != hteerr.InvalidState {
			t.Errorf("%s: %v", name, err)
		}
	}
	raw := Site{Domain: "proxy.test", Aliases: []string{}, Kind: KindRaw, RawConf: "<VirtualHost 127.0.0.1:80>\n</VirtualHost>\n", HTTPS: false, Enabled: true}
	if _, err := EncodeSite(raw); err != nil {
		t.Fatalf("raw site: %v", err)
	}
	raw.PHP, raw.Profile = "8.3", "dev"
	if _, err := EncodeSite(raw); code(err) != hteerr.InvalidState {
		t.Fatalf("raw pool without docroot: %v", err)
	}
	raw.Docroot = "/srv/proxy"
	if _, err := EncodeSite(raw); err != nil {
		t.Fatalf("raw pool: %v", err)
	}
	raw.RawConf = " \n"
	if _, err := EncodeSite(raw); code(err) != hteerr.InvalidState {
		t.Fatalf("empty raw: %v", err)
	}
}

func TestSettings(t *testing.T) {
	f := newFixture(t)
	if _, err := f.store.LoadSettings(); code(err) != hteerr.HTENotInstalled || !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("not installed: %v", err)
	}
	s := DefaultSettings(StandardPorts, "none")
	must(t, f.store.SaveSettings(s))
	got, err := f.store.LoadSettings()
	must(t, err)
	if diff := cmp.Diff(s, got); diff != "" {
		t.Fatal(diff)
	}
	data, _ := os.ReadFile(f.store.Paths.Settings())
	want := "listen = [\"127.0.0.1\", \"::1\"]\ndefault_php = \"\"\nmodules = []\nautostart = \"none\"\n\n[ports]\nhttp = 80\nhttps = 443\n"
	if string(data) != want {
		t.Fatalf("hte.toml:\n%s", data)
	}
	bad := []Settings{
		{Ports: Ports{0, 443}, Listen: []string{"127.0.0.1"}, Autostart: "none"},
		{Ports: Ports{80, 65536}, Listen: []string{"127.0.0.1"}, Autostart: "none"},
		{Ports: Ports{80, 80}, Listen: []string{"127.0.0.1"}, Autostart: "none"},
		{Ports: StandardPorts, Listen: []string{"0.0.0.0"}, Autostart: "none"},
		{Ports: StandardPorts, Listen: []string{"::1"}, Autostart: "none"},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1", "127.0.0.1"}, Autostart: "none"},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1", "*"}, Autostart: "none"},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1"}, Autostart: "auto"},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1"}, Autostart: "none", DefaultPHP: "8"},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1"}, Autostart: "none", Modules: []string{"mod_x.so"}},
		{Ports: StandardPorts, Listen: []string{"127.0.0.1"}, Autostart: "none", Modules: []string{"info", "info"}},
	}
	for _, b := range bad {
		if err := b.Validate(); code(err) != hteerr.InvalidState {
			t.Errorf("%+v: %v", b, err)
		}
	}
	for name, content := range map[string]string{
		"missing default_php": strings.Replace(want, "default_php = \"\"\n", "", 1),
		"missing ports.https": strings.Replace(want, "https = 443\n", "", 1),
		"unknown":             want + "hosts = \"off\"\n",
	} {
		if _, err := DecodeSettings([]byte(content), "hte.toml"); code(err) != hteerr.InvalidState {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// snapshot maps every file under root to its content.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	must(t, filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() {
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[p] = string(bytes.Clone(b))
		} else {
			out[p+"/"] = ""
		}
		return nil
	}))
	return out
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// TestExtraPorts: --extra-http-port and --extra-https-port (design/hte.md
// 2, LEDGER HTE-68).
func TestExtraPorts(t *testing.T) {
	f := newFixture(t)
	env := f.env
	env.Ports = HighPorts
	ports := func(p ...int) *[]int { return &p }
	s, err := NewSite(Flags{Domain: "a.test", Docroot: ptr(f.pub), ExtraHTTPPorts: ports(3000, 8000), ExtraHTTPSPorts: ports(3443)}, env, nil)
	must(t, err)
	if !slices.Equal(s.ExtraHTTPPorts, []int{3000, 8000}) || !slices.Equal(s.ExtraHTTPSPorts, []int{3443}) {
		t.Fatalf("ports: %v %v", s.ExtraHTTPPorts, s.ExtraHTTPSPorts)
	}
	for name, tc := range map[string]struct {
		http, https *[]int
		noHTTPS     bool
		want        hteerr.Code
	}{
		"zero":         {http: ports(0), want: hteerr.InvalidState},
		"too high":     {https: ports(70000), want: hteerr.InvalidState},
		"80":           {http: ports(80), want: hteerr.ConflictingOptions},
		"443":          {https: ports(443), want: hteerr.ConflictingOptions},
		"stack http":   {http: ports(8080), want: hteerr.ConflictingOptions},
		"stack https":  {https: ports(8443), want: hteerr.ConflictingOptions},
		"twice":        {http: ports(3000, 3000), want: hteerr.ConflictingOptions},
		"both lists":   {http: ports(3000), https: ports(3000), want: hteerr.ConflictingOptions},
		"https no TLS": {https: ports(3443), noHTTPS: true, want: hteerr.ConflictingOptions},
	} {
		fl := Flags{Domain: "b.test", Docroot: ptr(f.pub), ExtraHTTPPorts: tc.http, ExtraHTTPSPorts: tc.https}
		if tc.noHTTPS {
			fl.HTTPS = ptr(false)
		}
		if _, err := NewSite(fl, env, nil); code(err) != tc.want {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Another site may share an extra port with the same protocol only.
	if _, err := NewSite(Flags{Domain: "b.test", Docroot: ptr(f.pub), ExtraHTTPPorts: ports(3000)}, env, []Site{s}); err != nil {
		t.Fatalf("shared HTTP port: %v", err)
	}
	if _, err := NewSite(Flags{Domain: "b.test", Docroot: ptr(f.pub), ExtraHTTPSPorts: ports(3000)}, env, []Site{s}); code(err) != hteerr.ConflictingOptions {
		t.Fatalf("protocol clash: %v", err)
	}
	// modify: absent keeps, given replaces, empty clears.
	next, changed, err := ModifySite(s, Flags{Domain: "a.test", ExtraHTTPPorts: ports(9000)}, env, []Site{s})
	must(t, err)
	if !changed || !slices.Equal(next.ExtraHTTPPorts, []int{9000}) || !slices.Equal(next.ExtraHTTPSPorts, []int{3443}) {
		t.Fatalf("modify: %v %v", next.ExtraHTTPPorts, next.ExtraHTTPSPorts)
	}
	next, _, err = ModifySite(next, Flags{Domain: "a.test", ExtraHTTPPorts: ports(), ExtraHTTPSPorts: ports()}, env, nil)
	must(t, err)
	if len(next.ExtraHTTPPorts)+len(next.ExtraHTTPSPorts) != 0 || next.ExtraHTTPPorts == nil {
		t.Fatalf("clear: %v %v", next.ExtraHTTPPorts, next.ExtraHTTPSPorts)
	}
	if _, _, err := ModifySite(s, Flags{Domain: "a.test", HTTPS: ptr(false)}, env, nil); code(err) != hteerr.ConflictingOptions {
		t.Fatalf("https off with extra HTTPS ports: %v", err)
	}
}
