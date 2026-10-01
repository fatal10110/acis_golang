package pets

import (
	"testing"

	xmldata "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
	"github.com/rs/zerolog"
)

// Oracle for the regen wolf (CON 43) under the shipped Decrease Weight
// (1257) level 3, <add stat="weightLimit" val="9000"/>. A bounded Java probe
// over the aCis build classes (revision bd5d8fe9) evaluated the
// Pet.getWeightLimit base 34500 * Formulas.CON_BONUS[43] * 1.0 through the
// real FuncAdd, then the Pet.refreshWeightPenalty band ladder and the
// PetInventory.validateWeight check (current + weight <= limit):
//
//	1257 lvl 1/2/3 limit 57510/60510/63510
//	weight 54510 unbuffed 54510 LEVEL_4 fits+10=false | buffed 63510 LEVEL_3 fits+10=true
//	weight 54520 unbuffed 54510 LEVEL_4 fits+10=false | buffed 63510 LEVEL_3 fits+10=true
const buffedPetWeightLimit = 63510

// TestDecreaseWeightRaisesPetWeightLimit lands Decrease Weight on a wolf
// loaded exactly to its 54510 limit. Pet.getWeightLimit reads the
// WEIGHT_LIMIT stat on every call, so the buff raises the limit at once:
// the stat change's republish (Creature.broadcastModifiedStats ->
// Summon.updateAndBroadcastStatusAndInfos -> Pet.updateAndBroadcastStatus
// -> refreshWeightPenalty) sends a PetInfo carrying the buffed limit and
// moves the band from LEVEL_4 to LEVEL_3, which republishes the status once
// more. A give the unbuffed limit refused then fits. The buff ending
// restores the unbuffed limit and band, and the same give is refused again.
func TestDecreaseWeightRaisesPetWeightLimit(t *testing.T) {
	t.Parallel()
	decreaseWeight := decreaseWeightEffect(t)
	saved := pet.State{Level: wolfLevel, Exp: wolfLevelExp, CurHP: 100, CurMP: 10, Fed: wolfMaxMeal}
	h := bootBallastWolfCarrying(t, saved, 5451) // 54510: the unbuffed limit
	wolf, burst := h.spawnWolf(t)
	if info, ok := firstOpcode(burst, serverpackets.OpcodePetInfo); !ok {
		t.Fatalf("spawn burst has no PetInfo: opcodes %x", frameOpcodes(burst))
	} else if got := readPetInfoLoad(t, info); got.weight != petWeightLimit || got.limit != petWeightLimit {
		t.Fatalf("spawn PetInfo weight/limit = %d/%d, want %d/%d", got.weight, got.limit, petWeightLimit, petWeightLimit)
	}
	requireWolfBand(t, "loaded to the limit", wolf, 4, 0)
	h.srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	startInWorld(t, observer)
	drainUntilQuiet(t, h.client)
	drainUntilQuiet(t, observer)
	ballast := h.seededItem(t, ballastID)

	h.requireGiveRefused(t, ballast, "unbuffed")

	decreaseWeight.Effector, decreaseWeight.Effected = wolf, wolf
	addToPet(t, wolf, decreaseWeight)
	requireLimitRepublish(t, "buff landing", h, observer, wolf, petWeightLimit, buffedPetWeightLimit)
	requireWolfBand(t, "buffed", wolf, 3, 66)

	frames := h.giveToPet(t, ballast, 1)
	if got := h.collarItemCount(t, ballastID); got != 5452 {
		t.Fatalf("pet ballast after a buffed give = %d, want 5452", got)
	}
	info, ok := firstOpcode(frames, serverpackets.OpcodePetInfo)
	if !ok {
		t.Fatalf("buffed give sent no PetInfo: opcodes %x", frameOpcodes(frames))
	}
	if got := readPetInfoLoad(t, info); got.weight != 54520 || got.limit != buffedPetWeightLimit {
		t.Fatalf("buffed give PetInfo weight/limit = %d/%d, want 54520/%d", got.weight, got.limit, buffedPetWeightLimit)
	}
	requireWolfBand(t, "buffed and loaded", wolf, 3, 66)
	drainFrames(t, observer)

	onPetQueue(t, wolf, func() { wolf.EffectList().Remove(decreaseWeight) })
	requireLimitRepublish(t, "buff ending", h, observer, wolf, 54520, petWeightLimit)
	requireWolfBand(t, "buff ended", wolf, 4, 0)

	h.requireGiveRefused(t, ballast, "buff ended")
	if got := h.collarItemCount(t, ballastID); got != 5452 {
		t.Fatalf("pet ballast after the refused give = %d, want 5452", got)
	}
}

// decreaseWeightEffect builds the shipped Decrease Weight level 3 buff.
func decreaseWeightEffect(t *testing.T) *effect.Effect {
	t.Helper()
	table, err := xmldata.LoadSkillDefinitions(datapack.Path(t, "data", "xml", "skills"), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}
	def, ok := table.Get(modelskill.ID(1257), 3)
	if !ok || len(def.Effects) != 1 {
		t.Fatalf("shipped 1257 level 3 = %+v (found %v), want one effect", def, ok)
	}
	tmpl := def.Effects[0]
	if len(tmpl.Funcs) != 1 || tmpl.Funcs[0].Stat != "weightLimit" || tmpl.Funcs[0].Op != modelskill.FuncAdd || tmpl.Funcs[0].Value != 9000 {
		t.Fatalf("shipped 1257 level 3 funcs = %+v, want add weightLimit 9000", tmpl.Funcs)
	}
	e, err := effect.New(effect.Skill{ID: 1257, Level: 3}, tmpl)
	if err != nil {
		t.Fatalf("effect.New(1257): %v", err)
	}
	return e
}

// requireLimitRepublish checks a weight-limit change that moves the band:
// the owner's first PetInfo already carries weight and limit, and the band change
// republishes the status once more than a stat change that keeps the band
// (see TestPAtkBuffRepublishesPetInfoAndSummonInfo): two PetStatusUpdate
// for the owner, and for the observer those two NpcInfo plus the one the
// effect-icon refresh after every effect change sends.
func requireLimitRepublish(t *testing.T, what string, h *petWorld, observer *testsupport.ScriptedClient, wolf *summon.Actor, weight, limit int32) {
	t.Helper()
	owner, watched := drainFrames(t, h.client), drainFrames(t, observer)
	info, _ := requireRepublished(t, what, owner, watched, wolf.ObjectID())
	if got := readPetInfoLoad(t, info); got.weight != weight || got.limit != limit {
		t.Fatalf("%s: PetInfo weight/limit = %d/%d, want %d/%d", what, got.weight, got.limit, weight, limit)
	}
	if n := countOpcode(owner, serverpackets.OpcodePetStatusUpdate); n != 2 {
		t.Fatalf("%s: owner got %d PetStatusUpdate, want 2 (opcodes %x)", what, n, frameOpcodes(owner))
	}
	if n := countNPCInfoFor(watched, wolf.ObjectID()); n != 3 {
		t.Fatalf("%s: observer got %d pet NpcInfo, want 3", what, n)
	}
	if got := wolf.WeightLimit(); got != int(limit) {
		t.Fatalf("%s: WeightLimit() = %d, want %d", what, got, limit)
	}
}

// requireWolfBand checks the wolf's weight-penalty band and movement speed.
func requireWolfBand(t *testing.T, what string, wolf *summon.Actor, band int, speed float64) {
	t.Helper()
	if got := wolf.WeightPenalty(); got != band {
		t.Fatalf("%s: weight-penalty band = %d, want %d", what, got, band)
	}
	if got := wolf.Move().Speed(); got != speed {
		t.Fatalf("%s: movement speed = %v, want %v", what, got, speed)
	}
}

// requireGiveRefused gives the pet one ballast and checks that the give is
// answered with the too-encumbered message alone and moves nothing.
func (h *petWorld) requireGiveRefused(t *testing.T, ballast int32, what string) {
	t.Helper()
	for range 2 {
		h.srv.InventoryUpdates.Tick()
		drainFrames(t, h.client)
	}
	before := h.ownerItemCount(t, ballastID)
	h.client.Send(encodeRequestGiveItemToPet(ballast, 1))
	refused := syncFrames(t, h.client)
	h.srv.InventoryUpdates.Tick()
	refused = append(refused, drainFrames(t, h.client)...)
	if len(refused) != 1 {
		t.Fatalf("%s: give past the limit sent %x, want only the too-encumbered message", what, frameOpcodes(refused))
	}
	assertStaticSystemMessage(t, refused[0], serverpackets.SystemMessagePetTooEncumbered)
	if got := h.ownerItemCount(t, ballastID); got != before {
		t.Fatalf("%s: owner ballast after refused give = %d, want %d", what, got, before)
	}
}
