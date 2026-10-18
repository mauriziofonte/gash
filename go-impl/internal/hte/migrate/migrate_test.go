// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package migrate

import (
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/hte/render"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

var update = flag.Bool("update", false, "rewrite the golden plans under testdata/hte/golden/migrate")

const (
	legacyRoot = "../../../testdata/hte/legacy"
	goldenDir  = "../../../testdata/hte/golden/migrate"
	sa         = "/etc/apache2/sites-available/"
)

type dirInfo struct{ name string }

func (d dirInfo) Name() string       { return d.name }
func (d dirInfo) Size() int64        { return 0 }
func (d dirInfo) Mode() fs.FileMode  { return fs.ModeDir | 0o755 }
func (d dirInfo) ModTime() time.Time { return time.Time{} }
func (d dirInfo) IsDir() bool        { return true }
func (d dirInfo) Sys() any           { return nil }

// existingDirs reports every path as an existing directory except those
// in missing: the document roots of the fixtures are not on this machine.
func existingDirs(missing ...string) func(string) (fs.FileInfo, error) {
	return func(p string) (fs.FileInfo, error) {
		if slices.Contains(missing, p) {
			return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
		}
		return dirInfo{filepath.Base(p)}, nil
	}
}

var userPaths = state.Paths{
	ConfigDir:  "/home/dev/.config/gash/hte",
	StateDir:   "/home/dev/.local/state/gash/hte",
	DataDir:    "/home/dev/.local/share/gash/hte",
	RuntimeDir: "/run/user/1000/gash-hte",
}

func envAt(t *testing.T, root string) Env {
	t.Helper()
	sr, err := platform.NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	l, err := legacy.LayoutFor(platform.FamilyDebian, "")
	if err != nil {
		t.Fatal(err)
	}
	return Env{
		Sysroot: sr, OS: platform.OS{ID: "ubuntu", VersionID: "24.04", Family: platform.FamilyDebian},
		Layout: l, User: "dev", Paths: userPaths, Ports: state.StandardPorts,
		Listen: []string{state.LoopbackIPv4, state.LoopbackIPv6}, Installed: []string{"7.4", "8.3"},
		Stat:     existingDirs(),
		ReadFile: htaccessFiles(nil),
	}
}

// htaccessFiles serves the given document root files; every other path
// does not exist (the fixtures' document roots are not on this machine).
func htaccessFiles(files map[string]string) func(string) ([]byte, error) {
	return func(p string) ([]byte, error) {
		if s, ok := files[p]; ok {
			return []byte(s), nil
		}
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
}

func fixtureDir(t *testing.T, variant string) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(legacyRoot, variant))
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// copyVariant copies a legacy fixture, symlinks included, so a test can
// add files to it.
func copyVariant(t *testing.T, variant string) string {
	t.Helper()
	dst := t.TempDir()
	if err := os.CopyFS(dst, os.DirFS(fixtureDir(t, variant))); err != nil {
		t.Fatal(err)
	}
	return dst
}

func write(t *testing.T, root, sys, content string) {
	t.Helper()
	p := filepath.Join(root, sys)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func enable(t *testing.T, root, name string) {
	t.Helper()
	if err := os.Symlink("../sites-available/"+name, filepath.Join(root, "etc/apache2/sites-enabled", name)); err != nil {
		t.Fatal(err)
	}
}

// snapshot records every entry of a tree: a dry run must leave it as is.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		entry := fi.Mode().String()
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			entry += " -> " + target
		case fi.Mode().IsRegular():
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			entry += " " + string(data)
		}
		out[p] = entry + " " + fi.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func mustPlan(t *testing.T, env Env, opt Options) *Plan {
	t.Helper()
	p, err := BuildPlan(env, opt)
	if err != nil {
		t.Fatalf("BuildPlan: %v", err)
	}
	return p
}

func site(t *testing.T, p *Plan, path string) SitePlan {
	t.Helper()
	for _, s := range p.Sites {
		if s.Path == path {
			return s
		}
	}
	t.Fatalf("no plan for %s", path)
	return SitePlan{}
}

func reports(p *Plan, kind string) []Report {
	var out []Report
	for _, r := range p.Reports {
		if r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

// renderSite renders the vhost of an imported site with the userland
// paths of the tests.
func renderSite(t *testing.T, s state.Site) string {
	t.Helper()
	in := render.Input{
		GenDir: userPaths.RenderDir() + "/gen-1", Paths: userPaths,
		Apache: render.Apache{Modules: []render.Module{{Name: "ssl", Path: "/usr/lib/apache2/modules/mod_ssl.so"}}, MimeTypes: "/etc/mime.types"},
		Ports:  state.StandardPorts, Listen: []string{state.LoopbackIPv4, state.LoopbackIPv6},
		PHP: map[string]render.PHPVersion{"7.4": {}, "8.3": {}}, Sites: []state.Site{s},
	}
	g, err := render.Render(in)
	if err != nil {
		t.Fatalf("render %s: %v", s.Domain, err)
	}
	return string(g.File("sites/" + s.Domain + ".conf"))
}

// forceBlock is the force-HTTPS part of a rendered vhost.
func forceBlock(vhost string) string {
	i := strings.Index(vhost, "    # Force HTTPS")
	if i < 0 {
		return ""
	}
	j := strings.Index(vhost[i:], "[R=307,L]\n")
	return vhost[i : i+j+len("[R=307,L]\n")]
}

func TestGoldenPlans(t *testing.T) {
	for _, variant := range []string{"V1", "V1-fixed", "V2", "Broken-bypass", "Malformed"} {
		t.Run(variant, func(t *testing.T) {
			p := mustPlan(t, envAt(t, fixtureDir(t, variant)), Options{})
			js, err := p.JSON()
			if err != nil {
				t.Fatal(err)
			}
			var pretty map[string]any
			if err := json.Unmarshal(js, &pretty); err != nil {
				t.Fatalf("plan JSON: %v", err)
			}
			for _, ext := range []struct {
				name string
				data []byte
			}{{variant + ".txt", []byte(p.Text())}, {variant + ".json", append(js, '\n')}} {
				path := filepath.Join(goldenDir, ext.name)
				if *update {
					if err := os.MkdirAll(goldenDir, 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(path, ext.data, 0o644); err != nil {
						t.Fatal(err)
					}
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run go test -update)", err)
				}
				if diff := cmp.Diff(string(want), string(ext.data)); diff != "" {
					t.Errorf("%s (-golden +plan):\n%s", ext.name, diff)
				}
			}
		})
	}
}

// TestLedger_HTE05: identity is the exact pool path plus its first
// section, never a substring search.
func TestLedger_HTE05(t *testing.T) {
	root := copyVariant(t, "Malformed")
	before := snapshot(t, root)
	p := mustPlan(t, envAt(t, root), Options{})
	valid := site(t, p, sa+"025-valid.test.conf")
	if valid.Action != ActionImport || valid.Pool.Path != "/etc/php/8.3/fpm/pool.d/valid.test.conf" || !valid.Pool.Found {
		t.Fatalf("valid.test: %+v %+v", valid, valid.Pool)
	}
	orphans := reports(p, ReportOrphanPool)
	if len(orphans) != 2 || orphans[0].Paths[0] != "/etc/php/7.4/fpm/pool.d/aaa-notes.conf" ||
		orphans[1].Paths[0] != "/etc/php/custom/fpm/pool.d/orphan.test.conf" {
		t.Fatalf("orphan pools: %+v", orphans)
	}
	if diff := cmp.Diff(before, snapshot(t, root)); diff != "" {
		t.Fatalf("the dry run changed the tree:\n%s", diff)
	}
	// A pool at the exact path whose first section is another site is not
	// this site's pool.
	write(t, root, "/etc/php/8.3/fpm/pool.d/valid.test.conf", "[other.test]\nuser = dev\n; [valid.test]\n")
	valid = site(t, mustPlan(t, envAt(t, root), Options{}), sa+"025-valid.test.conf")
	if valid.Pool.Found || valid.Site.Profile != "dev" || len(valid.Warnings) == 0 {
		t.Fatalf("foreign first section: %+v %+v", valid.Pool, valid.Warnings)
	}
}

const markedVhost = `<VirtualHost 127.0.0.1:80>
    # Auto-generated by HTE-Cli on 2026-04-24 10:11:12
    ServerName {name}
    DocumentRoot /home/dev/sites/{dir}/public
    <FilesMatch \.php$>
        SetHandler "proxy:unix:/var/run/php/php8.3-fpm-{sock}.sock|fcgi://localhost/"
    </FilesMatch>
    <Directory /home/dev/sites/{dir}/public/>
        Require all granted
    </Directory>
</VirtualHost>
`

func marked(name, sock, dir string) string {
	return strings.NewReplacer("{name}", name, "{sock}", sock, "{dir}", dir).Replace(markedVhost)
}

func TestLedger_HTE21(t *testing.T) {
	root := copyVariant(t, "V2")
	write(t, root, sa+"004-127.0.0.1.conf", marked("127.0.0.1", "127.0.0.1", "ip"))
	enable(t, root, "004-127.0.0.1.conf")
	p := mustPlan(t, envAt(t, root), Options{})
	ip := site(t, p, sa+"004-127.0.0.1.conf")
	if ip.Action != ActionSkip || !strings.Contains(ip.Reason, "is an IP address") || ip.Site != nil {
		t.Fatalf("IP vhost: %+v", ip)
	}
	if strings.Contains(p.Hosts.Block, "127.0.0.1 127.0.0.1") || !slices.Contains(p.Blocking, ip.Path) {
		t.Fatalf("hosts %q blocking %v", p.Hosts.Block, p.Blocking)
	}
}

// TestLedger_HTE23: every force block hte-cli fix knew imports as forced
// and renders gash's block.
func TestLedger_HTE23(t *testing.T) {
	for _, tc := range []struct{ variant, file string }{
		{"V1", "003-forced.test.conf"}, {"V1-fixed", "003-forced.test.conf"},
		{"Broken-bypass", "001-broken.test.conf"}, {"Broken-bypass", "002-broken0.test.conf"},
	} {
		s := site(t, mustPlan(t, envAt(t, fixtureDir(t, tc.variant)), Options{}), sa+tc.file)
		if s.Action != ActionImport || !s.Site.ForceHTTPS || !s.Site.HTTPS {
			t.Fatalf("%s %s: %+v", tc.variant, tc.file, s)
		}
		vhost := renderSite(t, *s.Site)
		if forceBlock(vhost) == "" || strings.Contains(vhost, "R=301") {
			t.Fatalf("%s %s rendered without gash's force block:\n%s", tc.variant, tc.file, vhost)
		}
	}
}

// TestLedger_HTE28: an unreadable entry is reported and the scan goes on.
func TestLedger_HTE28(t *testing.T) {
	root := copyVariant(t, "V2")
	loop := filepath.Join(root, "etc/apache2/sites-available/009-loop.conf")
	if err := os.Symlink("009-loop.conf", loop); err != nil {
		t.Fatal(err)
	}
	if syscall.Geteuid() != 0 {
		write(t, root, sa+"locked/010-locked.test.conf", marked("locked.test", "locked.test", "locked"))
		if err := os.Chmod(filepath.Join(root, "etc/apache2/sites-available/locked"), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(filepath.Join(root, "etc/apache2/sites-available/locked"), 0o755) })
	}
	p := mustPlan(t, envAt(t, root), Options{})
	un := reports(p, ReportUnreadable)
	if len(un) == 0 || un[0].Paths[0] != sa+"009-loop.conf" {
		t.Fatalf("unreadable: %+v", un)
	}
	if syscall.Geteuid() != 0 && (len(un) != 2 || un[1].Paths[0] != sa+"locked") {
		t.Fatalf("unreadable directory: %+v", un)
	}
	for _, f := range []string{"001-http.test.conf", "002-https.test.conf", "003-forced.test.conf"} {
		if s := site(t, p, sa+f); s.Action != ActionImport {
			t.Errorf("%s: %+v", f, s)
		}
	}
}

// TestLedger_HTE30: forced is the comment (design option a); a commented
// rule is warned about.
func TestLedger_HTE30(t *testing.T) {
	root := copyVariant(t, "V2")
	p := filepath.Join(root, "etc/apache2/sites-available/003-forced.test.conf")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	edited := strings.Replace(string(data), "RewriteRule ^/(.*)", "#RewriteRule ^/(.*)", 1)
	if edited == string(data) {
		t.Fatal("no rule to comment")
	}
	if err := os.WriteFile(p, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	s := site(t, mustPlan(t, envAt(t, root), Options{}), sa+"003-forced.test.conf")
	if !s.Site.ForceHTTPS || !slices.ContainsFunc(s.Warnings, func(w string) bool { return strings.Contains(w, "no RewriteRule") }) {
		t.Fatalf("%+v", s)
	}
}

func TestLedger_HTE31(t *testing.T) {
	root := copyVariant(t, "Malformed")
	before := snapshot(t, root)
	p := mustPlan(t, envAt(t, root), Options{})
	want := map[string]struct {
		class  legacy.Class
		action Action
		reason string
	}{
		"000-default.conf":             {legacy.ClassRaw, ActionSkip, "no ServerName: nothing names the site"},
		"020-.conf":                    {legacy.ClassMalformed, ActionSkip, "no SetHandler to a PHP-FPM socket"},
		"021-.conf":                    {legacy.ClassMalformed, ActionSkip, "empty domain in the socket /var/run/php/php8.3-fpm-.sock"},
		"022-default-socket.test.conf": {legacy.ClassMalformed, ActionSkip, "default socket /var/run/php/php8.3-fpm.sock (the distribution's pool, no domain)"},
		"023-nocert.test.conf":         {legacy.ClassV1, ActionImport, ""},
		"024-quoted.test.conf":         {legacy.ClassV2, ActionImport, ""},
		"025-valid.test.conf":          {legacy.ClassV2, ActionImport, ""},
	}
	for f, w := range want {
		s := site(t, p, sa+f)
		if s.Class != w.class || s.Action != w.action || s.Reason != w.reason {
			t.Errorf("%s: %s %s %q", f, s.Class, s.Action, s.Reason)
		}
	}
	if len(p.Sites) != len(want) {
		t.Fatalf("%d site plans", len(p.Sites))
	}
	quoted := site(t, p, sa+"024-quoted.test.conf")
	if quoted.Site.Docroot != "/home/dev/sites/my quoted/public" {
		t.Fatalf("quoted docroot %q", quoted.Site.Docroot)
	}
	// Only the enabled skipped file blocks the stop; 020-022 are disabled.
	if diff := cmp.Diff([]string{sa + "000-default.conf"}, p.Blocking); diff != "" {
		t.Fatal(diff)
	}
	if diff := cmp.Diff(before, snapshot(t, root)); diff != "" {
		t.Fatalf("the dry run changed the tree:\n%s", diff)
	}
}

func TestLedger_HTE45(t *testing.T) {
	root := copyVariant(t, "V2")
	before := snapshot(t, root)
	p := mustPlan(t, envAt(t, root), Options{})
	want := []hostshelper.Entry{
		{Address: "127.0.0.1", Names: []string{"http.test", "www.http.test"}},
		{Address: "127.0.0.1", Names: []string{"https.test", "www.https.test"}},
		{Address: "127.0.0.1", Names: []string{"forced.test", "www.forced.test"}},
	}
	if diff := cmp.Diff(want, p.Hosts.Entries); diff != "" {
		t.Fatal(diff)
	}
	if _, err := hostshelper.ParseBlock([]byte(p.Hosts.Block)); err != nil {
		t.Fatalf("the block does not pass the helper's grammar: %v", err)
	}
	if len(p.Hosts.Legacy) != 3 || p.Hosts.Legacy[0].Text != "127.0.0.1    http.test www.http.test # hte-cli" {
		t.Fatalf("legacy lines: %+v", p.Hosts.Legacy)
	}
	if diff := cmp.Diff(before, snapshot(t, root)); diff != "" {
		t.Fatalf("hosts changed:\n%s", diff)
	}
	// A legacy name no imported site covers still goes into the block.
	data, _ := os.ReadFile(filepath.Join(root, "etc/hosts"))
	write(t, root, "/etc/hosts", string(data)+"127.0.0.1    gone.test www.gone.test # hte-cli\n10.0.0.1 lan.test # hte-cli\n")
	p = mustPlan(t, envAt(t, root), Options{})
	if last := p.Hosts.Entries[len(p.Hosts.Entries)-1]; !slices.Equal(last.Names, []string{"gone.test", "www.gone.test"}) {
		t.Fatalf("leftover legacy names: %+v", last)
	}
	if r := reports(p, ReportHostsLine); len(r) != 1 || !strings.Contains(r[0].Message, "10.0.0.1") {
		t.Fatalf("non-loopback legacy line: %+v", r)
	}
}

const proxyVhost = `<VirtualHost *:80>
    ServerName proxy.test
    ServerAlias *.proxy.test api.proxy.test
    ProxyPreserveHost On
    ProxyPass / http://127.0.0.1:3000/
    ErrorLog ${APACHE_LOG_DIR}/proxy-error.log
    CustomLog /var/log/apache2/proxy-access.log combined
</VirtualHost>
<VirtualHost *:443>
    ServerName proxy.test
    SSLEngine on
    SSLCertificateFile /etc/ssl/certs/proxy.pem
    SSLCertificateKeyFile /etc/ssl/private/proxy.key
    ProxyPass / http://127.0.0.1:3000/
</VirtualHost>
`

const proxyWant = `<VirtualHost 127.0.0.1:80 [::1]:80>
    ServerName proxy.test
    ServerAlias *.proxy.test api.proxy.test
    ProxyPreserveHost On
    ProxyPass / http://127.0.0.1:3000/
    ErrorLog /home/dev/.local/state/gash/hte/logs/proxy.test/proxy-error.log
    CustomLog /home/dev/.local/state/gash/hte/logs/proxy.test/proxy-access.log combined
</VirtualHost>
<VirtualHost 127.0.0.1:443 [::1]:443>
    ServerName proxy.test
    SSLEngine on
    SSLCertificateFile "/home/dev/.local/share/gash/hte/certs/proxy.test/cert.pem"
    SSLCertificateKeyFile "/home/dev/.local/share/gash/hte/certs/proxy.test/key.pem"
    ProxyPass / http://127.0.0.1:3000/
</VirtualHost>
`

// TestLedger_HTE64: V2 sites, a raw reverse proxy, an excluded vhost and a
// conf-enabled snippet, each with its outcome.
func TestLedger_HTE64(t *testing.T) {
	root := copyVariant(t, "V2")
	write(t, root, sa+"010-proxy.test.conf", proxyVhost)
	enable(t, root, "010-proxy.test.conf")
	write(t, root, "/etc/ssl/certs/proxy.pem", "cert\n")
	write(t, root, "/etc/ssl/private/proxy.key", "key\n")
	write(t, root, "/etc/apache2/conf-enabled/security.conf", "ServerTokens Prod\n")
	before := snapshot(t, root)
	p := mustPlan(t, envAt(t, root), Options{Exclude: []string{"https.test"}})

	proxy := site(t, p, sa+"010-proxy.test.conf")
	if proxy.Action != ActionImportRaw || proxy.Site.Kind != state.KindRaw || !proxy.Site.HTTPS || !proxy.Site.Enabled {
		t.Fatalf("proxy: %+v", proxy)
	}
	if diff := cmp.Diff(proxyWant, proxy.Site.RawConf); diff != "" {
		t.Fatalf("raw conf (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff([]string{"api.proxy.test"}, proxy.Site.Aliases); diff != "" {
		t.Fatal(diff)
	}
	if proxy.Cert.Action != CertCopy || proxy.Cert.Key != "/etc/ssl/private/proxy.key" {
		t.Fatalf("proxy cert: %+v", proxy.Cert)
	}
	if !slices.Contains(p.RootOps, RootOp{Op: "copy_key", Domain: "proxy.test", Src: "/etc/ssl/private/proxy.key",
		Dst: "/home/dev/.local/share/gash/hte/certs/proxy.test/key.pem"}) {
		t.Fatalf("root steps: %+v", p.RootOps)
	}
	excluded := site(t, p, sa+"002-https.test.conf")
	if excluded.Action != ActionExcluded {
		t.Fatalf("excluded: %+v", excluded)
	}
	if diff := cmp.Diff([]string{sa + "002-https.test.conf", "/etc/apache2/conf-enabled/security.conf"}, p.NoLongerServed); diff != "" {
		t.Fatal(diff)
	}
	if len(p.Blocking) != 0 {
		t.Fatalf("blocking: %v", p.Blocking)
	}
	if diff := cmp.Diff(before, snapshot(t, root)); diff != "" {
		t.Fatalf("the dry run changed the tree:\n%s", diff)
	}
	// --site keeps only the named domains.
	p = mustPlan(t, envAt(t, root), Options{Sites: []string{"HTTP.test"}})
	for _, s := range p.Sites {
		if (s.Domain == "http.test") != (s.Action == ActionImport) {
			t.Errorf("--site http.test: %s %s", s.Domain, s.Action)
		}
	}
}

// TestLedger_HTE65: the bypass follows the variant; every forced site
// renders 307.
func TestLedger_HTE65(t *testing.T) {
	blocks := map[bool][]string{}
	for _, tc := range []struct {
		variant, file string
		bypass        bool
	}{
		{"V1", "003-forced.test.conf", false}, {"V1-fixed", "003-forced.test.conf", true},
		{"V2", "003-forced.test.conf", true}, {"Broken-bypass", "001-broken.test.conf", true},
		{"Broken-bypass", "002-broken0.test.conf", true},
	} {
		s := site(t, mustPlan(t, envAt(t, fixtureDir(t, tc.variant)), Options{}), sa+tc.file)
		if s.Site.ForceHTTPSBypassLoopback != tc.bypass {
			t.Errorf("%s: bypass %v", tc.variant, s.Site.ForceHTTPSBypassLoopback)
		}
		block := forceBlock(renderSite(t, *s.Site))
		if !strings.Contains(block, "[R=307,L]") {
			t.Errorf("%s: %q", tc.variant, block)
		}
		blocks[tc.bypass] = append(blocks[tc.bypass], block)
		if tc.variant == "V1" && !slices.ContainsFunc(s.Notes, func(n string) bool { return strings.Contains(n, "307 instead of 301") }) {
			t.Errorf("V1: no note on the 301 to 307 change: %v", s.Notes)
		}
	}
	for bypass, bs := range blocks {
		for _, b := range bs[1:] {
			if b != bs[0] {
				t.Errorf("bypass %v: different force blocks:\n%s\n%s", bypass, bs[0], b)
			}
		}
	}
	if strings.Contains(blocks[false][0], "REMOTE_ADDR") || !strings.Contains(blocks[true][0], "REMOTE_ADDR") {
		t.Fatal("bypass conditions do not follow the setting")
	}
}

func TestLedger_HTE66(t *testing.T) {
	// V1: dev by values, no differences.
	p := mustPlan(t, envAt(t, fixtureDir(t, "V1")), Options{})
	for _, f := range []string{"001-http.test.conf", "002-https.test.conf", "003-forced.test.conf"} {
		s := site(t, p, sa+f)
		if s.Site.Profile != "dev" || s.Pool.Source != legacy.ProfileFromValues || len(s.Pool.Diffs) != 0 || len(s.Notes) == 0 {
			t.Errorf("V1 %s: %+v %+v", f, s.Site, s.Pool)
		}
	}
	// V2: the marker.
	p = mustPlan(t, envAt(t, fixtureDir(t, "V2")), Options{})
	for f, want := range map[string]profiles.Name{"001-http.test.conf": profiles.Dev, "002-https.test.conf": profiles.Staging, "003-forced.test.conf": profiles.Hardened} {
		s := site(t, p, sa+f)
		if s.Site.Profile != string(want) || s.Pool.Source != legacy.ProfileFromMarker || len(s.Pool.Diffs) != 0 {
			t.Errorf("V2 %s: %+v %+v", f, s.Site.Profile, s.Pool)
		}
	}
	// An extra php_admin_value and a changed pm value: reported; kept in
	// [php_ini] only with --keep-pool-values, and only the php_* one.
	root := copyVariant(t, "V2")
	pool := filepath.Join(root, "etc/php/8.3/fpm/pool.d/http.test.conf")
	data, _ := os.ReadFile(pool)
	edited := strings.Replace(string(data), "pm.max_children = 5", "pm.max_children = 8", 1) +
		"php_admin_value[max_input_vars] = 5000\n"
	edited = strings.Replace(edited, "user = dev", "user = root", 1)
	if err := os.WriteFile(pool, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	s := site(t, mustPlan(t, envAt(t, root), Options{}), sa+"001-http.test.conf")
	if len(s.Pool.Diffs) != 2 || s.Site.PHPIni != nil {
		t.Fatalf("without --keep-pool-values: %+v %+v", s.Pool.Diffs, s.Site.PHPIni)
	}
	if s.Action != ActionImport || !slices.ContainsFunc(s.Warnings, func(w string) bool { return strings.Contains(w, "ran as root") }) {
		t.Fatalf("foreign pool user: %+v", s)
	}
	s = site(t, mustPlan(t, envAt(t, root), Options{KeepPoolValues: true}), sa+"001-http.test.conf")
	if diff := cmp.Diff(map[string]string{"max_input_vars": "5000"}, s.Site.PHPIni); diff != "" {
		t.Fatal(diff)
	}
	if !slices.Equal(s.Pool.NotKept, []string{"pm.max_children"}) {
		t.Fatalf("not kept: %v", s.Pool.NotKept)
	}
}

func TestLedger_HTE67(t *testing.T) {
	p := mustPlan(t, envAt(t, fixtureDir(t, "V2")), Options{})
	https := site(t, p, sa+"002-https.test.conf")
	if https.Cert.Action != CertCopy || https.Cert.Cert != "/etc/apache2/certs-selfsigned/https.test.crt" {
		t.Fatalf("cert: %+v", https.Cert)
	}
	want := RootOp{Op: "copy_key", Domain: "https.test", Src: "/etc/apache2/certs-selfsigned/https.test.key",
		Dst: "/home/dev/.local/share/gash/hte/certs/https.test/key.pem"}
	if !slices.Contains(p.RootOps, want) {
		t.Fatalf("root steps: %+v", p.RootOps)
	}
	nocert := site(t, mustPlan(t, envAt(t, fixtureDir(t, "Malformed")), Options{}), sa+"023-nocert.test.conf")
	if nocert.Site.HTTPS || nocert.Cert != nil {
		t.Fatalf("nocert: %+v", nocert)
	}
	p = mustPlan(t, envAt(t, fixtureDir(t, "V2")), Options{ReissueCerts: true})
	if len(p.RootOps) != 0 || site(t, p, sa+"002-https.test.conf").Cert.Action != CertReissue {
		t.Fatalf("--reissue-certs: %+v", p.RootOps)
	}
	// A missing key: a new pair, with a warning.
	root := copyVariant(t, "V2")
	if err := os.Remove(filepath.Join(root, "etc/apache2/certs-selfsigned/https.test.key")); err != nil {
		t.Fatal(err)
	}
	s := site(t, mustPlan(t, envAt(t, root), Options{}), sa+"002-https.test.conf")
	if s.Cert.Action != CertReissue || len(s.Warnings) == 0 {
		t.Fatalf("missing key: %+v", s)
	}
}

func TestLedger_HTE68(t *testing.T) {
	root := copyVariant(t, "V2")
	// Duplicates, compared case-insensitively.
	write(t, root, sa+"004-App.test.conf", marked("App.test", "App.test", "app"))
	write(t, root, sa+"005-app.test.conf", marked("app.test", "app.test", "app2"))
	// A raw vhost on http.test's socket.
	write(t, root, sa+"006-tool.test.conf", "<VirtualHost 127.0.0.1:80>\n    ServerName tool.test\n    DocumentRoot /srv/tool\n"+
		"    <FilesMatch \\.php$>\n        SetHandler \"proxy:unix:/var/run/php/php8.3-fpm-http.test.sock|fcgi://localhost/\"\n    </FilesMatch>\n</VirtualHost>\n")
	// A custom port on a marked vhost.
	write(t, root, sa+"007-port.test.conf", strings.Replace(marked("port.test", "port.test", "port"), "127.0.0.1:80>", "127.0.0.1:80 127.0.0.1:8081>", 1))
	// Orphan certificate files and APACHE_RUN_USER.
	write(t, root, "/etc/apache2/certs-selfsigned/gone.test.crt", "x\n")
	write(t, root, "/etc/apache2/certs-selfsigned/gone.test.key", "x\n")
	write(t, root, "/etc/apache2/certs-selfsigned/old.test.info", "x\n")
	write(t, root, "/etc/apache2/envvars", "export APACHE_RUN_USER=dev\nexport APACHE_RUN_GROUP=dev\n")
	before := snapshot(t, root)
	p := mustPlan(t, envAt(t, root), Options{})

	dup := reports(p, ReportDuplicate)
	if len(dup) != 1 || !slices.Equal(dup[0].Paths, []string{sa + "004-App.test.conf", sa + "005-app.test.conf"}) {
		t.Fatalf("duplicates: %+v", dup)
	}
	for _, f := range []string{"004-App.test.conf", "005-app.test.conf"} {
		if s := site(t, p, sa+f); s.Action != ActionSkip {
			t.Errorf("%s: %s", f, s.Action)
		}
	}
	shared := reports(p, ReportSharedSocket)
	if len(shared) != 1 || !slices.Equal(shared[0].Paths, []string{sa + "001-http.test.conf", sa + "006-tool.test.conf"}) {
		t.Fatalf("shared sockets: %+v", shared)
	}
	tool := site(t, p, sa+"006-tool.test.conf")
	sock := render.SocketPath(userPaths.RuntimeDir, "8.3", "tool.test")
	if tool.Action != ActionImportRaw || tool.Site.PHP != "8.3" || !strings.Contains(tool.Site.RawConf, "proxy:unix:"+sock+"|") {
		t.Fatalf("tool: %+v", tool)
	}
	if ports := reports(p, ReportCustomPort); len(ports) != 1 || !strings.Contains(ports[0].Message, "port 8081") {
		t.Fatalf("custom ports: %+v", ports)
	}
	var orphans []string
	for _, r := range p.Reports {
		if r.Kind == ReportOrphanCert || r.Kind == ReportOrphanInfo {
			orphans = append(orphans, r.Kind+" "+r.Paths[0])
		}
	}
	if diff := cmp.Diff([]string{
		"orphan_certificate /etc/apache2/certs-selfsigned/gone.test.crt",
		"orphan_certificate /etc/apache2/certs-selfsigned/gone.test.key",
		"orphan_info /etc/apache2/certs-selfsigned/old.test.info",
	}, orphans); diff != "" {
		t.Fatal(diff)
	}
	if r := reports(p, ReportRunUser); len(r) != 1 {
		t.Fatalf("APACHE_RUN_USER: %+v", p.Reports)
	}
	if diff := cmp.Diff(before, snapshot(t, root)); diff != "" {
		t.Fatalf("the dry run changed the tree:\n%s", diff)
	}
}

func TestPlanRefusals(t *testing.T) {
	env := envAt(t, fixtureDir(t, "V2"))
	for _, tc := range []struct {
		opt  Options
		want hteerr.Code
	}{
		{Options{Sites: []string{"http.test"}, Exclude: []string{"HTTP.test"}}, hteerr.ConflictingOptions},
		{Options{Sites: []string{"nope.test"}}, hteerr.SiteNotFound},
		{Options{Exclude: []string{"nope.test"}}, hteerr.SiteNotFound},
		{Options{Sites: []string{"not a domain"}}, hteerr.InvalidDomain},
	} {
		if _, err := BuildPlan(env, tc.opt); hteerr.CodeOf(err) != tc.want {
			t.Errorf("%+v: %v", tc.opt, err)
		}
	}
	// Validation failures are skips with their code, never defaults.
	env.Installed = []string{"8.3"}
	env.Stat = existingDirs("/home/dev/sites/http/public")
	p := mustPlan(t, env, Options{})
	if s := site(t, p, sa+"002-https.test.conf"); s.Action != ActionSkip || s.Code != hteerr.PHPVersionNotInstalled {
		t.Errorf("PHP 7.4 not installed: %+v", s)
	}
	if s := site(t, p, sa+"001-http.test.conf"); s.Action != ActionSkip || s.Code != hteerr.DocrootNotFound {
		t.Errorf("missing docroot: %+v", s)
	}
	if len(p.Blocking) != 2 {
		t.Errorf("blocking: %v", p.Blocking)
	}
	// Already managed sites are left alone (migrate is idempotent).
	env = envAt(t, fixtureDir(t, "V2"))
	env.Existing = []state.Site{{Domain: "http.test", Aliases: []string{"www.http.test"}, Kind: state.KindManaged}}
	if s := site(t, mustPlan(t, env, Options{}), sa+"001-http.test.conf"); s.Action != ActionManaged {
		t.Errorf("existing: %+v", s)
	}
}

func TestRawRefusals(t *testing.T) {
	for name, tc := range map[string]struct{ conf, reason string }{
		"custom port": {"<VirtualHost *:8080>\n    ServerName r.test\n</VirtualHost>\n", "listens on a port other than 80 and 443"},
		"listen":      {"Listen 8080\n<VirtualHost *:80>\n    ServerName r.test\n</VirtualHost>\n", "Listen in a vhost file"},
		"two names": {"<VirtualHost *:80>\n    ServerName r.test\n</VirtualHost>\n<VirtualHost *:80>\n    ServerName s.test\n</VirtualHost>\n",
			"several ServerName values"},
		"no php version": {"<VirtualHost *:80>\n    ServerName r.test\n    DocumentRoot /srv/r\n    SetHandler \"proxy:unix:/run/php/custom.sock|fcgi://localhost/\"\n</VirtualHost>\n",
			"names no PHP version"},
		"half a pair": {"<VirtualHost *:443>\n    ServerName r.test\n    SSLCertificateFile /etc/ssl/r.pem\n</VirtualHost>\n", "must come together"},
		"unbalanced":  {"<VirtualHost *:80>\n    ServerName r.test\n", "is never closed"},
		"php without docroot": {"<VirtualHost *:80>\n    ServerName r.test\n    SetHandler \"proxy:unix:/run/php/php8.3-fpm.sock|fcgi://localhost/\"\n</VirtualHost>\n",
			"has no docroot"},
	} {
		root := copyVariant(t, "V2")
		write(t, root, sa+"050-r.test.conf", tc.conf)
		s := site(t, mustPlan(t, envAt(t, root), Options{}), sa+"050-r.test.conf")
		if s.Action != ActionSkip || s.Site != nil || !strings.Contains(s.Reason, tc.reason) {
			t.Errorf("%s: %s %q", name, s.Action, s.Reason)
		}
	}
}

// TestHtaccessWarning: migrate warns when a document root's .htaccess
// redirects to HTTPS on its own (LEDGER HTE-26).
func TestHtaccessWarning(t *testing.T) {
	env := envAt(t, fixtureDir(t, "V2"))
	env.ReadFile = htaccessFiles(map[string]string{
		"/home/dev/sites/http/public/.htaccess": "RewriteEngine On\nRewriteCond %{HTTP:X-Forwarded-Proto} !https\nRewriteRule ^ https://%{HTTP_HOST}%{REQUEST_URI} [L,R=301]\n",
	})
	s := site(t, mustPlan(t, env, Options{}), sa+"001-http.test.conf")
	if s.Action != ActionImport || !slices.ContainsFunc(s.Warnings, func(w string) bool {
		return strings.HasPrefix(w, "/home/dev/sites/http/public/.htaccess:3 redirects to HTTPS")
	}) {
		t.Fatalf("%+v", s.Warnings)
	}
}
