package gatekeeper

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/travel"
)

// SetTables is TeleportData.reload and InstantTeleportData.reload: the
// next request reads the new destinations, and a request beside it reads
// one table or the other.
func TestSetTablesReplacesTheDestinations(t *testing.T) {
	old := travel.InstantTable{7: {{X: 1, Y: 1, Z: 1}}}
	fresh := travel.InstantTable{7: {{X: 2, Y: 2, Z: 2}}, 8: {{X: 3, Y: 3, Z: 3}}}
	s := NewService(travel.TeleportTable{}, old, true, nil)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			if trip := s.Instant(7, 0); !trip.Depart {
				t.Error("npc 7 has no destination during the reload")
				return
			}
			s.Window(1, 7, travel.Kind(0))
		}
	}()
	s.SetTables(travel.TeleportTable{}, fresh)
	wg.Wait()

	if trip := s.Instant(7, 0); trip.Destination != (location.Location{X: 2, Y: 2, Z: 2}) {
		t.Fatalf("npc 7 goes to %+v, want the new destination", trip.Destination)
	}
	if trip := s.Instant(8, 0); !trip.Depart {
		t.Fatal("npc 8 of the new table has no destination")
	}
}
