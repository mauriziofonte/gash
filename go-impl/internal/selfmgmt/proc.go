// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mauriziofonte/gash/internal/invoke"
)

// procs is the process seam of the package. It stays private until
// internal/runner (built in parallel) lands; then the module takes a
// runner instead and this file goes away.
type procs interface {
	// found reports command -v NAME for an executable on path.
	found(path, name string) bool
	// output runs argv with the caller's exported environment, stdin closed and
	// stderr discarded, and returns stdout as $(...) does (trailing
	// newlines removed); "" when it fails to start.
	output(c *invoke.Call, path string, argv ...string) string
}

type osProcs struct{}

func (osProcs) found(path, name string) bool { return lookPath(path, name) != "" }

func (osProcs) output(c *invoke.Call, path string, argv ...string) string {
	bin := lookPath(path, argv[0])
	if bin == "" {
		return ""
	}
	ctx := c.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, bin, argv[1:]...)
	cmd.Args[0] = argv[0]
	cmd.Env = c.Env.Child()
	var out bytes.Buffer
	cmd.Stdout = &out
	_ = cmd.Run() // `|| true`: the output counts whatever the status
	return strings.TrimRight(out.String(), "\n")
}

// lookPath searches the directories of path (the caller's PATH) for an
// executable regular file, as Bash's command search does; an empty entry
// is the current directory.
func lookPath(path, name string) string {
	if strings.Contains(name, "/") {
		if isExec(name) {
			return name
		}
		return ""
	}
	for _, dir := range filepath.SplitList(path) {
		if dir == "" {
			dir = "."
		}
		p := dir + string(filepath.Separator) + name
		if isExec(p) {
			return p
		}
	}
	return ""
}

func isExec(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}
