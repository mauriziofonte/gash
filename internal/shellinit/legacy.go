// Gash 4.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/selfmgmt"
)

// legacyReminder is the 4.0 notice of an interactive shell whose startup
// files (~/.bashrc, ~/.bash_local and the others of gash doctor --legacy)
// call a former flat name of Gash 3.x that this shell no longer defines:
// one line on stderr, at most once a day, never with GASH_LEGACY_NAMES=1.
// The day is stamped only when the line is printed, so a shell whose files
// are clean writes nothing.
func (st *state) legacyReminder(b *strings.Builder) {
	if st.s.Get(registry.LegacyVar) == "1" {
		return
	}
	dir := st.stateDir()
	now := st.e.now()
	if data, err := st.e.readFile(dir + "/legacy-notice"); err == nil {
		if n := digitsOnly(string(data)); n != "" {
			if last, perr := strconv.ParseInt(n, 10, 64); perr == nil && now-last < 86400 {
				return
			}
		}
	}
	var files, names []string
	for _, f := range selfmgmt.StartupFiles {
		data, err := st.e.readFile(st.home + "/" + f)
		if err != nil {
			continue
		}
		hits := registry.ScanLegacy(string(data))
		if len(hits) == 0 {
			continue
		}
		files = append(files, config.ShownPath(st.home+"/"+f, st.home))
		for _, h := range hits {
			if !contains(names, h.Name) {
				names = append(names, h.Name)
			}
		}
	}
	if len(names) == 0 {
		return
	}
	if err := st.e.mkdirAll(dir, 0o755); err == nil {
		_ = st.e.writeFile(dir+"/legacy-notice", []byte(strconv.FormatInt(now, 10)+"\n"), 0o644)
	}
	b.WriteString("__gash_note " + quote(LegacyNotice(files, names)) + "\n")
}

// LegacyNotice is the line of legacyReminder.
func LegacyNotice(files, names []string) string {
	shown := names
	if len(shown) > 3 {
		shown = append(append([]string(nil), shown[:3]...), "...")
	}
	verb := " calls "
	if len(files) > 1 {
		verb = " call "
	}
	return "gash: " + strings.Join(files, " and ") + verb + strings.Join(shown, ", ") +
		", former names of Gash 3.x that Gash 4 no longer defines; gash doctor --legacy lists them, GASH_LEGACY_NAMES=1 keeps them"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
