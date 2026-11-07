// Gash 3.5. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package complete

import (
	"bufio"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/config"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
)

// probeTimeout bounds each program a completion runs (git, docker,
// systemctl): a slow daemon costs one empty completion, never a hung TAB.
const probeTimeout = 800 * time.Millisecond

// providers read the values of an argument kind where they live.
var providers = map[registry.ArgKind]func(Env) []Candidate{
	registry.ArgHost:      hosts,
	registry.ArgDB:        databases,
	registry.ArgGitTag:    gitTags,
	registry.ArgGitRef:    gitRefs,
	registry.ArgContainer: containers,
	registry.ArgUnit:      units,
	registry.ArgProc:      procs,
	registry.ArgPort:      ports,
}

// envFile is the ~/.gash_env of the caller.
func envFile(env Env) *config.File {
	home := env.get("HOME")
	return config.Load(config.Options{Path: config.FilePath(home, env.get("GASH_ENV_FILE")), Home: home, Headless: true})
}

func (e Env) get(name string) string {
	if e.Get == nil {
		return os.Getenv(name)
	}
	return e.Get(name)
}

// hosts are the HOST: aliases, described by their target (no secret).
func hosts(env Env) []Candidate {
	var out []Candidate
	for _, h := range envFile(env).Hosts {
		out = append(out, Candidate{h.Name, h.Value})
	}
	return out
}

// databases are the DB: names, described by their URL with the password
// masked.
func databases(env Env) []Candidate {
	var out []Candidate
	for _, d := range envFile(env).DB {
		out = append(out, Candidate{d.Name, config.MaskDBURL(d.Value)})
	}
	return out
}

// probe runs a program for its output lines; nil when it fails.
func probe(env Env, prog string, args ...string) []string {
	if env.Run == nil {
		return nil
	}
	res, err := env.Run.Run(env.Ctx, runner.Cmd{Path: prog, Args: args, Options: runner.Options{
		Env: env.Environ, Dir: env.Dir, Timeout: probeTimeout, KillAfter: 200 * time.Millisecond, Setsid: true,
	}})
	if err != nil || !res.Success() {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(res.Stdout), "\n") {
		if l = strings.TrimRight(l, "\r"); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func gitTags(env Env) []Candidate {
	var out []Candidate
	for _, t := range probe(env, "git", "tag", "-l", "--sort=-creatordate") {
		out = append(out, Candidate{t, ""})
	}
	return out
}

func gitRefs(env Env) []Candidate {
	var out []Candidate
	for _, l := range probe(env, "git", "for-each-ref", "--format=%(refname:short)\t%(refname:rstrip=-2)", "refs/heads", "refs/tags", "refs/remotes") {
		name, kind, _ := strings.Cut(l, "\t")
		desc := map[string]string{"refs/heads": "branch", "refs/tags": "tag", "refs/remotes": "remote branch"}[kind]
		if strings.HasSuffix(name, "/HEAD") {
			continue
		}
		out = append(out, Candidate{name, desc})
	}
	return out
}

func containers(env Env) []Candidate {
	var out []Candidate
	for _, l := range probe(env, "docker", "ps", "-a", "--format", "{{.Names}}\t{{.Status}}") {
		name, status, _ := strings.Cut(l, "\t")
		out = append(out, Candidate{name, status})
	}
	return out
}

func units(env Env) []Candidate {
	var out []Candidate
	for _, l := range probe(env, "systemctl", "list-units", "--type=service", "--all", "--no-legend", "--plain", "--no-pager") {
		f := strings.Fields(l)
		if len(f) < 4 {
			continue
		}
		name := strings.TrimSuffix(f[0], ".service")
		out = append(out, Candidate{name, f[3] + " " + strings.Join(f[4:], " ")})
	}
	return out
}

// procs are the command names of the running processes, once each.
func procs(env Env) []Candidate {
	seen := map[string]bool{}
	var names []string
	add := func(n string) {
		n = strings.TrimSpace(n)
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	if runtime.GOOS == "linux" {
		dirs, _ := filepath.Glob("/proc/[0-9]*/comm")
		for _, d := range dirs {
			if b, err := os.ReadFile(d); err == nil {
				add(string(b))
			}
		}
	} else {
		for _, l := range probe(env, "ps", "-A", "-o", "comm=") {
			add(filepath.Base(l))
		}
	}
	sort.Strings(names)
	out := make([]Candidate, 0, len(names))
	for _, n := range names {
		out = append(out, Candidate{n, ""})
	}
	return out
}

// ports are the TCP ports something listens on (Linux: /proc/net/tcp*).
func ports(env Env) []Candidate {
	seen := map[int]bool{}
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		fh, err := os.Open(f)
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(fh)
		for sc.Scan() {
			fields := strings.Fields(sc.Text())
			if len(fields) < 4 || fields[3] != "0A" { // LISTEN
				continue
			}
			_, port, ok := strings.Cut(fields[1], ":")
			if n, err := strconv.ParseUint(port, 16, 16); ok && err == nil {
				seen[int(n)] = true
			}
		}
		_ = fh.Close()
	}
	list := make([]int, 0, len(seen))
	for p := range seen {
		list = append(list, p)
	}
	sort.Ints(list)
	out := make([]Candidate, 0, len(list))
	for _, p := range list {
		out = append(out, Candidate{strconv.Itoa(p), "listening"})
	}
	return out
}
