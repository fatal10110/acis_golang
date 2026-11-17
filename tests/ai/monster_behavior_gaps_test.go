package ai

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestDefaultNpcHateRatioAddsTheGroupAndOccupationRatios ports DefaultNpc's
// hate ratio as the hate a hit adds: a player of class 0 (a human fighter)
// that an NPC's SetHateGroup or SetHateOccupation names has the ratio of
// each named class group and class added to its hate, in percent; one that
// is not named adds nothing. The hit is the AttackLowHP retarget of a
// level 1 NPC, which lists the attacker with the damage's hate times 100
// plus 300.
func TestDefaultNpcHateRatioAddsTheGroupAndOccupationRatios(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		ratio  float64
	}{
		{"no ratio", nil, 0},
		{"its class group", npc.AIParams{"SetHateGroup": "@fighter_group", "SetHateGroupRatio": "50"}, 50},
		{"another class group", npc.AIParams{"SetHateGroup": "@mage_group", "SetHateGroupRatio": "50"}, 0},
		{"its class", npc.AIParams{"SetHateOccupation": "@fighter", "SetHateOccupationRatio": "30"}, 30},
		{"another class", npc.AIParams{"SetHateOccupation": "@mage", "SetHateOccupationRatio": "30"}, 0},
		{"its class group and class", npc.AIParams{
			"SetHateGroup": "@fighter_group", "SetHateGroupRatio": "50", "SetHateOccupation": "@fighter", "SetHateOccupationRatio": "30",
		}, 80},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			params := npc.AIParams{"AttackLowHP": "1"}
			for k, v := range row.params {
				params[k] = v
			}
			w := bootWizardBase(t, wizardBaseTemplate(wizardBaseOnlyID, params, 1, nil), wizardBaseTemplate(wizardFoeID, nil, 1, nil))
			h := w.spawn(t, wizardBaseOnlyID, 150, 0)
			foe := w.spawn(t, wizardFoeID, -150, 0)
			w.listHate(t, h, script.NPCOf(foe), 500)
			w.setPlayerHPRatio(t, 0.2)
			w.draws.set(0)

			w.hit(t, h, 10)
			want := 10.0/8*(1+row.ratio/100)*100 + 300
			if got := hateOf(h, w.player); math.Abs(got-want) > 0.001 {
				t.Fatalf("hate list amount of the character = %v, want %v", got, want)
			}
		})
	}
}

// TestMonsterBehaviorChargesSpiritShots ports the spirit shot recharge of
// MonsterBehavior's attacked hook, the twin of its soul shot recharge: a
// draw under SpiritShotRate charges the NPC's spirit shot on a hit, and
// the SpiritShot parameter must be set.
func TestMonsterBehaviorChargesSpiritShots(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		params npc.AIParams
		want   bool
	}{
		{"rate 100", npc.AIParams{"SpiritShot": "1", "SpiritShotRate": "100"}, true},
		{"no SpiritShot", npc.AIParams{"SpiritShotRate": "100"}, false},
		{"no rate", npc.AIParams{"SpiritShot": "1"}, false},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootBehaviors(t, warriorTemplate(behaviorWarriorID, row.params))
			h := w.spawn(t, behaviorWarriorID, 150, 0)

			w.hit(t, h, 10)
			if got := h.SpiritshotCharged(); got != row.want {
				t.Fatalf("spirit shot charged = %v, want %v", got, row.want)
			}
		})
	}
}

// TestWarriorBehaviorTurnsOnACasterItSeesInCombat ports WarriorBehavior's
// see-spell hook: an NPC in combat that sees a creature cast a spell on
// itself wants to attack the caster with 1000 more; one that is not in
// combat does not.
func TestWarriorBehaviorTurnsOnACasterItSeesInCombat(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name   string
		fought bool
		want   float64
	}{{"in combat", true, 1000}, {"not in combat", false, 0}} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			list, catalog := behaviorScripts()
			tmpl := warriorTemplate(behaviorWarriorID, nil)
			extra := ind1AggroSpellBoot(t)
			extra.opts = append(extra.opts,
				gameservertest.WithScriptRand(func(int) int { return 0 }), gameservertest.WithAttackStanceClock(time.Now),
				gameservertest.WithNPCScripts(map[int32]script.NPCKind{behaviorWarriorID: script.KindHostile}, list, catalog))
			w := &ind1World{warriorWorld: bootRaid(t, extra, tmpl)}
			h := w.spawn(t, behaviorWarriorID, 150, 0)
			if row.fought {
				w.hit(t, h, 10)
				w.onQueue(t, h, h.EnterAttackStance)
				w.srv.AdvanceUntil(t, "the NPC's attack stance", func() bool {
					cycle(t, w.srv)
					return h.InCombat()
				})
			}
			before := 0.0
			if d, ok := attackOn(h, w.player); ok {
				before = d.Weight
			}
			if row.fought && before == 0 {
				t.Fatal("setup: the NPC wants no attack on the character")
			}

			w.castAggroBuff()
			w.srv.Advance(t, 2*time.Second)
			after := 0.0
			if d, ok := attackOn(h, w.player); ok {
				after = d.Weight
			}
			if got := after - before; got != row.want {
				t.Fatalf("attack desire added by the spell = %v (%v to %v), want %v", got, before, after, row.want)
			}
		})
	}
}

// TestDefaultNpcTryToAttackWaitsForItsAggressiveTime ports the aggressive
// time of DefaultNpc's attack on sight, as a WarriorAggressive NPC takes it
// on seeing the character in its territory. With SetAggressiveTime -1 it
// waits until it has lived 3 to 7 AI cycles, by a draw of 5 taken each time
// it sees the character; with a time over 0 it attacks at once as well, since the
// time's own branch and the one after it both want the attack; with 0 it
// attacks at once. Each sight wants 200.
func TestDefaultNpcTryToAttackWaitsForItsAggressiveTime(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name string
		time string
		draw int
		life int32 // its life time when it first attacks: 1 is the first cycle, as soon as the loop looks
	}{
		{"-1, draw 0", "-1", 0, 3},
		{"-1, draw 4", "-1", 4, 7},
		{"5, draw 0", "5", 0, 1},
		{"5, draw 3", "5", 3, 1},
		{"0", "0", 4, 1},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			w := bootIndividual1(t, raidBoot{}, warriorTemplate(aggressiveID, npc.AIParams{"SetAggressiveTime": row.time}))
			h := w.spawn(t, aggressiveID, 150, 0)
			w.draws.set(row.draw)

			// The NPC looks once as it appears, then again each time the
			// character is told to it anew, as a jump of the character does.
			var first int32 = -1
			for i := 0; i < 12 && first < 0; i++ {
				w.ticks(t, 1)
				w.sightCharacterAgain(t)
				if h.AggroHate(w.player) > 0 {
					first = h.LifeTime()
				}
			}
			if first != row.life {
				t.Fatalf("the NPC first hated the character, seen again each cycle, at life time %d, want exactly %d", first, row.life)
			}
			if got := h.AggroHate(w.player); got < 200 || math.Mod(got, 200) != 0 {
				t.Fatalf("hate for the character = %v, want whole wants of 200", got)
			}
		})
	}
}

// sightCharacterAgain tells the NPCs around the character about it once
// more, on its queue.
func (w *warriorWorld) sightCharacterAgain(t *testing.T) {
	t.Helper()
	done := make(chan struct{})
	if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		npc.ForceSeeCreature(w.srv.State, w.player)
	}) {
		t.Fatal("the character's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}
