// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/invoke"
	plat "github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/prereq"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/term"
)

// rockySystem is a Rocky Linux 9 user with sudo where only the programs in
// have are installed; installed adds programs once an install ran.
func rockySystem(have map[string]bool) *prereq.System {
	return &prereq.System{
		OS:      plat.OS{GOOS: "linux", ID: "rocky", VersionID: "9.4", Name: "Rocky Linux 9.4 (Blue Onyx)", Family: plat.FamilyFedora},
		Manager: prereq.Dnf, Sudo: true,
		LookPath: func(name string, _ []string) (string, bool) {
			return "/usr/bin/" + name, have[name]
		},
		RunProbe:   func(string, []string, []string) bool { return true },
		RunVersion: func(string, []string) string { return "GNU bash, version 5.1.8(1)-release" },
	}
}

// allButRemote has every catalog tool but the SSH ones.
func allBut(missing ...string) map[string]bool {
	have := map[string]bool{}
	for _, t := range prereq.Catalog() {
		names := t.Commands
		if len(names) == 0 {
			names = []string{t.Name}
		}
		for _, n := range names {
			have[n] = true
		}
	}
	for _, n := range missing {
		delete(have, n)
	}
	return have
}

func doctorCall(t *testing.T, m *Module, tm *term.Term, env map[string]string, args ...string) (string, string, int) {
	t.Helper()
	tbl := invoke.NewTable()
	m.Register(tbl)
	var out, errb bytes.Buffer
	if env == nil {
		env = map[string]string{}
	}
	e := invoke.EnvMap(env)
	if tm == nil {
		tm = term.Fixed(e, false, false, false)
	}
	c := &invoke.Call{Name: "gash_doctor", Args: args, Env: e, Stdout: &out, Stderr: &errb, Term: tm}
	rc := tbl.Run(c)
	return out.String(), errb.String(), rc
}

func TestDoctorPrereqs(t *testing.T) {
	defer prereq.SetSystem(rockySystem(allBut("rsync", "tar", "fzf")))()
	out, errs, rc := doctorCall(t, &Module{}, nil, nil, "--prereqs")
	require.Equal(t, 1, rc, "a missing required tool is an issue")
	require.Contains(t, out, "Prerequisites:  Rocky Linux 9.4 (dnf)\n")
	require.Contains(t, out, "  Remote hosts and SSH\n")
	require.NotContains(t, out, "Windows", "not WSL")
	require.Contains(t, errs, "OK:     git (/usr/bin/git)\n")
	require.Contains(t, errs, "Error:     tar — not installed: unpacks the Gash release archive (needed by archive_extract (tar archives))\n"+
		"          Install tar: sudo dnf install -y tar\n")
	require.Contains(t, errs, "Warning:     rsync — not installed: syncs directories with remote hosts (needed by remote_sync_pull and remote_sync_push)\n"+
		"          Install rsync: sudo dnf install -y rsync\n")
	require.Contains(t, errs, "Info:     fzf — not installed: picks an entry from a list (needed by hgrep (--pick)) (optional)\n"+
		"          Install fzf: sudo dnf install -y epel-release && sudo dnf install -y fzf\n")
	require.Contains(t, errs, "Hint: gash_doctor --install installs the missing tools")

	defer prereq.SetSystem(rockySystem(allBut("rsync")))()
	_, _, rc = doctorCall(t, &Module{}, nil, nil, "--prereqs")
	require.Equal(t, 0, rc, "a missing feature tool is a warning")
}

func TestDoctorJSON(t *testing.T) {
	defer prereq.SetSystem(rockySystem(allBut("rsync")))()
	out, errs, rc := doctorCall(t, &Module{}, nil, nil, "--json")
	require.Equal(t, 0, rc)
	require.Empty(t, errs)
	require.True(t, strings.HasPrefix(out, `{"system":{"name":"Rocky Linux 9.4 (Blue Onyx)","id":"rocky","version":"9.4","manager":"dnf","root":false,"sudo":true,"wsl":false},"tools":[{"name":"bash","level":"required","area":"core","found":true,"path":"/usr/bin/bash"},`), out)
	require.Contains(t, out, `{"name":"rsync","level":"feature","area":"remote","found":false,"needed_by":["remote_sync_pull","remote_sync_push"],"package":"rsync","install":"sudo dnf install -y rsync","advice":"Install rsync: sudo dnf install -y rsync"}`)
	require.True(t, strings.HasSuffix(out, `],"missing":1,"missing_required":0}`+"\n"), out)
}

func TestDoctorInstall(t *testing.T) {
	t.Run("headless refuses", func(t *testing.T) {
		defer prereq.SetSystem(rockySystem(allBut("rsync")))()
		_, errs, rc := doctorCall(t, &Module{run: runner.NewFake(t)}, nil, map[string]string{"GASH_HEADLESS": "1"}, "--install", "--yes")
		require.Equal(t, 1, rc)
		require.Equal(t, `{"error":"install_needs_confirmation","details":"gash_doctor installs packages only for a person at a terminal","action":"STOP","recoverable":true,"hint":"Ask the user to run: sudo dnf install -y rsync","install":"sudo dnf install -y rsync"}`+"\n", errs)
	})
	t.Run("no terminal, no --yes", func(t *testing.T) {
		defer prereq.SetSystem(rockySystem(allBut("rsync")))()
		e := invoke.EnvMap(nil)
		_, errs, rc := doctorCall(t, &Module{run: runner.NewFake(t)}, term.Fixed(e, false, false, false).WithoutTTY(), nil, "--install")
		require.Equal(t, 1, rc)
		require.Contains(t, errs, "Command: sudo dnf install -y rsync\n")
		require.Contains(t, errs, "Nothing installed.")
	})
	t.Run("confirmed", func(t *testing.T) {
		have := allBut("rsync", "zstd")
		sys := rockySystem(have)
		defer prereq.SetSystem(sys)()
		fake := runner.NewFake(t, runner.Expect{Argv: []string{"sudo", "dnf", "install", "-y", "rsync", "zstd"}})
		e := invoke.EnvMap(nil)
		tm := term.Fixed(e, true, false, false).WithTTY("y\n")
		// The fake install puts the programs on PATH.
		sys.LookPath = func(name string, _ []string) (string, bool) {
			if name == "rsync" || name == "zstd" {
				return "/usr/bin/" + name, len(fake.Calls()) > 0
			}
			return "/usr/bin/" + name, have[name]
		}
		_, errs, rc := doctorCall(t, &Module{run: fake}, tm, nil, "--install")
		require.Equal(t, 0, rc, errs)
		require.Contains(t, errs, "To install on Rocky Linux 9.4 (dnf): rsync zstd\n")
		require.Contains(t, errs, "OK:   rsync (/usr/bin/rsync)\n")
		require.Contains(t, errs, "OK:   zstd (/usr/bin/zstd)\n")
	})
	t.Run("apt updates first", func(t *testing.T) {
		sys := rockySystem(allBut("rsync"))
		sys.OS = plat.OS{GOOS: "linux", ID: "debian", VersionID: "12", Name: "Debian GNU/Linux 12 (bookworm)"}
		sys.Manager, sys.Root = prereq.Apt, true
		defer prereq.SetSystem(sys)()
		fake := runner.NewFake(t,
			runner.Expect{Argv: []string{"apt-get", "update"}},
			runner.Expect{Argv: []string{"apt-get", "install", "-y", "rsync"}, Status: 100})
		_, errs, rc := doctorCall(t, &Module{run: fake}, nil, nil, "--install", "--yes", "rsync")
		require.Equal(t, 1, rc)
		require.Contains(t, errs, "The install command failed (status 100): apt-get install -y rsync")
	})
	t.Run("one at a time after a failure", func(t *testing.T) {
		sys := rockySystem(allBut("rsync", "zstd", "lsof"))
		defer prereq.SetSystem(sys)()
		fake := runner.NewFake(t,
			runner.Expect{Argv: []string{"sudo", "dnf", "install", "-y", "rsync", "lsof", "zstd"}, Status: 1},
			runner.Expect{Argv: []string{"sudo", "dnf", "install", "-y", "rsync"}},
			runner.Expect{Argv: []string{"sudo", "dnf", "install", "-y", "lsof"}},
			runner.Expect{Argv: []string{"sudo", "dnf", "install", "-y", "zstd"}, Status: 1})
		sys.LookPath = func(name string, _ []string) (string, bool) {
			if name == "rsync" || name == "lsof" {
				return "/usr/bin/" + name, len(fake.Calls()) > 1
			}
			return "/usr/bin/" + name, name != "zstd"
		}
		_, errs, rc := doctorCall(t, &Module{run: fake}, nil, nil, "--install", "--yes")
		require.Equal(t, 1, rc)
		require.Contains(t, errs, "Installing the packages one at a time.")
		require.Contains(t, errs, "The install command failed (status 1): sudo dnf install -y zstd")
		require.Contains(t, errs, "OK:   rsync (/usr/bin/rsync)\n")
		require.Contains(t, errs, "zstd — still not found after the install")
		require.Contains(t, errs, "OK:   lsof (/usr/bin/lsof)\n")
	})
	t.Run("non-free only by name, package lists first", func(t *testing.T) {
		for _, tc := range []struct {
			id, name string
			mgr      prereq.Manager
			refresh  []string
		}{
			{"debian", "Debian GNU/Linux 12 (bookworm)", prereq.Apt, []string{"apt-get", "update"}},
			{"alpine", "Alpine Linux v3.22", prereq.Apk, []string{"apk", "update"}},
		} {
			sys := rockySystem(allBut("rsync", "unrar"))
			sys.OS = plat.OS{GOOS: "linux", ID: tc.id, Name: tc.name}
			sys.Manager, sys.Root = tc.mgr, true
			restore := prereq.SetSystem(sys)
			fake := runner.NewFake(t, runner.Expect{Argv: tc.refresh},
				runner.Expect{Argv: sys.InstallArgv(false, "rsync")[0]})
			_, errs, _ := doctorCall(t, &Module{run: fake}, nil, nil, "--install", "--yes")
			require.Contains(t, errs, "unrar: Install unrar: ", tc.id)
			restore()
		}
		sys := rockySystem(allBut("unrar"))
		sys.OS = plat.OS{GOOS: "linux", ID: "arch", Name: "Arch Linux"}
		sys.Manager, sys.Root, sys.NoPacmanDB = prereq.Pacman, true, true
		defer prereq.SetSystem(sys)()
		fake := runner.NewFake(t, runner.Expect{Argv: []string{"pacman", "-Sy"}},
			runner.Expect{Argv: []string{"pacman", "-S", "--needed", "--noconfirm", "unrar"}})
		_, _, rc := doctorCall(t, &Module{run: fake}, nil, nil, "--install", "--yes", "unrar")
		require.Equal(t, 1, rc, "the fake installs nothing")
		require.Len(t, fake.Calls(), 2)
	})
	t.Run("named, installed, unknown, manual", func(t *testing.T) {
		defer prereq.SetSystem(rockySystem(allBut("docker")))()
		_, errs, rc := doctorCall(t, &Module{run: runner.NewFake(t)}, nil, nil, "--install", "git")
		require.Equal(t, 0, rc)
		require.Contains(t, errs, "git is installed (/usr/bin/git)")
		_, errs, rc = doctorCall(t, &Module{run: runner.NewFake(t)}, nil, nil, "--install", "nosuchtool")
		require.Equal(t, usageStatus, rc)
		require.Contains(t, errs, "Unknown tool: nosuchtool")
		_, errs, rc = doctorCall(t, &Module{run: runner.NewFake(t)}, nil, nil, "--install", "docker")
		require.Equal(t, 1, rc)
		require.Contains(t, errs, "docker: Install docker: https://docs.docker.com/engine/install/")
	})
	t.Run("no sudo", func(t *testing.T) {
		have := allBut("rsync")
		sys := rockySystem(have)
		sys.Sudo = false
		defer prereq.SetSystem(sys)()
		_, errs, rc := doctorCall(t, &Module{run: runner.NewFake(t)}, nil, nil, "--install", "--yes")
		require.Equal(t, 1, rc)
		require.Contains(t, errs, "Installing packages needs root, and sudo is not installed. As root, run: dnf install -y rsync")
	})
	t.Run("usage", func(t *testing.T) {
		_, errs, rc := doctorCall(t, &Module{}, nil, nil, "--yes")
		require.Equal(t, usageStatus, rc)
		require.Contains(t, errs, "--yes and --all go with --install")
		_, errs, rc = doctorCall(t, &Module{}, nil, nil, "git")
		require.Equal(t, usageStatus, rc)
		require.Contains(t, errs, "Unexpected argument: git")
	})
}
