package player

import (
	"testing"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

func TestUpdatePvPFlagRefreshesUserInfoOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	updates := 0
	c.SetUserInfoUpdater(func() { updates++ })

	c.UpdatePvPFlag(task.PvPFlagOn)
	if got := c.PvPFlagState(); got != task.PvPFlagOn {
		t.Fatalf("PvPFlagState() = %v, want PvPFlagOn", got)
	}
	if updates != 1 {
		t.Fatalf("updates after first change = %d, want 1", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if updates != 1 {
		t.Fatalf("updates after unchanged state = %d, want 1 (no-op)", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if updates != 2 {
		t.Fatalf("updates after second change = %d, want 2", updates)
	}
}

func TestUpdatePvPFlagBroadcastsRelationsOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	broadcasts := 0
	c.SetRelationBroadcaster(func() { broadcasts++ })

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts != 1 {
		t.Fatalf("broadcasts after first change = %d, want 1", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts != 1 {
		t.Fatalf("broadcasts after unchanged state = %d, want 1 (no-op)", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if broadcasts != 2 {
		t.Fatalf("broadcasts after second change = %d, want 2", broadcasts)
	}
}

func TestUpdatePvPFlagNoopWithoutRelationBroadcaster(t *testing.T) {
	c := &Character{ID: 1}

	// Should not panic when no hook is wired.
	c.UpdatePvPFlag(task.PvPFlagOn)
}

func TestNotePvPHitFromAttackerFlagsInnocentVictimHit(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false] (normal duration)", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsMutualPvPZone(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	attacker.SetInPvPZone(true)
	victim.SetInPvPZone(true)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.notePvPHitFromAttacker(attacker)

	if called {
		t.Fatal("hook fired for two PvP-zone players")
	}
}

type pvpFlagNPC struct{ guard bool }

func (pvpFlagNPC) Category() skilltarget.Category { return skilltarget.CategoryAttackable }
func (n pvpFlagNPC) Guard() bool                  { return n.guard }

func TestNotePvPHitFromAttackerUsesFlaggedDurationForOngoingPvPFight(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != true {
		t.Fatalf("hook calls = %v, want [true] (PvP-vs-PvP duration)", calls)
	}
}

func TestNotePvPHitFromAttackerUsesNormalDurationWhenAttackerHasKarma(t *testing.T) {
	attacker := &Character{ID: 1, KarmaPoints: 500}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.notePvPHitFromAttacker(attacker)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false]: a karma'd attacker never gets the PvP-vs-PvP duration", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsWhenVictimHasKarma(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2, KarmaPoints: 500}
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.notePvPHitFromAttacker(attacker)

	if called {
		t.Fatal("hook fired, want no-op: hitting a karma'd (PK) victim never flags the attacker")
	}
}

func TestNotePvPHitFromAttackerSkipsNonPlayerAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	// Should not panic and should be a no-op for a non-*Character attacker.
	victim.notePvPHitFromAttacker(npcKiller{id: 99})
}

func TestNotePvPHitFromAttackerSkipsNilAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	victim.notePvPHitFromAttacker(nil)
}

func TestNotePvPHitFromAttackerSkipsSelfHit(t *testing.T) {
	c := &Character{ID: 1}
	called := false
	c.SetPvPFlagHook(func(bool) { called = true })

	c.notePvPHitFromAttacker(c)

	if called {
		t.Fatal("hook fired, want no-op: self-damage never flags the actor")
	}
}

func TestNotePvPHitFromAttackerNoopWithoutHook(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}

	// Should not panic when no hook is wired (e.g. character not attached
	// to a live session).
	victim.notePvPHitFromAttacker(attacker)
}

func TestCharacterNotePvPAttackFlagsInnocentVictim(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPAttack(victim)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls after NotePvPAttack = %v, want [false]", calls)
	}
}

func TestCharacterNotePvPAttackFlagsOwnerOfSummonedTarget(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPAttack(summonKiller{owner: victim})

	if len(calls) != 1 || calls[0] {
		t.Fatalf("hook calls after summoned target = %v, want [false]", calls)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsEligibleNonOffensiveTargets(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	flagged := liveCharacter(2, tmpl, items)
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	attacker.NotePvPSkillTargets([]any{flagged, pvpFlagNPC{}}, false, "DUMMY")

	if len(calls) != 2 || calls[0] || calls[1] {
		t.Fatalf("hook calls after NotePvPSkillTargets = %v, want [false false]", calls)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsOwnerOfFlaggedSummon(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	attacker.NotePvPSkillTargets([]any{summonKiller{owner: victim}}, false, "DUMMY")

	if !called {
		t.Fatal("non-offensive cast at a flagged summon did not flag its owner")
	}
}

// TestCharacterReduceHPByDOTDoesNotFlagAttacker documents the deliberate
// scope cut: a DOT tick continues a skill cast whose own initial hit
// already flagged the attacker in the reference (CreatureCast.java calls
// updatePvPStatus once, at cast time, not per DOT tick), so periodic damage
// must not re-trigger the flag hook here.
func TestCharacterReduceHPByDOTDoesNotFlagAttacker(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	called := false
	attacker.SetPvPFlagHook(func(bool) { called = true })

	victim.ReduceHPByDOT(10, attacker, true)

	if called {
		t.Fatal("hook fired on ReduceHPByDOT, want no-op: DOT ticks don't re-flag the attacker")
	}
}
