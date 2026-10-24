// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// stockDefaultVhost is Debian's 000-default.conf without its comments.
const stockDefaultVhost = "<VirtualHost *:80>\n\tServerAdmin webmaster@localhost\n\tDocumentRoot /var/www/html\n" +
	"\tErrorLog ${APACHE_LOG_DIR}/error.log\n\tCustomLog ${APACHE_LOG_DIR}/access.log combined\n</VirtualHost>\n"

// TestMigrateSwitchOnly: a system Apache that serves only its stock
// default vhost blocks install on the standard ports (HTE-75), and
// migrate --apply --exclude <it> --yes, with nothing to import, still
// makes the switch: the system Apache stops and is disabled, the stack
// starts on 80 and 443, the excluded vhost stays as it is. It used to end
// with "nothing to migrate", a dead end. Uninstall starts the system
// Apache again and leaves the tree as it was.
func TestMigrateSwitchOnly(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	// No hte-cli tree: Debian as apt installs apache2.
	for _, p := range []string{"sites-enabled/001-http.test.conf", "sites-enabled/002-https.test.conf", "sites-enabled/003-forced.test.conf",
		"sites-available/001-http.test.conf", "sites-available/002-https.test.conf", "sites-available/003-forced.test.conf"} {
		must(t, os.Remove(filepath.Join(m.root, "etc/apache2", p)))
	}
	must(t, os.RemoveAll(filepath.Join(m.root, "etc/apache2/certs-selfsigned")))
	for _, v := range []string{"7.4", "8.3"} {
		must(t, os.RemoveAll(filepath.Join(m.root, "etc/php", v, "fpm/pool.d")))
	}
	stock := "/etc/apache2/sites-available/000-default.conf"
	m.write(stock, stockDefaultVhost, 0o644)
	must(t, os.Symlink("../sites-available/000-default.conf", filepath.Join(m.root, "etc/apache2/sites-enabled/000-default.conf")))
	before := m.snapshot()

	ins, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "standard", Autostart: "none"})
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if ins.Started || ins.Next != "gash hte migrate" || !slices.Equal(ins.Blocking, []string{stock}) {
		t.Fatalf("install: started %v, next %q, blocking %v", ins.Started, ins.Next, ins.Blocking)
	}

	dry, err := m.e.Migrate(ctx, MigrateOptions{Exclude: []string{stock}})
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Plan.Blocking) != 0 || !slices.Equal(dry.Plan.NoLongerServed, []string{stock}) ||
		!slices.ContainsFunc(dry.RootPlan, func(l string) bool { return strings.Contains(l, "apache2") && strings.Contains(l, "stop") }) {
		t.Errorf("dry run: blocking %+v, no longer served %v, root plan %q", dry.Plan.Blocking, dry.Plan.NoLongerServed, dry.RootPlan)
	}
	if _, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Exclude: []string{stock}}); code(err) != hteerr.ConfirmationRequired {
		t.Fatalf("apply without --yes: %v", err)
	}
	if !m.active["apache2"] || m.stack.Running() != 0 {
		t.Fatalf("the refused apply: apache2 active %v, masters %d", m.active["apache2"], m.stack.Running())
	}

	res, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true, Exclude: []string{stock}})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !res.Switched || len(res.Imported) != 0 || len(res.Archived) != 0 || slices.Contains(res.Warnings, "nothing to migrate") {
		t.Errorf("apply: %+v", res)
	}
	if m.active["apache2"] || m.enabled["apache2"] {
		t.Errorf("apache2 active %v, enabled %v", m.active["apache2"], m.enabled["apache2"])
	}
	if !slices.ContainsFunc(res.Changes, func(c receipt.Change) bool { return c.Kind == receipt.Unit && c.Name == "apache2" }) {
		t.Errorf("receipt changes %+v", res.Changes)
	}
	st, err := m.e.ServiceStatus(ctx)
	if err != nil || !st.Running {
		t.Fatalf("status: %+v %v", st, err)
	}
	m.mu.Lock()
	direct := slices.Clone(m.direct)
	m.mu.Unlock()
	if !slices.ContainsFunc(direct, func(l string) bool {
		return strings.HasPrefix(l, "start apache2 ") && strings.Contains(l, "127.0.0.1:80 ") && strings.Contains(l, "127.0.0.1:443")
	}) {
		t.Errorf("the stack's Apache starts: %q, want one on 127.0.0.1:80 and :443", direct)
	}
	if after := m.snapshot(); after[stock] != before[stock] || after["/etc/apache2/sites-enabled/000-default.conf"] != before["/etc/apache2/sites-enabled/000-default.conf"] {
		t.Error("the excluded vhost changed")
	}
	// A second apply is idempotent: the unit is already stopped and
	// disabled, nothing changes, the stack keeps serving.
	again, err := m.e.Migrate(ctx, MigrateOptions{Apply: true, Yes: true, Exclude: []string{stock}})
	if err != nil || len(again.Changes) != 0 || m.stack.Running() == 0 {
		t.Errorf("second apply: %+v %v", again, err)
	}

	if _, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true}); err != nil {
		t.Fatal(err)
	}
	if !m.active["apache2"] || !m.enabled["apache2"] || m.stack.Running() != 0 {
		t.Errorf("after uninstall: apache2 active %v, enabled %v, masters %d", m.active["apache2"], m.enabled["apache2"], m.stack.Running())
	}
	after := m.snapshot()
	for p, v := range before {
		if after[p] != v {
			t.Errorf("%s: %q, before %q", p, after[p], v)
		}
	}
	for p := range after {
		if _, ok := before[p]; !ok && !strings.HasPrefix(p, "/etc/hosts") {
			t.Errorf("%s left behind", p)
		}
	}
}
