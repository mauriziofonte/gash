// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package remote

// The remote gash binary (plan S5, D5). A --host call runs the same
// function remotely: when the remote host has this very binary in its
// cache, ~/.cache/gash/remote/<version>-<sha256 prefix>/gash, the call is
// one ssh invocation that runs it with the same arguments. When it does
// not and the remote `uname -sm` matches this binary's OS and architecture,
// the binary is shipped once (one more invocation: upload, size check, a
// `gash version` test run, atomic rename, then the call itself); every
// later call is one invocation again. On another platform, or when the
// upload fails, the function's POSIX sh fallback answers, when it has one
// for these arguments, in the same invocation.

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/buildinfo"
)

// Binary is the local gash binary as the remote cache addresses it.
type Binary struct {
	Path string
	Size int64
	// Dir is the cache directory below the remote $HOME:
	// .cache/gash/remote/<version>-<first 12 hex of the sha256>.
	Dir string
	// Platform is the case pattern of `uname -s`/`uname -m` this binary
	// runs on ("Linux/x86_64|Linux/amd64"); "" when it is shipped nowhere.
	Platform string
}

// LocalBinary hashes the running binary (exe returns its path; nil means
// os.Executable). ok is false when it cannot be read: the call then runs
// without the remote binary (cache test and shipping left out).
func LocalBinary(exe func() (string, error)) (Binary, bool) {
	if exe == nil {
		exe = os.Executable
	}
	p, err := exe()
	if err != nil {
		return Binary{}, false
	}
	f, err := os.Open(p)
	if err != nil {
		return Binary{}, false
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil || n == 0 {
		return Binary{}, false
	}
	sum := hex.EncodeToString(h.Sum(nil))
	return Binary{
		Path:     p,
		Size:     n,
		Dir:      ".cache/gash/remote/" + cacheVersion(buildinfo.Version) + "-" + sum[:12],
		Platform: platformCase(runtime.GOOS, runtime.GOARCH),
	}, true
}

// cacheVersion is the version as one safe path component.
func cacheVersion(v string) string {
	out := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, v)
	if out == "" {
		return "dev"
	}
	return out
}

// platformCase is the `uname -s`/`uname -m` case pattern of goos/goarch,
// "" for a platform gash is not shipped to.
func platformCase(goos, goarch string) string {
	sys := map[string]string{"linux": "Linux", "darwin": "Darwin"}[goos]
	machines := map[string][]string{"amd64": {"x86_64", "amd64"}, "arm64": {"aarch64", "arm64"}}[goarch]
	if sys == "" || machines == nil {
		return ""
	}
	pats := make([]string, len(machines))
	for i, m := range machines {
		pats[i] = sys + "/" + m
	}
	return strings.Join(pats, "|")
}

// runGash is the rest of the sh line that runs the binary named just
// before it with args, ending with the "rc" trailer.
func runGash(nonce string, args []string) string {
	return QuoteAll(args) + " </dev/null; gr=$?; " + Trailer(nonce, "rc", `"$gr"`) + "; exit 0\n"
}

// fallbackPart is the end of a dispatch script: the fallback body in a
// subshell (so its exits end only the subshell) with the "fb" trailer, or
// the "nobin" trailer with the remote platform when there is none.
func fallbackPart(nonce, fallback, noneTag string) string {
	if fallback == "" {
		return TrailerDetail(nonce, noneTag, "0", `"$(uname -sm 2>/dev/null)"`) + "\nexit 0\n"
	}
	return "(\n" + fallback + "\n)\ngr=$?\n" + Trailer(nonce, "fb", `"$gr"`) + "\nexit 0\n"
}

// DispatchScript runs fn ARGS on the remote side: the cached binary when
// it is there; else, when the platform matches and shipping is possible,
// the "ship" trailer (the caller then sends ShipScript); else the fallback
// body (trailer "fb"), or the "nobin" trailer.
func DispatchScript(nonce string, b Binary, haveBin bool, fn string, args []string, fallback string) string {
	var s strings.Builder
	s.WriteString("N=" + Quote(nonce) + "\n")
	if haveBin {
		s.WriteString(`G="$HOME"/` + Quote(b.Dir+"/gash") + "\n")
		s.WriteString(`if [ -x "$G" ]; then "$G" ` + runGash(nonce, append([]string{fn}, args...)) + "fi\n")
		if b.Platform != "" {
			s.WriteString(`case "$(uname -s 2>/dev/null)/$(uname -m 2>/dev/null)" in ` + b.Platform + ") " +
				Trailer(nonce, "ship", "0") + "; exit 0 ;; esac\n")
		}
	}
	s.WriteString(fallbackPart(nonce, fallback, "nobin"))
	return s.String()
}

// ShipScript installs the binary that arrives on stdin into the cache
// (umask 077, size check, a `gash version` test run, atomic rename) and
// runs fn ARGS with it. When any step fails it drains stdin and runs the
// fallback body, or ends with the "shipfail" trailer.
func ShipScript(nonce string, b Binary, fn string, args []string, fallback string) string {
	var s strings.Builder
	s.WriteString("N=" + Quote(nonce) + "\n")
	s.WriteString(`D="$HOME"/` + Quote(b.Dir) + "\n")
	s.WriteString(`T="$D/gash.$$"` + "\n")
	s.WriteString(`if (umask 077 && mkdir -p "$D" && cat >"$T" && [ "$(wc -c <"$T" | tr -d ' ')" = ` +
		Quote(strconv.FormatInt(b.Size, 10)) + ` ] && chmod 700 "$T" && "$T" version >/dev/null 2>&1 && mv -f "$T" "$D/gash"); then` + "\n")
	s.WriteString(`  "$D/gash" ` + runGash(nonce, append([]string{fn}, args...)))
	s.WriteString("fi\n")
	s.WriteString(`rm -f "$T" 2>/dev/null; cat >/dev/null 2>&1` + "\n")
	s.WriteString(fallbackPart(nonce, fallback, "shipfail"))
	return s.String()
}
