// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package legacy

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/profiles"
	"github.com/mauriziofonte/gash/internal/platform"
)

const fixtures = "../../../testdata/hte/legacy"

func scanFixture(t *testing.T, variant string) *Inventory {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join(fixtures, variant))
	if err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	l, err := LayoutFor(platform.FamilyDebian, "")
	if err != nil {
		t.Fatal(err)
	}
	return Scan(sr, l)
}

type want struct {
	class   Class
	reason  string
	enabled bool
	domain  string
	php     string
	docroot string
	https   bool
	forced  bool
	bypass  bool
}

func TestClassifyFixtures(t *testing.T) {
	sa := "/etc/apache2/sites-available/"
	cases := map[string]map[string]want{
		"V1": {
			"001-http.test.conf":   {class: ClassV1, enabled: true, domain: "http.test", php: "8.3", docroot: "/home/dev/sites/http/public"},
			"002-https.test.conf":  {class: ClassV1, enabled: true, domain: "https.test", php: "7.4", docroot: "/home/dev/sites/https/public", https: true},
			"003-forced.test.conf": {class: ClassV1, enabled: true, domain: "forced.test", php: "8.3", docroot: "/home/dev/sites/forced/public", https: true, forced: true},
		},
		"V1-fixed": {
			"001-http.test.conf":   {class: ClassV1, enabled: true, domain: "http.test", php: "8.3", docroot: "/home/dev/sites/http/public"},
			"002-https.test.conf":  {class: ClassV1, enabled: true, domain: "https.test", php: "7.4", docroot: "/home/dev/sites/https/public", https: true},
			"003-forced.test.conf": {class: ClassV1Fixed, enabled: true, domain: "forced.test", php: "8.3", docroot: "/home/dev/sites/forced/public", https: true, forced: true, bypass: true},
		},
		"V2": {
			"001-http.test.conf":   {class: ClassV2, enabled: true, domain: "http.test", php: "8.3", docroot: "/home/dev/sites/http/public", bypass: true},
			"002-https.test.conf":  {class: ClassV2, enabled: true, domain: "https.test", php: "7.4", docroot: "/home/dev/sites/https/public", https: true, bypass: true},
			"003-forced.test.conf": {class: ClassV2, enabled: true, domain: "forced.test", php: "8.3", docroot: "/home/dev/sites/forced/public", https: true, forced: true, bypass: true},
		},
		"Broken-bypass": {
			"001-broken.test.conf":  {class: ClassBrokenBypass, enabled: true, domain: "broken.test", php: "8.3", docroot: "/home/dev/sites/broken.test/public", https: true, forced: true, bypass: true},
			"002-broken0.test.conf": {class: ClassBrokenBypass, enabled: true, domain: "broken0.test", php: "8.3", docroot: "/home/dev/sites/broken0.test/public", https: true, forced: true, bypass: true},
		},
		"Malformed": {
			"000-default.conf":             {class: ClassRaw, enabled: true, docroot: "/var/www/html"},
			"020-.conf":                    {class: ClassMalformed, reason: "no SetHandler to a PHP-FPM socket"},
			"021-.conf":                    {class: ClassMalformed, reason: "empty domain in the socket /var/run/php/php8.3-fpm-.sock", docroot: "/home/dev/sites/empty/public"},
			"022-default-socket.test.conf": {class: ClassMalformed, reason: "default socket /var/run/php/php8.3-fpm.sock (the distribution's pool, no domain)", docroot: "/home/dev/sites/ds/public"},
			"023-nocert.test.conf":         {class: ClassV1, domain: "nocert.test", php: "8.3", docroot: "/home/dev/sites/nocert/public"},
			"024-quoted.test.conf":         {class: ClassV2, enabled: true, domain: "quoted.test", php: "8.3", docroot: "/home/dev/sites/my quoted/public", bypass: true},
			"025-valid.test.conf":          {class: ClassV2, enabled: true, domain: "valid.test", php: "8.3", docroot: "/home/dev/sites/valid/public", bypass: true},
		},
	}
	for variant, files := range cases {
		t.Run(variant, func(t *testing.T) {
			inv := scanFixture(t, variant)
			if len(inv.Unreadable) != 0 {
				t.Fatalf("unreadable: %v", inv.Unreadable)
			}
			got := map[string]want{}
			for _, v := range inv.Vhosts {
				got[strings.TrimPrefix(v.Path, sa)] = want{v.Class, v.Reason, v.Enabled, v.Domain, v.PHP, v.Docroot, v.HTTPS, v.Forced, v.Bypass}
			}
			if diff := cmp.Diff(files, got, cmp.AllowUnexported(want{})); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
		})
	}
}

// The facts agree with what hte-cli 1.1.1 read back from the same trees
// (expected-discovery-1.1.1.json), except where gash is strict by design.
func TestMatchesHTECLIDiscovery(t *testing.T) {
	type disc struct {
		Conf        string  `json:"conf"`
		PHPVer      string  `json:"phpver"`
		Domain      string  `json:"domain"`
		Docroot     string  `json:"docroot"`
		Enabled     bool    `json:"enabled"`
		SSL         bool    `json:"ssl"`
		ForcedSSL   bool    `json:"forcedssl"`
		CertFile    *string `json:"sslcertfile"`
		KeyFile     *string `json:"sslkeyfile"`
		Pool        string  `json:"pool"`
		PoolPHPVer  string  `json:"pool_phpver"`
		Profile     string  `json:"profile"`
		NeedsBypass bool    `json:"needs_localhost_bypass_fix"`
	}
	for _, variant := range []string{"V1", "V1-fixed", "V2", "Broken-bypass", "Malformed"} {
		data, err := os.ReadFile(filepath.Join(fixtures, variant, "expected-discovery-1.1.1.json"))
		if err != nil {
			t.Fatal(err)
		}
		var exp struct {
			Vhosts []disc `json:"vhosts"`
		}
		if err := json.Unmarshal(data, &exp); err != nil {
			t.Fatal(err)
		}
		inv := scanFixture(t, variant)
		for _, e := range exp.Vhosts {
			i := slices.IndexFunc(inv.Vhosts, func(v Vhost) bool { return v.Path == e.Conf })
			if i < 0 {
				t.Errorf("%s: %s not found", variant, e.Conf)
				continue
			}
			v := inv.Vhosts[i]
			cert, key := "", ""
			if e.CertFile != nil {
				cert, key = *e.CertFile, *e.KeyFile
			}
			if v.Domain != e.Domain || v.PHP != e.PHPVer || v.Docroot != e.Docroot || v.Enabled != e.Enabled ||
				v.HTTPS != e.SSL || v.Forced != e.ForcedSSL || v.CertFile != cert || v.KeyFile != key {
				t.Errorf("%s %s: got %+v, hte-cli %+v", variant, e.Conf, v, e)
			}
			// hte-cli fix rewrote the V1 force block and the Broken-bypass one.
			if e.NeedsBypass != (v.Class == ClassBrokenBypass || v.Class == ClassV1 && v.Forced) {
				t.Errorf("%s %s: class %s, needs_localhost_bypass_fix %v", variant, e.Conf, v.Class, e.NeedsBypass)
			}
			// The pool: exact path, not hte-cli's substring search. The
			// only difference on the fixtures is valid.test (Malformed),
			// which hte-cli gave the hand-written aaa-notes.conf.
			pool := inv.Layout.PoolPath(v.PHP, v.Domain)
			if e.Domain == "valid.test" {
				if e.Pool != "/etc/php/7.4/fpm/pool.d/aaa-notes.conf" || pool != "/etc/php/8.3/fpm/pool.d/valid.test.conf" {
					t.Errorf("valid.test pool %s (hte-cli %s)", pool, e.Pool)
				}
			} else if pool != e.Pool {
				t.Errorf("%s %s: pool %s, hte-cli %s", variant, e.Domain, pool, e.Pool)
			}
			p, ok := inv.Pools[pool]
			if !ok {
				t.Errorf("%s: pool %s not parsed", e.Domain, pool)
				continue
			}
			if p.Section != v.Domain {
				t.Errorf("%s: pool section %q", e.Domain, p.Section)
			}
			if m := DetectProfile(p, v.Docroot); string(m.Profile) != e.Profile {
				t.Errorf("%s: profile %s (%s), hte-cli %s", e.Domain, m.Profile, m.Source, e.Profile)
			}
		}
	}
}

func TestDetectProfile(t *testing.T) {
	inv := scanFixture(t, "V1")
	m := DetectProfile(inv.Pools["/etc/php/8.3/fpm/pool.d/forced.test.conf"], "/home/dev/sites/forced/public")
	if m.Profile != profiles.Dev || m.Source != ProfileFromValues || len(m.Diffs) != 0 {
		t.Fatalf("V1 pool: %+v", m)
	}
	inv = scanFixture(t, "Malformed")
	m = DetectProfile(inv.Pools["/etc/php/8.3/fpm/pool.d/quoted.test.conf"], "/home/dev/sites/my quoted/public")
	if m.Profile != profiles.Dev || m.Source != ProfileFromDefault || m.Warning == "" || len(m.Diffs) != 0 {
		t.Fatalf("custom marker: %+v", m)
	}
	m = DetectProfile(inv.Pools["/etc/php/8.3/fpm/pool.d/nocert.test.conf"], "/home/dev/sites/nocert/public")
	if m.Profile != profiles.Hardened || m.Source != ProfileFromMarker || len(m.Diffs) != 0 {
		t.Fatalf("HARDENED marker: %+v", m)
	}
	// An extra value and a changed value are differences.
	p, err := ParsePool("x.conf", []byte("[x.test]\nuser = dev\n"+
		"php_admin_value[disable_functions] = apache_child_terminate,apache_get_modules,apache_getenv,apache_note,apache_setenv\n"+
		"php_admin_flag[allow_url_fopen] = off\npm = dynamic\npm.max_children = 9\npm.start_servers = 2\npm.min_spare_servers = 1\n"+
		"pm.max_spare_servers = 3\nchdir = /\ncatch_workers_output = yes\nrequest_terminate_timeout = 180s\n"+
		"php_flag[display_errors] = off\nphp_admin_flag[log_errors] = on\nphp_admin_value[post_max_size] = 128M\n"+
		"php_admin_value[upload_max_filesize] = 128M\nphp_admin_value[memory_limit] = 1024M\nphp_value[memory_limit] = 1024M\n"+
		"php_value[short_open_tag] = On\nphp_admin_value[max_input_vars] = 5000\n"))
	if err != nil {
		t.Fatal(err)
	}
	m = DetectProfile(p, "/srv")
	want := []PoolDiff{
		{Key: "pm.max_children", Pool: "9", Profile: "5", Line: 6},
		{Key: "php_admin_value[max_input_vars]", Pool: "5000", Line: 20},
	}
	if m.Profile != profiles.Dev || m.Source != ProfileFromDefault || !cmp.Equal(want, m.Diffs) {
		t.Fatalf("diffs: %+v", m)
	}
	if k, ok := INIKey("php_admin_value[max_input_vars]"); !ok || k != "max_input_vars" {
		t.Fatal(k)
	}
	if _, ok := INIKey("pm.max_children"); ok {
		t.Fatal("pm.max_children is not a php.ini setting")
	}
	for _, bad := range []string{"user = dev\n", "[x]\nnot a value\n", "; only a comment\n"} {
		if _, err := ParsePool("bad.conf", []byte(bad)); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestInventoryExtras(t *testing.T) {
	inv := scanFixture(t, "Malformed")
	if diff := cmp.Diff([]string{"001-019"}, inv.IndexGaps()); diff != "" {
		t.Fatal(diff)
	}
	var pools []string
	for p := range inv.Pools {
		pools = append(pools, p)
	}
	slices.Sort(pools)
	if diff := cmp.Diff([]string{
		"/etc/php/7.4/fpm/pool.d/aaa-notes.conf",
		"/etc/php/8.3/fpm/pool.d/nocert.test.conf",
		"/etc/php/8.3/fpm/pool.d/quoted.test.conf",
		"/etc/php/8.3/fpm/pool.d/valid.test.conf",
		"/etc/php/custom/fpm/pool.d/orphan.test.conf",
	}, pools); diff != "" {
		t.Fatal(diff)
	}
	inv = scanFixture(t, "V2")
	if len(inv.Hosts) != 3 || inv.Hosts[2].Line != 8 || !slices.Equal(inv.Hosts[2].Names, []string{"forced.test", "www.forced.test"}) {
		t.Fatalf("hosts: %+v", inv.Hosts)
	}
	if len(inv.CertFiles) != 6 || inv.IndexGaps() != nil {
		t.Fatalf("certs %v gaps %v", inv.CertFiles, inv.IndexGaps())
	}
}
