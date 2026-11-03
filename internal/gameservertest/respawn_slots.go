package gameservertest

import "time"

// RespawnArmed reports whether a respawn is armed for the spawn slot named
// slot (manager.Npcs.SlotID).
func (s *Server) RespawnArmed(slot string) bool {
	id, ok := s.NpcSpawns.SlotID(slot)
	return ok && s.NpcRespawns.Tracked(id)
}

// CancelRespawn cancels the respawn armed for the spawn slot named slot,
// reporting whether one was.
func (s *Server) CancelRespawn(slot string) bool {
	id, ok := s.NpcSpawns.SlotID(slot)
	return ok && s.NpcRespawns.Cancel(id)
}

// RespawnDeadline returns when the respawn armed for the spawn slot named
// slot comes due; ok is false when none is armed.
func (s *Server) RespawnDeadline(slot string) (at time.Time, ok bool) {
	id, found := s.NpcSpawns.SlotID(slot)
	if !found {
		return time.Time{}, false
	}
	return s.NpcRespawns.Deadline(id)
}

// RespawnSlot brings back the NPC of the spawn slot named slot, as its
// respawn coming due does: the harness never fires a respawn timer.
// Nothing happens when no slot has that name.
func (s *Server) RespawnSlot(slot string) {
	if id, ok := s.NpcSpawns.SlotID(slot); ok {
		s.NpcSpawns.Respawn(id)
	}
}
