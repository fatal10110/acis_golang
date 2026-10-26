package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// moveBeside puts the character dx in front of h and runs one AI cycle.
func (w *ind7World) moveBeside(t *testing.T, h *npc.Hostile, dx int) {
	t.Helper()
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	x, y, z := h.Position()
	if err := w.srv.State.Move(obj, x-dx, y, z); err != nil {
		t.Fatalf("move the character: %v", err)
	}
	w.ticks(t, 1)
}

// TestValakasPrivateLavasaurusStrikesItsMark ports ValakasPrivateLavasaurus's
// created and timer hooks. Raised naming the character in its first spawn
// parameter, it wants its magic strike on it, held, at 1000000; raised
// naming nobody, it wants nothing. A minute later the one that is not
// fighting has left the world; the one the character hit, fighting it,
// stays.
func TestValakasPrivateLavasaurusStrikesItsMark(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	master := w.spawn(t, rhwMasterID, 400, 0)
	me := w.player.ObjectID()
	marked := w.raise(t, master, rhwLavaID, 500, 0, script.Params{P1: me})
	w.wantCasts(t, "raised naming the character", marked, map[modelskill.ID]ind1Cast{rhwDDMagic: {me, 1000000}})
	idle := w.raise(t, master, rhwLavaID, -3000, 0, script.Params{})
	w.wantCasts(t, "raised naming nobody", idle, nil)
	fighter := w.raise(t, master, rhwLavaID, 0, 3000, script.Params{})
	w.moveBeside(t, fighter, 50)

	w.srv.Advance(t, 59*time.Second)
	live := w.liveNPCs(rhwLavaID)
	if !slices.Contains(live, idle) || !slices.Contains(live, fighter) {
		t.Fatal("a lavasaurus left the world before its minute")
	}
	if i := script.NPCOf(fighter).Intention(); i != script.IntentionAttack && i != script.IntentionCast {
		t.Fatalf("the lavasaurus beside the character has intention %v, want attack or cast", i)
	}
	w.srv.Advance(t, time.Second)
	live = w.liveNPCs(rhwLavaID)
	if slices.Contains(live, idle) {
		t.Error("the idle lavasaurus is still in the world a minute after it was raised")
	}
	if !slices.Contains(live, fighter) {
		t.Error("the fighting lavasaurus left the world")
	}
}

// TestWarriorDDMagicHoldAnswersHits ports WarriorDDMagicHold's attacked and
// clan-attacked hooks. Out of a fight, the character's hit wants its magic
// strike on the character, held, at 1000000 and a held attack at 100; its
// clan fellow 50 away, past its seventh AI cycle, the strike and a held
// attack at 50, and nothing before that cycle. Fighting the character,
// another hit adds the attack alone, 100, beside the 50 of the clan call
// it makes to itself.
func TestWarriorDDMagicHoldAnswersHits(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	master := w.spawn(t, rhwMasterID, 400, 0)
	me := w.player.ObjectID()
	young, youngFellow := w.raise(t, master, rhwLavaID, 500, 0, script.Params{}), w.raise(t, master, rhwLavaID, 550, 0, script.Params{})
	old, oldFellow := w.raise(t, master, rhwLavaID, -500, 0, script.Params{}), w.raise(t, master, rhwLavaID, -550, 0, script.Params{})
	strike := map[modelskill.ID]ind1Cast{rhwDDMagic: {me, 1000000}}

	w.hit(t, young, 10)
	w.wantCasts(t, "hit out of a fight", young, strike)
	if got := w.attackWeight(young); got != 100 {
		t.Errorf("attack desire after the hit = %v, want 100", got)
	}
	w.wantCasts(t, "a clan fellow in its first AI cycles", youngFellow, nil)
	if got := w.attackWeight(youngFellow); got != 0 {
		t.Errorf("a clan fellow's attack desire in its first AI cycles = %v, want 0", got)
	}

	w.ticks(t, 8)
	w.clearDesires(t, old)
	w.hit(t, old, 10)
	w.wantCasts(t, "a clan fellow past its seventh AI cycle", oldFellow, strike)
	if got := w.attackWeight(oldFellow); got != 50 {
		t.Errorf("a clan fellow's attack desire past its seventh AI cycle = %v, want 50", got)
	}

	w.moveBeside(t, young, 50)
	w.srv.AdvanceUntil(t, "the fight", func() bool {
		i := script.NPCOf(young).Intention()
		return i == script.IntentionAttack || i == script.IntentionCast
	})
	before, casts := w.attackWeight(young), ind1Casts(young)
	w.hit(t, young, 10)
	if got := w.attackWeight(young); got != before+150 {
		t.Errorf("attack desire after a hit in the fight = %v, want %v", got, before+150)
	}
	if got := ind1Casts(young); len(got) != len(casts) || got[rhwDDMagic].Weight != casts[rhwDDMagic].Weight {
		t.Errorf("casts after a hit in the fight = %+v, want unchanged %+v", got, casts)
	}
}

// TestWarriorDDMagicHoldAggressiveStrikesWhatItSees ports
// WarriorDDMagicHoldAggressive's see-creature hook: the character coming in
// sight draws its magic strike, held, at 1000000 and a held attack at 50.
func TestWarriorDDMagicHoldAggressiveStrikesWhatItSees(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	master := w.spawn(t, rhwMasterID, 400, 0)
	lava := w.raise(t, master, rhwLavaID, 3000, 0, script.Params{})
	w.wantCasts(t, "out of its sight", lava, nil)
	w.moveBeside(t, lava, 100)
	w.wantCasts(t, "after it saw the character", lava, map[modelskill.ID]ind1Cast{rhwDDMagic: {w.player.ObjectID(), 1000000}})
	if got := w.attackWeight(lava); got != 50 {
		t.Errorf("attack desire after it saw the character = %v, want 50", got)
	}
}

// TestWarriorDDMagicHoldKeepsItsTargetClose ports WarriorDDMagicHold's
// use-skill-finished and timer hooks. Its strike on the character 100
// away ends wanting the next one: it keeps striking. A cast from 550
// ends with no strike wanted; a second later, the character 100 away and
// still its top desire target, its check wants the strike.
func TestWarriorDDMagicHoldKeepsItsTargetClose(t *testing.T) {
	t.Parallel()
	w := bootRhw(t, raidBoot{})
	master := w.spawn(t, rhwMasterID, 400, 0)
	lava := w.raise(t, master, rhwLavaID, 3000, 0, script.Params{})
	me := w.charOf(t)
	strike := map[modelskill.ID]ind1Cast{rhwDDMagic: {me.ObjectID(), 1000000}}

	// The character coming in sight 100 away draws the strike. Each strike
	// takes 1 s and its desire with it; the one after it is the hook's.
	w.moveBeside(t, lava, 100)
	w.srv.Advance(t, 3*time.Second)
	strikes := 0
	for _, id := range skillUses(w.srv.ReadQueued(t, w.srv.Client), lava.ObjectID()) {
		if id == rhwDDMagic {
			strikes++
		}
	}
	if strikes < 2 {
		t.Fatalf("strikes in the 3 s after it saw the character 100 away = %d, want 2 or more", strikes)
	}

	w.moveBeside(t, lava, 550)
	w.castOnceAt(t, lava, me, rhwDebuffA)
	w.wantCasts(t, "after a cast on the character 550 away", lava, nil)
	// An attack desire keeps the character its top desire target until the
	// check.
	w.moveBeside(t, lava, 100)
	w.clearDesires(t, lava)
	w.onQueue(t, lava, func() { script.NPCOf(lava).AddAttackDesire(me, 1) })
	w.srv.Advance(t, time.Second)
	w.wantCasts(t, "a second later, the character 100 away", lava, strike)
}
