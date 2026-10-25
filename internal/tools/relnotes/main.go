// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

// Command relnotes prints the CHANGELOG.md entry of one version, the body of
// its "## [VERSION] - DATE" section without the heading, for the release
// workflow to pass to goreleaser as the GitHub Release notes:
//
//	go run ./internal/tools/relnotes -version 3.0.1 > notes.md
//
// It fails when the section is missing or empty, so a tag whose CHANGELOG
// entry was forgotten does not publish a release.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func main() {
	version := flag.String("version", "", "the version whose entry to print (VERSION, without the v)")
	path := flag.String("changelog", "CHANGELOG.md", "the changelog file")
	flag.Parse()
	f, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relnotes:", err)
		os.Exit(1)
	}
	defer f.Close()
	notes, err := entry(f, strings.TrimPrefix(*version, "v"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "relnotes:", err)
		os.Exit(1)
	}
	fmt.Print(notes)
}

// entry returns the section of version: the lines after its "## [version]"
// heading up to the next "## [" heading or the link references at the end,
// trimmed of leading and trailing blank lines.
func entry(r io.Reader, version string) (string, error) {
	if version == "" {
		return "", errors.New("-version is required")
	}
	head := "## [" + version + "]"
	var lines []string
	in, found := false, false
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == head || strings.HasPrefix(line, head+" "):
			in, found = true, true
			continue
		case in && (strings.HasPrefix(line, "## [") || strings.HasPrefix(line, "[")):
			in = false
		}
		if in {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		return "", err
	}
	if !found {
		return "", fmt.Errorf("CHANGELOG.md has no %q section", head)
	}
	notes := strings.Trim(strings.Join(lines, "\n"), "\n")
	if strings.TrimSpace(notes) == "" {
		return "", fmt.Errorf("the %q section of CHANGELOG.md is empty", head)
	}
	return notes + "\n", nil
}
