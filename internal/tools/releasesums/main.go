// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command releasesums pins the release binaries in the tagged tree
// (design decision D-3, docs/port/design/upgrade.md 2): it builds `gash`
// for every release platform the way .goreleaser.yaml does and writes
// their SHA-256 to release/SHA256SUMS, one "HASH  OS_ARCH/gash" line per
// platform, which the compat bootstrap and gash_doctor check the binary
// against.
//
//	go run ./internal/tools/releasesums            # write release/SHA256SUMS
//	go run ./internal/tools/releasesums -check     # status 1 when it is stale
//
// The build is reproducible: CGO off, -trimpath, -buildvcs=false (the
// commit that holds the sums cannot be stamped into the binaries it pins),
// -ldflags "-s -w" plus the version, and the default GOAMD64/GOARM64
// levels goreleaser sets. The goreleaser hooks call the other modes:
//
//	-check-version V   V is the VERSION file (the tag being released)
//	-verify PATH -platform OS_ARCH
//	                   the binary goreleaser built matches its pin
//	-compare DIST      rebuild each binary of DIST/artifacts.json with the
//	                   version of DIST/metadata.json and require the same
//	                   bytes (CI proves the two builds agree on snapshots)
//
// -snapshot=true turns -check-version and -verify into no-ops, so the hooks
// can pass goreleaser's {{ .IsSnapshot }}.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// platforms are the release targets of .goreleaser.yaml (goos x goarch).
var platforms = []string{"darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"}

const (
	sumsPath   = "release/SHA256SUMS"
	versionVar = "github.com/mauriziofonte/gash/internal/buildinfo.Version"
)

func main() {
	check := flag.Bool("check", false, "report only; status 1 when release/SHA256SUMS is missing or stale")
	checkVersion := flag.String("check-version", "", "require the VERSION file to hold this version")
	verify := flag.String("verify", "", "a built binary to check against its pin")
	platform := flag.String("platform", "", "the OS_ARCH of -verify")
	compare := flag.String("compare", "", "a goreleaser dist directory to rebuild and compare")
	snapshot := flag.Bool("snapshot", false, "goreleaser snapshot: -check-version and -verify do nothing")
	flag.Parse()
	var err error
	switch {
	case *checkVersion != "":
		if !*snapshot {
			err = requireVersion(*checkVersion)
		}
	case *verify != "":
		if !*snapshot {
			err = verifyBinary(*verify, *platform)
		}
	case *compare != "":
		err = compareDist(*compare)
	default:
		err = writeSums(*check, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "releasesums:", err)
		os.Exit(1)
	}
}

// fileVersion is the VERSION file, trimmed.
func fileVersion() (string, error) {
	b, err := os.ReadFile("VERSION")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func requireVersion(v string) error {
	want, err := fileVersion()
	if err != nil {
		return err
	}
	if v != want {
		return fmt.Errorf("releasing %s but VERSION is %s", v, want)
	}
	return nil
}

// build compiles ./cmd/gash for platform with version stamped in and
// returns the binary's SHA-256.
func build(dir, platform, version string) (string, error) {
	goos, goarch, _ := strings.Cut(platform, "_")
	out := filepath.Join(dir, platform, "gash")
	cmd := exec.Command("go", "build", "-trimpath", "-buildvcs=false",
		"-ldflags", "-s -w -X "+versionVar+"="+version, "-o", out, "./cmd/gash")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOFLAGS=", "GOOS="+goos, "GOARCH="+goarch, "GOAMD64=v1", "GOARM64=v8.0")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("build %s: %w", platform, err)
	}
	return fileSum(out)
}

func fileSum(path string) (string, error) {
	f, err := os.Open(path)
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

func writeSums(check bool, stdout io.Writer) error {
	version, err := fileVersion()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "releasesums")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	var b strings.Builder
	for _, p := range platforms {
		sum, err := build(tmp, p, version)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s/gash\n", sum, p)
	}
	if check {
		have, err := os.ReadFile(sumsPath)
		if err != nil {
			return err
		}
		if string(have) != b.String() {
			return fmt.Errorf("%s does not pin the binaries of this tree (VERSION %s); run go run ./internal/tools/releasesums", sumsPath, version)
		}
		fmt.Fprintf(stdout, "%s pins this tree (VERSION %s)\n", sumsPath, version)
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(sumsPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(sumsPath, []byte(b.String()), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "wrote %s for VERSION %s\n", sumsPath, version)
	return nil
}

// pins reads release/SHA256SUMS as OS_ARCH -> hash.
func pins() (map[string]string, error) {
	b, err := os.ReadFile(sumsPath)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		sum, name, ok := strings.Cut(line, "  ")
		if !ok || !strings.HasSuffix(name, "/gash") {
			return nil, fmt.Errorf("%s: bad line %q", sumsPath, line)
		}
		m[strings.TrimSuffix(name, "/gash")] = sum
	}
	return m, nil
}

func verifyBinary(path, platform string) error {
	want, err := pins()
	if err != nil {
		return err
	}
	if want[platform] == "" {
		return fmt.Errorf("%s pins no binary for %s", sumsPath, platform)
	}
	got, err := fileSum(path)
	if err != nil {
		return err
	}
	if got != want[platform] {
		return fmt.Errorf("%s (%s) is %s, %s pins %s", path, platform, got, sumsPath, want[platform])
	}
	return nil
}

// compareDist rebuilds every gash binary goreleaser listed in
// DIST/artifacts.json and requires identical bytes.
func compareDist(dist string) error {
	var meta struct {
		Version string `json:"version"`
	}
	if err := readJSON(filepath.Join(dist, "metadata.json"), &meta); err != nil {
		return err
	}
	var arts []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		Goos   string `json:"goos"`
		Goarch string `json:"goarch"`
		Type   string `json:"type"`
	}
	if err := readJSON(filepath.Join(dist, "artifacts.json"), &arts); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "releasesums")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	n := 0
	for _, a := range arts {
		if a.Type != "Binary" || a.Name != "gash" {
			continue
		}
		p := a.Goos + "_" + a.Goarch
		mine, err := build(tmp, p, meta.Version)
		if err != nil {
			return err
		}
		theirs, err := fileSum(a.Path)
		if err != nil {
			return err
		}
		if mine != theirs {
			return fmt.Errorf("%s: goreleaser built %s, this tool %s", p, theirs, mine)
		}
		n++
	}
	if n != len(platforms) {
		return fmt.Errorf("found %d gash binaries in %s, want %d", n, dist, len(platforms))
	}
	fmt.Printf("goreleaser and releasesums build the same %d binaries (version %s)\n", n, meta.Version)
	return nil
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
