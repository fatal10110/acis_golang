package script

import (
	"reflect"
	"slices"
	"testing"

	"github.com/rs/zerolog"
)

// TestSeamGateRegistersAttackFinishedOnFolk: no game event raises the
// attack-finished hook on a civilian NPC, as its attack reports no end, so
// a behavior bound to one through that hook registers with the production
// raised hooks, keeps the binding, and is not refused for a hostile-only
// operation that hook reaches. Any other hook still refuses such an
// operation on a civilian NPC.
func TestSeamGateRegistersAttackFinishedOnFolk(t *testing.T) {
	kindOf := func(id int32) (NPCKind, bool) {
		switch id {
		case 1:
			return KindFolk, true
		case 2:
			return KindHostile, true
		}
		return KindOther, false
	}
	warrior := func() Script {
		return Script{Behavior: true, NPCs: []int32{1, 2}, Hooks: Hooks{
			OnAttacked:       func(*Script, Attacked) {},
			OnAttackFinished: func(*Script, AttackFinished) {},
		}}
	}
	catalog := Catalog{"ai.Warrior": warrior, "ai.FolkHP": warrior}
	ops := map[string]HandleOps{
		"ai.Warrior": {"OnAttackFinished": {"NPC.HP"}},
		"ai.FolkHP":  {"OnAttacked": {"NPC.HP"}},
	}
	logs := &logBuffer{}
	r := Build(listOf("ai.Warrior", "ai.FolkHP"), catalog, Config{KindOf: kindOf, Log: zerolog.New(logs), HandleOps: ops})

	registered := map[string]bool{}
	for _, e := range r.entries {
		registered[e.path] = e.state == entryRegistered
	}
	if want := map[string]bool{"ai.Warrior": true, "ai.FolkHP": false}; !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %v, want %v: %s", registered, want, logs)
	}
	for _, id := range []int32{1, 2} {
		if got := pathsOf(r.scripts(id, EventAttackFinished)); !slices.Equal(got, []string{"ai.Warrior"}) {
			t.Fatalf("attack-finished list of npc %d = %v, want the warrior", id, got)
		}
	}
	if n := logs.count("script: refused"); n != 1 {
		t.Fatalf("refusals logged %d times, want 1: %s", n, logs)
	}
}
