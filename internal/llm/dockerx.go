// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/mauriziofonte/gash/internal/docker"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// dockerLogTail is the default --tail of llm_docker_logs.
const dockerLogTail = 100

// dockerRun runs docker with args and the short environment of
// llm_docker_check (no secret reaches it). Without docker it writes
// docker_not_found (CONTINUE) and returns false.
func (m *Module) dockerRun(c *invoke.Call, args []string, o runner.Options) (runner.Result, bool) {
	bin, ok := m.lookPath(c, "docker")
	if !ok {
		fail(c, "docker_not_found", "docker is not installed", "Docker is not available here. Skip this check")
		return runner.Result{}, false
	}
	o.Env = append(dockerEnv(c.Env).Child(), "LC_ALL=C")
	res, err := m.run.Run(c.Ctx, runner.Cmd{Path: bin, Args: args, Options: o})
	if err != nil && res.ExitCode == 0 {
		res.ExitCode = 1
	}
	return res, true
}

// dockerFail writes the envelope of a failed docker call.
func dockerFail(c *invoke.Call, stderr []byte) int {
	class := docker.Failure(stderr)
	details := docker.FirstLine(stderr)
	if details == "" {
		details = "docker failed"
	}
	return fail(c, class, details, docker.FailureHint(class))
}

// dockerPS is llm_docker_ps (plan P4): every container, by name, as
// [{name,image,state,health,ports,up}] instead of docker ps --format
// strings rebuilt per call; --running keeps the running ones.
func (m *Module) dockerPS(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.FormatFlag | llmfmt.BudgetFlags)
	running := false
	args := c.Args
	for len(args) > 0 {
		n, ok := o.Take(c, args)
		if !ok {
			return 1
		}
		if n > 0 {
			args = args[n:]
			continue
		}
		if args[0] != "--running" {
			if !strings.HasPrefix(args[0], "-") {
				return fail(c, "unexpected_argument", args[0], "llm_docker_ps takes no argument: it lists every container (--running for the running ones)")
			}
			return fail(c, "unknown_option", args[0], "Use --running, --format F or the output budget options")
		}
		running = true
		args = args[1:]
	}
	res, ok := m.dockerRun(c, docker.PSArgs, runner.Options{})
	if !ok {
		return 1
	}
	if res.ExitCode != 0 {
		return dockerFail(c, res.Stderr)
	}
	t := llmfmt.Table{Cols: []string{"name", "image", "state", "health", "ports", "up"}}
	for _, ct := range docker.ParsePS(res.Stdout) {
		if running && ct.State != "running" {
			continue
		}
		t.Rows = append(t.Rows, []any{ct.Name, ct.Image, ct.State, ct.Health, ct.Ports, ct.Up})
	}
	o.Write(c, o.FormatOr(llmfmt.JSON), t, nil)
	return 0
}

// dockerLogsOut is the answer of llm_docker_logs.
type dockerLogsOut struct {
	Ctr  string   `json:"ctr"`
	Read int      `json:"read"`
	Log  []string `json:"log"`
}

// dockerLogs is llm_docker_logs CTR (plan P4): the last --tail N lines
// (default 100) of the container's output, stdout and stderr in the order
// docker gives them (2>&1), optionally since T and kept by --grep RE;
// each line cut at --width (default 300). "read" is how many lines docker
// returned before --grep.
func (m *Module) dockerLogs(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.BudgetFlags)
	tail, since, grep, ctr := dockerLogTail, "", "", ""
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		switch a {
		case "--width":
			n, ok := o.Take(c, args)
			if !ok {
				return 1
			}
			args = args[n:]
			continue
		case "--tail", "--since", "--grep":
			if len(args) < 2 {
				return fail(c, "missing_value", a, a+" needs a value")
			}
			v := args[1]
			switch a {
			case "--tail":
				if !validUint(c, "tail", v) {
					return 1
				}
				tail = atoi(v)
			case "--since":
				if v == "" || strings.HasPrefix(v, "-") || strings.ContainsAny(v, " \t\n") {
					return fail(c, "invalid_since", v, "Give a duration (10m, 2h) or a timestamp (2026-10-25T10:00:00)")
				}
				since = v
			default:
				grep = v
			}
			args = args[2:]
			continue
		}
		if strings.HasPrefix(a, "-") {
			return fail(c, "unknown_option", a, "Use CTR with --tail N, --since T, --grep RE or --width N")
		}
		if ctr != "" {
			return fail(c, "unexpected_argument", a, "Name one container")
		}
		ctr = a
		args = args[1:]
	}
	if ctr == "" {
		return fail(c, "missing_container", "", "Name the container, e.g. llm_docker_logs web --tail 50")
	}
	var re *regexp.Regexp
	if grep != "" {
		var err error
		if re, err = regexp.Compile(grep); err != nil {
			return fail(c, "invalid_regex", grep, "--grep takes a regular expression (RE2 syntax; (?i) for any case)")
		}
	}
	width := o.Width
	if width == 0 {
		width = waitLineWidth
	}
	dargs := []string{"logs", "--tail", strconv.Itoa(tail)}
	if since != "" {
		dargs = append(dargs, "--since", since)
	}
	dargs = append(dargs, "--", ctr)
	var buf bytes.Buffer
	res, ok := m.dockerRun(c, dargs, runner.Options{Stdout: &buf, Stderr: &buf})
	if !ok {
		return 1
	}
	if res.ExitCode != 0 {
		return dockerFail(c, buf.Bytes())
	}
	out := dockerLogsOut{Ctr: ctr, Log: []string{}}
	for _, l := range textLines(buf.Bytes()) {
		out.Read++
		if re != nil && !re.MatchString(l) {
			continue
		}
		out.Log = append(out.Log, llmfmt.Cut(l, width))
	}
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}

// containerProbe answers the llm_exec tool probes for a command that runs
// in a container: sed and gawk run with --sandbox (added below), so they
// count as available; the diff and grep extras of the host cannot be
// assumed, nor bwrap.
type containerProbe struct{}

func (containerProbe) HasSandboxedSed() bool  { return true }
func (containerProbe) HasSandboxedGawk() bool { return true }
func (containerProbe) HasDiffNoDeref() bool   { return false }
func (containerProbe) IsGNUGrep() bool        { return false }
func (containerProbe) HasBwrap() bool         { return false }

// containerArgv checks cmd under the llm_exec rules (allowlist, option
// rules, secret files, forbidden paths, blocklist) as one command line of
// single-quoted words, and returns the argv to run in the container with
// what llm_exec's child-side wrappers would add: sed and awk under
// --sandbox, grep and rg with the secret-file exclusions. git is refused:
// its rails live in a host-side wrapper. Commands named in
// GASH_LLM_EXEC_ALLOW run as given; GASH_LLM_EXEC=unrestricted does not
// apply (execRules).
func (m *Module) containerArgv(c *invoke.Call, cmd []string) ([]string, error) {
	s, env := m.execRules(c)
	s.Sandbox = "" // the container is the boundary; bwrap does not apply
	quoted := make([]string, len(cmd))
	for i, w := range cmd {
		quoted[i] = "'" + strings.ReplaceAll(w, "'", `'\''`) + "'"
	}
	if _, err := llmexec.Build(strings.Join(quoted, " "), s, env, containerProbe{}); err != nil {
		return nil, err
	}
	for _, w := range strings.FieldsFunc(s.ExecAllow, func(r rune) bool { return r == ',' || r == ':' || r == ' ' || r == '\t' || r == '\n' }) {
		if w == cmd[0] {
			return cmd, nil
		}
	}
	name := cmd[0]
	rest := cmd[1:]
	with := func(prog string, pre ...string) []string {
		return append(append([]string{prog}, pre...), rest...)
	}
	switch name {
	case "sed":
		return with("sed", "--sandbox"), nil
	case "awk", "gawk":
		return with("gawk", "--sandbox"), nil
	case "grep", "egrep", "fgrep":
		return with(name, pathguard.ExecSecretExcludes("grep")...), nil
	case "rg":
		return with("rg", pathguard.ExecSecretExcludes("rg")...), nil
	case "git":
		return nil, &pathguard.Refusal{Code: "command_not_allowed", Details: "git", Action: "STOP",
			Hint: "llm_docker_exec does not run git inside a container (its rails need the host-side wrapper): run git on the host, or ask the user before using agent_docker_exec"}
	}
	return cmd, nil
}

// dockerExec is llm_docker_exec CTR [--user U] -- CMD... (plan P5): a
// read-only command in a container, its words passed to docker exec as
// an array end to end (no shell, no quoting at any level), checked under
// the llm_exec rules first. Prints {"rc","out","err"} with the output
// budget applied to each stream; the exit status is 0 whenever the
// command ran (read rc).
func (m *Module) dockerExec(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.BudgetFlags)
	ctr, user, cmd, ok := docker.TakeExec(c, o, nil)
	if !ok {
		return 1
	}
	argv, err := m.containerArgv(c, cmd)
	if err != nil {
		return refusal(c, err)
	}
	res, ok := m.dockerRun(c, docker.ExecArgs(ctr, user, argv), runner.Options{})
	if !ok {
		return 1
	}
	if res.ExitCode != 0 && len(res.Stdout) == 0 {
		if class := docker.Failure(res.Stderr); class != "docker_error" {
			return dockerFail(c, res.Stderr)
		}
	}
	out := docker.ShapeExec(o, res.ExitCode, res.Stdout, res.Stderr)
	c.NoteTruncated(out.Truncated)
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}
