package player

import (
	"math/rand/v2"

	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// Character satisfies the cast pipeline's line-of-sight surface.
var _ target.SightChecker = (*Character)(nil)

// LineOfSight is the geodata query CanSee needs to gate targeting on real
// terrain occlusion between two actors.
type LineOfSight interface {
	CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool
}

// SetLineOfSight records the geodata line-of-sight query used by CanSee. A
// nil los (e.g. in tests that don't exercise geodata) leaves CanSee
// permissive.
func (c *Character) SetLineOfSight(los LineOfSight) {
	c.los = los
}

// PeaceZoneQuery reports whether any point within effectRange of (x, y, z) —
// sampled at the point and its four axis-aligned range offsets — falls
// inside a peace-suspending zone attached to the region containing
// (regionX, regionY). Callers pass their own position as the region anchor,
// matching the reference's caster-region-only zone lookup.
type PeaceZoneQuery interface {
	EffectRangeInPeaceZone(regionX, regionY, x, y, z, effectRange int) bool
}

// SetZones records the zone index EffectRangeInPeaceZone queries. A nil
// zones (e.g. in tests that don't exercise zone data) leaves it permissive.
func (c *Character) SetZones(zones PeaceZoneQuery) {
	c.zones = zones
}

// SetZoneRevalidator records the runtime hook that updates zone occupancy
// whenever the player's server-authoritative position changes.
func (c *Character) SetZoneRevalidator(revalidate func(location.Location)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.revalidateZones = revalidate
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

// SetWorld records the world registry BroadcastAttack reaches through.
func (c *Character) SetWorld(state *world.State) {
	c.world = state
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
	c.stateMu.RLock()
	revalidate := c.revalidateZones
	c.stateMu.RUnlock()
	if revalidate != nil {
		revalidate(previous)
	}
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

// WorldPlayer satisfies world.Player: a Character's presence keeps its
// world Region active.
func (c *Character) WorldPlayer() {}

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
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	var theight float64
	if h, ok := target.(interface{ CollisionHeight() float64 }); ok {
		theight = h.CollisionHeight()
	}
	tx, ty, tz := other.Position()
	return c.canSeePosition(tx, ty, tz, theight)
}

// CanSeeTarget reports whether t is visible to this player, satisfying
// handler/target.SightChecker for the cast pipeline's launch-phase
// line-of-sight gate. Same geodata query as CanSee, keyed to t's own eye
// height when it exposes one.
func (c *Character) CanSeeTarget(t target.Creature) bool {
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
