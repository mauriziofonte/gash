// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

// Runtime resolution (plan S8, V2): a node major or a php major.minor is
// resolved to the binary that serves it, without nvm.sh and without a
// shell. `gash runtime`, `gash run --runtime`, llm_runtime, llm_tools and
// llm_lint share it.
//
// Where the interpreters come from:
//
//   - node, npm, npx: the nvm tree ${NVM_DIR:-$HOME/.nvm}/versions/node/
//     vX.Y.Z/bin (the version is the directory name, nothing runs), then
//     the node on PATH (its `node --version`) when no nvm version matches;
//   - php, composer: /usr/bin/phpX.Y and /usr/local/bin/phpX.Y (Debian,
//     Ubuntu and the ondrej packages), then the php on PATH; composer is
//     the composer on PATH, /usr/local/bin/composer or /usr/bin/composer,
//     run by the resolved php.
//
// A request names the leading components of a version ("24", "8.4",
// "8.4.1"): the newest installed version that starts with them wins.
//
// The cache ($XDG_CACHE_HOME/gash/runtimes.json) holds the directory
// listings, keyed by the mtime of the directory (the nvm versions
// directory, /usr/bin, /usr/local/bin), and the full version of each
// probed binary, keyed by its mtime and size: installing, removing or
// upgrading a runtime changes one of them, and the next call sees it.
// Only the gash commands write the cache (Resolver.Store); the llm_*
// functions read it and never write (plan R7).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// RuntimeNames are the runtimes a request may name, in help order.
var RuntimeNames = []string{"php", "node", "composer", "npm", "npx"}

// RuntimeFamily is the interpreter a runtime runs on: php for php and
// composer, node for node, npm and npx; "" for an unknown name.
func RuntimeFamily(runtime string) string {
	switch runtime {
	case "php", "composer":
		return "php"
	case "node", "npm", "npx":
		return "node"
	}
	return ""
}

// reRequest is a version request: one to three numeric components (a
// leading v is allowed and dropped).
var reRequest = lazyre.MustCompile(`^v?[0-9]{1,4}(\.[0-9]{1,4}){0,2}$`)

// NormalizeRequest returns the request without a leading v, and false
// when it is not one to three numeric components.
func NormalizeRequest(req string) (string, bool) {
	if !reRequest.MatchString(req) {
		return "", false
	}
	return strings.TrimPrefix(req, "v"), true
}

// Installed is one installed interpreter.
type Installed struct {
	Version string `json:"version"`
	Path    string `json:"path"`
}

// Resolution is the answer of a runtime request: the JSON of gash runtime
// and llm_runtime, keys in this order. Version is the version of the
// interpreter that matched the request (node for npm and npx, php for
// composer).
type Resolution struct {
	Runtime string `json:"runtime"`
	Request string `json:"request"`
	Path    string `json:"path"`
	Version string `json:"version"`
	// PHP is the php binary composer runs under (composer only).
	PHP string `json:"php,omitempty"`
	// Interp is the interpreter binary (node for npm and npx, php for
	// composer, the path itself otherwise), as the host sees it.
	Interp string `json:"-"`
}

// NotInstalledError is a request no installed runtime serves: the
// version is missing (Installed lists what there is), or Missing names
// the program that is (composer, npm, npx).
type NotInstalledError struct {
	Runtime   string
	Request   string
	Installed []string
	Missing   string
}

func (e *NotInstalledError) Error() string {
	if e.Missing != "" {
		return e.Missing + " not found for " + RuntimeFamily(e.Runtime) + " " + e.Request
	}
	have := "none"
	if len(e.Installed) > 0 {
		have = strings.Join(e.Installed, ", ")
	}
	return e.Runtime + " " + e.Request + " is not installed; installed: " + have
}

// Hint is the STOP hint of the error: how to install the runtime, and
// which versions a request can name instead.
func (e *NotInstalledError) Hint() string {
	fam := RuntimeFamily(e.Runtime)
	switch {
	case e.Missing == "composer":
		return "Install composer (getcomposer.org) on PATH or in /usr/local/bin; ask the user"
	case e.Missing != "":
		return "The node of this version has no " + e.Missing + "; reinstall it with nvm; ask the user"
	case len(e.Installed) > 0 && fam == "node":
		return "Ask the user to install it (nvm install " + e.Request + ") or request an installed version: " + strings.Join(e.Installed, ", ")
	case len(e.Installed) > 0:
		return "Ask the user to install php" + e.Request + " or request an installed version: " + strings.Join(e.Installed, ", ")
	case fam == "node":
		return "No node found (nvm tree or PATH); ask the user to install one (nvm install " + e.Request + ")"
	}
	return "No php found (/usr/bin/phpX.Y or PATH); ask the user to install php" + e.Request
}

// Resolver resolves runtimes for one call. The zero value is not usable:
// build it with NewResolver.
type Resolver struct {
	env invoke.Env
	sys platform.Sysroot
	run runner.Runner
	ctx context.Context
	// store writes the cache back on Save (the gash commands).
	store bool

	mu    sync.Mutex
	cache *runtimeCache
	dirty bool
}

// NewResolver returns a resolver for the caller's environment: HOME,
// NVM_DIR, PATH and XDG_CACHE_HOME come from env, the probes run through
// run. store says whether Save writes the cache.
func NewResolver(ctx context.Context, env invoke.Env, run runner.Runner, store bool) *Resolver {
	if ctx == nil {
		ctx = context.Background()
	}
	return &Resolver{env: env, sys: platform.Real(), run: run, ctx: ctx, store: store}
}

// WithSysroot maps the /usr/bin and /usr/local/bin probes through sys
// (tests).
func (r *Resolver) WithSysroot(sys platform.Sysroot) *Resolver {
	r.sys = sys
	return r
}

// candidate is one interpreter binary: its system path (what the answer
// shows), its path on the host (the same, except through a test
// sysroot), the version its name gives ("8.4" for php8.4, the nvm
// directory's version, "" for a plain php or node on PATH) and its full
// version once known.
type candidate struct {
	path    string
	host    string
	nameVer string
	fullVer string
}

// Installed lists the installed interpreters of a family ("php" or
// "node"), oldest first, each with its full version (php binaries are
// probed once, then cached).
func (r *Resolver) Installed(family string) []Installed {
	cands := r.candidates(family)
	if p := r.pathCandidate(family, cands); p != nil {
		cands = append(cands, *p)
	}
	versions := make([]string, len(cands))
	var wg sync.WaitGroup
	for i := range cands {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			versions[i] = r.fullVersion(&cands[i])
		}(i)
	}
	wg.Wait()
	out := make([]Installed, 0, len(cands))
	for i, c := range cands {
		if versions[i] == "" {
			continue
		}
		out = append(out, Installed{Version: versions[i], Path: c.path})
	}
	sort.SliceStable(out, func(i, j int) bool { return verCmp(out[i].Version, out[j].Version) < 0 })
	return out
}

// Resolve answers a request for runtime (one of RuntimeNames) and request
// (already normalized). The error is a *NotInstalledError.
func (r *Resolver) Resolve(runtime, request string) (Resolution, error) {
	fam := RuntimeFamily(runtime)
	c, err := r.resolveFamily(fam, runtime, request)
	if err != nil {
		return Resolution{}, err
	}
	res := Resolution{Runtime: runtime, Request: request, Path: c.path, Version: c.fullVer, Interp: c.host}
	switch runtime {
	case "npm", "npx":
		p := filepath.Join(filepath.Dir(c.path), runtime)
		if !isExecutable(filepath.Join(filepath.Dir(c.host), runtime)) {
			return Resolution{}, &NotInstalledError{Runtime: runtime, Request: request, Missing: runtime}
		}
		res.Path = p
	case "composer":
		p := r.composer()
		if p == "" {
			return Resolution{}, &NotInstalledError{Runtime: runtime, Request: request, Missing: "composer"}
		}
		res.Path, res.PHP = p, c.path
	}
	return res, nil
}

// resolveFamily picks the newest candidate that serves request.
func (r *Resolver) resolveFamily(fam, runtime, request string) (*candidate, error) {
	cands := r.candidates(fam)
	// Newest first by the version the name gives.
	sort.SliceStable(cands, func(i, j int) bool { return verCmp(cands[i].nameVer, cands[j].nameVer) > 0 })
	for i := range cands {
		c := &cands[i]
		switch versionMatch(request, c.nameVer) {
		case matchNo:
			continue
		case matchProbe:
			// The request is finer than the name (php 8.4.1, php8.4):
			// the full version decides.
			if versionMatch(request, r.fullVersion(c)) != matchYes {
				continue
			}
		}
		if r.fullVersion(c) == "" {
			continue
		}
		return c, nil
	}
	p := r.pathCandidate(fam, cands)
	if p != nil && versionMatch(request, r.fullVersion(p)) == matchYes {
		return p, nil
	}
	e := &NotInstalledError{Runtime: runtime, Request: request}
	seen := map[string]bool{}
	for _, c := range cands {
		if v := c.nameVer; v != "" && !seen[v] {
			seen[v] = true
			e.Installed = append(e.Installed, v)
		}
	}
	if p != nil {
		if v := r.fullVersion(p); v != "" && !seen[v] {
			e.Installed = append(e.Installed, v)
		}
	}
	e.Installed = sortV(e.Installed)
	return nil, e
}

type match int

const (
	matchNo match = iota
	matchYes
	matchProbe
)

// versionMatch reports whether version starts with the components of
// request: yes, no, or probe when request has more components than
// version (the name gives 8.4, the request is 8.4.1).
func versionMatch(request, version string) match {
	if version == "" {
		return matchNo
	}
	rq := strings.Split(request, ".")
	vs := strings.Split(version, ".")
	for i, p := range rq {
		if i >= len(vs) {
			return matchProbe
		}
		if atoi(p) != atoi(vs[i]) {
			return matchNo
		}
	}
	return matchYes
}

// rePHPName is a versioned php binary name: php8.4.
var rePHPName = lazyre.MustCompile(`^php[0-9]+\.[0-9]+$`)

// phpDirs are the system directories that hold versioned php binaries.
var phpDirs = []string{"/usr/bin", "/usr/local/bin"}

// candidates lists the versioned interpreters of a family (the nvm tree,
// the phpX.Y binaries); the plain one on PATH is pathCandidate.
func (r *Resolver) candidates(fam string) []candidate {
	var out []candidate
	switch fam {
	case "node":
		dir := filepath.Join(nvmDirEnv(r.env), "versions", "node")
		for _, name := range r.listDir(dir, false, reRelease.MatchString) {
			p := filepath.Join(dir, name, "bin", "node")
			if !isExecutable(p) {
				continue
			}
			v := strings.TrimPrefix(name, "v")
			out = append(out, candidate{path: p, host: p, nameVer: v, fullVer: v})
		}
	case "php":
		for _, dir := range phpDirs {
			for _, name := range r.listDir(dir, true, rePHPName.MatchString) {
				p := dir + "/" + name
				if !isExecutable(r.sys.Path(p)) {
					continue
				}
				out = append(out, candidate{path: p, host: r.sys.Path(p), nameVer: strings.TrimPrefix(name, "php")})
			}
		}
	}
	return out
}

// pathCandidate is the plain php or node on the caller's PATH, unless it
// is one of the versioned candidates (the same file).
func (r *Resolver) pathCandidate(fam string, cands []candidate) *candidate {
	p, err := runner.LookPath(fam, r.env.Child(), "")
	if err != nil {
		return nil
	}
	st, err := os.Stat(p)
	if err != nil {
		return nil
	}
	for _, c := range cands {
		if cs, err := os.Stat(c.host); err == nil && os.SameFile(st, cs) {
			return nil
		}
	}
	return &candidate{path: p, host: p}
}

// composer is the composer program: on PATH, else /usr/local/bin, else
// /usr/bin (php_detect_version's order); "" when there is none.
func (r *Resolver) composer() string {
	if p, err := runner.LookPath("composer", r.env.Child(), ""); err == nil {
		return p
	}
	for _, p := range []string{"/usr/local/bin/composer", "/usr/bin/composer"} {
		if isExecutable(r.sys.Path(p)) {
			return p
		}
	}
	return ""
}

// nvmDirEnv is ${NVM_DIR:-$HOME/.nvm} of an environment.
func nvmDirEnv(env invoke.Env) string {
	if d := env.Get("NVM_DIR"); d != "" {
		return d
	}
	return filepath.Join(env.Get("HOME"), ".nvm")
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}

// --- the cache ---

// runtimeCacheVersion changes when the file layout does.
const runtimeCacheVersion = 1

type runtimeCache struct {
	Version int                  `json:"version"`
	Dirs    map[string]dirEntry  `json:"dirs"`
	Bins    map[string]binaryVer `json:"bins"`
}

// dirEntry is a directory listing (the names a filter kept) and the
// directory's mtime when it was read.
type dirEntry struct {
	Mtime int64    `json:"mtime"`
	Names []string `json:"names"`
}

// binaryVer is the probed version of a binary and the mtime and size of
// the file it was probed on.
type binaryVer struct {
	Mtime   int64  `json:"mtime"`
	Size    int64  `json:"size"`
	Version string `json:"version"`
}

// CachePath is the runtime cache file of env, "" without a usable HOME.
func CachePath(env invoke.Env) string {
	d, err := platform.ResolveDirs(env.Get)
	if err != nil {
		return ""
	}
	return filepath.Join(d.CacheHome, "gash", "runtimes.json")
}

func (r *Resolver) loadCache() *runtimeCache {
	if r.cache != nil {
		return r.cache
	}
	r.cache = &runtimeCache{Version: runtimeCacheVersion, Dirs: map[string]dirEntry{}, Bins: map[string]binaryVer{}}
	p := CachePath(r.env)
	if p == "" {
		return r.cache
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return r.cache
	}
	var c runtimeCache
	if json.Unmarshal(data, &c) != nil || c.Version != runtimeCacheVersion {
		return r.cache
	}
	if c.Dirs == nil {
		c.Dirs = map[string]dirEntry{}
	}
	if c.Bins == nil {
		c.Bins = map[string]binaryVer{}
	}
	r.cache = &c
	return r.cache
}

// listDir returns the names of dir that keep accepts, sorted, from the
// cache when the directory's mtime is the cached one. sys maps dir
// through the sysroot.
func (r *Resolver) listDir(dir string, sys bool, keep func(string) bool) []string {
	host := dir
	if sys {
		host = r.sys.Path(dir)
	}
	st, err := os.Stat(host)
	if err != nil || !st.IsDir() {
		return nil
	}
	mtime := st.ModTime().UnixNano()
	r.mu.Lock()
	defer r.mu.Unlock()
	c := r.loadCache()
	if e, ok := c.Dirs[dir]; ok && e.Mtime == mtime {
		return e.Names
	}
	entries, err := os.ReadDir(host)
	if err != nil {
		return nil
	}
	names := []string{}
	for _, e := range entries {
		if keep(e.Name()) {
			names = append(names, e.Name())
		}
	}
	c.Dirs[dir] = dirEntry{Mtime: mtime, Names: names}
	r.dirty = true
	return names
}

// fullVersion returns the full version of c: the nvm directory's, or the
// binary's --version (cached by mtime and size); "" when the probe fails.
func (r *Resolver) fullVersion(c *candidate) string {
	if c.fullVer != "" {
		return c.fullVer
	}
	host := c.host
	st, err := os.Stat(host)
	if err != nil {
		return ""
	}
	key := host
	r.mu.Lock()
	cache := r.loadCache()
	if b, ok := cache.Bins[key]; ok && b.Mtime == st.ModTime().UnixNano() && b.Size == st.Size() {
		r.mu.Unlock()
		c.fullVer = b.Version
		return c.fullVer
	}
	r.mu.Unlock()
	v := ProbeVersion(r.ctx, r.run, r.env, host, "--version")
	if v == "" {
		return ""
	}
	r.mu.Lock()
	cache.Bins[key] = binaryVer{Mtime: st.ModTime().UnixNano(), Size: st.Size(), Version: v}
	r.dirty = true
	r.mu.Unlock()
	c.fullVer = v
	return v
}

// Save writes the cache when the resolver may store it and something
// changed. A failure is ignored: the cache only saves time.
func (r *Resolver) Save() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.store || !r.dirty || r.cache == nil {
		return
	}
	p := CachePath(r.env)
	if p == "" {
		return
	}
	data, err := json.Marshal(r.cache)
	if err != nil {
		return
	}
	if writeFileAtomic(p, data) == nil {
		r.dirty = false
	}
}

// writeFileAtomic writes data to p through a temporary file in the same
// directory (mode 0600, the directory 0700), so a reader never sees half a
// file.
func writeFileAtomic(p string, data []byte) error {
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".runtimes-*")
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(f.Name())
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(f.Name(), p); err != nil {
		_ = os.Remove(f.Name())
		return err
	}
	return nil
}

// reVersionToken is the first version-like token of a --version line.
var reVersionToken = lazyre.MustCompile(`[0-9]+(\.[0-9]+)+`)

// ProbeTimeout bounds one --version probe.
const ProbeTimeout = 10 * time.Second

// ProbeVersion runs `path ARGS...` with the caller's environment and
// returns the first version-like token (8.3.6, 22.1.0) of its output,
// stdout first, then stderr (ssh -V, java -version); "" when it fails or
// prints none.
func ProbeVersion(ctx context.Context, run runner.Runner, env invoke.Env, path string, args ...string) string {
	res, err := run.Run(ctx, runner.Cmd{Path: path, Args: args, Options: runner.Options{
		Env: env.Child(), Timeout: ProbeTimeout, KillAfter: time.Second,
	}})
	if err != nil && len(res.Stdout) == 0 && len(res.Stderr) == 0 {
		return ""
	}
	for _, out := range [][]byte{res.Stdout, res.Stderr} {
		for _, line := range strings.Split(string(out), "\n") {
			if v := reVersionToken.FindString(line); v != "" {
				return v
			}
		}
	}
	return ""
}

// BinDir is the directory gash run puts first on PATH for res, so that
// the plain name (node, npm and npx; php, and composer through its
// `#!/usr/bin/env php`) runs the resolved interpreter: the interpreter's
// own directory when its plain name there is that interpreter (the nvm
// bin directory, a node or php on PATH), else a directory of the cache
// holding one symlink, `php` to the resolved binary (/usr/bin/php8.4
// next to a /usr/bin/php that is another version).
func (r *Resolver) BinDir(res Resolution) (string, error) {
	plain := RuntimeFamily(res.Runtime)
	dir := filepath.Dir(res.Interp)
	if a, err := os.Stat(filepath.Join(dir, plain)); err == nil {
		if b, err := os.Stat(res.Interp); err == nil && os.SameFile(a, b) {
			return dir, nil
		}
	}
	p := CachePath(r.env)
	if p == "" {
		return "", errors.New("no cache directory for the " + plain + " link (HOME is not set)")
	}
	sum := sha256.Sum256([]byte(res.Interp))
	shim := filepath.Join(filepath.Dir(p), "runtime-bin", filepath.Base(res.Interp)+"-"+hex.EncodeToString(sum[:4]))
	link := filepath.Join(shim, plain)
	if t, err := os.Readlink(link); err == nil && t == res.Interp {
		return shim, nil
	}
	if err := os.MkdirAll(shim, 0o700); err != nil {
		return "", err
	}
	tmp := link + ".tmp-" + hex.EncodeToString(sum[4:8])
	_ = os.Remove(tmp)
	if err := os.Symlink(res.Interp, tmp); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, link); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return shim, nil
}
