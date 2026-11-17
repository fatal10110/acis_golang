package npcs

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSellItemAllRowsSkippedStillSendsSoldPage pins a sale whose every row is
// skipped (an object the player does not hold, and more units than held): the
// merchant still answers its sold page, nothing is sold, no adena is paid,
// and no inventory change is announced.
func TestSellItemAllRowsSkippedStillSendsSoldPage(t *testing.T) {
	t.Parallel()
	w := bootSeller(t, [2]int32{potionID, 10})
	f := w.spawnFolk(t, folkTemplate("Merchant", merchantID), 50)

	w.selectFolk(t, f)
	w.c.Send(encodeRequestSellItem(0,
		sellRow{w.items[potionID] + 100000, potionID, 1},
		sellRow{w.items[potionID], potionID, 11},
	))
	frames := drainFrames(t, w.c)
	page, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage)
	if !ok {
		t.Fatalf("sale of skipped rows answered %x, want the sold page", opcodes(frames))
	}
	if objectID, html, _ := htmlMessage(t, page); objectID != f.ObjectID() || html != wantChatPage(soldPage, f) {
		t.Fatalf("sold page = %d %q, want %d %q", objectID, html, f.ObjectID(), wantChatPage(soldPage, f))
	}
	if containsOpcode(frames, serverpackets.OpcodeInventoryUpdate) || containsOpcode(frames, serverpackets.OpcodeItemList) {
		t.Fatalf("sale of skipped rows answered %x, want no inventory change", opcodes(frames))
	}
	if potions, adena := w.held(t, w.items[potionID]), w.adena(t); potions != 10 || adena != startAdena {
		t.Fatalf("after skipped rows: potions %d adena %d, want 10 and %d", potions, adena, startAdena)
	}
}
