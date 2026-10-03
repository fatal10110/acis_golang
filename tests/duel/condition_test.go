package duel

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	"github.com/fatal10110/acis_golang/internal/gameserver/duel"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Reference: PlayerCondition (storeEffect(true) at the start,
// stopAllEffects + restoreEffects at the end), Player.storeEffect's early
// return while isInDuel (Player.java:4392), Player.restoreEffects (reads,
// then deletes, the saved rows).

const (
	// buffBefore is cast before the duel, buffDuring while it runs.
	buffBefore = 1204
	buffDuring = 1040
)

// conditionArena boots a challenger and a rival whose challenger knows
// both buffs, with saved effects kept across sessions.
func conditionArena(t *testing.T) *arena {
	t.Helper()
	buff := func(id modelskill.ID) modelskill.Definition {
		return modelskill.Definition{
			ID: id, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetSelf,
			HitTime: 500, StaticHitTime: true, MPConsume: 1, SkillType: "BUFF",
			Effects: []modelskill.EffectTemplate{{Name: "Buff", Count: 2, Time: 300}},
		}
	}
	db := sqltest.SharedDB(t)
	skills := skillstate.NewPersistence(gamesql.NewSkillSaveStore(db),
		modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, buff(buffBefore), buff(buffDuring)}),
		gamesql.NewCharacterSkillStore(db))
	return bootArenaWith(t, []gameservertest.Option{gameservertest.WithSkills(skills), gameservertest.WithCapturedLog()},
		func(a *arena) {
			for _, id := range []int{buffBefore, buffDuring} {
				if err := a.srv.KnownSkills.SetKnownSkill(context.Background(), a.players[0].id, 0, id, 1); err != nil {
					t.Fatalf("seed known skill %d: %v", id, err)
				}
			}
		}, "Challenger", "Rival")
}

// effectState is one effect's saved shape: ticks left and seconds into
// its current tick.
type effectState struct{ count, elapsed int32 }

// effectsOf returns player i's effects by skill, read on its queue at its
// clock.
func (a *arena) effectsOf(t *testing.T, i int) map[modelskill.ID]effectState {
	t.Helper()
	obj, ok := a.srv.State.Player(a.players[i].id)
	if !ok {
		t.Fatalf("player %d not in world", a.players[i].id)
	}
	holder := obj.(interface {
		EffectList() *effect.List
		Now() time.Time
	})
	out := map[modelskill.ID]effectState{}
	a.onQueue(t, i, func() {
		now := holder.Now()
		for _, e := range holder.EffectList().All() {
			count, elapsed := e.SaveState(now)
			out[e.Skill.ID] = effectState{count, elapsed}
		}
	})
	return out
}

// cast has player i cast skill id on itself and waits for its effect.
func (a *arena) cast(t *testing.T, i int, id modelskill.ID) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestMagicSkillUse)
	w.WriteInt32(int32(id))
	w.WriteInt32(0)
	w.WriteUint8(0)
	a.players[i].c.Send(w.Bytes())
	a.srv.AdvanceUntil(t, "buff lands", func() bool {
		_, ok := a.effectsOf(t, i)[id]
		return ok
	})
	a.quiet(t)
}

// savedRows counts the character's character_skills_save rows for skill
// id, or for every skill when id is 0.
func (a *arena) savedRows(t *testing.T, i int, id modelskill.ID) int {
	t.Helper()
	a.srv.FlushPersistence(t)
	q, args := "SELECT COUNT(*) FROM character_skills_save WHERE char_obj_id = ?", []any{a.players[i].id}
	if id != 0 {
		q, args = q+" AND skill_id = ?", append(args, int32(id))
	}
	var n int
	if err := a.srv.DB.QueryRowContext(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// TestDuelRestoresEffectsAtEnd: the duel's start saves the challenger's
// effects; during the duel one is lost and another gained. Its end puts
// back the saved effect with the time it had left at the start, drops the
// one gained, and deletes the saved rows.
func TestDuelRestoresEffectsAtEnd(t *testing.T) {
	t.Parallel()
	a := conditionArena(t)
	a.cast(t, 0, buffBefore)
	a.challenge(t, 0, 1)
	start := a.effectsOf(t, 0)[buffBefore]
	if got := a.savedRows(t, 0, buffBefore); got != 1 {
		t.Fatalf("saved rows for the duel-start buff = %d, want 1", got)
	}

	obj, _ := a.srv.State.Player(a.players[0].id)
	a.onQueue(t, 0, obj.(interface{ StopAllEffects() }).StopAllEffects)
	a.cast(t, 0, buffDuring)
	if _, ok := a.effectsOf(t, 0)[buffBefore]; ok {
		t.Fatal("the duel-start buff survived its removal")
	}
	a.srv.Advance(t, 5*time.Second)

	a.players[1].c.Send(encodeDuelSurrender())
	a.srv.AdvanceUntil(t, "duel ends", func() bool {
		return a.standing(t, 0).DuelID() == 0 && a.standing(t, 1).DuelID() == 0
	})
	got := a.effectsOf(t, 0)
	back, ok := got[buffBefore]
	if !ok {
		t.Fatal("the duel-start buff was not put back")
	}
	if _, ok := got[buffDuring]; ok {
		t.Fatal("the buff gained during the duel survived its end")
	}
	// Put back with what it had left at the start, not run on through the
	// duel: the 5 s advanced above would otherwise show.
	if back.count != start.count || back.elapsed > start.elapsed+1 {
		t.Fatalf("restored buff = %+v, want the %+v it had when the duel began", back, start)
	}
	if n := a.savedRows(t, 0, 0); n != 0 {
		t.Fatalf("character_skills_save rows after the duel = %d, want 0", n)
	}
}

// TestDuelRelogKeepsStartEffects: a logout during a duel saves nothing over
// the rows the duel's start wrote, so the next login gets back the effects
// of the duel's start, not those of the moment it left.
func TestDuelRelogKeepsStartEffects(t *testing.T) {
	t.Parallel()
	a := conditionArena(t)
	a.cast(t, 0, buffBefore)
	a.challenge(t, 0, 1)
	a.cast(t, 0, buffDuring)

	ch := a.players[0]
	ch.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	ch.c.ReadWithTimeout(quiet) // LeaveWorld
	a.srv.AdvanceUntil(t, "challenger out of the world", func() bool {
		_, ok := a.srv.State.Player(ch.id)
		return !ok
	})
	if got, during := a.savedRows(t, 0, buffBefore), a.savedRows(t, 0, buffDuring); got != 1 || during != 0 {
		t.Fatalf("saved rows after a mid-duel logout: start buff %d, duel buff %d; want 1, 0", got, during)
	}
	a.srv.AdvanceUntil(t, "duel ends for the rival", func() bool { return a.standing(t, 1).DuelID() == 0 })

	a.players[0].c = a.srv.DialClient(t, a.srv.Account(), 1)
	startInWorld(t, a.players[0].c)
	got := a.effectsOf(t, 0)
	if _, ok := got[buffBefore]; !ok {
		t.Fatal("the duel-start buff did not come back at login")
	}
	if _, ok := got[buffDuring]; ok {
		t.Fatal("the buff gained during the duel came back at login")
	}
	if a.standing(t, 0).InDuel() || a.standing(t, 0).DuelState() != duel.NoDuel {
		t.Fatal("the relogged challenger is still in a duel")
	}
}
