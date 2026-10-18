// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hostshelper"
	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/receipt"
	"github.com/mauriziofonte/gash/internal/platform"
)

const helperBytes = "#!helper binary\n"

// fixture is a system root: the V2 hte-cli tree plus what the root step
// reads (sudoers, the port sysctl, the tools it runs, SysV links).
type fixture struct {
	t      *testing.T
	root   string
	sr     platform.Sysroot
	mu     sync.Mutex
	active map[string]bool // system units that run
	failOn string          // a command line prefix that fails
	lines  []string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("../../../testdata/hte/legacy/V2")); err != nil {
		t.Fatal(err)
	}
	f := &fixture{t: t, root: root, active: map[string]bool{"apache2": true, "php8.3-fpm": true}}
	f.write("/etc/sudoers", "Defaults env_reset\nroot ALL=(ALL:ALL) ALL\n@includedir /etc/sudoers.d\n", 0o440)
	f.write("/etc/sudoers.d/README", "readme\n", 0o440)
	f.write(sysctlProc, "1024\n", 0o644)
	for _, tool := range []string{"/usr/sbin/visudo", "/usr/sbin/sysctl", "/usr/sbin/service", "/usr/sbin/update-rc.d", "/usr/bin/openssl", "/usr/bin/loginctl"} {
		f.write(tool, "", 0o755)
	}
	f.write("/home/me/bin/gash-hosts-helper", helperBytes, 0o755)
	for _, rc := range []string{"rc2.d", "rc3.d"} {
		if err := os.MkdirAll(filepath.Join(root, "etc", rc), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, u := range []string{"apache2", "php8.3-fpm"} {
			if err := os.Symlink("../init.d/"+u, filepath.Join(root, "etc", rc, "S01"+u)); err != nil {
				t.Fatal(err)
			}
		}
	}
	f.write("/var/lib/keep", "", 0o644)
	sr, err := platform.NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	f.sr = sr
	return f
}

func (f *fixture) write(sys, content string, mode os.FileMode) {
	f.t.Helper()
	p := filepath.Join(f.root, sys)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chmod(p, mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) read(sys string) string {
	data, _ := os.ReadFile(filepath.Join(f.root, sys))
	return string(data)
}

// Run plays the tools the root step runs.
func (f *fixture) Run(_ context.Context, c proc.Cmd) (proc.Result, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	line := filepath.Base(c.Path) + " " + strings.Join(c.Args, " ")
	f.lines = append(f.lines, line)
	if f.failOn != "" && strings.HasPrefix(line, f.failOn) {
		return proc.Result{ExitCode: 1, Stderr: []byte("simulated failure")}, nil
	}
	switch filepath.Base(c.Path) {
	case "visudo":
		if !strings.HasPrefix(c.Args[2], f.root) {
			f.t.Errorf("visudo checks %s, outside the root", c.Args[2])
		}
	case "sysctl":
		k, v, _ := strings.Cut(c.Args[1], "=")
		if k != PortSysctl {
			f.t.Errorf("sysctl %s", k)
		}
		f.write(sysctlProc, v+"\n", 0o644)
	case "service":
		switch c.Args[1] {
		case "status":
			if !f.active[c.Args[0]] {
				return proc.Result{ExitCode: 3}, nil
			}
		case "stop":
			f.active[c.Args[0]] = false
		case "start":
			f.active[c.Args[0]] = true
		}
	case "update-rc.d":
		from, to := "S", "K"
		if c.Args[1] == "enable" {
			from, to = "K", "S"
		}
		for _, rc := range []string{"rc2.d", "rc3.d"} {
			dir := filepath.Join(f.root, "etc", rc)
			os.Rename(filepath.Join(dir, from+"01"+c.Args[0]), filepath.Join(dir, to+"01"+c.Args[0]))
		}
	case "openssl":
		if c.StdinFile == nil || c.Credential == nil || c.Credential.UID != 1000 || c.Stdin != nil {
			f.t.Errorf("openssl without the open key or the user's credential: %+v", c)
		}
		data, _ := io.ReadAll(c.StdinFile)
		os.WriteFile(c.Args[2], data, 0o600)
	}
	return proc.Result{}, nil
}

func (f *fixture) env() Env {
	return Env{
		Sysroot: f.sr, Runner: f, Euid: 0,
		Getenv: func(k string) string {
			return map[string]string{"SUDO_USER": "me", "SUDO_UID": "1000", "SUDO_GID": "1000"}[k]
		},
		Now:    func() time.Time { return time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC) },
		OS:     platform.OS{GOOS: "linux", ID: "ubuntu", VersionID: "24.04", Family: platform.FamilyDebian},
		Layout: mustLayout(f.t),
		Groups: func(string) ([]uint32, error) { return []uint32{1000}, nil },
		Home:   func(string) (string, error) { return "/home/me", nil },
	}
}

func mustLayout(t *testing.T) legacy.Layout {
	l, err := legacy.LayoutFor(platform.FamilyDebian, "")
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// pipeStarter runs Serve in-process behind a Session.
type pipeStarter struct{ env Env }

type pipeSession struct {
	stdin  io.WriteCloser
	stdout io.Reader
	done   chan error
}

func (p *pipeSession) Stdin() io.WriteCloser { return p.stdin }
func (p *pipeSession) Stdout() io.Reader     { return p.stdout }
func (p *pipeSession) Wait() (proc.Result, error) {
	if err := <-p.done; err != nil {
		return proc.Result{ExitCode: 1, Stderr: []byte(err.Error())}, nil
	}
	return proc.Result{}, nil
}

func (s pipeStarter) Start(ctx context.Context, _ proc.Cmd) (proc.Session, error) {
	// Kernel pipes, buffered as sudo's are (io.Pipe is not).
	inR, inW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	ps := &pipeSession{stdin: inW, stdout: outR, done: make(chan error, 1)}
	go func() {
		err := Serve(ctx, inR, outW, s.env)
		outW.Close()
		io.Copy(io.Discard, inR)
		inR.Close()
		ps.done <- err
	}()
	return ps, nil
}

func session(t *testing.T, env Env, plan []Op) *Client {
	t.Helper()
	c, err := Open(context.Background(), pipeStarter{env}, SudoCmd("/usr/bin/sudo", "/usr/local/bin/gash", false), plan)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// snapshot is every entry of the tree: kind, mode, content or target.
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(p)
			out[rel] = "link " + target
		case fi.IsDir():
			out[rel] = fmt.Sprintf("dir %04o", fi.Mode().Perm())
		default:
			data, _ := os.ReadFile(p)
			out[rel] = fmt.Sprintf("file %04o %s", fi.Mode().Perm(), sha(string(data)))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func installPlan() []Op {
	return []Op{
		{ID: 1, Op: OpInstallHelper, Src: "/home/me/bin/gash-hosts-helper", SHA256: sha(helperBytes)},
		{ID: 2, Op: OpWriteSudoers},
		{ID: 3, Op: OpSysctlPorts, Value: 80},
		{ID: 4, Op: OpHostsSnapshot},
		{ID: 5, Op: OpUnitStop, Name: "apache2"},
		{ID: 6, Op: OpArchiveVhost, Path: "/etc/apache2/sites-available/001-http.test.conf"},
		{ID: 7, Op: OpArchivePool, Path: "/etc/php/8.3/fpm/pool.d/http.test.conf"},
		{ID: 8, Op: OpUnitReload, Name: "php8.3-fpm"},
		{ID: 9, Op: OpRecordAutostart, Mode: "shell"},
	}
}

func uninstall(t *testing.T, f *fixture) {
	t.Helper()
	r, ok, err := receipt.Load(filepath.Join(f.root, receipt.RootPath("me")))
	if err != nil || !ok {
		t.Fatalf("root receipt: %v %v", ok, err)
	}
	var plan []Op
	for i := len(r.Changes) - 1; i >= 0; i-- {
		plan = append(plan, Op{ID: len(plan) + 1, Op: OpRestore, Change: r.Changes[i].ID})
	}
	plan = append(plan, Op{ID: len(plan) + 1, Op: OpFinish})
	c := session(t, f.env(), plan)
	defer c.Close()
	for _, o := range plan {
		if _, err := c.Run(o.ID); err != nil {
			t.Fatalf("%s: %v", o, err)
		}
	}
}

// TestLedger_HTE61: install, then uninstall from the root receipt, leaves
// the system tree byte for byte as it was; the user copy mirrors the root
// copy after every operation.
func TestLedger_HTE61(t *testing.T) {
	f := newFixture(t)
	before := snapshot(t, f.root)
	var userCopy string
	c := session(t, f.env(), installPlan())
	c.OnReceipt = func(text string) error { userCopy = text; return nil }
	for _, o := range installPlan() {
		if _, err := c.Run(o.ID); err != nil {
			t.Fatalf("%s: %v", o, err)
		}
		if root := f.read(receipt.RootPath("me")); root != userCopy {
			t.Fatalf("after %d the copies differ", o.ID)
		}
	}
	c.Close()

	// What install did.
	if got := f.read(HelperPath); got != helperBytes {
		t.Fatalf("helper %q", got)
	}
	if got := f.read(SudoersPath); got != string(SudoersContent("me")) {
		t.Fatalf("sudoers %q", got)
	}
	if fi, _ := os.Stat(filepath.Join(f.root, SudoersPath)); fi.Mode().Perm() != 0o440 {
		t.Fatalf("sudoers mode %v", fi.Mode())
	}
	if got := f.read(sysctlProc); got != "80\n" {
		t.Fatalf("sysctl %q", got)
	}
	if f.active["apache2"] || !strings.HasPrefix(f.read(receipt.ArchiveDir+"/20261018T100000Z/etc/apache2/sites-available/001-http.test.conf"), "") {
		t.Fatal("apache2 still active")
	}
	if _, err := os.Lstat(filepath.Join(f.root, "etc/apache2/sites-enabled/001-http.test.conf")); !os.IsNotExist(err) {
		t.Fatal("the vhost is still enabled")
	}
	if !slices.Contains(f.lines, "service php8.3-fpm reload") {
		t.Fatalf("system FPM not reloaded: %v", f.lines)
	}
	r, _, err := receipt.Load(filepath.Join(f.root, receipt.RootPath("me")))
	if err != nil {
		t.Fatal(err)
	}
	var kinds []receipt.Kind
	for _, ch := range r.Changes {
		kinds = append(kinds, ch.Kind)
		if ch.Pending {
			t.Errorf("change %d still pending", ch.ID)
		}
	}
	want := []receipt.Kind{receipt.DirCreated, receipt.DirCreated, receipt.DirCreated, receipt.FileCreated, receipt.FileCreated,
		receipt.DirCreated, receipt.Sysctl, receipt.HostsBlock, receipt.Unit, receipt.LegacyVhost, receipt.LegacyPool, receipt.Autostart}
	if diff := cmp.Diff(want, kinds); diff != "" {
		t.Fatal(diff)
	}
	// The helper then writes the block.
	if err := hostshelper.Rewrite(filepath.Join(f.root, "etc/hosts"), func(h []byte) ([]byte, error) {
		return hostshelper.Splice(h, []byte("127.0.0.1 http.test\n"))
	}); err != nil {
		t.Fatal(err)
	}

	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatalf("tree after uninstall (-before +after):\n%s", diff)
	}
	if !f.active["apache2"] {
		t.Fatal("apache2 not started again")
	}
}

// TestLedger_HTE56: the sysctl drop-in and the value come back byte for
// byte, also when a drop-in of the same name existed; a value already at
// 80 or lower changes nothing and the receipt says so.
func TestLedger_HTE56(t *testing.T) {
	f := newFixture(t)
	f.write(SysctlFile, "# mine\nnet.ipv4.ip_unprivileged_port_start = 1000\n", 0o644)
	before := snapshot(t, f.root)
	c := session(t, f.env(), []Op{{ID: 1, Op: OpSysctlPorts, Value: 80}})
	if _, err := c.Run(1); err != nil {
		t.Fatal(err)
	}
	c.Close()
	if !strings.HasSuffix(f.read(SysctlFile), PortSysctl+" = 80\n") {
		t.Fatalf("%q", f.read(SysctlFile))
	}
	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatal(diff)
	}
	if !slices.Contains(f.lines, "sysctl -w "+PortSysctl+"=1024") {
		t.Fatalf("%v", f.lines)
	}

	f = newFixture(t)
	f.write(sysctlProc, "0\n", 0o644)
	c = session(t, f.env(), []Op{{ID: 1, Op: OpSysctlPorts, Value: 80}})
	resp, err := c.Run(1)
	c.Close()
	if err != nil || len(resp.Warnings) != 1 || len(resp.Changes) != 1 || string(resp.Changes[0].Prior) != `"0"` || resp.Changes[0].File != nil {
		t.Fatalf("%+v %v", resp, err)
	}
	if _, err := os.Lstat(filepath.Join(f.root, SysctlFile)); !os.IsNotExist(err) {
		t.Fatal("drop-in written although nothing changes")
	}
}

// TestInterrupted: a step that fails halfway leaves a pending change;
// uninstall still returns the tree to its state.
func TestInterrupted(t *testing.T) {
	f := newFixture(t)
	before := snapshot(t, f.root)
	f.failOn = "sysctl -w"
	c := session(t, f.env(), installPlan())
	for _, id := range []int{1, 2} {
		if _, err := c.Run(id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := c.Run(3); hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired {
		t.Fatalf("sysctl failure: %v", err)
	}
	c.Close()
	r, _, _ := receipt.Load(filepath.Join(f.root, receipt.RootPath("me")))
	if last, _ := r.Last(); last.Kind != receipt.Sysctl || !last.Pending {
		t.Fatalf("last change %+v", last)
	}
	f.failOn = ""
	uninstall(t, f)
	if diff := cmp.Diff(before, snapshot(t, f.root)); diff != "" {
		t.Fatal(diff)
	}
}

// TestLedger_HTE17: root runs only what the plan holds, once, and checks
// every path against its own reading of the system.
func TestLedger_HTE17(t *testing.T) {
	f := newFixture(t)
	plan := []Op{
		{ID: 1, Op: OpArchiveVhost, Path: "/etc/passwd.conf"},
		{ID: 2, Op: OpCopyKey, Src: "/etc/shadow", Dst: "/home/me/.local/share/gash/hte/certs/x.test/.new-1/key.pem"},
		{ID: 3, Op: OpArchivePool, Path: "/etc/php/8.3/fpm/pool.d/none.conf"},
		{ID: 4, Op: OpInstallHelper, Src: "/home/me/bin/gash-hosts-helper", SHA256: sha("other bytes")},
		{ID: 5, Op: OpCopyKey, Src: "/etc/apache2/certs-selfsigned/https.test.key", Dst: "/tmp/k/key.pem"},
	}
	c := session(t, f.env(), plan)
	defer c.Close()
	for id, want := range map[int]hteerr.Code{1: hteerr.InvalidState, 2: hteerr.InvalidState, 3: hteerr.InvalidState, 4: hteerr.InvalidState} {
		if _, err := c.Run(id); hteerr.CodeOf(err) != want {
			t.Errorf("op %d: %v", id, err)
		}
	}
	if _, err := c.Run(1); err == nil || !strings.Contains(err.Error(), "already ran") {
		t.Errorf("rerun: %v", err)
	}
	if err := c.enc.Encode(Request{Run: 99}); err != nil {
		t.Fatal(err)
	}
	if resp, _ := c.read(); resp.Error == nil || !strings.Contains(resp.Error.Details, "not in the plan") {
		t.Errorf("unknown op: %+v", resp)
	}
	// The legacy key: opened by root, read by openssl as the user.
	dst := filepath.Join(t.TempDir(), "key.pem")
	c2 := session(t, f.env(), []Op{{ID: 1, Op: OpCopyKey, Src: "/etc/apache2/certs-selfsigned/https.test.key", Dst: dst}})
	defer c2.Close()
	if _, err := c2.Run(1); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(dst); string(got) != f.read("/etc/apache2/certs-selfsigned/https.test.key") {
		t.Fatal("key not copied")
	}
	// Bad plans are refused before anything runs.
	for _, bad := range [][]Op{
		{{ID: 1, Op: "exec", Path: "/bin/sh"}},
		{{ID: 1, Op: OpUnitStop, Name: "sshd"}},
		{{ID: 1, Op: OpArchiveVhost, Path: "/etc/apache2/../shadow.conf"}},
		{{ID: 1, Op: OpHostsSnapshot}, {ID: 1, Op: OpHostsSnapshot}},
	} {
		if _, err := Open(context.Background(), pipeStarter{f.env()}, proc.Cmd{Path: "/usr/bin/sudo"}, bad); hteerr.CodeOf(err) != hteerr.InvalidState {
			t.Errorf("%+v: %v", bad, err)
		}
	}
}

func TestIdentity(t *testing.T) {
	f := newFixture(t)
	env := f.env()
	env.Euid = 1000
	if _, err := Open(context.Background(), pipeStarter{env}, proc.Cmd{Path: "/usr/bin/sudo"}, nil); hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired {
		t.Fatalf("not root: %v", err)
	}
	env = f.env()
	env.Getenv = func(k string) string {
		return map[string]string{"SUDO_USER": "root", "SUDO_UID": "0", "SUDO_GID": "0"}[k]
	}
	if _, err := Open(context.Background(), pipeStarter{env}, proc.Cmd{Path: "/usr/bin/sudo"}, nil); hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired {
		t.Fatalf("root as the user: %v", err)
	}
}

func TestSudoersRefusals(t *testing.T) {
	f := newFixture(t)
	f.write("/etc/sudoers", "root ALL=(ALL:ALL) ALL\n", 0o440)
	c := session(t, f.env(), []Op{{ID: 1, Op: OpWriteSudoers}})
	_, err := c.Run(1)
	c.Close()
	if hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired || !strings.Contains(err.Error(), "does not include") {
		t.Fatalf("%v", err)
	}
	f = newFixture(t)
	f.failOn = "visudo"
	c = session(t, f.env(), []Op{{ID: 1, Op: OpWriteSudoers}})
	_, err = c.Run(1)
	c.Close()
	if hteerr.CodeOf(err) != hteerr.PrivilegedStepRequired || !strings.Contains(err.Error(), "visudo refused") {
		t.Fatalf("%v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(f.root, SudoersDir)); len(entries) != 1 {
		t.Fatalf("left in sudoers.d: %v", entries)
	}
	if got := string(SudoersContent("me")); !strings.Contains(got, "me ALL=(root) NOPASSWD: "+HelperPath+" apply\n") || strings.Count(got, "NOPASSWD") != 2 {
		t.Fatalf("%s", got)
	}
}

func TestRestoreOrder(t *testing.T) {
	f := newFixture(t)
	c := session(t, f.env(), []Op{{ID: 1, Op: OpHostsSnapshot}, {ID: 2, Op: OpRecordAutostart, Mode: "none"}})
	c.Run(1)
	c.Run(2)
	c.Close()
	c = session(t, f.env(), []Op{{ID: 1, Op: OpRestore, Change: 1}, {ID: 2, Op: OpFinish}})
	defer c.Close()
	if _, err := c.Run(1); err == nil || !strings.Contains(err.Error(), "not the last") {
		t.Fatalf("out of order: %v", err)
	}
	if _, err := c.Run(2); err == nil {
		t.Fatal("finish with changes left")
	}
}

// undo_unit_stop undoes only the stop this session made, while it is the
// last change: the migrate rollback when a site does not answer.
func TestUndoUnitStop(t *testing.T) {
	f := newFixture(t)
	plan := []Op{{ID: 1, Op: OpUndoUnitStop, Name: "apache2"}, {ID: 2, Op: OpUnitStop, Name: "apache2"},
		{ID: 3, Op: OpUndoUnitStop, Name: "apache2"}, {ID: 4, Op: OpUndoUnitStop, Name: "php8.3-fpm"}}
	c := session(t, f.env(), plan)
	defer c.Close()
	if _, err := c.Run(1); err == nil {
		t.Fatal("undo before any stop")
	}
	if _, err := c.Run(2); err != nil {
		t.Fatal(err)
	}
	if f.active["apache2"] {
		t.Fatal("apache2 still active")
	}
	if _, err := c.Run(4); err == nil {
		t.Fatal("undo of a unit this session did not stop")
	}
	resp, err := c.Run(3)
	if err != nil {
		t.Fatal(err)
	}
	if !f.active["apache2"] {
		t.Error("apache2 not started again")
	}
	if r, perr := receipt.Parse([]byte(resp.Receipt), "root"); perr == nil && len(r.Changes) != 0 {
		t.Errorf("receipt changes left: %+v", r.Changes)
	}
}
