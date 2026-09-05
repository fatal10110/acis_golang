package world

import (
	"testing"
)

func BenchmarkForEachKnownInRadius(b *testing.B) {
	// inCap places 63 neighbors + the origin in one region. crowded spreads
	// 1500 neighbors across the 3x3 known neighborhood, matching the #2256
	// measurement. Both stay inside knownInRadiusObjectCap.
	for _, tc := range []struct {
		name   string
		n      int
		spread bool
	}{
		{"inCap", 63, false},
		{"crowded", 1500, true},
	} {
		b.Run(tc.name, func(b *testing.B) {
			s := New()
			origin := &spawnOrderObject{id: 1}
			spawnRadiusCrowd(s, origin, tc.n, tc.spread)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var count int
				s.ForEachKnownInRadius(origin, -1, func(Tracked) { count++ })
				if count != tc.n {
					b.Fatalf("visited %d, want %d", count, tc.n)
				}
			}
		})
	}
}

func spawnRadiusCrowd(s *State, origin Tracked, n int, spread bool) {
	ox := MinX + 2*regionSize + regionSize/2
	oy := MinY + 2*regionSize + regionSize/2
	s.Spawn(origin, ox, oy, 0, 0)
	for i := 0; i < n; i++ {
		x, y := ox, oy
		if spread {
			x += ((i % 3) - 1) * regionSize
			y += (((i / 3) % 3) - 1) * regionSize
		}
		s.Spawn(&spawnOrderObject{id: int32(i + 2)}, x, y, 0, 0)
	}
}
