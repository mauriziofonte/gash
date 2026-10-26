// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mauriziofonte/gash/internal/platform"
)

// debianCopy is a writable copy of the Debian sysroot, with the given
// files (relative paths, mode 0755) added and removed.
func debianCopy(t *testing.T, add, remove []string) platform.Sysroot {
	t.Helper()
	src, err := filepath.Abs(filepath.Join(sysroots, "debian"))
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(src)); err != nil {
		t.Fatal(err)
	}
	for _, rel := range add {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, rel := range remove {
		if err := os.Remove(filepath.Join(dir, rel)); err != nil {
			t.Fatal(err)
		}
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	return sr
}

// A program gash hte runs as the user is found in Homebrew's directory,
// ~/.local/bin or the user's PATH; sudo only in the system directories
// (3.4: an openssl outside the base PATH was "not installed").
func TestFindUserDirs(t *testing.T) {
	f := newFixture(t)
	f.e.Sysroot = debianCopy(t, []string{"home/me/.local/bin/openssl", "home/me/.local/bin/sudo", "srv/tools/systemctl"}, nil)
	f.env["HOME"] = "/home/me"
	f.env["PATH"] = "/srv/tools:relative"
	if got := f.e.find("openssl"); got != "/home/me/.local/bin/openssl" {
		t.Errorf("openssl: %q", got)
	}
	if got := f.e.find("systemctl"); got != "/srv/tools/systemctl" {
		t.Errorf("systemctl: %q", got)
	}
	if got := f.e.find("sudo"); got != "" {
		t.Errorf("sudo taken from a user directory: %q", got)
	}
	// The doctor sees that openssl.
	res, err := f.e.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "openssl" }); i < 0 ||
		res.Checks[i].Status != CheckOK || res.Checks[i].Details != "/home/me/.local/bin/openssl" {
		t.Errorf("openssl check: %+v", res.Checks)
	}
}

// Without PHP-FPM the doctor prints the install command of the platform
// (3.4: "install php-fpm with the system package manager", while
// PHPInstallHint went unused).
func TestDoctorPHPInstallCommand(t *testing.T) {
	f := newFixture(t)
	f.e.Sysroot = debianCopy(t, nil, []string{"usr/sbin/php-fpm8.3", "usr/sbin/php-fpm8.2", "usr/sbin/php-fpm7.4"})
	res, err := f.e.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "php" })
	if i < 0 || res.Checks[i].Status != CheckWarn || res.Checks[i].Details != "no PHP-FPM version is installed" ||
		res.Checks[i].Remedy != "sudo apt install php-fpm" {
		t.Errorf("php check: %+v", res.Checks)
	}
}
