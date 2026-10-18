// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/mauriziofonte/gash/internal/pathguard"
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

// resolveDir is `realpath -m "$path" 2>/dev/null || path="$PWD"`: the
// GNU realpath -m form of the argument (LEDGER DOCK-02), the caller's
// $PWD for the one argument it refuses, "" (DOCK-01, kept). GNU -m keeps
// a looping symlink as a plain name where the walk refuses it, so a path
// through a loop takes its lexical form and never the caller's $PWD.
func (x *call) resolveDir(p string) string {
	if p == "" {
		return x.pwd()
	}
	wd, _ := os.Getwd()
	g := pathguard.NewGuard(pathguard.OSFS{}, wd, x.Env.Get("HOME"), "")
	if r, err := g.Realpath(p, false); err == nil {
		return r
	}
	if r, err := g.Realpath(p, true); err == nil {
		return r
	}
	return x.pwd()
}

// composeNames are the file names 2.x looks for, in its order (LEDGER
// DOCK-29, kept).
var composeNames = []string{"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml"}

// composeFile is the first regular file (after symlinks) DIR/NAME, "".
func composeFile(dir string) string {
	for _, n := range composeNames {
		f := dir + "/" + n
		if st, err := os.Stat(f); err == nil && st.Mode().IsRegular() {
			return f
		}
	}
	return ""
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

// readFields is `IFS='|' read -r V1 ... Vn <<<"$line"`: one field per
// variable, the last one taking the rest of the line, which loses a
// single trailing '|' when it holds no other one.
func readFields(line string, n int) []string {
	out := make([]string, n)
	for i := 0; i < n-1; i++ {
		j := strings.IndexByte(line, '|')
		if j < 0 {
			out[i] = line
			return out
		}
		out[i], line = line[:j], line[j+1:]
	}
	if strings.HasSuffix(line, "|") && !strings.Contains(line[:len(line)-1], "|") {
		line = line[:len(line)-1]
	}
	out[n-1] = line
	return out
}

// hereLines are the lines `while read ...; done <<<"$VAR"` reads: one
// for an empty value, as the here-string adds a newline.
func hereLines(v string) []string { return strings.Split(v, "\n") }

// service is one "name|image" line of __gash_parse_compose_services.
type service struct{ name, image string }

// parseServices is $(__gash_parse_compose_services FILE): "name|image"
// lines. With Compose v2 on the PATH they come from `docker compose -f
// FILE --project-directory DIR config --format json`, parsed natively
// (LEDGER DOCK-26: 2.x needed jq for this); without it, or when config
// fails, from the YAML-subset reader ported rule for rule.
func (x *call) parseServices(file string) string {
	if st, err := os.Stat(file); err != nil || !st.Mode().IsRegular() {
		return ""
	}
	if cmd := x.composeCmd(); len(cmd) == 2 {
		res := x.run(nil, io.Discard, "docker", "compose", "-f", file, "--project-directory", dirname(file),
			"config", "--format", "json")
		if res.ExitCode == 0 {
			if out, ok := configServices(res.Stdout); ok {
				return strings.TrimRight(strings.ReplaceAll(out, "\x00", ""), "\n")
			}
		}
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	return strings.TrimRight(lineParser(data), "\n")
}

// configServices is `jq -r '.services // {} | to_entries[] |
// select(.value.image != null) | "\(.key)|\(.value.image)"'` on the
// config output: one line per service with an image, in the order of the
// JSON. ok is false where jq fails (not JSON, services or a service not
// an object), which sends 2.x to the YAML-subset reader.
func configServices(data []byte) (string, bool) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var b strings.Builder
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			return b.String(), true
		}
		if err != nil {
			return "", false
		}
		if tok == nil {
			continue // null: .services of null is null
		}
		if d, ok := tok.(json.Delim); !ok || d != '{' {
			return "", false
		}
		if !topServices(dec, &b) {
			return "", false
		}
	}
}

// topServices reads the members of the top object after its '{'.
func topServices(dec *json.Decoder, b *strings.Builder) bool {
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
		b.WriteString(k.(string) + "|" + jqText(img) + "\n")
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
// comments, blanks and quotes trimmed (LEDGER DOCK-28, kept).
func lineParser(data []byte) string {
	text := string(data)
	recs := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		recs = recs[:len(recs)-1]
	}
	var b strings.Builder
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
				b.WriteString(current + "|" + img + "\n")
			}
		}
	}
	return b.String()
}

// unquote is the awk unquote(): leading blanks, a " #..." comment and
// trailing blanks or CR go, then one pair of surrounding quotes.
func unquote(s string) string {
	s = strings.TrimLeft(s, " \t")
	if loc := reComment.FindStringIndex(s); loc != nil {
		s = s[:loc[0]]
	}
	s = strings.TrimRight(s, " \t\r")
	if len(s) >= 2 && (s[0] == '"' && s[len(s)-1] == '"' || s[0] == '\'' && s[len(s)-1] == '\'') {
		s = s[1 : len(s)-1]
	}
	return s
}

// envFile is the KEY=VALUE map of __gash_resolve_env_vars: lines that
// are not comments, one leading and one trailing quote of each kind
// stripped, the last definition winning (LEDGER DOCK-13, kept).
func envFile(path string) map[string]string {
	vars := map[string]string{}
	if st, err := os.Stat(path); err != nil || !st.Mode().IsRegular() {
		return vars
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return vars
	}
	text := strings.ReplaceAll(string(data), "\x00", "")
	recs := strings.Split(text, "\n")
	if strings.HasSuffix(text, "\n") {
		recs = recs[:len(recs)-1]
	}
	for _, line := range recs {
		if line == "" || line[0] == '#' {
			continue
		}
		m := reEnvLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		val := strings.TrimPrefix(m[2], `"`)
		val = strings.TrimSuffix(val, `"`)
		val = strings.TrimPrefix(val, `'`)
		val = strings.TrimSuffix(val, `'`)
		vars[m[1]] = val
	}
	return vars
}

var (
	reEnvLine = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)=(.*)$`)
	reVarRef  = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)((:?-)([^}]*))?\}`)
	reEchoOpt = regexp.MustCompile(`^-[neE]+$`)
)

// resolveEnvVars is $(__gash_resolve_env_vars VALUE ENV_FILE): every
// ${VAR}, ${VAR:-default} and ${VAR-default} (both as :-) replaced by
// the non-empty value from ENV_FILE, else from the caller's exported
// environment, else the default, at most 50 rounds (then the warning).
// Replacement is literal (LEDGER DOCK-12); the rules are 2.x's (DOCK-13).
func (x *call) resolveEnvVars(value, envPath string) string {
	vars := envFile(envPath)
	result := value
	for i := 0; ; i++ {
		m := reVarRef.FindStringSubmatch(result)
		if m == nil {
			break
		}
		if i >= 50 {
			// Printed from inside a command substitution: stdout is not
			// the terminal there, so the plain form.
			plain := x.p
			plain.Color = false
			plain.Warning("Unresolvable variable reference (circular?) in: " + value)
			break
		}
		resolved := vars[m[1]]
		if resolved == "" {
			resolved = x.exported.Get(m[1])
		}
		if resolved == "" {
			resolved = m[4]
		}
		result = strings.ReplaceAll(result, m[0], resolved)
	}
	// echo "$result", in a command substitution.
	if reEchoOpt.MatchString(result) {
		return ""
	}
	return strings.TrimRight(result, "\n")
}

// image is the registry|name|tag of __gash_normalize_image, as `IFS='|'
// read -r registry name tag` splits it.
type image struct{ registry, name, tag string }

// normalizeImage is __gash_normalize_image ported rule for rule: a
// digest after the last '@', a tag after the last ':' of the last path
// segment (none when an '@' is left, so image:tag@sha256:... keeps its
// tag, LEDGER DOCK-06, kept), a first segment with '.' or ':' or
// "localhost" as the registry, library/ for single names (DOCK-07, kept).
func normalizeImage(ref string) image {
	registry, name, tag := "docker.io", "", "latest"
	img := ref
	if strings.Contains(img, "@sha256:") {
		i := strings.LastIndexByte(img, '@')
		tag = "@" + img[i+1:]
		img = img[:i]
	}
	lastSeg := img[strings.LastIndexByte(img, '/')+1:]
	if strings.Contains(lastSeg, ":") && !strings.Contains(img, "@") {
		i := strings.LastIndexByte(img, ':')
		tag, img = img[i+1:], img[:i]
	}
	if strings.Contains(img, "/") {
		first := img[:strings.IndexByte(img, '/')]
		if strings.Contains(first, ".") || strings.Contains(first, ":") || first == "localhost" {
			registry, name = first, img[strings.IndexByte(img, '/')+1:]
		} else {
			name = img
		}
	} else {
		name = "library/" + img
	}
	f := readFields(registry+"|"+name+"|"+tag, 3)
	return image{f[0], f[1], f[2]}
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
// of the image's repository from `docker inspect` (no --type image,
// LEDGER DOCK-16, kept; the reference in option position, DOCK-31,
// kept), else the first one, when it is a sha256 digest.
func (x *call) localDigest(ref string) string {
	repo := ref
	if i := strings.LastIndexByte(repo, '@'); i >= 0 {
		repo = repo[:i]
	}
	if strings.Contains(repo[strings.LastIndexByte(repo, '/')+1:], ":") {
		repo = repo[:strings.LastIndexByte(repo, ':')]
	}
	digests, st := x.subst("docker", "inspect", "--format", `{{range .RepoDigests}}{{.}}{{"\n"}}{{end}}`, ref)
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
