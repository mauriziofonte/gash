// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"os"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/ui"
)

// envInit is gash_env_init [--force] (lib/core/config.sh): copies
// $GASH_DIR/.gash_env.template to GASH_ENV_FILE or ~/.gash_env with mode
// 600, refusing an existing file without --force. Any other argument is
// refused before anything is written (3.0: SELF-15 fixed; 2.x ignored it).
//
// The copy and the mode change run cp and chmod from the caller's PATH, as
// 2.x did, so what they print on failure, what a directory or a symlink as
// the target does, and the umask of a new file stay the 2.x ones.
func (m *Module) envInit(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	if badArgs(c, "gash_env_init [--force]", "--force") {
		return usageStatus
	}
	force := c.Arg(0) == "--force"
	target := c.Env.Get("GASH_ENV_FILE")
	if target == "" {
		target = c.Env.Get("HOME") + "/.gash_env"
	}
	template := c.Env.Get("GASH_DIR") + "/.gash_env.template"

	p := ui.StyleOf(c.Term, false).Printer(c.Stderr)
	if isFile(target) && !force {
		p.Warning(target + " already exists. Use --force to overwrite.")
		return 1
	}
	if !isFile(template) {
		p.Error("Template not found: " + template)
		return 1
	}
	if !m.tool(c, "cp", template, target) {
		p.Error("Failed to copy template")
		return 1
	}
	if !m.tool(c, "chmod", "600", target) {
		p.Error("Failed to set permissions")
		return 1
	}
	p.Success("Created " + target)
	p.Info("Edit with your SSH keys and database credentials")
	return 0
}

// isFile is [[ -f PATH ]]: PATH, followed through symlinks, is a regular
// file.
func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular()
}

// tool runs NAME ARGS as a 2.x function body ran a command: from the
// caller's PATH with the caller's exported environment, on the call's
// stdin, stdout and stderr; true when it exits 0. A program that does not
// start is reported as Bash reports it.
func (m *Module) tool(c *invoke.Call, name string, args ...string) bool {
	res, err := m.run.Run(ctxOf(c), runner.Cmd{Path: name, Args: args, Options: runner.Options{
		Env: c.Env.Child(), Stdin: c.Stdin, Stdout: c.Stdout, Stderr: c.Stderr,
	}})
	if se, ok := err.(*runner.StartError); ok {
		_, _ = io.WriteString(c.Stderr, "bash: "+se.Error()+"\n")
		return false
	}
	return err == nil && res.Success()
}
