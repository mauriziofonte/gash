// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hteerr

import (
	"fmt"
	"slices"
	"testing"

	"github.com/mauriziofonte/gash/internal/contract"
)

// The actions of design/hte.md section 9, verbatim.
func TestCatalogActions(t *testing.T) {
	want := map[Code]Action{
		"hte_not_installed": Stop, "site_not_found": Retry, "site_exists": Retry,
		"invalid_domain": Retry, "invalid_docroot": Retry, "docroot_not_found": Retry,
		"php_version_not_installed": Retry, "invalid_profile": Retry,
		"confirmation_required": Retry, "lock_timeout": Retry, "port_in_use": Stop,
		"privileged_step_required": Stop, "config_test_failed": Stop, "reload_failed": Stop,
		"apache_not_found": Fatal, "platform_unsupported": Fatal, "receipt_mismatch": Fatal,
		"root_not_allowed": Fatal, "missing_value": Retry,
		"conflicting_options": Retry, "invalid_state": Stop, "apache_module_missing": Stop,
		"openssl_not_found": Fatal, "php_not_enabled": Retry, "invalid_ports": Retry, "invalid_autostart": Retry,
	}
	if len(Codes()) != len(want) {
		t.Fatalf("catalog has %d codes, want %d", len(Codes()), len(want))
	}
	for c, a := range want {
		if got := ActionOf(c); got != a {
			t.Errorf("%s: %s, want %s", c, got, a)
		}
	}
	if ActionOf("no_such_code") != Stop {
		t.Error("unknown code must be STOP")
	}
}

func TestErrorValue(t *testing.T) {
	e := New(ApacheNotFound, "no apache2 binary", "install apache2")
	if e.Recoverable() || e.Action() != Fatal || e.Error() != "apache_not_found: no apache2 binary" {
		t.Fatalf("%+v", e)
	}
	wrapped := fmt.Errorf("render: %w", Newf(InvalidDocroot, "", "bad %q", "x"))
	if got, ok := As(wrapped); !ok || got.Details != `bad "x"` || CodeOf(wrapped) != InvalidDocroot {
		t.Fatalf("As: %v %v", got, ok)
	}
	if CodeOf(fmt.Errorf("plain")) != "" || New(SiteExists, "", "").Error() != "site_exists" {
		t.Fatal("CodeOf/Error")
	}
}

// pendingContract are the codes internal/contract does not register yet;
// the lead adds their rows (area "hte", origin 3.x) and empties the list.
var pendingContract = []Code{
	ApacheModuleMissing, RootNotAllowed, ConflictingOptions, InvalidState,
	OpenSSLNotFound, PHPNotEnabled, InvalidPorts, InvalidAutostart,
}

// Every code of the HTE catalog is a row of the contract catalog with the
// same action (LEDGER HTE-72), but for the pending ones.
func TestContractCatalog(t *testing.T) {
	for _, c := range Codes() {
		a, ok := contract.ActionOf(string(c))
		pending := slices.Contains(pendingContract, c)
		switch {
		case ok && pending:
			t.Errorf("%s is registered now: drop it from pendingContract", c)
		case !ok && !pending:
			t.Errorf("%s is not in the contract catalog", c)
		case ok && string(a) != string(ActionOf(c)):
			t.Errorf("%s: the contract says %s, the HTE catalog %s", c, a, ActionOf(c))
		}
	}
}
