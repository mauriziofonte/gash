// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/gitx"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

// repoGuard tests the names git prints (relative to the repository root)
// against the paths the user protects (PROTECT: in ~/.gash_env): a name
// in a PROTECT:all tree is left out of every listing, one in a
// PROTECT:content tree is listed without its content. A nil *repoGuard
// (the user protects nothing) keeps every name.
type repoGuard struct {
	g   *pathguard.Guard
	top string
}

// repoGuard is the guard of dir's repository; nil when the user protects
// nothing, or when git names no work tree (a bare repository has no
// files to protect).
func (m *Module) repoGuard(c *invoke.Call, dir string) *repoGuard {
	g := m.guard(c)
	if len(g.Hidden) == 0 && len(g.Sealed) == 0 {
		return nil
	}
	res := gitx.Safe(c.Ctx, m.run, c.Env.Child(), nil, nil, io.Discard, "-C", dir, "rev-parse", "--show-toplevel")
	top := strings.TrimSuffix(string(res.Stdout), "\n")
	if !res.Success() || !strings.HasPrefix(top, "/") {
		return nil
	}
	return &repoGuard{g: g, top: top}
}

func (r *repoGuard) abs(name string) string { return strings.TrimSuffix(r.top, "/") + "/" + name }

// hides reports whether a repository name is in a PROTECT:all tree.
func (r *repoGuard) hides(name string) bool {
	return r != nil && name != "" && r.g.Hides(r.abs(name))
}

// seals reports whether a repository name is in a PROTECT:content tree.
func (r *repoGuard) seals(name string) bool {
	return r != nil && name != "" && r.g.Seals(r.abs(name))
}

// keep is names without those hides reports.
func (r *repoGuard) keep(names []string) []string {
	if r == nil {
		return names
	}
	out := names[:0:0]
	for _, n := range names {
		if !r.hides(n) {
			out = append(out, n)
		}
	}
	return out
}

// keepStatus is the entries of st without those in a PROTECT:all tree.
func (r *repoGuard) keepStatus(st gitx.Status) gitx.Status {
	if r == nil {
		return st
	}
	var out []gitx.StatusEntry
	for _, e := range st.Entries {
		if !r.hides(e.Path) && !r.hides(e.Orig) {
			out = append(out, e)
		}
	}
	st.Entries = out
	return st
}

// keepChanges is changes without those in a PROTECT:all tree.
func (r *repoGuard) keepChanges(changes []gitx.FileChange) []gitx.FileChange {
	if r == nil {
		return changes
	}
	var out []gitx.FileChange
	for _, f := range changes {
		if !r.hides(f.Path) && !r.hides(f.Orig) {
			out = append(out, f)
		}
	}
	return out
}

// excludes are the pathspecs that keep the PROTECT:all trees inside the
// repository out of a git diff, show or log (for a call without a
// pathspec of its own: --literal-pathspecs turns the magic off).
func (r *repoGuard) excludes() []string {
	if r == nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	top := strings.TrimSuffix(r.top, "/")
	for _, h := range r.g.Hidden {
		rel, ok := strings.CutPrefix(h, top+"/")
		if !ok || rel == "" || seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, ":(top,exclude,literal)"+rel)
	}
	return out
}
