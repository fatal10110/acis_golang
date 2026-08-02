package player

import (
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

const (
	defaultPlayerAttackSpeed      = 300
	defaultPlayerMagicAttackSpeed = 333
)

var weaponRange = map[item.WeaponType]int{
	item.WeaponBow:  500,
	item.WeaponPole: 66,
}

type activeWeapon struct {
	tmpl *item.Template
	inst *item.Instance
}

func (w activeWeapon) stat(stat string, fallback float64) float64 {
	if w.tmpl == nil {
		return fallback
	}
	for _, mod := range w.tmpl.Modifiers {
		if mod.Stat == stat && mod.Op == item.FuncSet {
			return mod.Value
		}
	}
	return fallback
}

func (w activeWeapon) attackType() item.WeaponType {
	if w.tmpl == nil || w.tmpl.Weapon == nil {
		return item.WeaponFist
	}
	return w.tmpl.Weapon.Type
}

func (w activeWeapon) reuseDelay() time.Duration {
	if w.tmpl == nil || w.tmpl.Weapon == nil {
		return 0
	}
	return time.Duration(w.tmpl.Weapon.ReuseDelay) * time.Millisecond
}

func (w activeWeapon) grade() int {
	if w.tmpl == nil {
		return 0
	}
	return int(w.tmpl.Crystal)
}

type physicalTarget interface {
	attackable.Combatant
	Position() (int, int, int)
	PDef() float64
	Evasion() int
}

// LineOfSight is the geodata query CanSee needs to gate targeting on real
// terrain occlusion between two actors.

// SetLineOfSight records the geodata line-of-sight query used by CanSee. A
// nil los (e.g. in tests that don't exercise geodata) leaves CanSee
// permissive.

// PeaceZoneQuery reports whether any point within effectRange of (x, y, z) —
// sampled at the point and its four axis-aligned range offsets — falls
// inside a peace-suspending zone attached to the region containing
// (regionX, regionY). Callers pass their own position as the region anchor,
// matching the reference's caster-region-only zone lookup.

// SetZones records the zone index EffectRangeInPeaceZone queries. A nil
// zones (e.g. in tests that don't exercise zone data) leaves it permissive.

// SetGroundTarget records the last ground-click point a ground-targeted
// skill cast (RequestExMagicSkillUseGround) resolved, reused across casts
// until the next ground click overwrites it.

// GroundTarget returns the last recorded ground-click point.

// CanSeePoint reports whether an arbitrary world point is visible to this
// player: a geodata line-of-sight query from this player's position and eye
// height to the raw point (no height offset on the point end, matching the
// reference's ground-target LOS query), or permissive when no
// line-of-sight query is attached (e.g. in tests).

// EffectRangeInPeaceZone reports whether the given point's effect range
// overlaps a peace-suspending zone attached to this player's own current
// region, or permissive (false) when no zone index is attached (e.g. in
// tests).

// AttachRuntime records the static template and restored inventory used by
// live combat and visibility code. Call it before exposing c to the world.

// AddRewardItem creates and adds one kill-reward item stack to this live
// character's inventory. objectID must be allocated by the reward caller.

// Inventory returns the carried item collection attached by AttachRuntime,
// or nil if the character has none yet.

// SetWorld records the world registry BroadcastAttack reaches through.

// SyncPosition moves this player's live world-grid presence to position.

// SetLastKnownPosition records position and heading as this player's last
// known world state. Call it whenever a client-reported move is accepted,
// alongside the world-grid presence and CreatureMove position it must
// stay consistent with.

// SetFrameSender records the session send hook used by network-owned live
// player wrappers. Passing nil disconnects the character from that session.
func (c *Character) SetFrameSender(send func(wire.Frame) bool) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.sendFrame = send
}

// SetAttackBroadcaster records the packet-layer hook that broadcasts attack
// snapshots to nearby connected clients.
func (c *Character) SetAttackBroadcaster(broadcast func(attack.Snapshot)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastAttack = broadcast
}

// SetMoveBroadcaster records the packet-layer hook that broadcasts movement
// events to this character's own session and nearby connected clients.
func (c *Character) SetMoveBroadcaster(broadcast func(move.Event)) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastMove = broadcast
}

// SetStopBroadcaster records the packet-layer hook that broadcasts a
// stop-in-place notice to this character's own session and nearby connected
// clients when server-driven movement is cancelled mid-flight.
func (c *Character) SetStopBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastStop = broadcast
}

// SetDieBroadcaster records the packet-layer hook that broadcasts the death
// packet to this character's own session and nearby connected clients at
// the moment this character dies.
func (c *Character) SetDieBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastDie = broadcast
}

// SetStatusBroadcaster records the packet-layer hook that broadcasts this
// character's current HP to nearby connected clients whenever it changes.
func (c *Character) SetStatusBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastStatus = broadcast
}

// SetAbnormalEffectUpdater records the packet-layer hook that sends this
// character's own session its current active-effect icon list.
func (c *Character) SetAbnormalEffectUpdater(update func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateAbnormalEffect = update
}

// SetUserInfoUpdater records the packet-layer hook that resends this
// character's own UserInfo, the packet carrying its experience, SP and
// level.
func (c *Character) SetUserInfoUpdater(update func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.updateUserInfo = update
}

// UpdateUserInfo resends this character's UserInfo through the runtime
// packet hook, mirroring PlayerStatus.addExp() pushing a fresh UserInfo on
// every experience change — without it the client keeps displaying the
// experience, SP and level it was last told about.
func (c *Character) UpdateUserInfo() {
	c.stateMu.RLock()
	update := c.updateUserInfo
	c.stateMu.RUnlock()
	if update != nil {
		update()
	}
}

// UpdateAbnormalEffect refreshes this character's abnormal-effect icon
// state through the runtime packet hook, implementing the effect list's
// abnormalUpdater hook: it fires on every effect start and stop, matching
// Creature.addEffect()/removeEffect() unconditionally queueing an
// EffectList icon update on each attempt.
func (c *Character) UpdateAbnormalEffect() {
	c.stateMu.RLock()
	update := c.updateAbnormalEffect
	c.stateMu.RUnlock()
	if update != nil {
		update()
	}
}

// BroadcastStatus sends this character's current HP through the runtime
// packet hook.
func (c *Character) BroadcastStatus() {
	c.stateMu.RLock()
	broadcast := c.broadcastStatus
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}

// SetMPStatusBroadcaster records the packet-layer hook that broadcasts this
// character's current HP and MP to its own session whenever an MP-only
// change (e.g. a mana-drain tick) needs to reach the client. Separate from
// SetStatusBroadcaster because PlayerStatus.broadcastStatusUpdate()
// (CreatureStatus.java's Player override) unconditionally includes CUR_MP
// in every status packet it sends, unlike the generic HP-only,
// threshold-gated broadcast the base Creature path uses.
func (c *Character) SetMPStatusBroadcaster(broadcast func()) {
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	c.broadcastMPStatus = broadcast
}

// BroadcastMPStatus sends this character's current HP and MP through the
// runtime packet hook.
func (c *Character) BroadcastMPStatus() {
	c.stateMu.RLock()
	broadcast := c.broadcastMPStatus
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}

// SendFrame sends frame to the connected client, if any.
func (c *Character) SendFrame(frame wire.Frame) bool {
	c.stateMu.RLock()
	send := c.sendFrame
	c.stateMu.RUnlock()
	if send == nil {
		frame.Release()
		return false
	}
	return send(frame)
}

// SetRollSource overrides MakeAttackHit's random source for deterministic
// tests.

// ObjectID returns the persistent world object id assigned to this player.

// WorldPlayer satisfies world.Player: a Character's presence keeps its
// world Region active.

// LevelValue returns the player's current level for live-owned actors.

// Level satisfies the cast/target handler interfaces (cancelTarget,
// seedableTarget, spoilableTarget, sowCaster, harvestCaster, magicCaster)
// that require a Level() int method.

// Karma satisfies the cross-package karma-gated target checks (e.g. a
// Guard's or friendly monster's attack-target rule) that type-assert for a
// Karma() int method.

// Position returns the live world position when c is spawned, otherwise the
// persisted last-known location.

func (c *Character) template() *Template {
	return c.runtimeTemplate
}

func (c *Character) activeWeapon() activeWeapon {
	if c.inventory == nil {
		return activeWeapon{tmpl: c.fistTemplate()}
	}
	inst := c.inventory.ItemAt(itemcontainer.RHand)
	if inst == nil {
		return activeWeapon{tmpl: c.fistTemplate()}
	}
	if tmpl, ok := c.inventory.Templates().Get(inst.TemplateID); ok && tmpl != nil && tmpl.Weapon != nil {
		return activeWeapon{tmpl: tmpl, inst: inst}
	}
	return activeWeapon{tmpl: c.fistTemplate()}
}

func (c *Character) fistTemplate() *item.Template {
	tmpl := c.template()
	if tmpl == nil || c.inventory == nil || tmpl.FistsItemID == 0 {
		return nil
	}
	fists, _ := c.inventory.Templates().Get(int32(tmpl.FistsItemID))
	return fists
}

// AttackDisabled reports whether this player can start a physical attack.
func (c *Character) AttackDisabled() bool {
	return c.AlikeDead()
}

// MovementDisabled reports whether this player is unable to move.
func (c *Character) MovementDisabled() bool {
	return false
}

// InAttackRange reports whether target is inside this player's weapon range.
func (c *Character) InAttackRange(target attackable.Combatant) bool {
	other, ok := target.(interface {
		Position() (int, int, int)
		CollisionRadius() float64
	})
	if !ok {
		return false
	}

	tx, ty, tz := other.Position()
	totalRadius := c.PhysicalAttackRange() + int(c.CollisionRadius()) + int(other.CollisionRadius())
	at := c.CurrentLocation()
	return location.In3DRange(at.X, at.Y, at.Z, tx, ty, tz, totalRadius)
}

// Knows reports whether target is visible to this player.

// CanSee reports whether target is visible to this player: a geodata
// line-of-sight query between the two actors' positions and eye heights, or
// permissive when no line-of-sight query is attached (e.g. in tests).

// AttackType resolves from the equipped right-hand weapon, falling back to
// the character template's fist weapon.
func (c *Character) AttackType() item.WeaponType {
	return c.activeWeapon().attackType()
}

// AttackSpeed resolves the equipped weapon's pAtkSpd stat-set value.
func (c *Character) AttackSpeed() int {
	return int(c.calcStat(stat.PowerAttackSpeed, c.activeWeapon().stat("pAtkSpd", defaultPlayerAttackSpeed)))
}

// MagicAttackSpeed returns the casting speed used by magic-skill timing.
func (c *Character) MagicAttackSpeed() int {
	return int(c.calcStat(stat.MagicAttackSpeed, defaultPlayerMagicAttackSpeed))
}

// Accuracy returns this player's physical accuracy rating.
func (c *Character) Accuracy() int {
	return int(c.calcStat(stat.AccuracyCombat, 0))
}

// CriticalRate returns this player's physical critical rate.
func (c *Character) CriticalRate() float64 {
	return c.calcStat(stat.CriticalRate, c.activeWeapon().stat("rCrit", 4))
}

// MagicCriticalRate returns this player's magic critical rate.
func (c *Character) MagicCriticalRate() float64 {
	return c.calcStat(stat.MCriticalRate, 8)
}

// RunSpeed returns the current run speed.
func (c *Character) RunSpeed() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return 0
	}
	return c.calcStat(stat.RunSpeed, tmpl.RunSpeed) * c.weightPenaltySpeedMultiplier()
}

// PhysicalAttackRange returns the attack range for the active weapon
// family.
func (c *Character) PhysicalAttackRange() int {
	if rng, ok := weaponRange[c.AttackType()]; ok {
		return rng
	}
	return 40
}

// WeaponReuseDelay returns the active weapon reuse delay, used for bows.
func (c *Character) WeaponReuseDelay() time.Duration {
	return c.activeWeapon().reuseDelay()
}

// WeaponGrade returns the active weapon crystal grade for attack packets.
func (c *Character) WeaponGrade() int {
	return c.activeWeapon().grade()
}

// SoulshotCharged reports whether a soulshot charge is currently active.

// SpiritshotCharged reports whether a spiritshot charge is currently active.

// BlessedSpiritshotCharged reports whether a blessed spiritshot charge is currently active.

// ChargeShotResult distinguishes why a direct-use shot charge attempt did
// or didn't take, so the network layer can pick the matching client
// message (or suppress it for an auto-shot-enabled item, the way the
// reference does).

// ChargeShotOK means the weapon accepted the charge.

// ChargeShotNoCapacity means no real weapon is equipped, or it can't
// carry this shot kind at all.

// ChargeShotGradeMismatch means the shot's crystal grade doesn't match
// the weapon's.

// ChargeShotAlreadyCharged means the weapon already carries this
// charge; the reference answers this case with total silence, not a
// system message.

// ChargeSoulshot attempts to charge the active weapon with a soulshot of
// shotCrystal grade, using reducedRoll (a 0-99 percentile roll) to decide
// whether the weapon's reduced-consumption count applies. Checks run
// capacity, then grade, then already-charged — the reference's own order
// for this shot kind, which differs from ChargeSpiritshot's order. On
// ChargeShotOK the weapon is marked charged and consume is the count to
// destroy from the item stack.

// ChargeSpiritshot attempts to charge the active weapon with a spiritshot
// of shotCrystal grade (kind is ShotSpirit or ShotBlessedSpirit; both draw
// from the weapon's same spiritshot capacity). Checks run capacity, then
// already-charged, then grade — the reference's own order for this shot
// kind, which differs from ChargeSoulshot's order. On ChargeShotOK the
// weapon is marked charged with kind and consume is the count to destroy
// from the item stack.

// SetHeadingTo orients this player toward target.
func (c *Character) SetHeadingTo(target attackable.Combatant) {
	other, ok := target.(interface{ Position() (int, int, int) })
	if !ok {
		return
	}
	sx, sy, _ := c.Position()
	tx, ty, _ := other.Position()
	c.Presence.SetHeading(location.Location{X: sx, Y: sy}.HeadingTo(location.Location{X: tx, Y: ty}))
}

// MakeAttackHit resolves one physical attack result.

// BroadcastAttack sends an attack snapshot through the runtime packet hook.
func (c *Character) BroadcastAttack(snapshot attack.Snapshot) {
	c.stateMu.RLock()
	broadcast := c.broadcastAttack
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast(snapshot)
	}
}

// BroadcastMove sends a movement event through the runtime packet hook.
func (c *Character) BroadcastMove(event move.Event) {
	c.stateMu.RLock()
	broadcast := c.broadcastMove
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast(event)
	}
}

// BroadcastStop sends a stop-in-place notice through the runtime packet
// hook.
func (c *Character) BroadcastStop() {
	c.stateMu.RLock()
	broadcast := c.broadcastStop
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}

// BroadcastDie sends the death packet through the runtime packet hook.
func (c *Character) BroadcastDie() {
	c.stateMu.RLock()
	broadcast := c.broadcastDie
	c.stateMu.RUnlock()
	if broadcast != nil {
		broadcast()
	}
}

// InPeaceZone reports whether c is in a combat-blocking peace zone.
func (c *Character) InPeaceZone() bool { return false }

// TryToIdle is the player attack stop hook. AI idle state is not modeled yet.
func (c *Character) TryToIdle() {}

// CheckAndEquipArrows ensures a bow user has matching arrows equipped.
func (c *Character) CheckAndEquipArrows() bool {
	if c.inventory == nil {
		return false
	}
	weapon := c.activeWeapon()
	if weapon.tmpl == nil {
		return false
	}
	arrows := c.inventory.FindArrowForBow(weapon.tmpl.Crystal)
	if arrows == nil {
		return false
	}
	if arrows.Snapshot().Location == item.LocationPaperdoll {
		return true
	}
	tmpl, ok := c.inventory.Templates().Get(arrows.TemplateID)
	if !ok {
		return false
	}
	c.inventory.SetPaperdollItem(itemcontainer.LHand, arrows, tmpl)
	return true
}

// WeaponMPConsume returns the active weapon's MP cost per attack.
func (c *Character) WeaponMPConsume() int {
	weapon := c.activeWeapon()
	if weapon.tmpl == nil || weapon.tmpl.Weapon == nil {
		return 0
	}
	return int(weapon.tmpl.Weapon.MPConsume)
}

// MP returns current MP as an integer for attack gating.
func (c *Character) MP() int {
	return c.CurrentMP()
}

// ClearRecentFakeDeath clears the recent fake-death state. Fake death is not
// modeled yet, so this is a no-op.
func (c *Character) ClearRecentFakeDeath() {}

// ClientActionFailed sends the client-action-failed notification. The packet
// is not modeled yet, so this is a no-op.
func (c *Character) ClientActionFailed() {}

// PAtk returns the physical attack value used by the current minimal combat
// pipeline.
func (c *Character) PAtk() float64 {
	return c.pAtk(c.activeWeapon())
}

func (c *Character) pAtk(weapon activeWeapon) float64 {
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.PAtk > 0 {
		base = tmpl.PAtk
	}
	return c.calcStat(stat.PowerAttack, weapon.stat("pAtk", base))
}

// PDef returns the current physical defence value.
func (c *Character) PDef() float64 {
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.PDef > 0 {
		base = tmpl.PDef
	}
	return c.calcStat(stat.PowerDefence, base)
}

// Evasion returns this player's physical evasion rating.
func (c *Character) Evasion() int {
	tmpl := c.template()
	if tmpl == nil {
		return c.CharLevel
	}
	return int(c.calcStat(stat.EvasionRate, 0))
}

// CollisionRadius returns this player's body radius.
func (c *Character) CollisionRadius() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return 0
	}
	if c.Sex == SexFemale {
		return tmpl.CollisionRadiusFemale
	}
	return tmpl.CollisionRadius
}

// CollisionHeight returns this player's body height, used for line-of-sight
// eye-height calculation.
func (c *Character) CollisionHeight() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return 0
	}
	if c.Sex == SexFemale {
		return tmpl.CollisionHeightFemale
	}
	return tmpl.CollisionHeight
}

// TakeDamage applies physical damage, broadcasts the resulting HP to nearby
// observers, and runs the once-only death path when HP reaches zero. A hit
// against an already-dead character is a no-op: no damage is applied and no
// status is broadcast.

// Dead reports whether the player has died.

// AlikeDead reports whether this player is dead or dead-equivalent,
// including a Fake Death toggle that is currently active.

// MarkDead transitions this player into its dead state.

// Revive clears this player's dead state and restores HP to fraction of
// calculated max HP. It reports whether the player was dead and is now
// revived; a call on a living player is a no-op.

// Die runs this player's death sequence: the once-only dead-state
// transition, then the death packet broadcast to this player's own session
// and every observer, so the corpse-fall animation plays live instead of
// only on a later dead reconnect.

// SiegeGuard reports whether this player is a defensive siege guard.
func (c *Character) SiegeGuard() bool { return false }

// Playable reports whether this combatant is player-controlled.
func (c *Character) Playable() bool { return true }

// AttackableBy reports whether attacker may attack this player.
func (c *Character) AttackableBy(attack.CreatureActor) bool {
	return !c.AlikeDead()
}

// Roll draws a uniform random integer in [0, n) from c's combat random source.

// RandomDamageSpread returns the active weapon's random-damage spread.
func (c *Character) RandomDamageSpread() int {
	weapon := c.activeWeapon()
	if weapon.tmpl == nil || weapon.tmpl.Weapon == nil {
		return 0
	}
	return int(weapon.tmpl.Weapon.RandomDamage)
}

var _ attack.PlayerActor = (*Character)(nil)
var _ move.Actor = (*Character)(nil)
var _ physicalTarget = (*Character)(nil)
