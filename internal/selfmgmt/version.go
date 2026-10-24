// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package selfmgmt

import (
	"regexp"
	"strconv"
	"strings"
)

// versionTag is a release or pre-release tag: an optional v, dot-separated
// numbers, an optional -PRERELEASE (semver identifiers) and +BUILD.
var versionTag = regexp.MustCompile(`^v?([0-9]+(?:\.[0-9]+)*)(?:-([0-9A-Za-z.-]+))?(?:\+[0-9A-Za-z.-]+)?$`)

// compareVersions compares two version tags as semver does: the numbers
// first (missing ones count as 0, so v2.1 equals v2.1.0), then a
// pre-release is lower than its release, and two pre-releases compare by
// their dot-separated identifiers (numeric ones as numbers, below
// alphanumeric ones). ok is false when either tag is not a version.
func compareVersions(a, b string) (cmp int, ok bool) {
	ma, mb := versionTag.FindStringSubmatch(a), versionTag.FindStringSubmatch(b)
	if ma == nil || mb == nil {
		return 0, false
	}
	na, nb := strings.Split(ma[1], "."), strings.Split(mb[1], ".")
	for i := 0; i < len(na) || i < len(nb); i++ {
		if c := compareNumeric(at(na, i), at(nb, i)); c != 0 {
			return c, true
		}
	}
	pa, pb := ma[2], mb[2]
	switch {
	case pa == pb:
		return 0, true
	case pa == "":
		return 1, true
	case pb == "":
		return -1, true
	}
	ia, ib := strings.Split(pa, "."), strings.Split(pb, ".")
	for i := 0; i < len(ia) && i < len(ib); i++ {
		if c := comparePrerelease(ia[i], ib[i]); c != 0 {
			return c, true
		}
	}
	return sign(len(ia) - len(ib)), true
}

func at(s []string, i int) string {
	if i < len(s) {
		return s[i]
	}
	return "0"
}

// compareNumeric compares two digit strings of any length as numbers.
func compareNumeric(a, b string) int {
	a, b = strings.TrimLeft(a, "0"), strings.TrimLeft(b, "0")
	if len(a) != len(b) {
		return sign(len(a) - len(b))
	}
	return strings.Compare(a, b)
}

func comparePrerelease(a, b string) int {
	_, ea := strconv.ParseUint(a, 10, 64)
	_, eb := strconv.ParseUint(b, 10, 64)
	switch {
	case ea == nil && eb == nil:
		return compareNumeric(a, b)
	case ea == nil:
		return -1
	case eb == nil:
		return 1
	}
	return strings.Compare(a, b)
}

func sign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	}
	return 0
}
