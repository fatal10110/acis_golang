package script

import (
	"testing"

	"github.com/rs/zerolog"
)

// TestProductionGateSeeCreature pins the production seam gate for the
// see-creature hook, raised on civilian and hostile NPCs: a behavior of
// either kind that subscribes to it registers, and SeesCreatures names the
// ids it is bound to.
func TestProductionGateSeeCreature(t *testing.T) {
	kindOf := func(id int32) (NPCKind, bool) {
		switch id {
		case 1:
			return KindFolk, true
		case 2, 3:
			return KindHostile, true
		}
		return KindOther, false
	}
	see := Hooks{OnSeeCreature: func(*Script, SeeCreature) {}}
	catalog := Catalog{
		"ai.FolkSee":    func() Script { return Script{Behavior: true, NPCs: []int32{1}, Hooks: see} },
		"ai.HostileSee": func() Script { return Script{Behavior: true, NPCs: []int32{2}, Hooks: see} },
		"ai.Idle": func() Script {
			return Script{Behavior: true, NPCs: []int32{3}, Hooks: Hooks{OnNoDesire: func(*Script, NoDesire) {}}}
		},
	}
	logs := &logBuffer{}
	// No raises override: this is the gate the server boots with.
	r := Build(listOf("ai.FolkSee", "ai.HostileSee", "ai.Idle"), catalog, Config{KindOf: kindOf, Log: zerolog.New(logs)})
	for _, e := range r.entries {
		if e.state != entryRegistered {
			t.Fatalf("%s not registered: %s", e.path, logs)
		}
	}
	for id, want := range map[int32]bool{1: true, 2: true, 3: false, 4: false} {
		if got := r.SeesCreatures(id); got != want {
			t.Errorf("SeesCreatures(%d) = %v, want %v", id, got, want)
		}
	}
	if (*Registry)(nil).SeesCreatures(1) {
		t.Error("a nil registry binds a see-creature hook")
	}
}
