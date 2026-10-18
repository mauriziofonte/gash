// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hteerr

import (
	"fmt"
	"testing"
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
		"conflicting_options": Retry, "invalid_state": Stop,
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
