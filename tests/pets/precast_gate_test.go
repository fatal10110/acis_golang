package pets

import (
	"context"
	"fmt"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
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
	srv := bootPets(t,
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{wolfTemplate(), treeTemplate(), catTemplate()})),
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

// TestServitorCastDuringRestoreWaitsForThePet casts a servitor SUMMON skill
// while the collar's pets-row read is in flight: inside the cast's hold, and
// after the hold ceiling ended the cast. The reference's caster is still in
// the collar's cast until the pet lands, so PlayableAI.tryToCast queues the
// request as the next intention and answers ActionFailed
// (PlayableAI.java:313-318). Once the pet is out the request runs, and
// L2SkillSummon.checkCondition refuses it with SUMMON_ONLY_ONE before any
// cost (L2SkillSummon.java:86-90, reached from PlayableCast.canCast:70):
// neither its consume item nor its MP is taken, and no cast starts.
func TestServitorCastDuringRestoreWaitsForThePet(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name        string
		pastCeiling bool
	}{
		{name: "inside the hold"},
		{name: "past the hold ceiling", pastCeiling: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := bootSummoner(t, seedItem{TemplateID: catConsumeItemID, Count: 5})
			mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)

			release := h.useCollarRestoreHeld(t)
			if tt.pastCeiling {
				h.passHoldCeiling(t)
			}
			drainFrames(t, h.client)

			h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
			frames := drainFrames(t, h.client)
			if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
				t.Fatalf("servitor cast during the restore = opcodes %x, want ActionFailed alone", frameOpcodes(frames))
			}

			release()
			h.awaitPet(t)
			frames = drainFrames(t, h.client)
			assertNoCatCast(t, frames, "servitor request replayed after the pet landed")
			if !hasStaticSystemMessage(frames, serverpackets.SystemMessageSummonOnlyOne) {
				t.Fatalf("servitor request replayed after the pet landed = opcodes %x, want SUMMON_ONLY_ONE", frameOpcodes(frames))
			}
			assertCatCostsUntouched(t, h, mpBefore, 5)
			obj, _ := h.srv.State.Summon(h.ownerID)
			if s, ok := obj.(interface{ NPCID() int }); !ok || s.NPCID() != wolfNPCID {
				t.Fatalf("active summon = %v, want the inbound wolf", obj)
			}
		})
	}
}

// TestServitorCastWithSummonOutAnswersSummonOnlyOneBeforeCost casts a
// servitor SUMMON skill with a pet already out. L2SkillSummon.checkCondition
// answers SUMMON_ONLY_ONE (L2SkillSummon.java:86-90) from PlayableCast.canCast
// (PlayableCast.java:70), ahead of the consume-item check
// (PlayableCast.java:81-97), so it answers the same with no consume item at
// all. A canCast refusal in PlayerAI.thinkCast sends no ActionFailed
// (PlayerAI.java:288-294). Nothing is paid and no cast starts.
func TestServitorCastWithSummonOutAnswersSummonOnlyOneBeforeCost(t *testing.T) {
	t.Parallel()
	for _, items := range []int{5, 0} {
		t.Run(fmt.Sprintf("%d consume items", items), func(t *testing.T) {
			t.Parallel()
			var seeds []seedItem
			if items > 0 {
				seeds = append(seeds, seedItem{TemplateID: catConsumeItemID, Count: int32(items)})
			}
			h := bootSummoner(t, seeds...)
			h.spawnWolf(t)
			drainUntilQuiet(t, h.client)
			mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)

			h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
			frames := drainFrames(t, h.client)
			if len(frames) != 1 {
				t.Fatalf("servitor cast with a pet out = opcodes %x, want SUMMON_ONLY_ONE alone", frameOpcodes(frames))
			}
			assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessageSummonOnlyOne)
			if h.srv.PlayerCastingNow(t, h.ownerID) {
				t.Fatal("servitor cast with a pet out left a cast running")
			}
			assertCatCostsUntouched(t, h, mpBefore, items)
			obj, _ := h.srv.State.Summon(h.ownerID)
			if s, ok := obj.(interface{ NPCID() int }); !ok || s.NPCID() != wolfNPCID {
				t.Fatalf("active summon = %v, want the wolf", obj)
			}
		})
	}
}

// TestServitorCastWhileMountedRefusedBeforeCost casts a servitor SUMMON
// skill while riding a wyvern. checkCondition does not look at the mount;
// PlayerCast.canCast refuses a rider with SUMMON_ONLY_ONE
// (PlayerCast.java:271-277), after PlayableCast.canCast's consume-item check
// (PlayableCast.java:88-97): with the item it answers SUMMON_ONLY_ONE, and
// without it S1_CANNOT_BE_USED names the skill instead. Either way nothing
// is paid, no cast starts and no servitor spawns.
func TestServitorCastWhileMountedRefusedBeforeCost(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		items     int
		messageID int
	}{
		{items: 5, messageID: serverpackets.SystemMessageSummonOnlyOne},
		{items: 0, messageID: serverpackets.SystemMessageS1CannotBeUsed},
	} {
		t.Run(fmt.Sprintf("%d consume items", tt.items), func(t *testing.T) {
			t.Parallel()
			seeds := []seedItem{{TemplateID: wyvernCollarID, Count: 1}}
			if tt.items > 0 {
				seeds = append(seeds, seedItem{TemplateID: catConsumeItemID, Count: int32(tt.items)})
			}
			h := bootSummoner(t, seeds...)
			h.client.Send(encodeUseItem(h.seededItem(t, wyvernCollarID), false))
			readUntilOpcode(t, h.client, serverpackets.OpcodeUserInfo, "mounted UserInfo")
			drainUntilQuiet(t, h.client)
			if !h.character(t).Mounted() {
				t.Fatal("wyvern collar did not mount the owner")
			}
			mpBefore := h.srv.PlayerCurrentMP(t, h.ownerID)

			h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
			frames := drainFrames(t, h.client)
			if len(frames) != 1 {
				t.Fatalf("servitor cast while mounted = opcodes %x, want one system message", frameOpcodes(frames))
			}
			assertSystemMessageID(t, frames[0], tt.messageID)
			if h.srv.PlayerCastingNow(t, h.ownerID) {
				t.Fatal("servitor cast while mounted left a cast running")
			}
			assertCatCostsUntouched(t, h, mpBefore, tt.items)
			if _, ok := h.srv.State.Summon(h.ownerID); ok {
				t.Fatal("servitor spawned onto a mounted owner")
			}
		})
	}
}

// TestServitorCastMidSwingRunsAfterTheSwing casts a servitor SUMMON skill
// while the owner's swing is in flight. PlayableAI.tryToCast queues the
// request behind the swing with ActionFailed (PlayableAI.java:313-318), and
// the swing clears isAttackingNow before its FINISHED_ATTACK runs the queued
// cast (CreatureAttack.java:213-221), so checkCondition's
// YOU_CANNOT_SUMMON_IN_COMBAT (L2SkillSummon.java:92-96) never answers it:
// the servitor is summoned once the swing is over, and the attack stance
// that outlives the swing does not refuse it.
func TestServitorCastMidSwingRunsAfterTheSwing(t *testing.T) {
	t.Parallel()
	h := bootSummoner(t, seedItem{TemplateID: catConsumeItemID, Count: 5})
	startOwnerSwing(t, h)

	h.client.Send(encodeRequestMagicSkillUse(summonCatSkillID))
	frames := readImmediate(h.client)
	if len(frames) != 1 || frames[0][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("servitor cast mid-swing = opcodes %x, want ActionFailed alone", frameOpcodes(frames))
	}
	h.srv.AdvanceUntil(t, "servitor in world state", func() bool {
		_, ok := h.srv.State.Summon(h.ownerID)
		return ok
	})
	frames = drainFrames(t, h.client)
	if hasStaticSystemMessage(frames, serverpackets.SystemMessageYouCannotSummonInCombat) {
		t.Fatalf("servitor cast after the swing = opcodes %x, answered YOU_CANNOT_SUMMON_IN_COMBAT", frameOpcodes(frames))
	}
	obj, _ := h.srv.State.Summon(h.ownerID)
	if s, ok := obj.(interface{ NPCID() int }); !ok || s.NPCID() != catNPCID {
		t.Fatalf("active summon = %v, want the cat", obj)
	}
}

// assertNoCatCast fails when frames carry the servitor skill's cast start.
func assertNoCatCast(t *testing.T, frames [][]byte, what string) {
	t.Helper()
	for _, frame := range frames {
		if frame[0] != serverpackets.OpcodeMagicSkillUse {
			continue
		}
		r := wire.NewReader(frame[1:])
		r.ReadInt32()
		r.ReadInt32()
		if r.ReadInt32() == summonCatSkillID {
			t.Fatalf("%s started the servitor cast: opcodes %x", what, frameOpcodes(frames))
		}
	}
}

// hasStaticSystemMessage reports whether frames carry system message id.
func hasStaticSystemMessage(frames [][]byte, id int) bool {
	for _, frame := range frames {
		if frame[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(frame[1:]).ReadInt32() == int32(id) {
			return true
		}
	}
	return false
}

// assertCatCostsUntouched checks that the owner still has mpBefore MP and
// items of the servitor's consume item, read from the live inventory: a held
// persistence lane would stall an items-table flush.
func assertCatCostsUntouched(t *testing.T, h *petWorld, mpBefore, items int) {
	t.Helper()
	h.srv.Advance(t, 0)
	if got := h.srv.PlayerCurrentMP(t, h.ownerID); got != mpBefore {
		t.Fatalf("MP = %d after a refused servitor cast, want %d", got, mpBefore)
	}
	if got := h.srv.PlayerInventory(t, h.ownerID).ItemCount(catConsumeItemID, -1, true); got != items {
		t.Fatalf("consume item count = %d after a refused servitor cast, want %d", got, items)
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
