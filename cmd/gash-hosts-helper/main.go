// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command gash-hosts-helper edits the gash-managed block of /etc/hosts. It
// is installed root:root 0755 as /usr/local/libexec/gash/gash-hosts-helper
// and is the only command of the sudoers drop-in (design/hte.md 7.1):
//
//	gash-hosts-helper apply   # the desired block on stdin
//	gash-hosts-helper check   # prints the current block
//
// It takes no other argument, never a path, and ignores its environment.
package main

import (
	"os"
	"os/exec"
	"runtime"

	"github.com/mauriziofonte/gash/internal/hostshelper"
)

func main() {
	os.Exit(hostshelper.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, hostshelper.Config{
		HostsPath: hostshelper.HostsFile,
		GOOS:      runtime.GOOS,
		Exec:      execFixed,
	}))
}

// execFixed runs one of the fixed flush commands: an absolute path, no
// shell, an empty environment, no standard streams.
func execFixed(argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:noctx // a fixed flush command run to completion; the helper has no context
	cmd.Env = []string{}
	cmd.Dir = "/"
	return cmd.Run()
}
