// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package platform

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/adrg/xdg"
	"github.com/google/go-cmp/cmp"
)

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSysrootPathMapping(t *testing.T) {
	root := t.TempDir()
	sr, err := NewSysroot(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := sr.Path("/etc/hosts"); got != root+"/etc/hosts" {
		t.Fatalf("Path = %q", got)
	}
	if got := sr.Path("/"); got != root {
		t.Fatalf("Path(/) = %q", got)
	}
	if got, ok := sr.SystemPath(root + "/etc/../etc/hosts"); !ok || got != "/etc/hosts" {
		t.Fatalf("SystemPath = %q %v", got, ok)
	}
	if _, ok := sr.SystemPath("/elsewhere"); ok {
		t.Fatal("SystemPath accepted a path outside the sysroot")
	}
	if Real().Path("/etc/hosts") != "/etc/hosts" || !Real().IsReal() {
		t.Fatal("Real is not the identity")
	}
	if _, err := NewSysroot("rel"); !errors.Is(err, ErrRelative) {
		t.Fatalf("relative sysroot: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("Path accepted a relative path")
		}
	}()
	sr.Path("etc/hosts")
}

func TestSysrootSymlinksStayInside(t *testing.T) {
	root := t.TempDir()
	sr, _ := NewSysroot(root)
	write(t, root+"/etc/apache2/sites-available/001-a.conf", "inside\n")
	// A relative link as a2ensite makes, an absolute link, and a link
	// whose absolute target exists on the host but not in the tree.
	must(t, os.MkdirAll(root+"/etc/apache2/sites-enabled", 0o755))
	must(t, os.Symlink("../sites-available/001-a.conf", root+"/etc/apache2/sites-enabled/001-a.conf"))
	must(t, os.Symlink("/etc/apache2/sites-available/001-a.conf", root+"/etc/apache2/sites-enabled/abs.conf"))
	must(t, os.Symlink("/etc/passwd", root+"/etc/apache2/sites-enabled/escape.conf"))
	must(t, os.Symlink("/etc/apache2", root+"/apache-link"))

	for _, p := range []string{"/etc/apache2/sites-enabled/001-a.conf", "/etc/apache2/sites-enabled/abs.conf", "/apache-link/sites-available/001-a.conf"} {
		b, err := sr.ReadFile(p)
		if err != nil || string(b) != "inside\n" {
			t.Fatalf("ReadFile(%s) = %q, %v", p, b, err)
		}
	}
	if _, err := sr.ReadFile("/etc/apache2/sites-enabled/escape.conf"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("escaping link read a host file: %v", err)
	}
	r, err := sr.Resolve("/etc/apache2/sites-enabled/abs.conf")
	if err != nil || r != "/etc/apache2/sites-available/001-a.conf" {
		t.Fatalf("Resolve = %q, %v", r, err)
	}
	fi, err := sr.Lstat("/etc/apache2/sites-enabled/abs.conf")
	if err != nil || fi.Mode()&fs.ModeSymlink == 0 {
		t.Fatalf("Lstat followed the link: %v", err)
	}
	if tgt, err := sr.Readlink("/etc/apache2/sites-enabled/001-a.conf"); err != nil || tgt != "../sites-available/001-a.conf" {
		t.Fatalf("Readlink = %q, %v", tgt, err)
	}
	if !sr.IsRegular("/etc/apache2/sites-enabled/abs.conf") || sr.IsDir("/etc/apache2/sites-enabled/abs.conf") {
		t.Fatal("IsRegular/IsDir")
	}
	// A loop ends with an error, not a hang.
	must(t, os.Symlink("/loop2", root+"/loop1"))
	must(t, os.Symlink("/loop1", root+"/loop2"))
	if _, err := sr.Resolve("/loop1/x"); err == nil {
		t.Fatal("symlink loop resolved")
	}
}

func TestSysrootWalkAndReadDir(t *testing.T) {
	root := t.TempDir()
	sr, _ := NewSysroot(root)
	write(t, root+"/etc/a/x.conf", "")
	write(t, root+"/etc/a/sub/y.conf", "")
	must(t, os.Symlink("/etc/a", root+"/etc/link"))
	var got []string
	err := sr.Walk("/etc/link", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		got = append(got, p)
		return nil
	})
	must(t, err)
	want := []string{"/etc/link", "/etc/link/sub", "/etc/link/sub/y.conf", "/etc/link/x.conf"}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("Walk (-want +got):\n%s", diff)
	}
	ents, err := sr.ReadDir("/etc/a")
	must(t, err)
	if len(ents) != 2 {
		t.Fatalf("ReadDir: %d entries", len(ents))
	}
}

func TestResolveDirs(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want Dirs
		err  error
	}{
		{"defaults", map[string]string{"HOME": "/home/me"}, Dirs{
			Home: "/home/me", ConfigHome: "/home/me/.config", StateHome: "/home/me/.local/state",
			DataHome: "/home/me/.local/share", CacheHome: "/home/me/.cache"}, nil},
		{"set", map[string]string{"HOME": "/home/me/", "XDG_CONFIG_HOME": "/c", "XDG_STATE_HOME": "/s/",
			"XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/k", "XDG_RUNTIME_DIR": "/run/user/1000"}, Dirs{
			Home: "/home/me", ConfigHome: "/c", StateHome: "/s", DataHome: "/d", CacheHome: "/k",
			RuntimeDir: "/run/user/1000"}, nil},
		{"relative values are ignored", map[string]string{"HOME": "/h", "XDG_CONFIG_HOME": "rel",
			"XDG_RUNTIME_DIR": "run"}, Dirs{Home: "/h", ConfigHome: "/h/.config", StateHome: "/h/.local/state",
			DataHome: "/h/.local/share", CacheHome: "/h/.cache"}, nil},
		{"no home", map[string]string{}, Dirs{}, ErrNoHome},
		{"relative home", map[string]string{"HOME": "home"}, Dirs{}, ErrNoHome},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ResolveDirs(func(k string) string { return tt.env[k] })
			if !errors.Is(err, tt.err) {
				t.Fatalf("err = %v, want %v", err, tt.err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Fatalf("(-want +got):\n%s", diff)
			}
		})
	}
}

// On Linux the results agree with github.com/adrg/xdg (macOS deliberately
// differs: adrg/xdg uses ~/Library there).
func TestResolveDirsMatchesAdrgXDGOnLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("adrg/xdg uses ~/Library on darwin; gash keeps the XDG layout")
	}
	for _, env := range []map[string]string{
		{"HOME": "/home/me"},
		{"HOME": "/home/me", "XDG_CONFIG_HOME": "/c", "XDG_STATE_HOME": "/s", "XDG_DATA_HOME": "/d", "XDG_CACHE_HOME": "/k"},
		{"HOME": "/home/me", "XDG_CONFIG_HOME": "relative"},
	} {
		for _, k := range []string{"HOME", "XDG_CONFIG_HOME", "XDG_STATE_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
			t.Setenv(k, env[k])
		}
		xdg.Reload()
		got, err := ResolveDirs(func(k string) string { return env[k] })
		must(t, err)
		want := Dirs{Home: xdg.Home, ConfigHome: xdg.ConfigHome, StateHome: xdg.StateHome,
			DataHome: xdg.DataHome, CacheHome: xdg.CacheHome}
		got.RuntimeDir = ""
		if diff := cmp.Diff(want, got); diff != "" {
			t.Fatalf("env %v (-adrg/xdg +gash):\n%s", env, diff)
		}
	}
}

// TestLedger_HTE33 covers the runtime-directory part of LEDGER HTE-33: the
// fallback is created 0700 and refused when it is a symlink, not a
// directory, accessible to others, or owned by another user.
func TestLedger_HTE33(t *testing.T) {
	d := Dirs{RuntimeDir: "/run/user/1000"}
	if got := d.RuntimeBase("gash-hte", 1000); got != "/run/user/1000/gash-hte" {
		t.Fatalf("RuntimeBase = %q", got)
	}
	if got := (Dirs{}).RuntimeBase("gash-hte", 1000); got != "/tmp/gash-hte-1000" {
		t.Fatalf("fallback RuntimeBase = %q", got)
	}
	uid := os.Getuid()
	base := t.TempDir()
	ok := filepath.Join(base, "ok")
	must(t, EnsurePrivateDir(ok, uid))
	fi, _ := os.Lstat(ok)
	if fi.Mode().Perm() != 0o700 {
		t.Fatalf("mode %v", fi.Mode())
	}
	must(t, EnsurePrivateDir(ok, uid)) // idempotent

	link := filepath.Join(base, "link")
	must(t, os.Symlink(ok, link))
	if err := EnsurePrivateDir(link, uid); !errors.Is(err, ErrUnsafeDir) {
		t.Fatalf("symlink: %v", err)
	}
	file := filepath.Join(base, "file")
	write(t, file, "")
	if err := EnsurePrivateDir(file, uid); !errors.Is(err, ErrUnsafeDir) {
		t.Fatalf("file: %v", err)
	}
	open := filepath.Join(base, "open")
	must(t, os.Mkdir(open, 0o700))
	must(t, os.Chmod(open, 0o755))
	if err := EnsurePrivateDir(open, uid); !errors.Is(err, ErrUnsafeDir) {
		t.Fatalf("0755: %v", err)
	}
	if err := EnsurePrivateDir(ok, uid+1); !errors.Is(err, ErrUnsafeDir) {
		t.Fatalf("foreign owner: %v", err)
	}
}

func TestDetectOS(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		files    map[string]string
		family   Family
		platform string
		wsl      bool
	}{
		{"ubuntu", "linux", map[string]string{"/etc/os-release": "PRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nNAME=\"Ubuntu\"\nVERSION_ID=\"24.04\"\nID=ubuntu\nID_LIKE=debian\n"}, FamilyDebian, "ubuntu-24.04", false},
		{"debian in usr/lib", "linux", map[string]string{"/usr/lib/os-release": "ID=debian\nVERSION_ID=\"12\"\n"}, FamilyDebian, "debian-12", false},
		{"fedora", "linux", map[string]string{"/etc/os-release": "ID=fedora\nVERSION_ID=40\n"}, FamilyFedora, "fedora-40", false},
		{"rocky", "linux", map[string]string{"/etc/os-release": "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\n"}, FamilyFedora, "rocky-9.4", false},
		{"arch", "linux", map[string]string{"/etc/os-release": "ID=arch\nBUILD_ID=rolling\n"}, FamilyArch, "arch", false},
		{"manjaro", "linux", map[string]string{"/etc/os-release": "ID=manjaro\nID_LIKE=arch\n"}, FamilyArch, "manjaro", false},
		{"wsl interop", "linux", map[string]string{"/etc/os-release": "ID=ubuntu\nVERSION_ID=22.04\n", "/proc/sys/fs/binfmt_misc/WSLInterop": "enabled\n"}, FamilyDebian, "ubuntu-22.04", true},
		{"wsl version", "linux", map[string]string{"/etc/os-release": "ID=ubuntu\n", "/proc/version": "Linux version 5.15.153.1-microsoft-standard-WSL2\n"}, FamilyDebian, "ubuntu", true},
		{"unknown", "linux", map[string]string{"/etc/os-release": "ID=alpine\nVERSION_ID=3.20.1\n"}, FamilyUnknown, "alpine-3.20.1", false},
		{"no os-release", "linux", nil, FamilyUnknown, "linux", false},
		{"macos", "darwin", map[string]string{"/System/Library/CoreServices/SystemVersion.plist": "<dict>\n\t<key>ProductName</key>\n\t<string>macOS</string>\n\t<key>ProductVersion</key>\n\t<string>14.5</string>\n</dict>\n"}, FamilyDarwin, "macos-14.5", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for p, c := range tt.files {
				write(t, root+p, c)
			}
			sr, _ := NewSysroot(root)
			o := DetectOS(sr, tt.goos)
			if o.Family != tt.family || o.Platform() != tt.platform || o.WSL != tt.wsl {
				t.Fatalf("got family %s platform %s wsl %v", o.Family, o.Platform(), o.WSL)
			}
		})
	}
}

func TestParseOSRelease(t *testing.T) {
	got := ParseOSRelease([]byte("# c\nA=plain\nB=\"x \\\"q\\\" \\$y \\\\ z\"\nC='single $x'\nlower=no\nD=\"unterminated\nE=\n"))
	want := map[string]string{"A": "plain", "B": `x "q" $y \ z`, "C": "single $x", "E": ""}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
	if !slices.Equal(DetectOS(Real(), "darwin").IDLike, nil) {
		t.Fatal("darwin has no ID_LIKE")
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
