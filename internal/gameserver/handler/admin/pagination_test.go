package admin

import (
	"strings"
	"testing"
)

// TestPaginate pins the Pagination constructor: the page shown is clamped
// to [1, total], a number past the last page shows the last one
// (Math.min(page, _total)), a number below 1 on a list with entries shows
// no page (the reference's subList throws), and an empty list shows page 1
// of 0 with no entries.
func TestPaginate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name           string
		n, number, lim int
		want           Page
		ok             bool
		spaceRows      int
	}{
		{name: "first page", n: 20, number: 1, lim: 7, want: Page{Number: 1, Total: 3, Limit: 7, Start: 0, End: 7}, ok: true},
		{name: "middle page", n: 20, number: 2, lim: 7, want: Page{Number: 2, Total: 3, Limit: 7, Start: 7, End: 14}, ok: true},
		{name: "partial last page", n: 20, number: 3, lim: 7, want: Page{Number: 3, Total: 3, Limit: 7, Start: 14, End: 20}, ok: true, spaceRows: 1},
		{name: "past the last page", n: 20, number: 9, lim: 7, want: Page{Number: 3, Total: 3, Limit: 7, Start: 14, End: 20}, ok: true, spaceRows: 1},
		{name: "exact last page", n: 21, number: 3, lim: 7, want: Page{Number: 3, Total: 3, Limit: 7, Start: 14, End: 21}, ok: true},
		{name: "page zero", n: 20, number: 0, lim: 7, ok: false},
		{name: "negative page", n: 20, number: -2, lim: 7, ok: false},
		{name: "empty list", n: 0, number: 5, lim: 7, want: Page{Number: 1, Total: 0, Limit: 7}, ok: true, spaceRows: 7},
		{name: "empty list page zero", n: 0, number: 0, lim: 7, want: Page{Number: 1, Total: 0, Limit: 7}, ok: true, spaceRows: 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := Paginate(tc.n, tc.number, tc.lim)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("Paginate(%d, %d, %d) = %+v, %v; want %+v, %v", tc.n, tc.number, tc.lim, got, ok, tc.want, tc.ok)
			}
			if !ok {
				return
			}
			var b strings.Builder
			got.Space(&b, 17)
			if want := strings.Repeat("<img height=17>", tc.spaceRows); b.String() != want {
				t.Fatalf("Space = %q, want %q", b.String(), want)
			}
		})
	}
}

// TestPageLinks compares the page bar byte for byte with
// Pagination.generatePages: four links before the shown page when that
// many exist (empty cells otherwise), the shown page highlighted, up to
// four links after it, and the last-page button on the total.
func TestPageLinks(t *testing.T) {
	t.Parallel()
	const (
		head  = `<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="go `
		first = `" back=L2UI_CH3.prev1_down fore=L2UI_CH3.prev1 width=16 height=16></td>`
		tail  = `" back=L2UI_CH3.next1_down fore=L2UI_CH3.next1 width=16 height=16></td></tr></table><img src="L2UI.SquareGray" width=280 height=1>`
		lastB = `<td FIXWIDTH=22 align=center><img height=2><button action="go `
		empty = `<td FIXWIDTH=26 align=center></td>`
	)
	cell := func(n string) string {
		return `<td FIXWIDTH=26 align=center><a action="go ` + n + `">` + pad(n) + `</a></td>`
	}
	shown := func(n string) string { return `<td FIXWIDTH=26 align=center><font color=LEVEL>` + n + `</font></td>` }
	cases := []struct {
		name          string
		n, number, li int
		want          string
	}{
		{
			name: "page 6 of 10", n: 100, number: 6, li: 10,
			want: head + "1" + first + cell("2") + cell("3") + cell("4") + cell("5") + shown("06") +
				cell("7") + cell("8") + cell("9") + cell("10") + lastB + "10" + tail,
		},
		{
			name: "page 2 of 3", n: 20, number: 2, li: 7,
			want: head + "1" + first + empty + empty + empty + cell("1") + shown("02") +
				cell("3") + empty + empty + empty + lastB + "3" + tail,
		},
		{
			name: "past the last of 3", n: 20, number: 9, li: 7,
			want: head + "1" + first + empty + empty + cell("1") + cell("2") + shown("03") +
				empty + empty + empty + empty + lastB + "3" + tail,
		},
		{
			name: "page 12 of 12", n: 120, number: 12, li: 10,
			want: head + "1" + first + cell("8") + cell("9") + cell("10") + cell("11") + shown("12") +
				empty + empty + empty + empty + lastB + "12" + tail,
		},
		{
			name: "empty list", n: 0, number: 1, li: 7,
			want: head + "1" + first + empty + empty + empty + empty + shown("01") +
				empty + empty + empty + empty + lastB + "0" + tail,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, ok := Paginate(tc.n, tc.number, tc.li)
			if !ok {
				t.Fatalf("Paginate(%d, %d, %d) shows no page", tc.n, tc.number, tc.li)
			}
			var b strings.Builder
			p.Links(&b, "go %page%")
			if b.String() != tc.want {
				t.Fatalf("Links =\n%s\nwant\n%s", b.String(), tc.want)
			}
		})
	}
}

// pad is String.format("%02d") on a decimal string.
func pad(n string) string {
	if len(n) < 2 {
		return "0" + n
	}
	return n
}
