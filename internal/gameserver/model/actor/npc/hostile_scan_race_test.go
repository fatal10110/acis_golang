package npc

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func TestRandomNearbyScansConcurrent(t *testing.T) {
	const regionSize = 2048
	tpl := &Template{ID: 9001, Type: "Monster", BaseAttackRange: 80, CanMove: true}
	ox := world.MinX + 2*regionSize + regionSize/2
	oy := world.MinY + 2*regionSize + regionSize/2
	state := world.New()
	scanner := newCombatHostile(t, 1, tpl)
	scanner.SetWorld(state)
	scanner.roll = func(int) int { return 0 }
	state.Spawn(scanner, ox, oy, 0, 0)
	for i := 0; i < 8; i++ {
		other := newCombatHostile(t, int32(i+2), tpl)
		other.SetWorld(state)
		state.Spawn(other, ox, oy, 0, 0)
	}

	const goroutines = 16
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				scanner.RandomNearbyMonster(600)
				return
			}
			scanner.RandomNearbyCombatant(1000)
		}()
	}
	wg.Wait()
}
