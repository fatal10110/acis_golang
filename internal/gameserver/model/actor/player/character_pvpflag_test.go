package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable/attackabletest"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// ---- from character_pvpflag_test.go ----
func TestUpdatePvPFlagRefreshesUserInfoOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	rec := recordEvents(c)

	c.UpdatePvPFlag(task.PvPFlagOn)
	if got := c.PvPFlagState(); got != task.PvPFlagOn {
		t.Fatalf("PvPFlagState() = %v, want PvPFlagOn", got)
	}
	if updates := event.Count[event.UserInfoChanged](rec); updates != 1 {
		t.Fatalf("updates after first change = %d, want 1", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if updates := event.Count[event.UserInfoChanged](rec); updates != 1 {
		t.Fatalf("updates after unchanged state = %d, want 1 (no-op)", updates)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if updates := event.Count[event.UserInfoChanged](rec); updates != 2 {
		t.Fatalf("updates after second change = %d, want 2", updates)
	}
}

func TestUpdatePvPFlagBroadcastsRelationsOnlyOnChange(t *testing.T) {
	c := &Character{ID: 1}
	rec := recordEvents(c)

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts := event.Count[event.RelationChanged](rec); broadcasts != 1 {
		t.Fatalf("broadcasts after first change = %d, want 1", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagOn)
	if broadcasts := event.Count[event.RelationChanged](rec); broadcasts != 1 {
		t.Fatalf("broadcasts after unchanged state = %d, want 1 (no-op)", broadcasts)
	}

	c.UpdatePvPFlag(task.PvPFlagBlinking)
	if broadcasts := event.Count[event.RelationChanged](rec); broadcasts != 2 {
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
	rec := recordEvents(attacker)

	victim.notePvPHitFromAttacker(attacker, false)
	calls := pvpFlagCalls(rec)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false] (normal duration)", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsMutualPvPZone(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	attacker.SetInPvPZone(true)
	victim.SetInPvPZone(true)
	rec := recordEvents(attacker)

	victim.notePvPHitFromAttacker(attacker, false)
	called := event.Count[event.PvPFlagged](rec) > 0

	if called {
		t.Fatal("hook fired for two PvP-zone players")
	}
}

type pvpFlagNPC struct {
	attackabletest.Combatant
	guard bool
}

func (pvpFlagNPC) ObjectID() int32 { return 4 }
func (n pvpFlagNPC) Guard() bool   { return n.guard }

// Attackable reports the attackable NPC a helpful skill flags its caster on.
func (pvpFlagNPC) Attackable() bool { return true }

func TestNotePvPHitFromAttackerUsesFlaggedDurationForOngoingPvPFight(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	rec := recordEvents(attacker)

	victim.notePvPHitFromAttacker(attacker, false)
	calls := pvpFlagCalls(rec)

	if len(calls) != 1 || calls[0] != true {
		t.Fatalf("hook calls = %v, want [true] (PvP-vs-PvP duration)", calls)
	}
}

func TestNotePvPHitFromAttackerUsesNormalDurationWhenAttackerHasKarma(t *testing.T) {
	attacker := &Character{ID: 1, KarmaPoints: 500}
	victim := &Character{ID: 2}
	victim.pvpFlag = task.PvPFlagOn
	rec := recordEvents(attacker)

	victim.notePvPHitFromAttacker(attacker, false)
	calls := pvpFlagCalls(rec)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls = %v, want [false]: a karma'd attacker never gets the PvP-vs-PvP duration", calls)
	}
}

func TestNotePvPHitFromAttackerSkipsWhenVictimHasKarma(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2, KarmaPoints: 500}
	rec := recordEvents(attacker)

	victim.notePvPHitFromAttacker(attacker, false)
	called := event.Count[event.PvPFlagged](rec) > 0

	if called {
		t.Fatal("hook fired, want no-op: hitting a karma'd (PK) victim never flags the attacker")
	}
}

func TestNotePvPHitFromAttackerSkipsNonPlayerAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	// Should not panic and should be a no-op for a non-*Character attacker.
	victim.notePvPHitFromAttacker(npcKiller{id: 99}, false)
}

func TestNotePvPHitFromAttackerSkipsNilAttacker(t *testing.T) {
	victim := &Character{ID: 2}

	victim.notePvPHitFromAttacker(nil, false)
}

func TestNotePvPHitFromAttackerSkipsSelfHit(t *testing.T) {
	c := &Character{ID: 1}
	rec := recordEvents(c)

	c.notePvPHitFromAttacker(c, false)
	called := event.Count[event.PvPFlagged](rec) > 0

	if called {
		t.Fatal("hook fired, want no-op: self-damage never flags the actor")
	}
}

func TestNotePvPHitFromAttackerNoopWithoutHook(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}

	// Should not panic when no hook is wired (e.g. character not attached
	// to a live session).
	victim.notePvPHitFromAttacker(attacker, false)
}

func TestCharacterNotePvPAttackFlagsInnocentVictim(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	victim := liveCharacter(2, tmpl, items)
	rec := recordEvents(attacker)

	attacker.NotePvPAttack(victim)
	calls := pvpFlagCalls(rec)

	if len(calls) != 1 || calls[0] != false {
		t.Fatalf("hook calls after NotePvPAttack = %v, want [false]", calls)
	}
}

func TestCharacterNotePvPAttackFlagsOwnerOfSummonedTarget(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	rec := recordEvents(attacker)

	attacker.NotePvPAttack(summonKiller{owner: victim})
	calls := pvpFlagCalls(rec)

	if len(calls) != 1 || calls[0] {
		t.Fatalf("hook calls after summoned target = %v, want [false]", calls)
	}
}

func TestCharacterNoteServitorPvPMarksTheFlagAsTheSummons(t *testing.T) {
	owner := &Character{ID: 1}
	victim := &Character{ID: 2}
	rec := recordEvents(owner)

	owner.NoteServitorPvPAttack(victim)
	owner.NoteServitorPvPSkillTargets([]attackable.Combatant{victim}, true, "PDAM")
	got := event.Of[event.PvPFlagged](rec)

	want := event.PvPFlagged{ByServitor: true}
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Fatalf("PvPFlagged after the summon's hit and skill = %+v, want two %+v", got, want)
	}
}

func TestCharacterNoteServitorPvPIgnoresHitsOnTheOwnerAndItsSummon(t *testing.T) {
	owner := &Character{ID: 1}
	rec := recordEvents(owner)

	owner.NoteServitorPvPAttack(owner)
	owner.NoteServitorPvPAttack(summonKiller{owner: owner})
	owner.NoteServitorPvPSkillTargets([]attackable.Combatant{owner, summonKiller{owner: owner}}, true, "PDAM")

	if n := event.Count[event.PvPFlagged](rec); n != 0 {
		t.Fatalf("PvPFlagged count = %d after the summon hit its owner and itself, want 0", n)
	}
}

func TestCharacterNoteServitorPvPNonOffensiveSkillMarksTheFlagAsTheSummons(t *testing.T) {
	owner := &Character{ID: 1}
	flagged := &Character{ID: 2}
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	pker := &Character{ID: 3, KarmaPoints: 500}
	rec := recordEvents(owner)

	owner.NoteServitorPvPSkillTargets([]attackable.Combatant{flagged}, false, "BUFF")
	owner.NoteServitorPvPSkillTargets([]attackable.Combatant{pker}, false, "HEAL")
	got := event.Of[event.PvPFlagged](rec)

	want := event.PvPFlagged{ByServitor: true}
	if len(got) != 2 || got[0] != want || got[1] != want {
		t.Fatalf("PvPFlagged after the summon buffed a flagged and a karma'd player = %+v, want two %+v", got, want)
	}
}

func TestCharacterNoteServitorPvPNonOffensiveSkillIgnoresTheOwnerAndItsSummon(t *testing.T) {
	owner := &Character{ID: 1, KarmaPoints: 500}
	owner.UpdatePvPFlag(task.PvPFlagOn)
	rec := recordEvents(owner)

	owner.NoteServitorPvPSkillTargets([]attackable.Combatant{owner, summonKiller{owner: owner}}, false, "BUFF")

	if n := event.Count[event.PvPFlagged](rec); n != 0 {
		t.Fatalf("PvPFlagged count = %d after the summon buffed its flagged, karma'd owner and itself, want 0", n)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsEligibleNonOffensiveTargets(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items)
	flagged := liveCharacter(2, tmpl, items)
	flagged.UpdatePvPFlag(task.PvPFlagOn)
	rec := recordEvents(attacker)

	attacker.NotePvPSkillTargets([]attackable.Combatant{flagged, pvpFlagNPC{}}, false, "DUMMY")
	calls := pvpFlagCalls(rec)

	if len(calls) != 2 || calls[0] || calls[1] {
		t.Fatalf("hook calls after NotePvPSkillTargets = %v, want [false false]", calls)
	}
}

func TestCharacterNotePvPSkillTargetsFlagsOwnerOfFlaggedSummon(t *testing.T) {
	attacker := &Character{ID: 1}
	victim := &Character{ID: 2}
	victim.UpdatePvPFlag(task.PvPFlagOn)
	rec := recordEvents(attacker)

	attacker.NotePvPSkillTargets([]attackable.Combatant{summonKiller{owner: victim}}, false, "DUMMY")
	called := event.Count[event.PvPFlagged](rec) > 0

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
	rec := recordEvents(attacker)

	victim.ReduceHPByDOT(10, attacker, true)
	called := event.Count[event.PvPFlagged](rec) > 0

	if called {
		t.Fatal("hook fired on ReduceHPByDOT, want no-op: DOT ticks don't re-flag the attacker")
	}
}

// npcKiller is a minimal non-player DeathActor double.
type npcKiller struct{ id int32 }

func (k npcKiller) ObjectID() int32 { return k.id }

type summonKiller struct {
	attackabletest.Combatant
	owner attackable.Combatant
}

func (k summonKiller) ObjectID() int32                     { return 3 }
func (k summonKiller) Kind() actor.Kind                    { return actor.KindSummon }
func (k summonKiller) Owner() (attackable.Combatant, bool) { return k.owner, k.owner != nil }

// pvpFlagCalls returns each PvPFlagged event's duration choice, in order.
func pvpFlagCalls(rec *event.Recorder) []bool {
	var calls []bool
	for _, e := range event.Of[event.PvPFlagged](rec) {
		calls = append(calls, e.UseFlaggedDuration)
	}
	return calls
}

func (pvpFlagNPC) Heading() int { return 0 }

func (pvpFlagNPC) Position() (x, y, z int) { return 0, 0, 0 }

func (summonKiller) Heading() int { return 0 }

func (summonKiller) Position() (x, y, z int) { return 0, 0, 0 }
