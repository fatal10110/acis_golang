package task

import "testing"

// BenchmarkNPCRegenBackstopPass is BenchmarkNPCRegenTickManyIdleActors with
// a backstop pass on every sweep: the cost of one pass over the 30k
// tracked actors, every one of them full or already armed.
func BenchmarkNPCRegenBackstopPass(b *testing.B) {
	regen := benchRegenPopulation()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		regen.sweeps = 0 // always a backstop pass
		regen.Tick()
		testLoop.Run()
	}
}
