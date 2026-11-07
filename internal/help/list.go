// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package help

import (
	"slices"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
)

// The listings of lib/core/help.sh over the static registry. Names sort
// by bytes (LEDGER SELF-11) and modules come in registry.ModuleOrder
// (CORE-07), where 2.x used sort(1) in the caller's locale and Bash's
// hash order.

// Registered returns the functions with a registered help page, sorted
// by bytes: the keys of __GASH_HELP_REGISTRY.
func Registered() []*registry.Function {
	var out []*registry.Function
	fns := registry.Functions()
	for i := range fns {
		if fns[i].HasHelp {
			out = append(out, &fns[i])
		}
	}
	slices.SortFunc(out, byName)
	return out
}

// byName orders functions by the bytes of their names, as sort(1) under
// LC_ALL=C, whatever the caller's locale (LEDGER SELF-11).
func byName(a, b *registry.Function) int { return strings.Compare(a.Name, b.Name) }
