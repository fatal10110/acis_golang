package lifecycle

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestEnterWorldAbortAfterAttachReleasesThePlayer forces one EnterWorld
// failure after attachLivePlayer has already run and requires the login to
// clean up after itself as thoroughly as a logout would.
//
// The trigger is a second datapack-edit shape: an equipped weapon whose
// template carries a stat modifier naming a stat that does not exist.
// stat.ByName rejects it inside EquipItemStats, so RestoreEquippedItemStats
// fails (skill/effect/item_stats.go:60-63, skill/persistence.go:456-464) —
// past the actor queue, the creature runtime and the shadow-item
// registration, and before world.Spawn.
//
// The shadow item is the destructive half of the leak this pins: a
// registration nobody removes keeps decaying one second per tick for a
// character who is not online and destroys the item at zero, so the tracker
// is run past the weapon's whole remaining duration.
func TestEnterWorldAbortAfterAttachReleasesThePlayer(t *testing.T) {
	templates := gameservertest.ItemTemplates()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 1, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithItemTemplates(templates),
	)
	c := srv.Client
	objID := srv.SoleObjectID(t)
	sword := giveShadowSword(t, srv, objID)

	// Equip it and burn one second, so the mana no longer sits at the
	// template's full duration: the next login then pays the repeated-equip
	// penalty, which is what makes its attach visible from outside.
	startInWorld(t, c)
	c.Send(encodeUseItem(sword, false))
	assertFrameOpcode(t, readSkippingEquipNoise(t, c, "equip UserInfo"), serverpackets.OpcodeUserInfo, "equip UserInfo")
	drainUntilQuiet(t, c)
	srv.ShadowItems.Tick()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
	readUntilOpcode(t, c, serverpackets.OpcodeCharSelectInfo)
	if got := shadowSwordMana(t, srv, objID, sword); got != 299 {
		t.Fatalf("mana after the first logout = %d, want 299", got)
	}
	// Baseline taken after a clean logout: whatever queues this boot keeps
	// open, the aborted login below must not add one.
	openQueues := srv.OpenActorQueues()

	tmpl, ok := templates.Get(shadowSwordID)
	if !ok {
		t.Fatal("shadow sword template missing from the catalog")
	}
	sound := tmpl.Modifiers
	tmpl.Modifiers = append(slices.Clone(sound), item.StatModifier{Op: item.FuncAdd, Stat: "notARealStat", Value: 1})

	c.Send(encodeRequestGameStart(0))
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "game start SSQInfo")
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "game start CharSelected")
	c.Send(encodeEnterWorld())
	if !c.AwaitClose(5 * time.Second) {
		t.Fatal("aborted login kept the connection open; the equipped item's stat restore was expected to fail")
	}

	if _, online := srv.State.Player(objID); online {
		t.Error("character stayed in world state after the aborted login")
	}
	if got := srv.OpenActorQueues(); got != openQueues {
		t.Errorf("open actor queues after the aborted login = %d, want %d: the attached player's queue was never closed", got, openQueues)
	}
	// 299 - 60: the attach that ran before the abort tracked the weapon and
	// charged the repeated-equip penalty, so this run really did enter the
	// post-attach window.
	manaAtAbort := shadowSwordMana(t, srv, objID, sword)
	if manaAtAbort != 239 {
		t.Fatalf("mana after the aborted login = %d, want 239 (299 minus the repeated-equip penalty)", manaAtAbort)
	}

	// Well past the weapon's remaining duration: a leaked registration would
	// have decayed it to zero and destroyed the row somewhere in here.
	for i := 0; i < manaAtAbort+10; i++ {
		srv.ShadowItems.Tick()
	}
	if got := shadowSwordMana(t, srv, objID, sword); got != manaAtAbort {
		t.Fatalf("mana after %d offline ticks = %d, want %d unchanged", manaAtAbort+10, got, manaAtAbort)
	}

	// Repair the catalog: a fresh login for the same character succeeds and
	// the weapon is still on it.
	tmpl.Modifiers = sound
	c2 := srv.DialClient(t, srv.Account(), 1)
	frames := startInWorld(t, c2)
	entries := readItemListEntries(t, burstFrame(t, frames, serverpackets.OpcodeItemList))
	e := findItemListEntry(entries, sword)
	if e == nil {
		t.Fatal("shadow weapon missing from the ItemList of the login after the abort")
	}
	if e.equipped != 1 {
		t.Fatalf("shadow weapon equipped flag = %d, want 1", e.equipped)
	}
}
