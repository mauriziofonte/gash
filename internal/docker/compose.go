// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/google/go-containerregistry/pkg/name"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// composeCmd is __gash_compose_cmd: ["docker" "compose"] when `docker
// compose version` succeeds, ["docker-compose"] when that program is on
// the PATH, nil otherwise (docker itself must be on the PATH). Every
// call asks again, as 2.x did.
func (x *call) composeCmd() []string {
	if !x.have("docker") {
		return nil
	}
	if x.run(io.Discard, io.Discard, "docker", "compose", "version").ExitCode == 0 {
		return []string{"docker", "compose"}
	}
	if x.have("docker-compose") {
		return []string{"docker-compose"}
	}
	return nil
}

// requireCompose is __gash_require_compose.
func (x *call) requireCompose() bool {
	if x.composeCmd() != nil {
		return true
	}
	x.p.Error("Docker Compose not found. Install docker-compose or Docker Desktop.")
	return false
}

// resolveDir is resolveComposeDir RAW: the GNU realpath -m form of the
// argument (LEDGER DOCK-02). An empty argument, or one that cannot be
// resolved, is refused (DOCK-01): 2.x fell back to the current directory.
// GNU -m keeps a looping symlink as a plain name where the walk refuses
// it, so a path through a loop takes its lexical form.
func (x *call) resolveDir(p string) (string, bool) {
	if p == "" {
		return "", false
	}
	wd, _ := os.Getwd()
	g := pathguard.NewGuard(pathguard.OSFS{}, wd, x.Env.Get("HOME"), "")
	if r, err := g.Realpath(p, false); err == nil {
		return r, true
	}
	if r, err := g.Realpath(p, true); err == nil {
		return r, true
	}
	return "", false
}

// refuseDir reports a PATH argument resolveDir refused: the envelope with
// --json, the error line otherwise.
func (x *call) refuseDir(raw string, jsonOut bool) int {
	if jsonOut {
		return x.Fail(contract.New("path_not_found", raw, "Pass the directory that holds the compose file (omit it for the current directory)"))
	}
	x.p.Error("Cannot resolve path: '" + raw + "'")
	return 1
}

// composeNames are the file names Compose discovers, in its order
// (compose-go DefaultFileNames, LEDGER DOCK-29).
var composeNames = []string{"compose.yaml", "compose.yml", "docker-compose.yml", "docker-compose.yaml"}

// composeFile is the first regular file (after symlinks) DIR/NAME, "".
// It names the project in the headings and the JSON; Compose itself
// discovers the files of a project (DOCK-04).
func composeFile(dir string) string {
	for _, n := range composeNames {
		f := dir + "/" + n
		if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
			return f
		}
	}
	return ""
}

// composeArgv is the compose command of a project in DIR, no -f: Compose
// applies its own discovery there (compose.yaml precedence, override
// files, COMPOSE_FILE from the environment or DIR/.env), and the call
// runs with DIR as its working directory (LEDGER DOCK-04).
func (x *call) composeArgv(dir string, args ...string) []string {
	return append(append(x.composeCmd(), "--project-directory", dir), args...)
}

// runCompose runs a compose argv in dir.
func (x *call) runCompose(dir string, stdout, stderr io.Writer, argv []string) runner.Result {
	res, _ := x.m.Run.Run(x.ctx(), runner.Cmd{Path: argv[0], Args: argv[1:], Options: runner.Options{
		Env: x.Env.Child(), Dir: dir, Stdin: x.Stdin, Stdout: stdout, Stderr: stderr,
	}})
	return res
}

// dirname is GNU dirname: the string without its last component and the
// slashes before it; "." for none, "/" at the root.
func dirname(s string) string {
	t := strings.TrimRight(s, "/")
	if t == "" {
		if s == "" {
			return "."
		}
		return "/"
	}
	i := strings.LastIndexByte(t, '/')
	if i < 0 {
		return "."
	}
	if d := strings.TrimRight(t[:i], "/"); d != "" {
		return d
	}
	return "/"
}

// basename is GNU basename: the last component, trailing slashes off.
func basename(s string) string {
	t := strings.TrimRight(s, "/")
	if t == "" {
		if s == "" {
			return ""
		}
		return "/"
	}
	return t[strings.LastIndexByte(t, '/')+1:]
}

// hereLines are the lines `while read ...; done <<<"$VAR"` reads: one
// for an empty value, as the here-string adds a newline.
func hereLines(v string) []string { return strings.Split(v, "\n") }

// service is one service of a compose file with its image, kept whole
// (LEDGER DOCK-08).
type service struct{ name, image string }

// parsed is what parseServices read: the services with an image, whether
// they came from `docker compose config` (already interpolated), and the
// first service name that starts with '-' (refused, LEDGER DOCK-31).
type parsed struct {
	services   []service
	fromConfig bool
	badName    string
}

// parseServices reads the services of FILE: with Compose v2 on the PATH
// from `docker compose --project-directory DIR config --format json`,
// parsed natively (LEDGER DOCK-26: 2.x needed jq for this), in DIR; a
// project (check, upgrade) gets no -f, so the override files and
// COMPOSE_FILE apply (DOCK-04), a scanned file keeps -f FILE. Without v2,
// or when config fails, the YAML-subset reader. Rows without a name are
// skipped (DOCK-05).
func (x *call) parseServices(file string, project bool) parsed {
	if st, err := os.Stat(file); err != nil || !st.Mode().IsRegular() {
		return parsed{}
	}
	var out parsed
	if cmd := x.composeCmd(); len(cmd) == 2 {
		dir := dirname(file)
		argv := []string{"docker", "compose", "--project-directory", dir, "config", "--format", "json"}
		if !project {
			argv = []string{"docker", "compose", "-f", file, "--project-directory", dir, "config", "--format", "json"}
		}
		res := x.runCompose(dir, nil, io.Discard, argv)
		if res.ExitCode == 0 {
			if svcs, ok := configServices(res.Stdout); ok {
				out = parsed{services: svcs, fromConfig: true}
			}
		}
	}
	if !out.fromConfig {
		data, err := os.ReadFile(file)
		if err != nil {
			return parsed{}
		}
		out.services = lineParser(data)
	}
	kept := out.services[:0]
	for _, s := range out.services {
		if s.name == "" {
			continue
		}
		if strings.HasPrefix(s.name, "-") && out.badName == "" {
			out.badName = s.name
		}
		kept = append(kept, s)
	}
	out.services = kept
	return out
}

// checkEnvFile refuses DIR/.env when Compose would refuse it: a quoted
// value that never closes makes Compose reject the whole file, so gash
// does not go on with the definitions before it (F-09). ok is whether
// the file is usable (an absent one is); otherwise rc is the status.
func (x *call) checkEnvFile(envPath string, jsonOut bool) (rc int, ok bool) {
	_, err := dotenv(envPath, x.exported.Lookup)
	var e *dotenvError
	bad := errors.As(err, &e)
	if !bad {
		return 0, true
	}
	hint := "Close the quoted value of " + e.key + " that opens on line " + strconv.Itoa(e.line) +
		": Compose refuses the whole file"
	if jsonOut {
		return x.Fail(contract.New("invalid_env_file", e.path+":"+strconv.Itoa(e.line), hint)), false
	}
	x.p.Error("Cannot read " + e.path + ": " + e.Error())
	x.p.Hint(hint)
	return 1, false
}

// refuseBadName is the refusal of a service name that starts with '-'.
func (x *call) refuseBadName(name, file string, jsonOut bool) int {
	if jsonOut {
		return x.Fail(contract.New("invalid_service_name", name, "Rename the service in "+file+": a name cannot start with '-'"))
	}
	x.p.Error("Invalid service name '" + name + "' in " + file)
	return 1
}

// configServices is `jq -r '.services // {} | to_entries[] |
// select(.value.image != null) | "\(.key)|\(.value.image)"'` on the
// config output: one service per entry with an image, in the order of
// the JSON, NUL bytes dropped. ok is false where jq fails (not JSON,
// services or a service not an object), which sends 2.x to the
// YAML-subset reader.
func configServices(data []byte) ([]service, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var out []service
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return out, true
		}
		if err != nil {
			return nil, false
		}
		if tok == nil {
			continue // null: .services of null is null
		}
		if d, ok := tok.(json.Delim); !ok || d != '{' {
			return nil, false
		}
		if !topServices(dec, &out) {
			return nil, false
		}
	}
}

// topServices reads the members of the top object after its '{'.
func topServices(dec *json.Decoder, out *[]service) bool {
	var services json.RawMessage
	found := false
	for dec.More() {
		k, err := dec.Token()
		if err != nil {
			return false
		}
		var v json.RawMessage
		if err := dec.Decode(&v); err != nil {
			return false
		}
		if k == "services" {
			services, found = v, true
		}
	}
	if _, err := dec.Token(); err != nil {
		return false
	}
	if !found || string(bytes.TrimSpace(services)) == "null" || string(bytes.TrimSpace(services)) == "false" {
		return true // .services // {}
	}
	sd := json.NewDecoder(bytes.NewReader(services))
	sd.UseNumber()
	t, err := sd.Token()
	if err != nil {
		return false
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return false
	}
	noNUL := strings.NewReplacer("\x00", "")
	for sd.More() {
		k, err := sd.Token()
		if err != nil {
			return false
		}
		var v any
		if err := sd.Decode(&v); err != nil {
			return false
		}
		var img any
		switch vv := v.(type) {
		case nil:
			continue
		case map[string]any:
			img = vv["image"]
		default:
			return false
		}
		if img == nil {
			continue
		}
		*out = append(*out, service{noNUL.Replace(k.(string)), noNUL.Replace(jqText(img))})
	}
	return true
}

// jqText is a value as jq's string interpolation writes it.
func jqText(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	out, _ := json.Marshal(v)
	return string(out)
}

var (
	reServices = regexp.MustCompile(`^services:[ \t]*(#.*)?\r?$`)
	reBlank    = regexp.MustCompile(`^[ \t]*(#.*)?\r?$`)
	reComment  = regexp.MustCompile(`[ \t]+#.*$`)
	reImage    = regexp.MustCompile(`^ *image:`)
)

// lineParser is the YAML-subset reader of 2.x (an awk program): block
// services at any indentation, `image:` at the first child indentation,
// comments, blanks and quotes trimmed.
func lineParser(data []byte) []service {
	text := string(data)
	recs := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		recs = recs[:len(recs)-1]
	}
	var out []service
	inServices := false
	svcInd, childInd := -1, -1
	current := ""
	for _, rec := range recs {
		if reServices.MatchString(rec) {
			inServices, svcInd = true, -1
			continue
		}
		if inServices && rec != "" && !strings.ContainsRune(" \t#\r", rune(rec[0])) {
			inServices = false
		}
		if !inServices || reBlank.MatchString(rec) {
			continue
		}
		ind := len(rec) - len(strings.TrimLeft(rec, " "))
		if svcInd < 0 {
			svcInd = ind
		}
		if ind == svcInd {
			name := strings.TrimLeft(rec, " ")
			if i := strings.IndexByte(name, ':'); i >= 0 {
				name = name[:i]
			}
			current, childInd = unquote(name), -1
			continue
		}
		if current == "" || ind < svcInd {
			continue
		}
		if childInd < 0 {
			childInd = ind
		}
		if ind == childInd && reImage.MatchString(rec) {
			if img := unquote(reImage.ReplaceAllString(rec, "")); img != "" {
				out = append(out, service{strings.ReplaceAll(current, "\x00", ""), strings.ReplaceAll(img, "\x00", "")})
			}
		}
	}
	return out
}

// unquote is the awk unquote() with quotes read first (LEDGER DOCK-28): a
// value that starts with a quote ends at the matching quote, whatever
// follows; an unquoted value loses a " #..." comment and its trailing
// blanks or CR.
func unquote(s string) string {
	s = strings.TrimLeft(s, " \t")
	if s != "" && (s[0] == '"' || s[0] == '\'') {
		if i := strings.IndexByte(s[1:], s[0]); i >= 0 {
			return s[1 : 1+i]
		}
	}
	if loc := reComment.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	return strings.TrimRight(s, " \t\r")
}

// resolveEnvVars interpolates an image the YAML-subset reader returned
// with Compose's rules (interp.go, LEDGER DOCK-13): the caller's exported
// environment first, then ENV_FILE. A malformed or failing template
// leaves the value as it is, with a warning. The caller has refused an
// ENV_FILE that Compose would refuse (checkEnvFile).
func (x *call) resolveEnvVars(value, envPath string) string {
	file, _ := dotenv(envPath, x.exported.Lookup)
	get := func(n string) (string, bool) {
		if v, ok := x.exported.Lookup(n); ok {
			return v, true
		}
		v, ok := file[n]
		return v, ok
	}
	out, err := interpolate(value, get)
	if err != nil {
		x.p.Warning("Cannot interpolate " + value + ": " + err.Error())
		return value
	}
	return out
}

// image is a parsed image reference: the registry ("docker.io" for
// Docker Hub, whatever host spelling the reference used), the host and
// scheme to query, the repository, and the tag or the digest it is
// pinned to. bad is a reference Docker cannot parse.
type image struct {
	registry, host, scheme, name, tag, digest string
	bad                                       bool
}

// normalizeImage parses ref with go-containerregistry's name package
// (LEDGER DOCK-06, DOCK-07): docker.io, index.docker.io and
// registry-1.docker.io are Docker Hub, a single-segment name gets
// library/, a digest pin is kept (an image:tag@sha256:... reference is
// pinned to its digest), and localhost, loopback and private registries
// are asked over plain HTTP as Docker does.
func normalizeImage(ref string) image {
	r, err := name.ParseReference(ref)
	if err != nil {
		reg := "docker.io"
		if first, _, ok := strings.Cut(ref, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
			reg = first
		}
		return image{registry: reg, name: ref, bad: true}
	}
	ctx := r.Context()
	img := image{registry: ctx.RegistryStr(), host: ctx.RegistryStr(), scheme: ctx.Scheme(), name: ctx.RepositoryStr()}
	if img.registry == name.DefaultRegistry || img.registry == "registry-1.docker.io" || img.registry == "docker.io" {
		img.registry, img.host = "docker.io", "registry-1.docker.io"
	}
	switch v := r.(type) {
	case name.Digest:
		img.digest = v.DigestStr()
	case name.Tag:
		img.tag = v.TagStr()
	}
	return img
}

var (
	reMajor      = regexp.MustCompile(`^[0-9]+$`)
	reMajorMinor = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	rePinned     = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+`)
)

// isUpgradeable is __gash_is_upgradeable TAG: mutable names, major and
// major.minor versions, and suffixed tags by their base; digests and
// full versions are pinned; anything else is upgradeable.
func isUpgradeable(tag string) bool {
	if strings.HasPrefix(tag, "@sha256:") {
		return false
	}
	switch tag {
	case "latest", "main", "master", "dev", "develop", "edge", "nightly", "canary":
		return true
	}
	if reMajor.MatchString(tag) || reMajorMinor.MatchString(tag) {
		return true
	}
	if rePinned.MatchString(tag) {
		return false
	}
	if base, _, _ := strings.Cut(tag, "-"); base != tag {
		return isUpgradeable(base)
	}
	return true
}

var reDigest = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// localDigest is $(__gash_get_local_digest IMAGE): the RepoDigests entry
// of the image's repository from `docker image inspect` (images only, a
// container of the same name is never inspected, LEDGER DOCK-16; the
// reference after --, DOCK-31), else the first one, when it is a sha256
// digest.
func (x *call) localDigest(ref string) string {
	repo := ref
	if i := strings.LastIndexByte(repo, '@'); i >= 0 {
		repo = repo[:i]
	}
	if strings.Contains(repo[strings.LastIndexByte(repo, '/')+1:], ":") {
		repo = repo[:strings.LastIndexByte(repo, ':')]
	}
	digests, st := x.subst("docker", "image", "inspect", "--format", `{{range .RepoDigests}}{{.}}{{"\n"}}{{end}}`, "--", ref)
	if st != 0 {
		digests = ""
	}
	digest := ""
	for _, line := range hereLines(digests) {
		if line == "" {
			continue
		}
		head := line
		if i := strings.LastIndexByte(line, '@'); i >= 0 {
			head = line[:i]
		}
		_, after, _ := strings.Cut(line, "@")
		if !strings.Contains(line, "@") {
			after = line
		}
		if head == repo || head == "docker.io/"+repo || head == "docker.io/library/"+repo {
			digest = after
			break
		}
		if digest == "" {
			digest = after
		}
	}
	if !reDigest.MatchString(digest) {
		return ""
	}
	return digest
}
