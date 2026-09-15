package player

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Character)(nil)

// Party, clan, alliance, duel, Olympiad and summon lookups are not modeled
// for players yet, and the NPC, corpse and door facts never apply to them:
// every method below is the neutral answer target resolution already gives
// a player without that state.
func (c *Character) Summon() (skilltarget.Actor, bool) { return nil, false }
func (c *Character) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool) bool {
	return true
}
func (c *Character) IsInParty() bool                      { return false }
func (c *Character) PartyContains(skilltarget.Actor) bool { return false }
func (c *Character) IsInSameParty(skilltarget.Actor) bool { return false }
func (c *Character) IsInSameClan(skilltarget.Actor) bool  { return false }
func (c *Character) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (c *Character) HasClan() bool                        { return false }
func (c *Character) DuelID() int32                        { return 0 }
func (c *Character) DuelTeam() int                        { return 0 }
func (c *Character) MageClass() bool                      { return false }
func (c *Character) OlympiadStarted() bool                { return false }
func (c *Character) ClanGroups() []string                 { return nil }
func (c *Character) Folk() bool                           { return false }
func (c *Character) FolkOrGuard() bool                    { return false }
func (c *Character) MonsterKind() bool                    { return false }
func (c *Character) Undead() bool                         { return false }
func (c *Character) Holy() bool                           { return false }
func (c *Character) Unlockable() bool                     { return false }
func (c *Character) IsPet() bool                          { return false }
func (c *Character) HasCorpse() bool                      { return false }
func (c *Character) CorpseDeadline() (time.Time, bool)    { return time.Time{}, false }
func (c *Character) CorpseTime() time.Duration            { return 0 }
func (c *Character) Spoiled() bool                        { return false }
func (c *Character) Seeded() bool                         { return false }
