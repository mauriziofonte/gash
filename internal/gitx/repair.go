// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/lazyre"
	"github.com/mauriziofonte/gash/internal/runner"
)

const repairUsage = "git_repair [-y] [--dry-run]"

// repair is git_repair [-y] [--dry-run]: count the empty files of the
// object store, print the plan, ask Y/n (-y skips it, headless or without
// a terminal it refuses), bare-clone origin into a temporary directory,
// delete the empty files, copy the clone's packs and loose objects in,
// then `git gc --quiet` and `git fsck --full`. An origin URL starting
// with '-' is refused, the URL follows -- in git clone, only empty loose
// objects (OBJECTS/xx/<38 or 62 hex>) count, and nothing is deleted
// before the clone succeeded (LEDGER GIT-08).
func (m *Module) repair(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	assumeYes, dryRun := false, false
	for _, a := range c.Args {
		switch a {
		case "-y", "--yes":
			assumeYes = true
		case "--dry-run":
			dryRun = true
		default:
			x.p.Error("Unknown option: " + a + " (usage: " + repairUsage + ")")
			return 1
		}
	}
	if !x.requireRepoWithRemote() {
		return 1
	}

	objects, ok := x.objectStore()
	if !ok {
		x.p.Error("Cannot locate the object store of this repository.")
		return 1
	}
	originURL, _ := x.gitOut("remote", "get-url", "origin")
	if strings.HasPrefix(originURL, "-") {
		x.p.Error("The origin URL starts with '-': refusing to pass it to git clone.")
		return 1
	}
	empty := emptyFiles(objects)

	x.s.KV(c.Stdout, "--width", "0", "Object store:", objects)
	x.s.KV(c.Stdout, "--width", "0", "Empty (corrupted) object files:", strconv.Itoa(len(empty)))
	if len(empty) == 0 {
		if x.gitQuiet("fsck", "--connectivity-only") {
			x.p.Success("No empty objects and the history is connected: nothing to repair.")
			return 0
		}
		_, _ = io.WriteString(c.Stdout, "git fsck reports problems that are not empty files; trying the repair anyway.\n")
	}
	x.s.KV(c.Stdout, "--width", "5", "Plan:", "make a fresh bare clone of "+originURL+", delete the empty files,",
		"", "copy the packs of the clone, run git gc and git fsck --full.")
	if dryRun {
		return 0
	}
	yes := "0"
	if assumeYes {
		yes = "1"
	}
	if !x.s.ConfirmPlan(c.Term, c.Stderr, yes) {
		return 1
	}

	// tmp_dir=$(mktemp -d), removed on return (rm -rf, silent).
	res := x.cmd(nil, c.Stderr, "mktemp", "-d")
	if !res.Success() {
		x.p.Error("Failed to create temporary directory.")
		return 1
	}
	tmp := runner.CommandSubst(res.Stdout)
	defer func() { _ = os.RemoveAll(tmp) }()

	recovery := tmp + "/recovery.git"
	x.p.Step("1", "5", "Cloning '"+originURL+"' into a temporary bare repo...")
	if !x.git("clone", "--quiet", "--bare", "--", originURL, recovery).Success() {
		x.p.Error("Failed to clone '" + originURL + "': nothing was deleted.")
		return 1
	}

	x.p.Step("2", "5", "Removing "+strconv.Itoa(len(empty))+" corrupted (0-byte) loose object(s)...")
	if len(empty) > 0 {
		if !x.cmd(c.Stdout, c.Stderr, "rm", append([]string{"-f", "--"}, empty...)...).Success() {
			return 1
		}
	}

	x.p.Step("3", "5", "Copying the recovered packs into the repo...")
	if !x.cmd(c.Stdout, c.Stderr, "mkdir", "-p", objects+"/pack").Success() {
		return 1
	}
	copied := 0
	for _, f := range glob(recovery+"/objects/pack", "pack-") {
		if !isFile(f) {
			continue
		}
		name := filepath.Base(f)
		if exists(objects + "/pack/" + name) {
			continue
		}
		if !x.cmd(c.Stdout, c.Stderr, "cp", f, objects+"/pack/").Success() {
			x.p.Error("Copying " + name + " into " + objects + "/pack failed.")
			return 1
		}
		copied++
	}
	for _, f := range looseObjects(recovery + "/objects") {
		if !isFile(f) {
			continue
		}
		rel := strings.TrimPrefix(f, recovery+"/objects/")
		if exists(objects + "/" + rel) {
			continue
		}
		dir := objects + "/" + rel[:strings.LastIndexByte(rel, '/')]
		if !x.cmd(c.Stdout, c.Stderr, "mkdir", "-p", dir).Success() ||
			!x.cmd(c.Stdout, c.Stderr, "cp", recovery+"/objects/"+rel, objects+"/"+rel).Success() {
			x.p.Error("Copying object " + rel + " failed.")
			return 1
		}
	}
	x.p.Info("Copied " + strconv.Itoa(copied) + " pack file(s).")

	x.p.Step("4", "5", "Running garbage collection...")
	if !x.git("gc", "--quiet").Success() {
		x.p.Warning("git gc reported a problem; checking integrity anyway.")
	}

	x.p.Step("5", "5", "Verifying repo integrity...")
	if !x.git("fsck", "--full").Success() {
		x.p.Error("Repo still reports issues after repair. Manual inspection needed.")
		return 1
	}
	x.p.Success("Repo repaired and verified clean.")
	return 0
}

// objectStore is `git rev-parse --path-format=absolute --git-path objects`,
// else the physical path of `git rev-parse --git-path objects` (cd X &&
// pwd -P).
func (x *call) objectStore() (string, bool) {
	if p, ok := x.gitOutQuiet("rev-parse", "--path-format=absolute", "--git-path", "objects"); ok {
		return p, true
	}
	// cd "$(...)" && pwd -P: an empty path is the current directory, as
	// Bash's cd "" is; cd's diagnostic is not printed (LEDGER DOCK-19).
	p, _ := x.gitOut("rev-parse", "--git-path", "objects")
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", false
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return "", false
	}
	return real, true
}

// emptyFiles are the empty loose objects of the store: regular files of
// size 0 named OBJECTS/xx/<38 hex> (SHA-1) or OBJECTS/xx/<62 hex>
// (SHA-256), symlinks neither followed nor matched; an empty pack .keep
// or info/ file is never one (LEDGER GIT-08).
func emptyFiles(root string) []string {
	var out []string
	dirs, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	for _, d := range dirs {
		if !d.IsDir() || !reFanout.MatchString(d.Name()) {
			continue
		}
		entries, err := os.ReadDir(filepath.Join(root, d.Name()))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.Type().IsRegular() || !reLoose.MatchString(e.Name()) {
				continue
			}
			if info, err := e.Info(); err == nil && info.Size() == 0 {
				out = append(out, root+"/"+d.Name()+"/"+e.Name())
			}
		}
	}
	return out
}

var (
	reFanout = lazyre.MustCompile(`^[0-9a-f]{2}$`)
	reLoose  = lazyre.MustCompile(`^([0-9a-f]{38}|[0-9a-f]{62})$`)
)

// glob is DIR/PREFIX*: the entries of dir whose name starts with prefix
// and not with a dot, sorted.
func glob(dir, prefix string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		n := e.Name()
		if strings.HasPrefix(n, prefix) && !strings.HasPrefix(n, ".") {
			out = append(out, dir+"/"+n)
		}
	}
	sort.Strings(out)
	return out
}

// looseObjects is OBJECTS/??/*: entries (no dot files) of every two-letter
// directory of objects (no dot names), sorted.
func looseObjects(objects string) []string {
	dirs, err := os.ReadDir(objects)
	if err != nil {
		return nil
	}
	var out []string
	var names []string
	for _, d := range dirs {
		n := d.Name()
		if len([]rune(n)) == 2 && !strings.HasPrefix(n, ".") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		entries, err := os.ReadDir(objects + "/" + n)
		if err != nil {
			continue
		}
		var files []string
		for _, e := range entries {
			if !strings.HasPrefix(e.Name(), ".") {
				files = append(files, objects+"/"+n+"/"+e.Name())
			}
		}
		sort.Strings(files)
		out = append(out, files...)
	}
	return out
}
