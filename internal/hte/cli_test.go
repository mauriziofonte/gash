// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte"
	"github.com/mauriziofonte/gash/internal/hte/htecmd"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/term"
)

// gash runs `gash hte ARGS...` through the command line (htecmd) on the
// engine of a rig.
type gash struct {
	t     *testing.T
	e     *hte.Engine
	rig   hte.CLIRig
	tty   bool   // stdin and stderr are terminals
	stdin string // what the user types
	env   map[string]string
}

type outcome struct {
	stdout, stderr string
	status         int
}

func newGash(t *testing.T) *gash {
	t.Helper()
	e, rig := hte.NewCLIRig(t)
	return &gash{t: t, e: e, rig: rig, env: map[string]string{}}
}

func (g *gash) run(args ...string) outcome {
	g.t.Helper()
	var stdout, stderr bytes.Buffer
	env := invoke.EnvMap(g.env)
	c := &invoke.Call{
		Ctx: context.Background(), Name: "hte", Args: args, Env: env,
		Stdin: strings.NewReader(g.stdin), Stdout: &stdout, Stderr: &stderr,
		Term: term.Fixed(env, g.tty, false, g.tty),
	}
	status := htecmd.Handler(htecmd.Options{Engine: func(_ *invoke.Call, interactive bool) (*hte.Engine, error) {
		g.e.Interactive = interactive
		return g.e, nil
	}})(c)
	return outcome{stdout.String(), stderr.String(), status}
}

// ok runs a command that must succeed.
func (g *gash) ok(args ...string) outcome {
	g.t.Helper()
	o := g.run(args...)
	if o.status != 0 {
		g.t.Fatalf("gash hte %s: status %d\nstdout:\n%s\nstderr:\n%s", strings.Join(args, " "), o.status, o.stdout, o.stderr)
	}
	return o
}

// fails runs a command that must end with the envelope of code.
func (g *gash) fails(code string, args ...string) map[string]any {
	g.t.Helper()
	o := g.run(args...)
	env := envelopeOf(g.t, o.stderr)
	if o.status != 1 || env["error"] != code {
		g.t.Fatalf("gash hte %s: status %d, envelope %v, want %s\nstdout:\n%s\nstderr:\n%s",
			strings.Join(args, " "), o.status, env, code, o.stdout, o.stderr)
	}
	return env
}

// envelopeOf parses the envelope: the last line of stderr.
func envelopeOf(t *testing.T, stderr string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimRight(stderr, "\n"), "\n")
	last := lines[len(lines)-1]
	// A prompt ends with the user's newline, which the test does not echo.
	if i := strings.Index(last, "{"); i > 0 {
		last = last[i:]
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(last), &env); err != nil {
		return map[string]any{"stderr": stderr}
	}
	return env
}

// object parses the one JSON object of --json.
func object(t *testing.T, o outcome) map[string]any {
	t.Helper()
	if strings.Count(o.stdout, "\n") != 1 {
		t.Fatalf("not one line of JSON:\n%s", o.stdout)
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(o.stdout), &v); err != nil {
		t.Fatalf("%v:\n%s", err, o.stdout)
	}
	return v
}

func contains(t *testing.T, what, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, got)
		}
	}
}

// TestCLIHelpAndUsage: help on stdout, exit 0; every command-line error
// is one envelope of its kind with the hint naming the command's help.
func TestCLIHelpAndUsage(t *testing.T) {
	g := newGash(t)
	for _, args := range [][]string{nil, {"--help"}, {"site"}, {"site", "create", "--help"}, {"apache", "modules", "-h"}} {
		o := g.run(args...)
		if o.status != 0 || o.stderr != "" || !strings.Contains(o.stdout, "Usage:") {
			t.Errorf("%v: %+v", args, o)
		}
	}
	help := g.run("--help").stdout
	contains(t, "gash hte --help", help, "gash hte [command]", "install", "uninstall", "doctor", "migrate", "site", "php",
		"apache", "service", "cert", "hosts", "--json")
	if strings.Contains(help, "__") || strings.Contains(help, "completion") {
		t.Errorf("hidden commands listed:\n%s", help)
	}
	contains(t, "site create --help", g.run("site", "create", "--help").stdout, "gash hte site create [flags]",
		"--domain", "--docroot", "--no-https", "--force-https-bypass", "--extra-https-port")

	for _, c := range []struct {
		args       []string
		code, hint string
	}{
		{[]string{"install", "--bogus"}, "unknown_option", "Run: gash hte install --help"},
		{[]string{"site", "create", "--domain"}, "missing_value", "Run: gash hte site create --help"},
		{[]string{"php", "ini", "set", "--domain", "a.test", "memory_limit"}, "missing_value", "Run: gash hte php ini set --help"},
		{[]string{"doctor", "extra"}, "unexpected_argument", "Run: gash hte doctor --help"},
		{[]string{"frobnicate"}, "unexpected_argument", "Run: gash hte --help"},
		{[]string{"site", "frob"}, "unexpected_argument", "Run: gash hte site --help"},
		{[]string{"help"}, "unexpected_argument", "Run: gash hte --help"},
	} {
		env := g.fails(c.code, c.args...)
		if env["action"] != "RETRY" || env["hint"] != c.hint {
			t.Errorf("%v: %v", c.args, env)
		}
	}
}

// TestCLIBeforeInstall: what needs install fails with hte_not_installed
// (STOP); php versions, doctor and the migrate dry run do not need it;
// flag conflicts and missing flags fail before the engine runs, and a
// missing flag is asked for on a terminal only.
func TestCLIBeforeInstall(t *testing.T) {
	g := newGash(t)
	for _, args := range [][]string{{"site", "list"}, {"--json", "service", "status"}, {"apache", "config", "path"}, {"hosts", "list"}} {
		if env := g.fails("hte_not_installed", args...); env["action"] != "STOP" || env["recoverable"] != true {
			t.Errorf("%v: %v", args, env)
		}
	}
	o := g.ok("php", "versions")
	contains(t, "php versions", o.stdout, "VERSION", "8.3", "/usr/sbin/php-fpm8.3")
	v := object(t, g.ok("php", "versions", "--json"))
	if vs, ok := v["versions"].([]any); !ok || len(vs) == 0 || v["problems"] == nil {
		t.Errorf("php versions --json: %v", v)
	}
	contains(t, "migrate", g.ok("migrate").stdout, "gash hte migrate: plan for", "http.test")

	g.fails("conflicting_options", "install", "--restore-stage", "/x", "--no-restore")
	g.fails("conflicting_options", "site", "create", "--domain", "a.test", "--docroot", "/srv", "--https", "--no-https")
	g.fails("invalid_ports", "install", "--ports", "low")
	g.fails("invalid_autostart", "install", "--autostart", "cron")
	g.fails("confirmation_required", "uninstall")

	// Headless or without a terminal a missing flag is missing_value...
	if env := g.fails("missing_value", "site", "show"); env["details"] != "--domain is required" || env["hint"] != "pass --domain" {
		t.Errorf("site show: %v", env)
	}
	g.tty, g.stdin = true, "app.test\n"
	g.env["GASH_HEADLESS"] = "1"
	g.fails("missing_value", "site", "show")
	// ...on a terminal it is asked for.
	delete(g.env, "GASH_HEADLESS")
	o = g.run("site", "show")
	if !strings.HasPrefix(o.stderr, "Domain: ") || envelopeOf(t, o.stderr)["error"] != "hte_not_installed" {
		t.Errorf("prompt: %+v", o)
	}
	g.stdin = "\n"
	if o := g.run("site", "show"); envelopeOf(t, o.stderr)["error"] != "missing_value" {
		t.Errorf("empty answer: %+v", o)
	}
}

// TestCLIRoot: euid 0 may run install, uninstall, migrate and doctor
// only; every other command is root_not_allowed (FATAL).
func TestCLIRoot(t *testing.T) {
	g := newGash(t)
	g.e.EUID = 0
	for _, args := range [][]string{{"site", "list"}, {"php", "versions"}, {"service", "start"}} {
		if env := g.fails("root_not_allowed", args...); env["action"] != "FATAL" || env["recoverable"] != false {
			t.Errorf("%v: %v", args, env)
		}
	}
	g.fails("root_not_allowed", "migrate", "--apply")
}

// TestCLI drives the stack through the command line: install, migrate,
// sites, PHP values, modules, service, hosts, certificates, doctor and
// uninstall, in text and JSON.
func TestCLI(t *testing.T) {
	g := newGash(t)

	o := g.ok("install", "--ports", "high", "--autostart", "none")
	contains(t, "install stdout", o.stdout, "Ports", "8080 and 8443", "Started", "yes", "Recorded in the receipt:", "file_created /etc/sudoers.d/gash-hte")
	contains(t, "install stderr", o.stderr, "Info: sudo runs these steps as root:", "  1. install /usr/local/libexec/gash/gash-hosts-helper",
		"OK: gash hte is installed and the stack runs")
	if g.rig.Running() == 0 {
		t.Fatal("the stack does not run")
	}
	// A second install finds every step done; with --json and no
	// terminal the steps are not shown on stderr.
	inst := object(t, g.ok("install", "--ports", "high", "--autostart", "none", "--json"))
	if inst["started"] != true || inst["settings"].(map[string]any)["ports"].(map[string]any)["http"] != 8080.0 {
		t.Errorf("install --json: %v", inst)
	}
	if o := g.ok("install", "--ports", "high", "--autostart", "none", "--json"); strings.Contains(o.stderr, "sudo runs") {
		t.Errorf("install --json showed the steps: %s", o.stderr)
	}

	o = g.ok("migrate")
	contains(t, "migrate", o.stdout, "Every root step of --apply, in order (one sudo):", "apachectl configtest")
	contains(t, "migrate", o.stderr, "Hint: Apply it: gash hte migrate --apply")
	g.fails("confirmation_required", "migrate", "--apply")
	mig := object(t, g.ok("migrate", "--apply", "--yes", "--json"))
	if mig["applied"] != true || len(mig["imported"].([]any)) != 2 {
		t.Fatalf("migrate --apply: %v", mig)
	}

	contains(t, "site list", g.ok("site", "list").stdout, "DOMAIN", "http.test", "forced.test")
	show := object(t, g.ok("site", "show", "--domain", "http.test", "--json"))
	if show["site"].(map[string]any)["domain"] != "http.test" {
		t.Errorf("site show: %v", show)
	}
	g.fails("site_not_found", "site", "show", "--domain", "nope.test")

	root := filepath.Join(g.rig.Home(), "www", "app", "public")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	o = g.ok("site", "create", "--domain", "app.test", "--docroot", root, "--php", "8.3", "--alias", "api.app.test")
	contains(t, "site create", o.stdout, "Domain", "app.test", "Aliases", "www.app.test api.app.test", "HTTPS", "on")
	contains(t, "site create", o.stderr, "OK: app.test created", "Info: the stack reloaded")
	g.fails("site_exists", "site", "create", "--domain", "app.test", "--docroot", root)
	g.fails("invalid_profile", "site", "modify", "--domain", "app.test", "--profile", "fast")
	o = g.ok("site", "modify", "--domain", "app.test", "--force-https")
	contains(t, "site modify", o.stdout, "Force HTTPS", "on")
	contains(t, "site modify", g.ok("site", "modify", "--domain", "app.test", "--force-https").stderr, "app.test: nothing to change")
	mod := object(t, g.ok("site", "modify", "--domain", "app.test", "--no-force-https", "--extra-http-port", "8081", "--json"))
	if s := mod["site"].(map[string]any); s["force_https"] != false || len(s["extra_http_ports"].([]any)) != 1 {
		t.Errorf("site modify --json: %v", mod)
	}

	g.ok("php", "ini", "set", "--domain", "app.test", "memory_limit", "256M")
	if o := g.ok("php", "ini", "get", "--domain", "app.test", "memory_limit"); o.stdout != "256M\n" {
		t.Errorf("php ini get: %+v", o)
	}
	if v := object(t, g.ok("php", "ini", "get", "--domain", "app.test", "upload_max_filesize", "--json")); v["set"] != false {
		t.Errorf("php ini get unset key: %v", v)
	}
	g.ok("php", "ini", "unset", "--domain", "app.test", "memory_limit")
	g.ok("php", "global", "set", "--version", "8.3", "max_input_vars", "2000")
	if o := g.ok("php", "global", "get", "--version", "8.3", "max_input_vars"); o.stdout != "2000\n" {
		t.Errorf("php global get: %+v", o)
	}

	if o := g.ok("apache", "config", "path"); !strings.HasSuffix(o.stdout, ".conf\n") || strings.Count(o.stdout, "\n") != 1 {
		t.Errorf("apache config path: %q", o.stdout)
	}
	contains(t, "modules list", g.ok("apache", "modules", "list").stdout, "MODULE", "ssl", "base")
	g.fails("apache_module_missing", "apache", "modules", "enable", "nosuch")

	st := object(t, g.ok("service", "status", "--json"))
	if st["running"] != true {
		t.Errorf("service status: %v", st)
	}
	contains(t, "service status", g.ok("service", "status").stdout, "MASTER", "apache", "running", "SITE")
	contains(t, "service reload", g.ok("service", "reload").stderr, "OK: the stack reloaded (generation")
	if o := g.ok("service", "logs", "--lines", "3"); !strings.HasPrefix(o.stdout, "==> ") {
		t.Errorf("service logs: %q", o.stdout)
	}

	contains(t, "hosts list", g.ok("hosts", "list").stdout, "127.0.0.1 ", "app.test")
	contains(t, "hosts check", g.ok("hosts", "check").stderr, "OK: the hosts block matches the sites")
	_, block, _ := strings.Cut(g.rig.Read("/etc/hosts"), "# >>> gash hte >>>")
	g.rig.Write("/etc/hosts", "127.0.0.1\tlocalhost\n", 0o644)
	if o := g.run("hosts", "check"); o.status != 1 || !strings.Contains(o.stdout, "Missing from the block:") {
		t.Errorf("drifted hosts check: %+v", o)
	}
	if d := object(t, g.run("hosts", "check", "--json")); d["in_sync"] != false {
		t.Errorf("hosts check --json: %v", d)
	}
	contains(t, "hosts apply", g.ok("hosts", "apply").stderr, "OK: the hosts block is written")
	if got := g.rig.Read("/etc/hosts"); got != "127.0.0.1\tlocalhost\n# >>> gash hte >>>"+block {
		t.Errorf("hosts after apply:\n%s\nwant the block:%s", got, block)
	}
	g.fails("platform_unsupported", "hosts", "windows")

	contains(t, "cert show", g.ok("cert", "show", "--domain", "app.test").stdout, "Names", "app.test www.app.test api.app.test",
		"Algorithm", "ECDSA P-256", "Valid until")
	g.fails("site_not_found", "cert", "show", "--domain", "nope.test")
	contains(t, "cert renew", g.ok("cert", "renew", "--domain", "app.test").stderr, "OK: app.test has a new certificate")

	doc := g.run("doctor", "--json")
	if res := object(t, doc); (res["ok"] == true) != (doc.status == 0) {
		t.Errorf("doctor: status %d, %v", doc.status, res)
	}
	contains(t, "doctor", g.run("doctor").stdout, "OK: ")

	g.fails("confirmation_required", "site", "remove", "--domain", "app.test")
	contains(t, "site remove", g.ok("site", "remove", "--domain", "app.test", "--yes").stderr, "OK: app.test removed")
	contains(t, "site disable", g.ok("site", "disable", "--domain", "http.test").stderr, "OK: http.test disabled")
	contains(t, "site enable", g.ok("site", "enable", "--domain", "http.test").stderr, "OK: http.test enabled")
	contains(t, "service stop", g.ok("service", "stop").stderr, "OK: the stack is stopped")
	contains(t, "service start", g.ok("service", "start").stderr, "OK: the stack runs")

	un := object(t, g.ok("uninstall", "--yes", "--json"))
	if stage, _ := un["stage"].(string); !strings.Contains(stage, "hte-stage-") {
		t.Errorf("uninstall: %v", un)
	}
	g.fails("hte_not_installed", "site", "list")
	// The next install finds the stage and asks what to do with it.
	if env := g.fails("missing_value", "install", "--ports", "high", "--autostart", "none"); !strings.Contains(env["hint"].(string), "--no-restore") {
		t.Errorf("install over a stage: %v", env)
	}
}
