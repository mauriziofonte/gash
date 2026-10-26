// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mauriziofonte/gash/internal/config"
)

// The paths the user protects (PROTECT: lines of ~/.gash_env, see
// config.Protect). A PROTECT:all tree is refused like /root: no llm_* or
// agent_* function reads, lists or writes it, and a walk leaves it out. A
// PROTECT:content tree holds secret files: its names and metadata are
// shown, its content is never read, searched, hashed or written.

// Rule is one protected path.
type Rule struct {
	Mode string // config.ProtectContent or config.ProtectAll
	Path string // absolute, ~ or ~/... (the home of the machine it applies on)
}

// ProtectVar carries the rules of a host to the gash binary that runs
// there for a --host call: one "MODE PATH" line each. They add to the
// rules of that host's own file; a variable can only add rules, never
// remove one.
const ProtectVar = "GASH__PROTECT"

// EncodeRules is rules as the value of ProtectVar.
func EncodeRules(rules []Rule) string {
	var b strings.Builder
	for _, r := range rules {
		b.WriteString(r.Mode + " " + r.Path + "\n")
	}
	return b.String()
}

// DecodeRules reads a ProtectVar value; lines that are not "MODE PATH"
// with a known mode and an absolute or ~ path are skipped.
func DecodeRules(v string) []Rule {
	var out []Rule
	for _, line := range strings.Split(v, "\n") {
		mode, p, ok := strings.Cut(line, " ")
		if !ok || (mode != config.ProtectContent && mode != config.ProtectAll) {
			continue
		}
		if strings.HasPrefix(p, "/") || p == "~" || strings.HasPrefix(p, "~/") {
			out = append(out, Rule{Mode: mode, Path: p})
		}
	}
	return out
}

// FileRules is the rules of the PROTECT: lines that apply on host alias
// ("" is this machine).
func FileRules(f *config.File, alias string) []Rule {
	var out []Rule
	for _, p := range f.ProtectFor(alias) {
		out = append(out, Rule{Mode: p.Mode, Path: p.Path})
	}
	return out
}

// rulesCache keeps the parsed file between the guards of one process
// (a walk builds many), keyed on the file and its size and time.
var rulesCache sync.Map // string -> []Rule

// Rules is the rules of a call (get returns its variables): the PROTECT:
// lines of its ~/.gash_env (GASH_ENV_FILE) that apply on this machine,
// plus those ProtectVar carries.
func Rules(get func(string) string) []Rule {
	home := get("HOME")
	path := config.FilePath(home, get("GASH_ENV_FILE"))
	var rules []Rule
	if fi, err := os.Stat(path); err == nil && fi.Mode().IsRegular() {
		key := path + "\x00" + home + "\x00" + fi.ModTime().String() + "\x00" + strconv.FormatInt(fi.Size(), 10)
		if v, ok := rulesCache.Load(key); ok {
			rules = v.([]Rule)
		} else {
			rules = FileRules(config.LoadCaller(get), "")
			rulesCache.Store(key, rules)
		}
	}
	if v := get(ProtectVar); v != "" {
		rules = append(append([]Rule{}, rules...), DecodeRules(v)...)
	}
	return rules
}

// WithRules adds rules to the guard: each path, ~ expanded to Home, is
// kept in its resolved and its lexical form, so a path reached through a
// symlink and the link itself are both caught. It returns g.
func (g *Guard) WithRules(rules []Rule) *Guard {
	for _, r := range rules {
		p := r.Path
		if p == "~" || strings.HasPrefix(p, "~/") {
			if g.Home == "" {
				continue
			}
			p = g.Home + p[1:]
		}
		if !strings.HasPrefix(p, "/") {
			continue
		}
		forms := []string{}
		for _, lexical := range []bool{false, true} {
			if f, err := g.Realpath(p, lexical); err == nil {
				forms = append(forms, f)
			}
		}
		switch r.Mode {
		case config.ProtectAll:
			g.Hidden = append(g.Hidden, forms...)
		case config.ProtectContent:
			g.Sealed = append(g.Sealed, forms...)
		}
	}
	return g
}

// below reports whether a is t or lies below it (whole components).
func below(a, t string) bool {
	return a == t || t == "/" || strings.HasPrefix(a, t+"/")
}

// forms is path (~ and ~/ expanded to Home) in the forms the guard
// compares: resolved, resolved without a leading /private (macOS) and
// lexical.
func (g *Guard) forms(path string) []string {
	if (path == "~" || strings.HasPrefix(path, "~/")) && g.Home != "" {
		path = g.Home + path[1:]
	}
	if !strings.HasPrefix(path, "/") {
		path = strings.TrimSuffix(g.Cwd, "/") + "/" + path
	}
	out := make([]string, 0, 3)
	if n, err := g.Realpath(path, false); err == nil {
		out = append(out, n, strings.TrimPrefix(n, "/private"))
	}
	if l, err := g.Realpath(path, true); err == nil {
		out = append(out, l)
	}
	return out
}

// inTrees reports whether any form of path is at or below one of trees.
func (g *Guard) inTrees(path string, trees []string) bool {
	if len(trees) == 0 || path == "" {
		return false
	}
	for _, cand := range g.forms(path) {
		for _, t := range trees {
			if below(cand, t) {
				return true
			}
		}
	}
	return false
}

// Hides reports whether path is at or below a PROTECT:all tree.
func (g *Guard) Hides(path string) bool { return g.inTrees(path, g.Hidden) }

// Seals reports whether path is at or below a PROTECT:content tree.
func (g *Guard) Seals(path string) bool { return g.inTrees(path, g.Sealed) }

// sealedFile is Seals for IsSecretFile, which judges every word of an
// llm_exec line: a bare name (no /, no ~) counts only when it names
// something in the working directory, so a word such as "diff" typed
// inside a PROTECT:content tree is not a file of it.
func (g *Guard) sealedFile(file string) bool {
	if len(g.Sealed) == 0 {
		return false
	}
	if !strings.ContainsRune(file, '/') && !strings.HasPrefix(file, "~") && !g.existsOrSymlink(strings.TrimSuffix(g.Cwd, "/")+"/"+file) {
		return false
	}
	return g.Seals(file)
}

// underTrees is the walk form of inTrees: path is already physical (an
// entry of a walk from a resolved root), so no lookup is needed.
func underTrees(path string, trees []string) bool {
	for _, cand := range [2]string{path, strings.TrimPrefix(path, "/private")} {
		for _, t := range trees {
			if below(cand, t) {
				return true
			}
		}
	}
	return false
}

// UnderTrees reports whether a physical path (resolved, as a walk from a
// resolved root meets it) is at or below one of trees, also with a
// leading /private stripped (macOS). A walk outside this package, the
// scanners' and gash logs', tests its entries against Hidden and Sealed
// with it.
func UnderTrees(path string, trees []string) bool {
	if len(trees) == 0 || path == "" {
		return false
	}
	return underTrees(strings.TrimSuffix(path, "/"), trees)
}

// WalkSkip is the per-entry test of a walk from a resolved root: the
// protected trees (UnderForbidden), and with mode "content" the
// PROTECT:content trees too, whose files a content walk may not open.
func (g *Guard) WalkSkip(mode string) func(string) bool {
	if mode != "content" || len(g.Sealed) == 0 {
		return g.UnderForbidden
	}
	return func(p string) bool {
		if g.UnderForbidden(p) {
			return true
		}
		if !strings.HasPrefix(p, "/") {
			p = strings.TrimSuffix(g.Cwd, "/") + "/" + p
		}
		return underTrees(strings.TrimSuffix(p, "/"), g.Sealed)
	}
}

// Trees is every tree a walk may have to leave out: the protected trees
// and the user's PROTECT: trees of both modes.
func (g *Guard) Trees() []string {
	out := append([]string{}, g.Forbidden...)
	out = append(out, g.Hidden...)
	return append(out, g.Sealed...)
}

// AboveProtected reports whether a walk that cannot leave a tree out (a
// program llm_exec runs, such as find or grep -r) would enter one of the
// user's trees from path: a PROTECT:all tree lies below it, or, with mode
// "content", a PROTECT:content tree does.
func (g *Guard) AboveProtected(path, mode string) bool {
	trees := g.Hidden
	if mode == "content" {
		trees = append(append([]string{}, g.Hidden...), g.Sealed...)
	}
	if len(trees) == 0 {
		return false
	}
	for _, cand := range g.forms(path) {
		cand = strings.TrimSuffix(cand, "/")
		for _, t := range trees {
			if cand == "" || strings.HasPrefix(t, cand+"/") {
				return true
			}
		}
	}
	return false
}
