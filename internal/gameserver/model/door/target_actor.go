package door

import (
	"time"

	skilltarget "github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

var _ skilltarget.Actor = (*Object)(nil)

// CanSeeTarget reports line of sight from this door to t, as when an area
// skill centered on the door picks its splash victims: the geodata query runs
// from the door's position raised by its eye height, a t that is itself a
// geodata object (another closed door) is left out of the query, and this
// door's own geodata stays in it, so a closed door standing in its own
// footprint sees nothing. A door with no query attached sees everything.
func (o *Object) CanSeeTarget(t skilltarget.Actor) bool {
	if o.sight == nil {
		return true
	}
	ox, oy, oz := o.Position()
	tx, ty, tz := t.Position()
	ignore, _ := t.(GeoShape)
	return o.sight.CanSeeActorIgnoring(ox, oy, oz, o.CollisionHeight(), tx, ty, tz, t.CollisionHeight(), ignore)
}

// A door only answers attackability and unlocking; it never casts, moves,
// leaves a corpse when broken or belongs to any social group, so every
// method below is the neutral answer.
func (o *Object) CanSeePoint(int, int, int) bool                 { return true }
func (o *Object) EffectRangeInPeaceZone(int, int, int, int) bool { return false }
func (o *Object) InPeaceZone() bool                              { return false }
func (o *Object) GroundTarget() (x, y, z int)                    { return 0, 0, 0 }
func (o *Object) Summon() (skilltarget.Actor, bool)              { return nil, false }
func (o *Object) Owner() (attackable.Combatant, bool)            { return nil, false }
func (o *Object) OlympiadMode() bool                             { return false }
func (o *Object) CanCastOnPlayable(skilltarget.Actor, *modelskill.Definition, bool, bool, bool) bool {
	return true
}
func (o *Object) IsInParty() bool                      { return false }
func (o *Object) PartyContains(skilltarget.Actor) bool { return false }
func (o *Object) IsInSameParty(skilltarget.Actor) bool { return false }
func (o *Object) IsInSameClan(skilltarget.Actor) bool  { return false }
func (o *Object) IsInSameAlly(skilltarget.Actor) bool  { return false }
func (o *Object) HasClan() bool                        { return false }
func (o *Object) DuelID() int32                        { return 0 }
func (o *Object) DuelTeam() int                        { return 0 }
func (o *Object) MageClass() bool                      { return false }
func (o *Object) OlympiadStarted() bool                { return false }
func (o *Object) ClanGroups() []string                 { return nil }
func (o *Object) Folk() bool                           { return false }
func (o *Object) FolkOrGuard() bool                    { return false }
func (o *Object) MonsterKind() bool                    { return false }
func (o *Object) Undead() bool                         { return false }
func (o *Object) Holy() bool                           { return false }
func (o *Object) IsPet() bool                          { return false }
func (o *Object) HasCorpse() bool                      { return false }
func (o *Object) CorpseDeadline() (time.Time, bool)    { return time.Time{}, false }
func (o *Object) CorpseTime() time.Duration            { return 0 }
func (o *Object) Spoiled() bool                        { return false }
func (o *Object) Seeded() bool                         { return false }

// CollisionHeight is half the door's blocking height, the way every other
// creature reports half its body: line of sight to a door ends at the
// configured share of its full height above its Z.
func (o *Object) CollisionHeight() float64 { return float64(o.height) / 2 }

// A door is no chest, beast or manor monster, and holds no resurrection,
// siege or festival standing of its own.
func (o *Object) Chest() bool                              { return false }
func (o *Object) Interacted() bool                         { return false }
func (o *Object) FeedableBeast() bool                      { return false }
func (o *Object) Seedable() bool                           { return false }
func (o *Object) SpawnLocation() (location.Location, bool) { return location.Location{}, false }
func (o *Object) ReviveOffer() (pending, forPet bool)      { return false, false }
func (o *Object) InSiegeZone() bool                        { return false }
func (o *Object) SiegeState() int32                        { return 0 }
func (o *Object) FestivalParticipant() bool                { return false }

// A door neither lists attackers nor reacts to an offensive skill.
func (o *Object) RecordAttacker(attackable.Combatant)                       {}
func (o *Object) NotifyAttacked(attackable.Combatant)                       {}
func (o *Object) SkillAttacked(attackable.Combatant, modelskill.Definition) {}
