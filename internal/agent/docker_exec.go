// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package agent

import (
	"strings"

	"github.com/mauriziofonte/gash/internal/docker"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmexec"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/pathguard"
	"github.com/mauriziofonte/gash/internal/runner"
)

// dockerExec is agent_docker_exec CTR [--user U] [--dry-run] -- CMD...
// (plan P5, A1): the write half of llm_docker_exec, same shape, any
// command (php artisan migrate, composer install, chown). The words reach
// docker exec as an array, no shell runs. The rails that stay: the
// dangerous-command blocklist of llm_exec (rm -rf /, mkfs, dd to a disk,
// ...) and no secret file named in the command (A5). --dry-run prints the
// docker argv and runs nothing.
func (m *Module) dockerExec(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	o := llmfmt.New(llmfmt.BudgetFlags)
	dry := false
	ctr, user, cmd, ok := docker.TakeExec(c, o, func(a string) int {
		if a == "--dry-run" {
			dry = true
			return 1
		}
		return 0
	})
	if !ok {
		return 1
	}
	if _, err := llmexec.Build(strings.Join(cmd, " "), llmexec.Settings{Exec: "unrestricted"}, llmexec.Environ{}, nil); err != nil {
		return refusal(c, err)
	}
	for _, w := range cmd {
		_, v, _ := strings.Cut(w, "=")
		if pathguard.SecretName(w) || (v != "" && pathguard.SecretName(v)) {
			return fail(c, "secret_file_blocked", w, "agent_docker_exec never names a secret file (.env, keys, credentials); ask the user")
		}
	}
	args := docker.ExecArgs(ctr, user, cmd)
	if dry {
		var b strings.Builder
		b.WriteString(`{"argv":[`)
		for i, a := range append([]string{"docker"}, args...) {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(llmfmt.Quote(a))
		}
		b.WriteString(`],"dry_run":true,"written":false}` + "\n")
		_, _ = c.Stdout.Write([]byte(b.String()))
		return 0
	}
	bin, err := runner.LookPath("docker", c.Env.Child(), "")
	if err != nil {
		return fail(c, "docker_not_found", "docker is not installed", "Docker is not available here")
	}
	res, err := m.run.Run(c.Ctx, runner.Cmd{Path: bin, Args: args, Options: runner.Options{
		Env: append(c.Env.Child(), "LC_ALL=C"),
	}})
	if err != nil && res.ExitCode == 0 {
		res.ExitCode = 1
	}
	if res.ExitCode != 0 && len(res.Stdout) == 0 {
		if class := docker.Failure(res.Stderr); class != "docker_error" {
			return fail(c, class, docker.FirstLine(res.Stderr), docker.FailureHint(class))
		}
	}
	out := docker.ShapeExec(o, res.ExitCode, res.Stdout, res.Stderr)
	c.NoteTruncated(out.Truncated)
	_ = llmfmt.JSONLine(c.Stdout, out)
	return 0
}
