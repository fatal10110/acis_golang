package admin

import (
	"strings"
	"testing"
)

// TestStripedRow pins the bytes of a striped list line: even rows black,
// attrs after the width, the gray separator after the row.
func TestStripedRow(t *testing.T) {
	for _, tc := range []struct {
		row   int
		attrs string
		want  string
	}{
		{0, "", "<table width=280 bgcolor=000000><tr>x" + separator},
		{1, "", "<table width=280><tr>x" + separator},
		{2, " height=41", "<table width=280 height=41 bgcolor=000000><tr>x" + separator},
		{3, " height=41", "<table width=280 height=41><tr>x" + separator},
	} {
		var b strings.Builder
		OpenStripedRow(&b, tc.row, tc.attrs)
		b.WriteString("x")
		CloseStripedRow(&b)
		if got := b.String(); got != tc.want {
			t.Errorf("row %d attrs %q = %q, want %q", tc.row, tc.attrs, got, tc.want)
		}
	}
}

const separator = `</tr></table><img src="L2UI.SquareGray" width=280 height=1>`
