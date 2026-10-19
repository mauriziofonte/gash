// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// releaseTag is the 2.x release tag pattern (__gash_latest_tag): pre-release
// tags such as v3.0.0-rc.1 never match.
var releaseTag = regexp.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// signatureMarker is the text __gash_verify_tag looks for in the tag object
// (LEDGER SELF-10, reproduced: unanchored, so a tag message quoting it
// counts as a signature).
var signatureMarker = regexp.MustCompile(`-{5}BEGIN (PGP|SSH) SIGNATURE-{5}`)

// upgrade is gash_upgrade (lib/modules/gash.sh): the install directory, a
// clean git checkout, moves to the highest release tag of origin, its
// signature checked first. rc.A keeps the 2.x tag choice and its
// equality-only comparison, so an install on a newer or pre-release tag
// moves back to the highest final one (LEDGER SELF-01, reproduced); extra
// arguments are ignored (SELF-15). The git commands run as 2.x ran them:
// the user's git, configuration and credentials, fetch and checkout on the
// terminal.
func (m *Module) upgrade(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	dir := installDir(c)

	if st, err := os.Stat(dir + "/.git"); err != nil || !st.IsDir() {
		p.Error("Gash is not installed in " + dir + " as a git checkout. See https://github.com/mauriziofonte/gash.")
		return 1
	}
	// The checkout below would overwrite local edits: refuse instead.
	if m.output(c, "git", "-C", dir, "status", "--porcelain") != "" {
		p.Error("Working tree at " + dir + " has local modifications. Commit, stash, or discard them before upgrading.")
		return 1
	}

	p.Info("Checking for a new release in " + dir + "...")
	if !m.tool(c, "git", "-C", dir, "fetch", "--quiet", "--tags", "origin") {
		p.Error("Failed to fetch updates. Check your network connection, or run 'git fetch' in " + dir + ".")
		return 1
	}

	latest := m.latestTag(c, dir)
	if latest == "" {
		p.Warning("No release tags found; leaving " + dir + " as it is.")
		return 0
	}
	current := m.output(c, "git", "-C", dir, "describe", "--tags", "--exact-match", "HEAD")
	if current == latest {
		date := m.output(c, "git", "-C", dir, "log", "-1", "--format=%as", current)
		p.Success("Gash is already up-to-date (" + current + ", released on " + date + ").")
		return 0
	}

	if !m.verifyTag(c, p, dir, latest) {
		p.Error("GASH_REQUIRE_SIGNED=1 is set; not upgrading to " + latest + ".")
		return 1
	}
	if !m.tool(c, "git", "-C", dir, "-c", "advice.detachedHead=false", "checkout", "--quiet", latest) {
		p.Error("Failed to check out " + latest + ".")
		return 1
	}
	from := ""
	if current != "" {
		from = "from " + current + " "
	}
	p.Success("Upgraded Gash " + from + "to " + latest + ".")
	p.Info("Open a new terminal (or run 'source ~/.gashrc') to load it.")
	return 0
}

// latestTag is __gash_latest_tag DIR: the last release tag in git's
// version order; "" when there is none.
func (m *Module) latestTag(c *invoke.Call, dir string) string {
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "tag", "--list", "--sort=v:refname"},
		Options: runner.Options{Env: c.Env.Child()}})
	if err != nil {
		return ""
	}
	latest := ""
	for _, tag := range strings.Split(string(res.Stdout), "\n") {
		if releaseTag.MatchString(tag) {
			latest = tag
		}
	}
	return latest
}

// verifyTag is __gash_verify_tag DIR TAG: reports a tag's signature when
// the tag object shows one; false only with GASH_REQUIRE_SIGNED=1 and a tag
// that is unsigned or does not verify.
func (m *Module) verifyTag(c *invoke.Call, p ui.Printer, dir, tag string) bool {
	required := c.Env.Get("GASH_REQUIRE_SIGNED") == "1"
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "cat-file", "-p", "refs/tags/" + tag},
		Options: runner.Options{Env: c.Env.Child()}})
	switch {
	case err == nil && signatureMarker.Match(res.Stdout):
		vr, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "verify-tag", tag},
			Options: runner.Options{Env: c.Env.Child()}})
		if err == nil && vr.Success() {
			p.Success("Signature of " + tag + " verified.")
			return true
		}
		p.Warning(tag + " is signed, but the signature could not be verified (is the key in your keyring?).")
	case required:
		p.Warning(tag + " is not signed.")
	default:
		return true
	}
	return !required
}
