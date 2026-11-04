package serverpackets

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/npcstring"
)

// TestFrameNpcSayResolvesNpcStringId pins issue #2028: a walkerRoutes.xml
// node's fstring resolves through the ported npcstring table (aCis
// NpcStringId.getMessage()) before broadcasting via NpcSay. The NpcSay
// bytes themselves are pinned by the NpcSay rows of testdata/packets.golden.
func TestFrameNpcSayResolvesNpcStringId(t *testing.T) {
	text, ok := npcstring.Text(3)
	if !ok || text != "Opening" {
		t.Fatalf("npcstring.Text(3) = (%q, %v), want (\"Opening\", true)", text, ok)
	}
}

func TestFramePetItemListRejectsOversizedCount(t *testing.T) {
	items := make([]*item.Instance, 1<<16)
	if _, err := FramePetItemList(items, item.NewTable(nil)); err == nil {
		t.Fatal("FramePetItemList oversized count error = nil, want error")
	}
}

// ---- from ride_test.go ----
