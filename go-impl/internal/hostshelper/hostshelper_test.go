// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hostshelper

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestValidHostname(t *testing.T) {
	long63 := strings.Repeat("a", 63)
	name253 := strings.Repeat(long63+".", 3) + strings.Repeat("b", 61) // 64*3+61 = 253
	good := []string{"shop.test", "www.shop.test", "a.b", "a-b.c-d.test", "x1.test", "1x.test",
		"123.test", "xn--p1ai.xn--p1ai", long63 + ".test", name253, "test.1x", "a.0xg"}
	bad := []string{"", "localhost", "Shop.test", "SHOP.TEST", "*.shop.test", "shop.test.", ".shop.test",
		"shop..test", "-a.test", "a-.test", "a_b.test", "shop.test/", "http://shop.test", "shop test",
		"shop.test\n", "shop.test\x00", "127.0.0.1", "10.0.1", "127.1", "0x7f.0x1", "0x7f.1", "1.2.3.4",
		"a.0x", "[::1]", "::1", strings.Repeat("a", 64) + ".test", name253 + "b", "shöp.test", "a.b-"}
	for _, n := range good {
		if !ValidHostname(n) {
			t.Errorf("ValidHostname(%q) = false", n)
		}
	}
	for _, n := range bad {
		if ValidHostname(n) {
			t.Errorf("ValidHostname(%q) = true", n)
		}
	}
	if len(name253) != 253 {
		t.Fatalf("name253 has %d bytes", len(name253))
	}
}

func TestParseBlock(t *testing.T) {
	got, err := ParseBlock([]byte("127.0.0.1 shop.test www.shop.test\n::1 shop.test www.shop.test\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{{"127.0.0.1", []string{"shop.test", "www.shop.test"}}, {"::1", []string{"shop.test", "www.shop.test"}}}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
	if e, err := ParseBlock(nil); err != nil || e != nil {
		t.Fatalf("empty input: %v %v", e, err)
	}
	if !bytes.Equal(FormatBlock(got), []byte("127.0.0.1 shop.test www.shop.test\n::1 shop.test www.shop.test\n")) {
		t.Fatal("FormatBlock does not round-trip")
	}
}

// TestLedger_HTE24 is the refusal half of LEDGER HTE-24: every input
// outside the grammar is refused with ExitRefused and nothing is written.
func TestLedger_HTE24(t *testing.T) {
	var many strings.Builder
	for range MaxLines + 1 {
		many.WriteString("127.0.0.1 a.test\n")
	}
	refused := map[string]string{
		"wrong address":         "10.0.0.5 two.test\n",
		"any address":           "0.0.0.0 a.test\n",
		"padded address":        "127.0.0.01 a.test\n",
		"mapped address":        "::ffff:127.0.0.1 a.test\n",
		"bracketed address":     "[::1] a.test\n",
		"wildcard":              "127.0.0.1 *.shop.test\n",
		"uppercase":             "127.0.0.1 Shop.test\n",
		"trailing dot":          "127.0.0.1 shop.test.\n",
		"single label":          "127.0.0.1 localhost\n",
		"ip as name":            "127.0.0.1 127.0.0.1\n",
		"injected line":         "127.0.0.1 a.test\n10.0.0.1 evil.test\n",
		"carriage return":       "127.0.0.1 a.test\r\n",
		"tab":                   "127.0.0.1\ta.test\n",
		"nul":                   "127.0.0.1 a.test\x00\n",
		"comment":               "# comment\n",
		"inline comment":        "127.0.0.1 a.test # hte-cli\n",
		"double space":          "127.0.0.1  a.test\n",
		"trailing space":        "127.0.0.1 a.test \n",
		"leading space":         " 127.0.0.1 a.test\n",
		"blank line":            "127.0.0.1 a.test\n\n",
		"address only":          "127.0.0.1\n",
		"no final newline":      "127.0.0.1 a.test",
		"too many lines":        many.String(),
		"oversize":              strings.Repeat("a", MaxInputBytes+1),
		"non-ascii":             "127.0.0.1 shöp.test\n",
		"name too long":         "127.0.0.1 " + strings.Repeat("a", 64) + ".test\n",
		"underscore":            "127.0.0.1 a_b.test\n",
		"shell metacharacters":  "127.0.0.1 a.test;rm\n",
		"path":                  "127.0.0.1 /etc/passwd\n",
		"numeric top label":     "127.0.0.1 10.0.1\n",
		"hex address as a name": "127.0.0.1 0x7f.1\n",
	}
	for name, in := range refused {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseBlock([]byte(in)); err == nil {
				t.Fatal("accepted")
			}
			path, orig := hostsFixture(t, "127.0.0.1 localhost\n")
			var stderr bytes.Buffer
			code := Run([]string{"apply"}, strings.NewReader(in), &bytes.Buffer{}, &stderr, Config{HostsPath: path, GOOS: "darwin", Exec: failExec(t)})
			if code != ExitRefused {
				t.Fatalf("exit %d, stderr %q", code, stderr.String())
			}
			assertUnchanged(t, path, orig)
		})
	}
}

func TestSplice(t *testing.T) {
	block := []byte("127.0.0.1 shop.test www.shop.test\n")
	region := BeginMarker + "\n127.0.0.1 shop.test www.shop.test\n" + EndMarker + "\n"
	tests := []struct{ name, in, want string }{
		{"append", "127.0.0.1 localhost\n", "127.0.0.1 localhost\n" + region},
		{"append without final newline", "127.0.0.1 localhost", "127.0.0.1 localhost\n" + region},
		{"append to empty", "", region},
		{"replace", "a\n" + BeginMarker + "\n127.0.0.1 old.test\n" + EndMarker + "\nz\n", "a\n" + region + "z\n"},
		{"replace at end without newline", "a\n" + BeginMarker + "\n" + EndMarker, "a\n" + region},
		{"legacy lines kept", "127.0.0.1    shop.test www.shop.test # hte-cli\n", "127.0.0.1    shop.test www.shop.test # hte-cli\n" + region},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Splice([]byte(tt.in), block)
			if err != nil {
				t.Fatal(err)
			}
			if diff := cmp.Diff(tt.want, string(got)); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
			inner, found, err := Extract(got)
			if err != nil || !found || !bytes.Equal(inner, block) {
				t.Fatalf("Extract = %q %v %v", inner, found, err)
			}
		})
	}
	for name, in := range map[string]string{
		"begin only":       BeginMarker + "\n",
		"end only":         EndMarker + "\n",
		"end before begin": EndMarker + "\n" + BeginMarker + "\n",
		"two blocks":       BeginMarker + "\n" + EndMarker + "\n" + BeginMarker + "\n" + EndMarker + "\n",
	} {
		if _, err := Splice([]byte(in), block); !errors.Is(err, ErrCorruptBlock) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestRunApplyAndCheck(t *testing.T) {
	outside := "127.0.0.1 localhost\n::1 localhost ip6-localhost\n\n127.0.0.1    http.test www.http.test # hte-cli\n"
	path, _ := hostsFixture(t, outside)
	must(t, os.Chmod(path, 0o640))
	before := stat(t, path)

	var calls [][]string
	exec := func(argv []string) error { calls = append(calls, argv); return nil }
	in := "127.0.0.1 shop.test www.shop.test\n::1 shop.test www.shop.test\n"
	var stderr bytes.Buffer
	if code := Run([]string{"apply"}, strings.NewReader(in), &bytes.Buffer{}, &stderr, Config{HostsPath: path, GOOS: "darwin", Exec: exec}); code != ExitOK {
		t.Fatalf("apply: exit %d: %s", code, stderr.String())
	}
	got, _ := os.ReadFile(path)
	if want := outside + BeginMarker + "\n" + in + EndMarker + "\n"; string(got) != want {
		t.Fatalf("hosts:\n%s", got)
	}
	after := stat(t, path)
	if after.Mode().Perm() != 0o640 || owner(after) != owner(before) {
		t.Fatalf("mode %v owner %v, want 0640 %v", after.Mode().Perm(), owner(after), owner(before))
	}
	if diff := cmp.Diff(FlushCommands("darwin"), calls); diff != "" {
		t.Fatalf("flush commands (-want +got):\n%s", diff)
	}
	assertNoTemp(t, path)

	var stdout bytes.Buffer
	if code := Run([]string{"check"}, nil, &stdout, &stderr, Config{HostsPath: path}); code != ExitOK || stdout.String() != in {
		t.Fatalf("check: exit %d out %q", code, stdout.String())
	}

	// The same block again changes nothing and flushes nothing.
	calls = nil
	ino := inode(after)
	if code := Run([]string{"apply"}, strings.NewReader(in), &bytes.Buffer{}, &stderr, Config{HostsPath: path, GOOS: "darwin", Exec: exec}); code != ExitOK {
		t.Fatal("second apply failed")
	}
	if len(calls) != 0 || inode(stat(t, path)) != ino {
		t.Fatal("an unchanged block was rewritten")
	}

	// An empty block keeps the markers and removes the names.
	if code := Run([]string{"apply"}, strings.NewReader(""), &bytes.Buffer{}, &stderr, Config{HostsPath: path, GOOS: "linux", Exec: failExec(t)}); code != ExitOK {
		t.Fatal("empty apply failed")
	}
	got, _ = os.ReadFile(path)
	if string(got) != outside+BeginMarker+"\n"+EndMarker+"\n" {
		t.Fatalf("after empty apply:\n%s", got)
	}
}

// TestLedger_HTE46: the exact flush commands per platform, none on Linux.
func TestLedger_HTE46(t *testing.T) {
	if FlushCommands("linux") != nil {
		t.Fatal("Linux runs a flush command")
	}
	want := [][]string{{"/usr/bin/dscacheutil", "-flushcache"}, {"/usr/bin/killall", "-HUP", "mDNSResponder"}}
	if diff := cmp.Diff(want, FlushCommands("darwin")); diff != "" {
		t.Fatal(diff)
	}
	path, _ := hostsFixture(t, "")
	var stderr bytes.Buffer
	exec := func([]string) error { return errors.New("boom") }
	if code := Run([]string{"apply"}, strings.NewReader("127.0.0.1 a.test\n"), &bytes.Buffer{}, &stderr, Config{HostsPath: path, GOOS: "darwin", Exec: exec}); code != ExitOK {
		t.Fatal("a failed flush must not fail the write")
	}
	if !strings.Contains(stderr.String(), "warning") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

func TestRunArgvProtocol(t *testing.T) {
	path, orig := hostsFixture(t, "127.0.0.1 localhost\n")
	for _, args := range [][]string{nil, {}, {"apply", "check"}, {"apply", path}, {"--path", path, "apply"},
		{"Apply"}, {"--help"}, {"add", "a.test"}, {"apply", "--yes"}, {""}} {
		var stderr bytes.Buffer
		code := Run(args, strings.NewReader("127.0.0.1 a.test\n"), &bytes.Buffer{}, &stderr, Config{HostsPath: path, Exec: failExec(t)})
		if code != ExitRefused || !strings.Contains(stderr.String(), "usage") {
			t.Errorf("%q: exit %d stderr %q", args, code, stderr.String())
		}
	}
	assertUnchanged(t, path, orig)
}

func TestRunRefusesBrokenHostsFiles(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real")
	must(t, os.WriteFile(real, []byte("x\n"), 0o644))
	link := filepath.Join(dir, "hosts")
	must(t, os.Symlink(real, link))
	corrupt, corruptOrig := hostsFixture(t, BeginMarker+"\n127.0.0.1 a.test\n")
	for name, p := range map[string]string{"symlink": link, "missing": filepath.Join(dir, "none"), "corrupt": corrupt, "directory": dir} {
		var stderr bytes.Buffer
		if code := Run([]string{"apply"}, strings.NewReader("127.0.0.1 a.test\n"), &bytes.Buffer{}, &stderr, Config{HostsPath: p}); code != ExitFailure {
			t.Errorf("%s apply: exit %d", name, code)
		}
		if code := Run([]string{"check"}, nil, &bytes.Buffer{}, &stderr, Config{HostsPath: p}); code != ExitFailure {
			t.Errorf("%s check: exit %d", name, code)
		}
	}
	if b, _ := os.ReadFile(real); string(b) != "x\n" {
		t.Fatal("the symlink target was written")
	}
	assertUnchanged(t, corrupt, corruptOrig)
}

func hostsFixture(t *testing.T, content string) (string, []byte) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "hosts")
	must(t, os.WriteFile(p, []byte(content), 0o644))
	return p, []byte(content)
}

func assertUnchanged(t *testing.T, path string, orig []byte) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(b, orig) {
		t.Fatalf("hosts changed: %q %v", b, err)
	}
	assertNoTemp(t, path)
}

func assertNoTemp(t *testing.T, path string) {
	t.Helper()
	ents, _ := os.ReadDir(filepath.Dir(path))
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".hosts.gash-hte-") {
			t.Fatalf("temporary file left: %s", e.Name())
		}
	}
}

func failExec(t *testing.T) func([]string) error {
	return func(argv []string) error {
		t.Errorf("unexpected command %q", argv)
		return nil
	}
}

func stat(t *testing.T, p string) os.FileInfo {
	t.Helper()
	fi, err := os.Stat(p)
	must(t, err)
	return fi
}

func owner(fi os.FileInfo) [2]uint32 {
	st := fi.Sys().(*syscall.Stat_t)
	return [2]uint32{st.Uid, st.Gid}
}

func inode(fi os.FileInfo) uint64 { return uint64(fi.Sys().(*syscall.Stat_t).Ino) }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestStrip(t *testing.T) {
	hosts := []byte("127.0.0.1 localhost\n" + BeginMarker + "\n127.0.0.1 a.test\n" + EndMarker + "\n::1 localhost\n")
	out, err := Strip(hosts)
	if err != nil || string(out) != "127.0.0.1 localhost\n::1 localhost\n" {
		t.Fatalf("%v %q", err, out)
	}
	if out, err := Strip([]byte("x\n")); err != nil || string(out) != "x\n" {
		t.Fatalf("no region: %v %q", err, out)
	}
	if _, err := Strip([]byte(BeginMarker + "\n")); err != ErrCorruptBlock {
		t.Fatalf("corrupt: %v", err)
	}
}

func TestRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hosts")
	orig := "127.0.0.1 localhost\n" + BeginMarker + "\n127.0.0.1 a.test\n" + EndMarker + "\n"
	if err := os.WriteFile(path, []byte(orig), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := Rewrite(path, Strip); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	fi, _ := os.Stat(path)
	if string(got) != "127.0.0.1 localhost\n" || fi.Mode().Perm() != 0o640 {
		t.Fatalf("%q %v", got, fi.Mode())
	}
	if err := Rewrite(path, func(h []byte) ([]byte, error) { return Splice(h, []byte("127.0.0.1 b.test\n")) }); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); !strings.HasSuffix(string(got), BeginMarker+"\n127.0.0.1 b.test\n"+EndMarker+"\n") {
		t.Fatalf("%q", got)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if err := Rewrite(link, Strip); err == nil {
		t.Fatal("symlink rewritten")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 2 {
		t.Fatalf("temporary file left: %v", entries)
	}
}
