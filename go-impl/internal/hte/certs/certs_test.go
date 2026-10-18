// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package certs

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/state"
)

func paths(t *testing.T) state.Paths {
	t.Helper()
	d := t.TempDir()
	return state.Paths{ConfigDir: filepath.Join(d, "config"), StateDir: filepath.Join(d, "state"),
		DataDir: filepath.Join(d, "data"), RuntimeDir: filepath.Join(d, "run")}
}

// manager returns a Manager on the real openssl; the tests that need it
// skip without one. openssl works only inside the temporary directory.
func manager(t *testing.T) Manager {
	t.Helper()
	bin := proc.Find("openssl", func(p string) bool { _, err := os.Stat(p); return err == nil })
	if bin == "" {
		t.Skip("no openssl")
	}
	return Manager{Runner: proc.OS{}, OpenSSL: bin, Paths: paths(t)}
}

func site(domain string, aliases ...string) state.Site {
	return state.Site{Domain: domain, Aliases: aliases, HTTPS: true, Enabled: true, Kind: state.KindManaged}
}

func certOf(t *testing.T, m Manager, domain string) *x509.Certificate {
	t.Helper()
	c, err := readCert(m.Paths.CertFile(domain))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// TestLedger_HTE39: ECDSA P-256, CN only, SAN, 10 years, CA:FALSE,
// digitalSignature, serverAuth.
func TestLedger_HTE39(t *testing.T) {
	m := manager(t)
	now := time.Now()
	m.Now = func() time.Time { return now }
	if err := m.Issue(context.Background(), site("shop.test", "www.shop.test")); err != nil {
		t.Fatal(err)
	}
	c := certOf(t, m, "shop.test")
	k, ok := c.PublicKey.(*ecdsa.PublicKey)
	if !ok || k.Curve != elliptic.P256() {
		t.Fatalf("key %T", c.PublicKey)
	}
	if c.Subject.String() != "CN=shop.test" || c.Issuer.String() != "CN=shop.test" {
		t.Fatalf("subject %s issuer %s", c.Subject, c.Issuer)
	}
	if diff := cmp.Diff([]string{"shop.test", "*.shop.test", "www.shop.test"}, c.DNSNames); diff != "" {
		t.Fatal(diff)
	}
	if !c.BasicConstraintsValid || c.IsCA {
		t.Fatal("basicConstraints is not CA:FALSE")
	}
	if c.KeyUsage != x509.KeyUsageDigitalSignature || !slices.Equal(c.ExtKeyUsage, []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}) {
		t.Fatalf("usage %v %v", c.KeyUsage, c.ExtKeyUsage)
	}
	if want := now.AddDate(ValidityYears, 0, 0); c.NotAfter.Sub(want).Abs() > 25*time.Hour || c.NotBefore.After(now.Add(time.Minute)) {
		t.Fatalf("validity %s to %s", c.NotBefore, c.NotAfter)
	}
	if c.SignatureAlgorithm != x509.ECDSAWithSHA256 {
		t.Fatalf("signature %s", c.SignatureAlgorithm)
	}
	// The serial is random: a renewal never repeats issuer and serial.
	first := c.SerialNumber
	if err := m.Issue(context.Background(), site("shop.test", "www.shop.test")); err != nil {
		t.Fatal(err)
	}
	if certOf(t, m, "shop.test").SerialNumber.Cmp(first) == 0 {
		t.Fatal("renewal kept the serial")
	}
}

// TestLedger_HTE14: the key is a plain PRIVATE KEY PEM, 0600, in a 0700
// directory. (The test reads the PEM header line only.)
func TestLedger_HTE14(t *testing.T) {
	m := manager(t)
	if err := m.Issue(context.Background(), site("a.test")); err != nil {
		t.Fatal(err)
	}
	key := m.Paths.KeyFile("a.test")
	f, err := os.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	line, _ := bufio.NewReader(f).ReadString('\n')
	f.Close()
	if line != "-----BEGIN PRIVATE KEY-----\n" {
		t.Fatalf("key header %q", line)
	}
	modes(t, m, "a.test")
}

func modes(t *testing.T, m Manager, domain string) {
	t.Helper()
	for path, want := range map[string]os.FileMode{
		m.Paths.CertDir(domain): 0o700, m.Paths.KeyFile(domain): 0o600, m.Paths.CertFile(domain): 0o644,
	} {
		fi, err := os.Lstat(path)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s: %04o, want %04o", path, fi.Mode().Perm(), want)
		}
		if st, ok := statOwner(fi); ok && st != os.Getuid() {
			t.Errorf("%s: owner %d", path, st)
		}
	}
}

func statOwner(fi os.FileInfo) (int, bool) {
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, false
	}
	return int(st.Uid), true
}

// TestLedger_HTE40: one directory per site, the file names, modes, and no
// temporary file after an interrupted run.
func TestLedger_HTE40(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	if err := m.Issue(ctx, site("shop.test")); err != nil {
		t.Fatal(err)
	}
	modes(t, m, "shop.test")
	entries, _ := os.ReadDir(m.Paths.CertDir("shop.test"))
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if !slices.Equal(names, []string{"cert.pem", "key.pem"}) {
		t.Fatalf("files %v", names)
	}
	// An openssl failure after the key leaves the old pair and no
	// temporary directory.
	before, _ := os.ReadFile(m.Paths.CertFile("shop.test"))
	failing := m
	failing.Runner = &proc.Fake{Handler: func(c proc.Cmd) (proc.Result, error) {
		if c.Args[0] == "req" {
			return proc.Result{ExitCode: 1, Stderr: []byte("req: boom")}, nil
		}
		return proc.OS{}.Run(context.Background(), c)
	}}
	if err := failing.Issue(ctx, site("shop.test")); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("failure: %v", err)
	}
	after, _ := os.ReadFile(m.Paths.CertFile("shop.test"))
	if string(before) != string(after) {
		t.Fatal("a failed renewal replaced the certificate")
	}
	// A crash leaves a .new- directory: the next run sweeps it.
	stale := filepath.Join(m.Paths.CertDir("shop.test"), ".new-123")
	if err := os.Mkdir(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := m.Issue(ctx, site("shop.test")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(stale); !os.IsNotExist(err) {
		t.Fatal("stale temporary directory kept")
	}
}

// TestLedger_HTE06 and TestLedger_HTE25: remove deletes exactly the site's
// directory; shop.test.it is untouched when shop.test goes.
func TestLedger_HTE06(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	for _, d := range []string{"shop.test", "shop.test.it"} {
		if err := m.Issue(ctx, site(d)); err != nil {
			t.Fatal(err)
		}
	}
	if err := m.Remove("shop.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(m.Paths.CertDir("shop.test")); !os.IsNotExist(err) {
		t.Fatal("directory kept")
	}
	if _, err := m.Show("shop.test.it"); err != nil {
		t.Fatalf("other site touched: %v", err)
	}
	if err := m.Remove("shop.test"); err != nil {
		t.Fatalf("second remove: %v", err)
	}
	if err := m.Remove("../x"); hteerr.CodeOf(err) != hteerr.InvalidDomain {
		t.Fatalf("bad domain: %v", err)
	}
}

func TestLedger_HTE25(t *testing.T) {
	m := manager(t)
	if err := m.Issue(context.Background(), site("a.test")); err != nil {
		t.Fatal(err)
	}
	// A symlink in place of the directory is removed as a link only.
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "keep"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, m.Paths.CertDir("b.test")); err != nil {
		t.Fatal(err)
	}
	if err := m.Remove("b.test"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "keep")); err != nil {
		t.Fatal("followed the symlink")
	}
	if err := m.Remove("a.test"); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(m.Paths.CertsDir()); len(entries) != 0 {
		t.Fatalf("left %v", entries)
	}
}

// TestLedger_HTE47 (certificate part): the SAN set is the names served.
func TestLedger_HTE47(t *testing.T) {
	for _, tc := range []struct {
		site state.Site
		want []string
	}{
		{site("shop.test", "www.shop.test"), []string{"shop.test", "*.shop.test", "www.shop.test"}},
		{site("www.x.test"), []string{"www.x.test", "*.www.x.test"}},
		{site("a.test", "other.test", "www.a.test"), []string{"a.test", "*.a.test", "other.test", "www.a.test"}},
	} {
		if diff := cmp.Diff(tc.want, Names(tc.site)); diff != "" {
			t.Errorf("%s: %s", tc.site.Domain, diff)
		}
	}
	m := manager(t)
	s := site("a.test", "other.test")
	if err := m.Issue(context.Background(), s); err != nil {
		t.Fatal(err)
	}
	info, err := m.Show("a.test")
	if err != nil {
		t.Fatal(err)
	}
	if miss := info.Uncovered(append(s.Names(), "deep.www.a.test")); !slices.Equal(miss, []string{"deep.www.a.test"}) {
		t.Fatalf("uncovered %v", miss)
	}
	// An alias added later is not covered until the pair is reissued.
	s.Aliases = append(s.Aliases, "new.test")
	if h := m.Check(context.Background(), s); len(h.Problems) != 1 || !strings.Contains(h.Problems[0], "new.test") {
		t.Fatalf("check: %+v", h)
	}
}

func TestCheck(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	s := site("a.test")
	if h := m.Check(ctx, s); len(h.Problems) != 1 || h.Info != nil {
		t.Fatalf("no pair: %+v", h)
	}
	if err := m.Issue(ctx, s); err != nil {
		t.Fatal(err)
	}
	if h := m.Check(ctx, s); len(h.Problems) != 0 || len(h.Warnings) != 0 || h.Info == nil || h.Info.Algorithm != "ECDSA P-256" {
		t.Fatalf("good pair: %+v", h)
	}
	// A key of another pair: the public keys differ.
	if err := m.Issue(ctx, site("b.test")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(m.Paths.KeyFile("b.test"), m.Paths.KeyFile("a.test")); err != nil {
		t.Fatal(err)
	}
	if h := m.Check(ctx, s); len(h.Problems) != 1 || !strings.Contains(h.Problems[0], "does not belong") {
		t.Fatalf("mismatch: %+v", h)
	}
	if err := os.Chmod(m.Paths.KeyFile("a.test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h := m.Check(ctx, s); !slices.ContainsFunc(h.Problems, func(p string) bool { return strings.Contains(p, "not 0600") }) {
		t.Fatalf("mode: %+v", h)
	}
	// Expiry: 30 days before the end it warns, after the end it fails.
	m2 := m
	m2.Now = func() time.Time { return time.Now().AddDate(ValidityYears, 0, -10) }
	if err := m.Issue(ctx, s); err != nil {
		t.Fatal(err)
	}
	if h := m2.Check(ctx, s); len(h.Warnings) != 1 || len(h.Problems) != 0 {
		t.Fatalf("soon: %+v", h)
	}
	m2.Now = func() time.Time { return time.Now().AddDate(ValidityYears, 1, 0) }
	if h := m2.Check(ctx, s); len(h.Problems) != 1 {
		t.Fatalf("expired: %+v", h)
	}
}

// TestOpenSSLArgv: no key material and no secret in any argv, the child
// environment is the base one, and openssl never prompts (new session).
func TestOpenSSLArgv(t *testing.T) {
	f := &proc.Fake{Handler: func(c proc.Cmd) (proc.Result, error) {
		if c.Args[0] == "req" {
			return proc.Result{ExitCode: 1, Stderr: []byte("stop here")}, nil
		}
		return proc.Result{}, nil
	}}
	m := Manager{Runner: f, OpenSSL: "/usr/bin/openssl", Paths: paths(t)}
	_ = m.Issue(context.Background(), site("a.test"))
	calls := f.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls %v", f.Lines())
	}
	for _, c := range calls {
		if !c.Setsid || c.Env != nil || c.Stdin != nil {
			t.Errorf("%s: setsid %v env %v", c, c.Setsid, c.Env)
		}
		for _, a := range c.Args {
			if strings.HasPrefix(a, "pass:") || strings.Contains(a, "PRIVATE") {
				t.Errorf("%s: secret in argv", c)
			}
		}
	}
	if !slices.Equal(calls[0].Args[:5], []string{"genpkey", "-algorithm", "EC", "-pkeyopt", "ec_paramgen_curve:P-256"}) {
		t.Fatalf("genpkey: %s", calls[0])
	}
	if m := (Manager{Runner: f, Paths: paths(t)}); hteerr.CodeOf(m.Issue(context.Background(), site("a.test"))) != hteerr.PlatformUnsupported {
		t.Fatal("no openssl accepted")
	}
}

func TestOpenSSLConfig(t *testing.T) {
	got := string(opensslConfig(Names(site("a.test", "www.a.test"))))
	for _, want := range []string{"CN = a.test\n", "DNS.1 = a.test\n", "DNS.2 = *.a.test\n", "DNS.3 = www.a.test\n",
		"basicConstraints = critical,CA:FALSE\n", "extendedKeyUsage = serverAuth\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in\n%s", want, got)
		}
	}
	if err := checkNames([]string{"a.test", "$x.test"}); hteerr.CodeOf(err) != hteerr.InvalidDomain {
		t.Fatalf("bad name: %v", err)
	}
}

// TestCopiedPair: Prepare and Finish, as migrate uses them around the
// root step that writes the legacy key.
func TestCopiedPair(t *testing.T) {
	m := manager(t)
	ctx := context.Background()
	src := manager(t)
	if err := src.Issue(ctx, site("legacy.test")); err != nil {
		t.Fatal(err)
	}
	certPEM, _ := os.ReadFile(src.Paths.CertFile("legacy.test"))
	tmp, err := m.Prepare("legacy.test")
	if err != nil {
		t.Fatal(err)
	}
	// The root step: openssl pkey as the user, the key on stdin.
	keyIn, err := os.Open(src.Paths.KeyFile("legacy.test"))
	if err != nil {
		t.Fatal(err)
	}
	defer keyIn.Close()
	if res, err := (proc.OS{}).Run(ctx, proc.Cmd{Path: m.OpenSSL, Args: []string{"pkey", "-out", tmp}, StdinFile: keyIn, Setsid: true}); err != nil || !res.OK() {
		t.Fatalf("%v %s", err, res.Output())
	}
	if err := m.Finish(ctx, "legacy.test", certPEM, tmp); err != nil {
		t.Fatal(err)
	}
	modes(t, m, "legacy.test")
	if h := m.Check(ctx, site("legacy.test")); len(h.Problems) != 0 {
		t.Fatalf("%+v", h)
	}
	// A key that does not match is refused and nothing is installed.
	tmp, _ = m.Prepare("other.test")
	if err := os.WriteFile(tmp, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := m.Finish(ctx, "other.test", certPEM, tmp); err == nil {
		t.Fatal("empty key accepted")
	}
	if _, err := os.Lstat(m.Paths.KeyFile("other.test")); !os.IsNotExist(err) {
		t.Fatal("refused key installed")
	}
	if err := m.Finish(ctx, "other.test", []byte("junk"), filepath.Join(m.Paths.CertDir("other.test"), ".new-x", "key.pem")); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("junk certificate: %v", err)
	}
	if err := m.Finish(ctx, "other.test", certPEM, "/tmp/key.pem"); err == nil {
		t.Fatal("foreign temporary path accepted")
	}
}
