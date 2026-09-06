package task

import (
	"testing"

	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// benchNoopStatOwner satisfies effect.StatOwner without recording anything,
// matching the effect package's own noopStatOwner test double.
type benchNoopStatOwner struct{}

func (benchNoopStatOwner) AddStatFuncs([]effect.Mod)          {}
func (benchNoopStatOwner) RemoveStatsByOwner(effect.ModOwner) {}
func (benchNoopStatOwner) MaxBuffCount() int                  { return 20 }

// BenchmarkEffectsTickManyIdleLists reproduces the population size from
// .agent-cache/reviews/perf-hot-paths.md (30k tracked actors, 1% carrying a
// live effect): Tick's cost should scale with the active fraction, not the
// idle 30k, since an empty list never registers with Effects at all. All
// 30k lists are kept alive in `lists` for the benchmark's duration — an
// idle list nothing retains would prove nothing about scanning a live but
// idle population, since it would never coexist with the active ones.
func BenchmarkEffectsTickManyIdleLists(b *testing.B) {
	const total = 30000
	const activeFraction = 100 // 1 in 100 carries a live effect

	e := NewEffects()
	lists := make([]*effect.List, total)
	for i := 0; i < total; i++ {
		list := effect.NewList(benchNoopStatOwner{})
		lists[i] = list
		if i%activeFraction == 0 {
			eff, err := effect.New(effect.Skill{ID: modelskill.ID(i + 1)}, modelskill.EffectTemplate{Name: "Buff"})
			if err != nil {
				b.Fatal(err)
			}
			list.Add(eff)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		e.Tick()
	}
	b.StopTimer()
	_ = lists // keep the idle 29,700 reachable through the whole benchmark
}

// benchRegenActor is a minimal npcRegenActor: TickRegen never finds itself
// below max, matching the review's "all objects already at full HP/MP"
// steady-state case that a full scan still has to visit.
type benchRegenActor struct{ id int32 }

func (a *benchRegenActor) ObjectID() int32 { return a.id }
func (a *benchRegenActor) TickRegen()      {}

// BenchmarkNPCRegenTickManyIdleActors reproduces the review's 30k tracked
// population for NPCRegen.Tick: the fix removes State.Objects()'s per-tick
// snapshot allocation via a reused scratch buffer, so steady-state allocs
// should fall to ~0 once the buffer's capacity stabilizes.
func BenchmarkNPCRegenTickManyIdleActors(b *testing.B) {
	const total = 30000

	state := world.New()
	for i := 0; i < total; i++ {
		state.AddObject(&benchRegenActor{id: int32(i) + 1})
	}
	regen := NewNPCRegen(state)
	regen.Tick()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		regen.Tick()
	}
}
