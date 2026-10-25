// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// releaseTag is the 2.x release tag pattern (__gash_latest_tag): pre-release
// tags such as v3.0.0-rc.1 never match.
var releaseTag = lazyre.MustCompile(`^v?[0-9]+(\.[0-9]+)*$`)

// signatureMarker is the text __gash_verify_tag looks for in the tag object
// (LEDGER SELF-10, reproduced: unanchored, so a tag message quoting it
// counts as a signature).
// tagSigned reports whether the tag object obj (git cat-file tag) carries a
// signature: after the header block, a line equal to "-----BEGIN PGP
// SIGNATURE-----" (or SSH) and, as the last line, the matching END line
// (3.0: SELF-10 fixed; 2.x matched the marker anywhere in the text of the
// object the tag names, so a message quoting it, or the gpgsig header of a
// signed commit under a lightweight tag, counted as a signature).
func tagSigned(obj []byte) bool {
	_, body, ok := strings.Cut(string(obj), "\n\n")
	if !ok {
		return false
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	last := lines[len(lines)-1]
	for _, kind := range []string{"PGP", "SSH"} {
		if last != "-----END "+kind+" SIGNATURE-----" {
			continue
		}
		for _, l := range lines[:len(lines)-1] {
			if l == "-----BEGIN "+kind+" SIGNATURE-----" {
				return true
			}
		}
	}
	return false
}

// upgrade is gash_upgrade (lib/modules/gash.sh), which runs the flow of
// gash update (upgrade.md 3.6, MATRIX "redesign") with the 2.x name, output
// lines and statuses. An argument is refused before any git command (3.0:
// SELF-15 fixed; 2.x ignored it).
func (m *Module) upgrade(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if badArgs(c, "gash_upgrade") {
		return usageStatus
	}
	return m.upgradeFlow(c)
}

// upgradeFlow moves the install directory, a clean git checkout, to the
// highest release tag of origin, its signature checked first (upgrade.md
// 3.6). A checkout newer than that release (a newer or pre-release tag, or
// a commit that has the release in its history) is left alone (3.0:
// SELF-01 fixed; 2.x compared for equality only and moved such an install
// back). When the tag's tree is a 3.x tree, the binary of its VERSION is
// downloaded and verified into the versions directory before the tree is
// touched; after the checkout a smoke test loads the new tree in child
// shells, and a failure checks out the previous commit again (moveTo).
// The git commands run as 2.x ran them: the user's git, configuration and
// credentials, fetch and checkout on the terminal.
func (m *Module) upgradeFlow(c *invoke.Call) int {
	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	dir := installDir(c)
	if !m.cleanCheckout(c, p, dir) {
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
	if shown, newer := m.newerThanRelease(c, dir, current, latest); newer {
		p.Info("This checkout (" + shown + ") is newer than the latest release (" + latest + "); not downgrading.")
		return 0
	}

	if !m.verifyTag(c, p, dir, latest) {
		p.Error("GASH_REQUIRE_SIGNED=1 is set; not upgrading to " + latest + ".")
		return 1
	}
	from := m.head(c, dir, current)
	to := m.inspect(c, dir, latest, latest)
	if rc := m.moveTo(c, p, dir, from, to); rc != 0 {
		return rc
	}
	shown := ""
	if current != "" {
		shown = "from " + current + " "
	}
	p.Success("Upgraded Gash " + shown + "to " + latest + ".")
	m.recordUpdate(c, dir, from, to)
	p.Info("Open a new terminal (or run 'source ~/.gashrc') to load it.")
	return 0
}

// newerThanRelease reports whether the checkout at dir is at or past the
// release latest, and how to name it. A checkout on a version tag
// (current) compares that tag with latest; any other checkout is newer when
// its nearest tag compares >= latest or latest is an ancestor of HEAD
// (LEDGER SELF-01).
func (m *Module) newerThanRelease(c *invoke.Call, dir, current, latest string) (string, bool) {
	if cmp, ok := compareVersions(current, latest); ok {
		return current, cmp >= 0
	}
	shown := current
	if shown == "" {
		shown = m.output(c, "git", "-C", dir, "describe", "--tags")
	}
	if shown == "" {
		shown = m.output(c, "git", "-C", dir, "rev-parse", "--short", "HEAD")
	}
	if nearest := m.output(c, "git", "-C", dir, "describe", "--tags", "--abbrev=0"); nearest != "" {
		if cmp, ok := compareVersions(nearest, latest); ok && cmp >= 0 {
			return shown, true
		}
	}
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "merge-base", "--is-ancestor", latest, "HEAD"},
		Options: runner.Options{Env: c.Env.Child()}})
	return shown, err == nil && res.Success()
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
// the tag is an annotated tag whose object carries one (tagSigned; `git
// cat-file tag` fails for a lightweight tag, which is not signed); false
// only with GASH_REQUIRE_SIGNED=1 and a tag that is unsigned or does not
// verify. An unsigned tag is accepted under GASH_REQUIRE_SIGNED=1 when the
// Sigstore signature of its release verifies for the tag's commit
// (verifyRelease; 3.0.1: the releases are signed keyless by the release
// workflow, not by a person's key); with cosign on PATH that signature is
// checked, and reported, without GASH_REQUIRE_SIGNED too.
func (m *Module) verifyTag(c *invoke.Call, p ui.Printer, dir, tag string) bool {
	required := c.Env.Get("GASH_REQUIRE_SIGNED") == "1"
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "cat-file", "tag", "refs/tags/" + tag},
		Options: runner.Options{Env: c.Env.Child()}})
	switch {
	case err == nil && res.Success() && tagSigned(res.Stdout):
		vr, err := m.run.Run(ctxOf(c), runner.Cmd{Path: "git", Args: []string{"-C", dir, "verify-tag", tag},
			Options: runner.Options{Env: c.Env.Child()}})
		if err == nil && vr.Success() {
			p.Success("Signature of " + tag + " verified.")
			return true
		}
		p.Warning(tag + " is signed, but the signature could not be verified (is the key in your keyring?).")
	default:
		r, detail := m.verifyRelease(c, dir, tag)
		return reportRelease(p, tag, r, detail, required)
	}
	return !required
}
