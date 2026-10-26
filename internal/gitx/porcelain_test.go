// Gash 3.4. Author: Maurizio Fonte <fonte.maurizio@gmail.com>

package gitx

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseStatus(t *testing.T) {
	out := "# branch.oid 524a5c13de42a89835fc0465aede8f08b288440d\x00" +
		"# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +2 -1\x00" +
		"1 .M N... 100644 100644 100644 aaa aaa file with space.txt\x00" +
		"1 MM N... 100644 100644 100644 aaa bbb both.txt\x00" +
		"2 R. N... 100644 100644 100644 aaa aaa R100 new name.txt\x00old name.txt\x00" +
		"u UU N... 100644 100644 100644 100644 a b c conflict.txt\x00" +
		"? untracked dir/\x00"
	st := ParseStatus([]byte(out))
	require.Equal(t, "524a5c13de42a89835fc0465aede8f08b288440d", st.Oid)
	require.Equal(t, "main", st.Branch)
	require.False(t, st.Detached)
	require.Equal(t, "origin/main", st.Upstream)
	require.Equal(t, 2, st.Ahead)
	require.Equal(t, 1, st.Behind)
	require.True(t, st.HasAB)
	require.Len(t, st.Entries, 5)
	require.Equal(t, "new name.txt", st.Entries[2].Path)
	require.Equal(t, "old name.txt", st.Entries[2].Orig)
	require.Equal(t, "R.", st.Entries[2].XY())
	require.Equal(t, "??", st.Entries[4].XY())
	s, m, u, c := st.Counts()
	require.Equal(t, []int{2, 2, 1, 1}, []int{s, m, u, c})
	ls, lm, lu, lc := st.Lists()
	require.Equal(t, []string{"both.txt", "new name.txt"}, ls)
	require.Equal(t, []string{"file with space.txt", "both.txt"}, lm)
	require.Equal(t, []string{"untracked dir/"}, lu)
	require.Equal(t, []string{"conflict.txt"}, lc)

	st = ParseStatus([]byte("# branch.oid (initial)\x00# branch.head (detached)\x00"))
	require.Equal(t, "", st.Oid)
	require.Equal(t, "", st.Branch)
	require.True(t, st.Detached)
	require.False(t, st.HasAB)
	ls, _, _, _ = st.Lists()
	require.NotNil(t, ls)
}

func TestParseRawNumstat(t *testing.T) {
	// --raw --numstat -z with a rename and a binary file.
	fields := SplitZ([]byte(":100644 100644 aaa bbb M\x00a.txt\x00" +
		":100644 100644 ccc ccc R086\x00old.txt\x00new.txt\x00" +
		":000000 100644 000 ddd A\x00img.png\x00" +
		"3\t1\ta.txt\x00" + "1\t0\t\x00old.txt\x00new.txt\x00" + "-\t-\timg.png\x00"))
	got := ParseRawNumstat(fields)
	require.Equal(t, []FileChange{
		{Path: "a.txt", Status: "M", Add: 3, Del: 1},
		{Path: "new.txt", Orig: "old.txt", Status: "R", Add: 1},
		{Path: "img.png", Status: "A", Binary: true},
	}, got)
	n, ins, del := Totals(got)
	require.Equal(t, []int{3, 4, 1}, []int{n, ins, del})

	// --numstat alone.
	got = ParseRawNumstat(SplitZ([]byte("2\t2\tx\x00")))
	require.Equal(t, []FileChange{{Path: "x", Add: 2, Del: 2}}, got)
	require.Nil(t, ParseRawNumstat(nil))
}

func TestLastLine(t *testing.T) {
	require.Equal(t, "fatal: bad revision 'x'", LastLine([]byte("hint: one\nfatal: bad revision 'x'\nhint: two\n"), 0))
	require.Equal(t, "last", LastLine([]byte("first\n\nlast\n\n"), 0))
	require.Equal(t, "ab…", LastLine([]byte("abcdef"), 2))
	require.Equal(t, "", LastLine(nil, 10))
}
