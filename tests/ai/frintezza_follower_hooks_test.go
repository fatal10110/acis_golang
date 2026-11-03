package ai

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"testing"
	"time"

	gamesql "github.com/fatal10110/acis_golang/internal/gameserver/data/sql"
	"github.com/fatal10110/acis_golang/internal/gameserver/data/sql/sqltest"
	actorai "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/ai"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	skillstate "github.com/fatal10110/acis_golang/internal/gameserver/skill"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// frSpiritCall is skill 5008 at level 2, the call of the evil spirits the
// follower answers: a good skill here, so a cast of it on the follower
// reaches its spelled hook.
func frSpiritCall() modelskill.Definition {
	return modelskill.Definition{
		ID: 5008, Level: 2, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "BUFF", CastRange: 1200, HitTime: 500, StaticHitTime: true, StaticReuse: true,
	}
}

// frSpellBoot has the character know frSpiritCall.
func frSpellBoot(t *testing.T) raidBoot {
	t.Helper()
	db := sqltest.SharedDB(t)
	def := frSpiritCall()
	table := modelskill.NewTable([]modelskill.Definition{{ID: 248, Level: 3}, {ID: 294, Level: 1}, def})
	return raidBoot{
		opts: []gameservertest.Option{
			gameservertest.WithSkills(skillstate.NewPersistence(gamesql.NewSkillSaveStore(db), table, gamesql.NewCharacterSkillStore(db))),
			gameservertest.WithScriptSkills(def),
		},
		ready: func(srv *gameservertest.Server, objID int32) {
			if err := srv.KnownSkills.SetKnownSkill(context.Background(), objID, 0, int(def.ID), def.Level); err != nil {
				t.Fatalf("seed skill %d: %v", def.ID, err)
			}
		},
	}
}

// me returns the character as a script creature.
func (w *frWorld) me(t *testing.T) script.Creature {
	t.Helper()
	var me script.Creature
	w.srv.RunScript(t, w.player.ObjectID(), frDirector, func(_ *script.Script, p *script.Player) { me = p })
	return me
}

// scoreMe makes the character the only creature h scores: score in its
// second slot.
func (w *frWorld) scoreMe(t *testing.T, h *npc.Hostile, score int32) {
	t.Helper()
	n := script.NPCOf(h)
	me := w.me(t)
	w.onQueue(t, h, func() {
		n.SetInt(npc.IntQuest2, score)
		n.SetSlot(npc.CreatureQuest2, me)
		n.SetInt(npc.IntQuest3, 0)
		n.SetSlot(npc.CreatureQuest3, nil)
		n.SetInt(npc.IntQuest4, 0)
		n.SetSlot(npc.CreatureQuest4, nil)
	})
}

// frCastDesires renders h's queued cast desires as "skill-level>target"
// with their weights.
func frCastDesires(h *npc.Hostile) []string {
	var out []string
	for _, d := range ind8Desires(h, actorai.IntentionCast) {
		out = append(out, fmt.Sprintf("%d-%d>%d=%v", d.Skill.ID, d.Skill.Level, d.FinalTarget.ObjectID(), d.Weight))
	}
	slices.Sort(out)
	return out
}

// frCasts renders the casts the MagicSkillUse frames show h start, in
// order, as "skill-level>target".
func frCasts(frames [][]byte, h *npc.Hostile) []string {
	var out []string
	for _, f := range framesOf(frames, serverpackets.OpcodeMagicSkillUse, h.ObjectID()) {
		target := int32(binary.LittleEndian.Uint32(f[5:9]))
		id, level := binary.LittleEndian.Uint32(f[9:13]), binary.LittleEndian.Uint32(f[13:17])
		out = append(out, fmt.Sprintf("%d-%d>%d", id, level, target))
	}
	return out
}

// TestFollowerOfFrintezzaAnswersTheSpiritCall runs the spelled hook and the
// dash chain at stage 6, every draw 99. The character's spirit call (5008
// level 2) on the follower runs maker event 1001 on both evil spirit
// makers, sets i_ai2 1 and has it attack the character it scores: the
// first dash (5015 level 2) at 10000, unchecked. Each cast's end runs the
// skill-finished hook, which attacks again: a second dash, then the dash
// at every creature around (5015 level 4), which ends the chain at i_ai2 0;
// past it the follower draws 99 of 10000, under the no-charge case, and
// casts nothing more.
func TestFollowerOfFrintezzaAnswersTheSpiritCall(t *testing.T) {
	t.Parallel()
	w := bootFollowerWith(t, frLiveRow(6), "GrandBoss", frSpellBoot(t))
	f := w.follower(t)
	n := script.NPCOf(f)
	w.draws.set(99)
	// Its 4000 timer drops the body change it wanted on its restore.
	w.srv.Advance(t, 5*time.Second)
	w.makers.take()
	w.scoreMe(t, f, 5000)
	w.selectTarget(t, f)
	w.cast(t, 5008)
	if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1001", "frintessa_evilate_maker2 1001"}; !slices.Equal(got, want) {
		t.Fatalf("maker events on the spirit call = %q, want %q", got, want)
	}
	me := w.player.ObjectID()
	if got, want := frCastDesires(f), []string{fmt.Sprintf("5015-2>%d=10000", me)}; !slices.Equal(got, want) {
		t.Fatalf("cast desires on the spirit call = %q, want %q", got, want)
	}
	if got := n.Int(script.SlotIntAI2); got != 2 {
		t.Fatalf("i_ai2 after the first dash was wanted = %d, want 2", got)
	}

	var frames [][]byte
	w.srv.AdvanceUntil(t, "the dash chain", func() bool {
		frames = append(frames, w.ticks(t, 1)...)
		frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
		return len(frCasts(frames, f)) >= 3 && !f.CastingNow()
	})
	for range 5 {
		frames = append(frames, w.ticks(t, 1)...)
	}
	w.srv.Advance(t, 3*time.Second)
	frames = append(frames, w.srv.ReadQueued(t, w.srv.Client)...)
	want := []string{fmt.Sprintf("5015-2>%d", me), fmt.Sprintf("5015-2>%d", me), fmt.Sprintf("5015-4>%d", me)}
	if got := frCasts(frames, f); !slices.Equal(got, want) {
		t.Fatalf("casts of the dash chain = %q, want %q", got, want)
	}
	if got := n.Int(script.SlotIntAI2); got != 0 {
		t.Fatalf("i_ai2 after the chain = %d, want 0", got)
	}
}

// TestFollowerOfFrintezzaAnswersTheSpiritCallHit runs the attacked hook's
// spirit call branch: a hit by 5008 level 2 at stage 6 runs maker event
// 1001 on both evil spirit makers and sets i_ai2 1, so the attack it then
// chooses on the character the hit scored is the first dash; at stage 5
// the same hit runs the maker events and leaves i_ai2 0.
func TestFollowerOfFrintezzaAnswersTheSpiritCallHit(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		stage   int
		ai2     int32
		desires func(me int32) []string
	}{
		{6, 2, func(me int32) []string { return []string{fmt.Sprintf("5015-2>%d=10000", me)} }},
		{5, 0, func(int32) []string { return nil }},
	} {
		t.Run(fmt.Sprintf("stage %d", row.stage), func(t *testing.T) {
			t.Parallel()
			w := bootFollower(t, frLiveRow(row.stage), "GrandBoss")
			f := w.follower(t)
			n := script.NPCOf(f)
			w.draws.set(99)
			w.srv.Advance(t, 5*time.Second)
			w.makers.take()
			done := make(chan struct{})
			if !w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
				defer close(done)
				f.ReduceHP(10, w.player, modelskill.Definition{ID: 5008, Level: 2})
			}) {
				t.Fatal("the character's queue is closed")
			}
			<-done
			w.srv.Settle(t)
			if got, want := w.makers.take(), []string{"frintessa_evilate_maker1 1001", "frintessa_evilate_maker2 1001"}; !slices.Equal(got, want) {
				t.Fatalf("maker events on the spirit call hit = %q, want %q", got, want)
			}
			if got := n.Int(script.SlotIntAI2); got != row.ai2 {
				t.Fatalf("i_ai2 after the hit = %d, want %d", got, row.ai2)
			}
			if got, want := frCastDesires(f), row.desires(w.player.ObjectID()); !slices.Equal(got, want) {
				t.Fatalf("cast desires after the hit = %q, want %q", got, want)
			}
		})
	}
}

// TestFollowerOfFrintezzaLooksAroundWithNothingToDo runs the no-desire
// hook at stage 5, 35 s after the restore (a look around takes at most one
// every 30 s), every draw 0. AI ticks with a desire queued see nothing
// new: the character standing still in front of the grand boss scores
// nothing. With nothing to do, the ticks' no-desire hook looks around, and
// the character it sees again takes the second slot, scoring 1000 plus a
// draw of 0, which the attack choice's draw of 0 of 100 then drops to 500. At stage 0 the no-desire hook drops every desire and hate:
// the 100 hate the follower held of the character is 0 after the tick.
func TestFollowerOfFrintezzaLooksAroundWithNothingToDo(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(5), "GrandBoss")
	f := w.follower(t)
	n := script.NPCOf(f)
	w.draws.set(99)
	w.onQueue(t, f, func() { n.AddDoNothingDesire(60, 50) })
	// A look around takes at most one every 30 s.
	w.srv.Advance(t, 35*time.Second)
	w.scoreMe(t, f, 0)
	w.onQueue(t, f, func() { n.SetSlot(npc.CreatureQuest2, nil) })
	w.draws.set(0)
	w.ticks(t, 5)
	for _, slot := range []script.IntSlot{npc.IntQuest2, npc.IntQuest3, npc.IntQuest4} {
		if got := n.Int(slot); got != 0 {
			t.Fatalf("score slot %d after ticks with a desire queued = %d, want 0", slot, got)
		}
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.ticks(t, 5)
	if got := n.Int(npc.IntQuest2); got != 500 {
		t.Fatalf("second score after ticks with nothing to do = %d, want 500", got)
	}
	if c := n.Slot(npc.CreatureQuest2); c == nil || c.ObjectID() != w.player.ObjectID() {
		t.Fatalf("second slot = %v, want the character", c)
	}

	me := w.me(t)
	w.onQueue(t, f, func() {
		f.AI().Desires().Clear()
		n.SetDBValue(0)
		n.HateList().Add(me, 100)
	})
	if got := f.AI().Hates().Hate(w.player); got != 100 {
		t.Fatalf("setup: hate of the character = %v, want 100", got)
	}
	w.ticks(t, 5)
	if got := f.AI().Hates().Hate(w.player); got != 0 {
		t.Fatalf("hate of the character after ticks at stage 0 = %v, want 0", got)
	}
}

// TestFollowerOfFrintezzaPlaysWithTheNPCsItSees runs the see-creature
// hook on NPCs, every draw 0: at stage 6, looking around with nothing to
// do 35 s after the restore, it tells each listener it sees event 0 with
// 20000 and plays social action 4; at stage 5 the same draw does nothing.
func TestFollowerOfFrintezzaPlaysWithTheNPCsItSees(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		stage   int
		heard   []string
		socials []int32
	}{
		{6, []string{"29993 event=0 arg1=20000 arg2=0", "29994 event=0 arg1=20000 arg2=0"}, []int32{4}},
		{5, nil, nil},
	} {
		t.Run(fmt.Sprintf("stage %d", row.stage), func(t *testing.T) {
			t.Parallel()
			w := bootFollower(t, frLiveRow(row.stage), "GrandBoss")
			f := w.follower(t)
			// A look around takes at most one every 30 s.
			w.srv.Advance(t, 35*time.Second)
			w.onQueue(t, f, func() { f.AI().Desires().Clear() })
			w.heard.take()
			w.srv.ReadQueued(t, w.srv.Client)
			frames := append(w.ticks(t, 5), w.srv.ReadQueued(t, w.srv.Client)...)
			if got := slices.Sorted(slices.Values(w.heard.take())); !slices.Equal(got, row.heard) {
				t.Fatalf("heard = %q, want %q", got, row.heard)
			}
			if got := ind8Socials(frames, f.ObjectID()); !slices.Equal(got, row.socials) {
				t.Fatalf("social actions = %v, want %v", got, row.socials)
			}
		})
	}
}

// TestFollowerOfFrintezzaHindersARider runs the attacked hook's strider
// check at stage 5, every draw 99: a hit by the character riding a strider,
// holding no Hinder Strider, makes the follower want Hinder Strider (4258
// level 1) on it at 1000000.
func TestFollowerOfFrintezzaHindersARider(t *testing.T) {
	t.Parallel()
	w := bootFollower(t, frLiveRow(5), "GrandBoss")
	f := w.follower(t)
	w.draws.set(99)
	obj, ok := w.srv.State.Player(w.player.ObjectID())
	if !ok {
		t.Fatal("the character is not in the world")
	}
	rider, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("the character is %T", obj)
	}
	done := make(chan struct{})
	w.srv.PlayerQueue(t, w.player.ObjectID()).Post(func() {
		defer close(done)
		if !rider.Mount(12526, 1) {
			t.Error("the character did not mount")
		}
	})
	<-done
	w.srv.Settle(t)
	w.srv.ReadQueued(t, w.srv.Client)
	if !script.Riding(w.me(t)) {
		t.Fatal("setup: the character is not riding")
	}
	w.onQueue(t, f, func() { f.AI().Desires().Clear() })
	w.hit(t, f, 1)
	if got, want := frCastDesires(f), []string{fmt.Sprintf("4258-1>%d=1e+06", w.player.ObjectID())}; !slices.Equal(got, want) {
		t.Fatalf("cast desires after the rider's hit = %q, want %q", got, want)
	}
}
