// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"database/sql"
	"math"
	"strconv"
	"strings"
	"time"
)

// mysqlColumn is what the text of a value depends on besides the value:
// the column type name and, for FLOAT and DOUBLE, a fixed number of
// decimals (FLOAT(M,D), DOUBLE(M,D), AVG and the like).
type mysqlColumn struct {
	typeName string
	decimals int // -1: not fixed
}

// mysqlColumns reads the column metadata the driver gives.
func mysqlColumns(cts []*sql.ColumnType) []mysqlColumn {
	out := make([]mysqlColumn, len(cts))
	for i, ct := range cts {
		out[i] = mysqlColumn{typeName: ct.DatabaseTypeName(), decimals: -1}
		if _, scale, ok := ct.DecimalSize(); ok && scale >= 0 && scale < 31 {
			out[i].decimals = int(scale)
		}
	}
	return out
}

// mysqlText is a value as the mysql client printed it (LEDGER DB-02): the
// server's text. go-sql-driver/mysql parses integer, FLOAT and DOUBLE
// columns even on the text protocol, so those are rebuilt: integers in
// decimal (YEAR zero-padded to four digits; ZEROFILL padding is lost,
// DB-18), FLOAT and DOUBLE with the server's formatting. DECIMAL, strings,
// dates and the rest are the bytes sent; NULL is "NULL".
func mysqlText(v any, col mysqlColumn) string {
	switch x := v.(type) {
	case nil:
		return "NULL"
	case []byte:
		return string(x)
	case string:
		return x
	case int64:
		if col.typeName == "YEAR" && x >= 0 && x < 1000 {
			return padYear(strconv.FormatInt(x, 10))
		}
		return strconv.FormatInt(x, 10)
	case uint64:
		if col.typeName == "YEAR" && x < 1000 {
			return padYear(strconv.FormatUint(x, 10))
		}
		return strconv.FormatUint(x, 10)
	case float32:
		return mysqlFloat(float64(x), 32, col.decimals)
	case float64:
		return mysqlFloat(x, 64, col.decimals)
	case time.Time:
		return x.Format("2006-01-02 15:04:05")
	}
	return ""
}

func padYear(s string) string {
	return strings.Repeat("0", 4-len(s)) + s
}

// mysqlFloat is the server's text of a FLOAT (bits 32) or DOUBLE value.
// With fixed decimals: that many digits after the point. Otherwise the
// significant digits (the shortest that read back as the double; for a
// FLOAT six, the server's precision, trailing zeros dropped) in fixed
// notation while the decimal point falls within 15 places of the first
// digit on either side, or anywhere inside the digits; else as D.DDDeN
// (no plus sign, no leading zeros). Checked against MySQL 8.0 and MariaDB
// 11, which agree; negative zero prints as "-0" (MariaDB sends "0").
func mysqlFloat(f float64, bits, decimals int) string {
	if math.IsInf(f, 0) || math.IsNaN(f) {
		// Not storable in MySQL; the driver cannot hand one over.
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	if decimals >= 0 {
		return strconv.FormatFloat(f, 'f', decimals, 64)
	}
	sign := ""
	if math.Signbit(f) {
		sign = "-"
		f = -f
	}
	if f == 0 {
		return sign + "0"
	}
	prec := -1
	if bits == 32 {
		prec = 5
	}
	e := strconv.FormatFloat(f, 'e', prec, 64)
	mant, exp, _ := strings.Cut(e, "e")
	digits := strings.TrimRight(strings.Replace(mant, ".", "", 1), "0")
	x, _ := strconv.Atoi(exp)
	decpt := x + 1 // the value is 0.DIGITS times 10^decpt
	n := len(digits)
	if decpt > -15 && (decpt <= 15 || decpt < n) {
		switch {
		case decpt <= 0:
			return sign + "0." + strings.Repeat("0", -decpt) + digits
		case decpt < n:
			return sign + digits[:decpt] + "." + digits[decpt:]
		default:
			return sign + digits + strings.Repeat("0", decpt-n)
		}
	}
	out := sign + digits[:1]
	if n > 1 {
		out += "." + digits[1:]
	}
	return out + "e" + strconv.Itoa(decpt-1)
}
