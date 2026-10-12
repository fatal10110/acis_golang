package npcs

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/route"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

const (
	// routeLineID is the chat line the fixture's far route node says.
	routeLineID   = 1010205
	routeLineText = "Where has he gone?"
	// ootLine is what the patrol's behavior says out of its territory.
	ootLine = "Out of my territory!"
	// clientTemplateID is a client template id unlike the fixture npc ids.
	clientTemplateID = 20001
)

// readAboutUntil reads frames, keeping those about objectID, until done
// holds for one of them; on the driven clock each wait moves the clock
// forward.
func readAboutUntil(t *testing.T, c *testsupport.ScriptedClient, objectID int32, what string, done func(npcFrame) bool) []npcFrame {
	t.Helper()
	var kept []npcFrame
	for range 400 {
		frame := c.ReadWithTimeout(300 * time.Millisecond)
		if len(frame) < 5 || wire.NewReader(frame[1:]).ReadInt32() != objectID {
			continue
		}
		fr := npcFrame{opcode: frame[0], raw: frame}
		kept = append(kept, fr)
		if done(fr) {
			return kept
		}
	}
	t.Fatalf("%s not shown; frames about the NPC = %#x", what, npcOpcodes(kept))
	return nil
}

// npcSay decodes an NpcSay frame's npc id and text.
func npcSay(t *testing.T, fr npcFrame) (npcID int32, text string) {
	t.Helper()
	if fr.opcode != serverpackets.OpcodeNpcSay {
		t.Fatalf("frame %#x, want NpcSay", fr.opcode)
	}
	r := fr.body()
	if sayType := r.ReadInt32(); sayType != serverpackets.SayTypeAll {
		t.Fatalf("NpcSay type %d, want %d", sayType, serverpackets.SayTypeAll)
	}
	return r.ReadInt32(), r.ReadString()
}

// A route arrival is settled by the AI before the route takes its step
// (NpcAI.onEvtArrived): a monster walking its route that arrives on a node
// outside its territory raises the out-of-territory hook first, and only
// then says the node's line and starts its next leg. Both lines name the
// monster by its npc id, not its client template id (NpcSay).
func TestRouteArrivalOutsideTerritoryRaisesTheHookBeforeTheRouteStep(t *testing.T) {
	t.Parallel()
	executors(t, testRouteArrivalOutsideTerritory)
}

func testRouteArrivalOutsideTerritory(t *testing.T, opts ...gameservertest.Option) {
	behavior := func() script.Script {
		return script.Script{Behavior: true, NPCs: []int32{patrolID}, Hooks: script.Hooks{
			OnCreated:        func(_ *script.Script, e script.Created) { e.NPC.AddMoveRouteDesire(patrolAlias, 10) },
			OnOutOfTerritory: func(_ *script.Script, e script.OutOfTerritory) { e.NPC.Say(ootLine) },
		}}
	}
	tune := func(patrol *npc.Template, routes route.WalkerRoutes) {
		patrol.TemplateID = clientTemplateID
		// The far node, 250 from the spawn point, lies outside the
		// monster's 200 drift range; the near one, 50 from it, inside.
		routes[patrolAlias][patrolAlias][1].NPCStringID = routeLineID
	}
	w := bootTunedRouteWorld(t, []script.Listing{{Path: "ai.Patrol"}}, script.Catalog{"ai.Patrol": behavior}, tune, opts...)
	n, _ := w.spawn(t, patrolID)
	h := n.(*npc.Hostile)
	w.tickAI(t)
	if dests, _ := legs(aboutNPC(w.tickAI(t), h.ObjectID())); len(dests) != 1 || dests[0] != w.a[patrolID] {
		t.Fatalf("patrol legs on its second AI tick = %v, want one to %v", dests, w.a[patrolID])
	}

	a, b := w.a[patrolID], w.b[patrolID]
	frames := readAboutUntil(t, w.c, h.ObjectID(), "leg back from the far node", func(fr npcFrame) bool {
		if fr.opcode != serverpackets.OpcodeMoveToLocation {
			return false
		}
		_, origin := moveLeg(fr)
		return origin == b
	})
	// Keep what the far node's arrival showed: the frames after the leg
	// that reached it.
	start := -1
	for i, fr := range frames {
		if isMoveTo(fr, b) {
			start = i + 1
		}
	}
	if start < 0 {
		t.Fatalf("no leg to the far node %v among %#x", b, npcOpcodes(frames))
	}
	arrival := frames[start:]
	want := []byte{serverpackets.OpcodeNpcSay, serverpackets.OpcodeNpcSay, serverpackets.OpcodeMoveToLocation}
	if got := npcOpcodes(arrival); string(got) != string(want) {
		t.Fatalf("frames at the far node = %#x, want the hook's NpcSay, the node's NpcSay, then MoveToLocation", got)
	}
	for i, line := range []string{ootLine, routeLineText} {
		if npcID, text := npcSay(t, arrival[i]); npcID != 1_000_000+patrolID || text != line {
			t.Fatalf("NpcSay %d = npc %d %q, want npc %d %q", i, npcID, text, 1_000_000+patrolID, line)
		}
	}
	if dest, origin := moveLeg(arrival[2]); dest != a || origin != b {
		t.Fatalf("leg after the far node = %v from %v, want %v from %v", dest, origin, a, b)
	}
}

// A civilian route walker names itself by its npc id, not its client
// template id, when a node says its line (NpcAI.onEvtArrived ->
// Npc.broadcastNpcSay -> NpcSay).
func TestRouteWalkerFolkLineNamesItsNpcID(t *testing.T) {
	t.Parallel()
	w := bootFolkWorld(t, nil)
	a := location.Location{X: w.at.X + 100, Y: w.at.Y, Z: w.at.Z}
	b := location.Location{X: w.at.X + 300, Y: w.at.Y, Z: w.at.Z}
	tmpl := walkerTemplate()
	tmpl.TemplateID = clientTemplateID
	routes := walkerRoute(route.WalkerLocation{Location: a, NPCStringID: routeLineID}, route.WalkerLocation{Location: b})
	f, _ := w.srv.SpawnRouteFolkNPCAt(t, tmpl, location.Location{X: w.at.X + 150, Y: w.at.Y, Z: w.at.Z}, routes, true)

	frames := readNpcFramesUntil(t, w.c, f, "the node's line", func(fr npcFrame) bool {
		return fr.opcode == serverpackets.OpcodeNpcSay
	})
	if npcID, text := npcSay(t, frames[len(frames)-1]); npcID != 1_000_000+walkerID || text != routeLineText {
		t.Fatalf("NpcSay = npc %d %q, want npc %d %q", npcID, text, 1_000_000+walkerID, routeLineText)
	}
}
