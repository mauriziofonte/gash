// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package shellinit

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mauriziofonte/gash/internal/runner"
)

// gnuProbe answers the alias-pack probes like a GNU/Linux toolchain: ls takes
// --color=auto and rm is GNU; everything else fails.
func gnuProbe(c runner.Call) (runner.Expect, bool) {
	if len(c.Argv) == 0 {
		return runner.Expect{}, false
	}
	switch c.Argv[0] {
	case "ls":
		return runner.Expect{Status: 0}, true
	case "rm":
		return runner.Expect{Stdout: "rm (GNU coreutils) 9.1\n", Status: 0}, true
	}
	return runner.Expect{}, false
}

func TestShellInit_AliasPackColors(t *testing.T) {
	// With a grep probe that exits 1 (empty pattern on /dev/null), no grep
	// alias is defined (SHELL-06).
	out := newHarness().withProgram("grep").emit(t)
	require.NotContains(t, out, "alias grep=")

	// LESS in the environment emits the LESS_TERMCAP_* exports.
	h := newHarness()
	h.env["LESS"] = "-R"
	require.Contains(t, h.emit(t), "export LESS_TERMCAP_md=")
}

func TestShellInit_AliasPackNavigation(t *testing.T) {
	h := newHarness().withProgram("ls", "dir", "vdir")
	h.probe = gnuProbe
	out := h.emit(t)
	// ls family carries the probed color flag; dir/vdir only when present.
	require.Contains(t, out, "alias l='ls -CF --color=auto'")
	require.Contains(t, out, "alias ll='ls -l --color=auto'")
	require.Contains(t, out, "alias dir='dir --color=auto'")
	require.Contains(t, out, "alias vdir='vdir --color=auto'")
	// Directory shortcuts are always defined.
	require.Contains(t, out, "alias cd..='cd ..'")
	require.Contains(t, out, "alias .4='cd ../../../..'")
	require.Contains(t, out, "alias .5='cd ../../../../..'")

	// Without a color flag the ls aliases drop it (ls itself is not aliased).
	plain := newHarness().withProgram("ls").emit(t)
	require.Contains(t, plain, "alias l='ls -CF'")
	require.NotContains(t, plain, "--color=auto")
}

// On macOS, BSD ls rejects --color=auto, so the probe fails and the pack
// falls back to -G (and exports LSCOLORS). The goos seam makes this
// deterministic regardless of the host the suite runs on.
func TestShellInit_AliasPackNavigationDarwin(t *testing.T) {
	h := newHarness()
	h.goos = "darwin"
	out := h.emit(t)
	require.Contains(t, out, "alias ls='ls -G'")
	require.Contains(t, out, "alias l='ls -CF -G'")
	require.Contains(t, out, "alias ll='ls -l -G'")
	require.NotContains(t, out, "--color=auto")
	// packColors exports the BSD LSCOLORS palette on macOS.
	require.Contains(t, out, "export LSCOLORS='CxahafafBxagagabababab'")
}

func TestShellInit_AliasPackSafety(t *testing.T) {
	h := newHarness().withProgram("rm", "ss")
	h.probe = gnuProbe
	out := h.emit(t)
	require.Contains(t, out, "alias cp='cp -iv'")
	require.Contains(t, out, "alias rm='rm -Iv --one-file-system --preserve-root'")
	require.Contains(t, out, "alias chown='chown --preserve-root'")
	require.Contains(t, out, "alias chmod='chmod --preserve-root'")
	require.Contains(t, out, "alias chgrp='chgrp --preserve-root'")
	require.Contains(t, out, "alias bc='bc -l'")
	require.Contains(t, out, "alias ports='ss -tulanp'")
	require.Contains(t, out, "alias reboot='sudo /sbin/reboot'")

	// A non-GNU rm gets the portable form and no --preserve-root family.
	bsd := newHarness().withProgram("rm")
	out2 := bsd.emit(t)
	require.Contains(t, out2, "alias rm='rm -iv'")
	require.NotContains(t, out2, "--preserve-root")

	// GASH_SAFETY_ALIASES=0 drops the cp/mv/rm family but keeps bc/reboot.
	off := newHarness().withProgram("rm")
	off.env["GASH_SAFETY_ALIASES"] = "0"
	out3 := off.emit(t)
	require.NotContains(t, out3, "alias cp=")
	require.NotContains(t, out3, "alias rm=")
	require.Contains(t, out3, "alias bc='bc -l'")
}

func TestShellInit_AliasPackGit(t *testing.T) {
	out := newHarness().withProgram("git").emit(t)
	// The gl family are thin stubs that call the binary.
	for _, fn := range []string{"gl", "gla", "glo", "glg", "gls", "glf"} {
		require.Contains(t, out, fn+"() { __gash_call "+fn+" \"$@\"; }")
	}
	require.Contains(t, out, "alias glog=gl")
	require.Contains(t, out, "alias gst='git status'")
	require.Contains(t, out, "alias gs='git status -sb'")
	require.Contains(t, out, "alias gcm='git commit -m'")
	require.Contains(t, out, "alias gpf='git push --force-with-lease'")

	// Without git on PATH the whole pack is empty.
	require.NotContains(t, newHarness().emit(t), "alias gst=")
}

// TestShellInit_EnvUser proves the banner names USER.
func TestShellInit_EnvUser(t *testing.T) {
	h := newHarness()
	h.env["USER"] = "alice"
	out := h.emit(t)
	require.Contains(t, out, "printf '%s' $'Gash ")
	require.Contains(t, out, "alice")

	// USER empty does not crash the banner (falls back to gash_username).
	h2 := newHarness()
	h2.env["USER"] = ""
	require.Contains(t, h2.emit(t), "printf '%s' $'Gash ")
}

// TestShellInit_EnvHome proves HOME locates ~/.gashrc for the 1.x migration
// reminder, which the binary emits when the account is still on Gash 1.x.
func TestShellInit_EnvHome(t *testing.T) {
	h := newHarness()
	h.withFile("/home/u/.gashrc", "export GASH_BASH=$HOME/.gash/gash.sh\n")
	out := h.emit(t)
	require.Contains(t, out, "__gash_note ")
	require.Contains(t, out, "Gash 1.x")
	// The notice stamp was written under HOME's XDG state dir.
	require.Contains(t, h.writes, "/home/u/.local/state/gash/notice")

	// A migrated account (schema 2) gets no reminder.
	h2 := newHarness()
	h2.withFile("/home/u/.gashrc", "export GASH_BASH=x\n")
	h2.withFile("/home/u/.local/state/gash/schema", "2\n")
	require.NotContains(t, h2.emit(t), "Gash 1.x")
}
