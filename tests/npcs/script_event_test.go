package npcs

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The script-event fixture's NPC templates: a sentinel kind that both
// sends and hears script events, its civilian counterpart, and a deaf kind
// no script hears through.
const (
	sentinelID     = 21301
	deafID         = 21302
	folkSentinelID = 21303
)

// eventLog records the script-event hooks of the fixture's scripts and the
// sends returning, one line each.
type eventLog struct {
	mu    sync.Mutex
	lines []string
	// names maps an NPC object id to its fixture name.
	names map[int32]string
}

func (l *eventLog) add(line string) {
	l.mu.Lock()
	l.lines = append(l.lines, line)
	l.mu.Unlock()
}

func (l *eventLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := l.lines
	l.lines = nil
	return out
}

func (l *eventLog) hook(who string) func(*script.Script, script.ScriptEvent) {
	return func(_ *script.Script, e script.ScriptEvent) {
		l.add(fmt.Sprintf("%s %s event=%d arg1=%d arg2=%d", who, l.names[e.NPC.ObjectID()], e.EventID, e.Arg1, e.Arg2))
	}
}

// bootScriptEvents boots the sentinel behavior and, listed after it, a
// quest bound to the sentinels' script event; the character enters the
// world.
func bootScriptEvents(t *testing.T) (*gameservertest.Server, int32, *eventLog) {
	t.Helper()
	log := &eventLog{names: map[int32]string{}}
	kinds := map[int32]script.NPCKind{sentinelID: script.KindHostile, deafID: script.KindHostile, folkSentinelID: script.KindFolk}
	list := []script.Listing{{Path: "ai.Sentinel"}, {Path: "quest.Listener"}}
	catalog := script.Catalog{
		"ai.Sentinel": func() script.Script {
			return script.Script{Behavior: true, NPCs: []int32{sentinelID, folkSentinelID}, Hooks: script.Hooks{OnScriptEvent: log.hook("sentinel")}}
		},
		"quest.Listener": func() script.Script {
			return script.Script{Bind: script.Bindings{script.EventScriptEvent: {sentinelID, folkSentinelID}}, Hooks: script.Hooks{OnScriptEvent: log.hook("listener")}}
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{spawnMonster(sentinelID, "Sentinel"), spawnMonster(deafID, "Deaf"), gameservertest.FolkTemplate("Folk", folkSentinelID)})),
		gameservertest.WithNPCScripts(kinds, list, catalog),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	enterWorld(t, srv, c)
	return srv, srv.SoleObjectID(t), log
}

// TestScriptEventsReachBoundScriptsInline: a script event sent to one NPC
// runs, before the send returns, every script bound to that NPC's script
// event in list order, with both arguments. A radius broadcast reaches
// every other NPC within the radius, body to body, with 0 as the second
// argument, and the extended broadcast passes its own; the sender, an NPC
// out of range and an NPC no script hears through get nothing.
func TestScriptEventsReachBoundScriptsInline(t *testing.T) {
	t.Parallel()
	srv, objID, log := bootScriptEvents(t)
	spawn := func(name string, id, x int) *script.NPC {
		h := srv.SpawnHostileNPCTemplateAt(t, spawnMonster(id, name), location.Location{X: x, Y: 20, Z: 30})
		log.names[h.ObjectID()] = name
		return script.NewNPC(h)
	}
	sender := spawn("sender", sentinelID, 40)
	spawn("near", sentinelID, 655)
	spawn("deaf", deafID, 100)
	spawn("far", sentinelID, 700)
	srv.Settle(t)

	send := func(fn func(s *script.Script)) []string {
		t.Helper()
		if !srv.RunScript(t, objID, "Listener", func(s *script.Script, _ *script.Player) {
			fn(s)
			log.add("returned")
		}) {
			t.Fatal("the send panicked")
		}
		return log.take()
	}

	// near stands 615 from the sender, centre to centre: within 600 once
	// both bodies' collision radii (10 each) are added; far, at 660, is not.
	if got, want := send(func(s *script.Script) { s.BroadcastScriptEvent(sender, 10001, 7, 600) }), []string{
		"sentinel near event=10001 arg1=7 arg2=0",
		"listener near event=10001 arg1=7 arg2=0",
		"returned",
	}; !slices.Equal(got, want) {
		t.Fatalf("broadcast = %q, want %q", got, want)
	}
	if got, want := send(func(s *script.Script) { s.BroadcastScriptEventEx(sender, 10016, 3, 9, 1000) }), 5; len(got) != want || got[len(got)-1] != "returned" {
		t.Fatalf("extended broadcast = %q, want two NPCs heard by two scripts each, then the return", got)
	} else {
		slices.Sort(got[:4])
		if want := []string{
			"listener far event=10016 arg1=3 arg2=9", "listener near event=10016 arg1=3 arg2=9",
			"sentinel far event=10016 arg1=3 arg2=9", "sentinel near event=10016 arg1=3 arg2=9",
		}; !slices.Equal(got[:4], want) {
			t.Fatalf("extended broadcast = %q, want %q", got[:4], want)
		}
	}
	if got, want := send(func(s *script.Script) { s.SendScriptEvent(sender, 5, 1, 2) }), []string{
		"sentinel sender event=5 arg1=1 arg2=2",
		"listener sender event=5 arg1=1 arg2=2",
		"returned",
	}; !slices.Equal(got, want) {
		t.Fatalf("send = %q, want %q", got, want)
	}
}

// TestScriptEventsFromAndToCivilians: a civilian NPC's radius broadcast
// reaches the civilian and hostile NPCs around it, each heard by every
// script bound to it, and leaves out the sender and an NPC out of range.
func TestScriptEventsFromAndToCivilians(t *testing.T) {
	t.Parallel()
	srv, objID, log := bootScriptEvents(t)
	folk := func(name string, x int) *script.NPC {
		f := srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("Folk", folkSentinelID), location.Location{X: x, Y: 20, Z: 30})
		log.names[f.ObjectID()] = name
		return script.NPCOf(f)
	}
	sender := folk("sender", 40)
	folk("civilian", 240)
	folk("far", 1400)
	h := srv.SpawnHostileNPCTemplateAt(t, spawnMonster(sentinelID, "Sentinel"), location.Location{X: 340, Y: 20, Z: 30})
	log.names[h.ObjectID()] = "hostile"
	srv.Settle(t)

	if !srv.RunScript(t, objID, "Listener", func(s *script.Script, _ *script.Player) {
		s.BroadcastScriptEvent(sender, 10002, 4, 600)
		log.add("returned")
	}) {
		t.Fatal("the send panicked")
	}
	got := log.take()
	if len(got) != 5 || got[4] != "returned" {
		t.Fatalf("broadcast = %q, want two NPCs heard by two scripts each, then the return", got)
	}
	slices.Sort(got[:4])
	if want := []string{
		"listener civilian event=10002 arg1=4 arg2=0", "listener hostile event=10002 arg1=4 arg2=0",
		"sentinel civilian event=10002 arg1=4 arg2=0", "sentinel hostile event=10002 arg1=4 arg2=0",
	}; !slices.Equal(got[:4], want) {
		t.Fatalf("broadcast = %q, want %q", got[:4], want)
	}
}
