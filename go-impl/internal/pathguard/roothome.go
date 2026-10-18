// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package pathguard

import "os/user"

// lookupRootHome returns root's home directory from the passwd database, as
// core.sh reads ~root. An empty string when it cannot be looked up (the
// forbidden list then keeps only the base entries).
func lookupRootHome() string {
	if u, err := user.Lookup("root"); err == nil {
		return u.HomeDir
	}
	return ""
}
