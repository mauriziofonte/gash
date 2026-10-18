// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
)

// ifsFields is the word splitting of an unquoted $VAR with the default
// IFS: runs of space, tab and newline separate words, empty words go.
// (Pathname expansion of the words is not repeated: docker prints IDs
// and names, which hold no glob characters.)
func ifsFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// stopAll is docker_stop_all: `docker stop` of `docker ps -q`, no
// question (LEDGER DOCK-22, kept). A failing listing reads as no
// container and extra arguments are ignored (DOCK-20, kept).
func (m *Module) stopAll(c *invoke.Call) int {
	return m.startStop(c, []string{"ps", "-q"}, "No running containers to stop.", "Stopping Docker containers...", "stop")
}

// startAll is docker_start_all: `docker start` of the exited and created
// containers.
func (m *Module) startAll(c *invoke.Call) int {
	return m.startStop(c, []string{"ps", "-aq", "--filter", "status=exited", "--filter", "status=created"},
		"No stopped containers to start.", "Starting Docker containers...", "start")
}

func (m *Module) startStop(c *invoke.Call, list []string, none, doing, verb string) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	if !x.requireDocker() {
		return 1
	}
	ids, _ := x.subst("docker", list...)
	if ids == "" {
		x.p.Info(none)
		return 0
	}
	x.p.Info(doing)
	x.run(c.Stdout, io.Discard, "docker", append([]string{verb}, ifsFields(ids)...)...)
	return 0
}

// lineCount is `| wc -l`: the newlines of a command's output.
func lineCount(out []byte) int { return strings.Count(string(out), "\n") }

// lines are the lines of a command's output as a reading loop or a
// filter sees them: the last one also without its newline.
func lines(out []byte) []string {
	s := string(out)
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// uniqueLines is `| sort -u | wc -l`: the distinct lines (bytes).
func uniqueLines(out []byte) int {
	seen := map[string]bool{}
	for _, l := range lines(out) {
		seen[l] = true
	}
	return len(seen)
}

// userNetworks is __gash_docker_user_networks: the network names but
// Docker's own bridge, host and none, matched as whole lines.
func (x *call) userNetworks() []string {
	res := x.run(nil, io.Discard, "docker", "network", "ls", "--format", "{{.Name}}")
	var out []string
	for _, l := range lines(res.Stdout) {
		if l != "bridge" && l != "host" && l != "none" {
			out = append(out, l)
		}
	}
	return out
}

const pruneUsage = "docker_prune_all [-y] [--dry-run]"

// pruneAll is docker_prune_all [-y] [--dry-run]: the counts, then (after
// the question unless -y) stop, rm, rmi, volume rm, network rm and
// system prune. Headless without -y it refuses after the warning with
// "Aborted." (LEDGER DOCK-21, kept); the counts of a failing daemon read
// 0 (DOCK-20, kept).
func (m *Module) pruneAll(c *invoke.Call) int {
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
			x.p.Error("Unknown option: " + a + " (usage: " + pruneUsage + ")")
			return 1
		}
	}
	if !x.requireDocker() {
		return 1
	}
	nCont := lineCount(x.run(nil, io.Discard, "docker", "ps", "-aq").Stdout)
	nImg := uniqueLines(x.run(nil, io.Discard, "docker", "images", "-q").Stdout)
	nVol := lineCount(x.run(nil, io.Discard, "docker", "volume", "ls", "-q").Stdout)
	nNet := len(x.userNetworks())
	x.p.Info("To remove: " + strconv.Itoa(nCont) + " container(s), " + strconv.Itoa(nImg) + " image(s), " +
		strconv.Itoa(nVol) + " volume(s), " + strconv.Itoa(nNet) + " network(s), plus the build cache.")
	if dryRun {
		return 0
	}
	if !assumeYes {
		x.p.Warning("This cannot be undone: volume data is lost.")
		if !x.s.Confirm(c.Term, c.Stderr, "Remove all Docker containers, images, volumes, and networks?") {
			x.p.Info("Aborted.")
			return 1
		}
	}

	removal := func(n, msg string, list []string, verb ...string) {
		x.p.Step(n, "6", msg)
		if ids, _ := x.subst("docker", list...); ids != "" {
			x.run(c.Stdout, io.Discard, "docker", append(verb, ifsFields(ids)...)...)
		}
	}
	removal("1", "Stopping Docker containers...", []string{"ps", "-q"}, "stop")
	removal("2", "Removing Docker containers...", []string{"ps", "-aq"}, "rm")
	removal("3", "Removing Docker images...", []string{"images", "-q"}, "rmi")
	removal("4", "Removing Docker volumes...", []string{"volume", "ls", "-q"}, "volume", "rm")

	x.p.Step("5", "6", "Removing Docker networks...")
	// The loop reads the names from a pipe, which each docker network rm
	// inherits as its stdin.
	nets := x.userNetworks()
	for i, net := range nets {
		rest := strings.Join(nets[i+1:], "\n")
		if rest != "" {
			rest += "\n"
		}
		x.runIn(strings.NewReader(rest), c.Stdout, io.Discard, "docker", "network", "rm", net)
	}

	x.p.Step("6", "6", "Cleaning Docker environment...")
	return x.run(c.Stdout, c.Stderr, "docker", "system", "prune", "--volumes", "-a", "--force").ExitCode
}
