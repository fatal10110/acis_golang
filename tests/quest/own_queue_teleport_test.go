package quest

import (
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	teleportQuest = "Q952_TeleportOrder"
	// teleporterID is a civilian NPC the quest talks through.
	teleporterID = 30986
)

// teleportOrderWorld is a player beside an NPC whose quest teleports it
// from a quest event, from a timer bound to the player alone, and from an
// engine timer reading the player out of a cell.
type teleportOrderWorld struct {
	*dialogWorld
	char *player.Character
	// dest is where every teleport lands.
	dest location.Location
	// moved records, per hook, whether the player stood at dest right
	// after the TeleportTo call returned.
	moved map[string]*atomic.Bool
	fired *atomic.Int32
}

func bootTeleportOrder(t *testing.T) *teleportOrderWorld {
	t.Helper()
	w := &teleportOrderWorld{moved: map[string]*atomic.Bool{"event": {}, "own": {}, "stored": {}}, fired: &atomic.Int32{}}
	held := &script.PlayerCell{}
	teleport := func(hook string, p *script.Player) {
		p.TeleportTo(script.Loc{X: w.dest.X, Y: w.dest.Y, Z: w.dest.Z}, 0)
		x, y, z := w.char.Position()
		w.moved[hook].Store(location.Location{X: x, Y: y, Z: z} == w.dest)
	}
	catalog := script.Catalog{
		"quest." + teleportQuest: func() script.Script {
			return script.Script{
				Title: "Teleport Order", QuestID: 952,
				Bind: script.Bindings{script.EventQuestStart: {teleporterID}, script.EventTalked: {teleporterID}},
				Hooks: script.Hooks{
					OnEvent: func(s *script.Script, e script.Event) string {
						switch e.Name {
						case "go":
							teleport("event", e.Player)
							return "after.htm"
						case "own":
							s.StartTimer("own", nil, e.Player, 100*time.Millisecond)
						case "stored":
							held.Set(e.Player)
							s.StartTimer("stored", nil, nil, 100*time.Millisecond)
						}
						return ""
					},
					OnTimer: func(_ *script.Script, e script.Timer) string {
						defer w.fired.Add(1)
						switch e.Name {
						case "own":
							teleport("own", e.Player)
							return "<html><body>landed</body></html>"
						case "stored":
							teleport("stored", held.Get())
						}
						return ""
					},
				},
			}
		},
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Traveler", 20, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		gameservertest.WithHTMLPages(map[string]string{
			"test/any.htm": anyBypassPage,
			"script/quest/" + teleportQuest + "/after.htm": "<html><body>after %objectId%</body></html>",
		}),
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{teleporterID: script.KindFolk}, []script.Listing{{Path: "quest." + teleportQuest}}, catalog),
	)
	objID := srv.SoleObjectID(t)
	enterWorld(t, srv)
	x, y, z := srv.PlayerPosition(t, objID)
	w.dialogWorld = &dialogWorld{srv: srv, player: objID, at: location.Location{X: x, Y: y, Z: z}, roles: map[int32]string{}, npcs: map[string]int32{}}
	w.dest = location.Location{X: x + 50, Y: y, Z: z}
	w.spawnFolk(t, "teleporter", teleporterID, 20, 0, 0)
	srv.RunQuest(t, objID, teleportQuest, func(_ *script.Quests, c *player.Character, _ *script.Script) { w.char = c })
	// The talk targets the NPC and makes it the last quest NPC, as before
	// the reference trace's event.
	w.interact(t, w.npcs["teleporter"])
	w.offerAll(t)
	return w
}

// teleportNames names the frames a teleport and a dialog answer send; any
// other frame is left out.
var teleportNames = map[byte]string{
	serverpackets.OpcodeActionFailed:       "ActionFailed",
	serverpackets.OpcodeTargetUnselected:   "TargetUnselected",
	serverpackets.OpcodeTeleportToLocation: "TeleportToLocation",
	serverpackets.OpcodeDeleteObject:       "DeleteObject",
	serverpackets.OpcodeNpcHtmlMessage:     "NpcHtmlMessage",
}

func teleportFrames(frames [][]byte) []string {
	var out []string
	for _, f := range frames {
		if name, ok := teleportNames[f[0]]; ok {
			out = append(out, name)
		}
	}
	return out
}

// abortAndTeleport is what a teleport sends its player, as the reference
// trace of Q247's teleporting event has it before the page: the abort of
// every action and of the target, the move, and the NPC left behind.
var abortAndTeleport = []string{
	"ActionFailed", "ActionFailed", "ActionFailed", "ActionFailed", "ActionFailed",
	"TargetUnselected", "TeleportToLocation", "DeleteObject",
}

// TestQuestEventTeleportLandsBeforeThePage: a quest event that teleports
// its player and answers a page teleports inside the call, as the
// reference's Player.teleportTo does: the hook reads the new position right
// after the call, and the teleport's packets go out before the page.
func TestQuestEventTeleportLandsBeforeThePage(t *testing.T) {
	t.Parallel()
	w := bootTeleportOrder(t)
	w.srv.Client.Send(encodeBypass("Quest " + teleportQuest + " go"))
	got := teleportFrames(w.srv.ReadQueued(t, w.srv.Client))
	want := append(slices.Clone(abortAndTeleport), "NpcHtmlMessage", "ActionFailed")
	if !slices.Equal(got, want) {
		t.Fatalf("event teleport frames:\n got %q\nwant %q", got, want)
	}
	if !w.moved["event"].Load() {
		t.Fatal("the event hook read the old position after TeleportTo")
	}
	if x, y, z := w.srv.PlayerPosition(t, w.player); (location.Location{X: x, Y: y, Z: z}) != w.dest {
		t.Fatalf("player at %d,%d,%d, want %v", x, y, z, w.dest)
	}
}

// TestOwnTimerTeleportLandsBeforeThePage: a timer bound to the player
// alone runs on the player's own queue, so its teleport also runs inside
// the call and lands before the page the timer answers.
func TestOwnTimerTeleportLandsBeforeThePage(t *testing.T) {
	t.Parallel()
	w := bootTeleportOrder(t)
	w.srv.Client.Send(encodeBypass("Quest " + teleportQuest + " own"))
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.AdvanceUntil(t, "the player's timer fires", func() bool { return w.fired.Load() > 0 })
	got := teleportFrames(w.srv.ReadQueued(t, w.srv.Client))
	want := append(slices.Clone(abortAndTeleport), "NpcHtmlMessage", "ActionFailed")
	if !slices.Equal(got, want) {
		t.Fatalf("timer teleport frames:\n got %q\nwant %q", got, want)
	}
	if !w.moved["own"].Load() {
		t.Fatal("the timer hook read the old position after TeleportTo")
	}
}

// TestStoredHandleTeleportPostsFromAnotherQueue: a handle the event hook
// stored in a cell, used by a hook on another queue (an engine timer), still
// asks the teleport of the player's own queue: that hook reads the old
// position, and the player lands once its queue runs the teleport.
func TestStoredHandleTeleportPostsFromAnotherQueue(t *testing.T) {
	t.Parallel()
	w := bootTeleportOrder(t)
	w.srv.Client.Send(encodeBypass("Quest " + teleportQuest + " stored"))
	w.srv.ReadQueued(t, w.srv.Client)
	w.srv.AdvanceUntil(t, "the engine timer fires", func() bool { return w.fired.Load() > 0 })
	if w.moved["stored"].Load() {
		t.Fatal("a teleport asked from another queue ran inside the call")
	}
	w.srv.AdvanceUntil(t, "the posted teleport lands", func() bool {
		x, y, z := w.srv.PlayerPosition(t, w.player)
		return location.Location{X: x, Y: y, Z: z} == w.dest
	})
	if got := teleportFrames(w.srv.ReadQueued(t, w.srv.Client)); !slices.Equal(got, abortAndTeleport) {
		t.Fatalf("posted teleport frames:\n got %q\nwant %q", got, abortAndTeleport)
	}
}
