package npc

import (
	"bytes"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestFolkGeoPathFailCountWrapsPastMaximum pins a walking civilian NPC's
// failed-pathfinding streak at a maximum of 2: it counts up to one past the
// maximum, then the next failure logs the overflow once and restarts the
// streak from zero instead of counting.
func TestFolkGeoPathFailCountWrapsPastMaximum(t *testing.T) {
	inst, err := NewInstance(1, &Template{ID: 31357, TemplateID: 31357, Type: "Folk", Name: "Leandro", Level: 1, HPMax: 100, CanMove: true})
	if err != nil {
		t.Fatalf("new instance: %v", err)
	}
	f, err := NewFolk(inst)
	if err != nil {
		t.Fatalf("new folk: %v", err)
	}
	var logs bytes.Buffer
	w, err := f.EnableMovement(FolkMovement{Geo: hostileGeo{}, Queue: idleQueue(), MaxGeoPathFailCount: 2, Log: zerolog.New(&logs)})
	if err != nil {
		t.Fatalf("enable movement: %v", err)
	}
	for i, want := range []int{1, 2, 3, 0, 1} {
		w.AddGeoPathFailCount()
		if got := w.GeoPathFailCount(); got != want {
			t.Fatalf("after failure %d streak = %d, want %d", i+1, got, want)
		}
	}
	if n := strings.Count(logs.String(), "geopath fail overflow"); n != 1 || !strings.Contains(logs.String(), `"npc":"Leandro"`) {
		t.Fatalf("overflow logged %d times as %q, want once for Leandro", n, logs.String())
	}
}
