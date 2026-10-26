// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
)

// TestBashFunctions runs the generated Bash on fake systems and checks that
// it names the manager and the install command the Go side names.
func TestBashFunctions(t *testing.T) {
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	script := BashFunctions("t_", 4, "")
	for _, tc := range []struct {
		name, release string
		programs      []string
		manager       string
	}{
		{"rocky", rocky9, []string{"dnf", "sudo"}, "dnf"},
		{"rocky minimal", rocky9, []string{"microdnf"}, "microdnf"},
		{"ubuntu", ubuntu2404, []string{"apt-get", "sudo"}, "apt"},
		{"debian, no sudo", "ID=debian\nVERSION_ID=\"12\"\n", []string{"apt-get"}, "apt"},
		{"amazon", amzn2023, []string{"dnf"}, "dnf"},
		{"arch", arch, []string{"pacman"}, "pacman"},
		{"leap", leap, []string{"zypper", "sudo"}, "zypper"},
		{"alpine", alpine, []string{"apk"}, "apk"},
		{"gentoo", gentoo, nil, ""},
		{"single quotes", "ID='fedora'\n", []string{"dnf"}, "dnf"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "etc/os-release"), []byte(tc.release), 0o644))
			bin := t.TempDir()
			for _, p := range tc.programs {
				require.NoError(t, os.WriteFile(filepath.Join(bin, p), []byte("#!/bin/sh\n"), 0o755))
			}
			body := script + "t_pkg_manager\nfor t in " + strings.Join(BashTools, " ") + "; do printf '%s=%s\\n' \"$t\" \"$(t_install_hint \"$t\")\"; done\n"
			cmd := exec.Command(bash, "--norc", "--noprofile", "-c", body)
			cmd.Env = []string{"PATH=" + bin, "GASH__SYSROOT=" + root, "OSTYPE=linux-gnu"}
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
			require.Equal(t, tc.manager, lines[0])

			sr, err := platform.NewSysroot(root)
			require.NoError(t, err)
			have := map[string]bool{}
			for _, p := range tc.programs {
				have[p] = true
			}
			s := &System{Root: os.Geteuid() == 0,
				LookPath: func(name string, _ []string) (string, bool) { return "/x/" + name, have[name] }}
			s.detect(sr, "linux")
			for i, name := range BashTools {
				want := strings.TrimSuffix(s.Report(name, "").Install(), "   (as root)")
				require.Equal(t, name+"="+want, lines[i+1], tc.name)
			}
		})
	}
}

func TestBashFunctionsIndent(t *testing.T) {
	out := BashFunctions("x_", 2, "  ")
	for _, l := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		require.True(t, l == "" || strings.HasPrefix(l, "  "), l)
	}
	require.Contains(t, out, "\n  x_install_hint() {\n    local m names")
	require.Contains(t, out, "\n      case \"$names:$t\" in\n        apk:")
}

// TestInstallHintSeveral installs several tools with one command, each
// package once, and --run runs it (apt-get update first on apt).
func TestInstallHintSeveral(t *testing.T) {
	bash, err := exec.LookPath("bash")
	require.NoError(t, err)
	for _, tc := range []struct {
		name, release string
		programs      []string
		print, run    string
	}{
		{"rocky", rocky9, []string{"dnf", "sudo"}, "sudo dnf install -y git tar curl\n",
			"sudo dnf install -y git tar curl\n"},
		{"debian", ubuntu2404, []string{"apt-get", "sudo"}, "sudo apt-get install -y git tar curl\n",
			"sudo apt-get update\nsudo apt-get install -y git tar curl\n"},
		{"alpine", alpine, []string{"apk", "sudo"}, "sudo apk add git tar curl\n",
			"sudo apk update\nsudo apk add git tar curl\n"},
		{"arch without databases", arch, []string{"pacman", "sudo"}, "sudo pacman -S --needed --noconfirm git tar curl\n",
			"sudo pacman -Sy\nsudo pacman -S --needed --noconfirm git tar curl\n"},
		{"unknown", gentoo, []string{"sudo"}, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Join(root, "etc"), 0o755))
			require.NoError(t, os.WriteFile(filepath.Join(root, "etc/os-release"), []byte(tc.release), 0o644))
			bin := t.TempDir()
			log := filepath.Join(t.TempDir(), "log")
			for _, p := range tc.programs {
				// Each program logs its own command line: sudo runs the rest.
				script := "#!/bin/sh\necho \"" + p + " $*\" >>" + log + "\n"
				if p == "sudo" {
					script = "#!/bin/sh\necho \"sudo $*\" >>" + log + "\n"
				}
				require.NoError(t, os.WriteFile(filepath.Join(bin, p), []byte(script), 0o755))
			}
			run := func(body string) string {
				cmd := exec.Command(bash, "--norc", "--noprofile", "-c", BashFunctions("t_", 4, "")+body)
				cmd.Env = []string{"PATH=" + bin, "GASH__SYSROOT=" + root, "OSTYPE=linux-gnu", "EUID=1000"}
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, string(out))
				return string(out)
			}
			if run("echo $EUID") == "0\n" {
				// Bash took the real EUID: root needs no sudo.
				tc.print = strings.ReplaceAll(tc.print, "sudo ", "")
				tc.run = strings.ReplaceAll(tc.run, "sudo ", "")
			}
			require.Equal(t, tc.print, run("t_install_hint git tar curl tar grep-is-unknown"))
			require.Equal(t, "", run("t_install_hint --run git tar curl tar"))
			got, _ := os.ReadFile(log)
			require.Equal(t, tc.run, string(got))
		})
	}
}

var update = flag.Bool("update", false, "rewrite the generated block of install.sh")

// TestInstaller keeps the generated block of install.sh current
// (go test ./internal/prereq -update rewrites it).
func TestInstaller(t *testing.T) {
	path := filepath.Join("..", "..", "install.sh")
	src, err := os.ReadFile(path)
	require.NoError(t, err)
	got, err := Installer(string(src))
	require.NoError(t, err)
	if *update {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o755))
		return
	}
	require.Equal(t, got, string(src), "install.sh is stale: go test ./internal/prereq -update")
	_, err = Installer("no block")
	require.Error(t, err)
}
