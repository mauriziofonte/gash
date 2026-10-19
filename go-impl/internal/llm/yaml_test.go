// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package llm

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type yqCase struct{ In, Out string }

// pythonYQ reads testdata/python_yq.json: YAML files and what 2.x
// llm_config printed for them with python-yq installed.
func pythonYQ(t *testing.T) []yqCase {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "python_yq.json"))
	require.NoError(t, err)
	_, body, _ := strings.Cut(string(data), "\n") // the header line
	var doc struct{ Cases []yqCase }
	require.NoError(t, json.Unmarshal([]byte(body), &doc))
	require.Greater(t, len(doc.Cases), 200)
	return doc.Cases
}

// TestLedger_LLM35: llm_config converts YAML itself, the same whichever
// yq is installed (none runs): the bytes python-yq and jq printed, values
// as YAML 1.2 reads them, and the raw text for a file that does not
// convert.
func TestLedger_LLM35(t *testing.T) {
	cases := pythonYQ(t)
	m := New()
	d := physTemp(t)
	// A yq on PATH that would leave a mark and print something else.
	bin := physTemp(t)
	mark := filepath.Join(d, "yq-ran")
	mustWrite(t, filepath.Join(bin, "yq"), "#!/bin/sh\ntouch "+mark+"\necho '{\"other\": true}'\n")
	require.NoError(t, os.Chmod(filepath.Join(bin, "yq"), 0o755))
	env := baseEnv(physTemp(t), map[string]string{"PATH": bin})
	run := func(in string) result {
		t.Helper()
		mustWrite(t, filepath.Join(d, "c.yml"), in)
		return runGo(t, m, d, env, "llm_config", "c.yml")
	}

	// The spec's file.
	require.Equal(t, result{"{\n  \"a\": 1\n}\n", "", 0}, run("a: 1\n"))
	// python-yq's bytes: numbers, keys, merges, tags, documents, strings,
	// and the raw text where it failed (parse errors, complex keys,
	// expansion bombs) or printed nothing (empty, comments only).
	for _, c := range cases {
		require.Equal(t, result{c.Out, "", 0}, run(c.In), "%q", c.In)
	}
	// Where python-yq and YAML 1.2 differ, 1.2 (and the parse succeeds).
	for _, c := range []yqCase{
		// A leading zero is decimal; python-yq's int constructor read it
		// as octal, and failed on 08.
		{"a: 010\nb: 08\nc: !!int 010\n", "{\n  \"a\": 10,\n  \"b\": 8,\n  \"c\": 10\n}\n"},
		// A tab is separation space; PyYAML refused it.
		{"a:\t1\nb: 2\t\nc: [1,\t2]\n", "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": [\n    1,\n    2\n  ]\n}\n"},
	} {
		require.Equal(t, result{c.Out, "", 0}, run(c.In), "%q", c.In)
	}
	// Keys that are one key in a Python dict, then in jq.
	require.Equal(t, "{\n  \"1\": \"d\",\n  \"0\": \"c\"\n}\n",
		run("1: a\ntrue: b\n0: x\nfalse: c\n1.0: y\n\"1\": d\n").Stdout)
	// A non-UTF-8 byte or a control character: the raw text.
	for _, in := range []string{"a: \xff\n", "a: \x01\n", "a: b\x00\n"} {
		require.Equal(t, result{in, "", 0}, run(in), "%q", in)
	}
	_, err := os.Stat(mark)
	require.True(t, os.IsNotExist(err), "yq ran")
}

// The fixture still matches the python-yq installed here, if any (the
// Debian/Ubuntu package; 2.x ran it as `yq . FILE`).
func TestPythonYQFixture(t *testing.T) {
	yq, err := exec.LookPath("yq")
	if err != nil {
		t.Skip("no yq")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("no jq")
	}
	v, _ := exec.Command(yq, "--version").CombinedOutput()
	if !regexp.MustCompile(`^yq [0-9]`).Match(v) {
		t.Skip("yq is not python-yq")
	}
	cases := pythonYQ(t)
	d := t.TempDir()
	got := make([]string, len(cases))
	var wg sync.WaitGroup
	next := make(chan int)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				f := filepath.Join(d, strconv.Itoa(i)+".yml")
				if err := os.WriteFile(f, []byte(cases[i].In), 0o600); err != nil {
					got[i] = err.Error()
					continue
				}
				cmd := exec.Command(yq, ".", f)
				cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C.UTF-8"}
				out, err := cmd.Output()
				if s := strings.TrimRight(string(out), "\n"); err == nil && s != "" {
					got[i] = s + "\n"
				} else {
					got[i] = cases[i].In
				}
			}
		}()
	}
	for i := range cases {
		next <- i
	}
	close(next)
	wg.Wait()
	for i, c := range cases {
		require.Equal(t, c.Out, got[i], "%q", c.In)
	}
}

func TestPyRepr(t *testing.T) {
	for f, want := range map[float64]string{
		1: "1.0", 1e16: "1e+16", 1e15: "1000000000000000.0", 0.0001: "0.0001", 0.00001: "1e-05",
		1.5e-7: "1.5e-07", 5e-324: "5e-324", 1.7976931348623157e308: "1.7976931348623157e+308",
		123.456: "123.456", 1e100: "1e+100", 0.1: "0.1",
	} {
		require.Equal(t, want, pyRepr(f))
	}
}
