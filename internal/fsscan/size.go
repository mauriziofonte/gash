// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"math"
	"math/big"
	"strings"

	"github.com/mauriziofonte/gash/internal/lazyre"
)

// maxInt64 is 2^63-1, the largest size a scanner accepts (FS-22).
const maxInt64 = math.MaxInt64

// SizeGrammar is the documented size grammar of every size flag (FS-11,
// FS-34): the help texts quote it.
const SizeGrammar = "bytes, or a number with up to 6 decimals and K, M, G, T, P or E, optionally followed by B, i or iB"

// sizeRe is the 3.0 size grammar: up to 15 integer digits, up to 6
// decimals, then an optional IEC unit (K..E, optionally followed by iB, i
// or B) or a bare B. Matched case-insensitively, no spaces.
var sizeRe = lazyre.MustCompile(`^([0-9]{1,15})(?:\.([0-9]{1,6}))?(?:([KMGTPE])(?:IB|I|B)?|B)?$`)

// ParseSize is the native size parser of the five scanners (FS-11): value
// = ceil(number x 1024^k) for unit letter k (K=1 .. E=6). ok is false for
// any input outside the grammar and for a value above 2^63-1 (FS-22). The
// result never depends on an installed tool (MUST NOT 8).
func ParseSize(input string) (int64, bool) {
	m := sizeRe.FindStringSubmatch(strings.ToUpper(input))
	if m == nil {
		return 0, false
	}
	shift := uint(0)
	if m[3] != "" {
		shift = 10 * uint(1+strings.IndexByte("KMGTPE", m[3][0]))
	}
	num, _ := new(big.Int).SetString(m[1]+m[2], 10)
	num.Lsh(num, shift)
	den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(m[2]))), nil)
	var r big.Int
	q, _ := new(big.Int).QuoRem(num, den, &r)
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	if !q.IsInt64() {
		return 0, false
	}
	return q.Int64(), true
}
