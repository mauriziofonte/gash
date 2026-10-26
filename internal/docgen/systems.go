// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package docgen

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"
)

// systemNames name the distribution images of the CI smoke.
var systemNames = map[string]string{
	"library/rockylinux":  "Rocky Linux",
	"library/almalinux":   "AlmaLinux",
	"library/oraclelinux": "Oracle Linux",
	"library/amazonlinux": "Amazon Linux",
	"library/fedora":      "Fedora",
	"library/debian":      "Debian",
	"library/ubuntu":      "Ubuntu",
	"library/alpine":      "Alpine Linux",
	"opensuse/leap":       "openSUSE Leap",
	"opensuse/tumbleweed": "openSUSE Tumbleweed",
	"library/archlinux":   "Arch Linux",
}

// hteImageRe reads the images of testdata/hte/e2e/run.sh:
// "    debian) def=mirror.gcr.io/library/debian:12 ...".
var hteImageRe = regexp.MustCompile(`(?m)^\s+[a-z]+\) def=mirror\.gcr\.io/(\S+)`)

// ciWorkflow is the part of .github/workflows/ci.yml the table reads.
type ciWorkflow struct {
	Jobs map[string]struct {
		Strategy struct {
			Matrix struct {
				OS      []string            `yaml:"os"`
				Image   []string            `yaml:"image"`
				Exclude []map[string]string `yaml:"exclude"`
			} `yaml:"matrix"`
		} `yaml:"strategy"`
	} `yaml:"jobs"`
}

// Systems is the table of the systems CI installs Gash on at every change:
// the images of the distro smoke job of ci.yml (install.sh from the bare
// image, gash_doctor, every catalog package), on amd64 and arm64 unless
// excluded, and whether the HTE end-to-end story (testdata/hte/e2e/run.sh)
// runs on the same image. macOS is the unit, testscript and HTE jobs.
func Systems(ciYAML, hteRun string) (string, error) {
	var wf ciWorkflow
	if err := yaml.Unmarshal([]byte(ciYAML), &wf); err != nil {
		return "", err
	}
	job, ok := wf.Jobs["distro-smoke"]
	if !ok {
		return "", fmt.Errorf("ci.yml has no distro-smoke job")
	}
	m := job.Strategy.Matrix
	if len(m.Image) == 0 || len(m.OS) == 0 {
		return "", fmt.Errorf("the distro-smoke matrix has no image or os")
	}
	hte := map[string]bool{}
	for _, sm := range hteImageRe.FindAllStringSubmatch(hteRun, -1) {
		hte[sm[1]] = true
	}
	if len(hte) == 0 {
		return "", fmt.Errorf("no image in testdata/hte/e2e/run.sh")
	}
	excluded := func(os, image string) bool {
		for _, e := range m.Exclude {
			if e["os"] == os && e["image"] == image {
				return true
			}
		}
		return false
	}
	archOf := func(os string) string {
		if strings.HasSuffix(os, "-arm") {
			return "arm64"
		}
		return "amd64"
	}
	var b strings.Builder
	b.WriteString("| System | Version | amd64 | arm64 | Local sites (`gash hte`) |\n|---|---|---|---|---|\n")
	for _, image := range m.Image {
		repo, tag, _ := strings.Cut(image, ":")
		name, ok := systemNames[repo]
		if !ok {
			return "", fmt.Errorf("no name for the image %s: add it to systemNames", image)
		}
		version := tag
		switch {
		case tag == "latest":
			version = "rolling"
		case strings.HasSuffix(tag, "-minimal"):
			version = strings.TrimSuffix(tag, "-minimal") + " (minimal image)"
		}
		var arches []string
		for _, os := range m.OS {
			if !excluded(os, image) {
				arches = append(arches, archOf(os))
			}
		}
		mark := func(arch string) string {
			if slices.Contains(arches, arch) {
				return "tested"
			}
			return "no image"
		}
		site := "not tested"
		if hte[image] {
			site = "tested"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", name, version, mark("amd64"), mark("arm64"), site)
	}
	b.WriteString("| macOS (Homebrew Bash) | current | built, not tested | tested | tested (Homebrew httpd and PHP) |\n")
	return b.String(), nil
}
