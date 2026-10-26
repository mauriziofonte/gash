// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import "testing"

// TestWindowsSecretNames: the Windows secrets reachable from WSL (3.4) are
// detected by their base name, wherever they live, case-insensitively.
func TestWindowsSecretNames(t *testing.T) {
	secret := []string{
		"/mnt/c/Users/me/NTUSER.DAT",
		"/mnt/c/Users/me/AppData/Local/ConnectionManager/saved.rdp",
		"/c/Windows/Panther/unattend.xml",
		"/mnt/c/Users/me/AppData/Roaming/Mozilla/Firefox/Profiles/x/logins.json",
		"/mnt/c/Users/me/AppData/Roaming/Mozilla/Firefox/Profiles/x/key4.db",
		"/mnt/c/Users/me/AppData/Local/Google/Chrome/User Data/Default/Login Data",
	}
	for _, p := range secret {
		if !SecretName(p) {
			t.Errorf("SecretName(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"/home/me/project/main.go", "/home/me/notes.txt", "/mnt/c/Users/me/report.docx"} {
		if SecretName(p) {
			t.Errorf("SecretName(%q) = true, want false", p)
		}
	}
}
