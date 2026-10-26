// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/pathguard"
)

var (
	nameRE    = lazyre.MustCompile(`^[A-Za-z0-9._+-]+$`)
	psPrevRE  = lazyre.MustCompile(`^(-[oOpqugtCGUsk]|--(pid|ppid|sid|user|User|group|Group|tty|format|sort|cols|columns|rows|width))$`)
	allAlpha  = lazyre.MustCompile(`^[A-Za-z]+$`)
	yqLoadRE  = lazyre.MustCompile(`load(_[A-Za-z0-9]+)?[[:space:]]*\(`)
	verRE     = lazyre.MustCompile(`^(--version|-v|-V|version)$`)
	ipCmdRE   = lazyre.MustCompile(`^(a|addr|address|r|route|l|link|n|neigh|neighbour|rule|maddr)$`)
	ipSubRE   = lazyre.MustCompile(`^(show|list|ls|get)$`)
	sysctlRE  = lazyre.MustCompile(`^(status|show|cat|is-active|is-enabled|is-failed|list-units|list-unit-files|list-timers|list-sockets|list-dependencies)$`)
	dockSubRE = lazyre.MustCompile(`^(image|container|volume|network|system|context|compose)$`)
	dockSub2  = lazyre.MustCompile(`^(ls|list|ps|inspect|history|logs|top|port|diff|df|info|show|config|images|version)$`)
	dockTopRE = lazyre.MustCompile(`^(ps|images|logs|inspect|version|info|stats|top|port|diff|history)$`)
)

// patternCmds are commands whose first operand is a pattern/script/filter,
// not a file, unless -e/-f supply it (core.sh usage in check_simple).
var patternCmds = map[string]bool{
	"grep": true, "egrep": true, "fgrep": true, "zgrep": true, "rg": true,
	"sed": true, "awk": true, "gawk": true, "jq": true, "yq": true,
}

var literalArgCmds = map[string]bool{
	"echo": true, "printf": true, "basename": true, "dirname": true,
}

// checkSimple is __llm_exec_check_simple: the command-name gate, the allowlist
// (with per-command and walk rules, skipped for GASH_LLM_EXEC_ALLOW commands,
// EXEC-05), and the per-argument secret, glob and path checks. It returns,
// for each parsed word in order, the validated words it stands for (one, or
// a glob's matches), which the D-1 A script renders single-quoted.
func (lc *lineChecker) checkSimple(words []pword) ([][]string, error) {
	name := words[0].lit
	if words[0].globby || !nameRE.MatchString(name) {
		if strings.Contains(name, "=") {
			return nil, refuse("construct_not_allowed", name[:strings.IndexByte(name, '=')]+"=...", "RETRY",
				"Variable assignments are not allowed; pass values as arguments")
		}
		return nil, refuse("command_not_allowed", name, "STOP",
			"Run commands by plain name; paths, globs and keywords are not allowed")
	}

	if !lc.extra[name] {
		if !allowedSet[name] {
			if writeCommands[name] {
				// 3.4: never suggest opening the read-only runner to a
				// command that changes files or processes.
				return nil, refuse("command_not_allowed", name, "STOP",
					"llm_exec runs read-only commands, and '"+name+"' changes files or processes. Make the change with an agent_* function when the user asked for it (agent_put, agent_replace, agent_kill), or ask the user to run it")
			}
			return nil, refuse("command_not_allowed", name, "STOP",
				"llm_exec runs read-only commands (ls, cat, grep, find, git status, ...). Ask the user to add '"+name+"' to GASH_LLM_EXEC_ALLOW in ~/.gash_env if it is needed")
		}
		if err := lc.checkArgs(words); err != nil {
			return nil, err
		}
	}
	// The walk rules (no symlink-following walk, no walk root at / or
	// above a protected tree) hold for commands in GASH_LLM_EXEC_ALLOW too;
	// only the per-command option rules are lifted (LEDGER EXEC-05).
	if err := lc.checkWalk(words); err != nil {
		return nil, err
	}

	plain := []string{name}
	perWord := [][]string{{name}}

	cwd := lc.cwd
	plainArg := cwd != lc.env.Cwd
	skipFirst := false
	if patternCmds[name] {
		skipFirst = true
		for _, w := range words[1:] {
			if isPatternOptWord(w.lit) {
				skipFirst = false
			}
		}
	}
	seenPos := false

	for i := 1; i < len(words); i++ {
		w := words[i].lit
		var renderedForWord []string

		if words[i].globby {
			matches := globExpand(words[i].glob, cwd)
			for _, m := range matches {
				if strings.HasPrefix(m, "-") {
					return nil, refuse("glob_matches_option", m, "RETRY",
						"A file name matched by the glob starts with '-'; prefix the pattern with ./")
				}
				if lc.guard.IsSecretFile(m) {
					return nil, refuse("secret_file_blocked", m, "FATAL",
						"Secret files cannot be read through llm_exec")
				}
				if err := lc.checkPath(m, cwd, true); err != nil {
					return nil, err
				}
			}
			if len(matches) > 0 {
				renderedForWord = append(renderedForWord, matches...)
			} else {
				renderedForWord = []string{lc.tildeExpand(w, words[i].uq)}
			}
		} else {
			renderedForWord = []string{lc.tildeExpand(w, words[i].uq)}
		}

		// Word-level secret check: the word, a "key=value" value, and a git
		// REV:PATH (EXEC-04 surfaces here; an attached option value is checked below,
		// EXEC-01).
		secret := lc.guard.IsSecretFile(w)
		if !secret && strings.Contains(w, "=") {
			secret = lc.guard.IsSecretFile(w[strings.IndexByte(w, '=')+1:])
		}
		if !secret && name == "git" && strings.Contains(w, ":") {
			secret = pathguard.SecretName(w[strings.LastIndexByte(w, ':')+1:])
		}
		if secret {
			return nil, refuse("secret_file_blocked", w, "FATAL",
				"Secret files cannot be read through llm_exec")
		}

		if !strings.HasPrefix(w, "-") && skipFirst && !seenPos {
			seenPos = true
			plain = append(plain, renderedForWord...)
			perWord = append(perWord, renderedForWord)
			continue
		}
		if literalArgCmds[name] {
			plain = append(plain, renderedForWord...)
			perWord = append(perWord, renderedForWord)
			continue
		}

		switch {
		case strings.HasPrefix(w, "--") && strings.Contains(w, "="):
			if err := lc.checkPath(w[strings.IndexByte(w, '=')+1:], cwd, plainArg); err != nil {
				return nil, err
			}
		case strings.HasPrefix(w, "--"):
			// no path check
		case strings.HasPrefix(w, "-") && len(w) >= 2:
			// A short option with an attached value, alone or at the end
			// of a cluster (-fFILE, -uvfFILE): 2.x stripped exactly two
			// characters and ran the forbidden-path check only. Every
			// split of the cluster is checked now, the secret-file check
			// and the path check, so the value is examined whichever
			// letter takes it (3.0: LEDGER EXEC-01 fixed).
			if err := lc.checkAttached(w, cwd); err != nil {
				return nil, err
			}
		case strings.Contains(w, "="):
			if err := lc.checkPath(w, cwd, plainArg); err != nil {
				return nil, err
			}
			if err := lc.checkPath(w[strings.IndexByte(w, '=')+1:], cwd, plainArg); err != nil {
				return nil, err
			}
		default:
			if err := lc.checkPath(w, cwd, plainArg); err != nil {
				return nil, err
			}
		}
		plain = append(plain, renderedForWord...)
		perWord = append(perWord, renderedForWord)
	}

	lc.cmdWords = append(lc.cmdWords, plain)
	lc.cmdRedirs = append(lc.cmdRedirs, append([]string(nil), lc.redirs...))
	return perWord, nil
}

func isPatternOptWord(w string) bool {
	return strings.HasPrefix(w, "-e") || strings.HasPrefix(w, "-f") ||
		strings.HasPrefix(w, "--regexp") || strings.HasPrefix(w, "--file") ||
		strings.HasPrefix(w, "--expression") || strings.HasPrefix(w, "--from-file")
}

// positionals returns the non-option words of list, mirroring
// __llm_exec_positionals (which drops the command name before scanning).
func positionals(list []string) []string {
	var out []string
	for _, w := range list {
		if !strings.HasPrefix(w, "-") {
			out = append(out, w)
		}
	}
	return out
}

// shortCluster reports whether w matches the bash regex ^-[^-]*<ch>: a short
// option or cluster that contains ch before any second dash.
func shortCluster(w string, ch byte) bool {
	if !strings.HasPrefix(w, "-") {
		return false
	}
	for k := 1; k < len(w); k++ {
		if w[k] == '-' {
			return false
		}
		if w[k] == ch {
			return true
		}
	}
	return false
}

// checkAttached checks the values a short-option word can carry: for
// -abcVALUE any of bcVALUE, cVALUE, VALUE may be the value of the letter
// before it, so each is checked as a path and as a secret file (the path
// check first, so the 2.x refusal of -XPATH keeps its details). The
// per-command tables are not needed to find the value, and a command they
// do not list is covered as well.
func (lc *lineChecker) checkAttached(w, cwd string) error {
	for k := 2; k < len(w); k++ {
		v := w[k:]
		if err := lc.checkPath(v, cwd, false); err != nil {
			return err
		}
		if lc.guard.IsSecretFile(v) {
			return refuse("secret_file_blocked", v, "FATAL",
				"Secret files cannot be read through llm_exec")
		}
	}
	return nil
}

// checkArgs is __llm_exec_check_args: the per-command writing-option rules.
func (lc *lineChecker) checkArgs(words []pword) error {
	name := words[0].lit
	args := make([]string, 0, len(words)-1)
	for _, w := range words[1:] {
		args = append(args, w.lit)
	}

	switch name {
	case "find":
		for _, w := range args {
			switch w {
			case "-exec", "-execdir", "-ok", "-okdir", "-delete", "-fprint", "-fprint0", "-fprintf", "-fls":
				return refuseOption("find", w)
			}
		}
	case "fd":
		for _, w := range args {
			if w == "-x" || w == "-X" || w == "--exec" || strings.HasPrefix(w, "--exec=") ||
				w == "--exec-batch" || strings.HasPrefix(w, "--exec-batch=") {
				return refuseOption("fd", w)
			}
		}
	case "rg":
		for _, w := range args {
			if w == "--pre" || strings.HasPrefix(w, "--pre=") {
				return refuseOption("rg", w)
			}
		}
	case "sort", "tree":
		for _, w := range args {
			if strings.HasPrefix(w, "--output") || strings.HasPrefix(w, "--compress-program") || shortCluster(w, 'o') {
				return refuseOption(name, w)
			}
		}
	case "sed", "yq":
		for _, w := range args {
			if strings.HasPrefix(w, "--in-place") || strings.HasPrefix(w, "--inplace") || shortCluster(w, 'i') {
				return refuseOption(name, w)
			}
			if name == "yq" {
				if strings.HasPrefix(w, "--split-exp") || shortCluster(w, 's') {
					return refuseOption("yq", w)
				}
				if yqLoadRE.MatchString(w) {
					return refuse("option_not_allowed", "yq "+w, "FATAL",
						"yq load functions read files the path checks do not see; pass the files as arguments")
				}
			}
		}
		if name == "sed" && !lc.pr.HasSandboxedSed() {
			return refuse("command_not_allowed", "sed", "STOP",
				"llm_exec allows sed only where GNU sed --sandbox exists; use grep, head or llm_grep")
		}
	case "awk", "gawk":
		if !lc.pr.HasSandboxedGawk() {
			return refuse("command_not_allowed", name, "STOP",
				"llm_exec allows awk only where gawk --sandbox exists; use grep, cut or jq")
		}
	case "file":
		for _, w := range args {
			if w == "--compile" || shortCluster(w, 'C') {
				return refuseOption("file", w)
			}
		}
	case "uniq":
		prev := ""
		var pos []string
		for _, w := range args {
			if !strings.HasPrefix(w, "-") && prev != "-f" && prev != "-s" && prev != "-w" {
				pos = append(pos, w)
			}
			prev = w
		}
		if len(pos) > 1 {
			return refuseOption("uniq", pos[1])
		}
	case "hostname":
		for _, w := range args {
			if !strings.HasPrefix(w, "-") || w == "-F" || strings.HasPrefix(w, "--file") || w == "-b" || w == "--boot" {
				return refuseOption("hostname", w)
			}
		}
	case "date":
		for _, w := range args {
			if strings.HasPrefix(w, "--set") || strings.HasPrefix(w, "-s") {
				return refuseOption("date", w)
			}
		}
	case "crontab":
		if !(len(args) == 1 && args[0] == "-l") {
			return refuseOption("crontab", strings.Join(args, " "))
		}
	case "ps":
		prev := ""
		for _, w := range args {
			if allAlpha.MatchString(w) && strings.Contains(w, "e") && !psPrevRE.MatchString(prev) {
				return refuse("option_not_allowed", "ps "+w, "FATAL",
					"ps e prints the environment of every process; list processes without it (ps aux, ps -ef)")
			}
			prev = w
		}
	case "getent":
		pos := positionals(args)
		if len(pos) > 0 && (pos[0] == "shadow" || pos[0] == "gshadow") {
			return refuse("forbidden_path", "getent "+pos[0], "FATAL",
				"The shadow databases (/etc/shadow, /etc/gshadow) are protected")
		}
	case "php", "node", "python", "python3", "ruby", "java", "composer":
		if !(len(args) == 1 && verRE.MatchString(args[0])) {
			return refuse("option_not_allowed", name+" "+strings.Join(args, " "), "FATAL",
				"llm_exec runs "+name+" only to print its version (--version)")
		}
	case "go", "npm", "pip", "pip3":
		ok := map[string]string{
			"go":   " version env ",
			"npm":  " --version -v ls list ",
			"pip":  " --version -V list show freeze ",
			"pip3": " --version -V list show freeze ",
		}
		if !(len(args) >= 1 && strings.Contains(ok[name], " "+args[0]+" ")) {
			return refuse("option_not_allowed", name+" "+strings.Join(args, " "), "FATAL",
				"llm_exec allows only: "+name+ok[name])
		}
	case "ip":
		pos := positionals(args)
		p0, p1 := "", ""
		if len(pos) > 0 {
			p0 = pos[0]
		}
		if len(pos) > 1 {
			p1 = pos[1]
		}
		if !ipCmdRE.MatchString(p0) || (p1 != "" && !ipSubRE.MatchString(p1)) {
			return refuseOption("ip", strings.Join(pos, " "))
		}
	case "systemctl":
		pos := positionals(args)
		if len(pos) > 0 && pos[0] != "" && !sysctlRE.MatchString(pos[0]) {
			return refuseOption("systemctl", pos[0])
		}
	case "journalctl":
		for _, w := range args {
			switch {
			case strings.HasPrefix(w, "--vacuum"), w == "--rotate", w == "--flush", w == "--sync",
				w == "--relinquish-var", w == "--smart-relinquish-var", w == "--setup-keys", w == "--update-catalog":
				return refuseOption("journalctl", w)
			}
		}
	case "docker":
		sub, sub2 := "", ""
		if len(args) > 0 {
			sub = args[0]
		}
		if len(args) > 1 {
			sub2 = args[1]
		}
		if dockSubRE.MatchString(sub) {
			if !dockSub2.MatchString(sub2) {
				return refuseOption("docker "+sub, sub2)
			}
		} else if !dockTopRE.MatchString(sub) {
			return refuseOption("docker", sub)
		}
	case "git":
		return lc.checkGit(args)
	case "diff":
		for _, w := range args {
			if w == "--" {
				break
			}
			if (w == "--recursive" || shortCluster(w, 'r')) && !lc.pr.HasDiffNoDeref() {
				return refuse("option_not_allowed", "diff "+w, "FATAL",
					"Recursive diff needs GNU diff (--no-dereference) here: it would follow symlinks out of the tree; compare single files")
			}
		}
	}
	return nil
}

// writeCommands change files, permissions or processes: the refusal of one
// points at the agent_* functions, not at the allowlist.
var writeCommands = map[string]bool{
	"rm": true, "rmdir": true, "mv": true, "cp": true, "ln": true, "mkdir": true, "touch": true,
	"chmod": true, "chown": true, "chgrp": true, "dd": true, "truncate": true, "shred": true,
	"install": true, "tee": true, "kill": true, "pkill": true, "killall": true,
	"sudo": true, "su": true, "reboot": true, "shutdown": true,
}
