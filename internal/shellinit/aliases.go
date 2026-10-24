// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/registry"
)

// aliasPacks emits the 2.x alias packs (colors, navigation, safety, git,
// docker, php, general, in that order) that the binary evaluates for the
// shell. The probes 2.x ran in the shell (ls --color, grep color, rm GNU,
// dircolors) run in the binary with the same commands and interpretation;
// the 2.x probe cache is moot and neither read nor written. Module short
// aliases are defined by gash.sh (headless mode too); the shadow rule for
// them is moduleShadows, after the packs.
func (st *state) aliasPacks(b *strings.Builder) {
	st.timed(b, "lib/aliases/colors.sh", st.packColors)
	st.timed(b, "lib/aliases/navigation.sh", st.packNavigation)
	st.timed(b, "lib/aliases/safety.sh", st.packSafety)
	st.timed(b, "lib/aliases/git.sh", st.packGit)
	st.timed(b, "lib/aliases/docker.sh", st.packDocker)
	st.timed(b, "lib/aliases/php.sh", st.packPhp)
	st.timed(b, "lib/aliases/general.sh", st.packGeneral)
	st.moduleShadows(b)
}

// moduleShadows applies the shadow rule to the module short aliases gash.sh
// defined before this script runs: a name that is also a program on PATH
// is removed again, so the program runs (fake dcc on PATH: no dcc alias).
// An alias the user defined before Gash loaded is left alone, and no module
// alias replaces a program on purpose, so none is exempt. 3.0: SHELL-12
// fixed: 2.x took the module aliases for the user's own and kept them all.
func (st *state) moduleShadows(b *strings.Builder) {
	seen := map[string]bool{}
	for _, a := range registry.Aliases() {
		if a.Pack != "" || a.Function == "" || seen[a.Name] {
			continue
		}
		seen[a.Name] = true
		if !exempt(a.Name) && st.has(a.Name) {
			b.WriteString("[[ -v '" + UserAliasesVar + "[" + a.Name + "]' ]] || unalias " + a.Name + " 2>/dev/null || true\n")
		}
	}
}

// shadowExempt is the fixed list of 2.x _loader.sh: an alias of one of these
// names is defined even when it hides a program on PATH (ls wraps ls on
// purpose, and so on). sl is no longer on it: the alias catches a typo of
// ls, and a user who installed the sl program wants that program instead
// (3.0: SHELL-12 fixed).
var shadowExempt = map[string]bool{
	"ls": true, "dir": true, "vdir": true, "cp": true, "mv": true,
	"rm": true, "mkdir": true, "chown": true, "chmod": true, "chgrp": true,
	"grep": true, "bc": true, "reboot": true, "shutdown": true, "less": true,
	"tail": true, "df": true, "top": true, "traceroute": true, "tracepath": true,
	"diff": true, "php": true, "composer": true,
}

// exempt reports whether name is outside the shadow rule: the fixed list or
// the php/composer version patterns (php[0-9]*, composer[0-9]*,
// 1composer[0-9]*).
func exempt(name string) bool {
	if shadowExempt[name] {
		return true
	}
	digitAfter := func(prefix string) bool {
		return strings.HasPrefix(name, prefix) && len(name) > len(prefix) &&
			name[len(prefix)] >= '0' && name[len(prefix)] <= '9'
	}
	return digitAfter("php") || digitAfter("composer") || digitAfter("1composer")
}

// UserAliasesVar is the associative array gash.sh fills, before Gash
// defines anything, with the aliases the user had (compat/gash.sh.tmpl).
const UserAliasesVar = "__GASH_USER_ALIASES"

// aliasStmt is the alias definition for name=value: a name the user defined
// before Gash loaded is left alone ([[ -v '__GASH_USER_ALIASES[ll]' ]] ||
// alias ll='ls -l'), and under the shadow rule a non-exempt name that hides
// a program on PATH is not defined at all (""). 3.0: SHELL-09 fixed: 2.x
// overwrote an alias the user defined first.
func (st *state) aliasStmt(name, value string) string {
	if !exempt(name) && st.has(name) {
		return ""
	}
	return "[[ -v '" + UserAliasesVar + "[" + name + "]' ]] || alias " + name + "=" + quote(value)
}

// alias writes one pack alias with its shadow decision.
func (st *state) alias(b *strings.Builder, name, value string) {
	if stmt := st.aliasStmt(name, value); stmt != "" {
		b.WriteString(stmt + "\n")
	}
}

// -----------------------------------------------------------------------------
// colors
// -----------------------------------------------------------------------------

func (st *state) packColors(b *strings.Builder) {
	if st.e.goos == "darwin" {
		b.WriteString("export LSCOLORS='CxahafafBxagagabababab'\n")
	}
	if st.c.Env.Get("LESS") != "" {
		b.WriteString(lessTermcap)
	}
	// LS_COLORS from dircolors (honors ~/.dircolors, then /etc/DIR_COLORS),
	// only when unset. The binary runs dircolors; the emitted guard keeps a
	// value the shell already has.
	if _, ok := st.program("dircolors"); ok {
		var args []string
		switch {
		case st.readable(st.home + "/.dircolors"):
			args = []string{"-b", st.home + "/.dircolors"}
		case st.readable("/etc/DIR_COLORS"):
			args = []string{"-b", "/etc/DIR_COLORS"}
		default:
			args = []string{"-b"}
		}
		if out := st.runOut("dircolors", args...); out != "" {
			b.WriteString("if [[ -z \"${LS_COLORS-}\" ]]; then\n")
			// dircolors prints `LS_COLORS='...';` and `export LS_COLORS`;
			// indent each line and drop the trailing statement `;` so the
			// generated block is shfmt-clean. The value is single-quoted, so
			// neither change touches it (LS_COLORS is identical).
			for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if line == "" {
					b.WriteByte('\n')
				} else {
					b.WriteString("    " + strings.TrimSuffix(line, ";") + "\n")
				}
			}
			b.WriteString("fi\n")
		}
	}
	// grep: defined when `grep --color=auto -q x` finds the x it reads, so
	// a grep that rejects --color has no alias (3.0: SHELL-06 fixed; the
	// 2.x probe grepped an empty file, always exited 1 and never defined it).
	if st.runOKInput("x\n", "grep", "--color=auto", "-q", "x") {
		st.alias(b, "grep", "grep --color=auto")
	}
}

// lessTermcap is the seven LESS_TERMCAP_* exports of colors.sh, set when
// LESS is in the environment.
const lessTermcap = `export LESS_TERMCAP_mb=$'\E[01;31m'
export LESS_TERMCAP_md=$'\E[01;38;5;74m'
export LESS_TERMCAP_me=$'\033[0m'
export LESS_TERMCAP_se=$'\033[0m'
export LESS_TERMCAP_so=$'\E[38;5;246m'
export LESS_TERMCAP_ue=$'\033[0m'
export LESS_TERMCAP_us=$'\E[04;38;5;146m'
`

// -----------------------------------------------------------------------------
// navigation
// -----------------------------------------------------------------------------

func (st *state) packNavigation(b *strings.Builder) {
	flag := st.lsColorFlag()
	if flag != "" {
		suffix := " " + flag
		st.alias(b, "ls", "ls"+suffix)
		st.alias(b, "ll", "ls -l"+suffix)
		st.alias(b, "la", "ls -la"+suffix)
		st.alias(b, "l", "ls -CF"+suffix)
		st.alias(b, "lash", "ls -lash"+suffix)
		st.alias(b, "sl", "ls"+suffix)
		if st.has("dir") {
			st.alias(b, "dir", "dir"+suffix)
		}
		if st.has("vdir") {
			st.alias(b, "vdir", "vdir"+suffix)
		}
	} else {
		st.alias(b, "ll", "ls -l")
		st.alias(b, "la", "ls -la")
		st.alias(b, "l", "ls -CF")
		st.alias(b, "lash", "ls -lash")
		st.alias(b, "sl", "ls")
	}
	st.alias(b, "..", "cd ..")
	st.alias(b, "...", "cd ../..")
	st.alias(b, "cd..", "cd ..")
	st.alias(b, "....", "cd ../../..")
	st.alias(b, ".....", "cd ../../../..")
	st.alias(b, ".4", "cd ../../../..")
	st.alias(b, ".5", "cd ../../../../..")
	st.alias(b, "cls", "clear")
	st.alias(b, "path", `tr ":" "\n" <<<"$PATH"`)
}

// lsColorFlag is navigation.sh's probe: --color=auto when ls accepts it,
// else -G on macOS, else none.
func (st *state) lsColorFlag() string {
	if st.runOK("ls", "--color=auto", ".") {
		return "--color=auto"
	}
	if st.e.goos == "darwin" {
		return "-G"
	}
	return ""
}

// -----------------------------------------------------------------------------
// safety
// -----------------------------------------------------------------------------

func (st *state) packSafety(b *strings.Builder) {
	if st.s.Get("GASH_SAFETY_ALIASES") == "1" {
		st.alias(b, "cp", "cp -iv")
		st.alias(b, "mv", "mv -iv")
		st.alias(b, "mkdir", "mkdir -pv")
		if strings.Contains(st.runOut("rm", "--version"), "GNU") {
			st.alias(b, "rm", "rm -Iv --one-file-system --preserve-root")
			st.alias(b, "chown", "chown --preserve-root")
			st.alias(b, "chmod", "chmod --preserve-root")
			st.alias(b, "chgrp", "chgrp --preserve-root")
		} else {
			st.alias(b, "rm", "rm -iv")
		}
	}
	st.alias(b, "bc", "bc -l")
	if st.has("ss") {
		st.alias(b, "ports", "ss -tulanp")
	} else if st.has("netstat") {
		st.alias(b, "ports", "netstat -an")
	}
	st.alias(b, "reboot", "sudo /sbin/reboot")
	st.alias(b, "shutdown", "sudo /sbin/shutdown")
}

// -----------------------------------------------------------------------------
// git
// -----------------------------------------------------------------------------

func (st *state) packGit(b *strings.Builder) {
	if !st.has("git") {
		return
	}
	// The gl family: Bash functions that call the binary (the output is a
	// git log); their handlers live in internal/gitx.
	for _, fn := range []string{"gl", "gla", "glo", "glg", "gls", "glf"} {
		b.WriteString(fn + "() { __gash_call " + fn + " \"$@\"; }\n")
	}
	st.alias(b, "glog", "gl")
	st.alias(b, "gst", "git status")
	if !st.has("gs") {
		st.alias(b, "gs", "git status -sb")
	}
	for _, kv := range [][2]string{
		{"ga", "git add"}, {"gaa", "git add --all"}, {"gap", "git add --patch"},
		{"gc", "git commit"}, {"gcm", "git commit -m"}, {"gca", "git commit --amend"},
		{"gcan", "git commit --amend --no-edit"},
		{"gp", "git pull"}, {"gpr", "git pull --rebase"}, {"gpsh", "git push"},
		{"gpf", "git push --force-with-lease"},
		{"gco", "git checkout"}, {"gcb", "git checkout -b"}, {"gsw", "git switch"},
		{"gswc", "git switch -c"},
		{"gb", "git branch"}, {"gba", "git branch -a"}, {"gbd", "git branch -d"},
		{"gbD", "git branch -D"},
		{"gd", "git diff"}, {"gds", "git diff --staged"}, {"gdw", "git diff --word-diff"},
		{"gsh", "git stash"}, {"gshp", "git stash pop"}, {"gshl", "git stash list"},
		{"gsha", "git stash apply"},
		{"gf", "git fetch"}, {"gfa", "git fetch --all --prune"}, {"gr", "git remote -v"},
		{"grh", "git reset HEAD"}, {"grh1", "git reset HEAD~1"}, {"grhh", "git reset --hard HEAD"},
		{"grb", "git rebase"}, {"grbc", "git rebase --continue"}, {"grba", "git rebase --abort"},
	} {
		st.alias(b, kv[0], kv[1])
	}
}

// -----------------------------------------------------------------------------
// docker
// -----------------------------------------------------------------------------

func (st *state) packDocker(b *strings.Builder) {
	if st.has("docker") {
		for _, kv := range [][2]string{
			{"dcls", "docker container ls -a"}, {"dclsr", "docker container ls"},
			{"dils", "docker image ls"}, {"dirm", "docker image prune -a"},
			{"dstop", "docker stop"}, {"dstart", "docker start"}, {"dexec", "docker exec -it"},
			{"dcrm", "docker container rm"}, {"drm", "docker rm"}, {"drmi", "docker rmi"},
			{"dlogs", "docker logs -f"}, {"dinspect", "docker inspect"}, {"dnetls", "docker network ls"},
		} {
			st.alias(b, kv[0], kv[1])
		}
		st.aliasIfFunc(b, "dstopall", "docker_stop_all")
		st.aliasIfFunc(b, "dstartall", "docker_start_all")
		st.aliasIfFunc(b, "dpruneall", "docker_prune_all")
	}
	if st.has("docker-compose") {
		for _, kv := range [][2]string{
			{"dc", "docker-compose"}, {"dcup", "docker-compose up -d"},
			{"dcdown", "docker-compose down"}, {"dclogs", "docker-compose logs -f"},
			{"dcb", "docker-compose build"}, {"dcrestart", "docker-compose restart"},
			{"dcps", "docker-compose ps"}, {"dcpull", "docker-compose pull"},
		} {
			st.alias(b, kv[0], kv[1])
		}
	}
	// These depend on a module function, not on docker on PATH.
	st.aliasIfFunc(b, "dcc", "docker_compose_check")
	st.aliasIfFunc(b, "dcup2", "docker_compose_upgrade")
	st.aliasIfFunc(b, "dcscan", "docker_compose_scan")
}

// aliasIfFunc emits a function-backed pack alias guarded by the shell test
// 2.x used (declare -F FUNC), combined with the shadow rule.
func (st *state) aliasIfFunc(b *strings.Builder, name, fn string) {
	if stmt := st.aliasStmt(name, fn); stmt != "" {
		b.WriteString("declare -F " + fn + " >/dev/null 2>&1 && { " + stmt + "; }\n")
	}
}

// -----------------------------------------------------------------------------
// php
// -----------------------------------------------------------------------------

func (st *state) packPhp(b *strings.Builder) {
	if !st.has("php") {
		return
	}
	const opts = "-d allow_url_fopen=1 -d memory_limit=2048M"
	composer, _ := st.program("composer")
	composer1, _ := st.program("composer1")
	phpLatest := ""
	for _, v := range []string{"8.5", "8.4", "8.3", "8.2", "8.1", "8.0", "7.4", "7.3", "7.2", "7.1", "7.0", "5.6"} {
		bin, ok := st.program("php" + v)
		if !ok {
			continue
		}
		if phpLatest == "" {
			phpLatest = bin
		}
		short := strings.ReplaceAll(v, ".", "")
		st.alias(b, "php"+short, bin+" "+opts)
		if composer != "" {
			st.alias(b, "composer"+short, bin+" "+opts+" "+composer)
		}
		switch v {
		case "7.2", "7.1", "7.0", "5.6":
			if composer1 != "" {
				st.alias(b, "1composer"+short, bin+" "+opts+" "+composer1)
			}
		}
	}
	if st.s.Get("GASH_REPLACE_TOOLS") == "1" && phpLatest != "" {
		st.alias(b, "php", phpLatest+" "+opts)
		if composer != "" {
			st.alias(b, "composer", phpLatest+" "+opts+" "+composer)
		}
	}
	if composer != "" {
		st.alias(b, "composer-packages-update", composer+" global update")
		st.alias(b, "composer-self-update", "sudo "+composer+" self-update")
	}
	if composer1 != "" {
		st.alias(b, "composer1-self-update", "sudo "+composer1+" self-update")
	}
	// The legacy hte, hte-create, hte-remove, hte-details aliases are dropped
	// (DEV-17): `gash hte` replaces them.
}

// -----------------------------------------------------------------------------
// general
// -----------------------------------------------------------------------------

func (st *state) packGeneral(b *strings.Builder) {
	b.WriteString(helpFunc)
	st.alias(b, "quit", "services_stop")
	if st.s.Get("GASH_REPLACE_TOOLS") == "1" {
		if st.has("most") {
			st.alias(b, "less", "most")
			b.WriteString("export PAGER='most'\n")
		}
		st.replaceAlias(b, "tail", "multitail")
		st.replaceAlias(b, "df", "pydf")
		st.replaceAlias(b, "top", "htop")
		st.replaceAlias(b, "traceroute", "mtr")
		st.replaceAlias(b, "tracepath", "mtr")
		st.replaceAlias(b, "diff", "colordiff")
	}
}

// replaceAlias is __gash_add_command_replace_alias: alias name=prog when prog
// is a program on PATH.
func (st *state) replaceAlias(b *strings.Builder, name, prog string) {
	if st.has(prog) {
		st.alias(b, name, prog)
	}
}

// helpFunc is general.sh's help wrapper: gash_help for Gash commands, the
// Bash builtin for options, builtins and keywords.
const helpFunc = `help() {
    if [[ $# -eq 0 ]]; then
        gash_help
    elif [[ "$1" == -* ]] || [[ "$(type -t "$1" 2>/dev/null)" == "builtin" || "$(type -t "$1" 2>/dev/null)" == "keyword" ]]; then
        builtin help "$@"
    else
        gash_help "$@"
    fi
}
`
