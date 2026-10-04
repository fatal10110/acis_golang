package items

import (
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/fish"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// caughtUndineID is the monster a level-20 fisher's catch turns into:
// 18319 + min(20/11, 7).
const caughtUndineID = 18320

// fishingActionPenaltyLevel is a pumping and reeling level three above the
// fixture's Fishing Expertise 1: it takes the expertise penalty.
const fishingActionPenaltyLevel = 4

// fishingMonsters holds the datapack's Caught Undine.
func fishingMonsters() *npc.Table {
	return npc.NewTable([]*npc.Template{{
		ID: caughtUndineID, TemplateID: caughtUndineID, Type: "Monster", Name: "Caught Undine", Level: 20,
		HPMax: 100, AtkSpd: 300, RunSpeed: 120, WalkSpeed: 60, CanMove: true, AIParams: commons.NewStatSet(),
	}})
}

// biteOn casts r's line and lets the fish bite, with check as the
// percentile roll from the cast until the bite, then 50.
func (r *fishingRig) biteOn(t *testing.T, check int32) []string {
	t.Helper()
	r.cast(t, fishingSkill)
	r.dice.check.Store(check)
	bite, _ := r.awaitEvent(t, "combat ")
	// The fight's first second, the next roll, is a second away: the clock
	// only moves on quiet reads.
	r.dice.check.Store(50)
	fishingEvents(t, r.c)
	return bite
}

// TestFishingCatchTurnsIntoMonster pins FishingStance.end's monster catch
// (Rnd.get(100) < 5): the Caught Undine of the fisher's level is placed at
// its feet, YOU_CAUGHT_SOMETHING_SMELLY_THROW_IT_BACK replaces
// YOU_CAUGHT_SOMETHING, and no fish is added.
func TestFishingCatchTurnsIntoMonster(t *testing.T) {
	t.Parallel()
	r := bootFishing(t, gameservertest.WithNPCs(fishingMonsters()), gameservertest.WithNpcSpawns(nil))
	id := r.objID
	r.biteOn(t, 50)

	// From here every percentile roll is 0: the resting fish stays at
	// rest, no pump is resisted and the catch is a monster.
	r.dice.check.Store(0)
	for range 3 {
		r.cast(t, pumpingSkill)
	}
	requireEvents(t, "catch", r.cast(t, pumpingSkill), append(castStart(pumpingSkill),
		"msg 1465 26",
		"gauge hp 0 mode 0 good 1 anim 1 penalty 0 deceptive 0",
		"msg 1655",
		fmt.Sprintf("end %d win 1", id),
		"msg 1460")...)

	var undines []int32
	for _, obj := range r.srv.State.Objects() {
		if h, ok := obj.(*npc.Hostile); ok && h.Instance.Template.ID == caughtUndineID {
			undines = append(undines, h.ObjectID())
		}
	}
	if len(undines) != 1 {
		t.Fatalf("caught undines in the world = %d, want 1", len(undines))
	}
	// At the fisher's (10, 20, 30), on the ground fishingGeo puts 100 below.
	rec, ok := r.srv.NpcSpawns.SpawnOf(undines[0])
	if !ok || !rec.Fixed || rec.At != (location.Location{X: 10, Y: 20, Z: -70}) {
		t.Fatalf("caught undine spawn = %+v (%v), want a fixed spawn at the fisher's feet", rec, ok)
	}
	r.srv.FlushItems(t)
	for _, inst := range persistedItems(t, r.srv, id) {
		if inst.TemplateID == fishCaughtID {
			t.Fatalf("a monster catch added fish %+v", inst)
		}
	}
}

// TestFishingEmptyDrawLosesBait pins FishingStance.start's miss: a lure
// that draws no fish row ends the cast at once with BAIT_LOST_FISH_GOT_AWAY,
// before any ExFishingStart, and the lure is used up all the same. The line
// is out of the water: the next cast is no cancel.
func TestFishingEmptyDrawLosesBait(t *testing.T) {
	t.Parallel()
	r := bootFishing(t, gameservertest.WithFish(fish.NewTable(nil)))
	id := r.objID
	for range 2 {
		requireEvents(t, "empty draw", r.cast(t, fishingSkill), append(castStart(fishingSkill),
			"msg 1452",
			fmt.Sprintf("end %d win 0", id),
			"msg 1460")...)
	}
	r.srv.Advance(t, 15*time.Second)
	requireEvents(t, "after the empty draws", fishingEvents(t, r.c))
	r.srv.FlushItems(t)
	if inst := mustFindItem(t, r.srv, id, r.lure); inst.Count != 3 {
		t.Fatalf("lures after two empty draws = %d, want 3", inst.Count)
	}
}

// fishingGaugeMode is an action gauge's fish mode.
var fishingGaugeMode = regexp.MustCompile(`^(gauge .* mode )\d`)

// TestFishingExpertisePenalty pins FishingSkill's expertise penalty: a
// pumping or reeling skill three levels above Fishing Expertise loses 50
// damage and says so first (1670); the penalty notice (1671 reeling, 1672
// pumping) follows only a successful action, never a failed or resisted
// one. Level 4's power 62 on a D-grade rod: (int)(62 * 1.1) - 50 = 18.
func TestFishingExpertisePenalty(t *testing.T) {
	t.Parallel()
	// A fish with no regen keeps its HP still between the actions.
	r := bootFishingSpec(t, fishingSpec{equipLure: true, actionLevel: fishingActionPenaltyLevel},
		gameservertest.WithFish(fish.NewTable([]fish.Fish{
			fish.New(fishCaughtID, 1, 100, 0, 1, 1, 500, 5000, 20000, 24000),
		})))
	id := r.objID
	// A bite roll of 80 hooks a fighting fish, which stays fighting while
	// the rolls are 50.
	requireEvents(t, "bite", r.biteOn(t, 80), fmt.Sprintf("combat %d time 24 hp 100 mode 1 lure 1 deceptive 0", id))

	requireEvents(t, "reel a fighting fish", r.cast(t, reelingSkill), append(castStartAt(reelingSkill, fishingActionPenaltyLevel),
		"msg 1670",
		"msg 1467 18",
		"msg 1671 50",
		"gauge hp 82 mode 1 good 1 anim 2 penalty 50 deceptive 0")...)
	requireEvents(t, "pump a fighting fish", r.cast(t, pumpingSkill), append(castStartAt(pumpingSkill, fishingActionPenaltyLevel),
		"msg 1670",
		"msg 1466 18",
		"gauge hp 100 mode 1 good 2 anim 1 penalty 50 deceptive 0")...)

	// A roll of 91 has every action resisted; it also turns the fish
	// around at its next even second, so the mode is left out.
	r.dice.check.Store(91)
	var resisted []string
	for _, e := range r.cast(t, pumpingSkill) {
		resisted = append(resisted, fishingGaugeMode.ReplaceAllString(e, "${1}_"))
	}
	requireEvents(t, "resisted pump", resisted, append(castStartAt(pumpingSkill, fishingActionPenaltyLevel),
		"msg 1670",
		"msg 1464",
		"gauge hp 100 mode _ good 0 anim 1 penalty 50 deceptive 0")...)
}
