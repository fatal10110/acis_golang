package skills

import (
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	skillhandler "github.com/fatal10110/acis_golang/internal/gameserver/handler/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// onlineCharacter returns the online character objID plays.
func onlineCharacter(t *testing.T, srv *gameservertest.Server, objID int32) *player.Character {
	t.Helper()
	obj, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("player %T is not an online character", obj)
	}
	return pc
}

// raceStackEffect builds an effect of skill id in the race_stack group,
// owned by and landing on pc.
func raceStackEffect(t *testing.T, pc *player.Character, id modelskill.ID, tmpl modelskill.EffectTemplate) *effect.Effect {
	t.Helper()
	tmpl.StackType, tmpl.Icon = "race_stack", true
	e, err := effect.New(effect.Skill{ID: id, Level: 1, SkillType: "BUFF"}, tmpl)
	if err != nil {
		t.Fatalf("effect.New(%d): %v", id, err)
	}
	e.Effector, e.Effected = pc, pc
	return e
}

// TestEffectTickRacesSameStackLanding ticks a player's heal-over-time while
// another goroutine, standing in for a second caster's queue, lands and
// dispels a stronger buff of the same stack group on that player, displacing
// and promoting the heal each time. Run under -race, the tick's read of
// which member is active must be ordered with those writes. Afterwards a
// displaced heal never acts and a promoted one acts on the next tick.
func TestEffectTickRacesSameStackLanding(t *testing.T) {
	t.Parallel()
	const (
		hotID, buffID = 9101, 9102
		heal          = 7
		ticks         = 400
	)
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
		// A displaced member stays held behind the stronger one, so the
		// heal goes back into use each time the stronger buff leaves.
		gameservertest.WithCancelLesserEffect(false),
	)
	skipOnWallClock(t, srv)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pc := onlineCharacter(t, srv, objID)
	list := pc.EffectList()

	hot := raceStackEffect(t, pc, hotID, modelskill.EffectTemplate{Name: "HealOverTime", Value: heal, Count: 1 << 20, Time: 1, StackOrder: 1})
	onPlayerQueue(t, srv, objID, func(*player.Character) { list.Add(hot) })
	// An hour without the effect sweep leaves an hour of heal ticks due, so
	// every sweep below claims one.
	srv.Advance(t, time.Hour)
	stronger := func() *effect.Effect {
		return raceStackEffect(t, pc, buffID, modelskill.EffectTemplate{Name: "Buff", Time: 3600, StackOrder: 2})
	}

	var stop atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() {
		for !stop.Load() {
			buff := stronger()
			list.Add(buff)
			list.Remove(buff)
		}
	})
	for range ticks {
		srv.TickEffects()
	}
	stop.Store(true)
	wg.Wait()
	c.ReadQueued()

	buff := stronger()
	onPlayerQueue(t, srv, objID, func(pc *player.Character) {
		list.Add(buff)
		pc.SetHP(10)
	})
	if hot.InUse() || !buff.InUse() {
		t.Fatalf("after the stronger buff lands: heal in use %v, buff in use %v; want false, true", hot.InUse(), buff.InUse())
	}
	srv.TickEffects()
	if got := pc.CurrentHP(); got != 10 {
		t.Fatalf("HP after a tick with the heal displaced = %d, want 10", got)
	}
	onPlayerQueue(t, srv, objID, func(*player.Character) { list.Remove(buff) })
	if !hot.InUse() {
		t.Fatal("heal not promoted after the stronger buff left")
	}
	srv.TickEffects()
	if got := pc.CurrentHP(); got != 10+heal {
		t.Fatalf("HP after a tick with the heal promoted = %d, want %d", got, 10+heal)
	}
}

// fusionCasterID and fusionForceID mirror the Kabokula channel (3626) and
// the four-level force it triggers (5106), whose stack order is its level.
const (
	fusionCasterID = 3626
	fusionForceID  = 5106
	fusionMaxLevel = 4
)

func fusionDefs() (modelskill.Definition, *modelskill.Table) {
	channel := modelskill.Definition{ID: fusionCasterID, Level: 1, SkillType: "FUSION", Target: modelskill.TargetOne}.
		WithExtra(modelskill.Extra{TriggeredID: fusionForceID, TriggeredLevel: 1})
	defs := []modelskill.Definition{channel}
	for level := 1; level <= fusionMaxLevel; level++ {
		defs = append(defs, modelskill.Definition{
			ID: fusionForceID, Level: level, SkillType: "CONT", Target: modelskill.TargetOne,
			Effects: []modelskill.EffectTemplate{{
				Name: "Fusion", Count: 0x7fffffff, Time: 600, Value: 15, Icon: true,
				StackType: "seizure_a", StackOrder: float64(level),
			}},
		})
	}
	return channel, modelskill.NewTable(defs)
}

// heldOf returns the effects of skill id list holds.
func heldOf(list *effect.List, id modelskill.ID) []*effect.Effect {
	var out []*effect.Effect
	for _, e := range list.All() {
		if e.Skill.ID == id {
			out = append(out, e)
		}
	}
	return out
}

// inParallel runs fn once on each of n goroutines, released together, while
// read runs in a loop on one more, and waits for them.
func inParallel(n int, fn, read func()) {
	var ready, done sync.WaitGroup
	release := make(chan struct{})
	var stop atomic.Bool
	ready.Add(n)
	for range n {
		done.Go(func() {
			ready.Done()
			<-release
			fn()
		})
	}
	var reader sync.WaitGroup
	reader.Go(func() {
		for !stop.Load() {
			read()
		}
	})
	ready.Wait()
	close(release)
	done.Wait()
	stop.Store(true)
	reader.Wait()
}

// TestConcurrentFusionChannelsRaiseForceOnceEach opens fusion channels onto
// one NPC from several goroutines at once, each standing in for its own
// caster's queue, then closes them the same way. Every channel opening
// raises the force by one level (the first lands it at level 1) and every
// closing lowers it by one until it is gone, while lookups of the force
// level run alongside; the level the lookup reports, the held effect's and
// its icon's agree.
func TestConcurrentFusionChannelsRaiseForceOnceEach(t *testing.T) {
	t.Parallel()
	const rounds = 100
	channel, defs := fusionDefs()
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pc := onlineCharacter(t, srv, objID)
	target := srv.SpawnHostileNPC(t)
	list := target.EffectList()
	read := func() {
		list.ActiveBySkillID(fusionForceID)
		list.IconEntries(time.Now())
	}

	for round := range rounds {
		inParallel(fusionMaxLevel, func() { skillhandler.StartFusion(defs, pc, target, channel) }, read)
		held := heldOf(list, fusionForceID)
		if len(held) != 1 {
			t.Fatalf("round %d: %d force effects held after %d channels opened, want 1", round, len(held), fusionMaxLevel)
		}
		level, ok := list.ActiveBySkillID(fusionForceID)
		if !ok || level != fusionMaxLevel || held[0].Level != fusionMaxLevel {
			t.Fatalf("round %d: force level %d (held %v, effect level %d) after %d channels opened, want %d",
				round, level, ok, held[0].Level, fusionMaxLevel, fusionMaxLevel)
		}
		icons := list.IconEntries(time.Now())
		if !slices.ContainsFunc(icons, func(e effect.IconEntry) bool {
			return e.ID == fusionForceID && e.Level == fusionMaxLevel
		}) {
			t.Fatalf("round %d: icons %v lack force level %d", round, icons, fusionMaxLevel)
		}

		inParallel(fusionMaxLevel, func() { skillhandler.DecreaseFusion(defs, pc, target, channel) }, read)
		if held := heldOf(list, fusionForceID); len(held) != 0 {
			t.Fatalf("round %d: %d force effects held after every channel closed, want 0", round, len(held))
		}
	}
}

// TestConcurrentSeedCastsRaisePowerOnceEach lands a seed skill on one player
// from several goroutines at once, each standing in for its own caster's
// queue, while the player's own seed-power lookup runs alongside. The first
// lands the seed at power 1 and every later one grows that same seed by one.
func TestConcurrentSeedCastsRaisePowerOnceEach(t *testing.T) {
	t.Parallel()
	const rounds, casters = 100, 4
	seed := modelskill.Definition{
		ID: seedOfFireID, Level: 1, Activation: modelskill.ActivationActive, Target: modelskill.TargetOne,
		SkillType: "SEED", Effects: []modelskill.EffectTemplate{{Name: "Seed", Time: 60, Icon: true}},
	}
	table := modelskill.NewTable([]modelskill.Definition{seed})
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Newbie", 5, 0),
		gameservertest.WithWantChars(1),
	)
	c, objID := srv.Client, srv.SoleObjectID(t)
	startInWorld(t, c)
	pc := onlineCharacter(t, srv, objID)
	list := pc.EffectList()
	registry := skillhandler.NewDefaultRegistryWithSignet(table, false, nil, skillhandler.SignetDeps{})

	for round := range rounds {
		inParallel(casters, func() {
			registry.Use(skillhandler.Cast{Caster: pc, Skill: seed, Targets: []skillhandler.Actor{pc}})
		}, func() { list.ActiveBySkillID(seedOfFireID) })
		held := heldOf(list, seedOfFireID)
		if len(held) != 1 {
			t.Fatalf("round %d: %d seed effects held after %d casts, want 1", round, len(held), casters)
		}
		if power, ok := list.ActiveBySkillID(seedOfFireID); !ok || power != casters {
			t.Fatalf("round %d: seed power %d (held %v) after %d casts, want %d", round, power, ok, casters, casters)
		}
		list.Remove(held[0])
	}
	c.ReadQueued()
}
