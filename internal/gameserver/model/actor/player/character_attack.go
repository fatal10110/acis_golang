package player

import (
	"math"
	"math/rand/v2"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

const defaultPlayerAttackSpeed = 300

var weaponRange = map[item.WeaponType]int{
	item.WeaponBow:  500,
	item.WeaponPole: 66,
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
	if c.roll == nil {
		c.roll = rand.IntN
	}
}

// SetWorld records the world registry BroadcastAttack reaches through.
func (c *Character) SetWorld(state *world.State) {
	c.world = state
}

// SetFrameSender records the session send hook used by network-owned live
// player wrappers. Passing nil disconnects the character from that session.
func (c *Character) SetFrameSender(send func(wire.Frame) bool) {
	c.sendFrame = send
}

// SetAttackBroadcaster records the packet-layer hook that broadcasts attack
// snapshots to nearby connected clients.
func (c *Character) SetAttackBroadcaster(broadcast func(attack.Snapshot)) {
	c.broadcastAttack = broadcast
}

// SendFrame sends frame to the connected client, if any.
func (c *Character) SendFrame(frame wire.Frame) bool {
	if c.sendFrame == nil {
		frame.Release()
		return false
	}
	return c.sendFrame(frame)
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

// Position returns the live world position when c is spawned, otherwise the
// persisted last-known location.
func (c *Character) Position() (int, int, int) {
	if c.Visible() {
		return c.Presence.Position()
	}
	return c.Location.X, c.Location.Y, c.Location.Z
}

func (c *Character) template() *Template {
	return c.runtimeTemplate
}

func (c *Character) activeWeaponTemplate() *item.Template {
	if c.inventory == nil {
		return c.fistTemplate()
	}
	inst := c.inventory.ItemAt(itemcontainer.RHand)
	if inst == nil {
		return c.fistTemplate()
	}
	if tmpl, ok := c.inventory.Templates().Get(inst.TemplateID); ok && tmpl != nil && tmpl.Weapon != nil {
		return tmpl
	}
	return c.fistTemplate()
}

func (c *Character) fistTemplate() *item.Template {
	tmpl := c.template()
	if tmpl == nil || c.inventory == nil || tmpl.FistsItemID == 0 {
		return nil
	}
	fists, _ := c.inventory.Templates().Get(int32(tmpl.FistsItemID))
	return fists
}

func (c *Character) weaponStat(stat string, fallback float64) float64 {
	weapon := c.activeWeaponTemplate()
	if weapon == nil {
		return fallback
	}
	for _, mod := range weapon.Modifiers {
		if mod.Stat == stat && mod.Op == item.FuncSet {
			return mod.Value
		}
	}
	return fallback
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
	return in3DRange(c.location(), location.Location{X: tx, Y: ty, Z: tz}, totalRadius)
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
	weapon := c.activeWeaponTemplate()
	if weapon == nil || weapon.Weapon == nil {
		return item.WeaponFist
	}
	return weapon.Weapon.Type
}

// AttackSpeed resolves the equipped weapon's pAtkSpd stat-set value.
func (c *Character) AttackSpeed() int {
	return int(c.weaponStat("pAtkSpd", defaultPlayerAttackSpeed))
}

// PhysicalAttackRange returns the Java WeaponType range for the active
// weapon family.
func (c *Character) PhysicalAttackRange() int {
	if rng, ok := weaponRange[c.AttackType()]; ok {
		return rng
	}
	return 40
}

// WeaponReuseDelay returns the active weapon reuse delay, used for bows.
func (c *Character) WeaponReuseDelay() time.Duration {
	weapon := c.activeWeaponTemplate()
	if weapon == nil || weapon.Weapon == nil {
		return 0
	}
	return time.Duration(weapon.Weapon.ReuseDelay) * time.Millisecond
}

// WeaponGrade returns the active weapon crystal grade for attack packets.
func (c *Character) WeaponGrade() int {
	weapon := c.activeWeaponTemplate()
	if weapon == nil {
		return 0
	}
	return int(weapon.Crystal)
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

	dexIdx := clampStatIndex(tmpl.DEX)
	accuracy := int(statbonus.BaseEvasionAccuracy[dexIdx]) + c.Level
	evasion := other.Evasion()

	_, _, sz := c.Position()
	_, _, tz := other.Position()
	rate := formulas.HitRate(accuracy, evasion, sz-tz, false, false, true)
	if formulas.Missed(rate, c.rollValue(1000)) {
		hit.Miss = true
		return hit
	}

	critRate := c.weaponStat("rCrit", 4) * statbonus.DEXBonus[dexIdx] * 10
	crit := formulas.CritSucceeds(critRate, c.rollValue(1000))

	randomMul := 1.0
	if weapon := c.activeWeaponTemplate(); weapon != nil && weapon.Weapon != nil {
		if spread := int(weapon.Weapon.RandomDamage); spread > 0 {
			randomMul = 1 + float64(c.rollValue(2*spread+1)-spread)/100
		}
	}

	defence := other.PDef()
	if defence <= 0 {
		defence = 1
	}
	damage := formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower:       c.PAtk(),
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
	if c.broadcastAttack != nil {
		c.broadcastAttack(snapshot)
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
	weapon := c.activeWeaponTemplate()
	if weapon == nil {
		return false
	}
	arrows := c.inventory.FindArrowForBow(weapon.Crystal)
	if arrows == nil {
		return false
	}
	if arrows.Location == item.LocationPaperdoll {
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
	weapon := c.activeWeaponTemplate()
	if weapon == nil || weapon.Weapon == nil {
		return 0
	}
	return int(weapon.Weapon.MPConsume)
}

// MP returns current MP as an integer for attack gating.
func (c *Character) MP() int {
	return int(c.CurMP)
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
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.PAtk > 0 {
		base = tmpl.PAtk
	}
	return c.weaponStat("pAtk", base)
}

// PDef returns the current physical defence value.
func (c *Character) PDef() float64 {
	tmpl := c.template()
	if tmpl == nil || tmpl.PDef <= 0 {
		return 1
	}
	return tmpl.PDef
}

// Evasion returns this player's physical evasion rating.
func (c *Character) Evasion() int {
	tmpl := c.template()
	if tmpl == nil {
		return c.Level
	}
	return int(statbonus.BaseEvasionAccuracy[clampStatIndex(tmpl.DEX)]) + c.Level
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
	if dmg < 0 {
		dmg = 0
	}

	c.hpMu.Lock()
	if c.CurHP <= 0 {
		c.hpMu.Unlock()
		return false
	}
	c.CurHP -= float64(dmg)
	dead := c.CurHP <= 0
	if dead {
		c.CurHP = 0
	}
	c.hpMu.Unlock()

	if !dead {
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

func (c *Character) location() location.Location {
	x, y, z := c.Position()
	return location.Location{X: x, Y: y, Z: z}
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

func clampStatIndex(v int) int {
	if v < 0 {
		return 0
	}
	if v >= statbonus.MaxStatValue {
		return statbonus.MaxStatValue - 1
	}
	return v
}

func in3DRange(a, b location.Location, radius int) bool {
	dx := float64(a.X - b.X)
	dy := float64(a.Y - b.Y)
	dz := float64(a.Z - b.Z)
	return math.Sqrt(dx*dx+dy*dy+dz*dz) <= float64(radius)
}

var _ attack.PlayerActor = (*Character)(nil)
var _ physicalTarget = (*Character)(nil)
