package pets

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// seeWatcherID is the monster template the recording behavior is bound to.
const seeWatcherID = int32(20062)

// seeCalls records the see-creature hooks of the watching monsters as
// "<monster> <creature>", the creature named "owner" for a player and
// "summon" for anything else that is not an NPC. Hooks run on whichever
// goroutine raises them, so the lines are guarded by mu.
type seeCalls struct {
	mu    sync.Mutex
	names map[int32]string
	lines []string
}

func (s *seeCalls) see(_ *script.Script, e script.SeeCreature) {
	s.mu.Lock()
	defer s.mu.Unlock()
	creature := "summon"
	switch e.Creature.(type) {
	case *script.Player:
		creature = "owner"
	case *script.NPC:
		creature = "npc"
	}
	s.lines = append(s.lines, fmt.Sprintf("%s %s", s.names[e.NPC.ObjectID()], creature))
}

func (s *seeCalls) take() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines := s.lines
	s.lines = nil
	slices.Sort(lines)
	return lines
}

func (s *seeCalls) want(t *testing.T, what string, want ...string) {
	t.Helper()
	if got := s.take(); !slices.Equal(got, want) {
		t.Fatalf("see-creature calls %s = %q, want %q", what, got, want)
	}
}

// TestSummonArrivalTellsNearbyMonsters pins the see-creature re-fire of a
// summon: a pet called out tells each monster with it in sight range, and
// a pet that follows its owner's teleport tells the monsters around its
// landing point once the owner's client reports it appeared. The owner's
// own landing tells nobody: its region comes back from the Appearing, which
// does not scan. The monsters are off the AI task, so no tick scan runs.
func TestSummonArrivalTellsNearbyMonsters(t *testing.T) {
	t.Parallel()
	calls := &seeCalls{names: map[int32]string{}}
	h := bootOwnerWithCollarOpts(t, []gameservertest.Option{
		gameservertest.WithNPCScripts(map[int32]script.NPCKind{seeWatcherID: script.KindHostile},
			[]script.Listing{{Path: "ai.SeeWatcher"}},
			script.Catalog{"ai.SeeWatcher": func() script.Script {
				return script.Script{Behavior: true, NPCs: []int32{seeWatcherID}, Hooks: script.Hooks{OnSeeCreature: calls.see}}
			}}),
	})
	x, y, z := h.srv.PlayerPosition(t, h.ownerID)
	watcher := func(name string, at location.Location) {
		t.Helper()
		tmpl := gameservertest.MovingHostileTemplate("Monster")
		tmpl.ID, tmpl.TemplateID = int(seeWatcherID), int(seeWatcherID)
		m := h.srv.SpawnHostileNPCTemplateAt(t, tmpl, at)
		calls.mu.Lock()
		calls.names[m.ObjectID()] = name
		calls.mu.Unlock()
	}
	watcher("home", location.Location{X: x + 200, Y: y, Z: z})
	dest := location.Location{X: x + 6000, Y: y, Z: z}
	watcher("landing", location.Location{X: dest.X + 150, Y: dest.Y, Z: dest.Z})
	drainUntilQuiet(t, h.client)
	calls.want(t, "before the pet is called")

	pet, _ := h.spawnWolf(t)
	h.srv.Settle(t)
	calls.want(t, "when the pet is called out", "home summon")

	obj, ok := h.srv.State.Player(h.ownerID)
	if !ok {
		t.Fatal("owner missing from the world")
	}
	owner, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("owner = %T, want an online character", obj)
	}
	owner.TeleportTo(dest.X, dest.Y, dest.Z, 0)
	readUntilOpcode(t, h.client, serverpackets.OpcodeTeleportToLocation, "owner TeleportToLocation")
	h.srv.Settle(t)
	calls.want(t, "on the owner's jump")

	// The pet's jump is shown around its old position, which the owner
	// left: the owner's client sees no TeleportToLocation for it.
	h.client.Send(encodeSingleOpcode(clientpackets.OpcodeAppearing))
	h.srv.AdvanceUntil(t, "the pet lands next to its owner", func() bool {
		px, _, _ := pet.Position()
		return px == dest.X
	})
	h.srv.Settle(t)
	drainUntilQuiet(t, h.client)
	got := calls.take()
	if !slices.Equal(got, []string{"landing summon"}) {
		t.Fatalf("see-creature calls on the landing = %q, want [landing summon]: the pet's jump tells the monster at the landing point, the owner's landing nobody", strings.Join(got, ", "))
	}
}
