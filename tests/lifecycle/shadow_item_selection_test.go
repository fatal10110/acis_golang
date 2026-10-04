package lifecycle

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestShadowItemTickDuringSelectionDoesNotLoseItsExpiry: a shadow weapon on
// its last mana second at login is not lost to a decay tick that lands
// while the selection is still restoring the character, before it is
// registered in the world. The reference's tracker holds the restored
// player itself, so an expiry then always reaches it
// (ShadowItemTaskManager._shadowItems). Here a tick in that window must not
// untrack the weapon with nobody to carry out its expiry: the weapon reaches
// the client with its last second, and the next tick destroys and unequips
// it as any expiry does.
func TestShadowItemTickDuringSelectionDoesNotLoseItsExpiry(t *testing.T) {
	var armed atomic.Bool
	var held atomic.Int32
	var srv *gameservertest.Server
	srv = gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithSelectionHold(func(int32) {
			if armed.Load() {
				srv.ShadowItems.Tick()
				held.Add(1)
			}
		}),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := giveShadowSword(t, srv, objID)

	// Equip the 300-second weapon and burn it down to 61 seconds, so the
	// next login's repeated-equip minute leaves it one second, which the
	// ItemList shows as 0 whole minutes.
	startInWorld(t, c)
	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)
	for i := 0; i < 239; i++ {
		srv.ShadowItems.Tick()
	}
	drainUntilQuiet(t, c)
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
	if got := shadowSwordMana(t, srv, objID, sword); got != 61 {
		t.Fatalf("mana after the logout = %d, want 61", got)
	}

	armed.Store(true)
	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	armed.Store(false)
	if got := held.Load(); got != 1 {
		t.Fatalf("selection hold ran %d times, want once", got)
	}
	c.Send(encodeEnterWorld())
	frames := readEnterWorldBurst(t, c)
	e := findItemListEntry(readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList)), sword)
	if e == nil {
		t.Fatal("shadow weapon missing from the login ItemList")
	}
	if e.equipped != 1 {
		t.Fatalf("login ItemList shadow weapon equipped flag = %d, want 1", e.equipped)
	}
	drainUntilQuiet(t, c)

	srv.ShadowItems.Tick()
	srv.Settle(t)
	if !sawManaRanOut(c) {
		t.Fatal("no remaining-mana-is-0 message after the weapon's last second")
	}
	srv.FlushItems(t)
	for _, inst := range persistedItems(t, srv, objID) {
		if inst.ObjectID == sword {
			t.Fatalf("expired shadow weapon row still saved: equipped location %v, mana %d", inst.Location, inst.ManaLeft)
		}
	}
}

// sawManaRanOut reads c until it is quiet and reports whether a
// remaining-mana-is-0 system message came.
func sawManaRanOut(c interface{ ReadWithTimeout(time.Duration) []byte }) bool {
	seen := false
	for i := 0; i < 100; i++ {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if frame == nil {
			return seen
		}
		if frame[0] == serverpackets.OpcodeSystemMessage && len(frame) >= 5 &&
			wire.NewReader(frame[1:]).ReadInt32() == serverpackets.SystemMessageRemainingManaIsNow0 {
			seen = true
		}
	}
	return seen
}
