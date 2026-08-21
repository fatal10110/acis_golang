package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func TestCreateInvalidNameKeepsConnectionOpen(t *testing.T) {
	srv := gameservertest.Boot(t)
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("bad name!", 0, 0, 0, 1, 0, 0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateFail {
		t.Fatalf("opcode = %#x, want CharCreateFail (%#x)", reply[0], serverpackets.OpcodeCharCreateFail)
	}

	// The connection must still be usable: a valid create now succeeds.
	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	reply = c.Read()
	if reply[0] != serverpackets.OpcodeCharCreateOk {
		t.Fatalf("opcode = %#x, want CharCreateOk (%#x)", reply[0], serverpackets.OpcodeCharCreateOk)
	}
}

func TestDeleteAndRestore(t *testing.T) {
	srv := gameservertest.Boot(t)
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	c.Read() // CharCreateOk
	c.Read() // CharSelectInfo

	objID := srv.SoleObjectID(t)

	c.Send(encodeRequestCharacterDelete(0))
	reply := c.Read()
	if reply[0] != serverpackets.OpcodeCharDeleteOk {
		t.Fatalf("opcode = %#x, want CharDeleteOk (%#x)", reply[0], serverpackets.OpcodeCharDeleteOk)
	}
	c.Read() // CharSelectInfo refresh

	ch, err := srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load deleted character: %v", err)
	}
	if ch.DeleteAt == 0 {
		t.Fatal("expected character to be scheduled for deletion")
	}

	c.Send(encodeCharacterRestore(0))
	c.Read() // CharSelectInfo refresh

	ch, err = srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load restored character: %v", err)
	}
	if ch.DeleteAt != 0 {
		t.Fatal("expected character's scheduled deletion to be cleared")
	}
}
