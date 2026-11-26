package siege

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// Engrave records that cl engraved the ruler on artifactID, one of the
// besieged castle's placed artifacts: the attackers, then the owner and the
// defenders, are told. Once every artifact of the castle carries cl's
// ruler, all of them are wiped and seize gives cl the castle, reporting
// whether it did and returning what then tells of it (nil for nothing).
// Engrave reports whether cl took the castle. An artifact not placed for
// the castle records nothing and tells no one.
//
// The castle changes hands under the same hold that saw the siege under
// way, so the siege's end decides wholly before the engraving, refusing
// it, or wholly after, with the new owner (#5142). seize therefore runs
// with the engine's lock held and must not call back into the sieges;
// what it returns runs after the engraving was told, with the step's other
// effects.
func (s *Siege) Engrave(cl *clan.Clan, artifactID int32, seize func() (told func(), ok bool)) bool {
	artifacts := s.castle.ArtifactIDs()
	if cl == nil || !slices.Contains(artifacts, artifactID) {
		return false
	}
	taken := false
	s.runLifecycle(func(fx *sim.Effects) {
		if s.status != StatusInProgress {
			return
		}
		if s.engraved == nil {
			s.engraved = map[int32]int32{}
		}
		s.engraved[artifactID] = cl.ID()
		s.tellLocked(fx, Message{ID: MsgClanEngravedRuler, Text: cl.Name()}, SideAttacker, SideDefender)
		if slices.ContainsFunc(artifacts, func(id int32) bool { return s.engraved[id] != cl.ID() }) {
			return
		}
		clear(s.engraved)
		told, ok := seize()
		if !ok {
			return
		}
		if told != nil {
			fx.Add(told)
		}
		taken = true
	})
	return taken
}

// Tell tells m to the members of the clans on each of sides in turn:
// SideAttacker for the attackers, SideDefender for the owner and the
// defenders.
func (s *Siege) Tell(m Message, sides ...Side) {
	var fx sim.Effects
	s.e.mu.Lock()
	s.tellLocked(&fx, m, sides...)
	s.e.mu.Unlock()
	fx.Run()
}
