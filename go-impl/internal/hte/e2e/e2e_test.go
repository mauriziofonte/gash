// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

//go:build hte_e2e

// Package e2e proves gash hte on a real system Apache and PHP-FPM, inside
// the disposable container of testdata/hte/e2e/run.sh: it runs there as
// the unprivileged user dev, and as root only through sudo, as the root
// step (`<binary> hte __root`). It refuses to run anywhere else.
package e2e

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/hte/rootops"
	"github.com/mauriziofonte/gash/internal/hte/state"
	"github.com/mauriziofonte/gash/internal/platform"
)

func inContainer() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// TestMain is also the root step: sudo runs this binary as
// `gash-e2e hte __root`, as it runs gash.
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "hte" && os.Args[2] == "__root" {
		if !inContainer() {
			fmt.Fprintln(os.Stderr, "the E2E root step runs only inside the E2E container")
			os.Exit(2)
		}
		if err := rootops.Serve(context.Background(), os.Stdin, os.Stdout, rootops.ProcessEnv()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

const (
	defaultVhost = "/etc/apache2/sites-available/000-default.conf"
	appDomain    = "app.test"
)

var legacyVhosts = []string{
	"/etc/apache2/sites-available/001-http.test.conf",
	"/etc/apache2/sites-available/002-https.test.conf",
	"/etc/apache2/sites-available/003-forced.test.conf",
}

// answer is one HTTP exchange with a site on 127.0.0.1.
type answer struct {
	status   int
	location string
	body     string
	cert     *x509.Certificate
	proto    int // HTTP major version
}

func get(scheme, host string, port int) (answer, error) { return getPath(scheme, host, port, "/", nil) }

// getPath requests path; tlsConf, when set, replaces the client TLS
// settings (the ServerName is set to host).
func getPath(scheme, host string, port int, path string, tlsConf *tls.Config) (answer, error) {
	if tlsConf == nil {
		tlsConf = &tls.Config{InsecureSkipVerify: true}
	}
	tlsConf.ServerName = host
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	tr := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", addr)
		},
		TLSClientConfig:   tlsConf,
		ForceAttemptHTTP2: true,
	}
	c := &http.Client{Transport: tr, Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := c.Get(scheme + "://" + host + path)
	if err != nil {
		return answer{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	a := answer{status: resp.StatusCode, location: resp.Header.Get("Location"), body: strings.TrimSpace(string(body)), proto: resp.ProtoMajor}
	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		a.cert = resp.TLS.PeerCertificates[0]
	}
	return a, nil
}

func mustGet(t *testing.T, scheme, host string, port int) answer {
	t.Helper()
	a, err := get(scheme, host, port)
	if err != nil {
		t.Fatalf("%s://%s on %d: %v", scheme, host, port, err)
	}
	return a
}

func code(err error) hteerr.Code {
	if he, ok := hteerr.As(err); ok {
		return he.Code
	}
	return ""
}

func systemApacheRuns() bool {
	return exec.Command("pgrep", "-u", "root", "-x", "apache2").Run() == nil
}

func read(t *testing.T, p string) string {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

func ptr[T any](v T) *T { return &v }

// TestE2E is one story: install next to the running system Apache,
// migrate its sites, run the stack, uninstall. Each step needs the ones
// before it, so the first failure ends the story.
func TestE2E(t *testing.T) {
	if os.Getenv("GASH_HTE_E2E") != "1" || !inContainer() {
		t.Skip("runs only inside the E2E container: testdata/hte/e2e/run.sh")
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
	step := func(name string, fn func(t *testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}

	step("install next to the system Apache", func(t *testing.T) {
		res, err := e.Install(ctx, hte.InstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("plan: %s", strings.Join(res.Plan, " | "))
		if res.Started {
			t.Error("the stack started while the system Apache holds 80 and 443")
		}
		for _, p := range append([]string{defaultVhost}, legacyVhosts...) {
			if !slices.Contains(res.Blocking, p) {
				t.Errorf("blocking %v lacks %s", res.Blocking, p)
			}
		}
		if slices.ContainsFunc(res.Plan, func(l string) bool { return strings.Contains(l, "stop and disable") }) {
			t.Errorf("the plan stops the system Apache: %v", res.Plan)
		}
		if !systemApacheRuns() {
			t.Error("install stopped the system Apache")
		}
		if fi, err := os.Stat(rootops.HelperPath); err != nil || fi.Mode().Perm() != 0o755 {
			t.Errorf("helper: %v %v", fi, err)
		}
		if got := strings.TrimSpace(read(t, "/proc/sys/net/ipv4/ip_unprivileged_port_start")); got != "80" {
			t.Errorf("ip_unprivileged_port_start = %s", got)
		}
		root, user := read(t, receipt.RootPath("dev")), read(t, paths.Receipt())
		if root != user || !strings.Contains(root, `"kind":"sysctl"`) {
			t.Errorf("receipt copies:\nroot %s\nuser %s", root, user)
		}
		again, err := e.Install(ctx, hte.InstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(again.Changes) != 0 {
			t.Errorf("a second install changed %+v", again.Changes)
		}
		if read(t, receipt.RootPath("dev")) != root {
			t.Error("a second install changed the receipt")
		}
	})

	step("migrate dry run", func(t *testing.T) {
		res, err := e.Migrate(ctx, hte.MigrateOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if res.Applied {
			t.Error("a dry run applied")
		}
		if len(res.Plan.Blocking) != 1 || res.Plan.Blocking[0].Exclude != "--exclude "+defaultVhost {
			t.Errorf("blocking %+v", res.Plan.Blocking)
		}
		imports := 0
		for _, sp := range res.Plan.Sites {
			if sp.Action == "import" {
				imports++
			}
		}
		if imports != 3 {
			t.Errorf("%d imports in %+v", imports, res.Plan.Sites)
		}
		t.Logf("root steps: %s", strings.Join(res.RootPlan, " | "))
	})

	step("migrate --apply needs --yes", func(t *testing.T) {
		_, err := e.Migrate(ctx, hte.MigrateOptions{Apply: true, Exclude: []string{defaultVhost}})
		if code(err) != hteerr.ConfirmationRequired {
			t.Fatalf("%v, want confirmation_required", err)
		}
		if !systemApacheRuns() {
			t.Fatal("the refused migrate stopped the system Apache")
		}
	})

	step("migrate --apply", func(t *testing.T) {
		res, err := e.Migrate(ctx, hte.MigrateOptions{Apply: true, Exclude: []string{defaultVhost}, Yes: true})
		if res != nil {
			t.Logf("staging %s, switched %v, imported %v, archived %v, warnings %v", res.Staging, res.Switched, res.Imported, res.Archived, res.Warnings)
			for _, v := range res.Verified {
				t.Logf("verified %+v", v)
			}
		}
		if err != nil {
			t.Fatal(err)
		}
		if !res.Switched || res.Staging == "" || len(res.Imported) != 3 {
			t.Fatalf("migrate: %+v", res)
		}
		if systemApacheRuns() {
			t.Error("the system Apache still runs")
		}
		for _, p := range legacyVhosts {
			if !slices.Contains(res.Archived, p) || exists(p) {
				t.Errorf("%s not archived", p)
			}
		}
		for _, p := range []string{"/etc/php/8.3/fpm/pool.d/http.test.conf", "/etc/php/8.3/fpm/pool.d/https.test.conf"} {
			if exists(p) {
				t.Errorf("%s not archived", p)
			}
		}
		if !exists("/etc/apache2/sites-enabled/000-default.conf") {
			t.Error("the excluded default vhost was disabled")
		}
		a := mustGet(t, "http", "http.test", 80)
		if a.status != 200 || a.body != "legacy:http.test:fpm-fcgi" {
			t.Errorf("http.test: %+v", a)
		}
		a = mustGet(t, "https", "https.test", 443)
		if a.status != 200 || a.body != "legacy:https.test:fpm-fcgi" || a.cert == nil || a.cert.PublicKeyAlgorithm != x509.RSA {
			t.Errorf("https.test (the copied hte-cli pair): %+v", a)
		}
		// V2 forced HTTPS keeps its loopback bypass: this client is
		// 127.0.0.1, so it is not redirected.
		if s := read(t, paths.SiteFile("forced.test")); !strings.Contains(s, "force_https = true") || !strings.Contains(s, "force_https_bypass_loopback = true") {
			t.Errorf("forced.test state:\n%s", s)
		}
		if a = mustGet(t, "http", "forced.test", 80); a.status != 200 {
			t.Errorf("forced.test over HTTP from loopback: %+v", a)
		}
	})

	step("doctor after the migration", func(t *testing.T) {
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
		if !slices.ContainsFunc(res.Checks, func(c hte.Check) bool {
			return c.Name == "certificate https.test" && c.Status == hte.CheckWarn && strings.Contains(c.Details, "hte-cli")
		}) {
			t.Error("doctor does not suggest renewing the copied RSA certificate")
		}
	})

	appRoot := filepath.Join(home, "www", "app", "public")
	step("site create", func(t *testing.T) {
		if err := os.MkdirAll(appRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		php := `<?php echo "app:", ini_get("memory_limit"), ":", ini_get("max_input_vars"), "\n";` + "\n"
		if err := os.WriteFile(filepath.Join(appRoot, "index.php"), []byte(php), 0o644); err != nil {
			t.Fatal(err)
		}
		res, err := e.SiteCreate(ctx, state.Flags{Domain: appDomain, Docroot: ptr(appRoot)})
		if err != nil {
			t.Fatal(err)
		}
		if !res.Changed || res.Applied == nil || !res.Applied.Changed || !res.Applied.Reloaded {
			t.Errorf("site create: %+v", res)
		}
		for _, scheme := range []string{"http", "https"} {
			port := map[string]int{"http": 80, "https": 443}[scheme]
			a := mustGet(t, scheme, appDomain, port)
			if a.status != 200 || a.body != "app:1024M:1000" {
				t.Errorf("%s: %+v", scheme, a)
			}
		}
		a := mustGet(t, "https", appDomain, 443)
		if a.cert == nil || a.cert.PublicKeyAlgorithm != x509.ECDSA || !slices.Contains(a.cert.DNSNames, "www."+appDomain) {
			t.Errorf("certificate: %+v", a.cert)
		}
		if strings.Count(read(t, "/etc/hosts"), appDomain) == 0 {
			t.Error("the hosts block lacks app.test")
		}
	})

	step("TLS, HTTP/2 and the certificate", func(t *testing.T) {
		// HTE-39: the pair verifies with cert.pem as the only root.
		pemData := read(t, paths.CertFile(appDomain))
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pemData)) {
			t.Fatal("cert.pem does not parse")
		}
		a, err := getPath("https", appDomain, 443, "/", &tls.Config{RootCAs: pool})
		if err != nil || a.status != 200 {
			t.Errorf("verified TLS: %+v %v", a, err)
		}
		// HTE-38: HTTP/2 over TLS.
		if a.proto != 2 {
			t.Errorf("HTTP/%d over TLS, want 2", a.proto)
		}
		// HTE-15: no TLS below 1.2.
		if _, err := getPath("https", appDomain, 443, "/", &tls.Config{InsecureSkipVerify: true, MaxVersion: tls.VersionTLS11}); err == nil {
			t.Error("a TLS 1.1 handshake was accepted")
		}
	})

	step("logs stay out of the document root", func(t *testing.T) {
		// HTE-07, HTE-48.
		warn := `<?php trigger_error("e2e-warning", E_USER_WARNING); echo "ok\n";` + "\n"
		if err := os.WriteFile(filepath.Join(appRoot, "warn.php"), []byte(warn), 0o644); err != nil {
			t.Fatal(err)
		}
		before, _ := os.ReadDir(appRoot)
		if a, err := getPath("http", appDomain, 80, "/warn.php", nil); err != nil || a.body != "ok" {
			t.Fatalf("warn.php: %+v %v", a, err)
		}
		logDir := paths.SiteLogDir(appDomain)
		if !eventually(filepath.Join(logDir, "php-error.log"), "e2e-warning") {
			t.Error("the PHP warning is not in php-error.log")
		}
		if !eventually(filepath.Join(logDir, "access.log"), "/warn.php") {
			t.Error("the request is not in access.log")
		}
		if after, _ := os.ReadDir(appRoot); len(after) != len(before) {
			t.Errorf("the document root gained files: %v", after)
		}
		// HTE-18: the masters and workers are the user's.
		for _, name := range []string{"apache2", "php-fpm8.3"} {
			if exec.Command("pgrep", "-u", "dev", "-x", name).Run() != nil {
				t.Errorf("no %s process of dev", name)
			}
		}
	})

	step("an overlay error leaves the sites answering", func(t *testing.T) {
		// HTE-09: the config test refuses the generation; nothing moves.
		overlay := paths.SiteOverlay(appDomain)
		if err := os.WriteFile(overlay, []byte("ThisIsNotADirective on\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := e.ServiceReload(ctx)
		if code(err) != hteerr.ConfigTestFailed {
			t.Errorf("reload with a broken overlay: %v, want config_test_failed", err)
		}
		if err := os.Remove(overlay); err != nil {
			t.Fatal(err)
		}
		for _, d := range []string{appDomain, "http.test"} {
			if a := mustGet(t, "http", d, 80); a.status != 200 {
				t.Errorf("%s: %+v", d, a)
			}
		}
	})

	step("forced HTTPS without the bypass", func(t *testing.T) {
		// HTE-10, HTE-52: 307 to HTTPS, also from loopback.
		if _, err := e.SiteModify(ctx, state.Flags{Domain: appDomain, ForceHTTPS: ptr(true)}); err != nil {
			t.Fatal(err)
		}
		if a := mustGet(t, "http", appDomain, 80); a.status != 307 || a.location != "https://"+appDomain+"/" {
			t.Errorf("forced: %+v", a)
		}
		if _, err := e.SiteModify(ctx, state.Flags{Domain: appDomain, ForceHTTPS: ptr(false)}); err != nil {
			t.Fatal(err)
		}
	})

	step("a document root with a space", func(t *testing.T) {
		// HTE-08: the root is quoted in the vhost and the pool, and the
		// vhost gives PHP-FPM the decoded SCRIPT_FILENAME.
		root := filepath.Join(home, "www", "with space", "public")
		if err := os.MkdirAll(root, 0o755); err != nil {
			t.Fatal(err)
		}
		php := "<?php echo 'space:', $_SERVER['PATH_INFO'] ?? '', ':', $_GET['q'] ?? '', \"\\n\";\n"
		if err := os.WriteFile(filepath.Join(root, "index.php"), []byte(php), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := e.SiteCreate(ctx, state.Flags{Domain: "space.test", Docroot: ptr(root), HTTPS: ptr(false)}); err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{"/index.php": "space::", "/index.php/extra?q=a%20b": "space:/extra:a b"} {
			if a, err := getPath("http", "space.test", 80, path, nil); err != nil || a.status != 200 || a.body != want {
				t.Errorf("space.test%s: %+v %v, want %q", path, a, err, want)
			}
		}
		if _, err := e.SiteRemove(ctx, "space.test", true); err != nil {
			t.Fatal(err)
		}
		// A character Apache or PHP-FPM reads as syntax is refused.
		dollar := filepath.Join(home, "www", "a$b")
		if err := os.MkdirAll(dollar, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := e.SiteCreate(ctx, state.Flags{Domain: "dollar.test", Docroot: ptr(dollar)}); code(err) != hteerr.InvalidDocroot {
			t.Errorf("docroot with $: %v", err)
		}
	})

	step("php ini and the global overlay", func(t *testing.T) {
		if _, err := e.PHPIniSet(ctx, appDomain, "memory_limit", "321M"); err != nil {
			t.Fatal(err)
		}
		if _, err := e.PHPGlobalSet(ctx, "8.3", "max_input_vars", "1234"); err != nil {
			t.Fatal(err)
		}
		if a := mustGet(t, "http", appDomain, 80); a.body != "app:321M:1234" {
			t.Errorf("after php ini set and php global set: %+v", a)
		}
	})

	step("a reload that fails rolls back", func(t *testing.T) {
		l, err := net.Listen("tcp4", "127.0.0.1:9099")
		if err != nil {
			t.Fatal(err)
		}
		defer l.Close()
		before := read(t, paths.SiteFile(appDomain))
		_, err = e.SiteModify(ctx, state.Flags{Domain: appDomain, ExtraHTTPPorts: &[]int{9099}})
		if code(err) != hteerr.ReloadFailed || !strings.Contains(err.Error(), "rolled back") {
			t.Fatalf("%v, want reload_failed with the rollback", err)
		}
		t.Logf("the failure: %v", err)
		if read(t, paths.SiteFile(appDomain)) != before {
			t.Error("the site state changed")
		}
		if a := mustGet(t, "http", appDomain, 80); a.status != 200 {
			t.Errorf("after the rollback: %+v", a)
		}
	})

	step("an extra port", func(t *testing.T) {
		if _, err := e.SiteModify(ctx, state.Flags{Domain: appDomain, ExtraHTTPPorts: &[]int{9099}}); err != nil {
			t.Fatal(err)
		}
		if a := mustGet(t, "http", appDomain, 9099); a.status != 200 {
			t.Errorf("port 9099: %+v", a)
		}
	})

	step("stop, start, restart, status, logs", func(t *testing.T) {
		if _, err := e.ServiceStop(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := get("http", appDomain, 80); err == nil {
			t.Error("port 80 answers after stop")
		}
		// HTE-32: stop leaves no process of the stack.
		for _, name := range []string{"apache2", "php-fpm8.3"} {
			if out, err := exec.Command("pgrep", "-u", "dev", "-a", "-x", name).Output(); err == nil {
				t.Errorf("%s processes after stop: %s", name, out)
			}
		}
		st, err := e.ServiceStatus(ctx)
		if err != nil || st.Running || st.Apache.Running {
			t.Errorf("status after stop: %+v %v", st, err)
		}
		if _, err := e.ServiceStart(ctx); err != nil {
			t.Fatal(err)
		}
		st, err = e.ServiceStatus(ctx)
		if err != nil || !st.Running {
			t.Fatalf("status after start: %+v %v", st, err)
		}
		pid := st.Apache.PID
		for _, s := range st.Sites {
			for _, p := range append(s.HTTP, s.HTTPS...) {
				if p.Status == 0 || p.Status >= 500 {
					t.Errorf("%s %s: %+v", s.Domain, p.URL, p)
				}
			}
		}
		if _, err := e.ServiceRestart(ctx); err != nil {
			t.Fatal(err)
		}
		if st, _ = e.ServiceStatus(ctx); st.Apache.PID == pid || !st.Running {
			t.Errorf("restart kept Apache pid %d: %+v", pid, st.Apache)
		}
		if _, err := e.ServiceReload(ctx); err != nil {
			t.Error(err)
		}
		logs, err := e.ServiceLogs(ctx, appDomain, 5)
		if err != nil || len(logs) == 0 {
			t.Fatalf("logs: %v %v", logs, err)
		}
		found := false
		for _, l := range logs {
			found = found || len(l.Lines) > 0
		}
		if !found {
			t.Errorf("no log lines: %+v", logs)
		}
	})

	step("hosts drift is repaired", func(t *testing.T) {
		d, err := e.HostsCheck(ctx)
		if err != nil || !d.InSync {
			t.Fatalf("before: %+v %v", d, err)
		}
		// The helper itself, through the drop-in, with an empty block.
		cmd := exec.Command("sudo", "-n", rootops.HelperPath, "apply")
		cmd.Stdin = bytes.NewReader(nil)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("helper: %v %s", err, out)
		}
		d, err = e.HostsCheck(ctx)
		if err != nil || d.InSync || !slices.Contains(d.Missing, appDomain) {
			t.Fatalf("after emptying the block: %+v %v", d, err)
		}
		res, err := e.Doctor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(res.Checks, func(c hte.Check) bool { return c.Name == "hosts" && c.Status == hte.CheckFixed }) {
			t.Errorf("doctor did not repair the block: %+v", res.Checks)
		}
		if d, err = e.HostsCheck(ctx); err != nil || !d.InSync {
			t.Errorf("after doctor: %+v %v", d, err)
		}
		// The helper refuses what is not its grammar.
		for _, bad := range []string{"10.0.0.1 evil.test\n", "127.0.0.1 *.evil.test\n", "127.0.0.1 a.test\n127.0.0.1 b.test extra/\n"} {
			cmd := exec.Command("sudo", "-n", rootops.HelperPath, "apply")
			cmd.Stdin = strings.NewReader(bad)
			if err := cmd.Run(); err == nil {
				t.Errorf("the helper took %q", bad)
			}
		}
		if out, err := exec.Command("sudo", "-n", rootops.HelperPath, "apply", "/etc/passwd").CombinedOutput(); err == nil {
			t.Errorf("sudo ran the helper with another argument: %s", out)
		}
	})

	step("site remove", func(t *testing.T) {
		if _, err := e.SiteRemove(ctx, appDomain, true); err != nil {
			t.Fatal(err)
		}
		if exists(paths.CertDir(appDomain)) || exists(paths.SiteFile(appDomain)) {
			t.Error("the site's files stay")
		}
		if !exists(filepath.Join(appRoot, "index.php")) {
			t.Error("remove touched the document root")
		}
		if strings.Contains(read(t, "/etc/hosts"), appDomain) {
			t.Error("the hosts block keeps app.test")
		}
		if _, err := get("http", appDomain, 9099); err == nil {
			t.Error("the extra port still answers")
		}
	})

	step("uninstall", func(t *testing.T) {
		res, err := e.Uninstall(ctx, hte.UninstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("stage %s, restored %d changes, warnings %v", res.Stage, len(res.Restored), res.Warnings)
		for _, p := range []string{rootops.HelperPath, rootops.SudoersPath, receipt.RootPath("dev"), paths.ConfigDir} {
			if exists(p) {
				t.Errorf("%s stays", p)
			}
		}
		if !exists(filepath.Join(res.Stage, "config", "sites", "http.test.toml")) {
			t.Error("the stage lacks the sites")
		}
		for _, p := range legacyVhosts {
			if !exists(p) {
				t.Errorf("%s is not back", p)
			}
		}
		if !systemApacheRuns() {
			t.Fatal("the system Apache does not run again")
		}
		if a := mustGet(t, "http", "http.test", 80); a.status != 200 || a.body != "legacy:http.test:fpm-fcgi" {
			t.Errorf("the system Apache serves http.test: %+v", a)
		}
	})
}

func mustDirs(t *testing.T) platform.Dirs {
	t.Helper()
	d, err := platform.ResolveDirs(os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// TestE2EReinstall runs after TestE2E in the same container: install on
// the high ports next to the system Apache that uninstall restored,
// taking the migrated sites back from the uninstall stage, then a
// certificate renewal, the shell autostart, and a second uninstall.
func TestE2EReinstall(t *testing.T) {
	if os.Getenv("GASH_HTE_E2E") != "1" || !inContainer() {
		t.Skip("runs only inside the E2E container: testdata/hte/e2e/run.sh")
	}
	ctx := context.Background()
	e, err := hte.ProcessEngine(false)
	if err != nil {
		t.Fatal(err)
	}
	paths := state.NewPaths(mustDirs(t), os.Getuid())
	stages, _ := filepath.Glob(paths.Stage("*"))
	if len(stages) != 1 {
		t.Fatalf("stages %v: TestE2E runs first", stages)
	}
	step := func(name string, fn func(t *testing.T)) {
		if !t.Run(name, fn) {
			t.FailNow()
		}
	}
	opt := hte.InstallOptions{Ports: "high", Autostart: "shell"}

	step("install offers the stage", func(t *testing.T) {
		_, err := e.Install(ctx, opt)
		if code(err) != hteerr.MissingValue || !strings.Contains(err.Error(), stages[0]) {
			t.Fatalf("%v, want missing_value naming %s", err, stages[0])
		}
	})

	step("install --ports high --restore-stage", func(t *testing.T) {
		opt.RestoreStage = stages[0]
		res, err := e.Install(ctx, opt)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("plan: %s", strings.Join(res.Plan, " | "))
		if !res.Started || res.Restored != stages[0] || exists(stages[0]) {
			t.Fatalf("install: %+v", res)
		}
		if slices.ContainsFunc(res.Plan, func(l string) bool { return strings.Contains(l, rootops.PortSysctl) }) {
			t.Error("the high ports need no sysctl")
		}
		if !systemApacheRuns() {
			t.Error("the system Apache stopped")
		}
		if a := mustGet(t, "http", "http.test", 8080); a.status != 200 || a.body != "legacy:http.test:fpm-fcgi" {
			t.Errorf("http.test on 8080: %+v", a)
		}
		if a := mustGet(t, "http", "http.test", 80); a.status != 200 {
			t.Errorf("the system Apache on 80: %+v", a)
		}
	})

	step("migrate on the high ports keeps the system Apache", func(t *testing.T) {
		// HTE-70: the managed sites' vhosts are archived and the system
		// Apache reloaded, never stopped; the excluded vhost stays its.
		// which.txt tells the document roots apart (index.php prints the
		// Host header, whichever vhost serves it).
		for _, d := range []string{"http", "https"} {
			if err := os.WriteFile("/home/dev/sites/"+d+"/public/which.txt", []byte(d), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		res, err := e.Migrate(ctx, hte.MigrateOptions{Apply: true, Yes: true, Exclude: []string{"https.test"}})
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("root plan: %s", strings.Join(res.RootPlan, " | "))
		if sa := res.Plan.SystemApache; sa == nil || !slices.Contains(sa.Ports, 80) || !slices.Contains(sa.Ports, 443) {
			t.Errorf("plan: %+v", res.Plan.SystemApache)
		}
		if !systemApacheRuns() || len(res.Archived) == 0 {
			t.Fatalf("system Apache running %v, archived %v", systemApacheRuns(), res.Archived)
		}
		if a := mustGet(t, "http", "https.test", 80); a.status != 200 || a.body != "legacy:https.test:fpm-fcgi" {
			t.Errorf("the excluded https.test on 80: %+v", a)
		}
		if a, err := getPath("http", "http.test", 80, "/which.txt", nil); err != nil || a.body == "http" {
			t.Errorf("the archived http.test is still served on 80: %+v %v", a, err)
		}
		if a, err := getPath("http", "http.test", 8080, "/which.txt", nil); err != nil || a.body != "http" {
			t.Errorf("http.test on 8080: %+v %v", a, err)
		}
		if a := mustGet(t, "http", "http.test", 8080); a.status != 200 || a.body != "legacy:http.test:fpm-fcgi" {
			t.Errorf("http.test on 8080: %+v", a)
		}
	})

	step("cert renew", func(t *testing.T) {
		before := mustGet(t, "https", "https.test", 8443)
		if before.cert == nil || before.cert.PublicKeyAlgorithm != x509.RSA {
			t.Fatalf("before: %+v", before.cert)
		}
		if _, err := e.CertRenew(ctx, "https.test"); err != nil {
			t.Fatal(err)
		}
		after := mustGet(t, "https", "https.test", 8443)
		if after.cert == nil || after.cert.PublicKeyAlgorithm != x509.ECDSA || after.status != 200 {
			t.Errorf("after: %+v", after)
		}
	})

	step("shell autostart", func(t *testing.T) {
		if _, err := e.ServiceStop(ctx); err != nil {
			t.Fatal(err)
		}
		if err := e.Autostart(ctx); err != nil {
			t.Fatal(err)
		}
		if a := mustGet(t, "http", "http.test", 8080); a.status != 200 {
			t.Errorf("after autostart: %+v", a)
		}
		if err := e.Autostart(ctx); err != nil {
			t.Errorf("autostart on a running stack: %v", err)
		}
	})

	step("uninstall", func(t *testing.T) {
		res, err := e.Uninstall(ctx, hte.UninstallOptions{Yes: true})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := get("http", "http.test", 8080); err == nil {
			t.Error("port 8080 answers after uninstall")
		}
		if !systemApacheRuns() {
			t.Error("the system Apache stopped")
		}
		// The vhosts are back and the system Apache was reloaded.
		if a, err := getPath("http", "http.test", 80, "/which.txt", nil); err != nil || a.body != "http" {
			t.Errorf("http.test on 80 after uninstall: %+v %v", a, err)
		}
		for _, d := range []string{"http", "https"} {
			os.Remove("/home/dev/sites/" + d + "/public/which.txt")
		}
		t.Logf("stage %s", res.Stage)
	})
}

// eventually waits up to 3 s for file to contain s: Apache writes the
// access log line after the response.
func eventually(file, s string) bool {
	for range 30 {
		if data, err := os.ReadFile(file); err == nil && strings.Contains(string(data), s) {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}
