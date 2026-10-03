package pathfind

import (
	"math/rand"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

// BenchmarkFinderSearch measures FindInto over search-heavy samples: on the
// synthetic split maze (always available) and, where the shared datapack's
// geodata is present, on real terrain around Gludio castle. "routed" cases
// find a path; "failed" cases exhaust the frontier or MaxIterations.
func BenchmarkFinderSearch(b *testing.B) {
	b.Run("maze", func(b *testing.B) {
		const width, height, split = 192, 160, 96
		e := splitMazeEngine(b, width, height, split, 1)
		finder := New(e, DefaultOptions())
		routed, failed := splitByResult(finder, randomGridCases(rand.New(rand.NewSource(1)), width, height, 120, 200))
		benchmarkCases(b, finder, "routed", routed)
		benchmarkCases(b, finder, "failed", failed)
	})
	b.Run("geodata", func(b *testing.B) {
		e := loadGeodataSample(b)
		finder := New(e, DefaultOptions())
		direct, searched := geodataCases(e, 2, 1000, 2000)
		routed, failed := splitByResult(finder, searched)
		benchmarkCases(b, finder, "direct", direct)
		benchmarkCases(b, finder, "routed", routed)
		benchmarkCases(b, finder, "failed", failed)
	})
}

func splitByResult(f *Finder, cases []pathCase) (found, failed []pathCase) {
	for _, c := range cases {
		if f.HasPath(c.origin, c.target) {
			found = append(found, c)
		} else {
			failed = append(failed, c)
		}
	}
	return found, failed
}

func benchmarkCases(b *testing.B, f *Finder, name string, cases []pathCase) {
	b.Run(name, func(b *testing.B) {
		if len(cases) == 0 {
			b.Skip("no cases in sample")
		}
		dst := make([]location.Location, 0, 64)
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			c := cases[i%len(cases)]
			dst, _, _ = f.FindInto(dst[:0], c.origin, c.target)
		}
	})
}
