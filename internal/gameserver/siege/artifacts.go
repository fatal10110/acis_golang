package siege

import (
	"slices"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
)

// Engrave records that cl engraved the ruler on artifactID, one of the
// besieged castle's placed artifacts: the attackers, then the owner and the
// defenders, are told. Once every artifact of the castle carries cl's
// ruler, all of them are wiped and Engrave reports true: the castle is cl's
// to take. An artifact not placed for the castle records nothing and tells
// no one.
func (s *Siege) Engrave(cl *clan.Clan, artifactID int32) bool {
	artifacts := s.castle.ArtifactIDs()
	if cl == nil || !slices.Contains(artifacts, artifactID) {
		return false
	}
	var fx effects
	s.e.mu.Lock()
	if s.engraved == nil {
		s.engraved = map[int32]int32{}
	}
	s.engraved[artifactID] = cl.ID()
	s.tellLocked(&fx, Message{ID: MsgClanEngravedRuler, Text: cl.Name()}, SideAttacker, SideDefender)
	taken := !slices.ContainsFunc(artifacts, func(id int32) bool { return s.engraved[id] != cl.ID() })
	if taken {
		clear(s.engraved)
	}
	s.e.mu.Unlock()
	fx.run()
	return taken
}

// Tell tells m to the members of the clans on each of sides in turn:
// SideAttacker for the attackers, SideDefender for the owner and the
// defenders.
func (s *Siege) Tell(m Message, sides ...Side) {
	var fx effects
	s.e.mu.Lock()
	s.tellLocked(&fx, m, sides...)
	s.e.mu.Unlock()
	fx.run()
}
