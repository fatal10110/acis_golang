package task

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
)

// SetRoutes is WalkerRouteData.reload: every later node lookup, a walker's
// next step included, reads the new routes, and a lookup beside it reads
// one set or the other.
func TestWalkerSetRoutes(t *testing.T) {
	old := route.WalkerRoutes{"r": {"n": {{Location: walkerNodeA}}}}
	fresh := route.WalkerRoutes{"r": {"n": {{Location: walkerNodeB}, {Location: walkerNodeC}}}}
	w, err := NewWalker(old, wrapBlockedPath{}, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 1000 {
			if nodes, err := w.nodes("r", "n"); err != nil || len(nodes) != 1 && len(nodes) != 2 {
				t.Errorf("nodes() = %v, %v during the reload", nodes, err)
				return
			}
		}
	}()
	w.SetRoutes(fresh)
	wg.Wait()

	nodes, err := w.nodes("r", "n")
	if err != nil || len(nodes) != 2 || nodes[0].Location != walkerNodeB {
		t.Fatalf("nodes() after SetRoutes = %v, %v; want the new route", nodes, err)
	}
}
