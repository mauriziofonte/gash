// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/mauriziofonte/gash/internal/platform"
)

func TestProcAddr(t *testing.T) {
	for in, want := range map[string]string{
		"0100007F":                         "127.0.0.1",
		"00000000":                         "0.0.0.0",
		"00000000000000000000000001000000": "[::1]",
		"0000000000000000FFFF00000100007F": "[127.0.0.1]",
	} {
		if got := procAddr(in); got != want {
			t.Errorf("procAddr(%s) = %s, want %s", in, got, want)
		}
	}
}

func TestListeners(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "proc/net"), 0o755); err != nil {
		t.Fatal(err)
	}
	tcp := "  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode\n" +
		"   0: 00000000:0050 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 1234 1 0 100 0 0 10 0\n" +
		"   1: 0100007F:1F90 0100007F:9C40 01 00000000:00000000 00:00000000 00000000  1000        0 5678 1 0 100 0 0 10 0\n"
	if err := os.WriteFile(filepath.Join(dir, "proc/net/tcp"), []byte(tcp), 0o644); err != nil {
		t.Fatal(err)
	}
	sr, err := platform.NewSysroot(dir)
	if err != nil {
		t.Fatal(err)
	}
	ls := listeners(sr)
	if got := ls[80]; len(got) != 1 || got[0].addr != "0.0.0.0:80" || got[0].uid != 0 || got[0].inode != "1234" {
		t.Errorf("port 80: %+v", got)
	}
	if len(ls[8080]) != 0 {
		t.Errorf("an established socket counted as listening: %+v", ls[8080])
	}
}

// Before install doctor reports the machine and fails on the install.
func TestDoctorNotInstalled(t *testing.T) {
	f := newFixture(t)
	res, err := f.e.Doctor(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.OK {
		t.Error("doctor OK before install")
	}
	i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "install" })
	if i < 0 || res.Checks[i].Status != CheckFail || res.Checks[i].Remedy != "run gash hte install" {
		t.Errorf("install check: %+v", res.Checks)
	}
	if i := slices.IndexFunc(res.Checks, func(c Check) bool { return c.Name == "php" }); i < 0 || res.Checks[i].Status != CheckOK {
		t.Errorf("php check: %+v", res.Checks)
	}
}
