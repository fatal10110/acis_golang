package script

import (
	"reflect"
	"strings"
	"testing"

	"github.com/rs/zerolog"
)

// TestProductionGateIdleAndArrivalHooks pins the production seam gate for
// the AI's idle and arrival hooks: no-desire is raised on civilian and
// hostile NPCs, move-finished and out-of-territory on hostile NPCs only, so
// a civilian behavior that waits on an arrival is refused at boot.
func TestProductionGateIdleAndArrivalHooks(t *testing.T) {
	kindOf := func(id int32) (NPCKind, bool) {
		switch id {
		case 1:
			return KindFolk, true
		case 2:
			return KindHostile, true
		}
		return KindOther, false
	}
	behavior := func(id int32, h Hooks) func() Script {
		return func() Script { return Script{Behavior: true, NPCs: []int32{id}, Hooks: h} }
	}
	moveFinished := Hooks{OnMoveToFinished: func(*Script, MoveToFinished) {}}
	outOfTerritory := Hooks{OnOutOfTerritory: func(*Script, OutOfTerritory) {}}
	noDesire := Hooks{OnNoDesire: func(*Script, NoDesire) {}}
	catalog := Catalog{
		"ai.FolkMoveFinished":      behavior(1, moveFinished),
		"ai.FolkOutOfTerritory":    behavior(1, outOfTerritory),
		"ai.FolkNoDesire":          behavior(1, noDesire),
		"ai.HostileMoveFinished":   behavior(2, moveFinished),
		"ai.HostileOutOfTerritory": behavior(2, outOfTerritory),
		"ai.HostileNoDesire":       behavior(2, noDesire),
	}
	paths := []string{
		"ai.FolkMoveFinished", "ai.FolkOutOfTerritory", "ai.FolkNoDesire",
		"ai.HostileMoveFinished", "ai.HostileOutOfTerritory", "ai.HostileNoDesire",
	}
	logs := &logBuffer{}
	// No raises override: this is the gate the server boots with.
	r := Build(listOf(paths...), catalog, Config{KindOf: kindOf, Log: zerolog.New(logs)})

	registered := map[string]bool{}
	for _, e := range r.entries {
		registered[e.path] = e.state == entryRegistered
	}
	want := map[string]bool{
		"ai.FolkMoveFinished": false, "ai.FolkOutOfTerritory": false, "ai.FolkNoDesire": true,
		"ai.HostileMoveFinished": true, "ai.HostileOutOfTerritory": true, "ai.HostileNoDesire": true,
	}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %v, want %v", registered, want)
	}
	if n := logs.count("script: refused"); n != 2 {
		t.Fatalf("refusals logged %d times, want 2: %s", n, logs)
	}
	for _, p := range []string{"ai.FolkMoveFinished", "ai.FolkOutOfTerritory"} {
		if !strings.Contains(logs.String(), `"script":"`+p+`"`) {
			t.Errorf("no refusal logged for %s: %s", p, logs)
		}
	}
	if !r.Behaves(1) || !r.Behaves(2) {
		t.Fatalf("Behaves(1)=%v Behaves(2)=%v, want both bound", r.Behaves(1), r.Behaves(2))
	}
}
