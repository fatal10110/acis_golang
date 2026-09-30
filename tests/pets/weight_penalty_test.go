package pets

import (
	"context"
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// ballastID is a weight-10 item a pet may carry, so a give lands the pet's
// load on any multiple of 10, its weight limit included.
const ballastID = int32(9510)

// Oracle for the regen wolf (CON 43, DEX 30, MEN 20, level 10, run speed
// 120, HP/MP regen 2.0/0.9). A bounded Java probe over the aCis build
// classes (revision bd5d8fe9) evaluated Pet.getWeightLimit, the
// Pet.refreshWeightPenalty band ladder, PetStatus.getMoveSpeed with
// FuncMoveSpeed, CreatureStatus.getMovementSpeedMultiplier and
// PetStatus.getRegenHp/getRegenMp with the real WeightPenalty enum and
// Formulas bonus tables:
//
//	weightLimit = (int)(34500 * CON_BONUS[43]=1.58 * 1.0) = 54510
//	weight 27250 NONE    move 132 multiplier 1.1f  regen HP 3.1284 MP 1.08702
//	weight 27260 LEVEL_1 move 132 multiplier 1.1f  regen HP 1.5642 MP 0.54351
//	weight 43610 LEVEL_3 move  66 multiplier 0.55f regen HP 1.5642 MP 0.54351
//	weight 54510 LEVEL_4 move   0 multiplier 0     regen HP 0.31284 MP 0.108702
const petWeightLimit = 54510

type petLoad struct {
	band       int
	weight     int32
	moveSpeed  int32
	multiplier float64
	hpRegen    float64
	mpRegen    float64
}

// TestPetWeightPenaltyBands loads a wolf through give-to-pet past 50%, 80%
// and exactly 100% of its weight limit, then unloads it below 50%. Each
// band crossing republishes the pet's status once more than a weight change
// that keeps the band (Pet.refreshWeightPenalty broadcasting from
// Pet.updateAndBroadcastStatus), and the PetInfo that follows carries the
// band's speed.
func TestPetWeightPenaltyBands(t *testing.T) {
	t.Parallel()
	h := bootBallastWolf(t)
	wolf, burst := h.spawnWolf(t)
	if info, ok := firstOpcode(burst, serverpackets.OpcodePetInfo); !ok {
		t.Fatalf("spawn burst has no PetInfo: opcodes %x", frameOpcodes(burst))
	} else if got := readPetInfoLoad(t, info); got.limit != petWeightLimit {
		t.Fatalf("spawn PetInfo weight limit = %d, want %d", got.limit, petWeightLimit)
	}
	h.srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer)
	ballast := h.seededItem(t, ballastID)

	steps := []struct {
		name     string
		give     int32
		crossing bool
		want     petLoad
	}{
		{"just under half", 1, false, petLoad{0, 27250, 132, float64(float32(1.1)), 3.1284, 1.08702}},
		{"past half", 1, true, petLoad{1, 27260, 132, float64(float32(1.1)), 1.5642, 0.54351}},
		{"past 80%", 1635, true, petLoad{3, 43610, 66, float64(float32(0.55)), 1.5642, 0.54351}},
		{"at the limit", 1090, true, petLoad{4, 54510, 0, 0, 0.31284, 0.108702}},
	}
	for _, step := range steps {
		frames := h.giveToPet(t, ballast, step.give)
		requirePetLoad(t, step.name, h, observer, wolf, frames, step.crossing, step.want)
	}

	carried := wolf.PetInventory().ItemByTemplateID(ballastID)
	if carried == nil {
		t.Fatal("pet carries no ballast")
	}
	frames := h.takeFromPet(t, carried.ObjectID, 2726)
	requirePetLoad(t, "back under half", h, observer, wolf, frames, true,
		petLoad{0, 27250, 132, float64(float32(1.1)), 3.1284, 1.08702})

	// The regeneration tick reads the restored rate: the wounded wolf gains
	// its full 3.1284 HP.
	regenTick(t, h)
	if got, want := wolf.HP(), 100+3.1284; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP after an unloaded regen tick = %v, want %v", got, want)
	}
}

// TestGiveToPetPastWeightLimitRefused gives a wolf carrying 27240 of its
// 54510 limit one ballast more than fits. PetInventory.validateWeight
// (current + count*weight <= limit) fails, so RequestGiveItemToPet answers
// with UNABLE_TO_PLACE_ITEM_YOUR_PET_IS_TOO_ENCUMBERED alone and moves
// nothing. A give that lands exactly on the limit passes.
func TestGiveToPetPastWeightLimitRefused(t *testing.T) {
	t.Parallel()
	h := bootBallastWolf(t)
	h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	ballast := h.seededItem(t, ballastID)
	// Settle the spawn's pending inventory updates, as giveToPet does, so
	// the tick after the refusal shows only what the refusal queued.
	for range 2 {
		h.srv.InventoryUpdates.Tick()
		drainFrames(t, h.client)
	}

	h.client.Send(encodeRequestGiveItemToPet(ballast, 2728))
	refused := syncFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	refused = append(refused, drainFrames(t, h.client)...)
	if len(refused) != 1 {
		t.Fatalf("over-limit give sent %x, want only the too-encumbered message", frameOpcodes(refused))
	}
	assertStaticSystemMessage(t, refused[0], serverpackets.SystemMessagePetTooEncumbered)
	if got := h.ownerItemCount(t, ballastID); got != 2728 {
		t.Fatalf("owner ballast after refused give = %d, want 2728", got)
	}
	if got := h.collarItemCount(t, ballastID); got != 2724 {
		t.Fatalf("pet ballast after refused give = %d, want 2724", got)
	}

	h.giveToPet(t, ballast, 2727)
	if got := h.collarItemCount(t, ballastID); got != 5451 {
		t.Fatalf("pet ballast after a give to the limit = %d, want 5451", got)
	}
}

// TestOverloadedPetRegenTick runs a regeneration tick on a wolf in the
// half-weight band: it regains half its unloaded rate, and the tick's
// status republish keeps the band without a second broadcast.
func TestOverloadedPetRegenTick(t *testing.T) {
	t.Parallel()
	h := bootBallastWolf(t)
	wolf, _ := h.spawnWolf(t)
	drainUntilQuiet(t, h.client)
	h.giveToPet(t, h.seededItem(t, ballastID), 2)

	regenTick(t, h)
	if got, want := wolf.HP(), 100+1.5642; math.Abs(got-want) > 1e-9 {
		t.Fatalf("HP after a LEVEL_1 regen tick = %v, want %v", got, want)
	}
	if got, want := wolf.MPValue(), 10+1.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("MP after a LEVEL_1 regen tick = %v, want %v (0.54351 raised to 1)", got, want)
	}
	if n := countOpcode(drainFrames(t, h.client), serverpackets.OpcodePetStatusUpdate); n != 1 {
		t.Fatalf("regen tick sent %d PetStatusUpdate, want 1 (band unchanged)", n)
	}
}

// bootBallastWolf boots the owner with 2728 ballast and a wounded regen
// wolf saved under the collar, already carrying 2724 ballast (27240, just
// under half its limit). The fixture owner's CON 0 limit is quadrupled (the
// pet's stays at the shipped 1.0) so the owner's ballast never puts it in a
// weight-penalty band of its own.
func bootBallastWolf(t *testing.T) *petWorld {
	t.Helper()
	catalog := append(gameservertest.ItemTemplates().All(), &item.Template{
		ID: ballastID, Name: "Ballast", Kind: item.KindEtcItem, Duration: -1,
		Stackable: true, Dropable: true, Tradable: true, Destroyable: true,
		EtcItem: &item.EtcItemDetail{}, Weight: 10,
	})
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{regenWolfTemplate(), treeTemplate()})),
		gameservertest.WithItemTemplates(item.NewTable(catalog)),
		gameservertest.WithWeightLimitMultiplier(4),
	}, seedItem{TemplateID: ballastID, Count: 2728})
	saved := pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal}
	if err := h.srv.Pets.Save(context.Background(), h.collarID, saved); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	carried := item.Instance{ObjectID: h.srv.NewObjectID(), TemplateID: ballastID, OwnerID: h.collarID, Count: 2724, Location: item.LocationPet}
	if err := h.srv.Items.Create(context.Background(), h.collarID, carried); err != nil {
		t.Fatalf("seed pet ballast: %v", err)
	}
	return h
}

// requirePetLoad checks one weight change: the owner's PetStatusUpdate and
// the observer's NpcInfo count (two on a band crossing, one otherwise), the
// PetInfo that follows, and the wolf's band, movement speed and regen.
func requirePetLoad(t *testing.T, name string, h *petWorld, observer *testsupport.ScriptedClient, wolf *summon.Actor, frames [][]byte, crossing bool, want petLoad) {
	t.Helper()
	broadcasts := 1
	if crossing {
		broadcasts = 2
	}
	if n := countOpcode(frames, serverpackets.OpcodePetStatusUpdate); n != broadcasts {
		t.Fatalf("%s: owner got %d PetStatusUpdate, want %d (opcodes %x)", name, n, broadcasts, frameOpcodes(frames))
	}
	if n := countNPCInfoFor(drainFrames(t, observer), wolf.ObjectID()); n != broadcasts {
		t.Fatalf("%s: observer got %d pet NpcInfo, want %d", name, n, broadcasts)
	}
	info, ok := firstOpcode(frames, serverpackets.OpcodePetInfo)
	if !ok {
		t.Fatalf("%s: no PetInfo (opcodes %x)", name, frameOpcodes(frames))
	}
	got := readPetInfoLoad(t, info)
	if got.weight != want.weight || got.moveSpeed != want.moveSpeed || got.multiplier != want.multiplier {
		t.Fatalf("%s: PetInfo weight/move speed/multiplier = %d/%d/%v, want %d/%d/%v",
			name, got.weight, got.moveSpeed, got.multiplier, want.weight, want.moveSpeed, want.multiplier)
	}
	if band := wolf.WeightPenalty(); band != want.band {
		t.Fatalf("%s: weight-penalty band = %d, want %d", name, band, want.band)
	}
	if speed := wolf.Move().Speed(); speed != float64(want.moveSpeed) {
		t.Fatalf("%s: movement speed = %v, want %d", name, speed, want.moveSpeed)
	}
	if hp, mp := wolf.HPRegenRate(), wolf.MPRegenRate(); math.Abs(hp-want.hpRegen) > 1e-9 || math.Abs(mp-want.mpRegen) > 1e-9 {
		t.Fatalf("%s: regen HP/MP = %v/%v, want %v/%v", name, hp, mp, want.hpRegen, want.mpRegen)
	}
}

// takeFromPet moves count of the pet's stack objectID back to the owner and
// returns the frames the post-take tick delivered.
func (h *petWorld) takeFromPet(t *testing.T, objectID, count int32) [][]byte {
	t.Helper()
	h.client.Send(encodeRequestGetItemFromPet(objectID, count))
	h.syncOnSkillList(t)
	drainFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	return drainFrames(t, h.client)
}

// syncFrames returns every frame the server sent before answering a
// neutral skill-list round trip, which proves it ran what was sent before.
func syncFrames(t *testing.T, c *testsupport.ScriptedClient) [][]byte {
	t.Helper()
	c.Send(encodeSingleOpcode(clientpackets.OpcodeRequestSkillList))
	var frames [][]byte
	for {
		frame := mustRead(t, c, "frames before SkillList")
		if frame[0] == serverpackets.OpcodeSkillList {
			return frames
		}
		frames = append(frames, frame)
	}
}

type petInfoLoad struct {
	multiplier               float64
	weight, limit, moveSpeed int32
}

// readPetInfoLoad returns PetInfo's movement speed multiplier, carried
// weight, weight limit and move speed fields.
func readPetInfoLoad(t *testing.T, frame []byte) petInfoLoad {
	t.Helper()
	readPetInfoName(t, frame)
	r := wire.NewReader(frame[1:])
	for i := 0; i < 19; i++ {
		r.ReadInt32()
	}
	var got petInfoLoad
	got.multiplier = r.ReadFloat64()
	for i := 1; i < 4; i++ {
		r.ReadFloat64()
	}
	for i := 0; i < 3; i++ {
		r.ReadInt32()
	}
	for i := 0; i < 5; i++ {
		r.ReadUint8()
	}
	r.ReadString()            // name
	r.ReadString()            // title
	for i := 0; i < 11; i++ { // 1, pvp flag, karma, fed/max, hp/max, mp/max, sp, level
		r.ReadInt32()
	}
	for i := 0; i < 3; i++ { // exp, this-level exp, next-level exp
		r.ReadInt64()
	}
	got.weight = r.ReadInt32()
	got.limit = r.ReadInt32()
	for i := 0; i < 7; i++ { // p.atk, p.def, m.atk, m.def, accuracy, evasion, critical
		r.ReadInt32()
	}
	got.moveSpeed = r.ReadInt32()
	if err := r.Err(); err != nil {
		t.Fatalf("read PetInfo load: %v", err)
	}
	return got
}
