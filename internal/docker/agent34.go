// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docker

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"

	"github.com/mauriziofonte/gash/internal/llmfmt"
)

// The 3.4 agent functions over docker (plan P4, P5): llm_docker_ps,
// llm_docker_logs, llm_docker_exec and agent_docker_exec share the
// parsing and the error classes below; the handlers live in internal/llm
// and internal/agent and run docker through their runner seam.

// Container is one row of llm_docker_ps.
type Container struct {
	Name, Image, State, Health, Ports, Up string
}

// PSArgs lists every container as one JSON object per line.
var PSArgs = []string{"ps", "-a", "--no-trunc", "--format", "{{json .}}"}

// ParsePS reads the PSArgs listing, sorted by name; a line that is not a
// JSON object is skipped.
func ParsePS(out []byte) []Container {
	var cs []Container
	for _, l := range bytes.Split(out, []byte{'\n'}) {
		l = bytes.TrimSpace(l)
		if len(l) == 0 {
			continue
		}
		var row struct {
			Names, Image, State, Status, Ports string
		}
		if json.Unmarshal(l, &row) != nil {
			continue
		}
		up, health := splitHealth(row.Status)
		state := row.State
		if state == "" {
			state = stateOf(row.Status)
		}
		cs = append(cs, Container{Name: row.Names, Image: row.Image, State: state, Health: health, Ports: compactPorts(row.Ports), Up: up})
	}
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Name < cs[j].Name })
	return cs
}

// splitHealth cuts the health part off a docker status: "Up 2 hours
// (healthy)" is "Up 2 hours" and "healthy"; "(health: starting)" is
// "starting".
func splitHealth(status string) (string, string) {
	status = strings.TrimSpace(status)
	for _, h := range []struct{ suffix, health string }{
		{"(healthy)", "healthy"}, {"(unhealthy)", "unhealthy"}, {"(health: starting)", "starting"},
	} {
		if strings.HasSuffix(status, h.suffix) {
			return strings.TrimSpace(strings.TrimSuffix(status, h.suffix)), h.health
		}
	}
	return status, ""
}

// stateOf is the state an older docker without .State shows in .Status.
func stateOf(status string) string {
	switch {
	case strings.HasPrefix(status, "Up") && strings.Contains(status, "(Paused)"):
		return "paused"
	case strings.HasPrefix(status, "Up"):
		return "running"
	case strings.HasPrefix(status, "Exited"):
		return "exited"
	case strings.HasPrefix(status, "Restarting"):
		return "restarting"
	case strings.HasPrefix(status, "Created"):
		return "created"
	case strings.HasPrefix(status, "Dead"):
		return "dead"
	}
	return strings.ToLower(status)
}

// compactPorts shortens docker's port list: "0.0.0.0:8080->80/tcp,
// :::8080->80/tcp" is "8080->80/tcp" (one entry for the IPv4 and IPv6
// bindings of all addresses), other addresses kept.
func compactPorts(p string) string {
	if p == "" {
		return ""
	}
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(p, ",") {
		part = strings.TrimSpace(part)
		for _, pre := range []string{"0.0.0.0:", ":::", "[::]:"} {
			if strings.HasPrefix(part, pre) {
				part = part[len(pre):]
				break
			}
		}
		if part != "" && !seen[part] {
			seen[part] = true
			out = append(out, part)
		}
	}
	return strings.Join(out, ", ")
}

// Failure is the error class of a failed docker call from its stderr:
// container_not_found, container_not_running, docker_unavailable (the
// daemon cannot be reached), or docker_error.
func Failure(stderr []byte) string {
	s := strings.ToLower(string(stderr))
	switch {
	case strings.Contains(s, "no such container"):
		return "container_not_found"
	case strings.Contains(s, "is not running") || strings.Contains(s, "is paused") || strings.Contains(s, "is restarting"):
		return "container_not_running"
	case strings.Contains(s, "cannot connect to the docker daemon"),
		strings.Contains(s, "failed to connect to the docker api"),
		strings.Contains(s, "error during connect"),
		strings.Contains(s, "permission denied while trying to connect"),
		strings.Contains(s, "is the docker daemon running"):
		return "docker_unavailable"
	}
	return "docker_error"
}

// FailureHint is the hint of a Failure class.
func FailureHint(class string) string {
	switch class {
	case "container_not_found":
		return "No container by that name or ID: list them with llm_docker_ps"
	case "container_not_running":
		return "The container is not running: check llm_docker_ps, start it only if the user wants that"
	case "docker_unavailable":
		return "The Docker daemon cannot be reached (not running, or no permission on its socket). Skip this check"
	}
	return "docker refused the call: see details"
}

// FirstLine is the first non-empty line of a docker stderr.
func FirstLine(stderr []byte) string {
	for _, l := range strings.Split(string(stderr), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			return l
		}
	}
	return ""
}

// ExecArgs are the docker arguments of `docker exec [--user=U] CTR CMD...`:
// the command is passed as its own words, never through a shell, so no
// word needs quoting (plan P5).
func ExecArgs(ctr, user string, cmd []string) []string {
	args := []string{"exec"}
	if user != "" {
		args = append(args, "--user="+user)
	}
	args = append(args, ctr)
	return append(args, cmd...)
}

// ExecOut is the answer of llm_docker_exec and agent_docker_exec: the
// command's exit status and its output under the output budget.
type ExecOut struct {
	RC        int    `json:"rc"`
	Out       string `json:"out"`
	Err       string `json:"err"`
	Truncated int    `json:"truncated,omitempty"`
}

// ShapeExec applies the output budget of o to each stream: lines cut at
// --width, the first --head and last --tail lines kept with one omitted
// marker line, invalid UTF-8 replaced.
func ShapeExec(o *llmfmt.Options, rc int, stdout, stderr []byte) ExecOut {
	out, n1 := shape(o, stdout)
	errs, n2 := shape(o, stderr)
	return ExecOut{RC: rc, Out: out, Err: errs, Truncated: n1 + n2}
}

func shape(o *llmfmt.Options, b []byte) (string, int) {
	if len(b) == 0 {
		return "", 0
	}
	var buf bytes.Buffer
	w := o.Lines(&buf, "lines")
	s := strings.ToValidUTF8(string(b), "�")
	trail := strings.HasSuffix(s, "\n")
	for _, l := range strings.Split(strings.TrimSuffix(s, "\n"), "\n") {
		w.Line(l)
	}
	n := w.Close()
	text := buf.String()
	if !trail {
		text = strings.TrimSuffix(text, "\n")
	}
	return text, n
}
