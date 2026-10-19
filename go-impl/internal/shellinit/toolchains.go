// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"os"
	"strings"

	"github.com/mauriziofonte/gash/internal/config"
)

// toolchains is lib/shell/toolchains.sh: GASH_PATH_ADD and GASH_NVM.
//
//   - GASH_PATH_ADD: the binary splits the value on space, tab and newline
//     and never globs (SHELL-07), then emits `path_add 'w1' 'w2' ...`, so a
//     first word of --append, -h or --help keeps its 2.x path_add meaning
//     (SHELL-08 reproduced).
//   - GASH_NVM=on|1: export NVM_DIR and source nvm.sh and its completion.
//   - GASH_NVM=lazy: the binary resolves the default version directory (no
//     =~ in the shell, SHELL-10), the script prepends its bin and defines
//     the 2.x nvm stub.
func (st *state) toolchains(b *strings.Builder) {
	if v := st.s.Get("GASH_PATH_ADD"); v != "" {
		if words := config.SplitPathAdd(v); len(words) > 0 {
			b.WriteString("path_add")
			for _, w := range words {
				b.WriteString(" " + quote(w))
			}
			b.WriteByte('\n')
		}
	}

	switch st.s.Get("GASH_NVM") {
	case "on", "1":
		b.WriteString(nvmLoadFunc)
		b.WriteString(`export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"` + "\n")
		b.WriteString(`if [[ -s "$NVM_DIR/nvm.sh" ]] && ! declare -F nvm >/dev/null; then` + "\n")
		b.WriteString("    __gash_nvm_load\n")
		b.WriteString("fi\n")
	case "lazy":
		b.WriteString(nvmLoadFunc)
		b.WriteString(`export NVM_DIR="${NVM_DIR:-$HOME/.nvm}"` + "\n")
		b.WriteString(`if [[ -s "$NVM_DIR/nvm.sh" ]] && ! declare -F nvm >/dev/null; then` + "\n")
		if dir := st.nvmDefaultDir(); dir != "" {
			b.WriteString("    path_add " + quote(dir+"/bin") + "\n")
		}
		b.WriteString("    nvm() {\n")
		b.WriteString("        __gash_nvm_load --no-use || return 1\n")
		b.WriteString("        nvm \"$@\"\n")
		b.WriteString("    }\n")
		b.WriteString("fi\n")
	}
}

// nvmLoadFunc is lib/shell/toolchains.sh __gash_nvm_load, shared by the on
// and lazy branches.
const nvmLoadFunc = `__gash_nvm_load() {
    local nvm_dir="${NVM_DIR:-$HOME/.nvm}"
    unset -f nvm
    [[ -s "$nvm_dir/nvm.sh" ]] || {
        __gash_error "nvm is not installed in $nvm_dir."
        return 1
    }
    # shellcheck source=/dev/null
    source "$nvm_dir/nvm.sh" "$@"
    # shellcheck source=/dev/null
    [[ -r "$nvm_dir/bash_completion" ]] && source "$nvm_dir/bash_completion"
    return 0
}
`

// nvmDir is ${NVM_DIR:-$HOME/.nvm} as the binary sees it.
func (st *state) nvmDir() string {
	if d := st.c.Env.Get("NVM_DIR"); d != "" {
		return d
	}
	return st.home + "/.nvm"
}

// nvmDefaultDir ports lib/shell/toolchains.sh __gash_nvm_default_dir: the
// directory of the version nvm's "default" alias points to, following the
// alias chain (up to 5 hops) the way nvm resolves it against what is
// installed. "" when it cannot tell. Done in Go so the shell leaves no
// BASH_REMATCH behind (SHELL-10).
func (st *state) nvmDefaultDir() string {
	dir := st.nvmDir()
	name, ok := st.readLine(dir + "/alias/default")
	if !ok || name == "" {
		return ""
	}
	for hops := 0; hops < 5; hops++ {
		if strings.Contains(name, "/../") {
			break
		}
		next, ok := st.readLine(dir + "/alias/" + name)
		if !ok {
			break
		}
		name = next
	}
	switch {
	case name == "node" || name == "stable":
		name = ""
	case strings.HasPrefix(name, "v") && len(name) > 1 && name[1] >= '0' && name[1] <= '9':
		name = name[1:]
	case name != "" && name[0] >= '0' && name[0] <= '9':
		// a partial or exact version like "22" or "22.1.0"
	default:
		return ""
	}
	base := dir + "/versions/node"
	if name != "" && isExactVersion(name) && st.isDir(base+"/v"+name) {
		return base + "/v" + name
	}
	// Newest installed version matching the (possibly empty) prefix.
	entries, err := os.ReadDir(base)
	if err != nil {
		return ""
	}
	best := ""
	for _, en := range entries {
		if !en.IsDir() || !strings.HasPrefix(en.Name(), "v") {
			continue
		}
		ver := strings.TrimPrefix(en.Name(), "v")
		if !(name == "" || ver == name || strings.HasPrefix(ver, name+".")) {
			continue
		}
		if best == "" || versionLess(best, ver) {
			best = ver
		}
	}
	if best == "" {
		return ""
	}
	return base + "/v" + best
}

// readLine reads the first line of a file (IFS= read -r), without the
// trailing newline. ok is false when the file is unreadable.
func (st *state) readLine(path string) (string, bool) {
	data, err := st.e.readFile(path)
	if err != nil {
		return "", false
	}
	s := string(data)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return s, true
}

// isExactVersion is the 2.x ^[0-9]+\.[0-9]+\.[0-9]+$ test.
func isExactVersion(v string) bool {
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return false
			}
		}
	}
	return true
}

// versionLess reports a < b in `sort -V` order for the node version strings
// the nvm resolver compares.
func versionLess(a, b string) bool {
	return sortVLess(a, b)
}

// sortVLess is a small version-sort comparison (numeric fields compared as
// integers, dot-separated), enough for node version directories.
func sortVLess(a, b string) bool {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		if x == y {
			continue
		}
		xn, xok := atoiAll(x)
		yn, yok := atoiAll(y)
		if xok && yok {
			return xn < yn
		}
		return x < y
	}
	return false
}

func atoiAll(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

// effectivePath is the PATH the alias-pack probes and the shadow rule see:
// the caller's PATH after GASH_PATH_ADD and GASH_NVM=lazy, computed with the
// 2.x path_add semantics so a tool reached only through an added directory
// is found exactly as the shell would find it.
func (st *state) effectivePath() string {
	path := st.c.Env.Get("PATH")
	if v := st.s.Get("GASH_PATH_ADD"); v != "" {
		path = st.pathAdd(path, config.SplitPathAdd(v))
	}
	if st.s.Get("GASH_NVM") == "lazy" {
		dir := st.nvmDir()
		if st.fileNonEmpty(dir + "/nvm.sh") {
			if d := st.nvmDefaultDir(); d != "" {
				path = st.pathAdd(path, []string{d + "/bin"})
			}
		}
	}
	return path
}

// fileNonEmpty is [[ -s FILE ]].
func (st *state) fileNonEmpty(path string) bool {
	fi, err := st.e.stat(path)
	return err == nil && !fi.IsDir() && fi.Size() > 0
}

// pathAdd reproduces lib/core/utils.sh path_add over a PATH string: a first
// word of --append appends (else prepend), -h / --help adds nothing
// (SHELL-08), ~ and ~/ expand, one trailing slash drops, missing or
// duplicate directories are skipped, and the order given is kept.
func (st *state) pathAdd(path string, words []string) string {
	if len(words) == 0 {
		return path
	}
	appendMode := false
	switch words[0] {
	case "--append":
		appendMode = true
		words = words[1:]
	case "-h", "--help":
		return path
	}
	order := words
	if !appendMode {
		order = make([]string, len(words))
		for i, w := range words {
			order[len(words)-1-i] = w
		}
	}
	entries := strings.Split(path, ":")
	in := func(d string) bool {
		for _, e := range entries {
			if e == d {
				return true
			}
		}
		return false
	}
	for _, dir := range order {
		dir = expandTilde(dir, st.home)
		dir = strings.TrimSuffix(dir, "/")
		if dir == "" || !st.isDir(dir) || in(dir) {
			continue
		}
		if appendMode {
			if path == "" {
				path = dir
			} else {
				path = path + ":" + dir
			}
			entries = append(entries, dir)
		} else {
			if path == "" {
				path = dir
			} else {
				path = dir + ":" + path
			}
			entries = append([]string{dir}, entries...)
		}
	}
	return path
}

// expandTilde is lib/core/utils.sh __gash_expand_tilde_path: ~ and a leading
// ~/ expand to HOME; ~user is left alone.
func expandTilde(p, home string) string {
	switch {
	case strings.HasPrefix(p, "~/"):
		return home + "/" + p[2:]
	case p == "~":
		return home
	}
	return p
}
