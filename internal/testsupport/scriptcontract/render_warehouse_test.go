package scriptcontract

import (
	"encoding/hex"
	"testing"
)

// A warehouse withdrawal list renders the kind, the player's adena and each
// item's template and count, leaving out the world's object ids.
func TestPacketRendersWarehouseWithdrawList(t *testing.T) {
	// The reference trace's list of a private warehouse holding one Ketra Orc item.
	payload, err := hex.DecodeString("420100000000000100040085040010121c0000010000000300000000000000000000000000850400100000000000000000")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Packet(payload, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "S WarehouseWithdrawList kind=1 adena=0 items=7186x1"; got != want {
		t.Errorf("got %s, want %s", got, want)
	}
	if _, err := Packet(payload[:20], nil); err == nil {
		t.Error("Packet rendered a cut payload")
	}
	if _, err := Packet(append(append([]byte(nil), payload...), 0), nil); err == nil {
		t.Error("Packet rendered a payload with a trailing byte")
	}
}
