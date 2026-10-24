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
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
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
	// The render-spec network rows (docker_compose_check, ip_public, the AI
	// functions, ...) talk to registries and APIs over net/http. httpsfake
	// (internal/tools/httpsfake, design/testing.md 2.4) is the local HTTPS
	// CONNECT proxy that answers them from the canned curl responses; a case
	// starts it in the background and points HTTPS_PROXY/SSL_CERT_FILE at it.
	// It is built once here and put on PATH as `httpsfake`.
	fakeBin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(fakeBin, 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command("go", "build", "-o", filepath.Join(fakeBin, "httpsfake"),
		"github.com/mauriziofonte/gash/internal/tools/httpsfake")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("building httpsfake: %v\n%s", err, out)
	}
	// testdata/bash/run.sh runs the suite with another Bash first on PATH
	// and names it here, so a PATH mistake fails instead of testing the
	// system Bash again.
	if want := os.Getenv("GASH_EXPECT_BASH"); want != "" {
		out, err := exec.Command("bash", "-c", `echo "${BASH_VERSINFO[0]}.${BASH_VERSINFO[1]}"`).Output()
		if got := strings.TrimSpace(string(out)); err != nil || got != want {
			t.Fatalf("GASH_EXPECT_BASH=%s but the bash on PATH is %q (%v)", want, got, err)
		}
	}
	// The 2.1.0 tree (git archive v2.1.0, never the checkout itself) for
	// the scripts that compare a 2.x load with a 3.x one in the same shell
	// (init_bash_state_parity.txtar, condition [has2x], $GASH_2X_SH). A
	// shallow clone has no tags: CI fetches v2.1.0 and sets
	// GASH_REQUIRE_2X=1, so a missing tree fails there instead of skipping.
	tree2x := filepath.Join(t.TempDir(), "gash-2.1.0")
	has2x := archive2x(root, tree2x) == nil
	if !has2x && os.Getenv("GASH_REQUIRE_2X") == "1" {
		t.Fatalf("GASH_REQUIRE_2X=1 but the v2.1.0 tree is not available: %v", archive2x(root, tree2x))
	}
	// testscript's Dir is non-recursive, but the render-spec fixtures are
	// grouped in a render/ subdirectory (testdata/script/README.md), so the
	// whole tree is collected explicitly and passed as Files.
	scriptDir := filepath.Join(root, "testdata", "script")
	var files []string
	if err := filepath.WalkDir(scriptDir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && filepath.Ext(p) == ".txtar" {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// The upgrade scripts' fixtures (fixture_test.go): built once, shared.
	fx := &fixtures{root: root, stage: stage, cache: t.TempDir(), builds: map[string]*gashBuild{}}
	testscript.Run(t, testscript.Params{
		Cmds: map[string]func(*testscript.TestScript, bool, []string){
			"gashfixture": fx.cmdFixture,
			"gashrelease": fx.cmdRelease,
		},
		Files:               files,
		UpdateScripts:       *update,
		RequireExplicitExec: true,
		RequireUniqueNames:  true,
		// schema NAME stdout|stderr|FILE validates JSON output against
		// testdata/schema/NAME.json (schema_test.go).
		Cmds: map[string]func(ts *testscript.TestScript, neg bool, args []string){
			"schema": cmdSchema(root),
		},
		Condition: func(cond string) (bool, error) {
			switch cond {
			case "root":
				return os.Geteuid() == 0, nil
			case "v2tree":
				// The v2.1.0 tag the 2.x upgrade fixtures check out; a
				// shallow checkout (actions/checkout fetch-depth 1) has none.
				t2, err := fx.v2Tree()
				return t2 != "", err
			case "has2x":
				return has2x, nil
			case "bash5":
				// The bash on PATH is 5.0 or newer (testdata/bash/run.sh
				// runs the suite with 4.4 too).
				return bashMajor() >= 5, nil
			case "linux", "darwin", "windows":
				return runtime.GOOS == cond, nil
			case "gnused":
				// llm_exec runs sed only as GNU `sed --sandbox`; macOS ships
				// BSD sed without it. Probe so the sed-sandbox rows run only
				// where that path exists (the refusal is a Go unit test).
				return exec.Command("sed", "--sandbox", "-n", "-e", "p", "/dev/null").Run() == nil, nil
			}
			return false, fmt.Errorf("unknown condition %q", cond)
		},
		Setup: func(env *testscript.Env) error {
			// A script's archive may hold files under home/ already.
			home := filepath.Join(env.WorkDir, "home")
			if err := os.MkdirAll(home, 0o755); err != nil {
				return err
			}
			env.Setenv("HOME", home)
			env.Setenv("PATH", fakeBin+string(os.PathListSeparator)+env.Getenv("PATH"))
			env.Setenv("GASH_STAGE", stage)
			env.Setenv("GASH_SH", filepath.Join(stage, "gash.sh"))
			env.Setenv("GASH_BIN", bin)
			env.Setenv("LANG", "C.UTF-8")
			env.Setenv("TERM", "dumb")
			if has2x {
				env.Setenv("GASH_2X_SH", filepath.Join(tree2x, "gash.sh"))
			}
			return nil
		},
	})
}

// archive2x extracts the v2.1.0 tag of the checkout at root into dir
// (once: an existing gash.sh there is reused).
func archive2x(root, dir string) error {
	if _, err := os.Stat(filepath.Join(dir, "gash.sh")); err == nil {
		return nil
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "v2.1.0^{commit}").Run(); err != nil {
		return fmt.Errorf("no v2.1.0 tag in %s (git fetch origin tag v2.1.0)", root)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("bash", "-c", `git -C "$1" archive v2.1.0 | tar -x -C "$2"`, "bash", root, dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git archive v2.1.0: %v: %s", err, out)
	}
	return nil
}

// bashMajor is the major version of the bash on PATH, 0 when unknown.
var bashMajor = sync.OnceValue(func() int {
	out, err := exec.Command("bash", "-c", `echo "${BASH_VERSINFO[0]}"`).Output()
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(strings.TrimSpace(string(out)))
	return n
})
