package serverpackets

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

// ---- from npcsay_test.go ----
func TestFrameNpcSay(t *testing.T) {
	got := framePayload(t, FrameNpcSay(500, 12564, SayTypeAll, "Hello"))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 500)
	want = appendD(want, 0)
	want = appendD(want, 1012564)
	want = append(want, encodeUTF16Z("Hello")...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

// TestFrameNpcSayResolvesNpcStringId pins issue #2028: a walkerRoutes.xml
// node's fstring resolves through the ported npcstring table (aCis
// NpcStringId.getMessage()) before broadcasting via NpcSay.
func TestFrameNpcSayResolvesNpcStringId(t *testing.T) {
	text, ok := npcstring.Text(3)
	if !ok || text != "Opening" {
		t.Fatalf("npcstring.Text(3) = (%q, %v), want (\"Opening\", true)", text, ok)
	}

	got := framePayload(t, FrameNpcSay(12345, 31357, SayTypeAll, text))
	want := []byte{OpcodeNpcSay}
	want = appendD(want, 12345)
	want = appendD(want, 0)
	want = appendD(want, 1_000_000+31357)
	want = append(want, encodeUTF16Z(text)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("FrameNpcSay() = %x, want %x", got, want)
	}
}

func TestFramePetStatusShow(t *testing.T) {
	got := framePayload(t, FramePetStatusShow(2))
	want := []byte{OpcodePetStatusShow, 0x02, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("FramePetStatusShow() = %x, want %x", got, want)
	}
}

func TestFramePetItemListRejectsOversizedCount(t *testing.T) {
	items := make([]*item.Instance, 1<<16)
	if _, err := FramePetItemList(items, item.NewTable(nil)); err == nil {
		t.Fatal("FramePetItemList oversized count error = nil, want error")
	}
}

// ---- from ride_test.go ----
