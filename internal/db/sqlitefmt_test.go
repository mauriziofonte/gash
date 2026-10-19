// Gash 3.0. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package db

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSqliteRealFixed(t *testing.T) {
	for in, want := range map[float64]string{
		0: "0.0", math.Copysign(0, -1): "0.0", 1: "1.0", 0.1: "0.100000000000000005", 1e300: "1.000000000000000052e+300",
		1e15: "1000000000000000.0", 1e20: "1.0e+20", 0.000001: "9.99999999999999954e-07", 1.0 / 3: "0.333333333333333314",
		123456789.123456789: "123456789.123456791", 5e-324: "4.94065645841246544e-324", 9.3e18: "9300000000000000000.0",
		0.0001: "0.000100000000000000004", 12345678901234567890.0: "12345678901234567160.0", -1.5: "-1.5",
		math.Inf(1): "9.0e+999", math.Inf(-1): "-9.0e+999",
	} {
		require.Equal(t, want, sqliteReal(in), "%v", in)
	}
}

// TestSqliteRealMatchesCLI checks random doubles against the sqlite3 shell
// the 2.x code ran (ieee754_from_blob gives it the exact bits).
func TestSqliteRealMatchesCLI(t *testing.T) {
	if _, err := exec.LookPath("sqlite3"); err != nil {
		t.Skip("sqlite3 is not installed")
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for round := 0; round < 20; round++ {
		var vals []float64
		var cols []string
		for i := 0; i < 200; i++ {
			var v float64
			switch i % 4 {
			case 0:
				v = math.Float64frombits(rng.Uint64())
			case 1:
				v = rng.Float64() * math.Pow(10, float64(rng.IntN(40)-20))
			case 2:
				v = float64(rng.IntN(1000000)) / float64(1+rng.IntN(1000))
			default:
				v = math.Float64frombits(rng.Uint64() & 0x800fffffffffffff)
			}
			if math.IsNaN(v) {
				continue
			}
			vals = append(vals, v)
			var b [8]byte
			binary.BigEndian.PutUint64(b[:], math.Float64bits(v))
			cols = append(cols, fmt.Sprintf("ieee754_from_blob(x'%s') AS c%d", hex.EncodeToString(b[:]), len(vals)-1))
		}
		out, err := exec.Command("sqlite3", "-json", ":memory:", "SELECT "+strings.Join(cols, ", ")).Output()
		require.NoError(t, err)
		dec := json.NewDecoder(strings.NewReader(string(out)))
		dec.UseNumber()
		var rows []map[string]json.Number
		require.NoError(t, dec.Decode(&rows))
		require.Len(t, rows, 1)
		for i, v := range vals {
			got := rows[0][fmt.Sprintf("c%d", i)].String()
			require.Equal(t, got, sqliteReal(v), "bits %016x", math.Float64bits(v))
		}
	}
}
