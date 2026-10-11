package scriptcontract

import (
	"encoding/hex"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// npcSayRef is an NpcSay of the reference server, as the Q316 oracle trace
// records it: a Varool Foulclaw (27020) of object 0x100004a4 says its line
// to everyone around.
const npcSayRef = "02a404001000000000ccab0f00570068007900200064006f00200079006f00750020006f00700070007200650073007300200075007300200073006f003f000000"

// An NpcSay renders with the speaker's role, the chat type, the client's
// NPC name id and the line, and the Go encoder writes the reference's
// bytes.
func TestPacketRendersNpcSay(t *testing.T) {
	roles := map[int32]string{0x100004a4: "varool"}
	ref, err := hex.DecodeString(npcSayRef)
	if err != nil {
		t.Fatal(err)
	}
	const want = `S NpcSay obj=varool type=0 npc=1027020 text="Why do you oppress us so?"`
	got, err := Packet(ref, roles)
	if err != nil {
		t.Fatalf("Packet: %v", err)
	}
	if got != want {
		t.Fatalf("Packet = %s, want %s", got, want)
	}
	if _, err := Packet(append(ref, 0), roles); err == nil {
		t.Error("Packet rendered a payload with a trailing byte")
	}
	if _, err := Packet(ref[:len(ref)-1], roles); err == nil {
		t.Error("Packet rendered a truncated payload")
	}
	encoded := payload(t, serverpackets.FrameNpcSay(0x100004a4, 27020, serverpackets.SayTypeAll, "Why do you oppress us so?"))
	if hex.EncodeToString(encoded) != npcSayRef {
		t.Fatalf("FrameNpcSay = %x, want the reference's %s", encoded, npcSayRef)
	}
}
