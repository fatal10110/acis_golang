package admin

import (
	"strings"
	"testing"
)

// TestSearch compares the search bar byte for byte with
// Pagination.generateSearch("bypass admin_help", 45) over 12 entries found.
func TestSearch(t *testing.T) {
	t.Parallel()
	const want = `<table width=280 height=45><tr><td width=70 align=center>Search</td>` +
		`<td width=140><edit var="search" width=130 height=15></td>` +
		`<td width=70><button value="Find" action="bypass admin_help 1 $search" width=65 height=19 back="L2UI_ch3.smallbutton2_over" fore="L2UI_ch3.smallbutton2"></td>` +
		`</tr><tr><td></td><td align=center>Found 12 results</td><td></td></tr></table>`
	var b strings.Builder
	Search(&b, "bypass admin_help", 45, 12)
	if b.String() != want {
		t.Fatalf("Search =\n%s\nwant\n%s", b.String(), want)
	}
}
