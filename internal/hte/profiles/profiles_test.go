// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

const legacy = "../../../testdata/hte/legacy"

// The profile blocks reproduce the pools hte-cli 1.1.1 rendered
// (testdata/hte/legacy/templates/v2-pool-*.conf, produced by running
// PhpProfiles::getFullConfig) byte for byte, given hte-cli's header and log
// paths.
func TestBlocksMatchHTECLIPools(t *testing.T) {
	for _, n := range All() {
		t.Run(string(n), func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join(legacy, "templates", "v2-pool-"+string(n)+".conf"))
			if err != nil {
				t.Fatal(err)
			}
			header := "[{domain}]\n; Profile: " + string(n) + "\nuser = {user}\ngroup = {group}\n" +
				"listen = /var/run/php/php{phpver}-fpm-{domain}.sock\nlisten.owner = {user}\nlisten.group = {group}\nlisten.mode = 0660\n\n"
			got := header + Text(Lines(n, Options{
				Docroot:  "{docroot}",
				SlowLog:  "{docroot}/php{phpver}-fpm-slow.log",
				ErrorLog: "{docroot}/php{phpver}-fpm-errors.log",
			}))
			if diff := cmp.Diff(string(want), got); diff != "" {
				t.Fatalf("(-hte-cli +gash):\n%s", diff)
			}
		})
	}
}

// The concrete V2 fixture pools carry the same values.
func TestValuesMatchV2FixturePools(t *testing.T) {
	cases := map[string]Name{
		"V2/etc/php/8.3/fpm/pool.d/http.test.conf":   Dev,
		"V2/etc/php/7.4/fpm/pool.d/https.test.conf":  Staging,
		"V2/etc/php/8.3/fpm/pool.d/forced.test.conf": Hardened,
		"V1/etc/php/8.3/fpm/pool.d/http.test.conf":   Dev,
	}
	docroots := map[string]string{"http.test": "/home/dev/sites/http/public", "https.test": "/home/dev/sites/https/public",
		"forced.test": "/home/dev/sites/forced/public"}
	for file, n := range cases {
		b, err := os.ReadFile(filepath.Join(legacy, file))
		if err != nil {
			t.Fatal(err)
		}
		domain := strings.TrimSuffix(filepath.Base(file), ".conf")
		got := map[string]string{}
		for _, line := range strings.Split(string(b), "\n") {
			k, v, ok := strings.Cut(line, "=")
			if !ok || strings.HasPrefix(line, ";") {
				continue
			}
			k, v = strings.TrimSpace(k), strings.Join(strings.Fields(v), " ")
			switch k {
			case "user", "group", "listen", "listen.owner", "listen.group", "listen.mode", "slowlog", "php_admin_value[error_log]":
				continue
			}
			got[k] = v
		}
		if diff := cmp.Diff(Values(n, docroots[domain]), got); diff != "" {
			t.Errorf("%s (-profile +pool):\n%s", file, diff)
		}
	}
}

func TestParseAndDescriptions(t *testing.T) {
	for _, s := range []string{"dev", "staging", "hardened"} {
		if n, ok := Parse(s); !ok || string(n) != s || Description(n) == "Unknown profile" {
			t.Errorf("Parse(%q)", s)
		}
	}
	for _, s := range []string{"", "Dev", "HARDENED", "custom", " dev"} {
		if _, ok := Parse(s); ok {
			t.Errorf("Parse(%q) accepted", s)
		}
	}
}

func TestOpenBasedirExtraAndPathMarks(t *testing.T) {
	lines := Lines(Staging, Options{Docroot: "/d", SlowLog: "/l/slow", ErrorLog: "/l/err", OpenBasedirExtra: []string{"/s/8.3/sessions"}})
	var paths []string
	for _, l := range lines {
		if l.Path {
			paths = append(paths, l.Key+"="+l.Value)
		}
	}
	want := []string{"slowlog=/l/slow", "php_admin_value[error_log]=/l/err",
		"php_admin_value[open_basedir]=/d:" + DefaultOpenBasedir + ":/s/8.3/sessions"}
	if diff := cmp.Diff(want, paths); diff != "" {
		t.Fatal(diff)
	}
	if k, v := AdminDirective("expose_php", "off"); k != "php_admin_flag[expose_php]" || v != "off" {
		t.Fatal(k, v)
	}
	if k, v := AdminDirective("auto_prepend_file", ""); k != "php_admin_value[auto_prepend_file]" || v != `""` {
		t.Fatal(k, v)
	}
}
