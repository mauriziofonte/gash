// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command gash is the Gash 3.x binary.
package main

import (
	"os"

	"github.com/mauriziofonte/gash/internal/app"
)

func main() {
	os.Exit(app.Main(os.Args[1:], os.Environ(), os.Stdin, os.Stdout, os.Stderr))
}
