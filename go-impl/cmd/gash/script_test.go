// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package main

// The black-box suite (design/testing.md 4): every testdata/script/*.txtar
// runs under testscript against this binary behind the generated shims.
//
// Each script gets a clean environment:
//
//	HOME        $WORK/home (the archive's home/ files; ~/.gash_env and the
//	            XDG dirs live there)
//	GASH_STAGE  a staged tree (`gash __compat stage`): the checkout with
//	            the generated gash.sh, shims and bash_completion
//	GASH_SH     $GASH_STAGE/gash.sh, for `exec bash -c 'source "$GASH_SH"; ...'`
//	GASH_BIN    the binary the shims call (this test binary, which runs
//	            as gash when started under that name)
//	LANG        C.UTF-8
//	TERM        dumb
//
// and `gash` on PATH, so `exec gash ...` runs the binary directly. A script
// proving a LEDGER entry is named after its test (TestLedger_LLM21.txtar:
// go test ./cmd/gash -run TestScript/TestLedger_LLM21). `go test
// ./cmd/gash -update` rewrites the files that cmp compares against, inside
// the archives.

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"

	"github.com/mauriziofonte/gash/internal/compatgen"
)

var update = flag.Bool("update", false, "rewrite the cmp files of testdata/script/*.txtar")

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"gash": main})
}

func TestScript(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("bash not available")
	}
	root, err := compatgen.FindRoot(".")
	if err != nil {
		t.Skip("not in a Gash checkout:", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	stage := filepath.Join(t.TempDir(), "stage")
	bin, err := compatgen.Stage(root, stage, exe)
	if err != nil {
		t.Fatal(err)
	}
	testscript.Run(t, testscript.Params{
		Dir:                 filepath.Join(root, "go-impl", "testdata", "script"),
		UpdateScripts:       *update,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		Setup: func(env *testscript.Env) error {
			// A script's archive may hold files under home/ already.
			home := filepath.Join(env.WorkDir, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				return err
			}
			env.Setenv("HOME", home)
			env.Setenv("GASH_STAGE", stage)
			env.Setenv("GASH_SH", filepath.Join(stage, "gash.sh"))
			env.Setenv("GASH_BIN", bin)
			env.Setenv("LANG", "C.UTF-8")
			env.Setenv("TERM", "dumb")
			return nil
		},
	})
}
