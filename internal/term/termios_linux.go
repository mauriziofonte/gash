// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package term

import "golang.org/x/sys/unix"

const (
	ioctlGetTermios = unix.TCGETS
	// TCSETSW: tcsetattr(TCSADRAIN), as Bash's read sets the terminal.
	ioctlSetTermios = unix.TCSETSW
)
