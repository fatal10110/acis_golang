package residence

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
)

func TestCopySpawns(t *testing.T) {
	src := map[SpawnType][]location.Location{
		SpawnOwner: {{X: 1, Y: 2, Z: 3}},
	}

	got := CopySpawns(src)
	src[SpawnOwner][0].X = 99

	if got[SpawnOwner][0].X != 1 {
		t.Fatalf("CopySpawns() shared backing slice with source")
	}
	if CopySpawns(nil) != nil {
		t.Fatalf("CopySpawns(nil) != nil")
	}
}
