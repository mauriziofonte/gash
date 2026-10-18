// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Package buildinfo holds the facts the build stamps into the binary.
package buildinfo

// Version is the Gash version of the binary, set at build time from the
// VERSION file:
//
//	go build -ldflags "-X github.com/mauriziofonte/gash/internal/buildinfo.Version=$(cat VERSION)"
//
// A build without the flag reports "dev".
var Version = "dev"
