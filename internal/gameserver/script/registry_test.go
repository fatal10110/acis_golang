package script

import (
	"bytes"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rs/zerolog"
)

// logBuffer is a goroutine-safe log sink.
type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *logBuffer) count(s string) int { return strings.Count(b.String(), s) }

func listOf(paths ...string) []Listing {
	out := make([]Listing, len(paths))
	for i, p := range paths {
		out[i] = Listing{Path: p}
	}
	return out
}

func raiseAll(hook, NPCKind) bool { return true }

func build(t *testing.T, list []Listing, catalog Catalog) (*Registry, *logBuffer) {
	t.Helper()
	logs := &logBuffer{}
	return Build(list, catalog, Config{KindOf: allTemplates, Log: zerolog.New(logs), raises: raiseAll}), logs
}

func pathsOf(list []*Script) []string {
	out := make([]string, len(list))
	for i, s := range list {
		out[i] = s.path
	}
	return out
}

func behavior(ids ...int32) func() Script {
	return func() Script {
		return Script{Behavior: true, NPCs: ids, Hooks: Hooks{
			OnAttacked:  func(*Script, Attacked) {},
			OnFirstTalk: func(*Script, FirstTalk) string { return "" },
		}}
	}
}

func quest(bind Bindings) func() Script {
	return func() Script { return Script{QuestID: 1, Bind: bind} }
}

func TestBuildFollowsListOrderAndSkipsMissing(t *testing.T) {
	built := map[string]int{}
	counted := func(path string, ctor func() Script) func() Script {
		return func() Script { built[path]++; return ctor() }
	}
	catalog := Catalog{
		"quest.A":    counted("quest.A", quest(Bindings{EventTalked: {1}})),
		"quest.B":    counted("quest.B", quest(Bindings{EventTalked: {1}})),
		"quest.Lone": counted("quest.Lone", quest(Bindings{EventTalked: {1}})),
		"script.Bad": counted("script.Bad", func() Script { panic("bad constructor") }),
	}
	r, logs := build(t, listOf("quest.B", "quest.Gone", "quest.A", "script.Bad"), catalog)

	if got, want := pathsOf(r.scripts(1, EventTalked)), []string{"quest.B", "quest.A"}; !slices.Equal(got, want) {
		t.Fatalf("talked list = %v, want list order %v", got, want)
	}
	if built["quest.Lone"] != 0 {
		t.Fatal("an unlisted catalog entry was constructed")
	}
	if built["quest.A"] != 1 || built["quest.B"] != 1 {
		t.Fatalf("constructions = %v, want one per listed path", built)
	}
	if n := logs.count(`"script":"quest.Gone"`); n != 1 {
		t.Fatalf("missing path logged %d times, want once: %s", n, logs)
	}
	if !strings.Contains(logs.String(), `"level":"error","script":"quest.Gone"`) {
		t.Fatalf("missing path not logged at error level: %s", logs)
	}
	if !strings.Contains(logs.String(), `"script":"script.Bad","panic":"bad constructor"`) {
		t.Fatalf("panicking constructor not logged: %s", logs)
	}
	var dump strings.Builder
	if err := r.Dump(&dump); err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{"script quest.Gone missing\n", "script script.Bad missing\n"} {
		if !strings.Contains(dump.String(), line) {
			t.Errorf("dump lacks %q:\n%s", line, dump.String())
		}
	}
}

// TestRegistrationRules covers the per-(NPC, event) rules: the last listed
// behavior wins, including for an id a parent and its child both claim;
// other scripts accumulate in list order; a script listed twice keeps one
// place, at the end.
func TestRegistrationRules(t *testing.T) {
	catalog := Catalog{
		"ai.Parent":  behavior(10, 11),
		"ai.Child":   behavior(11),
		"quest.Q1":   quest(Bindings{EventAttacked: {10, 11}}),
		"quest.Q2":   quest(Bindings{EventAttacked: {11}}),
		"ai.Talking": func() Script { return Script{Behavior: true, Bind: Bindings{EventAttacked: {12}}} },
	}
	r, _ := build(t, listOf("quest.Q1", "ai.Parent", "quest.Q2", "ai.Child", "quest.Q1", "ai.Talking"), catalog)

	for _, tc := range []struct {
		npc  int32
		want []string
	}{
		{10, []string{"ai.Parent", "quest.Q1"}},
		{11, []string{"quest.Q2", "ai.Child", "quest.Q1"}},
		{12, []string{"ai.Talking"}},
	} {
		if got := pathsOf(r.scripts(tc.npc, EventAttacked)); !slices.Equal(got, tc.want) {
			t.Errorf("npc %d attacked = %v, want %v", tc.npc, got, tc.want)
		}
	}
}

// TestFirstTalkSlot: the first script keeps the slot, a behavior replaces
// an earlier behavior, and first talk runs only with exactly one script.
func TestFirstTalkSlot(t *testing.T) {
	talker := func(answer string) func() Script {
		return func() Script {
			return Script{Bind: Bindings{EventFirstTalk: {1}}, Hooks: Hooks{OnFirstTalk: func(*Script, FirstTalk) string { return answer }}}
		}
	}
	behaviorTalker := func(answer string) func() Script {
		return func() Script {
			return Script{Behavior: true, NPCs: []int32{2}, Hooks: Hooks{OnFirstTalk: func(*Script, FirstTalk) string { return answer }}}
		}
	}
	catalog := Catalog{
		"script.First":  talker("first.htm"),
		"script.Second": talker("second.htm"),
		"ai.Old":        behaviorTalker("old.htm"),
		"ai.New":        behaviorTalker("new.htm"),
	}
	r, _ := build(t, listOf("script.First", "script.Second", "ai.Old", "ai.New"), catalog)

	if _, res, bound := r.FirstTalk(1, FirstTalk{}); !bound || res != (Result{ResultPageFile, "first.htm"}) {
		t.Fatalf("npc 1 first talk = %+v, %v; want the first registered script", res, bound)
	}
	if _, res, bound := r.FirstTalk(2, FirstTalk{}); !bound || res != (Result{ResultPageFile, "new.htm"}) {
		t.Fatalf("npc 2 first talk = %+v, %v; want the later behavior", res, bound)
	}
	if _, _, bound := r.FirstTalk(3, FirstTalk{}); bound {
		t.Fatal("npc 3 has no first-talk script but reported one")
	}
	if got := len(r.scripts(1, EventFirstTalk)); got != 1 {
		t.Fatalf("npc 1 first-talk list holds %d scripts, want 1", got)
	}
}

// TestAbnormalStatusReachesSeeSpellList: the abnormal-status hook runs over
// the see-spell list, not over its own registrations.
func TestAbnormalStatusReachesSeeSpellList(t *testing.T) {
	var got []string
	reacting := func(name string, hooks Hooks) func() Script {
		return func() Script {
			hooks.OnAbnormalStatusChanged = func(*Script, AbnormalStatusChanged) { got = append(got, name) }
			return Script{Behavior: true, NPCs: []int32{1}, Hooks: hooks}
		}
	}
	catalog := Catalog{
		// Sees spells: bound to SEE_SPELL and ABNORMAL_STATUS_CHANGED.
		"ai.Seer": reacting("ai.Seer", Hooks{OnSeeSpell: func(*Script, SeeSpell) {}}),
		"quest.Q": func() Script {
			return Script{Bind: Bindings{EventSeeSpell: {1}}, Hooks: Hooks{
				OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) { got = append(got, "quest.Q") },
			}}
		},
		// Only ABNORMAL_STATUS_CHANGED: never reached.
		"script.Blind": func() Script {
			return Script{Bind: Bindings{EventAbnormalStatusChanged: {1}}, Hooks: Hooks{
				OnAbnormalStatusChanged: func(*Script, AbnormalStatusChanged) { got = append(got, "script.Blind") },
			}}
		},
	}
	r, _ := build(t, listOf("ai.Seer", "quest.Q", "script.Blind"), catalog)
	r.AbnormalStatusChanged(1, AbnormalStatusChanged{})
	if want := []string{"ai.Seer", "quest.Q"}; !slices.Equal(got, want) {
		t.Fatalf("abnormal status reached %v, want the see-spell list %v", got, want)
	}
}

// TestSeamGateRefusesUnraisedHooks: a script whose hook has no raising
// site for the kind of an NPC it is bound to is refused at build, and so is
// one setting an unraised event or timer hook; the rest register.
func TestSeamGateRefusesUnraisedHooks(t *testing.T) {
	kindOf := func(id int32) (NPCKind, bool) {
		switch id {
		case 1:
			return KindFolk, true
		case 2:
			return KindHostile, true
		}
		return KindOther, false
	}
	raised := map[hook]kindSet{hookAttacked: kinds(KindHostile), hookTalk: kinds(KindFolk, KindHostile)}
	catalog := Catalog{
		"ai.HostileOnly": func() Script {
			return Script{Behavior: true, NPCs: []int32{2, 99}, Hooks: Hooks{OnAttacked: func(*Script, Attacked) {}}}
		},
		"ai.FolkAttacked": func() Script {
			return Script{Behavior: true, NPCs: []int32{1}, Hooks: Hooks{OnAttacked: func(*Script, Attacked) {}}}
		},
		"quest.Talker": quest(Bindings{EventTalked: {1, 2}, EventQuestStart: {1}}),
		"quest.Timer": func() Script {
			return Script{QuestID: 2, Bind: Bindings{EventTalked: {1}}, Hooks: Hooks{OnTimer: func(*Script, Timer) string { return "" }}}
		},
		"quest.Unraised": quest(Bindings{EventCreated: {2}}),
	}
	logs := &logBuffer{}
	r := Build(listOf("ai.HostileOnly", "ai.FolkAttacked", "quest.Talker", "quest.Timer", "quest.Unraised"), catalog, Config{
		KindOf: kindOf, Log: zerolog.New(logs),
		raises: func(h hook, k NPCKind) bool { return raised[h]&kinds(k) != 0 },
	})

	registered := map[string]bool{}
	for _, e := range r.entries {
		registered[e.path] = e.state == entryRegistered
	}
	want := map[string]bool{"ai.HostileOnly": true, "ai.FolkAttacked": false, "quest.Talker": true, "quest.Timer": false, "quest.Unraised": false}
	if !reflect.DeepEqual(registered, want) {
		t.Fatalf("registered = %v, want %v", registered, want)
	}
	if n := logs.count("script: refused"); n != 3 {
		t.Fatalf("refusals logged %d times, want 3: %s", n, logs)
	}
	if got := pathsOf(r.scripts(1, EventAttacked)); len(got) != 0 {
		t.Fatalf("a refused script registered: %v", got)
	}
	// The id with no template is skipped silently, as the reference does.
	if got := pathsOf(r.scripts(99, EventAttacked)); len(got) != 0 {
		t.Fatalf("id without a template bound: %v", got)
	}

	// Production raises talk and first talk on both NPC kinds, and
	// attacked on hostile NPCs only.
	catalog["quest.FolkFirstTalk"] = quest(Bindings{EventFirstTalk: {1}})
	catalog["quest.HostileFirstTalk"] = quest(Bindings{EventFirstTalk: {2}})
	r = Build(listOf("quest.Talker", "ai.HostileOnly", "ai.FolkAttacked", "quest.FolkFirstTalk", "quest.HostileFirstTalk"), catalog, Config{KindOf: kindOf, Log: zerolog.Nop()})
	var got []entryState
	for _, e := range r.entries {
		got = append(got, e.state)
	}
	if want := []entryState{entryRegistered, entryRegistered, entryRefused, entryRegistered, entryRegistered}; !slices.Equal(got, want) {
		t.Fatalf("production gate states = %v, want %v: talk, hostile attacked and first talk registered; folk attacked refused", got, want)
	}
}

// TestBehavesNamesTheIDsABehaviorIsBoundTo: an NPC id is behavior-bound
// when a behavior registered on it for any event; a quest bound to it, a
// behavior with no hook and an id with no template do not count.
func TestBehavesNamesTheIDsABehaviorIsBoundTo(t *testing.T) {
	catalog := Catalog{
		"ai.Created": func() Script {
			return Script{Behavior: true, NPCs: []int32{1, 99}, Hooks: Hooks{OnCreated: func(*Script, Created) {}}}
		},
		"ai.Idle":     func() Script { return Script{Behavior: true, NPCs: []int32{2}} },
		"quest.Kills": quest(Bindings{EventAttacked: {3}}),
	}
	kindOf := func(id int32) (NPCKind, bool) { return KindHostile, id != 99 }
	r := Build(listOf("ai.Created", "ai.Idle", "quest.Kills"), catalog, RaiseAll(Config{KindOf: kindOf, Log: zerolog.Nop()}))
	for id, want := range map[int32]bool{1: true, 2: false, 3: false, 99: false} {
		if got := r.Behaves(id); got != want {
			t.Errorf("Behaves(%d) = %v, want %v", id, got, want)
		}
	}
	if (*Registry)(nil).Behaves(1) {
		t.Error("a nil registry binds a behavior")
	}
}

// TestRegistryHasNoWritePath pins the registry's exported surface to
// reads and hook raises: nothing changes it after Build. HostileDecayed
// and PlayerDetached change only the timers, a container of their own;
// CharacterCreated only the new character's journal.
func TestRegistryHasNoWritePath(t *testing.T) {
	var got []string
	typ := reflect.TypeFor[*Registry]()
	for i := range typ.NumMethod() {
		got = append(got, typ.Method(i).Name)
	}
	if want := []string{"AbnormalStatusChanged", "Behaves", "CharacterCreated", "Dump", "FirstTalk", "FolkCreated", "FolkDecayed", "FolkDying", "FolkMoveToFinished", "FolkNoDesire", "HostileAttacked", "HostileCreated", "HostileDecayed", "HostileDying", "HostileMoveToFinished", "HostileNoDesire", "HostileOutOfTerritory", "HostilePartyAttacked", "Interact", "Invoke", "ItemUsed", "JournalQuest", "PlayerDetached", "QuestEvent", "QuestWindow", "TutorialEvent", "ZoneEnterIDs", "ZoneEntered"}; !slices.Equal(got, want) {
		t.Fatalf("Registry methods = %v, want only the reads %v; a new method must not change the registry", got, want)
	}
	for i := range reflect.TypeFor[Registry]().NumField() {
		if f := reflect.TypeFor[Registry]().Field(i); f.IsExported() {
			t.Fatalf("Registry.%s is exported", f.Name)
		}
	}
}
