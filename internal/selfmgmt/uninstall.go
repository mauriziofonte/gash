// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/mauriziofonte/gash/internal/help"
	"github.com/mauriziofonte/gash/internal/invoke"
	"github.com/mauriziofonte/gash/internal/ui"
)

// UninstallStepVar names the step of gash_uninstall the stub asks for
// (compat/stubs.tmpl). The stub asks the two questions in the caller's
// shell with needs_confirm_prompt, which a harness may redefine, and runs
// the binary once per step:
//
//	check       the checks and the warning: 0 means ask "Continue?"
//	remove      the profiles, ~/.gashrc and the install directory; 10 means
//	            the settings file exists: ask whether to remove it
//	env-remove  remove the settings file, then the closing lines
//	env-keep    keep it, then the closing lines
//
// Without it (the binary run directly) one call asks both questions on
// the terminal itself.
const UninstallStepVar = "GASH__UNINSTALL"

// UninstallAskEnv is the status of the remove step when the settings file
// exists.
const UninstallAskEnv = 10

// gashrcSource is the test for a profile that loads Gash (grep -q
// 'source.*[./]gashrc').
var gashrcSource = regexp.MustCompile(`source.*[./]gashrc`)

// The profile edits of 2.x, run with sed -i.gash-uninstall.bak as 2.x ran
// them: the delimited block, or the 1.x block (its comment line and the
// four lines after it; the `;` before `}` lets BSD sed parse it too).
const (
	uninstallBackup     = ".gash-uninstall.bak"
	uninstallSedBlock   = `/# >>> GASH START >>>/,/# <<< GASH END <<</d`
	uninstallSed1xBlock = `/# Load Gash Bash/{N;N;N;N;d;}`
)

// uninstall is gash_uninstall (lib/modules/gash.sh): after a confirmation,
// removes the Gash block from ~/.bashrc, ~/.bash_profile and ~/.profile,
// ~/.gashrc and the install directory (only one that is unmistakably a
// Gash checkout), then asks about the settings file. A ~/.gashrc that holds
// the user's own lines is kept as ~/.gashrc.gash-uninstall.bak, and the
// profile backups sed makes are kept and listed (3.0: SELF-02 fixed; 2.x
// removed both). A linked profile is replaced by an edited regular file
// (SELF-04): sed -i runs as 2.x ran it.
func (m *Module) uninstall(c *invoke.Call) int {
	if help.NeedsOf(c, c.Arg(0)) {
		return 0
	}
	s := ui.StyleOf(c.Term, false)
	p := s.Printer(c.Stderr)
	// Any other value is no step: the binary asks both questions itself.
	step := c.Env.Get(UninstallStepVar)
	switch step {
	case "env-remove", "env-keep":
		return m.uninstallEnd(c, p, step == "env-remove")
	case "check", "remove":
	default:
		step = ""
	}
	staged := step != ""

	dir := installDir(c)
	real, ok := uninstallTarget(c, p, dir)
	if !ok {
		return 1
	}
	if step != "remove" {
		p.Warning("This will remove Gash (" + real + ") and its configuration from this account.")
		if staged {
			return 0
		}
		if !s.Confirm(c.Term, c.Stderr, "Continue?") {
			p.Info("Uninstall cancelled.")
			return 0
		}
	}

	home := c.Env.Get("HOME")
	for _, profile := range uninstallProfiles(home) {
		if !isFile(profile) {
			continue
		}
		text, err := os.ReadFile(profile)
		if err != nil {
			_, _ = io.WriteString(c.Stderr, "grep: "+profile+": "+errnoText(err)+"\n")
		}
		if err != nil || !gashrcSource.Match(text) {
			p.Info("No Gash block found in: " + profile + "; skipping.")
			continue
		}
		p.Info("Removing Gash block from: " + profile)
		switch {
		case strings.Contains(string(text), "# >>> GASH START >>>"):
			m.tool(c, "sed", "-i"+uninstallBackup, uninstallSedBlock, profile)
		case strings.Contains(string(text), "# Load Gash Bash"):
			m.tool(c, "sed", "-i"+uninstallBackup, uninstallSed1xBlock, profile)
		}
	}

	if isFile(home + "/.gashrc") {
		uninstallGashrc(p, home)
	} else {
		p.Info(home + "/.gashrc not found; skipping.")
	}

	p.Info("Removing Gash at " + real + "...")
	if err := os.RemoveAll(real); err != nil {
		p.Error("Failed to remove " + real + "; please remove it manually.")
	}

	envFile, shown := uninstallEnvFile(c)
	if !isFile(envFile) {
		return m.uninstallEnd(c, p, false)
	}
	p.Warning("Found " + shown + " (may contain database credentials).")
	if staged {
		return UninstallAskEnv
	}
	return m.uninstallEnd(c, p, s.Confirm(c.Term, c.Stderr, "Remove "+shown+"?"))
}

// uninstallTarget resolves the install directory and refuses one that is
// not a Gash checkout (or is / or HOME), so that a stray GASH_DIR never
// turns into a wipe.
func uninstallTarget(c *invoke.Call, p ui.Printer, dir string) (string, bool) {
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		p.Error("Gash is not installed on this system.")
		return "", false
	}
	real := physical(dir)
	home := c.Env.Get("HOME")
	realHome := physical(home)
	if realHome == "" {
		realHome = home
	}
	if real == "" || real == "/" || real == realHome || !isFile(real+"/gash.sh") || !isDir(real+"/lib/core") {
		p.Error(dir + " does not look like a Gash installation (no gash.sh and lib/core); refusing to remove it.")
		return "", false
	}
	return real, true
}

// uninstallGashrc removes ~/.gashrc when it is a template an installer
// wrote, and otherwise renames it to ~/.gashrc.gash-uninstall.bak, so the
// user's own lines (the 1.x migration appends them there) survive
// (LEDGER SELF-02).
func uninstallGashrc(p ui.Printer, home string) {
	gashrc := home + "/.gashrc"
	if data, err := os.ReadFile(gashrc); err == nil && pristineGashrc(data) {
		p.Info("Removing ~/.gashrc...")
		if err := os.Remove(gashrc); err != nil && !os.IsNotExist(err) {
			p.Error("Failed to remove ~/.gashrc; please remove it manually.")
		}
		return
	}
	if err := os.Rename(gashrc, gashrc+uninstallBackup); err != nil {
		p.Error("Failed to save ~/.gashrc as ~/.gashrc" + uninstallBackup + "; it was left in place.")
		return
	}
	p.Info("Saved your ~/.gashrc as ~/.gashrc" + uninstallBackup)
}

// uninstallEnd handles the settings file when it exists (removed when
// remove is true, else kept) and prints the closing lines, listing the
// backups uninstall left: the profiles as sed -i saved them before the
// edit and a ~/.gashrc that held the user's lines (3.0: SELF-02 fixed;
// 2.x removed the profile backups here).
func (m *Module) uninstallEnd(c *invoke.Call, p ui.Printer, remove bool) int {
	envFile, shown := uninstallEnvFile(c)
	if isFile(envFile) {
		switch {
		case !remove:
			p.Info("Kept " + shown)
		case removeFile(envFile) == nil:
			p.Info("Removed " + shown)
		default:
			p.Error("Failed to remove " + shown + "; please remove it manually.")
		}
	}
	p.Success("Gash uninstalled.")
	home := c.Env.Get("HOME")
	var kept []string
	for _, f := range append(uninstallProfiles(home), home+"/.gashrc") {
		if st, err := os.Lstat(f + uninstallBackup); err == nil && !st.IsDir() {
			kept = append(kept, "~"+strings.TrimPrefix(f, home)+uninstallBackup)
		}
	}
	if len(kept) > 0 {
		p.Info("Backups kept: " + strings.Join(kept, ", "))
	}
	p.Info("Restart your terminal to apply changes.")
	return 0
}

func uninstallProfiles(home string) []string {
	return []string{home + "/.bashrc", home + "/.bash_profile", home + "/.profile"}
}

// uninstallEnvFile is the settings file and its name as the messages show
// it (${env_file/#$HOME/\~}).
func uninstallEnvFile(c *invoke.Call) (string, string) {
	envFile := c.Env.Get("GASH_ENV_FILE")
	if envFile == "" {
		envFile = c.Env.Get("HOME") + "/.gash_env"
	}
	shown := envFile
	if home := c.Env.Get("HOME"); strings.HasPrefix(envFile, home) {
		shown = "~" + envFile[len(home):]
	}
	return envFile, shown
}

// physical is $(cd -- DIR && pwd -P): the absolute path with every symlink
// resolved; "" when it cannot be resolved.
func physical(dir string) string {
	if dir == "" {
		return ""
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return ""
	}
	if st, err := os.Stat(real); err != nil || !st.IsDir() {
		return ""
	}
	return real
}

func isDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

// removeFile is rm -f -- PATH for a path that is not a directory: nil when
// it is gone or was never there.
func removeFile(path string) error {
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return &os.PathError{Op: "remove", Path: path, Err: syscall.EISDIR}
	}
	return os.Remove(path)
}

// errnoText is the strerror text of err, as rm prints it.
func errnoText(err error) string {
	var errno syscall.Errno
	if pe, ok := err.(*os.PathError); ok {
		if e, ok := pe.Err.(syscall.Errno); ok {
			errno = e
		}
	}
	switch errno {
	case syscall.EISDIR:
		return "Is a directory"
	case syscall.EACCES:
		return "Permission denied"
	case syscall.EPERM:
		return "Operation not permitted"
	case syscall.EROFS:
		return "Read-only file system"
	}
	if errno != 0 {
		return errno.Error()
	}
	return err.Error()
}
