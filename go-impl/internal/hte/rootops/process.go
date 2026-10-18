// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package rootops

import (
	"os"
	"os/user"
	"runtime"
	"strconv"
	"time"

	"github.com/mauriziofonte/gash/internal/hte/discover"
	"github.com/mauriziofonte/gash/internal/hte/legacy"
	"github.com/mauriziofonte/gash/internal/hte/proc"
	"github.com/mauriziofonte/gash/internal/platform"
)

// ProcessEnv is the root side's Env of the running process (gash hte
// __root under sudo).
func ProcessEnv() Env {
	sr := platform.Real()
	o := platform.DetectOS(sr, runtime.GOOS)
	prefix := ""
	if a, err := discover.FindApache(sr, o); err == nil {
		prefix = a.Prefix
	}
	layout, _ := legacy.LayoutFor(o.Family, prefix)
	return Env{
		Sysroot: sr, Runner: proc.OS{}, Getenv: os.Getenv, Euid: os.Geteuid(), Now: time.Now, OS: o, Layout: layout,
		Systemd: sr.IsDir("/run/systemd/system"),
		Groups: func(name string) ([]uint32, error) {
			u, err := user.Lookup(name)
			if err != nil {
				return nil, err
			}
			ids, err := u.GroupIds()
			if err != nil {
				return nil, err
			}
			var out []uint32
			for _, id := range ids {
				n, err := strconv.ParseUint(id, 10, 32)
				if err != nil {
					return nil, err
				}
				out = append(out, uint32(n))
			}
			return out, nil
		},
		Home: func(name string) (string, error) {
			u, err := user.Lookup(name)
			if err != nil {
				return "", err
			}
			return u.HomeDir, nil
		},
	}
}
