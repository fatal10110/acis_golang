package script

import (
	"maps"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/testsupport/handleops"
	"github.com/rs/zerolog"
)

// TestSeamGateRefusesMissingHandleOps: a script whose hook reaches a handle
// operation that the kind of an NPC the hook runs for lacks is refused at
// build and logged, and the build goes on. A hook raised for NPC events
// runs for the NPCs bound to those events only; a hook raised for none runs
// for every NPC the script is bound to.
func TestSeamGateRefusesMissingHandleOps(t *testing.T) {
	kindOf := func(id int32) (NPCKind, bool) {
		switch id {
		case 1:
			return KindFolk, true
		case 2:
			return KindHostile, true
		}
		return KindOther, false
	}
	talk := func(*Script, Talk) string { return "" }
	catalog := Catalog{
		"quest.KillDrop": func() Script {
			return Script{QuestID: 1, Bind: Bindings{EventTalked: {1}, EventMyDying: {2}}, Hooks: Hooks{OnTalk: talk, OnMyDying: func(*Script, MyDying) {}}}
		},
		"ai.FolkHP":    behavior(1),
		"ai.HostileHP": behavior(2),
		"quest.TimerDecay": func() Script {
			return Script{QuestID: 2, Bind: Bindings{EventTalked: {1}}, Hooks: Hooks{OnTalk: talk, OnTimer: func(*Script, Timer) string { return "" }}}
		},
		"ai.Unknown": behavior(2),
		"ai.Timer": func() Script {
			return Script{Behavior: true, NPCs: []int32{2}, Hooks: Hooks{OnTimer: func(*Script, Timer) string { return "" }}}
		},
	}
	ops := map[string]HandleOps{
		"quest.KillDrop":   {"OnMyDying": {"NPC.DropItem"}},
		"ai.FolkHP":        {"OnAttacked": {"NPC.HP"}},
		"ai.HostileHP":     {"OnAttacked": {"NPC.HP", "NPC.MaxHP"}},
		"quest.TimerDecay": {"OnTimer": {"Spawner.DecayMe"}},
		"ai.Unknown":       {"OnAttacked": {"NPC.NoSuchOperation"}},
		"ai.Timer":         {"OnTimer": {"NPC.HateList"}},
	}
	paths := []string{"quest.KillDrop", "ai.FolkHP", "ai.HostileHP", "quest.TimerDecay", "ai.Unknown", "ai.Timer"}
	logs := &logBuffer{}
	r := Build(listOf(paths...), catalog, Config{KindOf: kindOf, Log: zerolog.New(logs), HandleOps: ops, raises: raiseAll})

	registered := map[string]bool{}
	for _, e := range r.entries {
		registered[e.path] = e.state == entryRegistered
	}
	want := map[string]bool{"quest.KillDrop": true, "ai.FolkHP": false, "ai.HostileHP": true, "quest.TimerDecay": false, "ai.Unknown": false, "ai.Timer": true}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %v, want %v", registered, want)
	}
	if n := logs.count("script: refused; a handle operation it calls does not exist for an NPC kind it is bound to"); n != 3 {
		t.Fatalf("handle refusals logged %d times, want 3: %s", n, logs)
	}
	for _, msg := range []string{
		`onAttacked on npc 1 calls NPC.HP, which folk npcs do not have`,
		`onTimer on npc 1 calls Spawner.DecayMe, which folk npcs do not have`,
		`onAttacked calls NPC.NoSuchOperation, which is no kind-specific handle operation`,
	} {
		if !strings.Contains(logs.String(), msg) {
			t.Fatalf("log misses %q: %s", msg, logs)
		}
	}
	if got := pathsOf(r.scripts(1, EventAttacked)); len(got) != 0 {
		t.Fatalf("a refused script registered: %v", got)
	}
	if got := pathsOf(r.scripts(2, EventMyDying)); !slices.Equal(got, []string{"quest.KillDrop"}) {
		t.Fatalf("kill hook list = %v, want the quest whose drop runs on a hostile npc", got)
	}
}

// TestKindOpsMatchSource: the operations the gate knows as hostile-only are
// exactly the exported ones that reach (*NPC).hostile.
func TestKindOpsMatchSource(t *testing.T) {
	prog, err := handleops.Load(filepath.Join("..", "..", ".."), "./internal/gameserver/script/...")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := slices.Sorted(maps.Keys(kindOps)), prog.Ops(); !slices.Equal(got, want) {
		t.Fatalf("kindOps lists %v, the source has %v", got, want)
	}
	for op, ks := range kindOps {
		if ks != hostileOnly {
			t.Fatalf("%s exists for %b, want hostile npcs only", op, ks)
		}
	}
}

// TestHookByFieldFollowsFieldOrder: each Hooks field names the hook of its
// position, as the manifest names it.
func TestHookByFieldFollowsFieldOrder(t *testing.T) {
	ht := reflect.TypeFor[Hooks]()
	for h := range hookCount {
		name := ht.Field(int(h)).Name
		got, ok := hookByField(name)
		if !ok || got != h {
			t.Fatalf("hookByField(%s) = %v, %v; want %v", name, got, ok, h)
		}
		if want := "on" + strings.TrimPrefix(name, "On"); h != hookEvent && hookMethods[h] != want {
			t.Fatalf("field %d is %s, hook %s", h, name, hookMethods[h])
		}
	}
	if _, ok := hookByField("own"); ok {
		t.Fatal("an unexported field named a hook")
	}
}
