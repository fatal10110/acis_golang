package task

import (
	"sync"
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
)

// TestEffectsConcurrentAddRemoveTick exercises the registration path AC #4
// on the tracked issue asks to be proven safe under -race: effect.List.Add
// and Remove (called from arbitrary goroutines whenever a skill lands or
// expires) driving Effects' registry concurrently with Tick, which only
// ever runs from the single scheduler goroutine per Effects' own contract.
func TestEffectsConcurrentAddRemoveTick(t *testing.T) {
	e := NewEffects()

	const listCount = 20
	lists := make([]*effect.List, listCount)
	for i := range lists {
		lists[i] = effect.NewList(benchNoopStatOwner{})
	}
	newEffect := func(id int) *effect.Effect {
		eff, err := effect.New(effect.Skill{ID: modelskill.ID(id)}, modelskill.EffectTemplate{Name: "Buff"})
		if err != nil {
			t.Fatalf("effect.New: %v", err)
		}
		return eff
	}

	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for j, list := range lists {
				list.Add(newEffect(i*listCount + j + 1))
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			for _, list := range lists {
				for _, eff := range list.All() {
					list.Remove(eff)
				}
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			e.Tick()
		}
	}()
	wg.Wait()
}
