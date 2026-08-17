package player

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// TestCharacterStatPipelineConcurrentReadsAndMutationsAreRaceFree exercises
// #1527's concurrency requirement at the Character level: statCalcs' array
// of lazily created *effect.Calculator slots must stay safe under
// concurrent CalcStat reads (some touching a Stat for the first time,
// racing the lazy slot creation) and concurrent AddStatFuncs/
// RemoveStatsByOwner writers. Run under -race; this test asserts nothing
// about the numeric outcome, which is inherently nondeterministic here.
func TestCharacterStatPipelineConcurrentReadsAndMutationsAreRaceFree(t *testing.T) {
	tmpl := combatTemplate()
	c := liveCharacter(1, tmpl, combatItems())
	owner := effect.ModOwnerEffect(&effect.Effect{})

	stats := []stat.Stat{stat.PowerAttack, stat.PowerDefence, stat.MagicAttack, stat.MagicDefence, stat.RunSpeed}

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 4; i++ {
		readers.Add(1)
		go func(i int) {
			defer readers.Done()
			s := stats[i%len(stats)]
			for {
				select {
				case <-stop:
					return
				default:
					c.CalcStat(s, 100)
				}
			}
		}(i)
	}

	for i := 0; i < 2; i++ {
		writers.Add(1)
		go func() {
			defer writers.Done()
			for j := 0; j < 200; j++ {
				c.AddStatFuncs([]effect.Mod{
					{Stat: stat.PowerAttack, Op: effect.OpAdd, Value: 1, Owner: owner},
					{Stat: stat.MagicAttack, Op: effect.OpMul, Value: 1.1, Owner: owner},
				})
				c.RemoveStatsByOwner(owner)
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}
