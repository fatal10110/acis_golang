package script

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/rs/zerolog"
)

// benchTimerWorld is a behavior script on n NPCs of its template, each on
// its own queue of a running pool.
func benchTimerWorld(b *testing.B, n int) (*Script, []*NPC) {
	b.Helper()
	pool := sim.NewPool(2, zerolog.Nop())
	pool.Start(context.Background())
	b.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = pool.Stop(ctx)
	})
	catalog := Catalog{"ai.A": func() Script {
		return Script{Behavior: true, NPCs: []int32{timerNPCID}, Hooks: Hooks{
			OnTimer:    func(*Script, Timer) string { return "" },
			OnAttacked: func(*Script, Attacked) {},
		}}
	}}
	r := Build(listOf("ai.A"), catalog, RaiseAll(Config{KindOf: allTemplates, Log: zerolog.Nop(), Queue: pool.NewQueue("script-timers")}))
	npcs := make([]*NPC, n)
	for i := range npcs {
		f := &timerNPCFake{combatant: combatant{objectID: int32(1000 + i)}, id: timerNPCID, queue: pool.NewQueue("n"), scratch: npc.NewScratch()}
		npcs[i] = &NPC{self: f}
	}
	return r.entries[0].script, npcs
}

// BenchmarkTimerSpawnArm is the memory of one NPC's spawn-armed fixed-rate
// timer, its index entries included: one timer on each of b.N NPCs.
func BenchmarkTimerSpawnArm(b *testing.B) {
	s, npcs := benchTimerWorld(b, b.N)
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		s.StartTimerAtFixedRate("tick", npcs[i], nil, time.Hour, time.Hour)
	}
	b.StopTimer()
	s.CancelTimers()
}

// BenchmarkTimerStartCancelParallel starts and cancels a one-shot timer on
// NPCs that each hold a spawn-armed fixed-rate timer, every goroutine on
// NPCs of its own: the lock contention of the registry.
func BenchmarkTimerStartCancelParallel(b *testing.B) {
	const perG = 64
	s, npcs := benchTimerWorld(b, 64*perG)
	for _, n := range npcs {
		s.StartTimerAtFixedRate("tick", n, nil, time.Hour, time.Hour)
	}
	var next atomic.Int32
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		mine := npcs[(int(next.Add(1)-1)%64)*perG:][:perG]
		i := 0
		for pb.Next() {
			n := mine[i%perG]
			s.StartTimer("once", n, nil, time.Hour)
			s.HasTimer("tick", n, nil)
			s.CancelTimer("once", n, nil)
			i++
		}
	})
	b.StopTimer()
	s.CancelTimers()
}
