// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// The release binaries the bootstrap installs (lib/compat/bootstrap.sh
// __gash_fetch) and gash update installs the same way.
const (
	binGash   = "gash"
	binHelper = "gash-hosts-helper"
	// releaseBase is the default GASH_RELEASE_URL, followed by /v<VERSION>.
	releaseBase = "https://github.com/mauriziofonte/gash/releases/download"
	// maxAsset bounds one download.
	maxAsset = 512 << 20
)

// Fetch timings, as the bootstrap's: curl --max-time 120, a lock older
// than 10 minutes is stale, 120 seconds of waiting for another fetch.
var (
	fetchTimeout = 120 * time.Second
	lockStale    = 10 * time.Minute
	lockWait     = 120 * time.Second
	lockPoll     = time.Second
)

// release is the binary set of one Gash version for this platform, as the
// tree of its tag describes it.
type release struct {
	version string
	// pins are the SHA-256 that the tree's release/SHA256SUMS holds for
	// this platform, by binary name ("gash", "gash-hosts-helper").
	pins map[string]string
}

// platform is the OS_ARCH of the release assets, "" when Gash ships none
// for this system.
func platform() string {
	switch runtime.GOOS + "/" + runtime.GOARCH {
	case "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64":
		return runtime.GOOS + "_" + runtime.GOARCH
	}
	return ""
}

// parsePins reads release/SHA256SUMS ("HASH  OS_ARCH/NAME" lines) for
// the binaries of plat.
func parsePins(sums, plat string) map[string]string {
	pins := map[string]string{}
	for _, line := range strings.Split(sums, "\n") {
		hash, name, ok := strings.Cut(strings.TrimRight(line, "\r"), "  ")
		if !ok {
			continue
		}
		for _, b := range []string{binGash, binHelper} {
			if name == plat+"/"+b {
				pins[b] = hash
			}
		}
	}
	return pins
}

// dataDir is ${XDG_DATA_HOME:-$HOME/.local/share}/gash, as the bootstrap
// resolves it (any non-empty XDG_DATA_HOME).
func dataDir(c *invoke.Call) string {
	if x := c.Env.Get("XDG_DATA_HOME"); x != "" {
		return x + "/gash"
	}
	return c.Env.Get("HOME") + "/.local/share/gash"
}

// stateDir is ${XDG_STATE_HOME:-$HOME/.local/state}/gash.
func stateDir(c *invoke.Call) string {
	if x := c.Env.Get("XDG_STATE_HOME"); x != "" {
		return x + "/gash"
	}
	return c.Env.Get("HOME") + "/.local/state/gash"
}

// versionDir is where the binaries of version v live.
func versionDir(c *invoke.Call, v string) string { return dataDir(c) + "/versions/" + v }

// fetchError is why a binary could not be installed, worded as the
// bootstrap's reasons are.
type fetchError struct{ reason string }

func (e *fetchError) Error() string { return e.reason }

func failf(format string, args ...any) error { return &fetchError{fmt.Sprintf(format, args...)} }

// installBinary makes versions/<v>/gash the verified binary of rel and
// reports how it was verified ("" when the binary was already installed
// and checked). It follows __gash_fetch: the binary pinned in the tree
// (release/SHA256SUMS), else the archive listed in the release's
// checksums.txt; GASH_REQUIRE_SIGNED=1 accepts only a pinned binary. The
// gash-hosts-helper of the archive is installed only when it is verified
// too: by its own pin, else by checksums.txt. Then the binary must answer
// `gash version --json` with v; a binary that does not is removed.
func (m *Module) installBinary(c *invoke.Call, rel release) (string, error) {
	plat := platform()
	if plat == "" {
		return "", failf("no release for %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	v := rel.version
	dir := versionDir(c, v)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", failf("cannot create %s", dir)
	}
	unlock, err := m.lock(c, dir+"/.lock")
	if err != nil {
		return "", err
	}
	defer unlock()

	bin := dir + "/" + binGash
	if isExecutable(bin) {
		if pin := rel.pins[binGash]; pin == "" || fileSHA256(bin) == pin {
			if m.binaryVersion(c, bin) == v {
				return "", nil
			}
		}
		// A binary that is not the one of v is replaced.
		_ = os.Remove(bin)
	}
	how, err := m.download(c, rel, plat, dir)
	if err != nil {
		return "", err
	}
	if got := m.binaryVersion(c, bin); got != v {
		_ = os.Remove(bin)
		if got == "" {
			return "", failf("the downloaded binary does not answer gash version --json")
		}
		return "", failf("the downloaded binary is version %s, not %s", got, v)
	}
	return how, nil
}

// download fetches and verifies the archive of rel into dir (through a
// temporary directory that is always removed).
func (m *Module) download(c *invoke.Call, rel release, plat, dir string) (string, error) {
	v := rel.version
	asset := "gash_" + v + "_" + plat + ".tar.gz"
	base := c.Env.Get("GASH_RELEASE_URL")
	if base == "" {
		base = releaseBase + "/v" + v
	}
	base = strings.TrimRight(base, "/")
	pin, helperPin := rel.pins[binGash], rel.pins[binHelper]
	signed := c.Env.Get("GASH_REQUIRE_SIGNED") == "1"
	if pin == "" && signed {
		return "", failf("this tree pins no binary for %s (GASH_REQUIRE_SIGNED=1)", strings.Replace(plat, "_", "/", 1))
	}

	tmp := dir + "/.tmp." + strconv.Itoa(os.Getpid())
	_ = os.RemoveAll(tmp)
	if err := os.Mkdir(tmp, 0o700); err != nil {
		return "", failf("cannot create %s", tmp)
	}
	defer os.RemoveAll(tmp)

	if err := m.get(ctxOf(c), base+"/"+asset, tmp+"/"+asset); err != nil {
		return "", failf("download failed: %s/%s", base, asset)
	}
	// The archive's checksum: required without a pin; for the helper only
	// when the tree pins the gash binary but not the helper (and never
	// with GASH_REQUIRE_SIGNED=1, which trusts only the tree).
	archiveOK := false
	if pin == "" || (helperPin == "" && !signed) {
		err := m.get(ctxOf(c), base+"/checksums.txt", tmp+"/checksums.txt")
		switch {
		case err != nil && pin == "":
			return "", failf("download failed: %s/checksums.txt", base)
		case err == nil:
			want := checksumOf(tmp+"/checksums.txt", asset)
			if want == "" || fileSHA256(tmp+"/"+asset) != want {
				return "", failf("checksum mismatch for %s", asset)
			}
			archiveOK = true
		}
	}
	got, err := extract(tmp+"/"+asset, tmp, binGash, binHelper)
	if err != nil || !got[binGash] {
		return "", failf("cannot extract %s", asset)
	}
	if pin != "" && fileSHA256(tmp+"/"+binGash) != pin {
		return "", failf("the binary does not match release/SHA256SUMS")
	}
	helper := false
	if got[binHelper] {
		switch {
		case helperPin != "":
			if fileSHA256(tmp+"/"+binHelper) != helperPin {
				return "", failf("gash-hosts-helper does not match release/SHA256SUMS")
			}
			helper = true
		case archiveOK:
			helper = true
		}
	}
	if helper {
		if err := os.Rename(tmp+"/"+binHelper, dir+"/"+binHelper); err != nil {
			return "", failf("cannot install into %s", dir)
		}
	}
	if err := os.Rename(tmp+"/"+binGash, dir+"/"+binGash); err != nil {
		return "", failf("cannot install into %s", dir)
	}
	if pin != "" {
		return "release/SHA256SUMS", nil
	}
	return "checksums.txt", nil
}

// lock takes the fetch lock of a version directory, mkdir DIR/.lock as the
// bootstrap takes it: a lock older than lockStale is removed, another one
// is waited for up to lockWait.
func (m *Module) lock(c *invoke.Call, path string) (func(), error) {
	deadline := time.Now().Add(lockWait)
	for {
		err := os.Mkdir(path, 0o755)
		if err == nil {
			return func() { _ = os.Remove(path) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, failf("cannot create %s", path)
		}
		if st, serr := os.Lstat(path); serr == nil && time.Since(st.ModTime()) > lockStale {
			_ = os.Remove(path)
			continue
		}
		if time.Now().After(deadline) {
			return nil, failf("another download holds %s", path)
		}
		select {
		case <-ctxOf(c).Done():
			return nil, failf("interrupted")
		case <-time.After(lockPoll):
		}
	}
}

// get downloads url to dst: file:// reads a local file, http(s) goes
// through net/http with the proxies of the environment.
func (m *Module) get(ctx context.Context, rawURL, dst string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	var body io.ReadCloser
	switch u.Scheme {
	case "file":
		f, err := os.Open(u.Path)
		if err != nil {
			return err
		}
		body = f
	case "http", "https":
		ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		client := m.http
		if client == nil {
			client = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment}}
		}
		res, err := client.Do(req)
		if err != nil {
			return err
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			return fmt.Errorf("HTTP %d", res.StatusCode)
		}
		body = res.Body
	default:
		return fmt.Errorf("unsupported URL %s", rawURL)
	}
	defer body.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	n, err := io.Copy(out, io.LimitReader(body, maxAsset+1))
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > maxAsset {
		err = errors.New("too large")
	}
	return err
}

// checksumOf is the hash checksums.txt lists for asset ("HASH  ASSET" or
// "HASH *ASSET"); "" when it lists none.
func checksumOf(path, asset string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	want := ""
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.HasSuffix(line, "  "+asset) || strings.HasSuffix(line, " *"+asset) {
			want, _, _ = strings.Cut(line, " ")
		}
	}
	return strings.ToLower(want)
}

// extract writes the regular files names of the archive's root into dir
// with mode 0755 and reports which it found.
func extract(archive, dir string, names ...string) (map[string]bool, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	tr := tar.NewReader(gz)
	found := map[string]bool{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return found, nil
		}
		if err != nil {
			return nil, err
		}
		name := strings.TrimPrefix(h.Name, "./")
		want := false
		for _, n := range names {
			want = want || name == n
		}
		if !want || h.Typeflag != tar.TypeReg || found[name] {
			continue
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o755)
		if err != nil {
			return nil, err
		}
		_, err = io.Copy(out, io.LimitReader(tr, maxAsset))
		if cerr := out.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return nil, err
		}
		if err := os.Chmod(filepath.Join(dir, name), 0o755); err != nil {
			return nil, err
		}
		found[name] = true
	}
}

// binaryVersion is the version a gash binary reports in `gash version
// --json`; "" when it does not answer.
func (m *Module) binaryVersion(c *invoke.Call, bin string) string {
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: bin, Args: []string{"version", "--json"},
		Options: runner.Options{Env: c.Env.Child(), Timeout: 30 * time.Second}})
	if err != nil || !res.Success() {
		return ""
	}
	var v struct {
		Version string `json:"version"`
	}
	if json.Unmarshal(res.Stdout, &v) != nil {
		return ""
	}
	return v.Version
}

func isExecutable(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}
