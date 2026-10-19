// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package fsscan

import (
	"math"
	"math/big"
	"strings"
)

// maxInt64 is 2^63-1, the largest value a size fits before Bash arithmetic
// wrapped (FS-22).
const maxInt64 = math.MaxInt64

// numfmtMax is 10^19: numfmt --from=iec refuses to print a value this large
// or larger ("value too large to be printed"), so 2.x fell through to the
// pure-bash fallback for it.
var numfmtMax = new(big.Int).Exp(big.NewInt(10), big.NewInt(19), nil)

// Size is the result of parsing a size string the way 2.x
// __gash_fs_parse_size did on the GNU path (numfmt present).
type Size struct {
	// Bytes is the value, clamped to maxInt64 when the true value is
	// larger (Big is then true).
	Bytes int64
	// Big reports that the true value exceeded 2^63-1.
	Big bool
	// Wrapped reports a value whose 2.x meaning came from a Bash-arithmetic
	// multiplication that overflowed intmax_t (the fallback K..P product):
	// rc.A refuses such a value (FS-22).
	Wrapped bool
}

// ParseSize emulates 2.x __gash_fs_parse_size on the documented GNU path:
// LC_ALL=C numfmt --from=iec first, and the pure-bash fallback for inputs
// numfmt refuses (FS-11). ok is false exactly where 2.x returned 1. The
// result never depends on whether numfmt is installed (MUST NOT 8, FS-22).
func ParseSize(input string) (Size, bool) {
	if input == "" {
		return Size{}, false
	}
	if n, ok := numfmtFromIEC(input); ok {
		return sizeOf(n), true
	}
	return fallbackSize(input)
}

// sizeOf turns a non-negative big.Int (a numfmt-path value) into a Size,
// clamping above 2^63-1.
func sizeOf(n *big.Int) Size {
	if n.IsInt64() {
		return Size{Bytes: n.Int64()}
	}
	return Size{Bytes: maxInt64, Big: true}
}

// numfmtFromIEC reproduces the subset of `numfmt --from=iec` whose output
// is a run of ASCII digits (the only output 2.x accepted, via
// `[[ "$nf" =~ ^[0-9]+$ ]]`). ok is false when numfmt would refuse the
// input, output a non-integer (a decimal with no suffix), or find it too
// large to print; the caller then falls back.
func numfmtFromIEC(in string) (*big.Int, bool) {
	s := strings.TrimLeft(in, " \t")
	shift := uint(0)
	if n := len(s); n > 0 {
		// A single uppercase IEC suffix; numfmt rejects k, m, i and B.
		switch s[n-1] {
		case 'K':
			shift = 10
		case 'M':
			shift = 20
		case 'G':
			shift = 30
		case 'T':
			shift = 40
		case 'P':
			shift = 50
		case 'E':
			shift = 60
		}
		if shift > 0 {
			s = s[:n-1]
		}
	}
	intPart, fracPart, okNum := splitDecimal(s)
	if !okNum {
		return nil, false
	}
	// numfmt refuses a mantissa with too many digits ("value too large to
	// be converted"); 2.x then fell back.
	if len(intPart)+len(fracPart) > 33 {
		return nil, false
	}
	var q *big.Int
	if shift == 0 {
		// No suffix: numfmt echoes an integer, but a decimal prints with a
		// dot and fails the digit test, so only a pure integer is accepted.
		if fracPart != "" {
			return nil, false
		}
		q, _ = new(big.Int).SetString(normInt(intPart), 10)
	} else {
		// value = ceil(mantissa * 2^shift), computed exactly.
		num, _ := new(big.Int).SetString(normInt(intPart)+fracPart, 10)
		den := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(len(fracPart))), nil)
		num.Lsh(num, shift)
		var r big.Int
		q, _ = new(big.Int).QuoRem(num, den, &r)
		if r.Sign() > 0 {
			q.Add(q, big.NewInt(1))
		}
	}
	if q.Cmp(numfmtMax) >= 0 {
		return nil, false // numfmt: value too large to be printed
	}
	return q, true
}

// splitDecimal splits a mantissa into its integer and fractional digit
// runs. ok is false unless there is at least one digit and the only
// non-digit is a single '.'.
func splitDecimal(s string) (intPart, fracPart string, ok bool) {
	dot := strings.IndexByte(s, '.')
	if dot < 0 {
		intPart = s
	} else {
		intPart, fracPart = s[:dot], s[dot+1:]
		if strings.IndexByte(fracPart, '.') >= 0 {
			return "", "", false
		}
	}
	if intPart+fracPart == "" {
		return "", "", false
	}
	for _, p := range []string{intPart, fracPart} {
		for i := 0; i < len(p); i++ {
			if p[i] < '0' || p[i] > '9' {
				return "", "", false
			}
		}
	}
	return intPart, fracPart, true
}

// normInt returns the integer digits with leading zeros trimmed but at
// least one digit kept, so SetString never sees an empty string.
func normInt(s string) string {
	s = strings.TrimLeft(s, "0")
	if s == "" {
		return "0"
	}
	return s
}

// fallbackSize reproduces the pure-bash fallback of __gash_fs_parse_size:
// strip spaces, join every digit into the number, take the rest as the
// unit, strip one trailing 'B' then one 'i', uppercase, and multiply by
// the IEC factor. A K..P product above 2^63-1 wrapped in Bash (FS-22);
// rc.A reports it wrapped so the caller refuses it. A bare number with no
// unit did no arithmetic, so it is kept (clamped) rather than wrapped.
func fallbackSize(input string) (Size, bool) {
	s := strings.ReplaceAll(input, " ", "")
	var num, unit strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			num.WriteByte(s[i])
		} else {
			unit.WriteByte(s[i])
		}
	}
	if num.Len() == 0 {
		return Size{}, false
	}
	u := unit.String()
	u = strings.TrimSuffix(u, "B")
	u = strings.TrimSuffix(u, "i")
	shift, okUnit := fallbackShift(strings.ToUpper(u))
	if !okUnit {
		return Size{}, false
	}
	n, _ := new(big.Int).SetString(num.String(), 10)
	if shift > 0 {
		n.Lsh(n, shift)
	}
	if n.IsInt64() {
		return Size{Bytes: n.Int64()}, true
	}
	if shift == 0 {
		// A plain integer numfmt could not print: no arithmetic ran, so
		// 2.x kept the digits (used as a float threshold); clamp it.
		return Size{Bytes: maxInt64, Big: true}, true
	}
	return Size{Bytes: maxInt64, Big: true, Wrapped: true}, true
}

// fallbackShift maps the uppercased, B/i-stripped unit to a left shift.
func fallbackShift(u string) (uint, bool) {
	switch u {
	case "":
		return 0, true
	case "K":
		return 10, true
	case "M":
		return 20, true
	case "G":
		return 30, true
	case "T":
		return 40, true
	case "P":
		return 50, true
	}
	return 0, false
}
