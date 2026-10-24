// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

// Fixtures of the upgrade scripts (docs/port/design/upgrade.md 6): an
// origin repository with Gash trees under local tags, and a file://
// release directory holding release builds of this checkout.
//
//	gashfixture DIR TAG:KIND...
//	    makes DIR a git repository with one commit and annotated tag per
//	    TAG, in order. KIND 3x is the staged 3.x tree (the generated Bash,
//	    without the Go sources and tests) with VERSION set to TAG minus its
//	    v; KIND 2x is the v2.1.0 tree of this repository (condition
//	    [v2tree]). The work tree is left at the last tag.
//	gashrelease [-stamp V] [-pin FILE] VERSION DIR
//	    builds ./cmd/gash as .goreleaser.yaml does, stamped V (default
//	    VERSION), and adds gash_VERSION_OS_ARCH.tar.gz to DIR with its line
//	    in DIR/checksums.txt. -pin writes the binary's release/SHA256SUMS
//	    line ("HASH  OS_ARCH/gash") to FILE.

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/rogpeppe/go-internal/testscript"
)

// fixtures holds what the commands share across scripts: the checkout,
// the staged tree, the extracted 2.x tree and the builds by stamp.
type fixtures struct {
	root, stage, cache string

	tree2Once sync.Once
	tree2     string
	tree2Err  error

	mu     sync.Mutex
	builds map[string]*gashBuild
}

type gashBuild struct {
	once sync.Once
	path string
	err  error
}

// tree3Skip are the top-level entries of the staged tree a release tree
// does not need (Go sources, tests, docs, the staged binary), and the
// checkout's release/SHA256SUMS: it pins the checkout's own release
// build, not the fixture's (a script that wants a pin writes it with
// gashrelease -pin).
var tree3Skip = map[string]bool{
	".gash-stage": true, ".github": true, "internal": true, "cmd": true, "testdata": true,
	"docs": true, "bench": true, "tests": true, "compat": true, "go.mod": true, "go.sum": true,
	"release": true,
}

// v2Tree extracts git archive v2.1.0 once; "" when the checkout has no
// such tag (a shallow CI checkout).
func (f *fixtures) v2Tree() (string, error) {
	f.tree2Once.Do(func() {
		if exec.Command("git", "-C", f.root, "rev-parse", "-q", "--verify", "refs/tags/v2.1.0^{commit}").Run() != nil {
			return
		}
		out, err := exec.Command("git", "-C", f.root, "archive", "--format=tar", "v2.1.0").Output()
		if err != nil {
			f.tree2Err = fmt.Errorf("git archive v2.1.0: %w", err)
			return
		}
		dir := filepath.Join(f.cache, "tree2")
		if err := untar(bytes.NewReader(out), dir); err != nil {
			f.tree2Err = err
			return
		}
		f.tree2 = dir
	})
	return f.tree2, f.tree2Err
}

func untar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		p := filepath.Join(dir, filepath.FromSlash(h.Name))
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			b, err := io.ReadAll(tr)
			if err != nil {
				return err
			}
			if err := os.WriteFile(p, b, fs.FileMode(h.Mode)&0o777); err != nil {
				return err
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, p); err != nil {
				return err
			}
		}
	}
}

// copyTree copies src into dst, leaving out the top-level names of skip.
func copyTree(src, dst string, skip map[string]bool) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		if rel == "." {
			return nil
		}
		if skip[strings.SplitN(filepath.ToSlash(rel), "/", 2)[0]] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		out := filepath.Join(dst, rel)
		switch {
		case d.IsDir():
			return os.MkdirAll(out, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			t, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(t, out)
		case d.Type().IsRegular():
			info, err := d.Info()
			if err != nil {
				return err
			}
			b, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(out, b, info.Mode().Perm())
		}
		return nil
	})
}

// gitEnv is the environment of the fixture's git commands: the script's
// HOME and PATH, a fixed identity, no system or global signing.
func gitEnv(ts *testscript.TestScript) []string {
	return []string{"HOME=" + ts.Getenv("HOME"), "PATH=" + ts.Getenv("PATH"), "LANG=C.UTF-8", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t"}
}

func runGit(ts *testscript.TestScript, dir string, args ...string) {
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "tag.gpgsign=false",
		"-c", "init.defaultBranch=main", "-c", "advice.detachedHead=false"}, args...)...)
	cmd.Dir = dir
	cmd.Env = gitEnv(ts)
	if out, err := cmd.CombinedOutput(); err != nil {
		ts.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func (f *fixtures) cmdFixture(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) < 2 {
		ts.Fatalf("usage: gashfixture DIR TAG:KIND...")
	}
	dir := ts.MkAbs(args[0])
	ts.Check(os.MkdirAll(dir, 0o755))
	runGit(ts, dir, "init", "-q")
	for _, spec := range args[1:] {
		tag, kind, _ := strings.Cut(spec, ":")
		entries, err := os.ReadDir(dir)
		ts.Check(err)
		for _, e := range entries {
			if e.Name() != ".git" {
				ts.Check(os.RemoveAll(filepath.Join(dir, e.Name())))
			}
		}
		switch kind {
		case "3x":
			ts.Check(copyTree(f.stage, dir, tree3Skip))
			ts.Check(os.WriteFile(filepath.Join(dir, "VERSION"), []byte(strings.TrimPrefix(tag, "v")+"\n"), 0o644))
		case "2x":
			t2, err := f.v2Tree()
			ts.Check(err)
			if t2 == "" {
				ts.Fatalf("gashfixture: no v2.1.0 tag in this checkout (guard the script with [v2tree])")
			}
			ts.Check(copyTree(t2, dir, nil))
		default:
			ts.Fatalf("gashfixture: unknown kind %q (2x or 3x)", kind)
		}
		runGit(ts, dir, "add", "-A")
		runGit(ts, dir, "commit", "-q", "-m", tag)
		runGit(ts, dir, "tag", "-a", tag, "-m", tag)
	}
}

// platformName is the OS_ARCH of the release assets.
func platformName() string { return runtime.GOOS + "_" + runtime.GOARCH }

func (f *fixtures) binary(stamp string) (string, error) {
	f.mu.Lock()
	b := f.builds[stamp]
	if b == nil {
		b = &gashBuild{}
		f.builds[stamp] = b
	}
	f.mu.Unlock()
	b.once.Do(func() {
		b.path = filepath.Join(f.cache, "build", stamp, "gash")
		cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false",
			"-ldflags", "-s -w -X github.com/mauriziofonte/gash/internal/buildinfo.Version="+stamp, "-o", b.path, "./cmd/gash")
		cmd.Dir = f.root
		cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=")
		if out, err := cmd.CombinedOutput(); err != nil {
			b.err = fmt.Errorf("go build (version %s): %v\n%s", stamp, err, out)
		}
	})
	return b.path, b.err
}

func (f *fixtures) cmdRelease(ts *testscript.TestScript, neg bool, args []string) {
	if neg {
		ts.Fatalf("usage: gashrelease [-stamp V] [-pin FILE] VERSION DIR")
	}
	stamp, pin := "", ""
	for len(args) > 1 && strings.HasPrefix(args[0], "-") {
		switch args[0] {
		case "-stamp":
			stamp = args[1]
		case "-pin":
			pin = ts.MkAbs(args[1])
		default:
			ts.Fatalf("gashrelease: unknown flag %s", args[0])
		}
		args = args[2:]
	}
	if len(args) != 2 {
		ts.Fatalf("usage: gashrelease [-stamp V] [-pin FILE] VERSION DIR")
	}
	version, dir := args[0], ts.MkAbs(args[1])
	if stamp == "" {
		stamp = version
	}
	bin, err := f.binary(stamp)
	ts.Check(err)
	data, err := os.ReadFile(bin)
	ts.Check(err)

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	ts.Check(tw.WriteHeader(&tar.Header{Name: "gash", Mode: 0o755, Size: int64(len(data)), Typeflag: tar.TypeReg}))
	_, err = tw.Write(data)
	ts.Check(err)
	ts.Check(tw.Close())
	ts.Check(gz.Close())

	ts.Check(os.MkdirAll(dir, 0o755))
	asset := "gash_" + version + "_" + platformName() + ".tar.gz"
	ts.Check(os.WriteFile(filepath.Join(dir, asset), buf.Bytes(), 0o644))
	var lines []string
	if old, err := os.ReadFile(filepath.Join(dir, "checksums.txt")); err == nil {
		for _, l := range strings.Split(strings.TrimSpace(string(old)), "\n") {
			if l != "" && !strings.HasSuffix(l, "  "+asset) {
				lines = append(lines, l)
			}
		}
	}
	lines = append(lines, hexSHA256(buf.Bytes())+"  "+asset)
	ts.Check(os.WriteFile(filepath.Join(dir, "checksums.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	if pin != "" {
		ts.Check(os.MkdirAll(filepath.Dir(pin), 0o755))
		ts.Check(os.WriteFile(pin, []byte(hexSHA256(data)+"  "+platformName()+"/gash\n"), 0o644))
	}
}

func hexSHA256(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
