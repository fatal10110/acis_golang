package items

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
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
// the item count that follows it: a hidden-window list of one item and a
// shown-window empty list would otherwise share the same four bytes.
func TestItemListShowWindowPerCallSite(t *testing.T) {
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	srv.GiveItem(t, srv.SoleObjectID(t), 20, 5)

	c.Send(encodeRequestGameStart(0))
	if reply := c.Read(); reply[0] != serverpackets.OpcodeSSQInfo {
		t.Fatalf("opcode = %#x, want SSQInfo (%#x)", reply[0], serverpackets.OpcodeSSQInfo)
	}
	if reply := c.Read(); reply[0] != serverpackets.OpcodeCharSelected {
		t.Fatalf("opcode = %#x, want CharSelected (%#x)", reply[0], serverpackets.OpcodeCharSelected)
	}

	c.Send(encodeEnterWorld())
	burst := readEnterWorldBurst(t, c)
	assertItemListShowWindow(t, findItemList(t, burst), itemListWindowHidden, "EnterWorld burst ItemList")
	drainUntilQuiet(t, c)

	c.Send(encodeRequestItemList())
	assertItemListShowWindow(t, readItemList(t, c), itemListWindowShown, "RequestItemList reply")
}

// findItemList returns the sole ItemList frame in frames.
func findItemList(t *testing.T, frames [][]byte) []byte {
	t.Helper()
	var found []byte
	for _, frame := range frames {
		if len(frame) > 0 && frame[0] == serverpackets.OpcodeItemList {
			if found != nil {
				t.Fatal("EnterWorld burst carried more than one ItemList")
			}
			found = frame
		}
	}
	if found == nil {
		t.Fatal("EnterWorld burst carried no ItemList")
	}
	return found
}

// readItemList reads until the next ItemList frame and returns it.
func readItemList(t *testing.T, c *testsupport.ScriptedClient) []byte {
	t.Helper()
	for range 100 {
		if frame := c.Read(); len(frame) > 0 && frame[0] == serverpackets.OpcodeItemList {
			return frame
		}
	}
	t.Fatal("no ItemList within 100 frames")
	return nil
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
