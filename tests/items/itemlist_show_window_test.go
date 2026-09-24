package items

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// ItemList's first field after the opcode is the showWindow flag, written as
// a little-endian 16-bit value. These are the reference's two literal
// on-the-wire encodings, written out by hand rather than produced by the
// frame builder under test, so a builder that ignored its flag argument
// could not make them agree.
var (
	itemListWindowShown  = []byte{0x01, 0x00}
	itemListWindowHidden = []byte{0x00, 0x00}
)

// TestItemListShowWindowPerCallSite pins the showWindow flag separately for
// each of the two sites that build an ItemList. The login burst seeds item
// state without popping the inventory open, so it sends 0x0000; an on-demand
// RequestItemList is the player asking for the window, so it sends 0x0001.
// A single shared value cannot satisfy both assertions.
//
// The character carries an item, so the flag field is distinguishable from
// the item count that follows it: under a flag/count field-order swap a
// one-item list would read 01 00 at the flag offset and pass the "shown"
// assertion, and the count guard is what catches that.
func TestItemListShowWindowPerCallSite(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	srv.GiveItem(t, srv.SoleObjectID(t), 20, 5)

	burst := startInWorld(t, c)
	burstItemList := burst[enterWorldBurstItemList]
	assertFrameOpcode(t, burstItemList, serverpackets.OpcodeItemList, "EnterWorld burst ItemList")
	assertItemListShowWindow(t, burstItemList, itemListWindowHidden, "EnterWorld burst ItemList")

	c.Send(encodeRequestItemList())
	assertItemListShowWindow(t, readItemList(t, c), itemListWindowShown, "RequestItemList reply")
}

// assertItemListShowWindow requires frame's showWindow field to be the exact
// two bytes want, and requires the list to be non-empty so the assertion
// cannot pass on a packet whose flag and count are both zero.
func assertItemListShowWindow(t *testing.T, frame, want []byte, what string) {
	t.Helper()
	if len(frame) < 5 {
		t.Fatalf("%s is %d bytes, too short for opcode + showWindow + count", what, len(frame))
	}
	if got := frame[1:3]; !bytes.Equal(got, want) {
		t.Errorf("%s showWindow = % x, want % x", what, got, want)
	}
	if count := frame[3:5]; bytes.Equal(count, []byte{0x00, 0x00}) {
		t.Errorf("%s item count = 0; the fixture must carry an item", what)
	}
}
