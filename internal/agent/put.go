// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/remote"
)

const (
	// maxPutBytes caps what agent_put uploads (held in memory to hash it
	// first).
	maxPutBytes = 64 << 20
	// putTimeout bounds the whole call.
	putTimeout = 300 * time.Second
)

var modeRE = lazyre.MustCompile(`^[0-7]{3,4}$`)

// putOut is the answer of agent_put; Exists only on a dry run, Replaced
// only on a write.
type putOut struct {
	Host     string  `json:"host"`
	Path     string  `json:"path"`
	Bytes    int     `json:"bytes"`
	SHA256   string  `json:"sha256"`
	Exists   *bool   `json:"exists,omitempty"`
	Replaced *bool   `json:"replaced,omitempty"`
	Backup   *string `json:"backup"`
	Verified bool    `json:"verified"`
	DryRun   bool    `json:"dry_run,omitempty"`
	Written  bool    `json:"written"`
}

// put is agent_put (plan E2): upload FILE (or stdin) to ALIAS:PATH in ONE
// ssh invocation: the content goes on stdin to a temporary file next to
// the target (the old file's mode kept), its size checked, its sha256
// checked with --verify, the old file copied to PATH.bak-<UTC time> with
// --backup, then the temporary file renamed over PATH. The remote write
// rails run here before connecting and again on the host on the resolved
// path; a failed check leaves the target as it was.
func (m *Module) put(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.TimeoutFlag)
	target, from, mode := "", "", ""
	haveFrom := false
	backup, verify, force, dry := false, false, false, false
	for args := c.Args; len(args) > 0; {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		a := args[0]
		switch a {
		case "--from", "--mode":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			if a == "--from" {
				from, haveFrom = args[1], true
			} else {
				mode = args[1]
			}
			args = args[2:]
			continue
		case "--backup":
			backup = true
		case "--verify":
			verify = true
		case "--force":
			force = true
		case "--dry-run":
			dry = true
		default:
			switch {
			case strings.HasPrefix(a, "-"):
				return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
			case target != "":
				return fail(c, "unexpected_argument", a, "agent_put writes one ALIAS:PATH per call")
			}
			target = a
		}
		args = args[1:]
	}
	if target == "" {
		return fail(c, "missing_host", "", "Name the target as ALIAS:PATH, ALIAS a registered HOST: alias")
	}
	if !haveFrom || from == "" {
		return fail(c, "missing_file", "", "Give the content with --from FILE, or --from - to read stdin")
	}
	if mode != "" && !modeRE.MatchString(mode) {
		return fail(c, "invalid_mode", mode, "Give the mode in octal, such as 644 or 0640")
	}
	alias, p, ok := strings.Cut(target, ":")
	if !ok || alias == "" || p == "" {
		return fail(c, "invalid_target", target, "Give the target as ALIAS:PATH, ALIAS a registered HOST: alias")
	}
	h, e := remote.Resolve(c.Env.Get, alias)
	if e != nil {
		return c.Fail(*e)
	}
	if e := remote.CheckWrite(p, force, h); e != nil {
		return c.Fail(*e)
	}
	data, ok := m.putSource(c, from)
	if !ok {
		return 1
	}
	if !remote.RequireSSH(c, "agent_put") {
		return 1
	}
	sum := sha256.Sum256(data)
	hexSum := hex.EncodeToString(sum[:])

	nonce := remote.NewNonce()
	tr := remote.Transport{Run: m.run, Env: c.Env}
	var stdin io.Reader
	if !dry {
		stdin = bytes.NewReader(data)
	}
	r := tr.Exec(c.Ctx, h, nonce, putScript(nonce, p, mode, len(data), hexSum, backup, verify, force, dry), stdin, o.TimeoutOr(putTimeout))
	if tool, ok := remote.MissingFrame(r.Stderr, nonce); ok {
		return c.Fail(remote.MissingOnHost("agent_put", tool, h, ""))
	}
	if code, _, details, ok := remote.ErrFrame(r.Stderr, nonce); ok {
		if _, known := contract.ActionOf(code); !known {
			code, details = "internal_error", "agent_put on "+h.Alias+": "+code+" "+details
		}
		return c.Fail(contract.New(code, details, putHints[code]))
	}
	if e := r.Failure(h); e != nil {
		return c.Fail(*e)
	}
	abs, okPath := remote.Frame(r.Stderr, nonce, "path")
	existed, okDone := remote.Frame(r.Stderr, nonce, "done")
	if !okPath || !okDone {
		return fail(c, "remote_protocol_error", h.Alias+": agent_put ended without its answer", "The remote login shell must run POSIX sh commands")
	}
	out := putOut{Host: h.Alias, Path: abs, Bytes: len(data), SHA256: hexSum}
	was := existed == "1"
	if dry {
		out.Exists, out.DryRun = &was, true
	} else {
		out.Replaced, out.Written, out.Verified = &was, true, verify
		if b, ok := remote.Frame(r.Stderr, nonce, "backup"); ok && b != "" {
			out.Backup = &b
		}
	}
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}

// putSource reads the content: stdin for "-", else a local file under the
// llm_* read rails (never a secret file), at most maxPutBytes.
func (m *Module) putSource(c *invoke.Call, from string) ([]byte, bool) {
	tooLarge := func(what string) ([]byte, bool) {
		fail(c, "file_too_large", what+" is over "+strconv.Itoa(maxPutBytes>>20)+" MiB", "agent_put uploads files up to 64 MiB; use remote_push for larger ones")
		return nil, false
	}
	if from == "-" {
		if c.Stdin == nil {
			return []byte{}, true
		}
		data, err := io.ReadAll(io.LimitReader(c.Stdin, maxPutBytes+1))
		if err != nil {
			fail(c, "path_unreadable", "stdin", "The content could not be read")
			return nil, false
		}
		if len(data) > maxPutBytes {
			return tooLarge("stdin")
		}
		return data, true
	}
	g := m.guard(c)
	safe, err := g.ValidatePath(from)
	if err != nil {
		refusal(c, err)
		return nil, false
	}
	if g.IsSecretFile(from) || g.IsSecretFile(safe) {
		fail(c, "secret_file_blocked", from, "agent_put never uploads secret files")
		return nil, false
	}
	fi, err := os.Stat(safe)
	switch {
	case err != nil:
		fail(c, "file_not_found", safe, "Check the file path and try again")
		return nil, false
	case !fi.Mode().IsRegular():
		fail(c, "invalid_target", safe+" is not a regular file", "--from takes a file, or - for stdin")
		return nil, false
	case fi.Size() > maxPutBytes:
		return tooLarge(safe)
	}
	data, err := m.readFile(safe)
	if err != nil {
		fail(c, "path_unreadable", safe, "The file could not be read")
		return nil, false
	}
	return data, true
}

// putHints are the hints of the remote error frames.
var putHints = map[string]string{
	"write_path_denied":   "This path is never written, not even with --force",
	"sensitive_path":      "Ask the user; with their agreement run again with --force",
	"secret_file_blocked": "agent_* functions never write secret files",
	"path_not_found":      "The target's directory must exist on the host; agent_put creates no directory",
	"invalid_target":      "Name the file to write, not a directory",
	"write_failed":        "Nothing was replaced; check the directory's permissions and free space on the host",
	"put_verify_failed":   "The upload did not arrive intact; nothing was replaced. Retry once, then ask the user",
	"hash_tool_not_found": "Run again without --verify (the size is always checked), or install coreutils on the host",
}

// putScript is the remote side of agent_put: the rails on the resolved
// path, the programs the write runs (a dry run checks them too), then
// (unless dry) the upload from stdin to a temporary file in the target's
// directory, the size and --verify checks, the --mode, the --backup copy
// and the rename. Frames: "path ABS", "done EXISTED", "backup PATH" (empty
// without one), "missing TOOL".
func putScript(nonce, p, mode string, size int, sum string, backup, verify, force, dry bool) string {
	flag := func(b bool) string {
		if b {
			return "1"
		}
		return ""
	}
	var s strings.Builder
	s.WriteString("N=" + remote.Quote(nonce) + "\nLC_ALL=C; export LC_ALL\n")
	s.WriteString(remote.ShErr() + remote.ShTools() + remote.ShResolve() + remote.ShForbidden() + remote.ShSecret() + remote.ShTier())
	for _, kv := range [][2]string{{"P", p}, {"MODE", mode}, {"SIZE", strconv.Itoa(size)}, {"SUM", sum},
		{"BACKUP", flag(backup)}, {"VERIFY", flag(verify)}, {"FORCE", flag(force)}, {"DRY", flag(dry)}} {
		s.WriteString(kv[0] + "=" + remote.Quote(kv[1]) + "\n")
	}
	s.WriteString(`(
abs=$(gash_resolve "$P") || exit 1
gash_secret "$abs" && gash_err secret_file_blocked secret "$abs"
gash_forbidden "$abs" && gash_err write_path_denied deny "$abs (matched: protected)"
t=$(gash_tier "$abs")
case $t in
  deny:*) gash_err write_path_denied deny "$abs (matched: ${t#deny:})" ;;
  ask:*) [ -n "$FORCE" ] || gash_err sensitive_path ask "$abs (matched: ${t#ask:})" ;;
esac
d=${abs%/*}; [ -n "$d" ] || d=/
[ -d "$d" ] || gash_err path_not_found nodir "$d"
[ -d "$abs" ] && gash_err invalid_target isdir "$abs is a directory"
e=0; [ -e "$abs" ] && e=1
[ -w "$d" ] || gash_err write_failed perm "$d is not writable"
gash_tools cat wc mv rm
[ "$e" = 1 ] && gash_tools cp
[ -n "$MODE" ] && gash_tools chmod
[ -n "$BACKUP" ] && [ "$e" = 1 ] && gash_tools date
printf '%s path %s\n' "$N" "$abs" >&2
if [ -n "$DRY" ]; then printf '%s done %s\n' "$N" "$e" >&2; exit 0; fi
T=$d/.gash-put.$$.tmp
trap 'rm -f "$T"' EXIT
if [ "$e" = 1 ]; then cp -p "$abs" "$T" 2>/dev/null || rm -f "$T"; fi
cat >"$T" || gash_err write_failed write "$abs: the upload failed"
set -- $(wc -c <"$T")
[ "$1" = "$SIZE" ] || gash_err write_failed short "$abs: the upload was cut short"
if [ -n "$VERIFY" ]; then
  if command -v sha256sum >/dev/null 2>&1; then h=$(sha256sum <"$T")
  elif command -v shasum >/dev/null 2>&1; then h=$(shasum -a 256 <"$T")
  elif command -v openssl >/dev/null 2>&1; then h=$(openssl dgst -sha256 -r <"$T")
  else gash_err hash_tool_not_found tool "no sha256sum, shasum or openssl on the host"; fi
  h=${h%%[ *]*}
  [ "$h" = "$SUM" ] || gash_err put_verify_failed verify "$abs: the host hashed $h"
fi
if [ -n "$MODE" ]; then chmod "$MODE" "$T" || gash_err write_failed mode "$abs: chmod $MODE failed"; fi
B=
if [ -n "$BACKUP" ] && [ "$e" = 1 ]; then
  B=$abs.bak-$(date -u +%Y%m%dT%H%M%SZ)
  [ -e "$B" ] && B=$B.$$
  cp -p "$abs" "$B" || gash_err write_failed backup "$B: the backup copy failed"
fi
mv -f "$T" "$abs" || gash_err write_failed rename "$abs: the rename failed"
printf '%s backup %s\n%s done %s\n' "$N" "$B" "$N" "$e" >&2
exit 0
)
gr=$?
`)
	s.WriteString(remote.Trailer(nonce, "rc", `"$gr"`) + "\nexit 0\n")
	return s.String()
}
