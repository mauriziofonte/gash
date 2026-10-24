// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/runner"
)

// reOSReleaseID is the shape of an os-release ID value once its quotes
// are gone (AI-11).
var reOSReleaseID = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// gatherContext is __ai_gather_context: a small JSON object of shell
// context (cwd, shell, distro, package manager, git branch, top files,
// last exit), jq-escaped, the keys in 2.x order.
func (m *Module) gatherContext(c *invoke.Call) string {
	cwd, _ := m.getwd()
	shell := "sh"
	switch {
	case c.Env.Get("BASH_VERSION") != "":
		shell = "bash"
	case c.Env.Get("ZSH_VERSION") != "":
		shell = "zsh"
	}
	exit := 0
	if n, err := strconv.Atoi(c.Env.Get("__AI_LAST_EXIT")); err == nil {
		exit = n
	}
	dist, pkg := m.distro()
	o := &contract.Object{}
	o.String("cwd", cwd).String("shell", shell).String("distro", dist).String("pkg", pkg)
	if branch := m.gitBranch(c); branch != "" {
		o.String("git", branch)
	} else {
		o.Raw("git", []byte("null"))
	}
	o.Raw("files", []byte(m.topFiles(cwd)))
	o.Int("exit", int64(exit))
	b, err := o.Bytes(contract.EscapeJQ)
	if err != nil {
		return ""
	}
	return string(b)
}

// distro is the ID of /etc/os-release, quoted or not ("linux" when it is
// missing or not a plain name), and its package manager: from the ID, else
// from the first known word of ID_LIKE (AI-11).
func (m *Module) distro() (string, string) {
	b, err := m.sys.ReadFile("/etc/os-release")
	if err != nil {
		return "linux", "unknown"
	}
	rel := platform.ParseOSRelease(b)
	id := rel["ID"]
	if !reOSReleaseID.MatchString(id) {
		id = "linux"
	}
	pkg := pkgManager(id)
	if pkg == "unknown" {
		for _, like := range strings.Fields(rel["ID_LIKE"]) {
			if p := likeManager(like); p != "" {
				pkg = p
				break
			}
		}
	}
	return id, pkg
}

// likeManager is the package manager of an ID_LIKE family, "" for one
// not known.
func likeManager(like string) string {
	switch {
	case like == "debian" || like == "ubuntu":
		return "apt"
	case like == "rhel" || like == "fedora" || like == "centos":
		return "dnf"
	case like == "arch":
		return "pacman"
	case like == "suse" || strings.HasPrefix(like, "opensuse"):
		return "zypper"
	}
	return ""
}

// pkgManager is the distro→package-manager table of 2.x, with Linux
// Mint's and AlmaLinux's real IDs added (AI-11).
func pkgManager(distro string) string {
	switch distro {
	case "ubuntu", "debian", "pop", "mint", "linuxmint":
		return "apt"
	case "fedora", "rhel", "centos", "rocky", "alma", "almalinux":
		return "dnf"
	case "arch", "manjaro", "endeavouros":
		return "pacman"
	case "suse":
		return "zypper"
	case "alpine":
		return "apk"
	case "macos":
		return "brew"
	}
	if strings.HasPrefix(distro, "opensuse") {
		return "zypper"
	}
	return "unknown"
}

// gitBranch is `git branch --show-current`, "" when git is absent or the
// cwd is not a repository.
func (m *Module) gitBranch(c *invoke.Call) string {
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	res, err := m.run.Run(ctx, runner.Cmd{Path: "git", Args: []string{"branch", "--show-current"},
		Options: runner.Options{Env: c.Env.Child()}})
	if err != nil || res.ExitCode != 0 {
		return ""
	}
	return strings.TrimRight(string(res.Stdout), "\n")
}

// topFiles is the first eight non-hidden entries of dir as a JSON array
// (`ls -1 | head -8`), "[]" when none or the directory cannot be read.
func (m *Module) topFiles(dir string) string {
	if dir == "" {
		return "[]"
	}
	entries, err := m.readDir(dir)
	if err != nil {
		return "[]"
	}
	names := make([]string, 0, 8)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		names = append(names, e.Name())
		if len(names) == 8 {
			break
		}
	}
	return strings.TrimRight(contract.JQArray(names...), "\n")
}
