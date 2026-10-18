// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package receipt

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"github.com/mauriziofonte/gash/internal/hte/hteerr"
)

const sum = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func sample() *Receipt {
	r := New("me", 1000, "ubuntu-24.04", time.Date(2026, 10, 18, 10, 0, 0, 0, time.UTC))
	r.Add(Change{Kind: DirCreated, Path: "/usr/local/libexec/gash"})
	r.Add(Change{Kind: FileCreated, Path: "/usr/local/libexec/gash/gash-hosts-helper", SHA256: sum, Mode: "0755"})
	r.Add(Change{Kind: Sysctl, Key: "net.ipv4.ip_unprivileged_port_start", Prior: Raw("1024"), Applied: Raw("80"),
		File: &FilePrior{Path: "/etc/sysctl.d/50-gash-hte.conf"}})
	r.Add(Change{Kind: Linger, Prior: Raw(false), Applied: Raw(true)})
	r.Add(Change{Kind: Unit, Name: "apache2.service", Prior: Raw(UnitState{true, true}), Applied: Raw(UnitState{})})
	r.Add(Change{Kind: HostsBlock, Prior: Raw(nil)})
	r.Add(Change{Kind: LegacyVhost, Path: "/etc/apache2/sites-available/001-shop.test.conf", Prior: Raw(VhostState{true}),
		Archive: ArchiveDir + "/20261018T100000Z/etc/apache2/sites-available/001-shop.test.conf",
		Link:    &Link{Path: "/etc/apache2/sites-enabled/001-shop.test.conf", Target: "../sites-available/001-shop.test.conf"}})
	r.Add(Change{Kind: Autostart, AutoMode: "shell"})
	return r
}

func TestRoundTrip(t *testing.T) {
	r := sample()
	file := filepath.Join(t.TempDir(), "receipt.json")
	if err := Save(file, r, 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok, err := Load(file)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if diff := cmp.Diff(r, got); diff != "" {
		t.Fatal(diff)
	}
	if c, _ := got.Last(); c.ID != 8 {
		t.Fatalf("last %d", c.ID)
	}
	if c, ok := got.Find(Unit, "apache2.service"); !ok || c.ID != 5 {
		t.Fatalf("find %+v", c)
	}
	var st UnitState
	if err := Decode(got.Changes[4].Prior, &st); err != nil || !st.Enabled || !st.Active {
		t.Fatalf("%+v %v", st, err)
	}
	got.Remove(5)
	if _, ok := got.Find(Unit, "apache2.service"); ok {
		t.Fatal("removed change found")
	}
	if _, ok, err := Load(filepath.Join(t.TempDir(), "none")); ok || err != nil {
		t.Fatal("missing receipt")
	}
}

func TestValidateRefuses(t *testing.T) {
	for name, mut := range map[string]func(*Receipt){
		"schema":        func(r *Receipt) { r.Schema = 2 },
		"user":          func(r *Receipt) { r.User = "a b" },
		"uid":           func(r *Receipt) { r.UID = 0 },
		"order":         func(r *Receipt) { r.Changes[1].ID = 1 },
		"kind":          func(r *Receipt) { r.Changes[0].Kind = "exec" },
		"relative path": func(r *Receipt) { r.Changes[0].Path = "usr/local" },
		"unclean path":  func(r *Receipt) { r.Changes[0].Path = "/usr/local/../etc" },
		"sha":           func(r *Receipt) { r.Changes[1].SHA256 = "x" },
		"archive":       func(r *Receipt) { r.Changes[6].Archive = "/tmp/x" },
		"unit state":    func(r *Receipt) { r.Changes[4].Prior = nil },
		"sysctl file":   func(r *Receipt) { p := "/tmp/backup"; r.Changes[2].File.Prior = &p },
	} {
		r := sample()
		mut(r)
		data, _ := Marshal(r)
		if _, err := Parse(data, "r.json"); hteerr.CodeOf(err) != hteerr.InvalidState {
			t.Errorf("%s: %v", name, err)
		}
	}
	data, _ := Marshal(sample())
	bad := strings.Replace(string(data), `"schema":1,`, `"schema":1,"extra":1,`, 1)
	if _, err := Parse([]byte(bad), "r.json"); hteerr.CodeOf(err) != hteerr.InvalidState {
		t.Fatalf("unknown field: %v", err)
	}
}

// TestCompare: the copies must be byte-identical (the HTE-61 tamper case).
func TestCompare(t *testing.T) {
	data, _ := Marshal(sample())
	if err := Compare("u", data, "r", data); err != nil {
		t.Fatal(err)
	}
	r := sample()
	r.Changes = r.Changes[:3]
	tampered, _ := Marshal(r)
	if err := Compare("u", tampered, "r", data); hteerr.CodeOf(err) != hteerr.ReceiptMismatch {
		t.Fatalf("%v", err)
	}
}
