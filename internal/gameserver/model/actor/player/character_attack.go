package player

import (
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/move"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
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

// AttachRuntime records the static template and restored inventory used by
// live combat and visibility code. Call it before exposing c to the world.
func (c *Character) AttachRuntime(tmpl *Template, inv *itemcontainer.Inventory) {
	c.runtimeTemplate = tmpl
	c.inventory = inv
	if c.effects == nil {
		c.effects = effect.NewList(c)
	}
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
	return c.inventory.AddNew(itemID, count, objectID) != nil
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
	c.locMu.Lock()
	c.Location = position
	c.locMu.Unlock()
	if c.world == nil {
		return
	}
	_ = c.world.Move(c, position.X, position.Y, position.Z)
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
func (c *Character) SetRollSource(f func(int) int) {
	c.roll = f
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
	return c.Level
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
		return activeWeapon{tmpl: tmpl}
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
func (c *Character) Knows(target attackable.Combatant) bool {
	tracked, ok := target.(world.Tracked)
	return ok && world.Knows(c, tracked)
}

// CanSee reports whether target is visible to this player. Geodata line of
// sight is not wired for live players yet, so known targets count as visible.
func (c *Character) CanSee(attackable.Combatant) bool {
	return true
}

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
	return c.calcStat(stat.MCriticalRate, 1)
}

// RunSpeed returns the current run speed.
func (c *Character) RunSpeed() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return 0
	}
	return c.calcStat(stat.RunSpeed, tmpl.RunSpeed)
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
func (c *Character) SoulshotCharged() bool {
	return false
}

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
func (c *Character) MakeAttackHit(target attackable.Combatant, split bool) attack.Hit {
	hit := attack.Hit{Target: target, TargetID: target.ObjectID()}
	other, ok := target.(physicalTarget)
	if !ok {
		hit.Miss = true
		return hit
	}

	tmpl := c.template()
	if tmpl == nil {
		hit.Miss = true
		return hit
	}
	weapon := c.activeWeapon()

	accuracy := c.Accuracy()
	evasion := other.Evasion()

	_, _, sz := c.Position()
	_, _, tz := other.Position()
	rate := formulas.HitRate(accuracy, evasion, sz-tz, false, false, true)
	if formulas.Missed(rate, c.rollValue(1000)) {
		hit.Miss = true
		return hit
	}

	critRate := c.CriticalRate()
	crit := formulas.CritSucceeds(critRate, c.rollValue(1000))

	randomMul := 1.0
	if weapon.tmpl != nil && weapon.tmpl.Weapon != nil {
		if spread := int(weapon.tmpl.Weapon.RandomDamage); spread > 0 {
			randomMul = 1 + float64(c.rollValue(2*spread+1)-spread)/100
		}
	}

	defence := other.PDef()
	if defence <= 0 {
		defence = 1
	}
	damage := formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower:       c.pAtk(weapon),
		Defence:           defence,
		Crit:              crit,
		PosMul:            formulas.PosMul(false, true, crit),
		ElementalMul:      1,
		RandomMul:         randomMul,
		RaceMul:           1,
		WeaponVulnMul:     1,
		PvPMul:            1,
		CritDamageMul:     1,
		CritDamagePosMul:  1,
		CritVulnMul:       1,
		CritDamageAddBase: 0,
	})
	if split {
		damage /= 2
	}

	hit.Damage = int(damage)
	hit.Crit = crit
	return hit
}

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
		return c.Level
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

// TakeDamage applies physical damage and runs the once-only death path when
// HP reaches zero.
func (c *Character) TakeDamage(dmg int, attacker creature.DeathActor) bool {
	if !c.ReduceCurrentHP(dmg) {
		return false
	}
	return c.Die(attacker)
}

// Dead reports whether the player has died.
func (c *Character) Dead() bool {
	c.deathMu.Lock()
	defer c.deathMu.Unlock()
	return c.dead
}

// AlikeDead reports whether this player is dead or dead-equivalent.
func (c *Character) AlikeDead() bool {
	return c.Dead()
}

// MarkDead transitions this player into its dead state.
func (c *Character) MarkDead() bool {
	c.deathMu.Lock()
	defer c.deathMu.Unlock()
	if c.dead {
		return false
	}
	c.dead = true
	return true
}

// Die runs this player's death sequence.
func (c *Character) Die(killer creature.DeathActor) bool {
	return creature.Die(c, killer, nil)
}

// SiegeGuard reports whether this player is a defensive siege guard.
func (c *Character) SiegeGuard() bool { return false }

// Playable reports whether this combatant is player-controlled.
func (c *Character) Playable() bool { return true }

// AttackableBy reports whether attacker may attack this player.
func (c *Character) AttackableBy(attack.CreatureActor) bool {
	return !c.AlikeDead()
}

func (c *Character) rollValue(n int) int {
	if n <= 0 {
		return 0
	}
	if c.roll != nil {
		return c.roll(n)
	}
	return rand.IntN(n)
}

var _ attack.PlayerActor = (*Character)(nil)
var _ move.Actor = (*Character)(nil)
var _ physicalTarget = (*Character)(nil)
