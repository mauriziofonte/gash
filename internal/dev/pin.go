// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package dev

import "github.com/mauriziofonte/gash/internal/invoke"

// ProjectRequest is the version word that asks for the version the
// project pins (--runtime php:project, llm_runtime node project).
const ProjectRequest = "project"

// ProjectPin is the version the project in dir pins for family ("php" or
// "node"), read as php_detect_version and node_detect_version read it
// (php: X.Y; node: the major), and the file that pins it. It is "" when
// no file does: their defaults (8.4, the newest installed node) are not
// pins.
func ProjectPin(c *invoke.Call, family, dir string) (version, file string) {
	m := New()
	switch family {
	case "php":
		return m.phpPin(c, dir)
	case "node":
		return m.nodePin(nvmDir(c), dir)
	}
	return "", ""
}

// PinFiles names the files ProjectPin reads for family, for the hint of
// a project that pins nothing.
func PinFiles(family string) string {
	if family == "node" {
		return ".nvmrc, .node-version or package.json engines.node"
	}
	return ".target-php-version, composer.json require.php or .github/copilot-instructions.md"
}
