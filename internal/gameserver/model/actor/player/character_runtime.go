package player

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/rs/zerolog"
)

// LineOfSight is the geodata query CanSee needs to gate targeting on real
// terrain occlusion between two actors.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// PeaceZoneQuery reports whether any point within effectRange of (x, y, z) —
// sampled at the point and its four axis-aligned range offsets — falls
// inside a peace-suspending zone attached to the region containing
// (regionX, regionY). Callers pass their own position as the region anchor,
// matching the reference's caster-region-only zone lookup.
type PeaceZoneQuery interface {
	EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool
}

// SetInPvPZone records the live zone engine's current PvP membership.
func (c *Character) SetInPvPZone(inside bool) {
	c.insidePvPZone.Store(inside)
}

// InPvPZone reports whether the character is currently in a PvP zone.
func (c *Character) InPvPZone() bool {
	return c.insidePvPZone.Load()
}

// SetInPeaceZone records the live zone engine's current peace membership.
func (c *Character) SetInPeaceZone(inside bool) {
	c.insidePeaceZone.Store(inside)
}

// InPeaceZone reports whether the character is currently in a peace zone.
func (c *Character) InPeaceZone() bool {
	return c.insidePeaceZone.Load()
}

// SetInSiegeZone records the live zone engine's current siege membership.
func (c *Character) SetInSiegeZone(inside bool) {
	c.insideSiegeZone.Store(inside)
}

// InSiegeZone reports whether the character is currently in a siege zone.
func (c *Character) InSiegeZone() bool {
	return c.insideSiegeZone.Load()
}

// SetInNoSummonFriendZone records the live zone engine's current
// NoSummonFriend membership (zone.FlagNoSummonFriend), the same way
// SetInPvPZone/SetInSiegeZone track their flags.
func (c *Character) SetInNoSummonFriendZone(inside bool) {
	c.insideNoSummonFriendZone.Store(inside)
}

// NoSummonFriendZone reports whether the character currently stands inside
// a zone that blocks SUMMON_FRIEND/SUMMON_PARTY, matching
// isInsideZone(ZoneId.NO_SUMMON_FRIEND) (SummonFriend.java:113,138).
func (c *Character) NoSummonFriendZone() bool {
	return c.insideNoSummonFriendZone.Load()
}

// SetGroundTarget records the last ground-click point a ground-targeted
// skill cast (RequestExMagicSkillUseGround) resolved, reused across casts
// until the next ground click overwrites it.
func (c *Character) SetGroundTarget(x, y, z int) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.groundTarget = location.Location{X: x, Y: y, Z: z}
}

// GroundTarget returns the last recorded ground-click point.
func (c *Character) GroundTarget() (x, y, z int) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.groundTarget.X, c.groundTarget.Y, c.groundTarget.Z
}

// SetCastModifiers records the Ctrl/Shift state of the client's most recent
// skill-cast request, reused across casts until the next request overwrites
// it — the domain cast-condition check and post-cast offensive-follow
// decision read it from here once those rules exist.
func (c *Character) SetCastModifiers(ctrl, shift bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.castCtrl = ctrl
	c.castShift = shift
}

// CastModifiers returns the last recorded Ctrl/Shift cast-request state.
func (c *Character) CastModifiers() (ctrl, shift bool) {
	c.stateMu.RLock()
	defer c.stateMu.RUnlock()
	return c.castCtrl, c.castShift
}

// CanSeePoint reports whether an arbitrary world point is visible to this
// player: a geodata line-of-sight query from this player's position and eye
// height to the raw point (no height offset on the point end, matching the
// reference's ground-target LOS query), or permissive when no
// line-of-sight query is attached (e.g. in tests).
func (c *Character) CanSeePoint(x, y, z int) bool {
	if c.los == nil {
		return true
	}
	ox, oy, oz := c.Position()
	return c.los.CanSeeActor(ox, oy, oz, c.CollisionHeight(), x, y, z, 0)
}

// EffectRangeInPeaceZone reports whether the given point's effect range
// overlaps a peace-suspending zone attached to this player's own current
// region, or permissive (false) when no zone index is attached (e.g. in
// tests).
func (c *Character) EffectRangeInPeaceZone(x, y, z, effectRange int) bool {
	if c.zones == nil {
		return false
	}
	rx, ry, _ := c.Position()
	return c.zones.EffectRangeInPeaceZone(rx, ry, x, y, z, effectRange)
}

// AttachRuntime records the static template and restored inventory used by
// live combat and visibility code. Call it before exposing c to the world.
func (c *Character) AttachRuntime(tmpl *Template, inv *itemcontainer.Inventory) {
	c.runtimeTemplate = tmpl
	c.inventory = inv
	if c.roll == nil {
		c.roll = rand.IntN
	}
}

// Rules is the server-configuration slice a character's own rules read.
type Rules struct {
	RateKarmaExpLost       float64
	WeightLimitMultiplier  float64
	PerfectShieldBlockRate int
	MaxBuffsAmount         int
	DeathPenaltyChance     int
	AllowDelevel           bool
	RaidCursesDisabled     bool
	AwardPKKillPVPPoint    bool
}

// Runtime is everything a persisted Character needs to act in the live
// world besides its event sink. A nil dependency leaves the matching query
// permissive (e.g. in tests that don't exercise geodata or zones).
type Runtime struct {
	World  *world.State
	LOS    LineOfSight
	Zones  PeaceZoneQuery
	Skills skillDefinitions
	Levels *LevelTable
	Log    zerolog.Logger
	Rules  Rules
}

// Configure installs rt. Call it before exposing c to the world.
func (c *Character) Configure(rt Runtime) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.world = rt.World
	c.los = rt.LOS
	c.zones = rt.Zones
	c.skillDefs = rt.Skills
	c.levelTable = rt.Levels
	c.log = rt.Log
	c.rateKarmaExpLost = rt.Rules.RateKarmaExpLost
	c.weightLimitMultiplier = rt.Rules.WeightLimitMultiplier
	c.perfectShieldBlockRate = rt.Rules.PerfectShieldBlockRate
	c.maxBuffsAmount = rt.Rules.MaxBuffsAmount
	c.deathPenaltyChance = rt.Rules.DeathPenaltyChance
	c.allowDelevel = rt.Rules.AllowDelevel
	c.raidCursesDisabled = rt.Rules.RaidCursesDisabled
	c.awardPKKillPVPPoint = rt.Rules.AwardPKKillPVPPoint
}

// Attach installs live as this character's crowd-control/movement runtime
// state and sink as the receiver of its events. Call it once, before the
// character is published into the world. Live is written under stateMu so a
// concurrent caller (e.g. a persisted-state assertion racing live setup)
// never observes a torn pointer.
func (c *Character) Attach(live *creature.Live, sink event.Sink) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.Live = live
	c.sink = sink
}

// DetachSession marks the owning session gone. Events the session alone
// delivered are dropped from then on, and a herb can no longer be applied.
func (c *Character) DetachSession() { c.sessionDetached.Store(true) }

// SessionDetached reports whether DetachSession has run.
func (c *Character) SessionDetached() bool { return c.sessionDetached.Load() }

func (c *Character) emit(e event.Event) {
	if c.sink != nil {
		c.sink.Emit(e)
	}
}

// AddRewardItem creates and adds one kill-reward item stack to this live
// character's inventory. objectID must be allocated by the reward caller.
func (c *Character) AddRewardItem(itemID int32, count int, objectID int32) bool {
	if c.inventory == nil {
		return false
	}
	if c.inventory.AddNew(itemID, count, objectID) == nil {
		return false
	}
	return true
}

// Inventory returns the carried item collection attached by AttachRuntime,
// or nil if the character has none yet.
func (c *Character) Inventory() *itemcontainer.Inventory {
	return c.inventory
}

// ForEachKnownCombatantInRadius visits nearby combatants through the world grid.
func (c *Character) ForEachKnownCombatantInRadius(radius int, fn func(attackable.Combatant)) {
	if c.world == nil {
		return
	}
	c.world.ForEachKnownInRadius(c, radius, func(candidate world.Tracked) {
		if combatant, ok := candidate.(attackable.Combatant); ok {
			fn(combatant)
		}
	})
}

// SyncPosition moves this player's live world-grid presence to position.
func (c *Character) SyncPosition(position location.Location) {
	previous := c.CurrentLocation()
	c.locMu.Lock()
	c.Location = position
	c.locMu.Unlock()
	if c.world == nil {
		return
	}
	_ = c.world.Move(c, position.X, position.Y, position.Z)
	c.emit(event.Relocated{Previous: previous})
}

// SetLastKnownPosition records position and heading as this player's last
// known world state. Call it whenever a client-reported move is accepted,
// alongside the world-grid presence and CreatureMove position it must
// stay consistent with.
func (c *Character) SetLastKnownPosition(position location.Location, heading int) {
	c.locMu.Lock()
	c.Location = position
	c.LastHeading = heading
	c.locMu.Unlock()
}

// ObjectID returns the persistent world object id assigned to this player.
func (c *Character) ObjectID() int32 {
	return c.ID
}

// Kind reports KindPlayer.
func (c *Character) Kind() actor.Kind { return actor.KindPlayer }

// CharacterName returns this player's display name for character-name packets.
func (c *Character) CharacterName() string { return c.Name }

// LevelValue returns the player's current level for live-owned actors.
func (c *Character) LevelValue() int {
	return c.CharLevel
}

// Level satisfies the cast/target handler interfaces (cancelTarget,
// seedableTarget, spoilableTarget, sowCaster, harvestCaster, magicCaster)
// that require a Level() int method.
func (c *Character) Level() int {
	return c.CharLevel
}

// Karma satisfies the cross-package karma-gated target checks (e.g. a
// Guard's or friendly monster's attack-target rule) that type-assert for a
// Karma() int method.
func (c *Character) Karma() int {
	return c.KarmaPoints
}

// Position returns the live world position when c is spawned, otherwise the
// persisted last-known location.
func (c *Character) Position() (int, int, int) {
	if c.Visible() {
		return c.Presence.Position()
	}
	c.locMu.RLock()
	defer c.locMu.RUnlock()
	return c.Location.X, c.Location.Y, c.Location.Z
}

// Knows reports whether target is visible to this player.
func (c *Character) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(c, tracked)
}

// CanSee reports whether target is visible to this player: a geodata
// line-of-sight query between the two actors' positions and eye heights, or
// permissive when no line-of-sight query is attached (e.g. in tests).
func (c *Character) CanSee(target attackable.Combatant) bool {
	tx, ty, tz := target.Position()
	return c.canSeePosition(tx, ty, tz, target.CollisionHeight())
}

// CanSeeTarget reports whether t is visible to this player, satisfying
// handler/target.SightChecker for the cast pipeline's launch-phase
// line-of-sight gate. Same geodata query as CanSee, keyed to t's own eye
// height when it exposes one.
func (c *Character) CanSeeTarget(t target.Actor) bool {
	var theight float64
	if h, ok := t.(interface{ CollisionHeight() float64 }); ok {
		theight = h.CollisionHeight()
	}
	tx, ty, tz := t.Position()
	return c.canSeePosition(tx, ty, tz, theight)
}

func (c *Character) canSeePosition(tx, ty, tz int, theight float64) bool {
	if c.los == nil {
		return true
	}
	ox, oy, oz := c.Position()
	return c.los.CanSeeActor(ox, oy, oz, c.CollisionHeight(), tx, ty, tz, theight)
}
