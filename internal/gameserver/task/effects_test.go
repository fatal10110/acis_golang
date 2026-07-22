package task

import (
	"testing"
	"time"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

type effectActor struct {
	world.Presence

	id      int32
	effects *effect.List
}

func (a *effectActor) ObjectID() int32 { return a.id }

func (a *effectActor) EffectList() *effect.List { return a.effects }

func TestEffectsStartTicksLiveEffectListsOverWallClock(t *testing.T) {
	state := world.New()
	actor := &effectActor{id: 1, effects: effect.NewList(nil)}
	state.Spawn(actor, 0, 0, 0, 0)

	ticks := make(chan struct{}, 1)
	e := &effect.Effect{
		Template: skill.EffectTemplate{Name: "DamOverTime", Count: 1, Time: 1},
		Type:     effect.TypeDamOverTime,
		OnStart:  func(*effect.Effect) bool { return true },
		OnAction: func(*effect.Effect) bool {
			ticks <- struct{}{}
			return true
		},
	}
	actor.effects.Add(e)

	ticker := NewEffects(state).Start(zerolog.Nop())
	defer ticker.Stop()

	select {
	case <-ticks:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("periodic effect was not ticked by the live scheduler")
	}

	if effects := actor.effects.All(); len(effects) != 0 {
		t.Fatalf("one-count effect remained after its live tick: %d effects", len(effects))
	}
}
