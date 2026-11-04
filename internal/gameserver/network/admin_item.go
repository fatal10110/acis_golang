package network

import (
	"fmt"
	"strings"

	handleradmin "github.com/fatal10110/acis_golang/internal/gameserver/handler/admin"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// adminItem answers //item: the item creation page; //item <id> [count]
// [radius] creates an item, then reopens the page; //item coin <name>
// [count] [radius] creates adena, ancient adena or festival adena by name;
// //item set [chestId] gives an armor set, then lists the sets.
//
// The items go to the selected player, else to gm, or with a radius to
// every player around gm instead.
func (l *GameClientLink) adminItem(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 {
		l.sendAdminFile(gm, "itemcreation.htm")
		return
	}
	target := adminTargetPlayer(gm, true)
	page := func() { l.sendAdminFile(gm, "itemcreation.htm") }
	switch param := args[0]; {
	case isDigits(param):
		// An id, count or radius out of the int range or not a number
		// answers nothing, as specified; the link leaves no client action
		// pending.
		id, count, radius, ok := parseItemArgs(param, args[1:])
		if !ok {
			return
		}
		l.createItem(gm, target, id, count, radius, page)
	case param == "coin":
		l.adminItemCoin(gm, target, args[1:], page)
	case param == "set":
		l.adminItemSet(gm, target, args[1:])
	default:
		// Any other parameter answers nothing, as specified; the link
		// leaves no client action pending.
	}
}

// adminGive answers //give <id> [count]: count units, one by default, of
// the item go to the selected player, else to gm.
func (l *GameClientLink) adminGive(gm *livePlayer, line string) {
	args := handleradmin.Args(line)
	if len(args) == 0 || !isDigits(args[0]) {
		sendText(gm, "Usage: //give itemId count")
		return
	}
	// A count or id out of the int range or not a number answers
	// nothing, as specified; the link leaves no client action pending.
	// Only the count follows the id.
	rest := args[1:]
	if len(rest) > 1 {
		rest = rest[:1]
	}
	id, count, _, ok := parseItemArgs(args[0], rest)
	if !ok {
		return
	}
	l.createItem(gm, adminTargetPlayer(gm, true), id, count, 0, nil)
}

// parseItemArgs reads an item id, then the optional count and radius that
// follow it as parseCountRadius reads them.
func parseItemArgs(idArg string, rest []string) (id int32, count, radius int, ok bool) {
	id, ok = parseInt32(idArg)
	if !ok {
		return 0, 0, 0, false
	}
	count, radius, ok = parseCountRadius(rest)
	return id, count, radius, ok
}

// parseCountRadius reads the optional count, one by default, and radius,
// none by default, of args; any further argument is ignored.
func parseCountRadius(args []string) (count, radius int, ok bool) {
	count = 1
	for i, dst := range []*int{&count, &radius} {
		if i >= len(args) {
			break
		}
		v, ok := parseInt32(args[i])
		if !ok {
			return 0, 0, false
		}
		*dst = int(v)
	}
	return count, radius, true
}

// coinIDs are the coins //item coin creates by name, any letter case.
var coinIDs = map[string]int32{"adena": 57, "ancient": 5575, "festival": 6673}

// adminItemCoin creates the coin args name: args are the coin's name,
// then the optional count and radius. A missing or unknown name or a bad
// count or radius answers the usage; every case but an unknown name then
// reopens the creation page.
func (l *GameClientLink) adminItemCoin(gm, target *livePlayer, args []string, page func()) {
	const usage = "Usage: //item coin name [amount] [radius]"
	if len(args) == 0 {
		sendText(gm, usage)
		page()
		return
	}
	id, known := coinIDs[strings.ToLower(args[0])]
	if !known {
		sendText(gm, usage)
		return
	}
	count, radius, ok := parseCountRadius(args[1:])
	if !ok {
		sendText(gm, usage)
		page()
		return
	}
	l.createItem(gm, target, id, count, radius, page)
}

// adminItemSet gives target one of each piece and the shield of the armor
// set whose chest is args' first, telling gm when target is someone else,
// then lists every set. A chest no set has answers so and lists nothing; a
// chest that is not a number answers the usage, then lists the sets.
func (l *GameClientLink) adminItemSet(gm, target *livePlayer, args []string) {
	list := func() { l.sendArmorSets(gm) }
	if len(args) == 0 {
		list()
		return
	}
	chest, ok := parseInt32(args[0])
	if !ok {
		sendText(gm, "Usage: //item set [chestId]")
		list()
		return
	}
	set, ok := l.armorSets.FindByChest(chest)
	if !ok {
		sendText(gm, "This chest has no set.")
		return
	}
	if !onPlayer(gm, target, func() {
		pieces := set.PieceIDs()
		for _, id := range append(pieces[:], set.Shield) {
			if id > 0 {
				target.AddCraftedItem(id, 1, l.nextObjectID)
			}
		}
		onPlayer(target, gm, func() {
			if target != gm {
				sendText(gm, "You have spawned "+set.Name+" in "+target.CharacterName()+"'s inventory.")
			}
			list()
		})
	}) {
		list()
	}
}

// sendArmorSets opens the armor set page listing every set as a link that
// gives it: two to a row, the second of each pair after its row's end, in
// the armor set table's order.
func (l *GameClientLink) sendArmorSets(gm *livePlayer) {
	var sb strings.Builder
	for i, set := range l.armorSets.InTableOrder() {
		newLine := i%2 == 0
		if newLine {
			sb.WriteString("<tr>")
		}
		fmt.Fprintf(&sb, `<td><a action="bypass -h admin_item set %d">%s</a></td>`, set.Chest, set.Name)
		if newLine {
			sb.WriteString("</tr>")
		}
	}
	const file = "data/html/admin/itemsets.htm"
	page := strings.ReplaceAll(l.setPage(file), "%sets%", sb.String())
	l.sendFilePage(gm, 0, file, page, 0)
}

// createItem gives count units of item id to target, or with a radius to
// every player within it of gm but gm, each told it picked them up, then
// runs then, when set, for gm. An unknown item, or one target's inventory
// has no room for, is refused with a message to gm; gm is told what went
// to another player or around it.
func (l *GameClientLink) createItem(gm, target *livePlayer, id int32, count, radius int, then func()) {
	done := func() {
		if then != nil {
			then()
		}
	}
	tmpl, ok := l.itemTemplates.Get(id)
	if !ok {
		sendText(gm, "This item doesn't exist.")
		done()
		return
	}
	if !onPlayer(gm, target, func() {
		fits := target.RewardItemFits(id, count)
		if fits && radius <= 0 {
			l.giveCreatedItem(target, id, count)
		}
		onPlayer(target, gm, func() {
			switch {
			case !fits:
				sendText(gm, "Your target's inventory is full.")
			case radius > 0:
				l.giveAround(gm, id, count, radius)
				sendText(gm, fmt.Sprintf("Surrounding players were rewarded with %d %s in a %d radius.", count, tmpl.Name, radius))
			case target != gm:
				sendText(gm, fmt.Sprintf("You have spawned %d %s (%d) in %s's inventory.", count, tmpl.Name, id, target.CharacterName()))
			}
			done()
		})
	}) {
		done()
	}
}

// giveAround gives count units of item id to every player within radius
// of gm but gm, each on its own queue.
func (l *GameClientLink) giveAround(gm *livePlayer, id int32, count, radius int) {
	if l.world == nil {
		return
	}
	l.world.ForEachKnownInRadius(gm, radius, func(o world.Tracked) {
		if p, ok := o.(*livePlayer); ok {
			postLive(p, func() { l.giveCreatedItem(p, id, count) })
		}
	})
}

// giveCreatedItem gives live, on its own queue, count units of item id
// named in chat as picked up. A cursed weapon makes live its holder
// through the obtained-item event the addition emits.
func (l *GameClientLink) giveCreatedItem(live *livePlayer, id int32, count int) {
	live.AddCreatedItem(id, count, l.nextObjectID)
}
