// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
)

// ifsFields is the word splitting of an unquoted $VAR with the default
// IFS: runs of space, tab and newline separate words, empty words go.
// (Pathname expansion of the words is not repeated: docker prints IDs
// and names, which hold no glob characters.)
func ifsFields(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' })
}

// stopAll is docker_stop_all [-y]: the running containers, the plan,
// the Y/n question (headless or without a terminal it needs -y, LEDGER
// DOCK-22), then one `docker stop` of every ID.
func (m *Module) stopAll(c *invoke.Call) int {
	return m.startStop(c, "docker_stop_all", []string{"ps"}, "No running containers to stop.", "Stopping Docker containers...", "stop")
}

// startAll is docker_start_all [-y]: the same for the exited and created
// containers and `docker start`.
func (m *Module) startAll(c *invoke.Call) int {
	return m.startStop(c, "docker_start_all", []string{"ps", "-a", "--filter", "status=exited", "--filter", "status=created"},
		"No stopped containers to start.", "Starting Docker containers...", "start")
}

// listFormat lists a container as its ID and its names.
const listFormat = "{{.ID}}\t{{.Names}}"

func (m *Module) startStop(c *invoke.Call, fn string, list []string, none, doing, verb string) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	x := m.begin(c)
	yes := "0"
	for _, a := range c.Args {
		switch a {
		case "-y", "--yes":
			yes = "1"
		default:
			x.p.Error("Unknown option: " + a + " (usage: " + fn + " [-y])")
			return 1
		}
	}
	if !x.requireDocker() {
		return 1
	}
	out, ok := x.list(append(list, "--format", listFormat)...)
	if !ok {
		return 1
	}
	var ids, names []string
	for _, l := range lines(out) {
		f := strings.SplitN(l, "\t", 2)
		id := strings.TrimSpace(f[0])
		if id == "" {
			continue
		}
		name := id
		if len(f) == 2 && strings.TrimSpace(f[1]) != "" {
			name = strings.TrimSpace(f[1])
		}
		ids, names = append(ids, id), append(names, name)
	}
	if len(ids) == 0 {
		x.p.Info(none)
		return 0
	}
	what := "stop"
	if verb == "start" {
		what = "start"
	}
	_, _ = io.WriteString(c.Stdout, "Will "+what+" "+strconv.Itoa(len(ids))+" container(s): "+strings.Join(names, " ")+"\n")
	if !x.s.ConfirmPlan(c.Term, c.Stderr, yes) {
		return 1
	}
	x.p.Info(doing)
	x.run(c.Stdout, io.Discard, "docker", append([]string{verb}, ids...)...)
	return 0
}

// list is one docker listing call: its output, or, when docker fails,
// `Error: Cannot list Docker objects: <docker's first stderr line>` and
// false (LEDGER DOCK-20: 2.x read a failing listing as an empty one).
func (x *call) list(args ...string) ([]byte, bool) {
	res := x.run(nil, nil, "docker", args...)
	if res.ExitCode == 0 {
		return res.Stdout, true
	}
	first, _, _ := strings.Cut(strings.TrimLeft(string(res.Stderr), "\n"), "\n")
	x.p.Error("Cannot list Docker objects: " + strings.TrimRight(first, "\r"))
	return nil, false
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
// Docker's own bridge, host and none, matched as whole lines; false when
// the listing failed (reported by list).
func (x *call) userNetworks() ([]string, bool) {
	res, ok := x.list("network", "ls", "--format", "{{.Name}}")
	if !ok {
		return nil, false
	}
	var out []string
	for _, l := range lines(res) {
		if l != "bridge" && l != "host" && l != "none" {
			out = append(out, l)
		}
	}
	return out, true
}

const pruneUsage = "docker_prune_all [-y] [--dry-run]"

// pruneAll is docker_prune_all [-y] [--dry-run]: the counts, then (after
// the question unless -y) stop, rm, rmi, volume rm, network rm and
// system prune. Headless or without a terminal it needs -y (LEDGER
// DOCK-21); a failing listing stops it before any action (DOCK-20).
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
	conts, ok := x.list("ps", "-aq")
	if !ok {
		return 1
	}
	imgs, ok := x.list("images", "-q")
	if !ok {
		return 1
	}
	vols, ok := x.list("volume", "ls", "-q")
	if !ok {
		return 1
	}
	nets, ok := x.userNetworks()
	if !ok {
		return 1
	}
	x.p.Info("To remove: " + strconv.Itoa(lineCount(conts)) + " container(s), " + strconv.Itoa(uniqueLines(imgs)) + " image(s), " +
		strconv.Itoa(lineCount(vols)) + " volume(s), " + strconv.Itoa(len(nets)) + " network(s), plus the build cache.")
	if dryRun {
		return 0
	}
	if !assumeYes {
		x.p.Warning("This cannot be undone: volume data is lost.")
		if x.noTerminal() {
			x.p.Error("No terminal to ask for confirmation: pass -y to go ahead.")
			return 1
		}
		if !x.s.Confirm(c.Term, c.Stderr, "Remove all Docker containers, images, volumes, and networks?") {
			x.p.Info("Aborted.")
			return 1
		}
	}

	removal := func(n, msg string, list []string, verb ...string) bool {
		x.p.Step(n, "6", msg)
		out, ok := x.list(list...)
		if !ok {
			return false
		}
		if ids := ifsFields(runner.CommandSubst(out)); len(ids) > 0 {
			x.run(c.Stdout, io.Discard, "docker", append(verb, ids...)...)
		}
		return true
	}
	if !removal("1", "Stopping Docker containers...", []string{"ps", "-q"}, "stop") ||
		!removal("2", "Removing Docker containers...", []string{"ps", "-aq"}, "rm") ||
		!removal("3", "Removing Docker images...", []string{"images", "-q"}, "rmi") ||
		!removal("4", "Removing Docker volumes...", []string{"volume", "ls", "-q"}, "volume", "rm") {
		return 1
	}

	x.p.Step("5", "6", "Removing Docker networks...")
	// The loop reads the names from a pipe, which each docker network rm
	// inherits as its stdin.
	nets, ok = x.userNetworks()
	if !ok {
		return 1
	}
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

// noTerminal is a question that cannot be asked: headless mode, or no
// controlling terminal.
func (x *call) noTerminal() bool {
	if x.headless() {
		return true
	}
	f, err := x.Term.OpenTTY()
	if err != nil {
		return true
	}
	_ = f.Close()
	return false
}
