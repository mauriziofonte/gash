// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package hte

import (
	"os"
	"testing"
)

// CLIRig is the rig of rig_test.go for the command line tests of package
// hte_test (cli_test.go): an engine on the Debian tree with the hte-cli
// V2 sites, every process played.
type CLIRig struct{ m *rig }

// NewCLIRig returns the engine of a new rig and the rig.
func NewCLIRig(t *testing.T) (*Engine, CLIRig) {
	t.Helper()
	m := newRig(t)
	return m.e, CLIRig{m}
}

// Home is the user's home directory.
func (c CLIRig) Home() string { return c.m.home }

// Env is the environment the engine reads.
func (c CLIRig) Env() map[string]string { return c.m.env }

// Read and Write read and write a file of the system tree.
func (c CLIRig) Read(sys string) string { return c.m.read(sys) }

func (c CLIRig) Write(sys, data string, mode os.FileMode) { c.m.write(sys, data, mode) }

// RootLines are the commands the root step ran.
func (c CLIRig) RootLines() []string {
	root, _ := c.m.lines()
	return root
}

// Running is the number of masters that run.
func (c CLIRig) Running() int { return c.m.stack.Running() }
