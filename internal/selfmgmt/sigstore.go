// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"os"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// The release signature: the release workflow signs checksums.txt keyless
// with cosign (.goreleaser.yaml signs) and publishes the Sigstore bundle as
// checksums.txt.sigstore.json. Its Fulcio certificate names the workflow,
// the tag and the commit the tag points at, so a bundle that verifies for
// the tag's commit covers the tree that commit holds, release/SHA256SUMS
// and the binaries it pins included.
const (
	releaseRepo     = "mauriziofonte/gash"
	releaseWorkflow = ".github/workflows/release.yml"
	actionsIssuer   = "https://token.actions.githubusercontent.com"
	bundleAsset     = "checksums.txt.sigstore.json"
)

// sigstoreResult is the outcome of verifyRelease.
type sigstoreResult int

const (
	sigstoreVerified sigstoreResult = iota
	sigstoreNoCosign                // cosign is not on PATH
	sigstoreMissing                 // the release has no bundle (or no checksums.txt)
	sigstoreInvalid                 // the bundle does not verify for the tag's commit
)

// cosignArgs is the cosign verify-blob command line that accepts only a
// bundle the release workflow of releaseRepo signed for tag at commit.
func cosignArgs(bundle, blob, tag, commit string) []string {
	ref := "refs/tags/" + tag
	return []string{"verify-blob", "--bundle", bundle,
		"--certificate-identity", "https://github.com/" + releaseRepo + "/" + releaseWorkflow + "@" + ref,
		"--certificate-oidc-issuer", actionsIssuer,
		"--certificate-github-workflow-repository", releaseRepo,
		"--certificate-github-workflow-ref", ref,
		"--certificate-github-workflow-sha", commit,
		blob}
}

// verifyRelease checks the Sigstore bundle of the release of tag (the
// checkout at dir holds the tag) with cosign, downloading checksums.txt and
// its bundle from GASH_RELEASE_URL, else the GitHub release of tag. The
// detail is cosign's last error line for sigstoreInvalid.
func (m *Module) verifyRelease(c *invoke.Call, dir, tag string) (sigstoreResult, string) {
	cosign, ok := m.lookPath(c, "cosign")
	if !ok {
		return sigstoreNoCosign, ""
	}
	commit := m.output(c, "git", "-C", dir, "rev-parse", "--verify", "--quiet", "refs/tags/"+tag+"^{commit}")
	if len(commit) != 40 {
		return sigstoreMissing, ""
	}
	base := c.Env.Get("GASH_RELEASE_URL")
	if base == "" {
		base = releaseBase + "/" + tag
	}
	base = strings.TrimRight(base, "/")

	tmp, err := os.MkdirTemp("", "gash-verify."+strconv.Itoa(os.Getpid())+".")
	if err != nil {
		return sigstoreMissing, ""
	}
	defer os.RemoveAll(tmp)
	blob, bundle := tmp+"/checksums.txt", tmp+"/"+bundleAsset
	if m.get(ctxOf(c), base+"/checksums.txt", blob) != nil || m.get(ctxOf(c), base+"/"+bundleAsset, bundle) != nil {
		return sigstoreMissing, ""
	}
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: cosign, Args: cosignArgs(bundle, blob, tag, commit),
		Options: runner.Options{Env: c.Env.Child()}})
	if err == nil && res.Success() {
		return sigstoreVerified, ""
	}
	return sigstoreInvalid, lastLine(string(res.Stderr))
}

// reportRelease prints the outcome of verifyRelease for tag and reports
// whether the upgrade may go on: always when the signature verified, else
// only without GASH_REQUIRE_SIGNED=1 (an invalid bundle is reported either
// way; a missing one, or no cosign, only when a signature is required).
func reportRelease(p ui.Printer, tag string, r sigstoreResult, detail string, required bool) bool {
	switch r {
	case sigstoreVerified:
		p.Success("Release signature of " + tag + " verified (Sigstore).")
		return true
	case sigstoreInvalid:
		msg := "The release signature of " + tag + " does not verify"
		if detail != "" {
			msg += ": " + detail
		}
		p.Warning(msg + ".")
	case sigstoreMissing:
		if required {
			p.Warning(tag + " is not signed, and its release has no Sigstore signature (" + bundleAsset + ").")
		}
	case sigstoreNoCosign:
		if required {
			p.Warning(tag + " is not signed; install cosign to check its Sigstore release signature " +
				"(https://docs.sigstore.dev/cosign/system_config/installation/).")
		}
	}
	return !required
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(strings.TrimPrefix(lines[len(lines)-1], "Error: "))
}
