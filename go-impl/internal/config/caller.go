// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package config

import "os"

// geteuid is the effective uid the owner check compares with (EUID).
var geteuid = os.Geteuid

// LoadCaller reads the settings file of a call as __gash_load_env did,
// get returning the caller's variables (invoke.Env.Get fits): the file
// ${GASH_ENV_FILE:-$HOME/.gash_env}, the permission checks skipped in
// headless mode unless GASH_DEBUG=1.
//
// The Warnings belong to the shell's own load: 2.x printed them when the
// shell first read the file (gash.sh, which 3.x does through `gash
// __compat env load`), and a function such as gash_db_list then read the
// cache silently. A handler therefore does not print them.
func LoadCaller(get func(name string) string) *File {
	home := get("HOME")
	return Load(Options{
		Path:     FilePath(home, get("GASH_ENV_FILE")),
		Home:     home,
		Headless: get("GASH_HEADLESS") == "1",
		Debug:    get("GASH_DEBUG") == "1",
		EUID:     geteuid(),
	})
}
