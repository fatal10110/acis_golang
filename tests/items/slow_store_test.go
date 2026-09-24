package items

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// slowStoreDelay is longer than the sim pool's 50 ms slow-task budget, so a
// write still made on an actor queue would be logged by the watchdog.
const slowStoreDelay = 120 * time.Millisecond

// TestSlowItemStoreKeepsQueuesFree drives the enchant flow against a store
// whose every item-row write takes longer than the sim pool's slow-task
// budget. The handler queues its writes on the persistence worker instead of
// making them itself, so the player's queue never waits on the database: the
// packets come back at memory speed, the watchdog logs nothing, and the rows
// land once the worker drains.
func TestSlowItemStoreKeepsQueuesFree(t *testing.T) {
	// Not parallel: the slow-task budget is wall-clock, so other tests' CPU
	// load fails it spuriously.
	srv := gameservertest.Boot(t,
		gameservertest.WithCapturedLog(),
		gameservertest.WithSlowStores(slowStoreDelay),
		gameservertest.WithEnchantRoll(func() float64 { return 0.0 }),
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	weapon := srv.GiveItem(t, objID, 30, 1)
	scroll := srv.GiveItem(t, objID, 955, 1)
	startInWorld(t, c)

	openEnchantSelection(t, c, scroll, 955)
	c.Send(encodeRequestEnchantItem(weapon))

	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeSystemMessage, "success SystemMessage")
	if id := systemMessageID(t, frame); id != serverpackets.SystemMessageS1SuccessfullyEnchanted {
		t.Fatalf("message id = %d, want S1SuccessfullyEnchanted (%d)", id, serverpackets.SystemMessageS1SuccessfullyEnchanted)
	}
	assertEnchantResult(t, c.Read(), serverpackets.EnchantResultSuccess)
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeUserInfo, "success UserInfo")

	srv.Settle(t)
	if lines := srv.SlowTaskLogs(); len(lines) != 0 {
		t.Fatalf("queue task blocked on the item store: %v", lines)
	}

	if inst := mustFindItem(t, srv, objID, weapon); inst.EnchantLevel != 1 {
		t.Fatalf("persisted enchant level = %d, want 1", inst.EnchantLevel)
	}
	assertItemGone(t, srv, objID, scroll)
}
