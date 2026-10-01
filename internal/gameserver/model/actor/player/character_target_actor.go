package player

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
)

var _ skilltarget.Actor = (*Character)(nil)

// Summon returns c's summon in the world, dead or alive. A summon cast still
// resolving its pets row is not yet registered, so it is not returned.
func (c *Character) Summon() (skilltarget.Actor, bool) {
	if c.world == nil {
		return nil, false
	}
	obj, ok := c.world.Summon(c.ObjectID())
	if !ok {
		return nil, false
	}
	summon, ok := obj.(skilltarget.Actor)
	return summon, ok
}

// Duel and Olympiad lookups are not modeled for players yet, and the NPC,
// corpse and door facts never apply to them: every method below is the
// neutral answer target resolution gives a player without that state. The
// party, clan and alliance lookups are in character_social.go.
func (c *Character) DuelID() int32                     { return 0 }
func (c *Character) DuelTeam() int                     { return 0 }
func (c *Character) OlympiadStarted() bool             { return false }
func (c *Character) ClanGroups() []string              { return nil }
func (c *Character) Folk() bool                        { return false }
func (c *Character) FolkOrGuard() bool                 { return false }
func (c *Character) MonsterKind() bool                 { return false }
func (c *Character) Undead() bool                      { return false }
func (c *Character) Holy() bool                        { return false }
func (c *Character) Unlockable() bool                  { return false }
func (c *Character) IsPet() bool                       { return false }
func (c *Character) HasCorpse() bool                   { return false }
func (c *Character) CorpseDeadline() (time.Time, bool) { return time.Time{}, false }
func (c *Character) CorpseTime() time.Duration         { return 0 }
func (c *Character) Spoiled() bool                     { return false }
func (c *Character) Seeded() bool                      { return false }
