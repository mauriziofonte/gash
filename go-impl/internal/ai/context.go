// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package ai

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/contract"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// reOSReleaseID is the `grep -oP '^ID=\K\w+'` of the context distro: ID=
// at the start of a line, then one or more word characters right after it
// (LEDGER AI-21: the native GNU-grep result; AI-11: the quoting gap stays,
// so ID="rocky" matches nothing).
var reOSReleaseID = regexp.MustCompile(`(?m)^ID=(\w+)`)

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
	dist := m.distro()
	o := &contract.Object{}
	o.String("cwd", cwd).String("shell", shell).String("distro", dist).String("pkg", pkgManager(dist))
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

// distro is the distro ID of /etc/os-release, "linux" when the ID line is
// missing or its value is empty under the `^ID=\K\w+` rule.
func (m *Module) distro() string {
	b, err := m.sys.ReadFile("/etc/os-release")
	if err == nil {
		if mm := reOSReleaseID.FindSubmatch(b); mm != nil {
			return string(mm[1])
		}
	}
	return "linux"
}

// pkgManager is the 2.x distro→package-manager table (LEDGER AI-11: mint
// is apt but Linux Mint's real ID linuxmint is unknown, opensuse* is a
// glob the truncated value still matches).
func pkgManager(distro string) string {
	switch distro {
	case "ubuntu", "debian", "pop", "mint":
		return "apt"
	case "fedora", "rhel", "centos", "rocky", "alma":
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
