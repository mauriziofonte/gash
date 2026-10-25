// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llmexec

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// gitSubcommands is the read-only subcommand allowlist of __llm_exec_check_git.
var gitSubcommands = map[string]bool{}

func init() {
	for _, s := range strings.Fields("status log diff show blame shortlog describe grep ls-files ls-tree rev-parse rev-list cat-file show-ref for-each-ref merge-base name-rev count-objects whatchanged version reflog branch tag remote config stash worktree diff-tree diff-index diff-files check-ignore check-attr") {
		gitSubcommands[s] = true
	}
}

var (
	gitRemoteRE = lazyre.MustCompile(`^(show|get-url)$`)
	gitStashRE  = lazyre.MustCompile(`^(list|show)$`)
	gitReflogRE = lazyre.MustCompile(`^(expire|delete)$`)
)

// checkGit is __llm_exec_check_git: read-only subcommands, no config
// overrides, no output files. args excludes the leading "git".
func (lc *lineChecker) checkGit(args []string) error {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		switch {
		case args[i] == "-C":
			i++ // consume the directory value
		case args[i] == "--no-pager" || args[i] == "-P" ||
			strings.HasPrefix(args[i], "--git-dir=") || strings.HasPrefix(args[i], "--work-tree=") ||
			args[i] == "--no-optional-locks":
			// allowed; not --literal-pathspecs (the secret exclusions are pathspec magic)
		default:
			return refuseOption("git", args[i])
		}
		i++
	}
	sub := ""
	if i < len(args) {
		sub = args[i]
		i++
	}
	rest := args[i:]

	if !gitSubcommands[sub] {
		s := sub
		if s == "" {
			s = "(no subcommand)"
		}
		return refuseOption("git", s)
	}

	for _, w := range rest {
		switch {
		case strings.HasPrefix(w, "--output"), strings.HasPrefix(w, "-O"),
			strings.HasPrefix(w, "--open-files-in-pager"), w == "--ext-diff", w == "--textconv",
			w == "--filters", w == "--no-index", w == "--batch", strings.HasPrefix(w, "--batch="),
			strings.HasPrefix(w, "--batch-command"):
			return refuseOption("git "+sub, w)
		}
	}

	pos := positionals(rest)
	p0 := ""
	if len(pos) > 0 {
		p0 = pos[0]
	}
	joined := " " + strings.Join(rest, " ") + " "

	switch sub {
	case "branch", "tag":
		for _, w := range rest {
			if isGitBranchTagWrite(w) {
				if sub == "branch" && w == "-a" {
					continue // -a means --all for branch
				}
				return refuseOption("git "+sub, w)
			}
		}
		if len(pos) > 0 && !strings.Contains(joined, " -l ") && !strings.Contains(joined, " --list ") {
			return refuse("option_not_allowed", "git "+sub+" "+pos[0], "FATAL",
				"Naming a "+sub+" creates it; to filter the list use: git "+sub+" --list PATTERN")
		}
	case "remote":
		if !(p0 == "" || gitRemoteRE.MatchString(p0)) {
			return refuseOption("git remote", p0)
		}
		if p0 == "show" && !strings.Contains(joined, " -n ") {
			return refuse("option_not_allowed", "git remote show", "FATAL",
				"git remote show contacts the network; use git remote show -n NAME or git remote -v")
		}
	case "config":
		hasRead := p0 == "get" || p0 == "list"
		for _, w := range rest {
			switch {
			case w == "--add", strings.HasPrefix(w, "--unset"), w == "--replace-all",
				w == "--rename-section", w == "--remove-section", w == "-e", w == "--edit",
				w == "set", w == "unset", w == "rename-section", w == "remove-section", w == "edit":
				return refuseOption("git config", w)
			case strings.HasPrefix(w, "--get"), w == "--list", w == "-l":
				hasRead = true
			}
		}
		if !hasRead {
			return refuse("option_not_allowed", "git config "+strings.Join(rest, " "), "FATAL",
				"Only reads: git config --get NAME, git config --list")
		}
	case "stash":
		if !gitStashRE.MatchString(p0) {
			d := p0
			if d == "" {
				d = "(push)"
			}
			return refuseOption("git stash", d)
		}
		// 2.x left git stash list unconstrained, and git stash list -p
		// printed the stashed patches of secret files: git puts the stash
		// ref after its own arguments, so no :(exclude) pathspec can be
		// added. A patch option is refused (3.0: LEDGER EXEC-02 fixed).
		if p0 == "list" {
			for _, w := range rest {
				if isGitPatchOpt(w) {
					return refuse("option_not_allowed", "git stash list "+w, "FATAL",
						"git stash list prints the list only here; for a stash's patch use: git diff 'stash@{0}^' 'stash@{0}'")
				}
			}
		}
		if p0 == "show" {
			for _, w := range rest {
				if !isGitStashShowOpt(w) {
					return refuse("option_not_allowed", "git stash show "+w, "FATAL",
						"git stash show prints summaries only here (--stat, --name-only); for the patch use: git diff 'stash@{0}^' 'stash@{0}'")
				}
			}
		}
	case "worktree":
		if p0 != "list" {
			return refuseOption("git worktree", p0)
		}
	case "reflog":
		if gitReflogRE.MatchString(p0) {
			return refuseOption("git reflog", p0)
		}
	}
	return nil
}

func isGitBranchTagWrite(w string) bool {
	switch w {
	case "-d", "-D", "-m", "-M", "-c", "-C", "-f", "-a", "-s", "-u", "-e", "-F",
		"--delete", "--move", "--copy", "--force", "--annotate", "--sign",
		"--edit", "--unset-upstream", "--edit-description", "--create-reflog":
		return true
	}
	return strings.HasPrefix(w, "--local-user") || strings.HasPrefix(w, "--file") ||
		strings.HasPrefix(w, "--message") || strings.HasPrefix(w, "-m") ||
		strings.HasPrefix(w, "--set-upstream-to") || strings.HasPrefix(w, "--track")
}

// isGitPatchOpt is the patch-option test of the child git wrapper
// (__llm_cx_git): an option that makes a log print diffs.
func isGitPatchOpt(w string) bool {
	switch w {
	case "-p", "-u", "-W", "--function-context", "-c", "--cc", "--dd", "-m", "--binary", "--full-diff", "-t":
		return true
	}
	for _, p := range []string{"--patch", "-U", "--unified", "--word-diff", "--color-words", "--remerge-diff", "--diff-merges"} {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

func isGitStashShowOpt(w string) bool {
	switch w {
	case "show", "-u", "--include-untracked", "--only-untracked", "--numstat",
		"--shortstat", "--name-only", "--name-status", "--summary", "--compact-summary",
		"--no-color", "--color", "-z", "--no-renames", "-q", "--quiet":
		return true
	}
	return strings.HasPrefix(w, "stash@") || (len(w) > 0 && w[0] >= '0' && w[0] <= '9') ||
		strings.HasPrefix(w, "--stat") || strings.HasPrefix(w, "--dirstat") ||
		strings.HasPrefix(w, "--color=") || strings.HasPrefix(w, "--abbrev")
}
