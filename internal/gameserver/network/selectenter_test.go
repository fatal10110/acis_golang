package network

import (
	"encoding/binary"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/link"
)

// TestSelectCharacterAdvancesToEntering exercises the packet sequence
// between a client picking a character slot and it being allowed to send
// EnterWorld: decode RequestGameStart, move the client from StateAuthed to
// StateEntering, and confirm the SSQInfo/CharSelected reply packets encode
// without needing anything the client hasn't been given yet (its own
// confirmed session key).
func TestSelectCharacterAdvancesToEntering(t *testing.T) {
	client := NewClient(nil)
	client.SetAuthenticated("player1", link.SessionKey{PlayKey1: 999, PlayKey2: 888})

	var payload []byte
	payload = append(payload, clientpackets.OpcodeRequestGameStart)
	payload = binary.LittleEndian.AppendUint32(payload, 0) // slot
	payload = binary.LittleEndian.AppendUint16(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored
	payload = binary.LittleEndian.AppendUint32(payload, 0) // ignored

	if !client.Accept(clientpackets.OpcodeRequestGameStart) {
		t.Fatal("Accept(RequestGameStart) = false in StateAuthed, want true")
	}
	req, err := clientpackets.DecodeRequestGameStart(payload)
	if err != nil {
		t.Fatalf("DecodeRequestGameStart: %v", err)
	}
	if req.Slot != 0 {
		t.Fatalf("req.Slot = %d, want 0", req.Slot)
	}

	// Resolving the slot to a persisted character is the character store's
	// job, not this sequence's; a fixed character stands in for it here.
	c := &player.Character{ObjectID: 1, Name: "Newbie", Position: location.Location{X: 1, Y: 2, Z: 3}}
	tmpl := &player.Template{}

	if len(serverpackets.EncodeSSQInfo()) == 0 {
		t.Fatal("EncodeSSQInfo: want non-empty packet")
	}

	client.SetState(StateEntering)

	charSelected := serverpackets.EncodeCharSelected(serverpackets.CharSelectedSnapshot{
		Character: c, Template: tmpl, SessionID: client.SessionKey().PlayKey1,
	})
	if len(charSelected) == 0 {
		t.Fatal("EncodeCharSelected: want non-empty packet")
	}

	if !client.Accept(clientpackets.OpcodeEnterWorld) {
		t.Error("Accept(EnterWorld) = false in StateEntering, want true")
	}
	if client.Accept(clientpackets.OpcodeRequestCharacterCreate) {
		t.Error("Accept(RequestCharacterCreate) = true in StateEntering, want false")
	}
}
