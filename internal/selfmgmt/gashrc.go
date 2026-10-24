// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"embed"
	"strings"
)

// knownGashrc holds every ~/.gashrc an installer ever wrote: the 2.x
// template (gashrc/2x.gashrc, the repository's .gashrc) and the four 1.x
// ones. A ~/.gashrc equal to one of them, or to the 2.x custom-directory
// variant, holds nothing of the user's (LEDGER SELF-02).
//
//go:embed gashrc/*.gashrc
var knownGashrc embed.FS

// gashrcHomeLine is the line the installer rewrites for a custom install
// directory (install.sh: __gash_home="${GASH_DIR:-DIR}").
const gashrcHomeLine = "__gash_home="

// pristineGashrc reports whether data is a ~/.gashrc as an installer wrote
// it, so that uninstall may remove it without keeping a copy.
func pristineGashrc(data []byte) bool {
	entries, err := knownGashrc.ReadDir("gashrc")
	if err != nil {
		return false
	}
	got := string(data)
	for _, e := range entries {
		tmpl, err := knownGashrc.ReadFile("gashrc/" + e.Name())
		if err != nil {
			continue
		}
		if got == string(tmpl) || customDirVariant(got, string(tmpl)) {
			return true
		}
	}
	return false
}

// customDirVariant reports whether got is tmpl with its __gash_home= line
// replaced by the installer's custom-directory line.
func customDirVariant(got, tmpl string) bool {
	g, w := strings.Split(got, "\n"), strings.Split(tmpl, "\n")
	if len(g) != len(w) {
		return false
	}
	replaced := false
	for i := range w {
		if g[i] == w[i] {
			continue
		}
		if replaced || !strings.HasPrefix(w[i], gashrcHomeLine) ||
			!strings.HasPrefix(g[i], gashrcHomeLine+`"${GASH_DIR:-`) || !strings.HasSuffix(g[i], `}"`) {
			return false
		}
		replaced = true
	}
	return true
}
