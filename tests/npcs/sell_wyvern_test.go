package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Wyvern managers: the first has a merchant/<id>-sold.htm page in
// sellPages, the second none.
const (
	wyvernPagedID   = 35420
	wyvernUnpagedID = 35421
)

// TestSellItemToWyvernManager pins a wyvern manager as a merchant-type
// buyer: a potion sold to one pays half its reference price (20), and the
// sold page comes from merchant/ when the manager's id has one, else no
// page is sent.
func TestSellItemToWyvernManager(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{potionID, 10})
	paged := w.spawnFolk(t, folkTemplate("WyvernManagerNpc", wyvernPagedID), 50)
	unpaged := w.spawnFolk(t, folkTemplate("WyvernManagerNpc", wyvernUnpagedID), 50)
	potion := sellRow{w.items[potionID], potionID, 1}

	w.selectFolk(t, paged)
	w.c.Send(encodeRequestSellItem(0, potion))
	frames := drainFrames(t, w.c)
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("sale to the wyvern manager answered %x, want its sold page", opcodes(frames))
	}
	if objectID, html, _ := htmlMessage(t, page); objectID != paged.ObjectID() || html != wantChatPage(soldPage, paged) {
		t.Fatalf("wyvern manager sold page = %d %q, want %d %q", objectID, html, paged.ObjectID(), wantChatPage(soldPage, paged))
	}
	if potions, adena := w.held(t, w.items[potionID]), w.adena(t); potions != 9 || adena != startAdena+20 {
		t.Fatalf("after selling the wyvern manager a potion: potions %d adena %d, want 9 and %d", potions, adena, startAdena+20)
	}

	w.selectFolk(t, unpaged)
	w.c.Send(encodeRequestSellItem(0, potion))
	if frames := drainFrames(t, w.c); containsOpcode(frames, serverpackets.OpcodeNpcHtmlMessage) {
		t.Fatalf("sale to the pageless wyvern manager answered %x, want no page", opcodes(frames))
	}
	if potions, adena := w.held(t, w.items[potionID]), w.adena(t); potions != 8 || adena != startAdena+40 {
		t.Fatalf("after selling the pageless wyvern manager a potion: potions %d adena %d, want 8 and %d", potions, adena, startAdena+40)
	}
}
