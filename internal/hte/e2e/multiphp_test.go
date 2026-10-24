// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build hte_e2e

package e2e

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// phpVersions are the PHP-FPM versions of the multi-PHP image: the distro
// 8.3 and the official builds of 7.4, 8.2 and 8.4 (Dockerfile.multiphp).
var phpVersions = []string{"7.4", "8.2", "8.3", "8.4"}

// fullVersion is what the image's php-fpmX.Y -v reports.
func fullVersion(t *testing.T, v string) string {
	t.Helper()
	out, err := exec.Command("/usr/sbin/php-fpm"+v, "-v").Output()
	if err != nil {
		t.Fatal(err)
	}
	f := strings.Fields(string(out))
	if len(f) < 2 {
		t.Fatalf("php-fpm%s -v: %q", v, out)
	}
	return f[1]
}

// answersWith waits up to 5 s for path on host to answer 200 with a body
// that starts with want (an FPM reload re-executes the master, so its pool
// sockets come back a moment after the signal).
func answersWith(t *testing.T, scheme, host string, port int, path, want string) {
	t.Helper()
	var a answer
	var err error
	for range 50 {
		if a, err = getPath(scheme, host, port, path, nil); err == nil && a.status == 200 && strings.HasPrefix(a.body, want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Errorf("%s://%s:%d%s: %+v %v, want a body starting with %q", scheme, host, port, path, a, err, want)
}

// fpmMasters are the names of the stack's running FPM masters.
func fpmMasters(t *testing.T, e *hte.Engine) []string {
	t.Helper()
	st, err := e.ServiceStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range st.FPM {
		if m.Running {
			out = append(out, m.Name)
		}
	}
	return out
}

// systemFPMRuns reports the system FPM master of version v (root's).
func systemFPMRuns(v string) bool {
	return exec.Command("pgrep", "-u", "root", "-f", "php-fpm: master process \\(/etc/php/"+v+"/fpm/php-fpm.conf\\)").Run() == nil
}

// TestE2EMultiPHP runs in the multi-PHP image (run.sh with
// GASH_HTE_E2E_MULTIPHP=1). The hte-cli V2 tree keeps https.test on the
// system PHP 7.4 (an official php:7.4-fpm build, a SysV service of its
// own), the other sites on the distro 8.3. Migrate moves each site to a
// userland master of its own version; sites on 8.2 and 8.4 run next to
// them; site modify --php moves a site between versions and the master
// nobody uses any more stops; php global set changes one version only;
// uninstall gives the 7.4 site back to the system PHP 7.4.
func TestE2EMultiPHP(t *testing.T) {
	if os.Getenv("GASH_HTE_E2E_MULTIPHP") != "1" || !inContainer() {
		t.Skip("runs only inside the multi-PHP E2E container: GASH_HTE_E2E_MULTIPHP=1 testdata/hte/e2e/run.sh")
	}
	if os.Geteuid() == 0 {
		t.Fatal("the E2E test runs as an unprivileged user")
	}
	ctx := context.Background()
	e, err := hte.ProcessEngine(false)
	if err != nil {
		t.Fatal(err)
	}
	uninstallOnFailure(t, e)
	home := os.Getenv("HOME")
	full := map[string]string{}
	for _, v := range phpVersions {
		full[v] = fullVersion(t, v)
	}
	step := func(name string, fn func(t *testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	step("discovery finds every version", func(t *testing.T) {
		res, err := e.PHPVersions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, p := range res.Versions {
			got = append(got, p.Version)
			if p.FPM != "/usr/sbin/php-fpm"+p.Version || p.INI != "/etc/php/"+p.Version+"/fpm/php.ini" || p.Full != full[p.Version] {
				t.Errorf("%+v", p)
			}
		}
		if !slices.Equal(got, phpVersions) || len(res.Problems) != 0 {
			t.Fatalf("versions %v, problems %v", got, res.Problems)
		}
	})

	step("migrate moves each site to a master of its version", func(t *testing.T) {
		ins, err := e.Install(ctx, hte.InstallOptions{Yes: true})
		if err != nil || ins.Next != "gash hte migrate" {
			t.Fatalf("install: %+v %v", ins, err)
		}
		res, err := e.Migrate(ctx, hte.MigrateOptions{Apply: true, Exclude: []string{defaultVhost}, Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Switched || len(res.Imported) != 3 {
			t.Fatalf("migrate: %+v", res)
		}
		for d, v := range map[string]string{"http.test": "8.3", "https.test": "7.4", "forced.test": "8.3"} {
			sh, err := e.SiteShow(ctx, d)
			if err != nil || sh.Site.PHP != v {
				t.Errorf("%s: %+v %v, want PHP %s", d, sh, err, v)
			}
		}
		if exists("/etc/php/7.4/fpm/pool.d/https.test.conf") {
			t.Error("the 7.4 pool of https.test was not archived")
		}
		// The system FPM 7.4 keeps its www pool, so it was reloaded, not
		// stopped.
		if !systemFPMRuns("7.4") {
			t.Error("the system PHP-FPM 7.4 stopped")
		}
		answersWith(t, "https", "https.test", 443, "/info.php", full["7.4"]+" ")
		answersWith(t, "http", "http.test", 80, "/info.php", full["8.3"]+" ")
		if got := fpmMasters(t, e); !slices.Equal(got, []string{"php-fpm 7.4", "php-fpm 8.3"}) {
			t.Errorf("masters %v", got)
		}
	})

	sites := map[string]string{"v82.test": "8.2", "v84.test": "8.4"}
	step("a site on each other version", func(t *testing.T) {
		for d, v := range sites {
			root := filepath.Join(home, "www", d, "public")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "info.php"), []byte(`<?php echo PHP_VERSION, " ", ini_get("max_input_vars"), "\n";`), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := e.SiteCreate(ctx, state.Flags{Domain: d, Docroot: ptr(root), PHP: ptr(v)}); err != nil {
				t.Fatalf("%s: %v", d, err)
			}
			answersWith(t, "http", d, 80, "/info.php", full[v]+" ")
			answersWith(t, "https", d, 443, "/info.php", full[v]+" ")
		}
		want := []string{"php-fpm 7.4", "php-fpm 8.2", "php-fpm 8.3", "php-fpm 8.4"}
		if got := fpmMasters(t, e); !slices.Equal(got, want) {
			t.Errorf("masters %v, want %v", got, want)
		}
		out, _ := exec.Command("pgrep", "-u", "dev", "-c", "-f", "php-fpm: master process").Output()
		if strings.TrimSpace(string(out)) != "4" {
			t.Errorf("%s FPM masters of dev run, want 4", strings.TrimSpace(string(out)))
		}
	})

	step("site modify --php moves a site, the unused master stops", func(t *testing.T) {
		res, err := e.SiteModify(ctx, state.Flags{Domain: "v82.test", PHP: ptr("8.4")})
		if err != nil || !res.Changed {
			t.Fatalf("%+v %v", res, err)
		}
		answersWith(t, "http", "v82.test", 80, "/info.php", full["8.4"]+" ")
		want := []string{"php-fpm 7.4", "php-fpm 8.3", "php-fpm 8.4"}
		if got := fpmMasters(t, e); !slices.Equal(got, want) {
			t.Errorf("masters %v, want %v", got, want)
		}
		if out, err := exec.Command("pgrep", "-u", "dev", "-a", "-f", "fpm/8.2/").Output(); err == nil {
			t.Errorf("the 8.2 master still runs: %s", out)
		}
	})

	step("php global set changes one version", func(t *testing.T) {
		if _, err := e.PHPGlobalSet(ctx, "7.4", "max_input_vars", "1111"); err != nil {
			t.Fatal(err)
		}
		answersWith(t, "https", "https.test", 443, "/info.php", full["7.4"]+" 1111")
		answersWith(t, "http", "http.test", 80, "/info.php", full["8.3"]+" 1000")
		answersWith(t, "http", "v84.test", 80, "/info.php", full["8.4"]+" 1000")
		if _, err := e.PHPGlobalUnset(ctx, "7.4", "max_input_vars"); err != nil {
			t.Fatal(err)
		}
		answersWith(t, "https", "https.test", 443, "/info.php", full["7.4"]+" 1000")
	})

	step("uninstall gives the 7.4 site back to the system PHP 7.4", func(t *testing.T) {
		for d := range sites {
			if _, err := e.SiteRemove(ctx, d, true); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := e.Uninstall(ctx, hte.UninstallOptions{Yes: true}); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("pgrep", "-u", "dev", "-a", "-f", "php-fpm: master process").Output(); err == nil {
			t.Errorf("FPM masters of dev after uninstall: %s", out)
		}
		if !systemApacheRuns() || !systemFPMRuns("7.4") || !exists("/etc/php/7.4/fpm/pool.d/https.test.conf") {
			t.Fatalf("system Apache %v, system FPM 7.4 %v", systemApacheRuns(), systemFPMRuns("7.4"))
		}
		answersWith(t, "https", "https.test", 443, "/info.php", full["7.4"]+" ")
		answersWith(t, "https", "https.test", 443, "/", "legacy:https.test:fpm-fcgi")
	})
}
