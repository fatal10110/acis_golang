package admin

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// atmosphereUsage is the pair of lines every malformed //atmosphere answers
// (AdminEffects.java admin_atmosphere).
var atmosphereUsage = []string{
	"Usage: //atmosphere <ssqinfo dawn|dusk|red|regular>",
	"Usage: //atmosphere <sky day|night|red>",
}

// TestAdminAtmosphere pins //atmosphere (AdminEffects.java
// admin_atmosphere): each sky or seven-signs state reaches every online
// player, the game master included, as one packet; a bad type, state or
// missing argument answers the two usage lines to the game master only; a
// player without the command's access level is refused and nobody sees a
// sky.
func TestAdminAtmosphere(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	first, _ := addPlayer(t, srv, "player2", "First", userLevel)
	second, _ := addPlayer(t, srv, "player3", "Second", userLevel)
	drain(t, gm)
	drain(t, first)
	drain(t, second)
	clients := []*testsupport.ScriptedClient{gm, first, second}

	cases := []struct {
		command string
		want    []byte
	}{
		{"atmosphere sky night", []byte{serverpackets.OpcodeSunSet}},
		{"atmosphere sky day", []byte{serverpackets.OpcodeSunRise}},
		{"atmosphere sky red", []byte{serverpackets.OpcodeExtended, 0x40, 0x00, 0x0a, 0x00, 0x00, 0x00}},
		{"atmosphere ssqinfo regular", []byte{serverpackets.OpcodeSSQInfo, 0x00, 0x01}},
		{"atmosphere ssqinfo dusk", []byte{serverpackets.OpcodeSSQInfo, 0x01, 0x01}},
		{"atmosphere ssqinfo dawn", []byte{serverpackets.OpcodeSSQInfo, 0x02, 0x01}},
		{"atmosphere ssqinfo red", []byte{serverpackets.OpcodeSSQInfo, 0x03, 0x01}},
		// Arguments past the state are ignored.
		{"atmosphere sky night extra", []byte{serverpackets.OpcodeSunSet}},
	}
	for _, tc := range cases {
		gm.Send(encodeBuildCmd(tc.command))
		received := make([][]byte, len(clients))
		for i, c := range clients {
			received[i] = c.Read()
			if !bytes.Equal(received[i], tc.want) {
				t.Fatalf("//%s client %d frame = % x, want % x", tc.command, i, received[i], tc.want)
			}
		}
		// Each recipient got its own copy: corrupting one leaves the others
		// intact.
		received[0][0] ^= 0xff
		for i := 1; i < len(received); i++ {
			if !bytes.Equal(received[i], tc.want) {
				t.Fatalf("//%s client %d frame changed with client 0's: % x", tc.command, i, received[i])
			}
		}
	}

	// The effects panel buttons reach the same command as a clicked link.
	gm.Send(encodeBypass("admin_atmosphere sky day"))
	for i, c := range clients {
		if got := c.Read(); !bytes.Equal(got, []byte{serverpackets.OpcodeSunRise}) {
			t.Fatalf("admin_atmosphere bypass client %d frame = % x, want SunRise", i, got)
		}
	}

	for _, bad := range []string{
		"atmosphere",
		"atmosphere sky",
		"atmosphere sky dusk",
		"atmosphere ssqinfo night",
		"atmosphere weather day",
		"atmosphere SKY day",
	} {
		assertTexts(t, exchange(t, gm, encodeBuildCmd(bad)), atmosphereUsage...)
	}
	for i, c := range clients[1:] {
		if frames := barrier(t, c); len(frames) != 0 {
			t.Fatalf("bystander %d frames after bad //atmosphere = %x, want none", i+1, testsupport.FrameOpcodes(frames))
		}
	}

	// A player without the access level is refused; nobody's sky changes.
	assertTexts(t, exchange(t, first, encodeBuildCmd("atmosphere sky night")), "You don't have the access right to use this command.")
	for _, c := range []*testsupport.ScriptedClient{gm, second} {
		if frames := barrier(t, c); len(frames) != 0 {
			t.Fatalf("frames after a refused //atmosphere = %x, want none", testsupport.FrameOpcodes(frames))
		}
	}
}

// barrier returns the frames c receives before the reply to a manor-list
// barrier sent now.
func barrier(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	return testsupport.SyncBarrierFrames(t, c, func() { c.Send(encodeManorBarrier()) }, serverpackets.OpcodeExtended)
}
