// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import (
	"errors"
	"sort"
	"strings"
)

// BashTools are the tools the Bash side reports before the binary exists:
// what the installer and the first-run download need.
var BashTools = []string{"bash", "curl", "tar", "gzip", "sha256sum", "git", "grep", "sed"}

// BashFunctions is the Bash the installer and the bootstrap carry, so that
// a missing tool is named with its install command before the binary is
// there: PREFIXpkg_manager prints the package manager of the system, and
// PREFIXinstall_hint TOOL the command that installs TOOL (nothing when
// none is known). Both come from this catalog, so the three cannot drift.
// Each level is indented with unit spaces, and every line with indent.
func BashFunctions(prefix string, unit int, indent string) string {
	var b strings.Builder
	w := func(s string) { b.WriteString(s + "\n") }
	w("# The package manager of this system (apt, dnf, microdnf, yum, pacman,")
	w("# zypper, apk, brew), empty when none is known. Generated from")
	w("# internal/prereq: do not edit.")
	w(prefix + "pkg_manager() {")
	w(`    local f key value id="" like="" v m=""`)
	w(`    if [[ "${OSTYPE-}" == darwin* ]]; then`)
	w(`        printf '%s\n' brew`)
	w(`        return 0`)
	w(`    fi`)
	w(`    for f in "${GASH__SYSROOT-}/etc/os-release" "${GASH__SYSROOT-}/usr/lib/os-release"; do`)
	w(`        [[ -r "$f" ]] || continue`)
	w(`        while IFS='=' read -r key value; do`)
	w(`            value="${value#\"}"`)
	w(`            value="${value%\"}"`)
	w(`            value="${value#\'}"`)
	w(`            value="${value%\'}"`)
	w(`            case "$key" in`)
	w(`                ID) id="$value" ;;`)
	w(`                ID_LIKE) like="$value" ;;`)
	w(`            esac`)
	w(`        done <"$f"`)
	w(`        break`)
	w(`    done`)
	w(`    for v in $id $like; do`)
	w(`        case "$v" in`)
	for _, d := range distroIDs {
		w(`            ` + strings.Join(d.IDs, " | ") + `) m=` + string(d.Manager) + ` ;;`)
	}
	w(`        esac`)
	w(`        [[ -n "$m" ]] && break`)
	w(`    done`)
	w(`    if [[ "$m" == dnf ]] && ! type -P dnf >/dev/null 2>&1; then`)
	w(`        if type -P microdnf >/dev/null 2>&1; then`)
	w(`            m=microdnf`)
	w(`        elif type -P yum >/dev/null 2>&1; then`)
	w(`            m=yum`)
	w(`        fi`)
	w(`    fi`)
	w(`    printf '%s\n' "$m"`)
	w(`}`)
	w(``)
	w("# Usage: " + prefix + "install_hint [--run] TOOL...")
	w("# The command that installs TOOL... on this system (each package once),")
	w("# nothing when no package is known for any of them; --run runs it instead,")
	w("# after fetching the package lists where the manager needs it. Generated")
	w("# from internal/prereq: do not edit.")
	w(prefix + "install_hint() {")
	w(`    local m names t pkg pkgs="" run=0 IFS=$' \t\n'`)
	w(`    local -a cmd update`)
	w(`    if [[ "${1-}" == --run ]]; then`)
	w(`        run=1`)
	w(`        shift`)
	w(`    fi`)
	w(`    m="$(` + prefix + `pkg_manager)"`)
	w(`    names="$m"`)
	w(`    [[ "$m" == microdnf || "$m" == yum ]] && names=dnf`)
	w(`    for t in "$@"; do`)
	w(`        pkg=""`)
	w(`        case "$names:$t" in`)
	var cases []string
	for _, name := range BashTools {
		t, _ := Lookup(name)
		for _, m := range Managers {
			pkg := t.Packages[m]
			if pkg == "" || (m == Brew && contains(t.Builtin, "darwin")) {
				continue
			}
			cases = append(cases, `            `+string(m)+`:`+name+`) pkg='`+pkg+`' ;;`)
		}
	}
	sort.Strings(cases)
	for _, c := range cases {
		w(c)
	}
	w(`        esac`)
	w(`        [[ -z "$pkg" || " $pkgs " == *" $pkg "* ]] || pkgs="$pkgs $pkg"`)
	w(`    done`)
	w(`    [[ -n "$pkgs" ]] || return 0`)
	w(`    case "$m" in`)
	w(`        apt) cmd=(apt-get install -y) ;;`)
	w(`        dnf | microdnf | yum) cmd=("$m" install -y) ;;`)
	w(`        pacman) cmd=(pacman -S --needed --noconfirm) ;;`)
	w(`        zypper) cmd=(zypper --non-interactive install) ;;`)
	w(`        apk) cmd=(apk add) ;;`)
	w(`        brew)`)
	w(`            type -P brew >/dev/null 2>&1 || return 0`)
	w(`            cmd=(brew install)`)
	w(`            ;;`)
	w(`        *) return 0 ;;`)
	w(`    esac`)
	w(`    # The package lists: apt and apk always, pacman when it has none.`)
	w(`    case "$m" in`)
	w(`        apt) update=(apt-get update) ;;`)
	w(`        apk) update=(apk update) ;;`)
	w(`        pacman) [[ -n "$(ls -A "${GASH__SYSROOT-}/var/lib/pacman/sync" 2>/dev/null)" ]] || update=(pacman -Sy) ;;`)
	w(`    esac`)
	w(`    if [[ "$m" != brew && "${EUID-}" != 0 ]] && type -P sudo >/dev/null 2>&1; then`)
	w(`        cmd=(sudo "${cmd[@]}")`)
	w(`        [[ ${#update[@]} -eq 0 ]] || update=(sudo "${update[@]}")`)
	w(`    fi`)
	w(`    # shellcheck disable=SC2206 # one word per package`)
	w(`    cmd+=($pkgs)`)
	w(`    if [[ "$run" == 0 ]]; then`)
	w(`        printf '%s\n' "${cmd[*]}"`)
	w(`        return 0`)
	w(`    fi`)
	w(`    if [[ ${#update[@]} -gt 0 ]]; then`)
	w(`        "${update[@]}" || return`)
	w(`    fi`)
	w(`    "${cmd[@]}"`)
	w(`}`)
	return reindent(b.String(), unit, indent)
}

// reindent rewrites the four-space levels of code as unit spaces each,
// then puts indent before every line that is not empty.
func reindent(code string, unit int, indent string) string {
	lines := strings.Split(strings.TrimSuffix(code, "\n"), "\n")
	for i, l := range lines {
		if l == "" {
			continue
		}
		body := strings.TrimLeft(l, " ")
		lead := (len(l) - len(body)) / 4 * unit
		lines[i] = indent + strings.Repeat(" ", lead) + body
	}
	return strings.Join(lines, "\n") + "\n"
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// The lines around the generated block of install.sh.
const (
	installerBegin = "  # >>> generated by internal/prereq (go test ./internal/prereq -update) >>>\n"
	installerEnd   = "  # <<< generated by internal/prereq <<<\n"
)

// Installer returns the installer src with the block between its markers
// replaced by the current gashinst_pkg_manager and gashinst_install_hint
// (two spaces per level, inside the installer's braces).
func Installer(src string) (string, error) {
	i := strings.Index(src, installerBegin)
	j := strings.Index(src, installerEnd)
	if i < 0 || j < i {
		return "", errors.New("install.sh: no generated block (" + strings.TrimSpace(installerBegin) + ")")
	}
	return src[:i+len(installerBegin)] + BashFunctions("gashinst_", 2, "  ") + src[j:], nil
}
