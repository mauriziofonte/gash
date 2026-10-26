// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/llmfmt"
	"github.com/mauriziofonte/gash/internal/platform"
	"github.com/mauriziofonte/gash/internal/registry"
	"github.com/mauriziofonte/gash/internal/runner"
	"github.com/mauriziofonte/gash/internal/sysinfo"
)

// sysinfoFacts is the seam of llm_sysinfo: the real system, or a fixture
// in the tests.
func (m *Module) sysinfoFacts(c *invoke.Call, mounts []string) sysinfo.Facts {
	return sysinfo.CollectFacts(sysinfo.FactsEnv{
		Sys:      platform.Real(),
		Hostname: os.Hostname,
		Uname:    sysinfo.Uname,
		Statfs:   sysinfo.Statfs,
		NumCPU:   runtime.NumCPU,
		Now:      time.Now,
		Tool: func(name string, args ...string) ([]byte, bool) {
			p, ok := m.lookPath(c, name)
			if !ok {
				return nil, false
			}
			res, err := m.run.Run(c.Ctx, runner.Cmd{Path: p, Args: args, Options: runner.Options{
				Env: append(c.Env.Child(), "LC_ALL=C"), Timeout: 5 * time.Second,
			}})
			if err != nil || res.ExitCode != 0 {
				return nil, false
			}
			return res.Stdout, true
		},
		Mounts: mounts,
	})
}

// sysinfoLLM is llm_sysinfo (plan R4): load, uptime, CPUs, memory, disks,
// GPUs and OS in one compact JSON object instead of uptime; free -h; df
// -h; nproc; uname; nvidia-smi. --mount PATH (repeatable) names the
// filesystems to report instead of the local disks.
func (m *Module) sysinfoLLM(c *invoke.Call) int {
	if helpAnywhere(c) {
		return 0
	}
	var mounts []string
	args := c.Args
	for len(args) > 0 {
		a := args[0]
		if a != "--mount" {
			if strings.HasPrefix(a, "-") {
				return fail(c, "unknown_option", a, registry.OptionsHint(c.Name))
			}
			return fail(c, "unexpected_argument", a, "llm_sysinfo takes no PATH: use --mount PATH")
		}
		if len(args) < 2 {
			return fail(c, "missing_value", a, a+" needs a path")
		}
		safe, ok := m.validatePath(c, args[1])
		if !ok {
			return 1
		}
		if _, err := os.Stat(safe); err != nil {
			return fail(c, "path_not_found", safe, "Name an existing path on the filesystem to report")
		}
		mounts = append(mounts, safe)
		args = args[2:]
	}
	_ = llmfmt.JSONLine(c.Stdout, m.sysinfoFacts(c, mounts))
	return 0
}
