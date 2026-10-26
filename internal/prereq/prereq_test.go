// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package prereq

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/ui"
)

const (
	rocky9     = "ID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\nPRETTY_NAME=\"Rocky Linux 9.4 (Blue Onyx)\"\n"
	ubuntu2404 = "ID=ubuntu\nID_LIKE=debian\nVERSION_ID=\"24.04\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n"
	amzn2023   = "ID=\"amzn\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"2023\"\nPRETTY_NAME=\"Amazon Linux 2023.6.20241010\"\n"
	oracle9    = "ID=\"ol\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"9.4\"\nPRETTY_NAME=\"Oracle Linux Server 9.4\"\n"
	rhel9      = "ID=\"rhel\"\nID_LIKE=\"fedora\"\nVERSION_ID=\"9.4\"\nPRETTY_NAME=\"Red Hat Enterprise Linux 9.4 (Plow)\"\n"
	fedora44   = "ID=fedora\nVERSION_ID=44\nPRETTY_NAME=\"Fedora Linux 44 (Container Image)\"\n"
	arch       = "NAME=\"Arch Linux\"\nID=arch\nPRETTY_NAME=\"Arch Linux\"\n"
	leap       = "ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\nVERSION_ID=\"15.6\"\nPRETTY_NAME=\"openSUSE Leap 15.6\"\n"
	alpine     = "ID=alpine\nVERSION_ID=3.22.2\nPRETTY_NAME=\"Alpine Linux v3.22\"\n"
	gentoo     = "ID=gentoo\nPRETTY_NAME=\"Gentoo Linux\"\n"
)

// fakeSystem builds a System from an os-release text (a macOS plist
// version for darwin), with the programs that are "on PATH".
func fakeSystem(t *testing.T, goos, release string, root bool, programs ...string) *System {
	t.Helper()
	dir := t.TempDir()
	if goos == "darwin" {
		p := filepath.Join(dir, "System/Library/CoreServices")
		require.NoError(t, os.MkdirAll(p, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(p, "SystemVersion.plist"),
			[]byte("<key>ProductVersion</key>\n<string>"+release+"</string>"), 0o644))
	} else {
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "etc"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "etc/os-release"), []byte(release), 0o644))
	}
	sr, err := platform.NewSysroot(dir)
	require.NoError(t, err)
	have := map[string]string{}
	for _, p := range programs {
		if strings.HasPrefix(p, "/") {
			have[p] = p
			have[filepath.Base(p)] = p
			continue
		}
		have[p] = "/usr/bin/" + p
	}
	s := &System{Root: root,
		LookPath: func(name string, _ []string) (string, bool) { p, ok := have[name]; return p, ok },
		RunProbe: func(string, []string, []string) bool { return true },
		Xcode:    func(string) bool { return false },
	}
	s.detect(sr, goos)
	return s
}

func TestDetectManager(t *testing.T) {
	cases := []struct {
		name, goos, release string
		programs            []string
		want                Manager
		describe            string
	}{
		{"rocky", "linux", rocky9, []string{"dnf"}, Dnf, "Rocky Linux 9.4 (dnf)"},
		{"rocky minimal", "linux", rocky9, []string{"microdnf"}, Microdnf, "Rocky Linux 9.4 (microdnf)"},
		{"ubuntu", "linux", ubuntu2404, []string{"apt-get"}, Apt, "Ubuntu 24.04.1 LTS (apt)"},
		{"amazon", "linux", amzn2023, []string{"dnf"}, Dnf, "Amazon Linux 2023.6.20241010 (dnf)"},
		{"arch", "linux", arch, []string{"pacman"}, Pacman, "Arch Linux (pacman)"},
		{"leap", "linux", leap, []string{"zypper"}, Zypper, "openSUSE Leap 15.6 (zypper)"},
		{"alpine", "linux", alpine, []string{"apk"}, Apk, "Alpine Linux v3.22 (apk)"},
		{"gentoo, nothing known", "linux", gentoo, nil, NoManager, "Gentoo Linux"},
		{"unknown id, apt on PATH", "linux", gentoo, []string{"apt-get"}, Apt, "Gentoo Linux (apt)"},
		{"macos with brew", "darwin", "15.1", []string{"/opt/homebrew/bin/brew"}, Brew, "macOS 15.1 (brew)"},
		{"macos without brew", "darwin", "15.1", nil, Brew, "macOS 15.1 (brew)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fakeSystem(t, tc.goos, tc.release, false, tc.programs...)
			require.Equal(t, tc.want, s.Manager)
			require.Equal(t, tc.describe, s.Describe())
		})
	}
}

func TestInstallCommand(t *testing.T) {
	cases := []struct {
		name, goos, release string
		root                bool
		programs            []string
		tool                string
		want                string
	}{
		{"dnf with sudo", "linux", rocky9, false, []string{"dnf", "sudo"}, "ps", "sudo dnf install -y procps-ng"},
		{"dnf as root", "linux", rocky9, true, []string{"dnf"}, "tar", "dnf install -y tar"},
		{"dnf without sudo", "linux", rocky9, false, []string{"dnf"}, "tar", "dnf install -y tar   (as root)"},
		{"microdnf", "linux", rocky9, true, []string{"microdnf"}, "tar", "microdnf install -y tar"},
		{"epel on rocky", "linux", rocky9, false, []string{"dnf", "sudo"}, "fzf",
			"sudo dnf install -y epel-release && sudo dnf install -y fzf"},
		{"epel on oracle", "linux", oracle9, true, []string{"dnf"}, "fzf",
			"dnf install -y oracle-epel-release-el9 && dnf install -y fzf"},
		{"epel on rhel", "linux", rhel9, true, []string{"dnf"}, "fzf",
			"dnf install -y https://dl.fedoraproject.org/pub/epel/epel-release-latest-9.noarch.rpm && dnf install -y fzf"},
		{"no epel on fedora", "linux", fedora44, true, []string{"dnf"}, "fzf", "dnf install -y fzf"},
		{"no epel on amazon", "linux", amzn2023, true, []string{"dnf"}, "fzf", ""},
		{"apt", "linux", ubuntu2404, false, []string{"apt-get", "sudo"}, "ss", "sudo apt-get install -y iproute2"},
		{"pacman", "linux", arch, true, []string{"pacman"}, "ssh", "pacman -S --needed --noconfirm openssh"},
		{"zypper", "linux", leap, true, []string{"zypper"}, "ssh", "zypper --non-interactive install openssh-clients"},
		{"apk", "linux", alpine, true, []string{"apk"}, "xz", "apk add xz"},
		{"brew, never sudo", "darwin", "15.1", false, []string{"/opt/homebrew/bin/brew", "sudo"}, "xz", "brew install xz"},
		{"brew missing", "darwin", "15.1", false, nil, "xz", ""},
		{"builtin on macOS", "darwin", "15.1", false, []string{"/opt/homebrew/bin/brew"}, "tar", ""},
		{"docker is never installed", "linux", ubuntu2404, true, []string{"apt-get"}, "docker", ""},
		{"unknown manager", "linux", gentoo, true, nil, "tar", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := fakeSystem(t, tc.goos, tc.release, tc.root, tc.programs...)
			require.Equal(t, tc.want, s.Report(tc.tool, "").Install())
		})
	}
}

func TestAdvice(t *testing.T) {
	rocky := fakeSystem(t, "linux", rocky9, false, "dnf", "sudo")
	require.Equal(t, "Install procps-ng: sudo dnf install -y procps-ng", rocky.Report("ps", "").Advice())
	require.Equal(t, "Install docker: https://docs.docker.com/engine/install/", rocky.Report("docker", "").Advice())
	require.Equal(t, "systemd is part of the operating system: Gash cannot install it. Under WSL, enable it in /etc/wsl.conf",
		rocky.Report("systemctl", "").Advice())
	require.Equal(t, "Install git: sudo dnf install -y git. On macOS git comes with the Command Line Tools: xcode-select --install",
		rocky.Report("git", "").Advice())

	deb := fakeSystem(t, "linux", ubuntu2404, true, "apt-get")
	require.Equal(t, "Install unrar: apt-get install -y unrar (the package is in the non-free (Debian) or multiverse (Ubuntu) repository)",
		deb.Report("unrar", "").Advice())

	mac := fakeSystem(t, "darwin", "15.1", false)
	require.Equal(t, "Install Homebrew (https://brew.sh), then: brew install xz", mac.Report("xz", "").Advice())
	require.Equal(t, "tar ships with macOS 15.1: check that the system directories (/usr/bin, /usr/sbin) are on PATH",
		mac.Report("tar", "").Advice())

	amzn := fakeSystem(t, "linux", amzn2023, true, "dnf")
	require.Equal(t, "fzf is not in the Amazon Linux repositories: install it from its project page", amzn.Report("fzf", "").Advice())

	gent := fakeSystem(t, "linux", gentoo, true)
	require.Equal(t, "Install tar with the package manager of Gentoo Linux", gent.Report("tar", "").Advice())
}

func TestEnvelope(t *testing.T) {
	s := fakeSystem(t, "linux", rocky9, false, "dnf", "sudo")
	require.Equal(t,
		`{"error":"ps_not_found","details":"llm_procs needs ps, which is not installed","action":"CONTINUE","recoverable":true,`+
			`"hint":"Install procps-ng: sudo dnf install -y procps-ng","missing":"ps","install":"sudo dnf install -y procps-ng"}`,
		s.Report("ps", "llm_procs").Envelope().String())
	require.Equal(t,
		`{"error":"prerequisite_missing","details":"remote_sync_push needs rsync, which is not installed","action":"STOP","recoverable":true,`+
			`"hint":"Install rsync: sudo dnf install -y rsync","missing":"rsync","install":"sudo dnf install -y rsync"}`,
		s.Report("rsync", "remote_sync_push").Envelope().String())
	require.Equal(t,
		`{"error":"docker_not_found","details":"docker is not installed","action":"CONTINUE","recoverable":true,`+
			`"hint":"Install docker: https://docs.docker.com/engine/install/","missing":"docker"}`,
		s.Report("docker", "").Envelope().String())
}

func TestPrint(t *testing.T) {
	s := fakeSystem(t, "linux", rocky9, false, "dnf", "sudo")
	var b bytes.Buffer
	s.Report("ps", "process_find").Print(ui.Printer{W: &b})
	require.Equal(t, "Error: process_find needs ps, which is not installed.\n"+
		"  ps lists the running processes (package procps-ng on Rocky Linux 9.4).\n"+
		"  Install it:  sudo dnf install -y procps-ng\n"+
		"  Or run:      gash_doctor --install ps\n", b.String())

	b.Reset()
	s.Report("docker", "docker_stop_all").Print(ui.Printer{W: &b})
	require.Equal(t, "Error: docker_stop_all needs docker, which is not installed.\n"+
		"  docker runs and inspects containers.\n"+
		"  Install docker: https://docs.docker.com/engine/install/.\n", b.String())

	b.Reset()
	root := fakeSystem(t, "linux", rocky9, false, "dnf")
	root.Report("tar", "").Print(ui.Printer{W: &b})
	require.Equal(t, "Error: tar is not installed.\n"+
		"  tar unpacks the Gash release archive (package tar on Rocky Linux 9.4).\n"+
		"  Install it:  dnf install -y tar   (as root)\n", b.String())
}

func TestFind(t *testing.T) {
	s := fakeSystem(t, "linux", rocky9, false, "dnf", "wget", "netstat")
	p, ok := s.Find(must(t, "curl"))
	require.True(t, ok)
	require.Equal(t, "/usr/bin/wget", p, "wget satisfies curl")
	_, ok = s.Find(must(t, "ss"))
	require.True(t, ok, "netstat satisfies ss")
	require.False(t, s.Has("tar"))
	require.True(t, s.Has("dnf"), "a name outside the catalog is a program")

	refused := fakeSystem(t, "linux", rocky9, false, "sed")
	refused.RunProbe = func(path string, args []string, _ []string) bool { return false }
	_, ok = refused.Find(must(t, "gnu-sed"))
	require.False(t, ok, "a sed without --sandbox does not count")

	mac := fakeSystem(t, "darwin", "15.1", false, "git")
	_, ok = mac.Find(must(t, "git"))
	require.False(t, ok, "the /usr/bin/git Xcode stub without the Command Line Tools")
	mac = fakeSystem(t, "darwin", "15.1", false, "git")
	mac.Xcode = func(string) bool { return true }
	_, ok = mac.Find(must(t, "git"))
	require.True(t, ok)
}

func TestFindVersion(t *testing.T) {
	versions := map[string]string{
		"/bin/bash":                "GNU bash, version 3.2.57(1)-release (arm64-apple-darwin24)",
		"/opt/homebrew/bin/bash":   "GNU bash, version 5.3.3(1)-release (aarch64-apple-darwin24.4.0)",
		"/usr/local/bin/bash":      "GNU bash, version 4.4.20(1)-release",
		"/usr/local/old/bin/bash":  "GNU bash, version 4.3.48(1)-release",
		"/usr/local/none/bin/bash": "",
	}
	for path, want := range map[string]bool{"/opt/homebrew/bin/bash": true, "/usr/local/bin/bash": true,
		"/bin/bash": false, "/usr/local/old/bin/bash": false, "/usr/local/none/bin/bash": false} {
		s := &System{
			LookPath:   func(string, []string) (string, bool) { return path, true },
			RunVersion: func(p string, _ []string) string { return versions[p] },
		}
		_, ok := s.Find(must(t, "bash"))
		require.Equal(t, want, ok, path)
	}
}

func TestFindAllOnPath(t *testing.T) {
	old, nu := t.TempDir(), t.TempDir()
	for _, d := range []string{old, nu} {
		require.NoError(t, os.WriteFile(filepath.Join(d, "bash"), []byte("#!/bin/sh\n"), 0o755))
	}
	s := &System{Env: []string{"PATH=" + old + ":" + nu},
		RunVersion: func(p string, _ []string) string {
			if filepath.Dir(p) == old {
				return "GNU bash, version 3.2.57(1)-release"
			}
			return "GNU bash, version 5.2.21(1)-release"
		}}
	p, ok := s.Find(must(t, "bash"))
	require.True(t, ok, "the second bash on PATH is new enough")
	require.Equal(t, filepath.Join(nu, "bash"), p)
}

func TestCatalog(t *testing.T) {
	seen := map[string]bool{}
	areas := map[string]bool{}
	for _, a := range Areas {
		areas[a.Key] = true
	}
	for _, tool := range Catalog() {
		require.False(t, seen[tool.Name], "duplicate %s", tool.Name)
		seen[tool.Name] = true
		require.True(t, areas[tool.Area], "%s: unknown area %q", tool.Name, tool.Area)
		require.NotEmpty(t, tool.Purpose, tool.Name)
		require.False(t, strings.HasSuffix(tool.Purpose, "."), "%s: the purpose is a phrase", tool.Name)
		if tool.NoInstall {
			require.True(t, tool.Manual != "" || tool.Note != "", "%s: never installed, so it says how", tool.Name)
			continue
		}
		for _, m := range Managers {
			_, has := tool.Packages[m]
			builtin := m == Brew && contains(tool.Builtin, "darwin")
			require.True(t, has || builtin || tool.Manual != "" || tool.NonFree || len(tool.OS) > 0,
				"%s: no package for %s and no manual page", tool.Name, m)
		}
		if tool.Code != "" {
			r := (&System{}).Report(tool.Name, "")
			require.NotPanics(t, func() { r.Envelope() }, "%s: code %s is in the contract", tool.Name, tool.Code)
		}
	}
}


func must(t *testing.T, name string) *Tool {
	t.Helper()
	tool, ok := Lookup(name)
	require.True(t, ok, name)
	return tool
}

func TestNeeds(t *testing.T) {
	used := map[string]bool{}
	for _, name := range registry.NeedNames() {
		if !strings.HasPrefix(name, "gash ") {
			_, ok := registry.Lookup(name)
			require.True(t, ok, "%s is not a registry function", name)
		}
		for _, n := range registry.NeedsOf(name) {
			_, ok := Lookup(n.Tool)
			require.True(t, ok, "%s needs %s, which is not in the catalog", name, n.Tool)
			used[n.Tool] = true
		}
	}
	for _, tool := range Catalog() {
		if tool.Level != Required {
			require.True(t, used[tool.Name], "no function needs %s", tool.Name)
		}
	}
	require.Equal(t, []string{"llm_procs (--port)", "port_kill"}, Users("lsof"))
}
