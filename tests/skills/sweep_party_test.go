package skills

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/party"
)

// Reference: Sweep.java hands each swept item of a partied sweeper to
// Party.distributeItem(player, itemId, count, true, monster): under
// ITEM_ORDER_SPOIL the next member in turn that is alive, has room and is
// in party range of the monster takes it through addEarnedItem, and every
// other member hears S1_SWEEPED_UP_S3_S2 (608) or S1_SWEEPED_UP_S2 (609)
// naming it. PlayerCast.canCast lets a member of the spoiler's party sweep
// (Player.isLooterOrInLooterParty).

// lootMessage is one decoded SystemMessage: its id and parameters, a
// string for text and an int32 otherwise.
type lootMessage struct {
	id     int32
	params []any
}

func decodeLootMessages(frames [][]byte) []lootMessage {
	var out []lootMessage
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeSystemMessage {
			continue
		}
		r := wire.NewReader(f[1:])
		m := lootMessage{id: r.ReadInt32()}
		for range r.ReadInt32() {
			if r.ReadInt32() == serverpackets.SystemMessageParamText {
				m.params = append(m.params, r.ReadString())
			} else {
				m.params = append(m.params, r.ReadInt32())
			}
		}
		out = append(out, m)
	}
	return out
}

func requireLootMessage(t *testing.T, who string, frames [][]byte, id int, params ...any) {
	t.Helper()
	var found []lootMessage
	for _, m := range decodeLootMessages(frames) {
		if m.id == int32(id) {
			found = append(found, m)
		}
	}
	if len(found) != 1 || !slices.Equal(found[0].params, params) {
		t.Fatalf("%s: messages %d = %+v, want one with %v", who, id, found, params)
	}
}

func encodeJoinPartyWithLoot(name string, loot party.LootRule) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestJoinParty)
	w.WriteString(name)
	w.WriteInt32(int32(loot))
	return w.Bytes()
}

// TestPartySweepFollowsTheSpoilTurn has Caster sweep a corpse its party
// member Mate spoiled, under the by-turn rule that includes spoil: the
// potions go to Mate, whose turn comes first, the weapon to Caster, and
// each hears the other sweep up its share.
func TestPartySweepFollowsTheSpoilTurn(t *testing.T) {
	t.Parallel()
	s := bootSocialScene(t, sweeperSkill())
	s.caster.Send(encodeJoinPartyWithLoot("Mate", party.LootByTurnIncludingSpoil))
	drainUntilQuiet(t, s.caster)
	s.mate.Send(encodePartyAnswer(1))
	s.quiet(t)

	mob := s.srv.SpawnHostileNPCTemplateAt(t, spoiledMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	s.quiet(t)
	targetHostile(t, s.caster, mob.ObjectID())
	s.quiet(t)
	if !mob.SpoilPool().Mark(s.mateID) {
		t.Fatal("fresh monster already spoiled")
	}
	obj, _ := s.srv.State.Player(s.casterID)
	killer, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("Caster is %T", obj)
	}
	if !mob.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	s.quiet(t)

	s.caster.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	readCastStartFrames(t, s.caster, s.casterID, sweeperSkillID, 1, 500, 500, mob.ObjectID())
	first := s.caster.ReadWithTimeout(2 * time.Second)
	if first == nil {
		t.Fatal("nothing followed the sweep's launch")
	}
	casterFrames := append([][]byte{first}, collectUntilQuiet(t, s.caster)...)
	mateFrames := collectUntilQuiet(t, s.mate)

	for _, c := range []struct {
		id      int32
		who     string
		potions int
		weapons int
	}{{s.mateID, "Mate", 3, 0}, {s.casterID, "Caster", 0, 1}} {
		inv := s.srv.PlayerInventory(t, c.id)
		if got := inv.ItemCount(20, -1, true); got != c.potions {
			t.Fatalf("%s carries %d potions, want %d", c.who, got, c.potions)
		}
		if got := inv.ItemCount(30, -1, true); got != c.weapons {
			t.Fatalf("%s carries %d weapons, want %d", c.who, got, c.weapons)
		}
	}
	requireLootMessage(t, "Caster", casterFrames, serverpackets.SystemMessageS1SweptUpS3S2, "Mate", int32(20), int32(3))
	requireLootMessage(t, "Caster", casterFrames, serverpackets.SystemMessageEarnedItemS1, int32(30))
	requireLootMessage(t, "Mate", mateFrames, serverpackets.SystemMessageEarnedS2S1S, int32(20), int32(3))
	requireLootMessage(t, "Mate", mateFrames, serverpackets.SystemMessageS1SweptUpS2, "Caster", int32(30))
	if mob.SpoilPool().Sweepable() || mob.SpoilPool().IsSpoiled() {
		t.Fatal("the swept corpse kept its spoil")
	}
}

// TestPartySweepWithoutSpoilRuleKeepsItAll: under the by-turn rule that
// leaves spoil out, the sweeper keeps the whole spoil, and the other
// members still hear what it swept up.
func TestPartySweepWithoutSpoilRuleKeepsItAll(t *testing.T) {
	t.Parallel()
	s := bootSocialScene(t, sweeperSkill())
	s.caster.Send(encodeJoinPartyWithLoot("Mate", party.LootByTurn))
	drainUntilQuiet(t, s.caster)
	s.mate.Send(encodePartyAnswer(1))
	s.quiet(t)

	mob := s.srv.SpawnHostileNPCTemplateAt(t, spoiledMonsterTemplate(), location.Location{X: hostileX, Y: hostileY, Z: hostileZ})
	s.quiet(t)
	targetHostile(t, s.caster, mob.ObjectID())
	s.quiet(t)
	mob.SpoilPool().Mark(s.casterID)
	obj, _ := s.srv.State.Player(s.casterID)
	killer, _ := network.OnlineCharacter(obj)
	if !mob.TakeDamage(1_000_000, killer) {
		t.Fatal("lethal hit did not kill the monster")
	}
	mob.SetCorpseDeadline(time.Now().Add(time.Minute))
	s.quiet(t)

	s.caster.Send(encodeRequestMagicSkillUse(sweeperSkillID, false, false))
	readCastStartFrames(t, s.caster, s.casterID, sweeperSkillID, 1, 500, 500, mob.ObjectID())
	if s.caster.ReadWithTimeout(2*time.Second) == nil {
		t.Fatal("nothing followed the sweep's launch")
	}
	drainUntilQuiet(t, s.caster)
	mateFrames := collectUntilQuiet(t, s.mate)

	inv := s.srv.PlayerInventory(t, s.casterID)
	if inv.ItemCount(20, -1, true) != 3 || inv.ItemCount(30, -1, true) != 1 {
		t.Fatal("the sweeper did not keep the whole spoil")
	}
	requireLootMessage(t, "Mate", mateFrames, serverpackets.SystemMessageS1SweptUpS3S2, "Caster", int32(20), int32(3))
	requireLootMessage(t, "Mate", mateFrames, serverpackets.SystemMessageS1SweptUpS2, "Caster", int32(30))
}
