package script

import (
	"testing"

	"github.com/rs/zerolog"
)

// TestReactsToCreatedNamesTheIDsACreatedHookIsBoundTo: an NPC id reacts to
// its creation when a script, a behavior or a plain one, registered its
// created hook on it; a script bound to another event and an id with no
// template do not count.
func TestReactsToCreatedNamesTheIDsACreatedHookIsBoundTo(t *testing.T) {
	onCreated := Hooks{OnCreated: func(*Script, Created) {}}
	catalog := Catalog{
		"ai.Created": func() Script {
			return Script{Behavior: true, NPCs: []int32{1, 99}, Hooks: onCreated}
		},
		"ai.group.Walkers": func() Script {
			return Script{Bind: Bindings{EventCreated: {2}}, Hooks: onCreated}
		},
		"quest.Kills": quest(Bindings{EventAttacked: {3}}),
	}
	kindOf := func(id int32) (NPCKind, bool) { return KindFolk, id != 99 }
	r := Build(listOf("ai.Created", "ai.group.Walkers", "quest.Kills"), catalog, RaiseAll(Config{KindOf: kindOf, Log: zerolog.Nop()}))
	for id, want := range map[int32]bool{1: true, 2: true, 3: false, 99: false} {
		if got := r.ReactsToCreated(id); got != want {
			t.Errorf("ReactsToCreated(%d) = %v, want %v", id, got, want)
		}
	}
	if r.Behaves(2) {
		t.Error("a plain created script counts as a behavior")
	}
	if (*Registry)(nil).ReactsToCreated(1) {
		t.Error("a nil registry binds a created hook")
	}
}
