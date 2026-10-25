// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import (
	"bytes"
	"strings"
	"testing"
)

// fuzzSecret is planted in every value and name of the fuzzed file.
const fuzzSecret = "Zq9SeCrEt"

// Parsing any ~/.gash_env never panics, and no warning or hint repeats any
// part of a line (settings_spec.sh:46): the secret planted after every "=",
// ":", "@" and at the start of every line never shows in them.
func FuzzParseNoLeak(f *testing.F) {
	for _, s := range []string{
		"GASH_PROMPT=1\n", "GASH_NOPE=x\n", "PASSWORD=hunter2\n", "AI:claude=sk-ant-x\n", "AI:openai\n",
		"DB:prod=mysql://u:pw@h:3306/db\n", "DB:x=pgsql://u@/db?host=/run\n", "HOST:box=user@host:22\n",
		"SSH:~/.ssh/id=pass\n", "GASH_X='unterminated\n", "GASH_X=\"a\\\"b\"\n", "\ufeffGASH_PROMPT=1\r\n",
		"# comment\n\n  \t\n", "export GASH_PROMPT=1\n", "DB:a=sqlite:///tmp/x.db\nDB:a=sqlite:///y\n",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if bytes.Contains(data, []byte(fuzzSecret)) {
			return
		}
		for _, planted := range [][]byte{plantAfter(data), plantBefore(data)} {
			file := Parse(planted, "/cfg/env", "/home/u")
			for _, w := range file.Warnings {
				if strings.Contains(w.Text, fuzzSecret) || strings.Contains(w.Hint, fuzzSecret) {
					t.Fatalf("a warning shows content of %q:\n%s\n%s", planted, w.Text, w.Hint)
				}
			}
		}
	})
}

// plantAfter puts the secret after every separator a value or name follows.
func plantAfter(data []byte) []byte {
	var b bytes.Buffer
	for _, c := range data {
		b.WriteByte(c)
		if c == '=' || c == ':' || c == '@' {
			b.WriteString(fuzzSecret)
		}
	}
	return b.Bytes()
}

// plantBefore puts the secret at the start of every line.
func plantBefore(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	for i, l := range lines {
		lines[i] = append([]byte(fuzzSecret), l...)
	}
	return bytes.Join(lines, []byte("\n"))
}
