// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package chrome holds chrome_headless_update of lib/modules/chrome.sh:
// one machine-wide chrome-headless-shell, pinned to the build the latest
// Playwright release expects. curl and jq became net/http and native JSON
// (LEDGER SYS-36); uname, unzip, ldd, apt-cache, sudo, cp, ln and find
// stay CLIs.
package chrome

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/sys/unix"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// The 2.x URLs.
const (
	npmLatest  = "https://registry.npmjs.org/playwright-core/latest"
	cftVersion = "https://googlechromelabs.github.io/chrome-for-testing/"
	cftIndex   = "https://googlechromelabs.github.io/chrome-for-testing/known-good-versions-with-downloads.json"
	pwMirror   = "https://cdn.playwright.dev/builds/cft/"
	binRel     = "chrome-headless-shell-linux64/chrome-headless-shell"
)

// version is the vre of 2.x: four dot-separated numbers.
var version = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$`)

// Module is the chrome handler with its seams: tests replace the runner
// and the HTTP client.
type Module struct {
	run runner.Runner
	// client makes every request; nil is net/http with the proxies and
	// the CA of the environment.
	client *http.Client
}

// New returns the module wired to the real system.
func New() *Module { return &Module{run: runner.Exec{}} }

// Register binds chrome_headless_update to t.
func Register(t *invoke.Table) { New().Register(t) }

// Register binds the function of m to t.
func (m *Module) Register(t *invoke.Table) {
	t.Handle("chrome_headless_update", m.update)
}

// call is one run of chrome_headless_update.
type call struct {
	m   *Module
	c   *invoke.Call
	ctx context.Context
	s   ui.Style
	p   ui.Printer
}

func (k *call) env() []string { return k.c.Env.Child() }

// output is `$(NAME ARGS...)` with stderr passed through.
func (k *call) output(name string, args ...string) (string, int) {
	res, err := k.m.run.Run(k.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: k.env(), Stderr: k.c.Stderr}})
	if se, ok := err.(*runner.StartError); ok {
		write(k.c.Stderr, "bash: "+se.Error()+"\n")
		return "", se.Status
	}
	return runner.CommandSubst(res.Stdout), res.ExitCode
}

// status runs a program with its output discarded.
func (k *call) status(name string, args ...string) int {
	res, err := k.m.run.Run(k.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: k.env(), Stdout: io.Discard, Stderr: io.Discard}})
	if se, ok := err.(*runner.StartError); ok {
		return se.Status
	}
	return res.ExitCode
}

// passthrough runs a program on the caller's terminal.
func (k *call) passthrough(name string, args ...string) int {
	res, err := k.m.run.Run(k.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: k.env(), Stdin: k.c.Stdin, Stdout: k.c.Stdout, Stderr: k.c.Stderr}})
	if se, ok := err.(*runner.StartError); ok {
		write(k.c.Stderr, "bash: "+se.Error()+"\n")
		return se.Status
	}
	return res.ExitCode
}

func (k *call) found(name string) bool {
	_, err := runner.LookPath(name, k.env(), "")
	return err == nil
}

func write(w io.Writer, s string) { _, _ = io.WriteString(w, s) }

// update is chrome_headless_update [-y] [--dry-run].
func (m *Module) update(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	s := ui.StyleOf(c.Term, false)
	k := &call{m: m, c: c, ctx: ctx, s: s, p: s.Printer(c.Stderr)}
	assumeYes, dryRun := false, false
	for _, a := range c.Args {
		switch a {
		case "-y", "--yes":
			assumeYes = true
		case "--dry-run":
			dryRun = true
		default:
			k.p.Error("Unknown option: " + a + " (usage: chrome_headless_update [-y] [--dry-run])")
			return 1
		}
	}

	if sys, _ := k.output("uname", "-s"); sys != "Linux" || k.machine() != "x86_64" {
		sys, _ = k.output("uname", "-s")
		k.p.Error("Chrome for Testing publishes chrome-headless-shell for Linux x86_64 only (this is " + sys + " " + k.machine() + ").")
		return 1
	}
	// curl and jq are not needed any more (SYS-36).
	for _, tool := range []string{"unzip", "ldd"} {
		if !k.found(tool) {
			k.p.Error("chrome_headless_update needs '" + tool + "'.")
			return 1
		}
	}

	root := c.Env.Get("GASH_CHROME_ROOT")
	if root == "" {
		root = "/opt/chrome-headless-shell"
	}
	link := c.Env.Get("GASH_CHROME_LINK")
	if link == "" {
		link = "/usr/local/bin/chrome-headless-shell"
	}

	// The symlink names the current version; the directories under ROOT
	// are the versions kept. The link is resolved, ROOT is not (SYS-23).
	current := ""
	if st, err := os.Lstat(link); err == nil && st.Mode()&os.ModeSymlink != 0 {
		current, _ = readlinkF(link)
		current = strings.TrimPrefix(current, root+"/")
		if i := strings.IndexByte(current, '/'); i >= 0 {
			current = current[:i]
		}
		if !version.MatchString(current) {
			current = ""
		}
	}
	installed := installedVersions(root)

	var pw, target, rev string
	if s.Task(c.Stderr, "Resolving the latest Playwright release", func() int {
		body, _ := m.get(ctx, npmLatest)
		pw = jqRun(body, jqVersion)
		return boolStatus(pw != "")
	}) != 0 {
		k.p.Error("Unable to read playwright-core from the npm registry (network?). Nothing changed.")
		return 1
	}
	if s.Task(c.Stderr, "Reading the pinned browser build", func() int {
		for _, cdn := range []string{"https://cdn.jsdelivr.net/npm/playwright-core@" + pw + "/browsers.json",
			"https://unpkg.com/playwright-core@" + pw + "/browsers.json"} {
			body, ok := m.get(ctx, cdn)
			if !ok {
				continue
			}
			target = jqRun(body, jqShell("browserVersion"))
			rev = jqRun(body, jqShell("revision"))
			if version.MatchString(target) {
				return 0
			}
		}
		return 1
	}) != 0 {
		k.p.Error("No chromium-headless-shell entry in browsers.json of playwright-core " + pw + ". Nothing changed.")
		return 1
	}

	revShown := rev
	if revShown == "" {
		revShown = "?"
	}
	currentShown := current
	if currentShown == "" {
		currentShown = "none"
	}
	s.KV(c.Stdout, "--width", "0",
		"Latest Playwright:", "playwright-core "+pw+" (chromium-headless-shell "+target+", revision "+revShown+")",
		"Installed:", currentShown)
	if current == target {
		write(c.Stdout, "chrome-headless-shell is already the newest version ("+target+").\n")
		return 0
	}

	var url string
	var size int64
	if s.Task(c.Stderr, "Locating the Chrome for Testing download", func() int {
		url, size = m.locate(ctx, target)
		return boolStatus(size > 0)
	}) != 0 {
		k.p.Error("No download found for chrome-headless-shell " + target + " (linux64). Nothing changed.")
		return 1
	}

	// Rollback policy: keep the outgoing version, remove every other old one.
	var prune []string
	for _, v := range installed {
		if v != target && v != current {
			prune = append(prune, v)
		}
	}
	if current == "" {
		s.Heading(c.Stdout, "The following NEW packages will be installed:")
		write(c.Stdout, "  chrome-headless-shell "+target+" (from playwright-core "+pw+")\n")
	} else {
		s.Heading(c.Stdout, "The following packages will be upgraded:")
		write(c.Stdout, "  chrome-headless-shell ("+current+" => "+target+", from playwright-core "+pw+")\n")
		s.KV(c.Stdout, "--width", "0", "Kept for rollback:", current)
	}
	if len(prune) > 0 {
		s.Heading(c.Stdout, "The following versions will be REMOVED:")
		write(c.Stdout, "  "+strings.Join(prune, " ")+"\n")
	}
	write(c.Stdout, "Need to get "+ui.HumanBytes(size)+" of archives from "+url+"\n")

	if dryRun {
		return 0
	}
	yes := "0"
	if assumeYes {
		yes = "1"
	}
	if !s.ConfirmPlan(c.Term, c.Stderr, yes) {
		return 1
	}
	return k.install(root, link, target, url, prune)
}

// machine is `$(uname -m)`.
func (k *call) machine() string {
	m, _ := k.output("uname", "-m")
	return m
}

func boolStatus(ok bool) int {
	if ok {
		return 0
	}
	return 1
}

// installedVersions are the directories of ROOT named like a version, in
// glob order.
func installedVersions(root string) []string {
	ents, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") || !version.MatchString(name) {
			continue
		}
		if st, err := os.Stat(filepath.Join(root, name)); err == nil && st.IsDir() {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// install is the second half: download, checks, the sudo install, the
// pruning.
func (k *call) install(root, link, target, url string, prune []string) int {
	c := k.c
	tmpBase := c.Env.Get("TMPDIR")
	if tmpBase == "" {
		tmpBase = "/tmp"
	}
	tmp, err := os.MkdirTemp(tmpBase, "chrome-headless-shell.")
	if err != nil {
		write(c.Stderr, "mktemp: "+err.Error()+"\n")
		return 1
	}
	zip := tmp + "/chrome-headless-shell-linux64.zip"
	if (ui.Downloader{Client: k.m.client}).Run(k.ctx, k.s, c.Term, c.Stderr, url, zip) != 0 {
		k.deleteTree(tmp, false)
		return 1
	}
	listing, _ := k.quiet("unzip", "-Z1", zip)
	if !hasLine(listing, binRel) {
		k.p.Error("The archive does not contain " + binRel + ".")
		k.deleteTree(tmp, false)
		return 1
	}
	if k.s.Task(c.Stderr, "Unpacking chrome-headless-shell "+target, func() int {
		res, err := k.m.run.Run(k.ctx, runner.Cmd{Path: "unzip", Args: []string{"-q", zip, "-d", tmp + "/x"},
			Options: runner.Options{Env: k.env(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr}})
		if se, ok := err.(*runner.StartError); ok {
			write(c.Stderr, "bash: "+se.Error()+"\n")
			return se.Status
		}
		return res.ExitCode
	}) != 0 {
		k.deleteTree(tmp, false)
		return 1
	}

	// Missing shared libraries: name the packages instead of failing later
	// with a loader error inside Playwright.
	lddOut, _ := k.quiet("ldd", tmp+"/x/"+binRel)
	if missing := notFound(lddOut); len(missing) > 0 {
		seen := map[string]bool{}
		var pkgs []string
		for _, lib := range missing {
			if p := k.m.PkgName(k.ctx, k.env(), LibToPkg(lib)); !seen[p] {
				seen[p] = true
				pkgs = append(pkgs, p)
			}
		}
		sort.Strings(pkgs)
		k.p.Error("Missing shared libraries: " + strings.Join(missing, " ") + " ")
		k.p.Info("Install them, then run chrome_headless_update again:  sudo apt-get install " + strings.Join(pkgs, " "))
		k.deleteTree(tmp, false)
		return 1
	}

	got, _ := k.quiet(tmp+"/x/"+binRel, "--version")
	if !strings.Contains(got, target) {
		shown := got
		if shown == "" {
			shown = "nothing"
		}
		k.p.Error("Version check failed: expected " + target + ", the binary says '" + shown + "'.")
		k.deleteTree(tmp, false)
		return 1
	}
	k.p.Info("Verified: " + got)

	rc := 0
	k.p.Info("Installing into " + root + "/" + target + " (sudo)...")
	// A target already there is linked as it is (SYS-24).
	if unix.Faccessat(unix.AT_FDCWD, root+"/"+target+"/"+binRel, unix.X_OK, unix.AT_EACCESS) != nil {
		if k.passthrough("sudo", "mkdir", "-p", root+"/"+target) != 0 ||
			k.passthrough("sudo", "cp", "-R", "--no-preserve=ownership", tmp+"/x/chrome-headless-shell-linux64", root+"/"+target+"/") != 0 {
			rc = 1
		}
	}
	if rc == 0 && k.passthrough("sudo", "ln", "-sfn", root+"/"+target+"/"+binRel, link) != 0 {
		rc = 1
	}
	k.deleteTree(tmp, false)
	if rc != 0 {
		k.p.Error("Installation into " + root + " failed.")
		return 1
	}
	for _, v := range prune {
		k.p.Info("Removing chrome-headless-shell " + v + "...")
		if k.deleteTree(root+"/"+v, true) != 0 {
			rc = 1
		}
	}
	ver, _ := k.quiet(link, "--version")
	resolved, _ := readlinkF(link)
	k.p.Success("Installed: " + ver + " -> " + resolved)
	return rc
}

// quiet is `$(NAME ARGS 2>/dev/null)`.
func (k *call) quiet(name string, args ...string) (string, int) {
	res, err := k.m.run.Run(k.ctx, runner.Cmd{Path: name, Args: args,
		Options: runner.Options{Env: k.env(), Stderr: io.Discard}})
	if se, ok := err.(*runner.StartError); ok {
		return "", se.Status
	}
	return runner.CommandSubst(res.Stdout), res.ExitCode
}

// deleteTree is __gash_chs_delete_tree DIR [sudo]: find DIR -depth
// -delete, file by file, never on / or a non-directory.
func (k *call) deleteTree(dir string, sudo bool) int {
	if dir == "" || dir == "/" {
		return 0
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return 0
	}
	if sudo {
		return k.passthrough("sudo", "find", dir, "-depth", "-delete")
	}
	return k.passthrough("find", dir, "-depth", "-delete")
}

// hasLine is `grep -qx LINE`.
func hasLine(text, line string) bool {
	for _, l := range strings.Split(text, "\n") {
		if l == line {
			return true
		}
	}
	return false
}

// notFound is `awk '/not found/ { print $1 }' | sort -u` over ldd's
// output.
func notFound(lddOut string) []string {
	seen := map[string]bool{}
	var libs []string
	for _, l := range strings.Split(lddOut, "\n") {
		if !strings.Contains(l, "not found") {
			continue
		}
		f := strings.Fields(l)
		lib := ""
		if len(f) > 0 {
			lib = f[0]
		}
		if !seen[lib] {
			seen[lib] = true
			libs = append(libs, lib)
		}
	}
	sort.Strings(libs)
	return libs
}

// readlinkF is `readlink -f PATH`: every symlink of every component
// resolved; all but the last component must exist.
func readlinkF(p string) (string, bool) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	for i := 0; i < 40; i++ {
		dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
		if err != nil {
			return "", false
		}
		cand := filepath.Join(dir, filepath.Base(abs))
		if filepath.Base(abs) == "/" {
			cand = dir
		}
		st, err := os.Lstat(cand)
		if err != nil || st.Mode()&os.ModeSymlink == 0 {
			return cand, true
		}
		target, err := os.Readlink(cand)
		if err != nil {
			return "", false
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		abs = target
	}
	return "", false
}
