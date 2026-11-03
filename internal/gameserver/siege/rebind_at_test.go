package siege

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
)

// TestAtFollowsTheReloadedBattlefield pins At across a zone reload: a
// lookup racing Rebind (a holy-artifact cast during //reload zone) reads
// the battlefield under the engine's field lock, and once Rebind returns
// At finds the siege only inside the reloaded battlefield.
func TestAtFollowsTheReloadedBattlefield(t *testing.T) {
	w := newWorld(t, DefaultConfig())
	gludio := w.siege(1)
	if s, ok := w.e.At(10, 10, 10); !ok || s != gludio {
		t.Fatal("At inside the boot battlefield found no siege")
	}

	form, err := zone.NewCuboid(4000, 6000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatal(err)
	}
	reloaded := zone.NewSiege(11, form)
	reloaded.ResidenceID = 1

	var wg sync.WaitGroup
	start := make(chan struct{})
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		for range 200 {
			w.e.At(5000, 0, 0)
			w.e.At(10, 10, 10)
		}
	}()
	close(start)
	w.e.Rebind([]*zone.Siege{reloaded})
	wg.Wait()

	if s, ok := w.e.At(5000, 0, 0); !ok || s != gludio {
		t.Fatal("At inside the reloaded battlefield found no siege")
	}
	if _, ok := w.e.At(10, 10, 10); ok {
		t.Fatal("At inside the replaced battlefield still found the siege")
	}
}
