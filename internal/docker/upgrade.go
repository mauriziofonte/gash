// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"io"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

const upgradeHelp = `docker_compose_upgrade - Upgrade Docker Compose services

USAGE:
  docker_compose_upgrade [PATH] [OPTIONS]

OPTIONS:
  PATH        Directory containing docker-compose.yml (default: current dir)
  --dry-run   Show what would be done without executing
  --force     Force upgrade even for pinned versions
  --yes       Skip the interactive confirmation (REQUIRED in headless mode)
  --no-color  Disable ANSI colors
  -h          Show this help

BEHAVIOR:
  Only upgrades services with mutable tags (latest, main, dev, etc.).
  Services with pinned versions (e.g., nginx:1.25.3) are skipped.
  Use --force to override and upgrade all services.
  In headless mode (GASH_HEADLESS=1) the upgrade refuses to run without
  an explicit --yes: pulling images and restarting a stack must never
  happen as a silent side effect of automation.

EXAMPLES:
  docker_compose_upgrade                  Upgrade in current directory
  docker_compose_upgrade /path/to/app     Upgrade specific project
  docker_compose_upgrade --dry-run        Preview changes
`

// upgrade is docker_compose_upgrade [PATH] [--dry-run] [--force] [--yes]
// [--no-color]: the plan, then pull, stop, up -d --remove-orphans of the
// services with mutable tags, docker image prune -f and ps. Every
// compose call carries -f FILE --project-directory DIR (LEDGER DOCK-04,
// kept), service names go without "--" (DOCK-31, kept), a file without
// image services plans one empty service (DOCK-05, kept) and a declined
// upgrade returns 0 (DOCK-18, kept).
func (m *Module) upgrade(c *invoke.Call) int {
	if isHelp(c) {
		_, _ = io.WriteString(c.Stdout, upgradeHelp)
		return 0
	}
	x := m.begin(c)
	if !x.requireDocker() || !x.requireCompose() {
		return 1
	}
	path := "."
	dryRun, force, assumeYes, noColor := false, false, false, false
	for _, a := range c.Args {
		switch {
		case a == "--dry-run":
			dryRun = true
		case a == "-n":
			// -n means "limit N" across Gash 2.x; dry-run is spelled out.
			x.p.Error("-n is not --dry-run anymore; use --dry-run")
			return 1
		case a == "--force" || a == "-f":
			force = true
		case a == "--yes" || a == "-y":
			assumeYes = true
		case a == "--no-color":
			noColor = true
		case strings.HasPrefix(a, "-"):
			x.p.Error("Unknown option: " + a)
			return 1
		default:
			path = a
		}
	}
	x.colors(noColor)
	w, s := c.Stdout, x.s
	put := func(t string) { _, _ = io.WriteString(w, t) }

	path = x.resolveDir(path)
	file := composeFile(path)
	if file == "" {
		x.p.Error("No docker-compose.yml found in " + path)
		return 1
	}
	dc := append(ifsFields(strings.Join(x.composeCmd(), " ")), "-f", file, "--project-directory", path)

	s.Section(w, "Docker Compose Upgrade")
	s.KV(w, "--value-color", "accent", "Path:", path)
	put("\n")
	if dryRun {
		s.Mark(w, "warning", "Dry run: no changes will be made")
		put("\n")
	}

	// One plan row per service: status mark, name, image; the mark's
	// color in hidden column 4. The marks are __gash_mark's.
	markUp, markSkip := "OK:", "Info:"
	if s.Color {
		markUp, markSkip = s.G.OK, s.G.Info
	}
	var services, rows []string
	envPath := path + "/.env"
	for _, line := range hereLines(x.parseServices(file)) {
		f := readFields(line, 2)
		resolved := x.resolveEnvVars(f[1], envPath)
		if force || isUpgradeable(normalizeImage(resolved).tag) {
			services = append(services, f[0])
			rows = append(rows, markUp+"\t"+f[0]+"\t("+resolved+")\tsuccess")
		} else {
			rows = append(rows, markSkip+"\t"+f[0]+"\t("+resolved+") - pinned version\tinfo")
		}
	}
	s.Table(w, ui.ReadRows(strings.NewReader(strings.Join(rows, "\n")+"\n")),
		"--no-header", "--indent", "2", "--gap", "1", "--cell-color", "1:4", "--color", "2:accent", "--color", "3:muted")
	put("\n")

	if len(services) == 0 {
		s.Print(w, "warning", "No services to upgrade (all pinned).")
		s.Print(w, "muted", "Use --force to upgrade pinned versions.")
		return 0
	}
	dcs, svcs := strings.Join(dc, " "), strings.Join(services, " ")
	if dryRun {
		s.Heading(w, "Would run:")
		put("  " + dcs + " pull " + svcs + "\n")
		put("  " + dcs + " stop " + svcs + "\n")
		put("  " + dcs + " up -d --remove-orphans " + svcs + "\n")
		put("  docker image prune -f\n")
		return 0
	}

	// --yes skips the question; headless the upgrade refuses without it.
	if !assumeYes {
		if x.headless() {
			x.p.Error("Refusing to upgrade in headless mode without --yes (pull + restart is destructive).")
			return 1
		}
		put("\n")
		s.Heading(w, "This will:")
		s.List(w, "pull new images for: "+svcs,
			"stop and recreate those containers",
			"remove containers of this project no longer in "+file[strings.LastIndexByte(file, '/')+1:]+" (--remove-orphans)",
			"remove dangling images on this host, from any project (docker image prune -f)")
		if !s.Confirm(c.Term, c.Stderr, "Proceed with upgrade?") {
			x.p.Info("Upgrade cancelled.")
			return 0
		}
	}

	compose := func(stdout, stderr io.Writer, args ...string) int {
		argv := append(append(append([]string{}, dc[1:]...), args...), services...)
		return x.run(stdout, stderr, dc[0], argv...).ExitCode
	}
	x.p.Step("1", "4", "Pulling updated images...")
	if compose(c.Stdout, c.Stderr, "pull") != 0 {
		x.p.Error("Failed to pull images. Aborting upgrade.")
		return 1
	}
	x.p.Step("2", "4", "Stopping old containers...")
	compose(c.Stdout, io.Discard, "stop")
	x.p.Step("3", "4", "Starting updated containers...")
	if compose(c.Stdout, c.Stderr, "up", "-d", "--remove-orphans") != 0 {
		x.p.Error("Failed to start containers. Check logs with: " + dcs + " logs")
		return 1
	}
	x.p.Step("4", "4", "Cleaning old images...")
	x.run(io.Discard, io.Discard, "docker", "image", "prune", "-f")
	put("\n")
	s.Mark(w, "ok", "Upgrade Complete")
	put("\n")
	return x.run(c.Stdout, c.Stderr, dc[0], append(append([]string{}, dc[1:]...), "ps")...).ExitCode
}
