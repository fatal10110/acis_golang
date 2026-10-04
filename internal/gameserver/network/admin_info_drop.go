package network

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/fatal10110/acis_golang/internal/commons"
	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

// Drop page limits: one category to a page, six drops to a category page.
const (
	npcDropCategoriesPerPage = 1
	npcDropsPerPage          = 6
)

// adminNpcDrops answers //info drop|spoil [page [subpage]]: page of inst's
// drop (or spoil) categories, each showing subpage of its drops. A page or
// subpage that does not read as an int, or one below 1 where there is
// something to show, opens the first drop page instead.
func (l *GameClientLink) adminNpcDrops(gm *livePlayer, inst *npc.Instance, isDrop bool, args []string) {
	page, subPage := 1, 1
	ok := true
	for i, dst := range []*int{&page, &subPage} {
		if i >= len(args) {
			break
		}
		n, err := commons.ParseInt(args[i], 32)
		if err != nil {
			ok = false
			break
		}
		*dst = int(n)
	}
	var rates item.Rates
	if npcs := l.npcSpawns.Load(); npcs != nil {
		rates = npcs.DropRates()
	}
	var content string
	if ok {
		content, ok = l.npcDropContent(inst, rates, page, subPage, isDrop)
	}
	if !ok {
		if content, ok = l.npcDropContent(inst, rates, 1, 1, true); !ok {
			l.log.Warn().Int("npc_id", inst.Template.ID).Msg("admin: //info drop names an unknown item")
			return
		}
	}
	sendFilledHTML(gm, 0, l.npcDefaultPage(content), 0)
}

// npcDropContent lists page of inst's drop categories (its spoil ones when
// isDrop is false), each with subPage of its drops, most likely first, and
// the rate rates gives its kind (the raid item rate for a raid or grand
// boss). ok is false where the reference list throws: a page or subpage
// below 1 with entries to show, or a drop of an item no template defines.
func (l *GameClientLink) npcDropContent(inst *npc.Instance, rates item.Rates, page, subPage int, isDrop bool) (content string, ok bool) {
	var categories []item.DropCategory
	for _, c := range inst.Template.Drops {
		if (c.Kind != item.DropSpoil) == isDrop {
			categories = append(categories, c)
		}
	}
	pg, ok := handleradmin.Paginate(len(categories), page, npcDropCategoriesPerPage)
	if !ok {
		return "", false
	}
	if len(categories) == 0 {
		if isDrop {
			return "This NPC doesn't hold any drops.", true
		}
		return "This NPC doesn't hold any spoils.", true
	}
	sub := "spoil"
	if isDrop {
		sub = "drop"
	}
	raid := slices.Contains([]string{"RaidBoss", "GrandBoss"}, npcClassName(inst))

	var b strings.Builder
	row := 0
	for _, c := range categories[pg.Start:pg.End] {
		b.WriteString("<br></center>Category: " + c.Kind.String() + " - Rate: " + formatPercent(c.Chance) +
			"% - Iterations: x" + formatPercent(rates.Resolve(c.Kind, raid)) + "<center>")
		drops := slices.Clone(c.Drops)
		slices.SortStableFunc(drops, func(a, b item.Drop) int { return cmp.Compare(b.Chance, a.Chance) })
		dpg, ok := handleradmin.Paginate(len(drops), subPage, npcDropsPerPage)
		if !ok {
			return "", false
		}
		for _, d := range drops[dpg.Start:dpg.End] {
			row, ok = l.writeDropRow(&b, d, row)
			if !ok {
				return "", false
			}
		}
		dpg.Space(&b, 41)
		dpg.Links(&b, "bypass admin_info "+sub+" "+strconv.Itoa(page)+" %page%")
	}
	pg.Space(&b, 30)
	pg.Links(&b, "bypass admin_info "+sub+" %page% 1")
	return b.String(), true
}

// writeDropRow writes drop d as the row-th row of a drop list and returns
// the next row number. ok is false when no item template defines d's item.
func (l *GameClientLink) writeDropRow(b *strings.Builder, d item.Drop, row int) (next int, ok bool) {
	if l.itemTemplates == nil {
		return row, false
	}
	tmpl, ok := l.itemTemplates.Get(d.ItemID)
	if !ok {
		return row, false
	}
	name := tmpl.Name
	if rest, found := strings.CutPrefix(name, "Recipe: "); found {
		name = "R: " + rest
	}
	name = trimAndDress(name, 45)
	color := "F08080"
	switch {
	case d.Chance > 80:
		color = "90EE90"
	case d.Chance > 5:
		color = "BDB76B"
	}
	amount := strconv.Itoa(int(d.Min))
	if d.Min != d.Max {
		amount += " - " + strconv.Itoa(int(d.Max))
	}
	if row%2 == 0 {
		b.WriteString("<table width=280 bgcolor=000000><tr>")
	} else {
		b.WriteString("<table width=280><tr>")
	}
	b.WriteString("<td width=34 height=40><img src=icon.noimage width=32 height=32></td>")
	b.WriteString("<td width=246>&nbsp;" + name + "<br1>")
	b.WriteString("<table width=240><tr><td width=80><font color=B09878>Rate:</font> <font color=" + color + ">" + formatPercent(d.Chance) +
		"%</font></td><td width=160><font color=B09878>Amount: </font>" + amount + "</td></tr></table>")
	b.WriteString("</td></tr></table><img src=L2UI.SquareGray width=280 height=1>")
	return row + 1, true
}
