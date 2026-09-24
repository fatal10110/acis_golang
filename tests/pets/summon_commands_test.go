package pets

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestPetAttackCommandSendsPetAgainstTarget targets a monster and presses
// the pet-attack shortcut: the pet engages, and its swings drain the
// monster's HP through the real combat stack.
func TestPetAttackCommandSendsPetAgainstTarget(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	petActor, _ := h.spawnWolf(t)
	hostile := h.srv.SpawnHostileNPC(t)

	h.client.Send(encodeAction(hostile.ObjectID(), hostileX, hostileY, hostileZ, false))
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petAttackAction, false))

	h.srv.AdvanceUntil(t, "pet's hits landing on the target", func() bool {
		return hostile.HP() < 1000
	})
	if hostile.HP() <= 0 {
		t.Fatal("fixture monster died; want an engagement, not a kill")
	}
	if petActor.Dead() {
		t.Fatal("pet died attacking the fixture monster")
	}
	drainUntilQuiet(t, h.client)
}

// TestPetCommandsWithNoActiveSummonAnswerActionFailed pins the shortcut
// contract with nothing summoned: every pet command still resolves the
// client's click with ActionFailed instead of silence.
func TestPetCommandsWithNoActiveSummonAnswerActionFailed(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	for _, action := range []int32{15, petAttackAction, 17, petReturnAction, petUnsummonAction} {
		h.client.Send(encodeRequestActionUse(action, false))
		frame := mustRead(t, h.client, "no-summon command reply")
		assertFrameOpcode(t, frame, serverpackets.OpcodeActionFailed, "no-summon ActionFailed")
	}
}

// TestDecorativeSummonSpawnsTreeAndBlocksDuplicate uses the decoration kit:
// the item is consumed, a Christmas Tree decoration spawns at the user's
// spot, and a second use nearby is rejected without consuming.
func TestDecorativeSummonSpawnsTreeAndBlocksDuplicate(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: treeKitID, Count: 2})
	treeKit := h.seededItem(t, treeKitID)

	h.client.Send(encodeUseItem(treeKit, false))
	assertFrameOpcode(t, mustRead(t, h.client, "tree NPCInfo"), serverpackets.OpcodeNPCInfo, "tree NPCInfo")
	h.srv.InventoryUpdates.Tick()
	assertFrameOpcode(t, mustRead(t, h.client, "kit consumption update"), serverpackets.OpcodeInventoryUpdate, "kit InventoryUpdate")
	if got := h.ownerItemCount(t, treeKitID); got != 1 {
		t.Fatalf("decoration kit count = %d, want 1 after one use", got)
	}

	h.client.Send(encodeUseItem(treeKit, false))
	frame := mustRead(t, h.client, "duplicate rejection")
	assertSystemMessageText(t, frame, 1142, "Tree")
	drainUntilQuiet(t, h.client)
	if got := h.ownerItemCount(t, treeKitID); got != 1 {
		t.Fatalf("duplicate rejection changed kit count to %d, want untouched 1", got)
	}
}

// TestWyvernCollarMountsPlayer uses the wyvern collar: Ride announces the
// mount and UserInfo refreshes the player's own view; while mounted, a
// second wyvern collar answers SUMMON_ONLY_ONE.
func TestWyvernCollarMountsPlayer(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t,
		seedItem{TemplateID: wyvernCollarID, Count: 2},
	)
	wyvernCollar := h.seeded[wyvernCollarID][0]
	secondCollar := wyvernCollar

	h.client.Send(encodeUseItem(wyvernCollar, false))
	frame := mustRead(t, h.client, "Ride broadcast")
	assertFrameOpcode(t, frame, serverpackets.OpcodeRide, "Ride")
	r := wire.NewReader(frame[1:])
	if id := r.ReadInt32(); id != h.ownerID {
		t.Fatalf("Ride rider id = %d, want %d", id, h.ownerID)
	}
	r.ReadInt32()
	if mountType := r.ReadInt32(); mountType != 2 {
		t.Fatalf("Ride mount type = %d, want 2 (wyvern)", mountType)
	}
	if npcID := r.ReadInt32(); npcID != wyvernNPCID+1_000_000 {
		t.Fatalf("Ride npc id = %d, want %d", npcID, wyvernNPCID+1_000_000)
	}
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")

	mounted, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	type mounter interface {
		MountType() int32
		MountObjectID() int32
	}
	m, ok := mounted.(mounter)
	if !ok {
		t.Fatalf("world player %T does not expose mount state", mounted)
	}
	if got := m.MountType(); got != 2 {
		t.Fatalf("MountType() = %d, want 2 (wyvern)", got)
	}
	if got := m.MountObjectID(); got != wyvernCollar {
		t.Fatalf("MountObjectID() = %d, want control item %d", got, wyvernCollar)
	}

	h.client.Send(encodeUseItem(secondCollar, false))
	frames := readUntilOpcode(t, h.client, serverpackets.OpcodeSystemMessage, "SUMMON_ONLY_ONE")
	assertStaticSystemMessage(t, frames[len(frames)-1], serverpackets.SystemMessageSummonOnlyOne)
	drainUntilQuiet(t, h.client)

	if got := h.ownerItemCount(t, wyvernCollarID); got != 2 {
		t.Fatalf("wyvern collar count = %d, want both collars kept (%d)", got, 2)
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("mounting registered a summon, want none")
	}
}

// TestUnsummonShortcutDismissesServitor casts a servitor SUMMON skill on a
// fresh session (no pet collar used first): the owner gets the servitor's
// PetInfo. The unsummon shortcut then sends PetDelete, and both the owner's
// summon slot and the object registry drop the servitor.
func TestUnsummonShortcutDismissesServitor(t *testing.T) {
	t.Parallel()
	const (
		summonCatSkill = 1111
		catNPCID       = 12600
	)
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{{
		ID: summonCatSkill, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "SUMMON", NpcID: catNPCID, SummonTotalLifeTime: 1_200_000,
		StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
	}}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: catNPCID, TemplateID: catNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{cat})),
		gameservertest.WithSkills(skills))
	c, ownerID := srv.Client, srv.SoleObjectID(t)
	if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, summonCatSkill, 1); err != nil {
		t.Fatalf("seed known skill: %v", err)
	}
	startInWorld(t, c)

	c.Send(encodeRequestMagicSkillUse(summonCatSkill))
	var servitor *summon.Actor
	srv.AdvanceUntil(t, "servitor in world state", func() bool {
		obj, ok := srv.State.Summon(ownerID)
		if ok {
			servitor, ok = obj.(*summon.Actor)
		}
		return ok
	})
	if !sawServitorPetInfo(drainFrames(t, c), servitor.ObjectID()) {
		t.Fatalf("spawn burst carried no servitor PetInfo for %d", servitor.ObjectID())
	}

	// Despawn sends PetDelete before it clears the object registry; the
	// harness read returns only once the server has finished the request.
	c.Send(encodeRequestActionUse(petUnsummonAction, false))
	frames := readUntilOpcode(t, c, serverpackets.OpcodePetDelete, "PetDelete")
	if got := wire.NewReader(frames[len(frames)-1][1:]).ReadInt32(); got != 1 {
		t.Fatalf("PetDelete summon type = %d, want 1 (servitor)", got)
	}
	if _, ok := srv.State.Summon(ownerID); ok {
		t.Fatal("owner still has an active summon after unsummon")
	}
	if _, ok := srv.State.Object(servitor.ObjectID()); ok {
		t.Fatalf("servitor %d still in the object registry after unsummon", servitor.ObjectID())
	}
}

func encodeRequestMagicSkillUse(skillID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(skillID)
	w.WriteInt32(0) // ctrl
	w.WriteUint8(0) // shift
	return w.Bytes()
}

// sawServitorPetInfo reports whether frames hold a servitor (summon type 1)
// PetInfo for objectID.
func sawServitorPetInfo(frames [][]byte, objectID int32) bool {
	for _, frame := range frames {
		if len(frame) == 0 || frame[0] != serverpackets.OpcodePetInfo {
			continue
		}
		r := wire.NewReader(frame[1:])
		if r.ReadInt32() == 1 && r.ReadInt32() == objectID {
			return true
		}
	}
	return false
}
