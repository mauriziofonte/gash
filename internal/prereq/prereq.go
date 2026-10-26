// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package prereq is the one catalog of the external programs Gash runs,
// and the one way to say that one is missing: what it is for, which
// package provides it on this system, and the command that installs it.
// gash_doctor lists the catalog, the installer and the bootstrap embed the
// part they need, and docs/requirements.md is generated from it.
package prereq

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

// Level says how much Gash needs a tool.
type Level int

const (
	// Required: Gash cannot be installed, fetched or started without it.
	Required Level = iota
	// Feature: a function, or one of its flags, stops without it.
	Feature
	// Optional: a function runs without it and reports less.
	Optional
)

// String is the level as reports and --json name it.
func (l Level) String() string {
	switch l {
	case Required:
		return "required"
	case Feature:
		return "feature"
	}
	return "optional"
}

// Manager is a package manager.
type Manager string

// The package managers Gash knows. A system with none of them gets the
// package name and no command.
const (
	Apt       Manager = "apt"
	Dnf       Manager = "dnf"
	Microdnf  Manager = "microdnf" // RHEL-family minimal images: dnf package names
	Yum       Manager = "yum"      // dnf package names
	Pacman    Manager = "pacman"
	Zypper    Manager = "zypper"
	Apk       Manager = "apk"
	Brew      Manager = "brew"
	NoManager Manager = ""
)

// Managers are the managers a tool lists packages for, in report order.
// Microdnf and yum read the dnf names.
var Managers = []Manager{Apt, Dnf, Pacman, Zypper, Apk, Brew}

// names is the manager whose package names m installs.
func (m Manager) names() Manager {
	if m == Microdnf || m == Yum {
		return Dnf
	}
	return m
}

// Tool is one entry of the catalog.
type Tool struct {
	// Name is the name reports show and Require takes ("ps", "curl").
	Name string
	// Commands are the programs that satisfy it, the preferred first
	// (curl, then wget). Empty means Name alone.
	Commands []string
	// Purpose says what Gash uses it for: a verb phrase that completes
	// "<name> ..." ("lists the running processes").
	Purpose string
	Level   Level
	// Area groups the tool in reports: "core", "git", "remote", ...
	Area string
	// OS limits the tool to "linux", "darwin" or "wsl" (Linux under WSL);
	// empty is every system.
	OS []string
	// Packages names the package that provides it, per manager (several,
	// space-separated, when one is not enough). A missing manager means
	// none is known there.
	Packages map[Manager]string
	// Builtin lists the systems where it ships with the OS ("darwin"):
	// missing there, it is not a package to install.
	Builtin []string
	// EPEL marks a dnf package that RHEL and its rebuilds (not Fedora,
	// not Amazon Linux) take from EPEL.
	EPEL bool
	// NonFree marks a package outside the default repositories (Debian
	// non-free, openSUSE non-oss): the command needs that repository.
	NonFree bool
	// Manual is the page that explains how to install it, for a tool Gash
	// never installs itself (Docker, nvm, cosign) or one that has no
	// package on some systems.
	Manual string
	// Note is extra advice shown with the install command ("put gnubin on
	// your PATH").
	Note string
	// Code is the contract code of a missing tool; empty is
	// prerequisite_missing.
	Code string
	// Probe, when set, are the arguments a found program must accept, with
	// stdin from /dev/null, to count (GNU sed --sandbox). The program
	// exiting non-zero does not satisfy the tool.
	Probe []string
	// MinVersion, when set, is the oldest version that counts: the first
	// N.N in the output of PROGRAM --version.
	MinVersion string
	// Gash never installs it: Manual tells how.
	NoInstall bool
	// Locate, when set, finds it when PATH has none of Commands: the gash
	// hte daemons live outside a user's PATH (/usr/sbin) and under
	// versioned names (php-fpm8.3), where gash hte looks for them. Its
	// Packages list only the systems where Locate can find it.
	Locate func(sr platform.Sysroot, o platform.OS) (string, bool)
}

// commands are the programs that satisfy t.
func (t *Tool) commands() []string {
	if len(t.Commands) == 0 {
		return []string{t.Name}
	}
	return t.Commands
}

// On reports whether t applies to a system (os is runtime.GOOS, wsl the
// WSL flag).
func (t *Tool) On(goos string, wsl bool) bool {
	if len(t.OS) == 0 {
		return true
	}
	for _, o := range t.OS {
		if o == goos || (o == "wsl" && wsl) {
			return true
		}
	}
	return false
}

// Lookup returns the catalog entry of a tool, by its name or by one of
// its commands.
func Lookup(name string) (*Tool, bool) {
	for i := range catalog {
		if catalog[i].Name == name {
			return &catalog[i], true
		}
	}
	for i := range catalog {
		for _, c := range catalog[i].Commands {
			if c == name {
				return &catalog[i], true
			}
		}
	}
	return nil, false
}

// Catalog returns every entry, in report order.
func Catalog() []*Tool {
	out := make([]*Tool, len(catalog))
	for i := range catalog {
		out[i] = &catalog[i]
	}
	return out
}

// System is what an install command depends on: the distribution, its
// package manager, whether the user is root and has sudo. It also finds
// programs, in the PATH of Env.
type System struct {
	OS      platform.OS
	Manager Manager
	Root    bool
	Sudo    bool
	// Brew is the brew program on darwin ("" when Homebrew is missing).
	Brew string
	// NoPacmanDB is an Arch system without the package databases (a fresh
	// container): pacman -Sy must fetch them before an install.
	NoPacmanDB bool
	// Env is the environment programs are looked up in (its PATH).
	Env []string
	// Sysroot is where Locate looks.
	Sysroot platform.Sysroot

	// For tests: the program lookup, the probe run and the Xcode check.
	LookPath   func(name string, env []string) (string, bool)
	RunProbe   func(path string, args []string, env []string) bool
	RunVersion func(path string, env []string) string
	Xcode      func(name string) bool

	once  sync.Mutex
	found map[string]foundAt
}

type foundAt struct {
	path string
	ok   bool
}

// Detect reads the running system. env is the caller's environment.
func Detect(env []string) *System {
	s := &System{Root: os.Geteuid() == 0, Env: env}
	s.detect(platform.Real(), runtime.GOOS)
	return s
}

// detect fills the system from the sysroot: the distribution, its
// manager and sudo, found with the lookup hooks already set.
func (s *System) detect(sr platform.Sysroot, goos string) {
	s.Sysroot = sr
	s.OS = platform.DetectOS(sr, goos)
	s.Manager = s.detectManager()
	_, s.Sudo = s.lookPath("sudo")
	if s.Manager == Pacman {
		dbs, _ := sr.ReadDir("/var/lib/pacman/sync")
		s.NoPacmanDB = len(dbs) == 0
	}
}

// detectManager is the manager of the distribution, confirmed on PATH
// (dnf, then microdnf and yum on the RHEL family).
func (s *System) detectManager() Manager {
	if s.OS.GOOS == "darwin" {
		for _, p := range []string{"brew", "/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if found, ok := s.lookPath(p); ok {
				s.Brew = found
				break
			}
		}
		return Brew
	}
	want := DistroManager(s.OS.ID, s.OS.IDLike)
	if want == NoManager {
		for _, m := range []Manager{Apt, Dnf, Microdnf, Pacman, Zypper, Apk, Yum} {
			if _, ok := s.lookPath(m.program()); ok {
				return m
			}
		}
		return NoManager
	}
	if _, ok := s.lookPath(want.program()); ok {
		return want
	}
	if want == Dnf {
		for _, m := range []Manager{Microdnf, Yum} {
			if _, ok := s.lookPath(m.program()); ok {
				return m
			}
		}
	}
	return want
}

// distroIDs are the os-release IDs (and ID_LIKE words) of each Linux
// manager. The bootstrap's generated Bash reads the same table.
var distroIDs = []struct {
	Manager Manager
	IDs     []string
}{
	{Apt, []string{"debian", "ubuntu", "linuxmint", "pop", "raspbian", "kali"}},
	{Dnf, []string{"fedora", "rhel", "centos", "rocky", "almalinux", "ol", "amzn"}},
	{Pacman, []string{"arch", "archarm", "manjaro", "endeavouros"}},
	{Zypper, []string{"opensuse", "opensuse-leap", "opensuse-tumbleweed", "suse", "sles"}},
	{Apk, []string{"alpine"}},
}

// DistroManager is the manager an os-release ID (or one of its ID_LIKE)
// names; NoManager when none is known.
func DistroManager(id string, like []string) Manager {
	for _, v := range append([]string{id}, like...) {
		for _, d := range distroIDs {
			for _, x := range d.IDs {
				if v == x {
					return d.Manager
				}
			}
		}
	}
	return NoManager
}

// program is the executable of the manager.
func (m Manager) program() string {
	if m == Apt {
		return "apt-get"
	}
	return string(m)
}

// Describe is the system as reports name it: "Rocky Linux 9.4 (dnf)".
func (s *System) Describe() string {
	name := shortName(s.OS.Name)
	if name == "" {
		name = s.OS.Platform()
	}
	if s.Manager == NoManager {
		return name
	}
	return name + " (" + string(s.Manager) + ")"
}

// el reports a RHEL rebuild, where some packages come from EPEL.
func (s *System) el() bool {
	if s.Manager.names() != Dnf {
		return false
	}
	switch s.OS.ID {
	case "rhel", "centos", "rocky", "almalinux", "ol":
		return true
	}
	return false
}

func (s *System) lookPath(name string) (string, bool) {
	if s.LookPath != nil {
		return s.LookPath(name, s.Env)
	}
	p, err := runner.LookPath(name, s.Env, "")
	return p, err == nil
}

// Find returns the program that satisfies t on this system: the first of
// its commands on PATH that passes its probe. On macOS an Xcode stub
// (/usr/bin/git without the Command Line Tools) does not count.
func (s *System) Find(t *Tool) (string, bool) {
	s.once.Lock()
	defer s.once.Unlock()
	if f, ok := s.found[t.Name]; ok {
		return f.path, f.ok
	}
	path, ok := s.find(t)
	if s.found == nil {
		s.found = map[string]foundAt{}
	}
	s.found[t.Name] = foundAt{path, ok}
	return path, ok
}

func (s *System) find(t *Tool) (string, bool) {
	for _, name := range t.commands() {
		for _, p := range s.lookAll(name, len(t.Probe) > 0 || t.MinVersion != "") {
			if s.OS.GOOS == "darwin" && xcodeStub(p) && !s.xcode(name) {
				continue
			}
			if len(t.Probe) > 0 && !s.probe(p, t.Probe) {
				continue
			}
			if t.MinVersion != "" && !versionAtLeast(s.version(p), t.MinVersion) {
				continue
			}
			return p, true
		}
	}
	if t.Locate != nil {
		return t.Locate(s.Sysroot, s.OS)
	}
	return "", false
}

// lookAll is the program NAME on PATH: the first match, or with all every
// match in PATH order (type -aP), so that a probe that refuses the first
// (macOS /bin/bash 3.2) can accept a later one.
func (s *System) lookAll(name string, all bool) []string {
	first, ok := s.lookPath(name)
	if !ok {
		return nil
	}
	if !all || s.LookPath != nil {
		return []string{first}
	}
	out := []string{first}
	path := ""
	for _, kv := range s.Env {
		if v, ok := strings.CutPrefix(kv, "PATH="); ok {
			path = v
			break
		}
	}
	seen := map[string]bool{first: true}
	for _, d := range strings.Split(path, ":") {
		if d == "" || !filepath.IsAbs(d) {
			continue
		}
		p := d + "/" + name
		if seen[p] {
			continue
		}
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0 {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// Has reports whether the named tool is installed (an unknown name is
// looked up as a program).
func (s *System) Has(name string) bool {
	t, ok := Lookup(name)
	if !ok {
		_, found := s.lookPath(name)
		return found
	}
	_, found := s.Find(t)
	return found
}

// version is the first N.N in PATH --version ("" when it prints none).
func (s *System) version(path string) string {
	var out string
	if s.RunVersion != nil {
		out = s.RunVersion(path, s.Env)
	} else {
		out = runVersion(path, s.Env)
	}
	return versionRe.FindString(out)
}

var versionRe = lazyre.MustCompile(`[0-9]+\.[0-9]+`)

// versionAtLeast compares two N.N versions numerically.
func versionAtLeast(have, want string) bool {
	if have == "" {
		return false
	}
	hMaj, hMin, _ := strings.Cut(have, ".")
	wMaj, wMin, _ := strings.Cut(want, ".")
	if a, b := atoi(hMaj), atoi(wMaj); a != b {
		return a > b
	}
	return atoi(hMin) >= atoi(wMin)
}

func atoi(s string) int {
	n := 0
	for i := 0; i < len(s) && s[i] >= '0' && s[i] <= '9' && n < 1<<20; i++ {
		n = n*10 + int(s[i]-'0')
	}
	return n
}

func (s *System) probe(path string, args []string) bool {
	if s.RunProbe != nil {
		return s.RunProbe(path, args, s.Env)
	}
	return runProbe(path, args, s.Env)
}

// xcodeStubs are the programs macOS ships in /usr/bin as stubs that ask
// to install the Command Line Tools.
var xcodeStubs = map[string]bool{
	"git": true, "python3": true, "pip3": true, "make": true, "cc": true, "gcc": true,
	"clang": true, "c++": true, "g++": true, "svn": true,
}

func xcodeStub(path string) bool {
	return filepath.Dir(path) == "/usr/bin" && xcodeStubs[filepath.Base(path)]
}

// xcode reports whether the developer tools behind a /usr/bin stub are
// installed: the Command Line Tools, or the developer directory
// xcode-select points at.
func (s *System) xcode(name string) bool {
	if s.Xcode != nil {
		return s.Xcode(name)
	}
	if _, err := os.Stat("/Library/Developer/CommandLineTools/usr/bin/" + name); err == nil {
		return true
	}
	if dir, err := os.Readlink("/var/db/xcode_select_link"); err == nil {
		for _, d := range []string{dir + "/usr/bin/", dir + "/Toolchains/XcodeDefault.xctoolchain/usr/bin/"} {
			if _, err := os.Stat(d + name); err == nil {
				return true
			}
		}
	}
	return false
}

// Package is the package that provides t here ("" when none is known).
func (s *System) Package(t *Tool) string {
	return t.Packages[s.Manager.names()]
}

// builtin reports a tool that ships with this OS.
func (s *System) builtin(t *Tool) bool {
	for _, b := range t.Builtin {
		if b == s.OS.GOOS {
			return true
		}
	}
	return false
}

// InstallArgv is the command that installs packages here, sudo included
// when the user is not root (nil when there is no known manager). The
// RHEL rebuilds get EPEL enabled first when epel is set: then the result
// is two commands.
func (s *System) InstallArgv(epel bool, pkgs ...string) [][]string {
	if len(pkgs) == 0 || s.Manager == NoManager {
		return nil
	}
	var cmds [][]string
	if epel && s.el() {
		cmds = append(cmds, s.withSudo(append(s.installWords(), s.epelRelease())))
	}
	words := s.installWords()
	for _, p := range pkgs {
		words = append(words, strings.Fields(p)...) // "httpd mod_ssl mod_http2"
	}
	return append(cmds, s.withSudo(words))
}

// RefreshArgv is the command that fetches the package lists before an
// install, nil when the manager does it itself: apt-get update and apk
// update always (a container ships without them), pacman -Sy only when the
// databases are missing (otherwise it would leave a partial upgrade).
func (s *System) RefreshArgv() []string {
	switch {
	case s.Manager == Apt:
		return s.withSudo([]string{"apt-get", "update"})
	case s.Manager == Apk:
		return s.withSudo([]string{"apk", "update"})
	case s.Manager == Pacman && s.NoPacmanDB:
		return s.withSudo([]string{"pacman", "-Sy"})
	}
	return nil
}

func (s *System) installWords() []string {
	switch s.Manager {
	case Apt:
		return []string{"apt-get", "install", "-y"}
	case Dnf, Microdnf, Yum:
		return []string{string(s.Manager), "install", "-y"}
	case Pacman:
		return []string{"pacman", "-S", "--needed", "--noconfirm"}
	case Zypper:
		return []string{"zypper", "--non-interactive", "install"}
	case Apk:
		return []string{"apk", "add"}
	case Brew:
		return []string{"brew", "install"}
	}
	return nil
}

// epelRelease is the package (or URL) that enables EPEL on this rebuild.
func (s *System) epelRelease() string {
	major, _, _ := strings.Cut(s.OS.VersionID, ".")
	switch s.OS.ID {
	case "ol":
		return "oracle-epel-release-el" + major
	case "rhel":
		return "https://dl.fedoraproject.org/pub/epel/epel-release-latest-" + major + ".noarch.rpm"
	}
	return "epel-release"
}

// withSudo puts sudo in front of argv when the user is neither root nor
// on Homebrew (which refuses root).
func (s *System) withSudo(argv []string) []string {
	if s.Root || s.Manager == Brew || !s.Sudo {
		return argv
	}
	return append([]string{"sudo"}, argv...)
}

// NeedsRoot reports that the install commands must run as root, and sudo
// is missing: they are shown, not run.
func (s *System) NeedsRoot() bool {
	return !s.Root && !s.Sudo && s.Manager != Brew && s.Manager != NoManager
}

// ShellJoin renders commands as one line of shell, joined with &&.
func ShellJoin(cmds [][]string) string {
	var parts []string
	for _, argv := range cmds {
		words := make([]string, len(argv))
		for i, w := range argv {
			words[i] = shellWord(w)
		}
		parts = append(parts, strings.Join(words, " "))
	}
	return strings.Join(parts, " && ")
}

func shellWord(w string) string {
	if w != "" && strings.Trim(w, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./:@+=,") == "" {
		return w
	}
	return "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
}

// Users are the functions and commands that need the named tool, with the
// case that needs it: "llm_procs (--port)", sorted.
func Users(tool string) []string {
	var out []string
	host := false
	for _, name := range registry.NeedNames() {
		for _, n := range registry.NeedsOf(name) {
			if n.Tool != tool {
				continue
			}
			if n.When == "--host" {
				host = true
				break
			}
			if n.When != "" {
				out = append(out, name+" ("+n.When+")")
			} else {
				out = append(out, name)
			}
			break
		}
	}
	if host {
		out = append([]string{"every --host call"}, out...)
	}
	return out
}

// Fresh returns the system with the lookups forgotten, to look again after
// an install.
func (s *System) Fresh() *System {
	return &System{OS: s.OS, Manager: s.Manager, Root: s.Root, Sudo: s.Sudo, Brew: s.Brew, Env: s.Env,
		LookPath: s.LookPath, RunProbe: s.RunProbe, RunVersion: s.RunVersion, Xcode: s.Xcode}
}
