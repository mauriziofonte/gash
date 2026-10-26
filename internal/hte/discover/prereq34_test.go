// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package discover

import (
	"testing"

	"github.com/mauriziofonte/gash/internal/platform"
)

// Without a version, PHPInstallHint names the distribution's own PHP-FPM
// (3.4: the doctor and install print it when none is installed).
func TestPHPInstallHintDefault(t *testing.T) {
	for f, want := range map[platform.Family]string{
		platform.FamilyDebian: "sudo apt install php-fpm",
		platform.FamilyFedora: "sudo dnf install php-fpm",
		platform.FamilyArch:   "sudo pacman -S php-fpm",
		platform.FamilyDarwin: "brew install php",
		platform.Family(""):   "",
	} {
		if got := PHPInstallHint(f, ""); got != want {
			t.Errorf("%q: %q, want %q", f, got, want)
		}
	}
}
