package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: Sweep.java:28-47 hands every item of a spoiled Monster's
// SpoilState to a partyless Player through addEarnedItem(itemId, count,
// true), then clears the state, spoiler included. addEarnedItem
// (Player.java:1861-1904) sends EARNED_S2_S1_S (53) with the item name and
// the count as an item number for more than one unit, EARNED_ITEM_S1 (54)
// with the item name otherwise, and adds the item. Sweeping checks no
// slots. SpoilState is a HashMap<Integer, Integer> filled from the spoil
// category's roll (Monster.java:486-491), so the items come in hash-bucket
// order: potion 20 (bucket 4) before weapon 30 (bucket 14) whatever their
// order in the drop list.

const sweeperSkillID = 42

// spoiledMonsterTemplate is a monster whose spoil category always yields
// one weapon and three potions, the weapon listed first. It has no corpse
// time, so its death schedules no decay.
func spoiledMonsterTemplate() *npc.Template {
	return &npc.Template{
		ID: 100, TemplateID: 100, Type: "Monster", Level: 1, HPMax: 1000,
		AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
		Drops: []item.DropCategory{{
			Kind: item.DropSpoil, Chance: 100,
			Drops: []item.Drop{
				{ItemID: 30, Min: 1, Max: 1, Chance: 100},
				{ItemID: 20, Min: 3, Max: 3, Chance: 100},
			},
		}},
	}
}

// sweepScene boots a Sweeper-casting player with no free slot, targets a
// fresh monster, marks it spoiled by the id spoilerFor returns unless that
// is zero, kills it with the player and gives the corpse a live deadline.
func sweepScene(t *testing.T, spoilerFor func(srv *gameservertest.Server, sweeperID int32) int32) (*gameservertest.Server, int32, *npc.Hostile) {
	t.Helper()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Sweeper", 5, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithSkills(skillPersistence(t, []modelskill.Definition{sweeperSkill()})),
		// No free slot: sweeping checks none.
		gameservertest.WithInventorySlots(1, 1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	seedKnownSkill(t, srv, objID, sweeperSkillID, 1)
	srv.GiveItem(t, objID, item.AdenaID, 1)
	startInWorld(t, c)

	mob := srv.SpawnHostileNPCTemplateAt(t, spoiledMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	drainUntilQuiet(t, c)
	targetHostile(t, c, mob.ObjectID())
	drainUntilQuiet(t, c)

	if spoilerID := spoilerFor(srv, objID); spoilerID != 0 && !mob.SpoilPool().Mark(spoilerID) {
		t.Fatal("fresh monster already spoiled")
	}
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	if !mob.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	// No corpse time schedules no decay; give the corpse a fresh deadline.
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	drainUntilQuiet(t, c)
	return srv, objID, mob
}

// TestSweepEarnsSpoilIntoInventory spoils a monster for the player, kills
// it and sweeps the corpse. The player reads the earned lines after the
// launch, holds the swept items, and the corpse's spoil is gone.
func TestSweepEarnsSpoilIntoInventory(t *testing.T) {
	t.Parallel()
	srv, objID, mob := sweepScene(t, func(_ *gameservertest.Server, sweeperID int32) int32 { return sweeperID })
	c := srv.Client
	if !mob.SpoilPool().Sweepable() {
		t.Fatal("the kill left nothing to sweep")
	}

	c.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	readCastStartFrames(t, c, objID, sweeperSkillID, 1, 500, 500, mob.ObjectID())
	// The sweep lands at the hit, after the launch.
	first := c.ReadWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("nothing followed the sweep's launch")
	}
	var messages [][]byte
	for _, f := range append([][]byte{first}, collectUntilQuiet(t, c)...) {
		if f[0] == serverpackets.OpcodeSystemMessage {
			messages = append(messages, f)
		}
	}
	if len(messages) != 2 {
		t.Fatalf("system messages after the launch = %d, want the two earned lines", len(messages))
	}
	assertSystemMessageParams(t, messages[0], serverpackets.SystemMessageEarnedS2S1S,
		systemMessageParam{serverpackets.SystemMessageParamItemName, 20},
		systemMessageParam{serverpackets.SystemMessageParamItemNumber, 3})
	assertSystemMessageParams(t, messages[1], serverpackets.SystemMessageEarnedItemS1,
		systemMessageParam{serverpackets.SystemMessageParamItemName, 30})

	inv := srv.PlayerInventory(t, objID)
	if got := inv.ItemCount(20, -1, true); got != 3 {
		t.Fatalf("carried potions = %d, want 3", got)
	}
	if got := inv.ItemCount(30, -1, true); got != 1 {
		t.Fatalf("carried weapons = %d, want 1", got)
	}
	if mob.SpoilPool().Sweepable() || mob.SpoilPool().IsSpoiled() {
		t.Fatal("the swept corpse kept its spoil")
	}
}

// Reference: PlayerCast.canCast, case SWEEP (PlayerCast.java:336-351). A
// player's CORPSE_MOB sweep of a Monster nobody spoiled is refused with
// SWEEPER_FAILED_TARGET_NOT_SPOILED (343). One another player spoiled is
// refused with SWEEP_NOT_ALLOWED (683): isLooterOrInLooterParty holds only
// for the spoiler itself while nobody is in a party. Either refusal starts no
// cast, so the corpse keeps its spoil and the caster gets nothing.

// TestSweepOfAnotherPlayersSpoilIsRefused has the player sweep a corpse a
// second character spoiled. The cast is refused with SWEEP_NOT_ALLOWED, the
// spoil stays on the corpse under its spoiler, and the sweeper holds none of
// it.
func TestSweepOfAnotherPlayersSpoilIsRefused(t *testing.T) {
	t.Parallel()
	var spoilerID int32
	srv, objID, mob := sweepScene(t, func(srv *gameservertest.Server, _ int32) int32 {
		spoilerID = srv.SeedCharacterFor(t, "player2", "Spoiler", 5, 0).ID
		return spoilerID
	})
	c := srv.Client
	if !mob.SpoilPool().Sweepable() {
		t.Fatal("the kill left nothing to sweep")
	}

	c.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSweepNotAllowed)
	assertNoActionFailedUntilQuiet(t, c, "sweep of another player's spoil")

	if !mob.SpoilPool().IsSpoiler(spoilerID) || !mob.SpoilPool().Sweepable() {
		t.Fatal("the refused sweep took the spoiler's pool")
	}
	inv := srv.PlayerInventory(t, objID)
	if got := inv.ItemCount(20, -1, true) + inv.ItemCount(30, -1, true); got != 0 {
		t.Fatalf("sweeper holds %d spoil items, want none", got)
	}
}

// TestSweepOfUnspoiledCorpseIsRefused sweeps a corpse nobody spoiled: the
// cast is refused with SWEEPER_FAILED_TARGET_NOT_SPOILED.
func TestSweepOfUnspoiledCorpseIsRefused(t *testing.T) {
	t.Parallel()
	srv, _, mob := sweepScene(t, func(*gameservertest.Server, int32) int32 { return 0 })
	c := srv.Client

	c.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	assertStaticSystemMessage(t, c.Read(), serverpackets.SystemMessageSweeperFailedTargetNotSpoiled)
	assertNoActionFailedUntilQuiet(t, c, "sweep of an unspoiled corpse")
	if mob.SpoilPool().IsSpoiled() || mob.SpoilPool().Sweepable() {
		t.Fatal("an unspoiled corpse gained spoil")
	}
}
