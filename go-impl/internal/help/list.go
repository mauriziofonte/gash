// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package help

import (
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
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

// FindByAlias is __gash_help_find_by_alias ALIAS: the function whose
// registered aliases include alias.
func FindByAlias(alias string) (*registry.Function, bool) {
	if alias == "" {
		return nil, false
	}
	for _, f := range Registered() {
		if slices.Contains(splitComma(f.Help.Aliases), alias) {
			return f, true
		}
	}
	return nil, false
}

// Row is __gash_help_row FUNCTION: name, "(aliases)" and the short
// description, tab separated.
func Row(f *registry.Function) string {
	aliases := ""
	if f.Help.Aliases != "" {
		aliases = "(" + f.Help.Aliases + ")"
	}
	return f.Name + "\t" + aliases + "\t" + f.Help.Short
}

// Widths is __gash_help_widths: the widest name and "(aliases)" of every
// registered function, so that consecutive tables line up.
func Widths() (name, aliases int) {
	for _, f := range Registered() {
		name = max(name, ui.Len(f.Name))
		if f.Help.Aliases != "" {
			aliases = max(aliases, ui.Len(f.Help.Aliases)+2)
		}
	}
	return name, aliases
}

// Rows is ROWS | __gash_help_rows INDENT WIDTH1 WIDTH2: the rows as a
// table fitted to the terminal, names and aliases in the accent color,
// descriptions muted.
func Rows(s ui.Style, w io.Writer, rows []string, indent, width1, width2 int) {
	s.Table(w, rows, "--no-header", "--indent", strconv.Itoa(indent),
		"--min", "1:"+strconv.Itoa(width1), "--min", "2:"+strconv.Itoa(width2), "--fit",
		"--color", "1:accent", "--color", "2:accent", "--color", "3:muted")
}

// mutedLine is printf '  %s%s%s\n' MUTED TEXT OFF, the colors only when
// __gash_use_color says so.
func mutedLine(s ui.Style, w io.Writer, text string) {
	if s.Color {
		text = s.P.Muted + text + s.P.Off
	}
	_, _ = io.WriteString(w, "  "+text+"\n")
}

// Search is __gash_help_search KEYWORD: the functions whose name,
// aliases, short description or help text contain keyword, case folded;
// false (after a "No results" line) when none does.
func Search(s ui.Style, w io.Writer, keyword string) bool {
	if keyword == "" {
		return false
	}
	kw := strings.ToLower(keyword)
	var rows []string
	for _, f := range Registered() {
		h := f.Help
		if strings.Contains(strings.ToLower(f.Name), kw) || strings.Contains(strings.ToLower(h.Aliases), kw) ||
			strings.Contains(strings.ToLower(h.Short), kw) || strings.Contains(strings.ToLower(h.Body), kw) {
			rows = append(rows, Row(f))
		}
	}
	if len(rows) == 0 {
		mutedLine(s, w, "No results for '"+keyword+"'.")
		return false
	}
	wn, wa := Widths()
	Rows(s, w, rows, 2, wn, wa)
	return true
}

// List is __gash_help_list: the registered functions grouped by module,
// in registry.ModuleOrder, then those without a module under OTHER.
func List(s ui.Style, w io.Writer) {
	wn, wa := Widths()
	byModule := map[string][]string{}
	var other []string
	for _, f := range Registered() {
		if f.Help.Module == "" {
			other = append(other, Row(f))
			continue
		}
		byModule[f.Help.Module] = append(byModule[f.Help.Module], Row(f))
	}
	order := append([]string(nil), registry.ModuleOrder...)
	var extra []string
	for m := range byModule {
		if !slices.Contains(order, m) {
			extra = append(extra, m)
		}
	}
	slices.Sort(extra)
	order = append(order, extra...)
	displayed := false
	for _, m := range order {
		rows, ok := byModule[m]
		if !ok {
			continue
		}
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", strings.ToUpper(m))
		Rows(s, w, rows, 4, wn, wa)
		displayed = true
	}
	if len(other) > 0 {
		_, _ = io.WriteString(w, "\n")
		s.Heading(w, "--indent", "2", "OTHER")
		Rows(s, w, other, 4, wn, wa)
	}
	if !displayed && len(other) == 0 {
		mutedLine(s, w, "No help entries registered.")
	}
}
