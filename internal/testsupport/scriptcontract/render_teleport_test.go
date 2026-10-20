package scriptcontract

import (
	"encoding/hex"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// teleportRef is a TeleportToLocation of the reference server, as the
// castle_defend_teleporter_defender scenario trace records it: the player
// of object 0x10000484 lands at -18615,108298,-2556.
const teleportRef = "288404001049b7ffff0aa7010004f6ffff00000000"

// A TeleportToLocation renders with the moved object's role, where it lands
// and its fast flag, and the Go encoder writes the reference's bytes.
func TestPacketRendersTeleportToLocation(t *testing.T) {
	roles := map[int32]string{0x10000484: "player"}
	ref, err := hex.DecodeString(teleportRef)
	if err != nil {
		t.Fatal(err)
	}
	const want = "S TeleportToLocation obj=player loc=-18615,108298,-2556 fast=0"
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
	encoded := payload(t, serverpackets.FrameTeleportToLocation(0x10000484, location.Location{X: -18615, Y: 108298, Z: -2556}, false))
	if hex.EncodeToString(encoded) != teleportRef {
		t.Fatalf("FrameTeleportToLocation = %x, want the reference's %s", encoded, teleportRef)
	}
	fast, err := Packet(payload(t, serverpackets.FrameTeleportToLocation(7, location.Location{X: 1, Y: 2, Z: 3}, true)), nil)
	if err != nil || fast != "S TeleportToLocation obj=7 loc=1,2,3 fast=1" {
		t.Fatalf("fast teleport = %q, %v", fast, err)
	}
}
