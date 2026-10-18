// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// breInput are the lines of the GNU grep 3.11 vectors below.
var breInput = []string{"a+b", "aab", "ab", "{1}a", "+a", "*a", "xa", "^a", "a$b", "[:space:]"}

// TestBREVectors pins the translation against `grep -i -- PATTERN` of GNU
// grep 3.11 (C.UTF-8) on breInput: the matching lines, or grep's error.
func TestBREVectors(t *testing.T) {
	all := strings.Join(breInput, "|")
	for _, tc := range []struct{ pattern, want string }{
		{`a+b`, "a+b"},
		{`a\+b`, "aab|ab"},
		{`\+a`, "+a"},
		{`\?a`, ""},
		{`\{1\}a`, "{1}a"},
		{`\{`, "{1}a"},
		{`\}`, "{1}a"},
		{`\]`, "[:space:]"},
		{`*a`, "*a"},
		{`^*a`, "*a"},
		{`\(*a\)`, "*a"},
		{`a\|^a`, all},
		{`a$b`, "a$b"},
		{`x\|*a`, "*a|xa"},
		{`a**`, all},
		{`\w`, all},
		{`\<a`, "!"},
		{`[]a]`, all},
		{`[^]a]`, all},
		{`[[=a=]]`, all},
		{`a\{1,\}`, all},
		{`ab\{0\}`, all},
		{`a\{,2\}b`, "a+b|aab|ab|a$b"},
		{`a\{1\}\{2\}`, "aab"},
		{`\(a\)\1`, "!"},
		{`[:space:]`, "grep: character class syntax is [[:space:]], not [:space:]"},
		{`[::]`, "[:space:]"},
		{`a\{2`, `grep: Unmatched \{`},
		{`a\{x`, `grep: Unmatched \{`},
		{`a\{2,1\}`, `grep: Invalid content of \{\}`},
		{`a\{1x\}`, `grep: Invalid content of \{\}`},
		{`a\{\}`, `grep: Invalid content of \{\}`},
		{`a\{99999\}`, "grep: Regular expression too big"},
		{`\(a`, `grep: Unmatched ( or \(`},
		{`a\)`, `grep: Unmatched ) or \)`},
		{`[z-a]`, "grep: Invalid range end"},
		{`[Z-a]`, "grep: Invalid range end"},
		{`[a-Z]`, all},
		{`[[:foo:]]`, "grep: Invalid character class name"},
		{`[[:alpha:]-z]`, "grep: Invalid range end"},
		{`[a`, "grep: Unmatched [, [^, [:, [., or [="},
		{`[[.ab.]]`, "grep: Invalid collation character"},
		{`\`, "grep: Trailing backslash"},
		{`\1`, "grep: Invalid back reference"},
	} {
		re, err := compileBRE(tc.pattern)
		var ge *grepError
		switch {
		case errors.As(err, &ge):
			require.Equal(t, tc.want, "grep: "+ge.msg, tc.pattern)
			continue
		case errors.Is(err, errNoRE2):
			require.Equal(t, "!", tc.want, tc.pattern)
			continue
		}
		require.NoError(t, err, tc.pattern)
		var got []string
		for _, l := range breInput {
			if re.MatchString(l) {
				got = append(got, l)
			}
		}
		require.Equal(t, tc.want, strings.Join(got, "|"), tc.pattern)
	}
}

func historyGrep(t *testing.T, stdin string, args ...string) result {
	t.Helper()
	m, _, _ := tm(t, runner.NewFake(t))
	return call(m, "history_grep", nil, nil, stdin, args...)
}

const historyFixture = "    1  git push\n    2  cp a a\n    3  ls ls ls\n    4  git push origin\n    5  history_grep git\n" +
	"    6  hg git\n    7  echo ÉCOLE\n    8  a+b c\n    9  aab\n   10  ssh host\n   11  cron x\n   12  git push origin\n" +
	"   13  x\t y   z\n   14  single\n"

func TestHistoryGrep(t *testing.T) {
	r := historyGrep(t, historyFixture, "git")
	require.Equal(t, result{"1     git push              git push\n2     git push             origin\n3     hg git                hg git\n", "", 0}, r)
	r = historyGrep(t, historyFixture, "single")
	require.Equal(t, "1     single                single\n", r.out)
	r = historyGrep(t, historyFixture, "y")
	require.Equal(t, "1     x y                  z\n", r.out)
	r = historyGrep(t, historyFixture, "zz")
	require.Equal(t, result{"", "", 0}, r)

	r = historyGrep(t, historyFixture)
	require.Equal(t, result{"", "Error: Missing pattern. Usage: history_grep <pattern>\n", 1}, r)
	r = historyGrep(t, historyFixture, "-h")
	require.Equal(t, 0, r.rc)
	require.Contains(t, r.out, "history_grep")
}

// SYS-08: every word after the pattern is a grep FILE operand.
func TestLedger_SYS08(t *testing.T) {
	d := t.TempDir()
	push := filepath.Join(d, "push")
	require.NoError(t, os.WriteFile(push, []byte("1 git from-a-file\n"), 0o600))
	r := historyGrep(t, "    1  git push origin\n", "git", push)
	require.Equal(t, result{"1     git from-a-file       git from-a-file\n", "", 0}, r)

	// Two files: grep's name: prefix becomes the first word; a missing
	// file is grep's message.
	r = historyGrep(t, "", "git", push, filepath.Join(d, "nope"), push)
	require.Equal(t, "grep: "+filepath.Join(d, "nope")+": No such file or directory\n", r.err)
	require.Equal(t, "1     git from-a-file       git from-a-file\n", r.out)
	r = historyGrep(t, "", "--", "-v")
	require.Equal(t, result{"", "grep: -v: No such file or directory\n", 0}, r)
	r = historyGrep(t, "", "x", d)
	require.Equal(t, "grep: "+d+": Is a directory\n", r.err)
}

// SYS-09: the two awk passes keep their defects: column 3 restarts at the
// first occurrence of word 3, dedup is on the rebuilt line, and only the
// literal history_grep is excluded (hg lines are listed).
func TestLedger_SYS09(t *testing.T) {
	r := historyGrep(t, "    1  cp a a\n    2  ls ls ls\n    3  git push\n    4  cp a a\n", "a\\|ls\\|push")
	require.Equal(t, "1     cp a                 a a\n2     ls ls                ls ls ls\n3     git push              git push\n", r.out)
	// HISTTIMEFORMAT puts the date in column 2 and defeats the dedup.
	r = historyGrep(t, "    1  2024-01-01 10:00:00 git push origin\n    2  2024-01-01 10:00:01 git push origin\n", "push")
	require.Equal(t, "1     2024-01-01 10:00:00  git push origin\n2     2024-01-01 10:00:01  git push origin\n", r.out)
	r = historyGrep(t, "    1  hg git\n    2  history_grep git\n", "git")
	require.Equal(t, "1     hg git                hg git\n", r.out)
}

// SYS-38: GNU BRE, case-insensitive with Unicode folding in every locale;
// a back-reference is refused.
func TestLedger_SYS38(t *testing.T) {
	r := historyGrep(t, historyFixture, "a+b")
	require.Equal(t, "1     a+b c                 a+b c\n", r.out)
	r = historyGrep(t, historyFixture, `a\+b`)
	require.Equal(t, "1     aab                   aab\n", r.out)
	r = historyGrep(t, historyFixture, `ssh\|cron`)
	require.Equal(t, "1     ssh host              ssh host\n2     cron x                cron x\n", r.out)
	// GNU grep folded É only in a UTF-8 locale; rc.A folds in every one.
	m, _, _ := tm(t, runner.NewFake(t))
	r = call(m, "history_grep", map[string]string{"LC_ALL": "C"}, nil, historyFixture, "école")
	// (the C locale table counts bytes)
	require.Equal(t, "1     echo ÉCOLE           echo ÉCOLE\n", r.out)
	r = historyGrep(t, historyFixture, `\(a\)\1`)
	require.Equal(t, result{"", "Error: Invalid pattern: \\(a\\)\\1\n", 1}, r)
	// A pattern GNU grep refuses: its message, no rows, status 0.
	r = historyGrep(t, historyFixture, `[b-a]`)
	require.Equal(t, result{"", "grep: Invalid range end\n", 0}, r)
	// Lines that are not UTF-8 are binary data for grep.
	r = historyGrep(t, "    1  b a\n    2  \xff a\n    3  c a\n", "a")
	require.Equal(t, result{"1     b a                   b a\n2     c a                   c a\n",
		"grep: (standard input): binary file matches\n", 0}, r)
}
