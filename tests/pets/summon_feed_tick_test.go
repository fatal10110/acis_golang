package pets

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/pet"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/summon"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The fixture wolf's feeding, out of combat: it eats 5 of its 3000 meal
// every 10 seconds, eats its own food below 55% (1650), is hungry below 30%
// (900), and very hungry below 10% (300).
const (
	petMealPeriod = 10 * time.Second
	wolfMeal      = 5
	wolfHungryFed = 900
)

// bootFedWolf boots the owner with a wolf whose saved meal gauge is fed,
// calls it out and drains the spawn. The owner also carries seeds.
func bootFedWolf(t *testing.T, fed int, extra []gameservertest.Option, seeds ...seedItem) (*petWorld, *summon.Actor) {
	t.Helper()
	h := bootOwnerWithCollarOpts(t, extra, seeds...)
	if !h.srv.DrivesClock() {
		t.Skip("counting 10-second feed ticks needs the driven clock")
	}
	if err := h.srv.Pets.Save(petCtx(), h.collarID, pet.State{
		Level: wolfLevel, Exp: wolfLevelExp, CurHP: wolfMaxHP, CurMP: wolfMaxMP, Fed: fed,
	}); err != nil {
		t.Fatalf("seed pets row: %v", err)
	}
	wolf, _ := h.spawnWolf(t)
	if got := wolf.Fed(); got != fed {
		t.Fatalf("wolf meal gauge = %d, want the saved %d", got, fed)
	}
	drainUntilQuiet(t, h.client)
	return h, wolf
}

// passMeal lets time pass a second at a time until the wolf's next meal, at
// most one feed period, and returns the owner's frames from the second it
// ate in.
func passMeal(t *testing.T, h *petWorld, wolf *summon.Actor) [][]byte {
	t.Helper()
	frames, _, _ := passMealWindow(t, h, wolf)
	return frames
}

// passMealWindow is passMeal also returning the stretch of the driven clock
// the meal fell in: after since and no later than until. Draining the
// client lets the driven clock run too.
func passMealWindow(t *testing.T, h *petWorld, wolf *summon.Actor) (frames [][]byte, since, until time.Time) {
	t.Helper()
	fed := wolf.Fed()
	for range int(petMealPeriod / time.Second) {
		since = wolf.Queue().Now()
		h.srv.Advance(t, time.Second)
		frames = drainFrames(t, h.client)
		if wolf.Fed() != fed {
			return frames, since, wolf.Queue().Now()
		}
	}
	t.Fatalf("wolf meal gauge stayed %d for a whole feed period", fed)
	return nil, time.Time{}, time.Time{}
}

// advanceTo lets the driven clock run to at, reading no frames on the way.
func advanceTo(t *testing.T, h *petWorld, wolf *summon.Actor, at time.Time) {
	t.Helper()
	if d := at.Sub(wolf.Queue().Now()); d > 0 {
		h.srv.Advance(t, d)
	}
}

// feedWolf has the owner feed the wolf the food item objectID it carries.
func feedWolf(t *testing.T, h *petWorld, objectID int32) {
	t.Helper()
	h.client.Send(encodeRequestPetUseItem(objectID))
	readUntilOpcode(t, h.client, serverpackets.OpcodeMagicSkillUse, "feed MagicSkillUse")
	drainFrames(t, h.client)
}

// moveTypes returns the running flags of the ChangeMoveType frames for
// objectID among frames, in order.
func moveTypes(frames [][]byte, objectID int32) []bool {
	var got []bool
	for _, f := range frames {
		if f[0] != serverpackets.OpcodeChangeMoveType {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadInt32() == objectID {
			got = append(got, r.ReadInt32() == 1)
		}
	}
	return got
}

// systemMessages returns the ids of the system messages among frames, in
// order.
func systemMessages(t *testing.T, frames [][]byte) []int {
	t.Helper()
	var ids []int
	for _, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage {
			ids = append(ids, systemMessageID(t, f))
		}
	}
	return ids
}

// TestPetEatsEveryTenSeconds lets a full wolf go through two meals: each
// comes a full feed period after the last, takes its normal meal off the
// gauge, and refreshes its owner's pet window.
func TestPetEatsEveryTenSeconds(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfMaxMeal, nil)

	frames, since, until := passMealWindow(t, h, wolf)
	if got := wolf.Fed(); got != wolfMaxMeal-wolfMeal {
		t.Fatalf("first meal: gauge = %d, want %d", got, wolfMaxMeal-wolfMeal)
	}
	if countOpcode(frames, serverpackets.OpcodePetStatusUpdate) == 0 {
		t.Fatalf("first meal: frames = opcodes %x, want a PetStatusUpdate", frameOpcodes(frames))
	}

	// The first meal fell after since, so the second is not due by since
	// plus a period, and is by until plus a period.
	advanceTo(t, h, wolf, since.Add(petMealPeriod))
	if got := wolf.Fed(); got != wolfMaxMeal-wolfMeal {
		t.Fatalf("gauge = %d before the second meal was due, want %d", got, wolfMaxMeal-wolfMeal)
	}
	advanceTo(t, h, wolf, until.Add(petMealPeriod))
	if got := wolf.Fed(); got != wolfMaxMeal-2*wolfMeal {
		t.Fatalf("second meal: gauge = %d, want %d", got, wolfMaxMeal-2*wolfMeal)
	}
	if frames := drainFrames(t, h.client); countOpcode(frames, serverpackets.OpcodePetStatusUpdate) == 0 {
		t.Fatalf("second meal: frames = opcodes %x, want a PetStatusUpdate", frameOpcodes(frames))
	}
}

// TestHungryPetEatsItsOwnFood has a wolf under its auto-feed limit carry two
// wolf food. Its meal takes 5 off 1000, then it eats one unit: its owner
// reads that it took the food, everyone sees the feed skill, and with 1495 it
// is still under the limit, which its owner is told too.
func TestHungryPetEatsItsOwnFood(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, 1000, nil, seedItem{TemplateID: wolfFoodID, Count: 2})
	h.giveToPet(t, h.seededItem(t, wolfFoodID), 2)

	frames := passMeal(t, h, wolf)
	if got := wolf.Fed(); got != 1000-wolfMeal+wolfFeedAmount {
		t.Fatalf("gauge after the meal = %d, want %d", got, 1000-wolfMeal+wolfFeedAmount)
	}
	took := -1
	for i, f := range frames {
		if f[0] == serverpackets.OpcodeSystemMessage && systemMessageID(t, f) == serverpackets.SystemMessagePetTookS1BecauseHeWasHungry {
			assertSystemMessageItem(t, f, serverpackets.SystemMessagePetTookS1BecauseHeWasHungry, wolfFoodID)
			took = i
		}
	}
	feed := frameIndex(frames, serverpackets.OpcodeMagicSkillUse, wolf.ObjectID())
	if took < 0 || feed < took {
		t.Fatalf("meal frames = opcodes %x, want PET_TOOK_S1 then the feed MagicSkillUse", frameOpcodes(frames))
	}
	if skill := wire.NewReader(frames[feed][9:]).ReadInt32(); skill != wolfFeedSkill {
		t.Fatalf("feed MagicSkillUse skill = %d, want %d", skill, wolfFeedSkill)
	}
	ids := systemMessages(t, frames)
	if len(ids) != 2 || ids[1] != serverpackets.SystemMessageYourPetAteALittleButIsStillHungry {
		t.Fatalf("meal messages = %v, want PET_TOOK_S1 then YOUR_PET_ATE_A_LITTLE_BUT_IS_STILL_HUNGRY", ids)
	}
	if food := wolf.PetInventory().ItemByTemplateID(wolfFoodID); food == nil || food.Snapshot().Count != 1 {
		t.Fatalf("pet food left = %v, want one unit", food)
	}
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 0 {
		t.Fatalf("a wolf fed past its hungry limit changed stance %v", got)
	}
}

// TestHungryPetWalksUntilFed lets a wolf's meal take it under its hungry
// limit with no food: it walks, which everyone around sees, at its walk
// speed. Fed by its owner, it runs again at its next meal.
func TestHungryPetWalksUntilFed(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfHungryFed+wolfMeal-1, nil, seedItem{TemplateID: wolfFoodID, Count: 1})
	runSpeed := wolf.MoveSpeed()

	frames := passMeal(t, h, wolf)
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 1 || got[0] {
		t.Fatalf("hungry meal stance changes = %v, want one to walking; opcodes %x", got, frameOpcodes(frames))
	}
	if ids := systemMessages(t, frames); len(ids) != 0 {
		t.Fatalf("a hungry wolf above its very-hungry line was told %v, want nothing", ids)
	}
	if wolf.Running() {
		t.Fatal("hungry wolf still runs")
	}
	if walk := wolf.MoveSpeed(); walk >= runSpeed || wolf.Move().Speed() != walk {
		t.Fatalf("walking speed = %v (movement %v), want below the run speed %v", walk, wolf.Move().Speed(), runSpeed)
	}

	h.giveToPet(t, h.seededItem(t, wolfFoodID), 1)
	feedWolf(t, h, h.seededItem(t, wolfFoodID))
	if wolf.Hungry() || wolf.Running() {
		t.Fatalf("fed wolf hungry = %v running = %v before its next meal, want fed but still walking", wolf.Hungry(), wolf.Running())
	}

	frames = passMeal(t, h, wolf)
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 1 || !got[0] {
		t.Fatalf("fed meal stance changes = %v, want one to running; opcodes %x", got, frameOpcodes(frames))
	}
	if !wolf.Running() || wolf.MoveSpeed() != runSpeed {
		t.Fatalf("fed wolf running = %v at %v, want running at %v", wolf.Running(), wolf.MoveSpeed(), runSpeed)
	}
}

// TestStarvingPetLeavesItsOwnerForGood empties a wolf's gauge with no food,
// on a 30% leave roll that comes up. Its owner reads that it is very hungry,
// then that it left; its items come back to the owner, and the owner loses
// the collar and the pet's saved row.
func TestStarvingPetLeavesItsOwnerForGood(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfMeal, []gameservertest.Option{gameservertest.WithPetRoll(func(int) int { return 0 })},
		seedItem{TemplateID: item.AdenaID, Count: 40})
	h.giveToPet(t, h.seededItem(t, item.AdenaID), 40)

	frames := passMeal(t, h, wolf)
	ids := systemMessages(t, frames)
	if len(ids) < 2 || ids[0] != serverpackets.SystemMessageYourPetIsVeryHungry || ids[1] != serverpackets.SystemMessageStarvingPetHasLeft {
		t.Fatalf("starving meal messages = %v, want YOUR_PET_IS_VERY_HUNGRY then STARVING_..._YOUR_PET_HAS_LEFT", ids)
	}
	if opcodeIndex(frames, serverpackets.OpcodePetDelete) < 0 {
		t.Fatalf("starving meal frames = opcodes %x, want PetDelete", frameOpcodes(frames))
	}
	if _, ok := h.srv.State.Summon(h.ownerID); ok {
		t.Fatal("the starved wolf is still its owner's summon")
	}
	if got := h.ownerItemCount(t, item.AdenaID); got != 40 {
		t.Fatalf("owner adena = %d, want the pet's 40 back", got)
	}
	if got := h.ownerItemCount(t, wolfCollarID); got != 0 {
		t.Fatalf("owner collars = %d, want the collar gone", got)
	}
	h.srv.FlushPersistence(t)
	if _, ok, err := h.srv.Pets.Get(petCtx(), h.collarID); err != nil || ok {
		t.Fatalf("pets row after the wolf left: ok=%v err=%v, want deleted", ok, err)
	}
}

// TestStarvingPetThatStaysWalks empties a wolf's gauge on a leave roll that
// does not come up: its owner reads that it is very hungry, and it stays,
// walking.
func TestStarvingPetThatStaysWalks(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfMeal, []gameservertest.Option{gameservertest.WithPetRoll(func(int) int { return 99 })})

	frames := passMeal(t, h, wolf)
	if ids := systemMessages(t, frames); len(ids) != 1 || ids[0] != serverpackets.SystemMessageYourPetIsVeryHungry {
		t.Fatalf("starving meal messages = %v, want only YOUR_PET_IS_VERY_HUNGRY", ids)
	}
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("the wolf left on a roll that did not come up")
	}
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 1 || got[0] {
		t.Fatalf("starving meal stance changes = %v, want one to walking", got)
	}
}

// TestRevivedPetRunsAndEatsAgain kills a walking wolf its owner had just fed
// past its hungry limit. Dead, it does not eat. Revived, it runs again at
// once, and eats a full feed period later.
func TestRevivedPetRunsAndEatsAgain(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfHungryFed+wolfMeal-1, nil, seedItem{TemplateID: wolfFoodID, Count: 1})
	passMeal(t, h, wolf)
	if wolf.Running() {
		t.Fatal("precondition: the hungry wolf still runs")
	}
	h.giveToPet(t, h.seededItem(t, wolfFoodID), 1)
	feedWolf(t, h, h.seededItem(t, wolfFoodID))
	fed := wolf.Fed()

	killPet(t, h, wolf)
	h.srv.Advance(t, petMealPeriod)
	drainFrames(t, h.client)
	if got := wolf.Fed(); got != fed {
		t.Fatalf("dead wolf gauge = %d after a feed period, want %d kept", got, fed)
	}

	revived := wolf.Queue().Now()
	runOn(t, wolf.Queue(), func() { wolf.Revive() })
	frames := drainFrames(t, h.client)
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 1 || !got[0] {
		t.Fatalf("revive stance changes = %v, want one to running; opcodes %x", got, frameOpcodes(frames))
	}
	if !wolf.Running() {
		t.Fatal("revived wolf walks")
	}

	advanceTo(t, h, wolf, revived.Add(petMealPeriod-time.Millisecond))
	if got := wolf.Fed(); got != fed {
		t.Fatalf("revived wolf ate %d before a feed period passed", fed-got)
	}
	advanceTo(t, h, wolf, revived.Add(petMealPeriod))
	if got := wolf.Fed(); got != fed-wolfMeal {
		t.Fatalf("revived wolf gauge after its meal = %d, want %d", got, fed-wolfMeal)
	}
}

// TestFleeingPetRuns makes a walking, hungry wolf flee: it is put in its run
// stance first.
func TestFleeingPetRuns(t *testing.T) {
	t.Parallel()
	h, wolf := bootFedWolf(t, wolfHungryFed+wolfMeal-1, nil)
	passMeal(t, h, wolf)
	if wolf.Running() {
		t.Fatal("precondition: the hungry wolf still runs")
	}
	hostile := h.srv.SpawnHostileNPC(t)
	drainFrames(t, h.client)

	runOn(t, wolf.Queue(), func() { wolf.FleeFrom(hostile, 500) })
	frames := drainFrames(t, h.client)
	if got := moveTypes(frames, wolf.ObjectID()); len(got) != 1 || !got[0] {
		t.Fatalf("flee stance changes = %v, want one to running; opcodes %x", got, frameOpcodes(frames))
	}
	if !wolf.Running() {
		t.Fatal("fleeing wolf walks")
	}
}
