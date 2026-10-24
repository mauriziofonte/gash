// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/mauriziofonte/gash/internal/hte/receipt"
)

// The command line sees the root steps before sudo starts; a failure
// after root steps ran returns what they changed with the error, so that
// the caller prints that part; a failure before any returns no result.
func TestPartialResults(t *testing.T) {
	ctx := context.Background()
	m := newRig(t)
	var shown [][]string
	m.e.BeforeSudo = func(steps []string) { shown = append(shown, steps) }

	if res, err := m.e.Install(ctx, InstallOptions{Ports: "low"}); res != nil || err == nil {
		t.Errorf("invalid ports: %+v %v", res, err)
	}
	m.failRoot = "visudo"
	res, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "standard", Autostart: "none"})
	if err == nil || res == nil || !slices.ContainsFunc(res.Changes, func(c receipt.Change) bool { return c.Path == "/usr/local/libexec/gash/gash-hosts-helper" }) {
		t.Fatalf("failed install: %+v %v", res, err)
	}
	if len(shown) != 1 || !slices.Equal(shown[0], res.Plan) {
		t.Errorf("shown %q, plan %q", shown, res.Plan)
	}

	m.failRoot = ""
	if _, err := m.e.Install(ctx, InstallOptions{Yes: true, Ports: "standard", Autostart: "none"}); err != nil {
		t.Fatal(err)
	}
	m.failRoot = "sysctl"
	un, err := m.e.Uninstall(ctx, UninstallOptions{Yes: true})
	if err == nil || un == nil || len(un.Restored) == 0 {
		t.Fatalf("failed uninstall: %+v %v", un, err)
	}
	if len(shown) != 3 || shown[2][0] != "undo change "+strconv.Itoa(un.Restored[0].ID)+" of the receipt" {
		t.Errorf("uninstall steps shown: %q", shown)
	}
}
