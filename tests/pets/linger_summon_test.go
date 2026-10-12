package pets

import (
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// orderedSeeCalls records the see-creature hooks of the watching monsters
// in the order they are raised: "owner" for a player, "summon" for
// anything else that is not an NPC.
type orderedSeeCalls struct {
	mu    sync.Mutex
	lines []string
}

func (s *orderedSeeCalls) see(_ *script.Script, e script.SeeCreature) {
	creature := "summon"
	switch e.Creature.(type) {
	case *script.Player:
		creature = "owner"
	case *script.NPC:
		creature = "npc"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lines = append(s.lines, creature)
}

func (s *orderedSeeCalls) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := s.lines
	s.lines = nil
	return lines
}

// TestLingeringOwnerTeleportBringsSummon pins Player.onTeleported for an
// owner whose client detached: it lands at once, and its summon jumps with
// it (Summon.teleportTo). The observer at the destination sees both, and
// the monster there runs its see-creature hooks for the summon first, from
// the summon's own teleport, then for the owner.
func TestLingeringOwnerTeleportBringsSummon(t *testing.T) {
	t.Parallel()
	calls := &orderedSeeCalls{}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{seeWatcherID: script.KindHostile},
			[]script.Listing{{Path: "ai.SeeWatcher"}},
			script.Catalog{"ai.SeeWatcher": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{seeWatcherID}, Hooks: script.Hooks{OnSeeCreature: calls.see}}
			}}),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	dest := location.Location{X: x + 6000, Y: y, Z: z}

	// The observer waits near the destination, out of the owner's sight.
	watched := h.srv.SeedCharacterFor(t, "player2", "Watcher", 1, 0)
	observer := h.srv.DialClient(t, "player2", 1)
	observer.Send(encodeRequestGameStart(0))
	observer.Read() // SSQInfo
	observer.Read() // CharSelected
	observer.Send(encodeEnterWorld())
	drainUntilQuiet(t, observer)
	watcherObj, ok := h.srv.State.Player(watched.ID)
	if !ok {
		t.Fatal("observer missing from the world")
	}
	watcher, ok := network.OnlineCharacter(watcherObj)
	if !ok {
		t.Fatalf("observer = %T, want an online character", watcherObj)
	}
	watcher.TeleportTo(dest.X+400, dest.Y, dest.Z, 0)
	readUntilOpcode(t, observer, serverpackets.OpcodeTeleportToLocation, "observer TeleportToLocation")
	observer.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	drainUntilQuiet(t, observer)
	drainUntilQuiet(t, h.client)

	tmpl := gameservertest.MovingHostileTemplate("Monster")
	tmpl.ID, tmpl.TemplateID = int(seeWatcherID), int(seeWatcherID)
	h.srv.SpawnHostileNPCTemplateAt(t, tmpl, location.Location{X: dest.X + 150, Y: dest.Y, Z: dest.Z})
	pet, _ := h.spawnWolf(t)
	h.srv.Settle(t)
	drainUntilQuiet(t, observer)
	calls.take()

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("owner = %T, want an online character", obj)
	}
	h.srv.SetPlayerInCombat(t, h.ownerID, true)
	if err := h.client.Close(); err != nil {
		t.Fatal(err)
	}
	h.srv.AdvanceUntil(t, "the server noticing the connection drop", func() bool {
		return network.ClientDetached(obj)
	})
	if _, ok := h.srv.State.Summon(h.ownerID); !ok {
		t.Fatal("a lingering owner's pet left the world")
	}

	owner.TeleportTo(dest.X, dest.Y, dest.Z, 0)
	h.srv.AdvanceUntil(t, "the pet lands next to its owner", func() bool {
		px, _, _ := pet.Position()
		return px == dest.X
	})
	h.srv.Settle(t)
	if got := calls.take(); !slices.Equal(got, []string{"summon", "owner"}) {
		t.Fatalf("see-creature calls on the landing = %q, want [summon owner]", got)
	}
	seen := seenObjects(t, observer)
	if !seen[serverpackets.OpcodeCharInfo][h.ownerID] {
		t.Fatalf("observer got no CharInfo of the owner %d at the destination", h.ownerID)
	}
	if !seen[serverpackets.OpcodeNPCInfo][pet.ObjectID()] {
		t.Fatalf("observer got no NpcInfo of the pet %d at the destination", pet.ObjectID())
	}
}

// seenObjects reads c until quiet and returns, per CharInfo and NpcInfo
// opcode, the object ids the frames describe.
func seenObjects(t *testing.T, c *testsupport.ScriptedClient) map[byte]map[int32]bool {
	t.Helper()
	seen := map[byte]map[int32]bool{serverpackets.OpcodeCharInfo: {}, serverpackets.OpcodeNPCInfo: {}}
	for range 100 {
		f := c.ReadWithTimeout(300 * time.Millisecond)
		if f == nil {
			return seen
		}
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeCharInfo:
			r.ReadInt32() // x
			r.ReadInt32() // y
			r.ReadInt32() // z
			r.ReadInt32() // boat
			seen[f[0]][r.ReadInt32()] = true
		case serverpackets.OpcodeNPCInfo:
			seen[f[0]][r.ReadInt32()] = true
		}
	}
	t.Fatal("client kept receiving frames after 100 reads")
	return nil
}
