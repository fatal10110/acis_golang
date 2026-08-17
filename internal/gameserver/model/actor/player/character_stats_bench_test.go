package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// BenchmarkCalcStat exercises the hot combat-resolution read path (see
// issue #1473, subsumed by #1527's stat-pipeline redesign): a warm
// CalcStat call must not take an exclusive lock, hash a map, or allocate.
func BenchmarkCalcStat(b *testing.B) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())

	// Warm the Calculator slot before measuring so the benchmark isolates
	// the steady-state read path from one-time slot creation.
	c.CalcStat(stat.PowerAttack, tmpl.PAtk)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.CalcStat(stat.PowerAttack, tmpl.PAtk)
	}
}
