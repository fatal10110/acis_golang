package ai

import (
	"context"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestAntharasRanksANewSpellCasterTwice ports the second ranking pass of a
// spell (phase 4), every draw 0, at db value 1, where no cast follows the
// rank. Between 1/2 and 3/4 of its HP, a spell of damage d from a character
// it has not ranked gives it the first slot with trunc(d/100*1000), then,
// ranked once more as a spell at high HP on the standings it just changed,
// the third with trunc(d/150*1000).
func TestAntharasRanksANewSpellCasterTwice(t *testing.T) {
	bolt := modelskill.Definition{
		ID: antharasBoltID, Level: 1, Activation: modelskill.ActivationActive,
		Target: modelskill.TargetOne, Offensive: true, Magic: true, SkillType: "MDAM",
		CastRange: 1500, HitTime: 500, StaticHitTime: true, StaticReuse: true, Power: 40,
	}
	db := sqltest.SharedDB(t)
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, bolt})
	w := bootAntharasWith(t, antharasLiveRow(1), "1hour", []gameservertest.Option{
		gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
		gameservertest.WithScriptSkills(bolt),
	}, func(srv *gameservertest.Server, objID int32) {
		if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(antharasBoltID), 1); err != nil {
			t.Fatalf("seed the spell: %v", err)
		}
	})
	boss := soleBaiumNPC(t, w, antharasID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	for range 2 {
		w.srv.Client.Send(encodeFolkAction(boss.ObjectID(), w.at))
		w.srv.Settle(t)
	}
	w.srv.ReadQueued(t, w.srv.Client)

	w.onQueue(t, boss, func() {
		boss.SetCurrentHP(boss.MaxHP() * 5 / 8)
		clearTargets(handle,
			[]script.CreatureSlot{script.SlotCreature2, script.SlotCreature3, script.SlotCreature4},
			[]script.IntSlot{script.SlotIntAI2, script.SlotIntAI3, script.SlotIntAI4})
	})
	before := boss.CurrentHP()
	w.castFrames(t, antharasBoltID)
	d := before - boss.CurrentHP()
	if d <= 0 {
		t.Fatalf("the spell did %d damage, want some", d)
	}
	if got := handle.Slot(script.SlotCreature2); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Antharas's first target = %v, want the character", got)
	}
	if got, want := handle.Int(script.SlotIntAI2), int32(float64(d)/100.0*1000); got != want {
		t.Fatalf("the character's first points after a spell of %d = %d, want %d", d, got, want)
	}
	if got := handle.Slot(script.SlotCreature4); got == nil || got.ObjectID() != w.player.ObjectID() {
		t.Fatalf("Antharas's third target = %v, want the character", got)
	}
	if got, want := handle.Int(script.SlotIntAI4), int32(float64(d)/150.0*1000); got != want {
		t.Fatalf("the character's third points after a spell of %d = %d, want %d", d, got, want)
	}
}

// baiumTargets are Baium's three target slots and their points.
var (
	baiumTargetSlots = []script.CreatureSlot{npc.CreatureQuest0, npc.CreatureQuest1, npc.CreatureQuest2}
	baiumTargetInts  = []script.IntSlot{npc.IntQuest0, npc.IntQuest1, npc.IntQuest2}
)

// clearTargets empties a boss's target slots and zeroes their points, so
// the next creature it ranks is a new one.
func clearTargets(h *script.NPC, who []script.CreatureSlot, pts []script.IntSlot) {
	for i := range who {
		h.SetSlot(who[i], nil)
		h.SetInt(pts[i], 0)
	}
}

// wanderingBaium boots Baium's lair, lets Baium wake and wander, then
// empties its three targets: the character and the archangels it saw meanwhile
// are unranked again.
func wanderingBaium(t *testing.T) (*baiumWorld, *npc.Hostile, *script.NPC) {
	t.Helper()
	w := bootBaium(t, "116033;17447;10107;0")
	w.advance(t, 5*time.Second)
	boss := soleBaiumNPC(t, w, baiumID).(*npc.Hostile)
	handle := script.NPCOf(boss)
	for i := 0; boss.AI().CurrentIntention() != actorai.IntentionWander; i++ {
		if i == 10 {
			t.Fatalf("Baium's intention after 10 AI ticks = %v, want wander", boss.AI().CurrentIntention())
		}
		w.ticks(t, 1)
	}
	w.onQueue(t, boss, func() {
		boss.AI().Desires().Clear()
		clearTargets(handle, baiumTargetSlots, baiumTargetInts)
	})
	return w, boss, handle
}

// wantBaiumTargets fails unless Baium's three targets are the objects want
// (0 for an empty slot) with points.
func wantBaiumTargets(t *testing.T, step string, h *script.NPC, want []int32, points []int32) {
	t.Helper()
	for i := range baiumTargetSlots {
		var id int32
		if got := h.Slot(baiumTargetSlots[i]); got != nil {
			id = got.ObjectID()
		}
		if id != want[i] {
			t.Errorf("%s: Baium's target %d = object %d, want %d", step, i+1, id, want[i])
		}
		if p := h.Int(baiumTargetInts[i]); p != points[i] {
			t.Errorf("%s: Baium's target %d points = %d, want %d", step, i+1, p, points[i])
		}
	}
}

// TestBaiumRanksANewAttackerTwice ports the second ranking pass of a hit
// (phase 4), every draw 0 (every even draw true): awake, wandering and with
// no target, a plain hit of 300 from the character gives it the first slot
// with 300000, then, ranked once more as a spell at high HP on the
// standings it just changed, the third with (300/150)*1000 = 2000. Its cast
// then drops the first, the best, to 500.
func TestBaiumRanksANewAttackerTwice(t *testing.T) {
	w, boss, handle := wanderingBaium(t)
	pc := w.player.ObjectID()

	w.hit(t, boss, 300)
	wantBaiumTargets(t, "after the hit", handle, []int32{pc, 0, pc}, []int32{500, 0, 2000})
}

// TestBaiumRanksASeenPriestTwice ports the second ranking pass of a
// sighting (phase 4), every draw 0 (every even draw true): awake, wandering,
// with no target and at full HP, Baium seeing a fighter ranks it once, into
// the first slot with 1000, which its cast drops to 500. Seeing a priest
// ranks it into the first slot with 2000, then once more with the points of
// anyone seen into the third with 1000; its cast drops the first to 500.
func TestBaiumRanksASeenPriestTwice(t *testing.T) {
	w, boss, handle := wanderingBaium(t)
	pc := w.player.ObjectID()
	see := func() {
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

	see()
	wantBaiumTargets(t, "after seeing a fighter", handle, []int32{pc, 0, 0}, []int32{500, 0, 0})

	w.onQueue(t, boss, func() {
		boss.AI().Desires().Clear()
		clearTargets(handle, baiumTargetSlots, baiumTargetInts)
	})
	w.onBaiumCharacter(t, func(c *player.Character) { c.SetClassID(15) }) // Cleric
	see()
	wantBaiumTargets(t, "after seeing a priest", handle, []int32{pc, 0, pc}, []int32{500, 0, 1000})
}
