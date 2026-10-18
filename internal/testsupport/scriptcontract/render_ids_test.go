package scriptcontract

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// A page names an object by its id: the id renders as its role, but only as
// a whole number. A longer number holding the id's digits, as the system
// message id 1039 of a warehouse link holds the fixture id 103, stays whole.
func TestPacketRendersRoleIDsAsWholeNumbers(t *testing.T) {
	roles := map[int32]string{103: "hestui", 104: "grookin"}
	html := `<a action="bypass -h npc_104_DepositC" msg="1039">Deposit</a><a action="bypass -h npc_104_Quest">103 10310 0103</a>`
	got, err := Packet(payload(t, serverpackets.FrameNpcHtmlMessage(104, html, 0)), roles)
	if err != nil {
		t.Fatal(err)
	}
	want := `S NpcHtmlMessage obj=grookin item=0 html="<a action=\"bypass -h npc_{grookin}_DepositC\" msg=\"1039\">Deposit</a><a action=\"bypass -h npc_{grookin}_Quest\">{hestui} 10310 0103</a>"`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}
