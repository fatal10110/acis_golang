package npc

import (
	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Hostile)(nil)

// NPCs hold no ground point, summon, party, clan, duel or Olympiad state, are
// never artifacts, folk or pets, and their clan-group tags are not modeled
// yet: every method below is the neutral answer target resolution already
// gives an NPC.
func (h *Hostile) CanSeePoint(int, int, int) bool    { return true }
func (h *Hostile) GroundTarget() (x, y, z int)       { return 0, 0, 0 }
func (h *Hostile) Summon() (skilltarget.Actor, bool) { return nil, false }
func (h *Hostile) OlympiadMode() bool                { return false }
func (h *Hostile) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}
func (h *Hostile) IsInParty() bool                      { return false }
func (h *Hostile) PartyContains(skilltarget.Actor) bool { return false }
func (h *Hostile) IsInSameParty(skilltarget.Actor) bool { return false }
func (h *Hostile) IsInSameClan(skilltarget.Actor) bool  { return false }
func (h *Hostile) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (h *Hostile) HasClan() bool                        { return false }
func (h *Hostile) DuelID() int32                        { return 0 }
func (h *Hostile) DuelTeam() int                        { return 0 }
func (h *Hostile) MageClass() bool                      { return false }
func (h *Hostile) OlympiadStarted() bool                { return false }
func (h *Hostile) ClanGroups() []string                 { return nil }
func (h *Hostile) Folk() bool                           { return false }
func (h *Hostile) Holy() bool                           { return false }
func (h *Hostile) IsPet() bool                          { return false }
