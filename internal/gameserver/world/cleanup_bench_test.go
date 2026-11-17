package world

import (
	"fmt"
	"testing"
)

// BenchmarkSpawnNonPlayerNearPlayers spawns and despawns one observing NPC
// beside players observers: the notification list is sized once, not grown
// by doubling.
func BenchmarkSpawnNonPlayerNearPlayers(b *testing.B) {
	for _, players := range []int{50, 200} {
		b.Run(fmt.Sprintf("players=%d", players), func(b *testing.B) {
			s := New()
			x, y := regionCenter(10, 10)
			for i := range players {
				s.Spawn(&relocateBenchPlayer{id: int32(i + 2)}, x, y, 0, 0)
			}
			npc := &relocateBenchObserver{id: 1}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				s.Spawn(npc, x, y, 0, 0)
				s.Despawn(npc)
			}
		})
	}
}

// BenchmarkKnownPlayerScan compares the full known-set copy with the
// players-only scan a broadcast needs, at 300 nearby objects of which 20 are
// players.
func BenchmarkKnownPlayerScan(b *testing.B) {
	s := New()
	x, y := regionCenter(10, 10)
	for i := range 280 {
		s.Spawn(&regionTestObject{id: int32(i + 100)}, x, y, 0, 0)
	}
	for i := range 20 {
		s.Spawn(&relocateBenchPlayer{id: int32(i + 10)}, x, y, 0, 0)
	}
	src := &relocateBenchPlayer{id: 1}
	s.Spawn(src, x, y, 0, 0)

	b.Run("ForEachKnown", func(b *testing.B) {
		b.ReportAllocs()
		n := 0
		for range b.N {
			s.ForEachKnown(src, func(o Tracked) {
				if isPlayer(o) {
					n++
				}
			})
		}
		_ = n
	})
	b.Run("KnownPlayers", func(b *testing.B) {
		b.ReportAllocs()
		n := 0
		for range b.N {
			for range s.KnownPlayers(src) {
				n++
			}
		}
		_ = n
	})
}

// BenchmarkOnlinePlayerScan compares copying every tracked object with
// copying only the online players, at 30,000 objects and 200 players.
func BenchmarkOnlinePlayerScan(b *testing.B) {
	s := New()
	for i := range 30000 {
		s.AddObject(&regionTestObject{id: int32(i + 1000)})
	}
	for i := range 200 {
		p := &onlineBenchPlayer{regionTestPlayer{regionTestObject{id: int32(i + 1)}}}
		s.AddObject(p)
		s.AddPlayer(p)
	}
	b.Run("Objects", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = s.Objects()
		}
	})
	b.Run("Players", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			_ = s.Players()
		}
	})
}

type onlineBenchPlayer struct{ regionTestPlayer }

func (*onlineBenchPlayer) AccountName() string { return "" }
