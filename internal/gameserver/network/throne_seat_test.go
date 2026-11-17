package network

import (
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
)

// TestThroneSeatReadsOffQueue pins the seat's ownership contract: the
// sitter's queue takes and releases the throne while an observer's
// goroutine reads it, as Discover (ChairSit) and Forget (the DeleteObject
// seated flag) do. Under -race the reads must not race the writes, and
// every read sees either no seat or the whole claimed chair.
func TestThroneSeatReadsOffQueue(t *testing.T) {
	chair, err := staticobject.NewObject(7, &staticobject.Template{
		ID:       24180017,
		Location: location.Location{X: 10, Y: 20, Z: 30},
		Type:     staticobject.ChairType,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	sitter := &livePlayer{}

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 1000 {
			sitter.takeThrone(chair)
			sitter.releaseChair()
		}
	})
	wg.Go(func() {
		for range 1000 {
			id, ok := sitter.throneID()
			if ok && id != chair.StaticObjectID() {
				t.Errorf("throne id = %d, want %d", id, chair.StaticObjectID())
				return
			}
			_ = sitter.seated()
		}
	})
	wg.Wait()

	if sitter.seated() {
		t.Fatal("seated after the last release")
	}
	sitter.takeThrone(chair)
	if id, ok := sitter.throneID(); !ok || id != chair.StaticObjectID() {
		t.Fatalf("throneID = %d, %v; want %d, true", id, ok, chair.StaticObjectID())
	}
}
