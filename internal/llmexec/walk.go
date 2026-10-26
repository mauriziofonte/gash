// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import "strings"

// walkCfg is the per-command walk configuration of __llm_exec_check_walk.
type walkCfg struct {
	mode        string
	walk        bool
	implicit    bool
	pattern     bool
	vals        map[string]bool
	shortvals   string
	recShort    string
	followShort string
}

func field(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

// walkConfigFor returns the walk configuration for a command, or ok=false when
// the command does not walk a tree.
func walkConfigFor(name string) (walkCfg, bool) {
	c := walkCfg{mode: "names", implicit: true}
	switch name {
	case "grep", "egrep", "fgrep":
		c.mode, c.pattern, c.recShort, c.followShort, c.shortvals = "content", true, "rR", "R", "efmABCdD"
		c.vals = field("-e -f -m -A -B -C -d -D --regexp --file --max-count --after-context --before-context --context --include --exclude --exclude-dir --exclude-from --label --binary-files --devices --directories")
	case "rg":
		c.mode, c.walk, c.pattern, c.followShort, c.shortvals = "content", true, true, "L", "efgtTmABCjMrEd"
		c.vals = field("-e -f -g -t -T -m -A -B -C -j -M -r -E -d --regexp --file --glob --iglob --type --type-not --type-add --type-clear --max-count --after-context --before-context --context --threads --max-columns --replace --encoding --max-depth --max-filesize --sort --sortr --pre-glob --ignore-file --path-separator --colors --context-separator --field-match-separator --field-context-separator --engine --dfa-size-limit --regex-size-limit")
	case "fd":
		c.walk, c.pattern, c.followShort, c.shortvals = true, true, "L", "etEdSjc"
		c.vals = field("-e -t -E -d -S -j -c --extension --type --exclude --max-depth --min-depth --exact-depth --size --changed-within --changed-before --owner --max-results --threads --format --ignore-file --path-separator --search-path --base-directory")
	case "tree":
		c.walk, c.followShort, c.shortvals = true, "l", "LPIHT"
		c.vals = field("-L -P -I -H -T --filelimit --charset --timefmt --sort")
	case "du":
		c.walk, c.followShort, c.shortvals = true, "L", "dBtX"
		c.vals = field("-d -B -t -X --max-depth --block-size --threshold --exclude --exclude-from --time-style")
	case "ls":
		c.recShort, c.followShort, c.shortvals = "R", "L", "IwT"
		c.vals = field("-I -w -T --ignore --hide --width --tabsize --block-size --format --sort --time --time-style --indicator-style --quoting-style")
	case "diff":
		c.mode, c.recShort, c.implicit, c.shortvals = "content", "r", false, "xXIFLCUSWD"
		c.vals = field("-x -X -I -F -L -C -U -S -W -D --exclude --exclude-from --ignore-matching-lines --show-function-line --label --starting-file --width --ifdef --from-file --to-file")
	default:
		return c, false
	}
	return c, true
}

// checkWalk is __llm_exec_check_walk: recursive commands may not follow
// symlinks and may not start at / or above a protected tree.
func (lc *lineChecker) checkWalk(words []pword) error {
	name := words[0].lit
	if name == "find" {
		return lc.checkFind(words)
	}
	cfg, ok := walkConfigFor(name)
	if !ok {
		return nil
	}
	mode, pattern, follow := cfg.mode, cfg.pattern, false
	recursive := cfg.walk
	expect := ""
	dashdash := false
	var ops []int
	var extraOps []string

	for i := 1; i < len(words); i++ {
		w := words[i].lit
		if expect != "" {
			switch expect {
			case "--search-path", "--base-directory":
				extraOps = append(extraOps, w)
			case "-d", "--directories":
				if name != "rg" && name != "fd" && name != "du" && w == "recurse" {
					recursive = true
				}
			}
			expect = ""
			continue
		}
		if !dashdash && strings.HasPrefix(w, "-") && w != "-" {
			switch {
			case w == "--":
				dashdash = true
			case w == "--recursive" || w == "--directories=recurse":
				if name != "rg" && name != "fd" {
					recursive = true
				}
			case w == "--dereference-recursive":
				recursive, follow = true, true
			case w == "--follow" || w == "--dereference":
				follow = true
			case w == "--files":
				if name == "rg" {
					pattern, mode = false, "names"
				}
			case w == "--regexp" || w == "--file":
				pattern = false
				expect = w
			case strings.HasPrefix(w, "--regexp=") || strings.HasPrefix(w, "--file="):
				pattern = false
			case strings.HasPrefix(w, "--search-path=") || strings.HasPrefix(w, "--base-directory="):
				extraOps = append(extraOps, w[strings.IndexByte(w, '=')+1:])
			case strings.HasPrefix(w, "--files0-from"):
				return refuse("option_not_allowed", name+" "+w, "FATAL",
					"Walks whose starting points come from a file are refused; name the directories")
			case strings.Contains(w, "=") && strings.HasPrefix(w, "--"):
				// --*=*: ignored
			case strings.HasPrefix(w, "--"):
				if cfg.vals[w] {
					expect = w
				}
			default:
				for k := 1; k < len(w); k++ {
					c := w[k]
					if strings.IndexByte(cfg.recShort, c) >= 0 {
						recursive = true
					}
					if strings.IndexByte(cfg.followShort, c) >= 0 {
						follow = true
					}
					if strings.IndexByte(cfg.shortvals, c) >= 0 {
						if (c == 'e' || c == 'f') && name != "fd" {
							pattern = false
						}
						if k == len(w)-1 {
							expect = "-" + string(c)
						}
						break
					}
				}
			}
			continue
		}
		ops = append(ops, i)
	}

	if !recursive {
		return nil
	}
	if follow {
		return refuse("option_not_allowed", name+": symlink-following walk", "FATAL",
			"Recursive walks that follow symlinks are refused (a link can lead into a protected tree or to a secret file); drop the option (grep: -r instead of -R)")
	}
	if pattern && len(ops) > 0 {
		ops = ops[1:]
	}
	return lc.checkWalkRoots(words, mode, name, cfg.implicit, ops, extraOps)
}

// checkFind is __llm_exec_check_find: starting points are the words before the
// first expression word; -L, -follow and -files0-from are refused.
func (lc *lineChecker) checkFind(words []pword) error {
	inOps := true
	skip := false
	var ops []int
	for i := 1; i < len(words); i++ {
		w := words[i].lit
		if skip {
			skip = false
			continue
		}
		switch {
		case w == "-L" || w == "-follow" || w == "-files0-from":
			return refuse("option_not_allowed", "find "+w, "FATAL",
				"find walks that follow symlinks or read their starting points from a file are refused; drop "+w)
		case w == "-H" || w == "-P" || strings.HasPrefix(w, "-O"):
			continue
		case w == "-D":
			skip = true
			continue
		}
		if inOps && !strings.HasPrefix(w, "-") && w != "(" && w != ")" && w != "!" && w != "," {
			ops = append(ops, i)
		} else {
			inOps = false
		}
	}
	return lc.checkWalkRoots(words, "names", "find", true, ops, nil)
}

// checkWalkRoots is __llm_exec_check_walk_roots: refuse a walk root at / or
// above a protected tree. Operand indices are positions in words (globs
// expanded by what they match), extraOps are literal paths; with none of
// either and implicit set, the walk starts at cwd.
func (lc *lineChecker) checkWalkRoots(words []pword, mode, name string, implicit bool, opIdx []int, extraOps []string) error {
	var roots []string
	for _, idx := range opIdx {
		if words[idx].glob != "" {
			roots = append(roots, globExpand(words[idx].glob, lc.cwd)...)
		} else {
			roots = append(roots, words[idx].lit)
		}
	}
	roots = append(roots, extraOps...)
	if len(roots) == 0 && implicit {
		roots = []string{"."}
	}
	for _, p := range roots {
		if lc.guard.PathAboveForbidden(lc.execAbs(p, lc.cwd), mode) {
			return refuse("forbidden_path", name+" "+p, "FATAL",
				"A recursive command cannot start at / or above a protected tree (/proc, /sys, /dev, /boot, /root; for content searches also /etc); name a narrower directory")
		}
		// The user's PROTECT: trees: the program cannot leave them out.
		if lc.guard.AboveProtected(lc.execAbs(p, lc.cwd), mode) {
			return refuse("forbidden_path", name+" "+p, "FATAL",
				"A recursive command cannot start above a path the user protects in ~/.gash_env (PROTECT:); name a narrower directory, or use llm_tree, llm_find or llm_grep, which leave it out")
		}
	}
	return nil
}
