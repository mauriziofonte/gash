// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package registry

import (
	"flag"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateOptions = flag.Bool("update", false, "rewrite testdata/options.golden")

// The options every function of the agent catalog lists, read from its
// help: one line per option, its names and the values it takes. A help
// edit that changes what --flag=value splits shows up here.
func TestOptionsGolden(t *testing.T) {
	var names []string
	for _, f := range Functions() {
		if EqualsForm(f.Name) && f.HasHelp {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		seen := map[string]int{}
		for _, o := range Options(n) {
			v := strconv.Itoa(o.Values)
			if o.Optional {
				v = "?"
			}
			for _, s := range o.Names {
				// One spelling, one meaning.
				if prev, ok := seen[s]; ok {
					require.Equal(t, prev, o.Values, "%s %s is listed twice with different values", n, s)
				}
				seen[s] = o.Values
			}
			b.WriteString(n + "\t" + strings.Join(o.Names, ",") + "\t" + v + "\n")
		}
	}
	golden := filepath.Join("testdata", "options.golden")
	if *updateOptions {
		require.NoError(t, os.MkdirAll("testdata", 0o755))
		require.NoError(t, os.WriteFile(golden, []byte(b.String()), 0o644))
	}
	want, err := os.ReadFile(golden)
	require.NoError(t, err)
	require.Equal(t, string(want), b.String(), "go test ./internal/registry -run TestOptionsGolden -update")
}

func TestSpecOptions(t *testing.T) {
	for spec, want := range map[string][]Option{
		"-C, --context N":                       {{Names: []string{"-C", "--context"}, Values: 1}},
		"--max-lines N, --head N, --tail N":     {{Names: []string{"--max-lines"}, Values: 1}, {Names: []string{"--head"}, Values: 1}, {Names: []string{"--tail"}, Values: 1}},
		"--absolute, --relative Absolute paths": {{Names: []string{"--absolute"}}, {Names: []string{"--relative"}}},
		"--null, -0":                            {{Names: []string{"--null", "-0"}}},
		"--http URL [--code N] [--json-path P --equals V]": {{Names: []string{"--http"}, Values: 1}, {Names: []string{"--code"}, Values: 1},
			{Names: []string{"--json-path"}, Values: 1}, {Names: []string{"--equals"}, Values: 1}},
		"--push [REMOTE]":                   {{Names: []string{"--push"}, Optional: true}},
		`--author "N <e>"`:                  {{Names: []string{"--author"}, Values: 1}},
		"--file-match FILE RE":              {{Names: []string{"--file-match"}, Values: 2}},
		"--signal S, -s S":                  {{Names: []string{"--signal"}, Values: 1}, {Names: []string{"-s"}, Values: 1}},
		"--sort name|mtime|size Path order": {{Names: []string{"--sort"}, Values: 1}},
		"--headers h1,h2":                   {{Names: []string{"--headers"}, Values: 1}},
		"--pid PID...":                      {{Names: []string{"--pid"}, Values: 1}},
		"-- CMD...":                         nil,
		"-l, --files-only":                  {{Names: []string{"-l", "--files-only"}}},
	} {
		require.Equal(t, want, specOptions(spec), spec)
	}
}
