package pets

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Herb fixtures for a pet's commanded pickup. The shared catalog's Herb of
// Life (8600) carries no is_tradable, which the pet's ItemSkills use
// refuses; the herbs below are tradable variants, so they reach the skill
// the reference casts on the pet.
const (
	// petHerbID is a tradable herb carrying the instant heal-over-time
	// potion skill petHerbSkill.
	petHerbID = int32(9620)
	// reuseHerbID is a tradable herb whose skill carries a one-minute reuse.
	reuseHerbID = int32(9621)
	// emptyHerbID is a tradable herb with no attached skill.
	emptyHerbID = int32(9622)
	// sharedHerbID is the shared catalog's non-tradable Herb of Life.
	sharedHerbID = int32(8600)

	petHerbSkill   = 2278
	reuseHerbSkill = 2280
)

// herbSkillDefinition is an instant, self-targeting heal-over-time potion,
// the shape of every shipped herb skill.
func herbSkillDefinition(id modelskill.ID, reuse int) modelskill.Definition {
	return modelskill.Definition{
		ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
		SkillType: "HOT", Potion: true, HitTime: 0, ReuseDelay: reuse,
		Effects: []modelskill.EffectTemplate{{Name: "HealOverTime", Count: 5, Time: 3, Value: 12, Icon: true}},
	}
}

// bootHerbWolf boots the owner with a wolf collar, the herb catalog and the
// herb skills, then calls the wolf out and settles its spawn updates.
func bootHerbWolf(t *testing.T, seeds ...seedItem) (*petWorld, *summon.Actor) {
	t.Helper()
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), modelskill.NewTable([]modelskill.Definition{
		{
			ID: summonCreatureID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			SkillType: "SUMMON_CREATURE", StaticHitTime: true, HitTime: 0, StaticReuse: true, ReuseDelay: 0,
		},
		herbSkillDefinition(petHerbSkill, 0),
		herbSkillDefinition(reuseHerbSkill, 60_000),
	}), gamesql.NewCharacterSkillStore(db))
	herb := func(id int32, name string, skills ...item.SkillRef) *item.Template {
		return &item.Template{
			ID: id, Name: name, Kind: item.KindEtcItem, Duration: -1, Stackable: true, Tradable: true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemHerb, Handler: "ItemSkills"},
			AttachedSkills: skills,
		}
	}
	catalog := item.NewTable(append(gameservertest.ItemTemplates().All(),
		herb(petHerbID, "Herb of Vigor", item.SkillRef{ID: petHerbSkill, Level: 1}),
		herb(reuseHerbID, "Herb of Patience", item.SkillRef{ID: reuseHerbSkill, Level: 1}),
		herb(emptyHerbID, "Hollow Herb"),
	))
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithItemTemplates(catalog),
		gameservertest.WithSkills(skills),
	}, seeds...)
	wolf, _ := h.spawnWolf(t)
	h.settleInventoryUpdates(t)
	return h, wolf
}

// settleInventoryUpdates drains the pending inventory updates, then runs one
// idle tick, as giveToPet does, so a later tick shows only what the step
// under test queued.
func (h *petWorld) settleInventoryUpdates(t *testing.T) {
	t.Helper()
	for range 2 {
		h.srv.InventoryUpdates.Tick()
		drainFrames(t, h.client)
	}
}

// seedGroundNearOwner puts count of templateID on the ground beside the
// owner, loot-owned by the owner, and returns its object id.
func (h *petWorld) seedGroundNearOwner(t *testing.T, templateID, count int32) int32 {
	t.Helper()
	known := map[int32]bool{}
	for _, snap := range h.srv.GroundItems.Snapshots(nil) {
		known[snap.ObjectID] = true
	}
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.srv.SeedGroundItem(t, h.ownerID, templateID, count, x+20, y, z)
	drainUntilQuiet(t, h.client)
	for _, snap := range h.srv.GroundItems.Snapshots(nil) {
		if !known[snap.ObjectID] {
			return snap.ObjectID
		}
	}
	t.Fatalf("seeded ground item %d not tracked", templateID)
	return 0
}

// petPickup commands the pet to loot groundID and returns every frame the
// owner received for it, the inventory-update tick that follows included.
func (h *petWorld) petPickup(t *testing.T, groundID int32) [][]byte {
	t.Helper()
	h.client.Send(encodeRequestPetGetItem(groundID))
	frames := syncFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	return append(frames, drainFrames(t, h.client)...)
}

// requirePickupHead checks the loot broadcast every pickup that took the
// item opens with: GetItem naming the pet, then the ground object's
// DeleteObject. It returns the frames after them.
func requirePickupHead(t *testing.T, frames [][]byte, wolf *summon.Actor, groundID int32) [][]byte {
	t.Helper()
	if len(frames) < 2 {
		t.Fatalf("pickup frames = %x, want GetItem then DeleteObject first", frameOpcodes(frames))
	}
	assertFrameOpcode(t, frames[0], serverpackets.OpcodeGetItem, "GetItem")
	r := wire.NewReader(frames[0][1:])
	if picker, ground := r.ReadInt32(), r.ReadInt32(); picker != wolf.ObjectID() || ground != groundID {
		t.Fatalf("GetItem = picker %d item %d, want pet %d item %d", picker, ground, wolf.ObjectID(), groundID)
	}
	assertFrameOpcode(t, frames[1], serverpackets.OpcodeDeleteObject, "DeleteObject")
	if got := wire.NewReader(frames[1][1:]).ReadInt32(); got != groundID {
		t.Fatalf("DeleteObject id = %d, want ground item %d", got, groundID)
	}
	return frames[2:]
}

// withoutOpcode returns frames minus every frame carrying opcode.
func withoutOpcode(frames [][]byte, opcode byte) [][]byte {
	return slices.DeleteFunc(slices.Clone(frames), func(f []byte) bool { return f[0] == opcode })
}

// requirePetStatusUpdate checks the closing status frame every herb pickup
// sends for the pet, handled or not. It pins the current Go frame, a plain
// StatusUpdate, and not the reference one: after destroying the herb the
// reference sends PetStatusUpdate to the owner and SummonInfo to other
// players. Tracked by #3007.
func requirePetStatusUpdate(t *testing.T, frame []byte, wolf *summon.Actor) {
	t.Helper()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStatusUpdate, "pet StatusUpdate")
	if got := wire.NewReader(frame[1:]).ReadInt32(); got != wolf.ObjectID() {
		t.Fatalf("StatusUpdate object = %d, want pet %d", got, wolf.ObjectID())
	}
}

// requireHerbGone checks a picked herb left the world and never reached the
// pet's inventory, live or persisted.
func (h *petWorld) requireHerbGone(t *testing.T, wolf *summon.Actor, groundID, templateID int32) {
	t.Helper()
	if _, ok := h.srv.State.Object(groundID); ok {
		t.Fatalf("herb %d still in the world after the pet's pickup", groundID)
	}
	if inst := wolf.PetInventory().ItemByTemplateID(templateID); inst != nil {
		t.Fatalf("pet inventory holds the herb: %+v", inst.Snapshot())
	}
	if got := h.collarItemCount(t, templateID); got != 0 {
		t.Fatalf("persisted pet herb count = %d, want 0", got)
	}
}

// TestPetPickupConsumesHerb commands the wolf to loot a tradable herb. The
// herb is used on the spot by the pet (SummonAI.thinkPickUp → ItemSkills):
// the pet casts the herb skill on itself, the owner reads PET_USES_S1, and
// the pet's status is rebroadcast. The herb never enters the pet inventory
// and the heal-over-time lands on the pet, not the owner.
func TestPetPickupConsumesHerb(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t)
	groundID := h.seedGroundNearOwner(t, petHerbID, 1)

	// The heal-over-time landing republishes the pet's PetInfo (the effect
	// list's own update, covered by summon_stat_republish_test); the herb
	// flow's frames are the rest.
	rest := withoutOpcode(requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID), serverpackets.OpcodePetInfo)
	if got, want := frameOpcodes(rest), []byte{
		serverpackets.OpcodeMagicSkillUse, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeStatusUpdate,
	}; !slices.Equal(got, want) {
		t.Fatalf("herb use frames = %x, want MagicSkillUse, PET_USES_S1, StatusUpdate (%x)", got, want)
	}
	r := wire.NewReader(rest[0][1:])
	if caster, target, skill, level := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); caster != wolf.ObjectID() || target != wolf.ObjectID() || skill != petHerbSkill || level != 1 {
		t.Fatalf("herb MagicSkillUse = caster %d target %d skill %d/%d, want pet %d on itself with %d/1",
			caster, target, skill, level, wolf.ObjectID(), petHerbSkill)
	}
	assertSystemMessageSkill(t, rest[1], serverpackets.SystemMessagePetUsesS1, petHerbSkill, 1)
	requirePetStatusUpdate(t, rest[2], wolf)

	h.requireHerbGone(t, wolf, groundID, petHerbID)
	if !slices.ContainsFunc(wolf.EffectList().All(), func(e *effect.Effect) bool { return e.Skill.ID == petHerbSkill }) {
		t.Fatal("pet carries no herb effect after consuming it")
	}
}

// TestPetPickupNonTradableHerbIsNotForPets loots the shared catalog's
// non-tradable Herb of Life. The pet still takes and destroys it, but
// ItemSkills refuses a pet an untradable item: the owner reads
// ITEM_NOT_FOR_PETS, no skill is cast, and the status broadcast closes the
// pickup.
func TestPetPickupNonTradableHerbIsNotForPets(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t)
	groundID := h.seedGroundNearOwner(t, sharedHerbID, 1)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	if len(rest) != 2 {
		t.Fatalf("non-tradable herb frames = %x, want ITEM_NOT_FOR_PETS then StatusUpdate", frameOpcodes(rest))
	}
	assertStaticSystemMessage(t, rest[0], serverpackets.SystemMessageItemNotForPets)
	requirePetStatusUpdate(t, rest[1], wolf)
	h.requireHerbGone(t, wolf, groundID, sharedHerbID)
	if effects := wolf.EffectList().All(); len(effects) != 0 {
		t.Fatalf("pet effects after a refused herb = %d, want none", len(effects))
	}
}

// TestPetPickupHerbUnderReuseReportsReuse loots two herbs whose skill has a
// one-minute reuse. The first is used and starts the reuse on the pet; the
// second is still taken and destroyed, but its skill is disabled, so the
// owner reads S1_PREPARED_FOR_REUSE naming it and nothing is cast.
func TestPetPickupHerbUnderReuseReportsReuse(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t)
	first := h.seedGroundNearOwner(t, reuseHerbID, 1)
	second := h.seedGroundNearOwner(t, reuseHerbID, 1)

	used := withoutOpcode(requirePickupHead(t, h.petPickup(t, first), wolf, first), serverpackets.OpcodePetInfo)
	if len(used) != 3 || used[0][0] != serverpackets.OpcodeMagicSkillUse {
		t.Fatalf("first herb frames = %x, want MagicSkillUse, PET_USES_S1, StatusUpdate", frameOpcodes(used))
	}
	assertSystemMessageSkill(t, used[1], serverpackets.SystemMessagePetUsesS1, reuseHerbSkill, 1)

	rest := requirePickupHead(t, h.petPickup(t, second), wolf, second)
	if len(rest) != 2 {
		t.Fatalf("reused herb frames = %x, want S1_PREPARED_FOR_REUSE then StatusUpdate", frameOpcodes(rest))
	}
	assertSystemMessageSkill(t, rest[0], serverpackets.SystemMessageS1PreparedForReuse, reuseHerbSkill, 1)
	requirePetStatusUpdate(t, rest[1], wolf)
	h.requireHerbGone(t, wolf, second, reuseHerbID)
}

// TestPetPickupUnhandledHerbIsAcknowledged loots a herb with no attached
// skill. The reference's ItemSkills logs the missing skill and does nothing
// else; the pet has still taken and destroyed the herb. The ActionFailed
// ahead of the status frame is a known Go-only divergence that the reference
// does not send; this test pins current behavior until #3007 removes it.
func TestPetPickupUnhandledHerbIsAcknowledged(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t)
	groundID := h.seedGroundNearOwner(t, emptyHerbID, 1)

	rest := requirePickupHead(t, h.petPickup(t, groundID), wolf, groundID)
	if len(rest) != 2 {
		t.Fatalf("unhandled herb frames = %x, want ActionFailed then StatusUpdate", frameOpcodes(rest))
	}
	// Go-only ActionFailed, pinned as a known divergence (#3007).
	assertFrameOpcode(t, rest[0], serverpackets.OpcodeActionFailed, "ActionFailed")
	requirePetStatusUpdate(t, rest[1], wolf)
	h.requireHerbGone(t, wolf, groundID, emptyHerbID)
}

// TestPetPickupMergesIntoCarriedStack has the wolf loot adena the owner
// dropped while the wolf already carries some. The looted stack merges into
// the carried one: the owner reads one PetInventoryUpdate, the carried row
// grows to the sum, and the looted instance is deleted rather than saved as
// a second pet row beside it.
func TestPetPickupMergesIntoCarriedStack(t *testing.T) {
	t.Parallel()
	h, wolf := bootHerbWolf(t, seedItem{TemplateID: item.AdenaID, Count: 50})
	adena := h.seededItem(t, item.AdenaID)
	h.giveToPet(t, adena, 10)
	carried := wolf.PetInventory().ItemByTemplateID(item.AdenaID).Snapshot().ObjectID

	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeRequestDropItem(adena, 40, int32(x+20), int32(y), int32(z)))
	readUntilOpcode(t, h.client, serverpackets.OpcodeDropItem, "DropItem")
	h.settleInventoryUpdates(t)

	rest := requirePickupHead(t, h.petPickup(t, adena), wolf, adena)
	if got, want := frameOpcodes(rest), []byte{serverpackets.OpcodePetInventoryUpdate}; !slices.Equal(got, want) {
		t.Fatalf("merge pickup frames after the loot broadcast = %x, want one PetInventoryUpdate", got)
	}
	h.srv.FlushItems(t)
	rows, err := h.srv.Items.ListByOwner(petCtx(), h.collarID)
	if err != nil {
		t.Fatalf("list pet items: %v", err)
	}
	if len(rows) != 1 || rows[0].ObjectID != carried || rows[0].Count != 50 || rows[0].Location != item.LocationPet {
		t.Fatalf("pet rows after the merge = %+v, want the carried stack %d grown to 50", rows, carried)
	}
	if h.persistedRowExists(t, adena) {
		t.Fatalf("looted instance %d kept its row after merging", adena)
	}
}

// persistedRowExists flushes pending item writes and reports whether a row
// for objectID is saved under the owner or the pet's collar.
func (h *petWorld) persistedRowExists(t *testing.T, objectID int32) bool {
	t.Helper()
	h.srv.FlushItems(t)
	for _, owner := range []int32{h.ownerID, h.collarID} {
		rows, err := h.srv.Items.ListByOwner(petCtx(), owner)
		if err != nil {
			t.Fatalf("list items of %d: %v", owner, err)
		}
		if slices.ContainsFunc(rows, func(r *item.Instance) bool { return r.ObjectID == objectID }) {
			return true
		}
	}
	return false
}

// TestPetPickupIgnoresWeightLimit loots ballast with the wolf already past
// its weight limit. SummonAI.thinkPickUp checks slot capacity only, never
// weight (only RequestGiveItemToPet weighs), so the pickup goes through with
// no encumbered refusal.
func TestPetPickupIgnoresWeightLimit(t *testing.T) {
	t.Parallel()
	h := bootBallastWolfCarrying(t, pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal}, petWeightLimit/10+1)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	groundID := h.seedGroundNearOwner(t, ballastID, 1)

	frames := h.petPickup(t, groundID)
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessagePetTooEncumbered {
			t.Fatalf("pickup over the weight limit refused as encumbered: %x", frameOpcodes(frames))
		}
	}
	rest := requirePickupHead(t, frames, wolf, groundID)
	if _, ok := firstOpcode(rest, serverpackets.OpcodePetInventoryUpdate); !ok {
		t.Fatalf("pickup over the weight limit frames = %x, want a PetInventoryUpdate", frameOpcodes(rest))
	}
	if got := h.collarItemCount(t, ballastID); got != petWeightLimit/10+2 {
		t.Fatalf("pet ballast after the pickup = %d, want %d", got, petWeightLimit/10+2)
	}
}

// TestPetPickupRefusedWhenPetSlotsFull loots adena with every one of the
// wolf's 12 slots (MaximumSlotsForPet) taken. The capacity check refuses it:
// the owner reads YOUR_PET_CANNOT_CARRY_ANY_MORE_ITEMS alone and the stack
// stays on the ground, whole and still lootable.
func TestPetPickupRefusedWhenPetSlotsFull(t *testing.T) {
	t.Parallel()
	h := bootOwnerWithCollar(t)
	for range 12 {
		sword := item.Instance{ObjectID: h.srv.NewObjectID(), TemplateID: 30, OwnerID: h.collarID, Count: 1, Location: item.LocationPet}
		if err := h.srv.Items.Create(context.Background(), h.collarID, sword); err != nil {
			t.Fatalf("seed pet sword: %v", err)
		}
	}
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.settleInventoryUpdates(t)
	groundID := h.seedGroundNearOwner(t, item.AdenaID, 40)

	frames := h.petPickup(t, groundID)
	if len(frames) != 1 {
		t.Fatalf("full-pet pickup frames = %x, want only the cannot-carry message", frameOpcodes(frames))
	}
	assertStaticSystemMessage(t, frames[0], serverpackets.SystemMessagePetCannotCarryMoreItems)
	obj, ok := h.srv.State.Object(groundID)
	if !ok {
		t.Fatal("refused stack left the ground")
	}
	if ground, ok := obj.(interface{ Count() int }); !ok || ground.Count() != 40 {
		t.Fatalf("refused ground stack = %T %+v, want 40 adena", obj, obj)
	}
	if wolf.PetInventory().ItemByTemplateID(item.AdenaID) != nil {
		t.Fatal("full pet took the adena")
	}

	// The refusal released the stack: the owner can still pick it up.
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	h.client.Send(encodeAction(groundID, int32(x+20), int32(y), int32(z), false))
	readUntilOpcode(t, h.client, serverpackets.OpcodeGetItem, "owner GetItem")
	h.srv.AdvanceUntil(t, "owner looted the refused stack", func() bool {
		_, ok := h.srv.State.Object(groundID)
		return !ok
	})
}
