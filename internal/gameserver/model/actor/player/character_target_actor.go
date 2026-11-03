package player

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
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

// The NPC, corpse and door facts never apply to players: every method
// below is the neutral answer target resolution gives a player. The duel
// lookups are in character_duel.go; the Olympiad ones in
// character_olympiad.go; the party, clan and alliance lookups in
// character_social.go.
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

// A player is no chest, beast or manor monster.
func (c *Character) Chest() bool                              { return false }
func (c *Character) Interacted() bool                         { return false }
func (c *Character) FeedableBeast() bool                      { return false }
func (c *Character) Seedable() bool                           { return false }
func (c *Character) SpawnLocation() (location.Location, bool) { return location.Location{}, false }

// A player lists no attackers and answers no skill aggro; its reaction to
// an attack is NotifyAttacked.
func (c *Character) RecordAttacker(attackable.Combatant)                       {}
func (c *Character) SkillAttacked(attackable.Combatant, modelskill.Definition) {}
