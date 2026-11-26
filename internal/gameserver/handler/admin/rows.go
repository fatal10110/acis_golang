package admin

import "strings"

// OpenStripedRow starts the row-th line of a 280-wide admin list: a one-row
// table with attrs written after its width, on a black background when row
// is even.
func OpenStripedRow(b *strings.Builder, row int, attrs string) {
	b.WriteString("<table width=280")
	b.WriteString(attrs)
	if row%2 == 0 {
		b.WriteString(" bgcolor=000000")
	}
	b.WriteString("><tr>")
}

// CloseStripedRow ends a line OpenStripedRow started and draws the gray
// separator under it.
func CloseStripedRow(b *strings.Builder) {
	b.WriteString(`</tr></table><img src="L2UI.SquareGray" width=280 height=1>`)
}
