// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"io"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// The pretty formats of lib/aliases/git.sh in the Gash palette (256-color
// numbers of lib/core/output.sh): hash accent (214), refs primary (39),
// date and author muted (245), bold for emphasis.
const (
	gitLogFormat        = `%C(214)%h%C(reset)%C(39)%d%C(reset) %s %C(245)(%cr) <%an>%C(reset)`
	gitLogFormatFull    = "%C(214)%h%C(reset) %C(bold)%an%C(reset) %C(245)%ar%C(reset)%C(39)%d%C(reset)%n  %s%n"
	gitLogFormatOneline = `%C(214)%h%C(reset)%C(39)%d%C(reset) %s`
)

// gitLogHelp is the heredoc gl -h/--help prints (shared by every gl*
// variant), verbatim from lib/aliases/git.sh.
const gitLogHelp = `Git Log Aliases - Professional log visualization

USAGE:
  gl [options]      Compact log with graph (current branch)
  gla [options]     All branches with graph
  glo [options]     Ultra-compact oneline format
  glg [options]     Graph focused (first-parent only)
  gls [options]     With file statistics
  glf [file]        Log for specific file with patches

EXAMPLES:
  gl -10            Show last 10 commits
  gl --since=1.week Show commits from last week
  gla               Show all branches
  gls -5            Last 5 commits with changed files
  glf src/main.js   History of specific file

OPTIONS (passed to git log):
  -n, -<number>     Limit number of commits
  --since=<date>    Show commits after date
  --until=<date>    Show commits before date
  --author=<name>   Filter by author
  --grep=<pattern>  Filter by commit message
  --no-merges       Exclude merge commits
  -p, --patch       Show patches (diffs)

TIPS:
  • Use 'q' to exit the pager
  • Combine with grep: gl | grep "fix"
  • Date formats: "2024-01-01", "1 week ago", "yesterday"
`

// registerLog binds the gl log family (lib/aliases/git.sh) to t.
func (m *Module) registerLog(t *invoke.Table) {
	t.Handle("gl", m.gl)
	t.Handle("gla", m.gla)
	t.Handle("glo", m.glo)
	t.Handle("glg", m.glg)
	t.Handle("gls", m.gls)
	t.Handle("glf", m.glf)
}

// isLogHelp is the `[[ "${1-}" == "-h" || "${1-}" == "--help" ]]` test the
// gl family shares.
func isLogHelp(arg string) bool { return arg == "-h" || arg == "--help" }

// logHelp prints the shared gl help to the caller's stdout (gl's `cat <<EOF`).
func (x *call) logHelp() { _, _ = io.WriteString(x.Stdout, gitLogHelp) }

// gitLog is __gash_git_log ARGS: the Gash palette through log.graphColors
// when colors are allowed and the user has not configured them, --no-color
// when NO_COLOR or GASH_NO_COLOR forbids them, nothing otherwise; the git
// output and the exit status go to the caller unchanged. Git itself drops
// color when its stdout is not a terminal, as 2.x. A missing git is the
// prerequisite report (3.4: status 127 and nothing on stderr).
func (x *call) gitLog(extra ...string) int {
	if !x.requireGit() {
		return 1
	}
	var argv []string
	switch {
	case !x.Term.ColorEnvOK():
		// post=(--no-color): git "${pre}" log --no-color "$@"
		argv = append(argv, "log", "--no-color")
	case !x.gitQuiet("config", "--get", "log.graphColors"):
		// pre=(-c log.graphColors=39,245): git -c ... log "$@"
		argv = append(argv, "-c", "log.graphColors=39,245", "log")
	default:
		argv = append(argv, "log")
	}
	argv = append(argv, extra...)
	return x.git(argv...).ExitCode
}

// gl is the compact log with graph for the current branch.
func (m *Module) gl(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	return x.gitLog(append([]string{"--graph", "--pretty=tformat:" + gitLogFormat, "--abbrev-commit"}, c.Args...)...)
}

// gla is gl across all branches.
func (m *Module) gla(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	return x.gitLog(append([]string{"--graph", "--pretty=tformat:" + gitLogFormat, "--abbrev-commit", "--all"}, c.Args...)...)
}

// glo is the ultra-compact oneline graph.
func (m *Module) glo(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	return x.gitLog(append([]string{"--graph", "--pretty=tformat:" + gitLogFormatOneline}, c.Args...)...)
}

// glg is the first-parent graph for a cleaner merge view.
func (m *Module) glg(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	return x.gitLog(append([]string{"--graph", "--pretty=tformat:" + gitLogFormat, "--abbrev-commit", "--first-parent"}, c.Args...)...)
}

// gls is the log with per-commit file statistics (no graph).
func (m *Module) gls(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	return x.gitLog(append([]string{"--pretty=tformat:" + gitLogFormatFull, "--stat"}, c.Args...)...)
}

// glf is the file-specific log with patches; without a file it prints the
// usage and the note and returns 1, as 2.x.
func (m *Module) glf(c *invoke.Call) int {
	x := m.begin(c)
	if isLogHelp(c.Arg(0)) {
		x.logHelp()
		return 0
	}
	if c.Arg(0) == "" {
		x.p.Usage("glf <file> [options]")
		x.p.Note("Shows commit history for a specific file with patches")
		return 1
	}
	// The options go before --: after it, git takes them for paths and
	// --follow refuses more than one (3.4).
	args := append([]string{"--follow", "-p"}, c.Args[1:]...)
	return x.gitLog(append(args, "--", c.Args[0])...)
}
