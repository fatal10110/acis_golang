package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func BenchmarkRandomNearbyMonster(b *testing.B) {
	const regionSize = 2048
	tpl := &Template{ID: 9001, Type: "Monster", BaseAttackRange: 80, CanMove: true}
	ox := world.MinX + 2*regionSize + regionSize/2
	oy := world.MinY + 2*regionSize + regionSize/2
	for _, tc := range []struct {
		name   string
		n      int
		spread bool
	}{
		{"inCap", 63, false},
		{"crowded", 1500, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			state := world.New()
			scanner := newCombatHostile(b, 1, tpl)
			scanner.SetWorld(state)
			scanner.roll = func(int) int { return 0 }
			state.Spawn(scanner, ox, oy, 0, 0)
			for i := 0; i < tc.n; i++ {
				x, y := ox, oy
				if tc.spread {
					x += ((i % 3) - 1) * regionSize
					y += (((i / 3) % 3) - 1) * regionSize
				}
				other := newCombatHostile(b, int32(i+2), tpl)
				other.SetWorld(state)
				state.Spawn(other, x, y, 0, 0)
			}
			// Radius -1 is the unfiltered worst case (match every object in
			// the searched regions). Production distrustStart passes 600.
			if _, ok := scanner.RandomNearbyMonster(-1); !ok {
				b.Fatal("warmup: RandomNearbyMonster: ok = false")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, ok := scanner.RandomNearbyMonster(-1); !ok {
					b.Fatal("RandomNearbyMonster: ok = false, want a crowded-region hit")
				}
			}
		})
	}
}
