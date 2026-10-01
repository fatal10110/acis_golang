package player

import (
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/geo/dynamic"
	"github.com/fatal10110/acis_golang/internal/gameserver/handler/target"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
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

// LineOfSightIgnoring is the optional LineOfSight query that leaves one
// dynamic geodata object out, used when the target is such an object.
type LineOfSightIgnoring interface {
	CanSeeActorIgnoring(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64, ignore dynamic.Object) bool
}

// MountBodies resolves a mount NPC template's collision footprint, which
// replaces a mounted player's own body for reach and line of sight.
type MountBodies interface {
	CollisionBody(npcID int32) (radius, height float64, ok bool)
}

// PeaceZoneQuery reports whether any point within effectRange of (x, y, z) —
// sampled at the point and its four axis-aligned range offsets — falls
// inside a peace-suspending zone attached to the region containing
// (regionX, regionY). Callers pass their own position as the region anchor:
// the zone lookup uses the caster's region only.
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
// a zone that blocks SUMMON_FRIEND/SUMMON_PARTY (a no-summon-friend zone).
func (c *Character) NoSummonFriendZone() bool {
	return c.insideNoSummonFriendZone.Load()
}

// SetInDangerArea records the live zone engine's current danger-zone
// membership (zone.FlagDanger).
func (c *Character) SetInDangerArea(inside bool) {
	c.insideDangerArea.Store(inside)
}

// InDangerArea reports whether the character stands in a damage or effect
// zone. It takes no lock, so status packets built under any other lock can
// read it.
func (c *Character) InDangerArea() bool {
	return c.insideDangerArea.Load()
}

// SetInWater records the live zone engine's current water membership, which
// switches the move speed to the swim speed.
func (c *Character) SetInWater(inside bool) {
	if c.insideWater.Swap(inside) != inside {
		c.refreshMoveSpeed()
	}
}

// InWater reports whether the character currently stands in a water zone.
func (c *Character) InWater() bool {
	return c.insideWater.Load()
}

// SetSwampMoveBonus records the move bonus, in percent, of the swamp the
// character stands in, or 0 outside any swamp.
func (c *Character) SetSwampMoveBonus(bonus int) {
	if c.swampMoveBonus.Swap(int32(bonus)) != int32(bonus) {
		c.refreshMoveSpeed()
	}
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

// SetCastModifiers records the Ctrl/Shift state the player's latest CAST
// intention was started with: a skill request's own modifiers, or an item
// use's force-use modifier and no shift. The attack a nextActionAttack cast
// hands on to once it ends is held with that shift.
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
// height to the raw point (no height offset on the point end, as for a
// ground-target LOS query), or permissive when no
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
	c.runtimeTemplate.Store(tmpl)
	c.inventory = inv
	if inv != nil {
		inv.SetLimiter(c)
	}
	if c.roll == nil {
		c.roll = rand.IntN
	}
}

// Rules is the server-configuration slice a character's own rules read.
type Rules struct {
	RateKarmaExpLost       float64
	RespawnRestoreHP       float64
	WeightLimitMultiplier  float64
	InventorySlots         InventorySlots
	StorageSlots           StorageSlots
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
	Mounts MountBodies
	// MountData resolves a mount's pet data; nil leaves every mount unfed
	// and riding at its rider's own speeds.
	MountData MountDataSource
	Skills    skillDefinitions
	Levels    *LevelTable
	Log       zerolog.Logger
	Rules     Rules
}

// Configure installs rt. Call it before exposing c to the world.
func (c *Character) Configure(rt Runtime) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.world = rt.World
	c.los = rt.LOS
	c.zones = rt.Zones
	c.mounts = rt.Mounts
	c.mountData = rt.MountData
	c.skillDefs = rt.Skills
	c.levelTable = rt.Levels
	c.log = rt.Log
	c.rateKarmaExpLost = rt.Rules.RateKarmaExpLost
	c.respawnRestoreHP = rt.Rules.RespawnRestoreHP
	c.weightLimitMultiplier = rt.Rules.WeightLimitMultiplier
	c.inventorySlots = rt.Rules.InventorySlots
	c.storageSlots = rt.Rules.StorageSlots
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
	c.Live = live
	c.sink = sink
	c.stateMu.Unlock()
	c.refreshMoveSpeed()
}

// stopper is an armed one-shot timer.
type stopper interface{ Stop() bool }

// afterLocked arms fn to run once d has elapsed, as a task on the queue
// Attach installed. The caller holds stateMu.
func (c *Character) afterLocked(d time.Duration, fn func()) stopper {
	return c.Live.Queue().After(d, fn)
}

// DetachSession marks the owning session gone. Events the session alone
// delivered are dropped from then on, and a herb can no longer be applied.
func (c *Character) DetachSession() { c.sessionDetached.Store(true) }

// SessionDetached reports whether DetachSession has run.
func (c *Character) SessionDetached() bool { return c.sessionDetached.Load() }

// PlayerCharacter returns c. A wrapper embedding *Character inherits it, so
// a combatant reached through a summon's owner resolves to its model.
func (c *Character) PlayerCharacter() *Character { return c }

// CharacterHolder resolves to its underlying *Character: the model itself or
// a live wrapper embedding it. NPCs, summons, and doors are not holders, so
// an assertion to it legitimately fails for them.
type CharacterHolder interface {
	PlayerCharacter() *Character
}

func (c *Character) emit(e event.Event) {
	if c.sink != nil {
		c.sink.Emit(e)
	}
}

// RewardItemFits reports whether count units of itemID fit in this
// character's inventory slots, the check an auto-looted kill reward must
// pass before AddRewardItem.
func (c *Character) RewardItemFits(itemID int32, count int) bool {
	return c.inventory != nil && c.inventory.ValidateCapacityByItemID(itemID, count)
}

// AddRewardItem creates and adds one kill-reward item stack to this live
// character's inventory, then runs ItemAdded's side effects: adena names
// its amount, anything else its template and count. objectID must be
// allocated by the reward caller.
func (c *Character) AddRewardItem(itemID int32, count int, objectID int32) bool {
	if c.inventory == nil {
		return false
	}
	if c.inventory.AddNew(itemID, count, objectID) == nil {
		return false
	}
	notice := event.ObtainCreated
	if itemID == item.AdenaID {
		notice = event.ObtainAdena
	}
	c.ItemAdded(event.ItemObtained{ItemID: itemID, Count: count, Notice: notice})
	return true
}

// ItemSlotsNeeded reports how many new inventory slots count units of
// itemID would take in this character's inventory: none for a held
// stackable, one for a new stack, one per unit of a non-stackable.
func (c *Character) ItemSlotsNeeded(itemID int32, count int) int {
	if c.inventory == nil {
		return count
	}
	return c.inventory.SlotsNeededForItemID(itemID, count)
}

// ItemSlotsFit reports whether slots more stacks fit within this
// character's inventory slot limit.
func (c *Character) ItemSlotsFit(slots int) bool {
	return c.inventory != nil && c.inventory.ValidateCapacity(slots)
}

// AddCreatedItem creates count units of itemID in this live character's
// inventory, named in chat as picked up, the way an opened capsule hands
// over its product. nextID allocates each new instance's object id. It
// reports whether anything was added or, for a herb, applied.
func (c *Character) AddCreatedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	return c.createItem(itemID, count, nextID, event.ObtainCreated)
}

// AddEarnedItem creates count units of itemID in this live character's
// inventory, named in chat as earned, the way a sweep or a harvest pays
// out. nextID allocates each new instance's object id. It reports whether
// anything was added or, for a herb, applied.
func (c *Character) AddEarnedItem(itemID int32, count int, nextID func() (int32, error)) bool {
	return c.createItem(itemID, count, nextID, event.ObtainEarned)
}

// createItem creates count units of itemID by template id: a herb is
// applied at once instead of carried, a stackable joins the held stack or
// starts one, and a non-stackable arrives as one instance per unit. The
// items are then named in chat as notice says. Slot and weight limits are
// the caller's to check first.
func (c *Character) createItem(itemID int32, count int, nextID func() (int32, error), notice event.ObtainNotice) bool {
	if c.inventory == nil || count < 1 || nextID == nil {
		return false
	}
	tmpl, ok := c.inventory.Templates().Get(itemID)
	if !ok {
		return false
	}
	if tmpl.EtcItem != nil && tmpl.EtcItem.Type == item.EtcItemHerb {
		return c.ConsumeHerb(itemID)
	}
	count = c.addByTemplate(tmpl, count, nextID)
	if count == 0 {
		return false
	}
	c.ItemAdded(event.ItemObtained{ItemID: itemID, Count: count, Notice: notice})
	return true
}

// AddCraftedItem puts count units of itemID into this character's
// inventory the way a finished craft hands over its product: straight into
// the inventory, with no chat line and no arrow auto-equip. nextID allocates
// each new instance's object id. It reports how many units were added.
func (c *Character) AddCraftedItem(itemID int32, count int, nextID func() (int32, error)) int {
	if c.inventory == nil || count < 1 || nextID == nil {
		return 0
	}
	tmpl, ok := c.inventory.Templates().Get(itemID)
	if !ok {
		return 0
	}
	return c.addByTemplate(tmpl, count, nextID)
}

// addByTemplate adds count units of tmpl: a stackable joins the held stack
// or starts one, and a non-stackable arrives as one instance per unit. It
// returns how many units were added.
func (c *Character) addByTemplate(tmpl *item.Template, count int, nextID func() (int32, error)) int {
	instances := 1
	if !tmpl.Stackable {
		instances = count
	}
	added := 0
	for range instances {
		id, err := nextID()
		if err != nil || c.inventory.AddNew(tmpl.ID, count, id) == nil {
			break
		}
		added++
	}
	if tmpl.Stackable && added > 0 {
		return count
	}
	return added
}

// ItemAdded runs the side effects of items already added to this
// character's inventory, shared by every path that grants items: it names
// them in chat, then, for arrows reaching a bow user whose left hand is
// empty, puts the bow's matching arrows on.
func (c *Character) ItemAdded(obtained event.ItemObtained) {
	if obtained.Count < 1 {
		return
	}
	c.emit(obtained)
	if obtained.Notice == event.ObtainAdena || c.inventory == nil {
		return
	}
	tmpl, ok := c.inventory.Templates().Get(obtained.ItemID)
	if !ok || tmpl.EtcItem == nil || tmpl.EtcItem.Type != item.EtcItemArrow {
		return
	}
	if c.AttackType() != item.WeaponBow || c.inventory.ItemAt(itemcontainer.LHand) != nil {
		return
	}
	c.CheckAndEquipArrows()
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
	return c.Level()
}

// Level satisfies the cast/target handler interfaces (cancelTarget,
// seedableTarget, spoilableTarget, sowCaster, harvestCaster, magicCaster)
// that require a Level() int method.
func (c *Character) Level() int {
	c.progressionMu.RLock()
	defer c.progressionMu.RUnlock()
	return c.CharLevel
}

// Karma satisfies the cross-package karma-gated target checks (e.g. a
// Guard's or friendly monster's attack-target rule) that type-assert for a
// Karma() int method.
func (c *Character) Karma() int {
	c.progressionMu.RLock()
	defer c.progressionMu.RUnlock()
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
// attackable stays a leaf, so a Combatant is not statically a world object;
// one that is not on the grid is never known.
func (c *Character) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(c, tracked)
}

// CanSee reports whether target is visible to this player: a geodata
// line-of-sight query between the two actors' positions and eye heights, or
// permissive when no line-of-sight query is attached (e.g. in tests).
func (c *Character) CanSee(target attackable.Combatant) bool {
	tx, ty, tz := target.Position()
	return c.canSeeObject(target, tx, ty, tz, target.CollisionHeight())
}

// CanSeeTarget reports whether t is visible to this player for the cast
// pipeline's line-of-sight gates. Same geodata query as CanSee, keyed to
// t's own eye height.
func (c *Character) CanSeeTarget(t target.Actor) bool {
	tx, ty, tz := t.Position()
	return c.canSeeObject(t, tx, ty, tz, t.CollisionHeight())
}

// canSeeObject queries sight to obj standing at (tx, ty, tz). An obj that
// is itself a geodata object (a closed door) is left out of the query, so it
// never hides itself.
func (c *Character) canSeeObject(obj any, tx, ty, tz int, theight float64) bool {
	if c.los == nil {
		return true
	}
	ox, oy, oz := c.Position()
	if geoObj, ok := obj.(dynamic.Object); ok {
		if los, ok := c.los.(LineOfSightIgnoring); ok {
			return los.CanSeeActorIgnoring(ox, oy, oz, c.CollisionHeight(), tx, ty, tz, theight, geoObj)
		}
	}
	return c.los.CanSeeActor(ox, oy, oz, c.CollisionHeight(), tx, ty, tz, theight)
}
