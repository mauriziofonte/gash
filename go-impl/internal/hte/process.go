// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime"

	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/hte/service"
	"github.com/mauriziofonte/gash/internal/platform"
)

// HelperName is the hosts helper the release ships next to the gash
// binary; install copies it into place.
const HelperName = "gash-hosts-helper"

// ProcessEngine is the Engine of the running process, the one the CLI
// calls. interactive allows sudo to prompt (a terminal, not headless).
func ProcessEngine(interactive bool) (*Engine, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("hte: the path of this binary: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(self); err == nil {
		self = resolved
	}
	u, err := user.LookupId(fmt.Sprint(os.Getuid()))
	if err != nil {
		return nil, fmt.Errorf("hte: uid %d has no user name: %w", os.Getuid(), err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "/"
	}
	var procs service.Procs = service.LinuxProcs{}
	if runtime.GOOS == "darwin" {
		procs = service.DarwinProcs{Runner: proc.OS{}, PS: "/bin/ps"}
	}
	return &Engine{
		Sysroot: platform.Real(), GOOS: runtime.GOOS, Getenv: os.Getenv,
		UID: os.Getuid(), EUID: os.Geteuid(), GID: os.Getgid(), User: u.Username, Cwd: cwd,
		Self: self, Helper: filepath.Join(filepath.Dir(self), HelperName),
		Runner: proc.OS{}, Starter: proc.OS{}, Procs: procs, Interactive: interactive,
	}, nil
}
