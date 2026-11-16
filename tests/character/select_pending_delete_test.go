package character

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// A character pending deletion cannot be selected: RequestGameStart is
// refused silently with the connection left open, as for a banned
// character (owner-approved divergence, #4766). Restoring the character
// makes it selectable again.

// expectNextOpcode reads the next frame and requires opcode want. It is how
// these tests prove a silent refusal: the request's reply would otherwise
// arrive before the follow-up's.
func expectNextOpcode(t *testing.T, c *testsupport.ScriptedClient, want byte, name string) {
	t.Helper()
	if got := c.Read()[0]; got != want {
		t.Fatalf("next opcode = %#x, want %s (%#x)", got, name, want)
	}
}

func deleteAtOf(t *testing.T, srv *gameservertest.Server, objID int32) int64 {
	t.Helper()
	ch, err := srv.Chars.Get(context.Background(), objID)
	if err != nil {
		t.Fatalf("load character %d: %v", objID, err)
	}
	return ch.DeleteAt
}

func TestSelectPendingDeleteIsRefused(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithReuseDelays(0, 0))
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharCreateOk, "CharCreateOk")
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	objID := srv.SoleObjectID(t)

	c.Send(encodeRequestCharacterDelete(0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharDeleteOk, "CharDeleteOk")
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	deleteAt := deleteAtOf(t, srv, objID)
	if deleteAt == 0 {
		t.Fatal("character was not scheduled for deletion")
	}

	// The crafted selection gets no reply; the restore sent after it is
	// answered first, so the selection neither entered the world nor
	// closed the connection.
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeCharacterRestore(0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo (restore refresh)")
	if got := deleteAtOf(t, srv, objID); got != 0 {
		t.Fatalf("DeleteAt after restore = %d, want 0", got)
	}

	// A restored character selects as before.
	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
}

// The refusal does not modify the row's deletion time.
func TestSelectPendingDeleteKeepsDeleteAt(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithReuseDelays(0, 0))
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharCreateOk, "CharCreateOk")
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	objID := srv.SoleObjectID(t)

	c.Send(encodeRequestCharacterDelete(0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharDeleteOk, "CharDeleteOk")
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	deleteAt := deleteAtOf(t, srv, objID)

	// A second create is answered before anything the refused selection
	// could have sent; the row's deletion time is unchanged.
	c.Send(encodeRequestGameStart(0))
	c.Send(encodeRequestCharacterCreate("Second", 0, 0, 0, 1, 0, 0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharCreateOk, "CharCreateOk")
	if got := deleteAtOf(t, srv, objID); got != deleteAt {
		t.Fatalf("DeleteAt after refused selection = %d, want %d", got, deleteAt)
	}
}

// A deletion stored after the character list was sent still refuses the
// selection: the row read at selection time is what counts.
func TestSelectRefusedWhenDeletionScheduledAfterList(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithReuseDelays(0, 0))
	c := srv.Client

	c.Send(encodeRequestCharacterCreate("Newbie", 0, 0, 0, 1, 0, 0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharCreateOk, "CharCreateOk")
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	objID := srv.SoleObjectID(t)

	deleteAt := time.Now().Add(24 * time.Hour).UnixMilli()
	if err := srv.Chars.SetDeleteAt(context.Background(), objID, deleteAt); err != nil {
		t.Fatalf("schedule deletion: %v", err)
	}

	c.Send(encodeRequestGameStart(0))
	c.Send(encodeCharacterRestore(0))
	expectNextOpcode(t, c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo (restore refresh)")

	c.Send(encodeRequestGameStart(0))
	readUntil(t, c, serverpackets.OpcodeCharSelected)
}
