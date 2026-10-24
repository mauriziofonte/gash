// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build hte_e2e

package e2e

import (
	"context"
	"crypto/x509"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

// sessionPHP starts a session and writes it: on a platform whose
// session.save_path the user cannot write (Fedora's /var/lib/php/session),
// the pools must give PHP a directory of the user's (LEDGER HTE-41).
const sessionPHP = `<?php
$ok = session_start();
$_SESSION["n"] = ($_SESSION["n"] ?? 0) + 1;
$ok = session_write_close() && $ok;
echo $ok ? "session:ok\n" : "session:fail\n";
`

// stackProcs matches the names of the Apache and PHP-FPM processes of
// every packaging, for pgrep -x (a command line can name them in an
// argument: the setup's own runuser line does).
const stackProcs = "(httpd|apache2|php-fpm.*)"

// TestE2EDistro runs in the Fedora and Arch images (run.sh fedora|arch),
// which have no hte-cli, and on a macOS CI runner (macos.sh): the system
// httpd and two PHP-FPM packagings side by side, as installed and never
// started (Fedora: the distro php-fpm and Remi's php83; Arch: php and
// php-legacy; macOS: Homebrew's httpd, php and php@8.3).
// GASH_HTE_E2E_LAYOUTS names the discovery layouts the system must show. The story: discovery, install on
// the standard ports (where a stock vhost of the distro httpd blocks it,
// that refusal, then the high ports), a site on each version (HTTP, HTTPS
// with HTTP/2, sessions), php global set on one version, site modify
// --php (the unused master stops), the service commands, doctor, site
// remove and uninstall; setup.sh then compares /etc with its state before
// install.
func TestE2EDistro(t *testing.T) {
	layouts := strings.Fields(os.Getenv("GASH_HTE_E2E_LAYOUTS"))
	if len(layouts) == 0 || !sandboxed() {
		t.Skip("runs only inside the Fedora or Arch E2E container (testdata/hte/e2e/run.sh fedora|arch) or on a macOS CI runner (macos.sh)")
	}
	if os.Geteuid() == 0 {
		t.Fatal("the E2E test runs as an unprivileged user")
	}
	ctx := context.Background()
	e, err := hte.ProcessEngine(false)
	if err != nil {
		t.Fatal(err)
	}
	home := os.Getenv("HOME")
	paths := state.NewPaths(mustDirs(t), os.Getuid())
	me := e.User
	step := func(name string, fn func(t *testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	// versions[i] is the PHP of layouts[i].
	var versions []string
	full := map[string]string{}
	step("discovery finds each packaging", func(t *testing.T) {
		res, err := e.PHPVersions(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range res.Versions {
			t.Logf("%+v", p)
		}
		if len(res.Problems) != 0 {
			t.Errorf("problems %v", res.Problems)
		}
		for _, l := range layouts {
			i := slices.IndexFunc(res.Versions, func(p discover.PHP) bool {
				return string(p.Layout) == l && !slices.Contains(versions, p.Version)
			})
			if i < 0 {
				t.Fatalf("no %s version in %+v", l, res.Versions)
			}
			p := res.Versions[i]
			out, err := exec.Command(p.FPM, "-v").Output()
			if err != nil || !strings.HasPrefix(string(out), "PHP "+p.Full+" (fpm-fcgi)") {
				t.Errorf("%s -v: %q %v, discovery says %s", p.FPM, out, err, p.Full)
			}
			if !exists(p.INI) {
				t.Errorf("%s: no %s", p.FPM, p.INI)
			}
			versions = append(versions, p.Version)
			full[p.Version] = p.Full
		}
		if len(versions) != 2 || versions[0] == versions[1] {
			t.Fatalf("versions %v: the image needs two different ones", versions)
		}
	})

	// GASH_HTE_E2E_BLOCKING names the stock vhosts of the distro httpd
	// (Fedora: mod_ssl's ssl.conf with its _default_:443 vhost): install
	// on the standard ports then leaves the stack stopped (HTE-75), and
	// the story goes on, after an uninstall, on the high ports.
	blocking := strings.Fields(os.Getenv("GASH_HTE_E2E_BLOCKING"))
	httpPort, httpsPort := 80, 443
	step("install on the standard ports", func(t *testing.T) {
		res, err := e.Install(ctx, hte.InstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("plan: %s", strings.Join(res.Plan, " | "))
		if fi, err := os.Stat(rootops.HelperPath); err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("helper: %v %v", fi, err)
		}
		if runtime.GOOS == "linux" {
			if got := strings.TrimSpace(read(t, "/proc/sys/net/ipv4/ip_unprivileged_port_start")); got != "80" {
				t.Errorf("ip_unprivileged_port_start = %s", got)
			}
		} else {
			// macOS: a direct bind where the kernel allows it, else the
			// pf redirect.
			t.Logf("redirect %q", res.Redirect)
		}
		if read(t, receipt.RootPath(me)) != read(t, paths.Receipt()) {
			t.Error("the two receipt copies differ")
		}
		// autostart auto: the LaunchAgent on macOS; nothing in a
		// container without systemd.
		want := map[string]string{"darwin": service.AutostartLaunchd}[runtime.GOOS]
		if want == "" {
			want = service.AutostartNone
		}
		if res.Settings.Autostart != want {
			t.Errorf("autostart %q, want %q", res.Settings.Autostart, want)
		}
		if agent := service.LaunchAgentPath(home); exists(agent) != (want == service.AutostartLaunchd) {
			t.Errorf("LaunchAgent %s exists %v with autostart %s", agent, exists(agent), want)
		}
		if !slices.Equal(res.Blocking, blocking) {
			t.Fatalf("blocking %v, want %v", res.Blocking, blocking)
		}
		if len(blocking) == 0 && !res.Started {
			t.Errorf("the stack did not start: %+v", res)
		}
		if len(blocking) > 0 && (res.Started || res.Next != "gash hte migrate") {
			t.Errorf("started %v, next %q, with blocking vhosts", res.Started, res.Next)
		}
		again, err := e.Install(ctx, hte.InstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(again.Changes) != 0 {
			t.Errorf("a second install changed %+v", again.Changes)
		}
	})

	if len(blocking) > 0 {
		step("migrate names the stock vhosts, install on the high ports", func(t *testing.T) {
			res, err := e.Migrate(ctx, hte.MigrateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, b := range res.Plan.Blocking {
				got = append(got, b.Path)
			}
			if !slices.Equal(got, blocking) {
				t.Errorf("migrate blocking %v, want %v", got, blocking)
			}
			if _, err := e.Uninstall(ctx, hte.UninstallOptions{Yes: true}); err != nil {
				t.Fatal(err)
			}
			ins, err := e.Install(ctx, hte.InstallOptions{Yes: true, Ports: "high", NoRestore: true})
			if err != nil {
				t.Fatal(err)
			}
			if !ins.Started || len(ins.Blocking) != 0 {
				t.Fatalf("install --ports high: %+v", ins)
			}
		})
		httpPort, httpsPort = 8080, 8443
	}

	domains := []string{"one.test", "two.test"}
	step("a site on each version", func(t *testing.T) {
		for i, d := range domains {
			root := filepath.Join(home, "www", d, "public")
			if err := os.MkdirAll(root, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, body := range map[string]string{
				"info.php":    `<?php echo PHP_VERSION, " ", ini_get("max_input_vars"), "\n";` + "\n",
				"session.php": sessionPHP,
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := e.SiteCreate(ctx, state.Flags{Domain: d, Docroot: ptr(root), PHP: ptr(versions[i])}); err != nil {
				t.Fatalf("%s: %v", d, err)
			}
			answersWith(t, "http", d, httpPort, "/info.php", full[versions[i]]+" 1000")
			answersWith(t, "https", d, httpsPort, "/info.php", full[versions[i]]+" 1000")
			answersWith(t, "http", d, httpPort, "/session.php", "session:ok")
			a := mustGet(t, "https", d, httpsPort)
			if a.proto != 2 || a.cert == nil || a.cert.PublicKeyAlgorithm != x509.ECDSA {
				t.Errorf("%s over TLS: HTTP/%d, certificate %+v", d, a.proto, a.cert)
			}
			if !strings.Contains(read(t, "/etc/hosts"), d) {
				t.Errorf("the hosts block lacks %s", d)
			}
		}
		want := []string{"php-fpm " + versions[0], "php-fpm " + versions[1]}
		slices.Sort(want)
		if got := fpmMasters(t, e); !slices.Equal(got, want) {
			t.Errorf("masters %v, want %v", got, want)
		}
		// HTE-18: the masters and workers are the user's, none is root's.
		for _, args := range [][]string{{"-u", me, "-x", "(httpd|apache2)"}, {"-u", me, "-f", "php-fpm: master process"}} {
			if exec.Command("pgrep", args...).Run() != nil {
				t.Errorf("pgrep %v: no process", args)
			}
		}
		if out, err := exec.Command("pgrep", "-l", "-u", "root", "-x", stackProcs).Output(); err == nil {
			t.Errorf("root processes of the stack: %s", out)
		}
	})

	step("php global set changes one version", func(t *testing.T) {
		if _, err := e.PHPGlobalSet(ctx, versions[1], "max_input_vars", "1111"); err != nil {
			t.Fatal(err)
		}
		answersWith(t, "http", domains[1], httpPort, "/info.php", full[versions[1]]+" 1111")
		answersWith(t, "http", domains[0], httpPort, "/info.php", full[versions[0]]+" 1000")
		if _, err := e.PHPGlobalUnset(ctx, versions[1], "max_input_vars"); err != nil {
			t.Fatal(err)
		}
		answersWith(t, "http", domains[1], httpPort, "/info.php", full[versions[1]]+" 1000")
	})

	step("site modify --php, the unused master stops", func(t *testing.T) {
		res, err := e.SiteModify(ctx, state.Flags{Domain: domains[1], PHP: ptr(versions[0])})
		if err != nil || !res.Changed {
			t.Fatalf("%+v %v", res, err)
		}
		answersWith(t, "http", domains[1], httpPort, "/info.php", full[versions[0]]+" ")
		answersWith(t, "http", domains[1], httpPort, "/session.php", "session:ok")
		if got := fpmMasters(t, e); !slices.Equal(got, []string{"php-fpm " + versions[0]}) {
			t.Errorf("masters %v", got)
		}
	})

	step("stop, start, restart, status, logs", func(t *testing.T) {
		if _, err := e.ServiceStop(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := get("http", domains[0], httpPort); err == nil {
			t.Errorf("port %d answers after stop", httpPort)
		}
		if out, err := exec.Command("pgrep", "-l", "-u", me, "-x", stackProcs).Output(); err == nil {
			t.Errorf("processes after stop: %s", out)
		}
		if _, err := e.ServiceStart(ctx); err != nil {
			t.Fatal(err)
		}
		st, err := e.ServiceStatus(ctx)
		if err != nil || !st.Running {
			t.Fatalf("status after start: %+v %v", st, err)
		}
		pid := st.Apache.PID
		if _, err := e.ServiceRestart(ctx); err != nil {
			t.Fatal(err)
		}
		if st, _ = e.ServiceStatus(ctx); st.Apache.PID == pid || !st.Running {
			t.Errorf("restart kept Apache pid %d: %+v", pid, st.Apache)
		}
		answersWith(t, "https", domains[0], httpsPort, "/info.php", full[versions[0]]+" ")
		logs, err := e.ServiceLogs(ctx, domains[0], 5)
		if err != nil || len(logs) == 0 {
			t.Errorf("logs: %v %v", logs, err)
		}
	})

	step("doctor", func(t *testing.T) {
		res, err := e.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		for _, c := range res.Checks {
			t.Logf("%-28s %-5s %s %s", c.Name, c.Status, c.Details, c.Remedy)
		}
		if !res.OK {
			t.Error("doctor reports failures")
		}
	})

	step("site remove and uninstall", func(t *testing.T) {
		for _, d := range domains {
			if _, err := e.SiteRemove(ctx, d, true); err != nil {
				t.Fatal(err)
			}
		}
		res, err := e.Uninstall(ctx, hte.UninstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("stage %s, restored %d changes, warnings %v", res.Stage, len(res.Restored), res.Warnings)
		for _, p := range []string{rootops.HelperPath, rootops.SudoersPath, receipt.RootPath(me), paths.ConfigDir, service.LaunchAgentPath(home)} {
			if exists(p) {
				t.Errorf("%s stays", p)
			}
		}
		if out, err := exec.Command("pgrep", "-l", "-x", stackProcs).Output(); err == nil {
			t.Errorf("processes after uninstall: %s", out)
		}
		if _, err := get("http", domains[0], httpPort); err == nil {
			t.Errorf("port %d answers after uninstall", httpPort)
		}
	})
}
