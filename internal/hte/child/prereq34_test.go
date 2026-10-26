// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package child

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// UserDirs adds Homebrew, ~/.local/bin and the user's PATH to BasePath,
// and leaves out what is not an absolute, private directory (3.4: only
// BasePath was searched).
func TestUserDirs(t *testing.T) {
	root := t.TempDir()
	mk := func(rel string, mode os.FileMode) string {
		t.Helper()
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, mode); err != nil {
			t.Fatal(err)
		}
		return "/" + rel
	}
	brew := mk("opt/homebrew/bin", 0o755)
	local := mk("home/me/.local/bin", 0o700)
	tools := mk("srv/tools/bin", 0o755)
	open := mk("srv/open", 0o777)
	group := mk("srv/group", 0o775)
	if err := os.WriteFile(filepath.Join(root, "srv/file"), nil, 0o755); err != nil {
		t.Fatal(err)
	}
	stat := func(p string) (os.FileInfo, error) { return os.Stat(filepath.Join(root, p)) }

	path := strings.Join([]string{"bin", "./x", "", tools, "/usr/bin", open, group, "/srv/file", "/srv/none", tools + "/"}, ":")
	got := UserDirs("/home/me", path, stat)
	want := append(strings.Split(BasePath, ":"), brew, local, tools)
	if !slices.Equal(got, want) {
		t.Errorf("UserDirs:\n got %q\nwant %q", got, want)
	}
	// A relative HOME adds no ~/.local/bin.
	if got := UserDirs("me", "", stat); !slices.Equal(got, append(strings.Split(BasePath, ":"), brew)) {
		t.Errorf("relative HOME: %q", got)
	}
}
