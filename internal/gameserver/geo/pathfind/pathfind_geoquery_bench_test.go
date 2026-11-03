package pathfind

import (
	"math/rand"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/engine"
)

// BenchmarkGeodataQueries measures Height, CanMove and CanSee on real terrain
// around Gludio castle, where the shared datapack's geodata is present: the
// complex and multilayer cell reads the synthetic one-block engine
// benchmarks never reach. Pairs are at most 1000 units apart, an AI's usual
// move and sight range.
func BenchmarkGeodataQueries(b *testing.B) {
	e := loadGeodataSample(b)
	cases := geodataQueryCases(e, 3, 4096, 1000)
	b.Run("Height", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			c := cases[i%len(cases)]
			_ = e.Height(c.origin.X, c.origin.Y, c.origin.Z+64)
		}
	})
	b.Run("CanMove", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			c := cases[i%len(cases)]
			_ = e.CanMove(c.origin.X, c.origin.Y, c.origin.Z, c.target.X, c.target.Y, c.target.Z)
		}
	})
	b.Run("CanSee", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; b.Loop(); i++ {
			c := cases[i%len(cases)]
			_ = e.CanSee(c.origin.X, c.origin.Y, c.origin.Z, c.target.X, c.target.Y, c.target.Z)
		}
	})
}

// geodataQueryCases draws n origin/target pairs on the sample's terrain,
// each target within span of its origin on both axes.
func geodataQueryCases(e *engine.Engine, seed int64, n, span int) []pathCase {
	direct, searched := geodataCases(e, seed, n, span)
	cases := append(direct, searched...)
	rand.New(rand.NewSource(seed)).Shuffle(len(cases), func(i, j int) { cases[i], cases[j] = cases[j], cases[i] })
	return cases
}
