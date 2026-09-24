package pets

import (
	"context"
	"testing"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Summoner fixture: a long self-cast to be mid-cast with, and a servitor
// SUMMON skill with a real item and MP cost.
const (
	longCastSkillID  = 3
	summonCatSkillID = 1111
	catNPCID         = 12600
	catConsumeItemID = 20 // the shared catalog's stackable Potion
	catMPConsume     = 5
	catReuseDelay    = 600_000
)

// bootSummoner is bootOwnerWithCollarOpts with the long cast and the servitor
// skill known, and the servitor's template loaded beside the pet fixtures.
func bootSummoner(t *testing.T, seeds ...seedItem) *petWorld {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: longCastSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "DUMMY", StaticHitTime: true, HitTime: 5000, StaticReuse: true, ReuseDelay: 0,
		},
		{
			ID: summonCatSkillID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON", NpcID: catNPCID, SummonTotalLifeTime: 1_200_000,
			StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: catReuseDelay,
			MPConsume: catMPConsume, ItemConsumeID: catConsumeItemID, ItemConsumeCount: 1,
		},
	}), gamesql.NewCharacterSkillStore(db))
	cat := &npc.Template{
		ID: catNPCID, TemplateID: catNPCID, Type: "Servitor", Name: "Kat the Cat", Level: 20,
		HPMax: 500, MPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60,
		CollisionRadius: 8, CollisionHeight: 20,
	}
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), cat})),
		gameservertest.WithSkills(skills))
	ownerID := srv.SoleObjectID(t)
	for _, id := range []int{longCastSkillID, summonCatSkillID} {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), ownerID, 0, id, 1); err != nil {
			t.Fatalf("seed known skill %d: %v", id, err)
		}
	}
	collarID := srv.GiveItem(t, ownerID, wolfCollarID, 1)
	h := &petWorld{srv: srv, client: srv.Client, ownerID: ownerID, collarID: collarID, seeded: map[int32][]int32{}}
	for _, s := range seeds {
		id := srv.GiveItem(t, ownerID, s.TemplateID, s.Count)
		h.seeded[s.TemplateID] = append(h.seeded[s.TemplateID], id)
	}
	startInWorld(t, h.client)
	return h
}

// assertNoCastFrames fails when frames carry any packet of a started cast.
func assertNoCastFrames(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	for _, frame := range frames {
		switch frame[0] {
		case serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeMagicSkillLaunched:
			t.Fatalf("%s started a cast: opcodes %x", what, frameOpcodes(frames))
		}
	}
}

// TestCollarWhileMountedAnswersSummonOnlyOneBeforeCast covers the mount half
// of the pet collar's pre-cast gate: SummonItems.java:42-46 rejects a pet
// collar while mounted with SUMMON_ONLY_ONE, before any cast starts.
func TestCollarWhileMountedAnswersSummonOnlyOneBeforeCast(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: wyvernCollarID, Count: 1})
	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
	drainFrames(t, h.client)

	h.client.Send(encodeUseItem(h.collarID, false))
	frame := mustRead(t, h.client, "SUMMON_ONLY_ONE")
	assertStaticSystemMessage(t, frame, serverpackets.SystemMessageSummonOnlyOne)
	assertNoCastFrames(t, drainFrames(t, h.client), "collar use while mounted")
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("collar use while mounted left a cast running")
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("pet spawned onto a mounted owner")
	}
}

// TestTreeKitSilentMidCast uses the decoration kit during an ordinary cast.
// SummonItems.java:37-38 returns silently above the switch that selects the
// decorative case, so the kit is kept and no tree is planted.
func TestTreeKitSilentMidCast(t *testing.T) {
	t.Parallel()
	h := bootSummoner(t, seedItem{TemplateID: treeKitID, Count: 1})
	h.client.Send(encodeRequestMagicSkillUse(longCastSkillID))
	assertFrameOpcode(t, mustRead(t, h.client, "long cast MagicSkillUse"), serverpackets.OpcodeMagicSkillUse, "MagicSkillUse")
	drainFrames(t, h.client)
	if !h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("long cast not in flight")
	}

	h.client.Send(encodeUseItem(h.seededItem(t, treeKitID), false))
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("tree kit mid-cast = opcodes %x, want silence", frameOpcodes(frames))
	}
	if got := h.ownerItemCount(t, treeKitID); got != 1 {
		t.Fatalf("tree kit count = %d after a mid-cast use, want 1", got)
	}
	if decorationCount(h) != 0 {
		t.Fatal("tree kit planted a decoration mid-cast")
	}
}

// decorationCount counts the decorations in the world.
func decorationCount(h *petWorld) int {
	n := 0
	for _, obj := range h.srv.State.Objects() {
		if _, ok := obj.(*npc.Decoration); ok {
			n++
		}
	}
	return n
}

// TestServitorCastRefusedDuringRestoreKeepsCosts casts a servitor SUMMON
// skill after the hold ceiling ended the collar's cast but while its pets-row
// read is still in flight. The reference is still in that cast, so
// PlayableAI.tryToCast answers ActionFailed and never starts the servitor
// cast: neither its consume item nor its MP is taken.
func TestServitorCastRefusedDuringRestoreKeepsCosts(t *testing.T) {
	t.Parallel()
	h := bootSummoner(t, seedItem{TemplateID: catConsumeItemID, Count: 5})
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)

	release := h.useCollarRestoreHeld(t)
	h.passHoldCeiling(t)
	drainFrames(t, h.client)

	h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
	frames := drainFrames(t, h.client)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("servitor cast during the restore = opcodes %x, want ActionFailed alone", frameOpcodes(frames))
	}
	h.srv.Advance(t, 0)
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP = %d after a refused servitor cast, want %d", got, mpBefore)
	}
	// The live inventory, not the items table: the held lane would stall a
	// flush.
	if got := h.srv.PlayerInventory(t, h.ownerID).ItemCount(catConsumeItemID, -1, true); got != 5 {
		t.Fatalf("consume item count = %d after a refused servitor cast, want 5", got)
	}

	release()
	h.awaitPet(t)
	obj, _ := h.srv.State.Summon(h.ownerID)
	if s, ok := obj.(interface{ NPCID() int }); !ok || s.NPCID() != wolfNPCID {
		t.Fatalf("active summon = %v, want the inbound wolf", obj)
	}
}

// TestServitorCastOnReuseDuringRestoreAnswersReuseFirst casts the servitor
// skill once, dismisses it, then casts it again inside a pet's pets-row read
// while it is still on reuse. PlayableAI.tryToCast runs canAttemptCast before
// the casting-now queue (PlayableAI.java:306 vs :313), so the reuse refusal
// still answers with S1_PREPARED_FOR_REUSE ahead of ActionFailed, and nothing
// is paid.
func TestServitorCastOnReuseDuringRestoreAnswersReuseFirst(t *testing.T) {
	t.Parallel()
	h := bootSummoner(t, seedItem{TemplateID: catConsumeItemID, Count: 5})
	h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
	h.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		_, ok := h.srv.State.Summon(h.ownerID)
		return ok
	})
	drainFrames(t, h.client)
	h.client.Send(encodeRequestActionUse(petUnsummonAction, false))
	readUntilOpcode(t, h.client, serverpackets.OpcodePetDelete, "PetDelete")
	drainFrames(t, h.client)

	release := h.useCollarRestoreHeld(t)
	h.passHoldCeiling(t)
	drainFrames(t, h.client)
	mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)
	inv := h.srv.PlayerInventory(t, h.ownerID)
	itemsBefore := inv.ItemCount(catConsumeItemID, -1, true)

	h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
	frames := drainFrames(t, h.client)
	if len(frames) != 2 {
		t.Fatalf("servitor cast on reuse during the restore = opcodes %x, want S1_PREPARED_FOR_REUSE then ActionFailed", frameOpcodes(frames))
	}
	assertSystemMessageID(t, frames[0], serverpackets.SystemMessageS1PreparedForReuse)
	assertFrameOpcode(t, frames[1], serverpackets.OpcodeActionFailed, "ActionFailed")
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP = %d after a refused servitor cast, want %d", got, mpBefore)
	}
	if got := inv.ItemCount(catConsumeItemID, -1, true); got != itemsBefore {
		t.Fatalf("consume item count = %d after a refused servitor cast, want %d", got, itemsBefore)
	}

	release()
	h.awaitPet(t)
}

// TestWyvernRejectedAfterCastStoppedMidRestore stops the collar's cast with a
// crowd-control effect while its pets-row read is in flight, then uses a
// wyvern collar. The reference's caster is still in that cast until the pet
// lands (SummonCreature.java:58-64), so SummonItems.java:37-38 returns
// silently and the owner is never mounted with a pet arriving beside them.
func TestWyvernRejectedAfterCastStoppedMidRestore(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t, seedItem{TemplateID: wyvernCollarID, Count: 1})
	release := h.useCollarRestoreHeld(t)

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from world state")
	}
	owner, ok := obj.(interface {
		effect.Actor
		EffectList() *effect.List
		Queue() *sim.Queue
		MountType() int32
	})
	if !ok {
		t.Fatalf("world player %T lacks the effect/mount surface", obj)
	}
	// RemoveTarget stops the cast without disabling skills, so only the
	// in-flight restore stands between the wyvern collar and a mount.
	e, err := effect.New(effect.Skill{ID: 101, Level: 1, Debuff: true}, modelskill.EffectTemplate{Name: "RemoveTarget", Time: 30})
	if err != nil {
		t.Fatalf("effect.New(RemoveTarget): %v", err)
	}
	e.Effector, e.Effected = owner, owner
	done := make(chan struct{})
	if !owner.Queue().Post(func() { owner.EffectList().Add(e); close(done) }) {
		t.Fatal("post RemoveTarget: queue closed")
	}
	<-done
	if h.srv.PlayerCastingNow(t, h.ownerID) {
		t.Fatal("RemoveTarget left the summon cast running: the window this test needs was never open")
	}
	if _, spawned := h.srv.State.Summon(h.ownerID); spawned {
		t.Fatal("pet already in world: the restore was no longer in flight")
	}
	drainFrames(t, h.client)

	h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
	if frames := drainFrames(t, h.client); len(frames) != 0 {
		t.Fatalf("wyvern collar after a stopped cast mid-restore = opcodes %x, want silence", frameOpcodes(frames))
	}

	release()
	h.awaitPet(t)
	if got := owner.MountType(); got != 0 {
		t.Fatalf("owner MountType() = %d with a pet out, want unmounted", got)
	}
}
