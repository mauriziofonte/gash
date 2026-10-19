// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/term"
)

// setupCorrupt: setupOrigin with the blob of a and one more loose object
// truncated to 0 bytes (git makes them read-only).
const setupCorrupt = setupOrigin + " && " + corrupt

const corrupt = `for o in $(git rev-parse HEAD:a) $(git rev-parse HEAD~1:a); do
	p=$(git rev-parse --git-path objects)/${o:0:2}/${o:2}; chmod u+w "$p"; : >"$p"
done`

func TestRepairMatches2x(t *testing.T) {
	cases := []struct {
		name, setup, cwd string
		args             []string
		env              []string
		rc               int
	}{
		{"unknown option", setupCorrupt, "w", []string{"--force"}, nil, 1},
		{"unknown option before the repo check", "", ".", []string{"-y", "x"}, nil, 1},
		{"outside a repository", "", ".", nil, nil, 1},
		{"no origin", setupRepo, "w", []string{"-y"}, nil, 1},
		{"healthy", setupOrigin, "w", nil, nil, 0},
		{"healthy from a subdirectory", setupOrigin + " && mkdir sub", "w/sub", []string{"--dry-run"}, nil, 0},
		{"dry run", setupCorrupt, "w", []string{"--dry-run"}, nil, 0},
		{"dry run and -y", setupCorrupt, "w", []string{"-y", "--dry-run"}, nil, 0},
		{"no terminal", setupCorrupt, "w", nil, nil, 1},
		{"headless", setupCorrupt, "w", nil, []string{"GASH_HEADLESS=1"}, 1},
		{"repaired", setupCorrupt, "w", []string{"-y"}, nil, 0},
		{"repaired with --yes", setupCorrupt, "w", []string{"--yes"}, nil, 0},
		{"broken history, no empty file", setupOrigin + ` && o=$(git rev-parse HEAD:a) && rm -f .git/objects/${o:0:2}/${o:2}`, "w", []string{"-y"}, nil, 0},
		{"clone fails", setupCorrupt + " && git remote set-url origin ../none.git", "w", []string{"-y"}, nil, 1},
		{"in a linked worktree", setupOrigin + " && git worktree add -q ../lw 2>/dev/null && " + corrupt, "lw", []string{"-y"}, nil, 0},
		{"help", "", ".", []string{"-h"}, nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newScene(t, tc.setup).same(mode{env: tc.env}, tc.cwd, "git_repair", tc.args...)
			require.Equal(t, tc.rc, r.RC)
		})
	}
	t.Run("color", func(t *testing.T) {
		newScene(t, setupCorrupt).same(color, "w", "git_repair", "-y")
		newScene(t, setupCorrupt).same(color, "w", "git_repair", "--dry-run")
	})
}

// The question of git_repair reads the controlling terminal: Enter, y and
// yes (any case) go ahead, anything else is "Aborted." rc 1, end of input
// is a newline and rc 1.
func TestRepairAsks(t *testing.T) {
	s := newScene(t, setupCorrupt)
	yes := s.run2x(plain, "w", "git_repair", "-y")
	plan := "Object store:" // the plan is on stdout before the question
	for _, in := range []string{"\n", "y\n", "YeS\n", "  y \n"} {
		got := s.runGo(plain, "w", "git_repair", func(tt *term.Term) *term.Term { return tt.WithTTY(in) })
		require.Equal(t, 0, got.RC, "%q", in)
		require.Equal(t, yes.Out, got.Out)
		require.Equal(t, "Do you want to continue? [Y/n] "+yes.Err, got.Err)
		require.Equal(t, yes.State, got.State)
	}
	no := s.run2x(plain, "w", "git_repair", "--dry-run")
	for in, tail := range map[string]string{"n\n": "Info: Aborted.\n", "yess\n": "Info: Aborted.\n", "": "\n", "y": "\n"} {
		got := s.runGo(plain, "w", "git_repair", func(tt *term.Term) *term.Term { return tt.WithTTY(in) })
		require.Equal(t, 1, got.RC, "%q", in)
		require.Contains(t, got.Out, plan)
		require.Equal(t, no.Out, got.Out)
		require.Equal(t, "Do you want to continue? [Y/n] "+tail, got.Err, "%q", in)
		require.Equal(t, no.State, got.State, "nothing changed")
	}
	got := s.runGo(color, "w", "git_repair", func(tt *term.Term) *term.Term { return tt.WithTTY("n\n") })
	require.Equal(t, "Do you want to continue? \x1b[1m[Y/n]\x1b[0m \x1b[38;5;75m*\x1b[0m Aborted.\n", got.Err)
}
