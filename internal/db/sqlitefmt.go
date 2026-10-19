// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"math"
	"math/big"
	"strconv"
)

// sqliteReal is the text the sqlite3 shell (3.45, the 2.x client) printed
// for a REAL in -json mode: sqlite3_snprintf "%!.20g", infinities as
// 9.0e+999 and -9.0e+999. Negative zero prints as 0.0, as there.
func sqliteReal(r float64) string {
	switch {
	case math.IsInf(r, 1):
		return "9.0e+999"
	case math.IsInf(r, -1):
		return "-9.0e+999"
	}
	digits, iDP, neg := fpDecode(r)
	return genericAlt2(digits, iDP, neg, 20)
}

// ld is a long double constant (x87 extended: 64-bit significand).
func ld(s string) *big.Float {
	f, _, err := big.ParseFloat(s, 10, 64, big.ToNearestEven)
	if err != nil {
		panic(err)
	}
	return f
}

var (
	ld1e119  = ld("1e119")
	ld1em100 = ld("1e-100")
	ld1e29   = ld("1e29")
	ld1em10  = ld("1e-10")
	ld1e19   = ld("1e19")
	ld1em1   = ld("1e-1")
	ld1em97  = ld("1e-97")
	ld1e100  = ld("1e100")
	ld1e7    = ld("1e7")
	ld1e10   = ld("1e10")
	ld1e17   = ld("1e17")
	ld10     = ld("10")
)

// fpDecode is sqlite3FpDecode(r, 20, 26) on the long double path the
// x86-64 builds take: the significant digits (trailing zeros removed),
// the position of the decimal point and the sign.
func fpDecode(r float64) (string, int, bool) {
	neg := false
	if r < 0 {
		neg = true
		r = -r
	} else if r == 0 {
		return "0", 1, false
	}
	rr := new(big.Float).SetPrec(64).SetMode(big.ToNearestEven).SetFloat64(r)
	mul := func(k *big.Float) { rr.Mul(rr, k) }
	exp := 0
	if rr.Cmp(ld1e19) >= 0 {
		for rr.Cmp(ld1e119) >= 0 {
			exp += 100
			mul(ld1em100)
		}
		for rr.Cmp(ld1e29) >= 0 {
			exp += 10
			mul(ld1em10)
		}
		for rr.Cmp(ld1e19) >= 0 {
			exp++
			mul(ld1em1)
		}
	} else {
		for rr.Cmp(ld1em97) < 0 {
			exp -= 100
			mul(ld1e100)
		}
		for rr.Cmp(ld1e7) < 0 {
			exp -= 10
			mul(ld1e10)
		}
		for rr.Cmp(ld1e17) < 0 {
			exp--
			mul(ld10)
		}
	}
	v, _ := rr.Uint64()
	z := strconv.FormatUint(v, 10)
	iDP := len(z) + exp
	n := len(z)
	for n > 1 && z[n-1] == '0' {
		n--
	}
	return z[:n], iDP, neg
}

// genericAlt2 is the etGENERIC conversion of sqlite's printf with the "!"
// flag and the given precision, from the decoded digits.
func genericAlt2(z string, iDP int, neg bool, precision int) string {
	var b []byte
	if neg {
		b = append(b, '-')
	}
	exp := iDP - 1
	if precision > 0 {
		precision--
	}
	isExp := exp < -4 || exp > precision
	e2 := 0
	if !isExp {
		precision -= exp
		e2 = iDP - 1
	}
	j := 0
	next := func() byte {
		if j < len(z) {
			j++
			return z[j-1]
		}
		return '0'
	}
	if e2 < 0 {
		b = append(b, '0')
	} else {
		for ; e2 >= 0; e2-- {
			b = append(b, next())
		}
	}
	b = append(b, '.')
	for e2++; e2 < 0 && precision > 0; precision, e2 = precision-1, e2+1 {
		b = append(b, '0')
	}
	for ; precision > 0; precision-- {
		b = append(b, next())
	}
	for b[len(b)-1] == '0' {
		b = b[:len(b)-1]
	}
	if b[len(b)-1] == '.' {
		b = append(b, '0')
	}
	if isExp {
		e := iDP - 1
		b = append(b, 'e')
		if e < 0 {
			b = append(b, '-')
			e = -e
		} else {
			b = append(b, '+')
		}
		if e >= 100 {
			b = append(b, byte('0'+e/100))
			e %= 100
		}
		b = append(b, byte('0'+e/10), byte('0'+e%10))
	}
	return string(b)
}
