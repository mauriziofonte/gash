// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

// llm_same (plan R3): is the deployed copy the local one? The local file
// or tree is hashed here (sha256), each ALIAS:REMOTE target on its host in
// ONE ssh round trip (sha256sum, shasum or openssl there; inside a
// container with --container), and the two listings are compared path by
// path.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/remote"
)

const (
	// sameTimeout bounds each target.
	sameTimeout = 120 * time.Second
	// sameDiffs is the default cap of the diff list.
	sameDiffs = 100
)

var containerNameRE = lazyre.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)

// sameTarget is one ALIAS:REMOTE argument.
type sameTarget struct {
	host remote.Host
	path string
}

// sameSide is one side's listing: kind is "file", "directory" or ""
// (missing); hashes maps a path (relative to a directory, "." for a file)
// to its sha256.
type sameSide struct {
	kind   string
	hashes map[string]string
}

type sameDiff struct {
	Host   string  `json:"host"`
	P      string  `json:"p"`
	Local  *string `json:"local"`
	Remote *string `json:"remote"`
}

type sameOut struct {
	Same           bool       `json:"same"`
	Files          int        `json:"files"`
	Diff           []sameDiff `json:"diff"`
	Truncated      int        `json:"truncated,omitempty"`
	SecretsSkipped int        `json:"secrets_skipped,omitempty"`
}

func (m *Module) same(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.BudgetFlags | llmfmt.JobsFlag | llmfmt.TimeoutFlag)
	local, haveLocal := "", false
	var args []string
	container := ""
	excludes := []string{".git"}
	for rest := c.Args; len(rest) > 0; {
		n, ok := o.Take(c, rest)
		if !ok {
			return 1
		}
		if n > 0 {
			rest = rest[n:]
			continue
		}
		a := rest[0]
		switch {
		case a == "--container" || a == "--exclude":
			if len(rest) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			v := rest[1]
			if a == "--container" {
				if !containerNameRE.MatchString(v) {
					return fail(c, "invalid_container_name", v, "A container name or id: letters, digits, _ . - (docker ps on the host lists them)")
				}
				container = v
			} else {
				if _, err := path.Match(v, ""); err != nil || v == "" || strings.Contains(v, "/") {
					return fail(c, "invalid_exclude", v, "--exclude takes a file or directory name glob such as node_modules or *.log (no /)")
				}
				excludes = append(excludes, v)
			}
			rest = rest[2:]
			continue
		case strings.HasPrefix(a, "-") && a != "-":
			return fail(c, "unknown_option", a, "Run llm_same --help for the options")
		case !haveLocal:
			local, haveLocal = a, true
		default:
			args = append(args, a)
		}
		rest = rest[1:]
	}
	if !haveLocal {
		return fail(c, "missing_file", "", "Provide the LOCAL file or directory, then ALIAS:REMOTE")
	}
	if len(args) == 0 {
		return fail(c, "missing_host", "", "Name at least one ALIAS:REMOTE target")
	}
	targets := make([]sameTarget, len(args))
	for i, a := range args {
		alias, p, ok := strings.Cut(a, ":")
		if !ok || alias == "" || p == "" {
			return fail(c, "invalid_target", a, "Give each target as ALIAS:PATH, ALIAS a registered HOST: alias")
		}
		h, e := remote.Resolve(c.Env.Get, alias)
		if e != nil {
			return c.Fail(*e)
		}
		if e := remote.CheckRead(p); e != nil {
			return c.Fail(*e)
		}
		targets[i] = sameTarget{host: h, path: p}
	}
	safe, ok := m.validatePath(c, local)
	if !ok {
		return 1
	}
	fi, err := os.Stat(safe)
	if err != nil {
		return fail(c, "path_not_found", safe, "Provide an existing file or directory")
	}
	mine := sameSide{kind: "file", hashes: map[string]string{}}
	secrets := map[string]bool{}
	if fi.IsDir() {
		under, ok := m.walkRoot(c, safe, "names")
		if !ok {
			return 1
		}
		mine.kind = "directory"
		mine.hashes, err = hashTree(c.Ctx, safe, excludes, under, secrets)
		if err != nil {
			return fail(c, "path_unreadable", err.Error(), "The file could not be read")
		}
	} else {
		if m.guard(c).IsSecretFile(safe) {
			return fail(c, "secret_file_blocked", safe, "Secret files are not compared")
		}
		h, err := hashFile(safe)
		if err != nil {
			return fail(c, "path_unreadable", safe, "The file could not be read")
		}
		mine.hashes["."] = h
	}

	tr := remote.Transport{Run: m.run, Env: c.Env}
	nonce := remote.NewNonce()
	timeout := o.TimeoutOr(sameTimeout)
	type got struct {
		side    sameSide
		secrets []string
		err     *contract.Envelope
	}
	sides := llmfmt.Map(c.Ctx, o.JobsOr(llmfmt.NetworkJobs), 0, targets, func(ctx context.Context, t sameTarget) got {
		r := tr.Exec(ctx, t.host, nonce, sameScript(nonce, t.path, excludes, container), nil, timeout)
		if e := r.Failure(t.host); e != nil {
			return got{err: e}
		}
		errOut := r.Stderr
		if e, ok := fbEnvelope(errOut, nonce, sameHint); ok {
			return got{err: &e}
		}
		kind, ok := fbFrame(errOut, nonce, "kind")
		if !ok {
			detail := t.host.Alias + ": " + strings.TrimSpace(lastLineOf(errOut))
			if container != "" {
				e := contract.New("container_unavailable", detail, "Check the container name and that it runs (docker ps on the host)")
				return got{err: &e}
			}
			e := contract.New("remote_protocol_error", detail, "The remote side ended without its answer")
			return got{err: &e}
		}
		g := got{side: sameSide{hashes: map[string]string{}}}
		switch kind {
		case "directory", "file":
			g.side.kind = kind
			for p, h := range parseHashes(string(r.Stdout), kind == "file") {
				if kind == "directory" && pathguard.SecretName(p) {
					g.secrets = append(g.secrets, p)
					continue
				}
				g.side.hashes[p] = h
			}
		}
		return g
	})
	var out sameOut
	out.Same = true
	out.Files = len(mine.hashes)
	for i, s := range sides {
		if s.err != nil {
			return c.Fail(*s.err)
		}
		for _, p := range s.secrets {
			secrets[p] = true
		}
		d := compareSides(targets[i].host.Alias, local, mine, s.side)
		if len(d) > 0 {
			out.Same = false
			out.Diff = append(out.Diff, d...)
		}
	}
	budget := *o
	if !budget.Capped() {
		budget.Head = sameDiffs
	}
	head, tail, omitted := llmfmt.Cap(&budget, out.Diff)
	out.Diff = append(append([]sameDiff{}, head...), tail...)
	if out.Diff == nil {
		out.Diff = []sameDiff{}
	}
	if o.Width > 0 {
		for i := range out.Diff {
			out.Diff[i].P = llmfmt.Cut(out.Diff[i].P, o.Width)
		}
	}
	out.Truncated = omitted
	if omitted > 0 {
		c.NoteTruncated(omitted)
	}
	out.SecretsSkipped = len(secrets)
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}

// compareSides lists the paths whose hash differs, or that one side lacks;
// a file against a directory, or a missing remote, is one entry for ".".
func compareSides(host, local string, mine, theirs sameSide) []sameDiff {
	short := func(h string) *string {
		s := h
		if len(s) > 12 {
			s = s[:12]
		}
		return &s
	}
	word := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	if mine.kind != theirs.kind {
		l := word(mine.kind)
		if mine.kind == "file" {
			l = short(mine.hashes["."])
		}
		return []sameDiff{{Host: host, P: ".", Local: l, Remote: word(theirs.kind)}}
	}
	if mine.kind == "file" {
		if mine.hashes["."] == theirs.hashes["."] {
			return nil
		}
		var r *string
		if h, ok := theirs.hashes["."]; ok {
			r = short(h)
		}
		return []sameDiff{{Host: host, P: local, Local: short(mine.hashes["."]), Remote: r}}
	}
	keys := map[string]bool{}
	for p := range mine.hashes {
		keys[p] = true
	}
	for p := range theirs.hashes {
		keys[p] = true
	}
	sorted := make([]string, 0, len(keys))
	for p := range keys {
		sorted = append(sorted, p)
	}
	sort.Strings(sorted)
	var out []sameDiff
	for _, p := range sorted {
		a, okA := mine.hashes[p]
		b, okB := theirs.hashes[p]
		if okA && okB && a == b {
			continue
		}
		d := sameDiff{Host: host, P: p}
		if okA {
			d.Local = short(a)
		}
		if okB {
			d.Remote = short(b)
		}
		out = append(out, d)
	}
	return out
}

// sameSkip reports whether a walk leaves a name out: an excluded name, or
// a name holding a newline or a backslash (the hash tools escape those).
func sameSkip(name string, excludes []string) bool {
	if strings.ContainsAny(name, "\n\\") {
		return true
	}
	for _, x := range excludes {
		if ok, _ := path.Match(x, name); ok {
			return true
		}
	}
	return false
}

// hashTree hashes the regular files below root (symlinks are not
// followed, excluded names and protected trees are skipped, secret files
// are counted in secrets and left out).
func hashTree(ctx context.Context, root string, excludes []string, under func(string) bool, secrets map[string]bool) (map[string]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == root {
				return err
			}
			return nil
		}
		if p == root {
			return nil
		}
		if sameSkip(d.Name(), excludes) || under(p) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel := filepath.ToSlash(strings.TrimPrefix(p, root+string(filepath.Separator)))
		if root == string(filepath.Separator) {
			rel = filepath.ToSlash(strings.TrimPrefix(p, root))
		}
		if pathguard.SecretName(rel) {
			secrets[rel] = true
			return nil
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sums := llmfmt.Map(ctx, llmfmt.LocalJobs(), 0, files, func(_ context.Context, rel string) string {
		h, err := hashFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return ""
		}
		return h
	})
	out := make(map[string]string, len(files))
	for i, rel := range files {
		if sums[i] != "" {
			out[rel] = sums[i]
		}
	}
	return out, nil
}

func hashFile(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// parseHashes reads "HASH  NAME" lines (sha256sum, shasum; openssl -r
// writes "HASH *NAME"): the names below "./" for a directory, "." for a
// file.
func parseHashes(out string, file bool) map[string]string {
	hashes := map[string]string{}
	for _, l := range strings.Split(out, "\n") {
		if len(l) < 66 {
			continue
		}
		h := strings.ToLower(l[:64])
		if _, err := hex.DecodeString(h); err != nil {
			continue
		}
		name := strings.TrimPrefix(l[64:], " ")
		if strings.HasPrefix(name, " ") || strings.HasPrefix(name, "*") {
			name = name[1:]
		}
		if file {
			hashes["."] = h
			continue
		}
		if rel, ok := strings.CutPrefix(name, "./"); ok && rel != "" {
			hashes[rel] = h
		}
	}
	return hashes
}

// sameScript is the remote side: the path checks of the fallbacks, then
// the hash listing of the file or the tree (frame "kind": file, directory
// or none); with container, all of it inside `docker exec CTR sh -c`.
func sameScript(nonce, p string, excludes []string, container string) string {
	var prune strings.Builder
	prune.WriteString(`\( -name "*$NL*" -o -name '*\\*'`)
	for _, x := range excludes {
		prune.WriteString(" -o -name " + remote.Quote(x))
	}
	prune.WriteString(` \) -prune -o -type f`)
	body := "N=" + remote.Quote(nonce) + "\n" + fbHeader("P", p) + `(
if command -v sha256sum >/dev/null 2>&1; then H=sha256sum
elif command -v shasum >/dev/null 2>&1; then H='shasum -a 256'
elif command -v openssl >/dev/null 2>&1; then H='openssl dgst -sha256 -r'
else gash_err hash_tool_not_found tool "no sha256sum, shasum or openssl on the host"; fi
abs=$(gash_resolve "$P")
gash_forbidden "$abs" && gash_err forbidden_path protected "$abs"
if [ -d "$abs" ]; then
  gash_forbidden "$abs" names && gash_err forbidden_path subdir "$abs"
  cd "$abs" || gash_err path_unreadable couldnot "$abs"
  printf '%s kind directory\n' "$N" >&2
  find . ` + prune.String() + ` -exec $H {} + 2>/dev/null
elif [ -f "$abs" ]; then
  printf '%s kind file\n' "$N" >&2
  $H "$abs" 2>/dev/null
else
  printf '%s kind none\n' "$N" >&2
fi
exit 0
)
`
	if container == "" {
		return body + "gr=$?\n" + remote.Trailer(nonce, "rc", `"$gr"`) + "\nexit 0\n"
	}
	return "S=" + remote.Quote(body) + "\ndocker exec " + remote.Quote(container) + ` sh -c "$S" </dev/null` + "\ngr=$?\n" +
		remote.Trailer(nonce, "rc", `"$gr"`) + "\nexit 0\n"
}

// sameHint gives the error frames of sameScript their hints.
func sameHint(code, key, details string) (string, string) {
	if key == "tool" {
		return details, "Install coreutils (sha256sum) on the host, or compare with llm_remote_run"
	}
	return details, fbHints[key]
}

func lastLineOf(b []byte) string {
	s := strings.TrimRight(string(b), "\n\r ")
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		s = s[i+1:]
	}
	return s
}
