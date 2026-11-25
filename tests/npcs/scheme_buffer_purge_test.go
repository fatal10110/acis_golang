package npcs

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestSchemeWriteQueuedBeforeAPurgeStoresNothing creates a scheme while the
// player's persistence lane is held, so its write stays queued, then
// restarts, the wait for that lane giving up, onto a character list that
// purges the character, its grace period over. Once the lane drains, the
// queued write has not stored the purged character's scheme again: the
// purge's own empty write runs behind it (#4699).
func TestSchemeWriteQueuedBeforeAPurgeStoresNothing(t *testing.T) {
	t.Parallel()
	w, f := schemeBufferWorld(t, nil, 0, storedSchemeBuffer(t),
		gameservertest.WithPersistWait(200*time.Millisecond),
		// The wait budget is a wall-clock deadline on the connection
		// goroutine, which a driven clock would not reach.
		gameservertest.WithRealPool(),
	)
	release := w.srv.HoldPersistenceLane(t, w.player)
	w.talkTo(t, f)
	for _, command := range []string{"support", "createscheme Fight"} {
		w.c.Send(encodeBypass(npcCommand(f, command)))
		readUntilOpcode(t, w.c, serverpackets.OpcodeNpcHtmlMessage, command+" page")
	}
	assertStoredSchemes(t, w, nil)

	// The deletion's grace period is already over, so the list the restart
	// reads purges the character.
	if _, err := w.srv.DB.ExecContext(context.Background(), "UPDATE characters SET deletetime = 1 WHERE obj_Id = ?", w.player); err != nil {
		t.Fatal(err)
	}
	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestRestart).Bytes())
	readUntilOpcode(t, w.c, serverpackets.OpcodeCharSelectInfo, "CharSelectInfo")
	if _, err := w.srv.Chars.Get(context.Background(), w.player); !errors.Is(err, gamesql.ErrCharacterNotFound) {
		t.Fatalf("character after the restart: err %v, want it purged", err)
	}

	release()
	w.srv.FlushPersistence(t)
	assertStoredSchemes(t, w, nil)
}
