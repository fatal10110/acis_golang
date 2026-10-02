package admin

import (
	"fmt"
	"strconv"
	"strings"
)

// Page is one page of an admin panel list.
type Page struct {
	// Number is the page shown, from 1.
	Number int
	// Total is how many pages the list fills.
	Total int
	// Limit is how many entries a page holds.
	Limit int
	// Start and End bound the entries the page shows: [Start, End).
	Start, End int
}

// Paginate returns page number of a list of n entries, limit to a page. A
// number past the last page shows the last one. ok is false for a number
// below 1 on a list with entries, which shows no page.
func Paginate(n, number, limit int) (p Page, ok bool) {
	p.Limit = max(limit, 1)
	p.Total = (n + p.Limit - 1) / p.Limit
	p.Number = min(max(number, 1), max(p.Total, 1))
	if n == 0 {
		return p, true
	}
	shown := min(number, p.Total)
	if shown < 1 {
		return Page{}, false
	}
	p.Start, p.End = (shown-1)*p.Limit, min(shown*p.Limit, n)
	return p, true
}

// Space pads a page short of entries with one empty row of height pixels
// per missing entry.
func (p Page) Space(b *strings.Builder, height int) {
	for range p.Limit - (p.End - p.Start) {
		b.WriteString("<img height=" + strconv.Itoa(height) + ">")
	}
}

// Links writes the page bar: first and last page buttons around links to
// up to four pages either side of the shown one. action is each link's
// bypass, %page% standing for the page number.
func (p Page) Links(b *strings.Builder, action string) {
	link := func(page int) string { return strings.ReplaceAll(action, "%page%", strconv.Itoa(page)) }
	b.WriteString(`<table width=280 bgcolor=000000><tr><td FIXWIDTH=22 align=center><img height=2><button action="` + link(1) + `" back=L2UI_CH3.prev1_down fore=L2UI_CH3.prev1 width=16 height=16></td>`)
	for i := p.Number - 5; i < p.Number-1; i++ {
		b.WriteString("<td FIXWIDTH=26 align=center>")
		if i >= 0 {
			fmt.Fprintf(b, `<a action="%s">%02d</a>`, link(i+1), i+1)
		}
		b.WriteString("</td>")
	}
	fmt.Fprintf(b, "<td FIXWIDTH=26 align=center><font color=LEVEL>%02d</font></td>", max(p.Number, 1))
	for i := p.Number; i < p.Number+4; i++ {
		b.WriteString("<td FIXWIDTH=26 align=center>")
		if i < p.Total {
			fmt.Fprintf(b, `<a action="%s">%02d</a>`, link(i+1), i+1)
		}
		b.WriteString("</td>")
	}
	b.WriteString(`<td FIXWIDTH=22 align=center><img height=2><button action="` + link(p.Total) + `" back=L2UI_CH3.next1_down fore=L2UI_CH3.next1 width=16 height=16></td></tr></table>`)
	b.WriteString(`<img src="L2UI.SquareGray" width=280 height=1>`)
}
