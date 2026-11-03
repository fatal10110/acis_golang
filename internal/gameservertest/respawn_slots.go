package gameservertest

import (
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/task"
)

// Slot returns the id of the live spawn slot named name
// (manager.Npcs.SlotID), failing t when no slot has that name. Resolve the
// id before a step that drops the slot to check its respawn afterwards.
func (s *Server) Slot(t testing.TB, name string) task.SlotID {
	t.Helper()
	id, ok := s.NpcSpawns.SlotID(name)
	if !ok {
		t.Fatalf("no spawn slot named %s; live slots: %q", name, slices.Sorted(maps.Values(s.NpcSpawns.Slots())))
	}
	return id
}

// RespawnArmed reports whether a respawn is armed for the spawn slot named
// slot, failing t when no slot has that name.
func (s *Server) RespawnArmed(t testing.TB, slot string) bool {
	t.Helper()
	return s.NpcRespawns.Tracked(s.Slot(t, slot))
}

// CancelRespawn cancels the respawn armed for the spawn slot named slot,
// reporting whether one was; it fails t when no slot has that name.
func (s *Server) CancelRespawn(t testing.TB, slot string) bool {
	t.Helper()
	return s.NpcRespawns.Cancel(s.Slot(t, slot))
}

// RespawnDeadline returns when the respawn armed for the spawn slot named
// slot comes due; ok is false when none is armed. It fails t when no slot
// has that name.
func (s *Server) RespawnDeadline(t testing.TB, slot string) (at time.Time, ok bool) {
	t.Helper()
	return s.NpcRespawns.Deadline(s.Slot(t, slot))
}

// RespawnSlot brings back the NPC of the spawn slot named slot, as its
// respawn coming due does: the harness never fires a respawn timer. It fails
// t when no slot has that name.
func (s *Server) RespawnSlot(t testing.TB, slot string) {
	t.Helper()
	s.NpcSpawns.Respawn(s.Slot(t, slot))
}

// RespawnsArmed returns how many spawn slots have a respawn armed, the
// dropped slots no name resolves any more included.
func (s *Server) RespawnsArmed() int {
	return s.NpcRespawns.Len()
}

// ArmedPrivates returns the names of the live script-private spawn slots
// (private#N) with a respawn armed, by N.
func (s *Server) ArmedPrivates() []string {
	var armed []int
	for id, name := range s.NpcSpawns.Slots() {
		seq, ok := strings.CutPrefix(name, "private#")
		if !ok || !s.NpcRespawns.Tracked(id) {
			continue
		}
		n, err := strconv.Atoi(seq)
		if err != nil {
			panic("gameservertest: private slot name " + name)
		}
		armed = append(armed, n)
	}
	slices.Sort(armed)
	names := make([]string, len(armed))
	for i, n := range armed {
		names[i] = "private#" + strconv.Itoa(n)
	}
	return names
}

// StrayRespawns returns how many respawns are armed for spawn slots no
// longer live. None should be: such a respawn brings nothing back.
func (s *Server) StrayRespawns() int {
	live := 0
	for id := range s.NpcSpawns.Slots() {
		if s.NpcRespawns.Tracked(id) {
			live++
		}
	}
	return s.NpcRespawns.Len() - live
}
