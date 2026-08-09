package player

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// TestCharacterTakeDamageFlagsAttackerOnInnocentHit is the end-to-end
// regression test for the #1249 gap: a live physical hit against a
// karma-free victim must reach the attacker's registered PvP-flag hook, not
// just the lower-level notePvPHitFromAttacker helper.
func TestCharacterTakeDamageFlagsAttackerOnInnocentHit(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.TakeDamage(10, attacker)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls after TakeDamage = %v, want [false]", calls)
	}
}

// TestCharacterReduceHPFlagsAttackerOnInnocentHit mirrors the above for
// skill (magic) damage, which lands on Character.ReduceHP rather than
// TakeDamage.
func TestCharacterReduceHPFlagsAttackerOnInnocentHit(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	var calls []bool
	attacker.SetPvPFlagHook(func(useFlagged bool) { calls = append(calls, useFlagged) })

	victim.ReduceHP(10, attacker, modelskill.Definition{})

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls after ReduceHP = %v, want [false]", calls)
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
