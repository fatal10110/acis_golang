package scenario

import (
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A run's first objects (the character, then the NPCs) take ids above
// objectIDsAfter, so a page number such as the pet managers' multisell 101,
// once the character's own id, stays a number in the rendered page while
// the NPC's own id in its bypass is named.
func TestPageNumbersAreNotRunObjectIDs(t *testing.T) {
	player, npc := int32(objectIDsAfter+1), int32(objectIDsAfter+2)
	roles := map[int32]string{player: "player", npc: "waters"}
	f := serverpackets.FrameNpcHtmlMessage(npc, fmt.Sprintf(`<a action="bypass -h npc_%d_multisell 101">Exchange</a> 102 103`, npc), 0)
	defer f.Release()
	if err := f.Err(); err != nil {
		t.Fatal(err)
	}
	got, ok, err := render(f.Bytes()[2:], roles)
	if err != nil || !ok {
		t.Fatalf("render: %v %v", ok, err)
	}
	want := `NpcHtmlMessage obj=waters item=0 html="<a action=\"bypass -h npc_{waters}_multisell 101\">Exchange</a> 102 103"`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
