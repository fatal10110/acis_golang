package npcs

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// heldWolf is a plain script bound to the wolf's created event: on the
// wolf's first life it keeps a handle on it and starts a timer bound to it
// and to no player, which stays on the engine queue across the wolf's
// decay. The timer records the NPC it fires with and has it speak.
type heldWolf struct {
	held    atomic.Pointer[script.NPC]
	started atomic.Bool
	mu      sync.Mutex
	fired   []heldFire
}

// heldFire is what the timer saw of its NPC as it fired, and whether the
// global memo still resolved the object id stored in the first life.
type heldFire struct {
	objectID   int32
	decayed    bool
	memoLookup bool
}

func (h *heldWolf) script() script.Script {
	return script.Script{Bind: script.Bindings{script.EventCreated: {lifeWolfID}}, Hooks: script.Hooks{
		OnCreated: func(s *script.Script, e script.Created) {
			if h.started.CompareAndSwap(false, true) {
				h.held.Store(e.NPC)
				s.GlobalMemo().Set("held_wolf", int(e.NPC.ObjectID()))
				s.StartTimer("watch", e.NPC, nil, 2*time.Minute)
			}
		},
		OnTimer: func(s *script.Script, e script.Timer) string {
			fire := heldFire{objectID: e.NPC.ObjectID(), decayed: e.NPC.Decayed(), memoLookup: s.GlobalMemo().Creature("held_wolf") != nil}
			h.mu.Lock()
			h.fired = append(h.fired, fire)
			h.mu.Unlock()
			e.NPC.Say("still here")
			return ""
		},
	}}
}

// fires returns what the timer saw so far.
func (h *heldWolf) fires() []heldFire {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]heldFire(nil), h.fired...)
}

// A handle held across the wolf's decay and its slot's respawn acts on the
// respawned wolf, as the reference's reused NPC object does: a plain
// script's timer bound to the wolf, started in its first life, fires after
// the respawn with the new life, alive, and the new life speaks. Between
// the decay and the respawn the held handle reads the decayed wolf; once
// the slot respawns it reads the new object id. The global memo lookup is
// not touched: an object id stored in the first life no longer resolves
// once the wolf has respawned.
func TestHeldNPCHandleFollowsItsSlotAcrossARespawn(t *testing.T) {
	t.Parallel()
	h := &heldWolf{}
	kinds := map[int32]script.NPCKind{lifeWolfID: script.KindHostile}
	srv := gameservertest.Boot(t,
		gameservertest.WithCharacter("Talker", playerLevel, 0), gameservertest.WithWantChars(1),
		gameservertest.WithNPCs(lifeTemplates()),
		gameservertest.WithNpcSpawns(lifeTable(t)),
		gameservertest.WithNPCScripts(kinds, []script.Listing{{Path: "quest.HeldWolf"}}, script.Catalog{"quest.HeldWolf": h.script}),
	)
	c := srv.Client
	c.Send(encodeRequestGameStart(0))
	c.Read() // SSQInfo
	c.Read() // CharSelected
	enterWorld(t, srv, c)

	first := liveOf(t, srv, lifeWolfID).(*npc.Hostile)
	held := h.held.Load()
	if held == nil || held.ObjectID() != first.ObjectID() {
		t.Fatalf("held handle = %v, want the wolf %d", held, first.ObjectID())
	}

	onQueueOf(t, first, func() { first.Decay(srv.State, srv.NpcSpawns.RespawnHook(first.ObjectID())) })
	if !held.Decayed() || held.ObjectID() != first.ObjectID() {
		t.Fatalf("held handle before the respawn: decayed %v, id %d; want the decayed wolf %d", held.Decayed(), held.ObjectID(), first.ObjectID())
	}
	srv.NpcSpawns.Respawn(lifeWolfKey)
	srv.Settle(t)
	drainFrames(t, c)

	second := liveOf(t, srv, lifeWolfID).(*npc.Hostile)
	if second.ObjectID() == first.ObjectID() {
		t.Fatal("the respawn kept the decayed wolf's object id")
	}
	if held.Decayed() || held.ObjectID() != second.ObjectID() {
		t.Fatalf("held handle after the respawn: decayed %v, id %d; want the live wolf %d", held.Decayed(), held.ObjectID(), second.ObjectID())
	}
	srv.Advance(t, 2*time.Minute)
	frames := drainFrames(t, c)
	if got := h.fires(); len(got) != 1 || got[0] != (heldFire{objectID: second.ObjectID()}) {
		t.Fatalf("timer fires = %+v, want one with the live wolf %d", got, second.ObjectID())
	}
	if got := npcSaysOf(frames, second.ObjectID()); got != 1 {
		t.Fatalf("NpcSay frames from the respawned wolf = %d, want 1", got)
	}
	if got := npcSaysOf(frames, first.ObjectID()); got != 0 {
		t.Fatalf("NpcSay frames from the decayed wolf = %d, want none", got)
	}
}

// npcSaysOf counts the NpcSay frames spoken by the NPC objectID.
func npcSaysOf(frames [][]byte, objectID int32) int {
	n := 0
	for _, f := range frames {
		if len(f) >= 5 && f[0] == serverpackets.OpcodeNpcSay && int32(binary.LittleEndian.Uint32(f[1:5])) == objectID {
			n++
		}
	}
	return n
}
