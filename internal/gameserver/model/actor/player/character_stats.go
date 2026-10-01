package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/commons"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// statCalc returns s's live Calculator, creating it (with its builtin
// finalize step) on first touch. The common warm case only takes statMu's
// read lock; the slot is created at most once per Stat per Character.
func (c *Character) statCalc(s stat.Stat) *effect.Calculator {
	c.statMu.RLock()
	if calc := c.statCalcs[s]; calc != nil {
		c.statMu.RUnlock()
		return calc
	}
	c.statMu.RUnlock()
	return c.statCalcOrCreate(s)
}

func (c *Character) statCalcOrCreate(s stat.Stat) *effect.Calculator {
	c.statMu.Lock()
	defer c.statMu.Unlock()
	if calc := c.statCalcs[s]; calc != nil {
		return calc
	}
	calc := effect.NewCalculator(defaultBuiltin(s))
	c.statCalcs[s] = &calc
	return &calc
}

func (c *Character) calcStat(s stat.Stat, base float64) float64 {
	value := c.statCalc(s).Calc(characterStatActor{c: c}, base)
	if s.CantBeNegative() && value <= 0 {
		return 1
	}
	return value
}

// CalcStat finalizes base for s through c's live stat calculator.
func (c *Character) CalcStat(s stat.Stat, base float64) float64 {
	return c.calcStat(s, base)
}

// defaultBuiltin returns the static, attribute-driven finalize step every
// player's calculation chain for s runs at order 10, or nil for a Stat with
// no builtin.
func defaultBuiltin(s stat.Stat) funcs.Func {
	switch s {
	case stat.MaxHP:
		return funcs.MaxHpMul
	case stat.MaxMP:
		return funcs.MaxMpMul
	case stat.MaxCP:
		return funcs.MaxCpMul
	case stat.RegenerateHPRate:
		return funcs.RegenHpMul
	case stat.RegenerateMPRate:
		return funcs.RegenMpMul
	case stat.RegenerateCPRate:
		return funcs.RegenCpMul
	case stat.PowerAttack:
		return funcs.PAtkMod
	case stat.PowerDefence:
		return funcs.PDefMod
	case stat.MagicAttack:
		return funcs.MAtkMod
	case stat.MagicDefence:
		return funcs.MDefMod
	case stat.PowerAttackSpeed:
		return funcs.PAtkSpeed
	case stat.MagicAttackSpeed:
		return funcs.MAtkSpeed
	case stat.AccuracyCombat:
		return funcs.AtkAccuracy
	case stat.EvasionRate:
		return funcs.AtkEvasion
	case stat.CriticalRate:
		return funcs.AtkCritical
	case stat.MCriticalRate:
		return funcs.MAtkCritical
	case stat.RunSpeed:
		return funcs.MoveSpeed
	case stat.StatSTR:
		return funcs.HennaSTR
	case stat.StatCON:
		return funcs.HennaCON
	case stat.StatDEX:
		return funcs.HennaDEX
	case stat.StatINT:
		return funcs.HennaINT
	case stat.StatWIT:
		return funcs.HennaWIT
	case stat.StatMEN:
		return funcs.HennaMEN
	default:
		return nil
	}
}

type characterStatActor struct {
	c *Character
}

var _ stat.PlayerActor = characterStatActor{}

func (a characterStatActor) STR() int {
	return int(a.c.calcStat(stat.StatSTR, a.c.baseAttribute(stat.StatSTR)))
}

func (a characterStatActor) CON() int {
	return int(a.c.calcStat(stat.StatCON, a.c.baseAttribute(stat.StatCON)))
}

func (a characterStatActor) DEX() int {
	return int(a.c.calcStat(stat.StatDEX, a.c.baseAttribute(stat.StatDEX)))
}

func (a characterStatActor) INT() int {
	return int(a.c.calcStat(stat.StatINT, a.c.baseAttribute(stat.StatINT)))
}

func (a characterStatActor) WIT() int {
	return int(a.c.calcStat(stat.StatWIT, a.c.baseAttribute(stat.StatWIT)))
}

func (a characterStatActor) MEN() int {
	return int(a.c.calcStat(stat.StatMEN, a.c.baseAttribute(stat.StatMEN)))
}

func (a characterStatActor) Level() int {
	return max(a.c.Level(), 1)
}

func (a characterStatActor) LevelMod() float64 {
	return (89 + float64(a.Level())) / 100
}

func (a characterStatActor) IsSummon() bool { return false }

func (a characterStatActor) IsMageClass() bool { return ClassMage(a.c.ClassID) }

func (a characterStatActor) HennaBonus(s stat.Stat) float64 { return hennaBonusFor(a.c, s) }

func (a characterStatActor) HasEquipped(slotMask int) bool {
	return a.hasEquipped(slotMask, funcs.SlotLFinger, itemcontainer.LFinger) ||
		a.hasEquipped(slotMask, funcs.SlotRFinger, itemcontainer.RFinger) ||
		a.hasEquipped(slotMask, funcs.SlotLEar, itemcontainer.LEar) ||
		a.hasEquipped(slotMask, funcs.SlotREar, itemcontainer.REar) ||
		a.hasEquipped(slotMask, funcs.SlotNeck, itemcontainer.Neck) ||
		a.hasEquipped(slotMask, funcs.SlotHead, itemcontainer.Head) ||
		a.hasEquipped(slotMask, funcs.SlotChest, itemcontainer.Chest) ||
		a.hasEquipped(slotMask, funcs.SlotLegs, itemcontainer.Legs) ||
		a.hasEquipped(slotMask, funcs.SlotGloves, itemcontainer.Gloves) ||
		a.hasEquipped(slotMask, funcs.SlotFeet, itemcontainer.Feet) ||
		a.hasFullBodyArmor(slotMask)
}

func (a characterStatActor) hasEquipped(slotMask, bit, paperdoll int) bool {
	if slotMask&bit == 0 || a.c.inventory == nil {
		return false
	}
	return a.c.inventory.ItemAt(paperdoll) != nil
}

func (a characterStatActor) hasFullBodyArmor(slotMask int) bool {
	if slotMask&funcs.FullBodyArmor == 0 || a.c.inventory == nil {
		return false
	}
	inst := a.c.inventory.ItemAt(itemcontainer.Chest)
	if inst == nil {
		return false
	}
	tmpl, ok := a.c.inventory.Templates().Get(inst.TemplateID)
	return ok && (tmpl.Slot == item.SlotFullArmor || tmpl.Slot == item.SlotAllDress)
}

func (a characterStatActor) HasWeaponEquipped() bool {
	if a.c.inventory == nil {
		return false
	}
	inst := a.c.inventory.ItemAt(itemcontainer.RHand)
	if inst == nil {
		return false
	}
	tmpl, ok := a.c.inventory.Templates().Get(inst.TemplateID)
	return ok && tmpl.Kind == item.KindWeapon
}

func (c *Character) baseAttribute(s stat.Stat) float64 {
	tmpl := c.template()
	if tmpl == nil {
		return 0
	}
	switch s {
	case stat.StatSTR:
		return float64(tmpl.STR)
	case stat.StatCON:
		return float64(tmpl.CON)
	case stat.StatDEX:
		return float64(tmpl.DEX)
	case stat.StatINT:
		return float64(tmpl.INT)
	case stat.StatWIT:
		return float64(tmpl.WIT)
	case stat.StatMEN:
		return float64(tmpl.MEN)
	default:
		return 0
	}
}

func (c *Character) STR() int { return characterStatActor{c: c}.STR() }
func (c *Character) CON() int { return characterStatActor{c: c}.CON() }
func (c *Character) DEX() int { return characterStatActor{c: c}.DEX() }
func (c *Character) INT() int { return characterStatActor{c: c}.INT() }
func (c *Character) WIT() int { return characterStatActor{c: c}.WIT() }
func (c *Character) MEN() int { return characterStatActor{c: c}.MEN() }

func (c *Character) LevelMod() float64 { return characterStatActor{c: c}.LevelMod() }

// ShieldDefense resolves c's shield-block outcome against an incoming skill.
func (c *Character) ShieldDefense(caster creature.FormulaActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
	if def.IgnoreShield || !c.secondaryShieldEquipped() {
		return formulas.ShieldFailed
	}

	baseRate := c.calcStat(stat.ShieldRate, 0)
	if baseRate == 0 {
		return formulas.ShieldFailed
	}

	degrees := int(c.calcStat(stat.ShieldDefenceAngle, 120))
	if degrees < 360 && !c.facing(caster, degrees) {
		return formulas.ShieldFailed
	}

	c.stateMu.RLock()
	perfectRate := c.perfectShieldBlockRate
	c.stateMu.RUnlock()

	result := formulas.ShieldUse(baseRate, c.DEX(), attackerUsesBow(caster), isCrit, perfectRate, c.rollValue(100))
	c.notifyShieldBlock(result)
	return result
}

// notifyShieldBlock sends this defending player's client feedback for a
// shield-block roll: no message on a failed block (Formulas.java:866-879).
func (c *Character) notifyShieldBlock(result formulas.ShieldDefense) {
	switch result {
	case formulas.ShieldSuccess:
		c.emit(event.ShieldBlocked{})
	case formulas.ShieldPerfect:
		c.emit(event.ShieldBlocked{Perfect: true})
	}
}

func (c *Character) secondaryShieldEquipped() bool {
	if c.inventory == nil {
		return false
	}
	inst := c.inventory.ItemAt(itemcontainer.LHand)
	if inst == nil {
		return false
	}
	tmpl, ok := c.inventory.Templates().Get(inst.TemplateID)
	return ok && tmpl != nil &&
		tmpl.Kind == item.KindArmor &&
		tmpl.Armor != nil &&
		tmpl.Armor.Type == item.ArmorShield
}

func (c *Character) facing(caster attackable.Combatant, degrees int) bool {
	if caster == nil {
		return false
	}
	x, y, z := caster.Position()
	targetFacing := location.OrientedLocation{Location: c.CurrentLocation(), Heading: c.CurrentHeading()}
	return targetFacing.IsFacing(location.Location{X: x, Y: y, Z: z}, degrees)
}

func attackerUsesBow(caster creature.FormulaActor) bool {
	return caster != nil && caster.AttackType() == item.WeaponBow
}

// MAtk returns the current magic attack value, truncated to a whole
// number. A rider casts from its mount's M.Atk. instead of its class and
// weapon.
func (c *Character) MAtk() float64 {
	if m, ok := c.ridden(); ok {
		return math.Trunc(c.calcStat(stat.MagicAttack, m.mAtk))
	}
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.MAtk > 0 {
		base = tmpl.MAtk
	}
	return math.Trunc(c.calcStat(stat.MagicAttack, c.activeWeapon().stat("mAtk", base)))
}

// MDef returns the current magic defence value, truncated to a whole
// number.
func (c *Character) MDef() float64 {
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.MDef > 0 {
		base = tmpl.MDef
	}
	return math.Trunc(c.calcStat(stat.MagicDefence, base))
}

// HP returns current HP as a floating-point skill-resource value.
func (c *Character) HP() float64 { return c.ResourceValues().CurrentHP }

// MPValue returns current MP as a floating-point skill-resource value.
func (c *Character) MPValue() float64 { return c.ResourceValues().CurrentMP }

// CP returns current CP as a floating-point skill-resource value.
func (c *Character) CP() float64 { return c.ResourceValues().CurrentCP }

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
func (c *Character) MaxHPValue() float64 { return c.ResourceValues().MaxHP }

// HPFull reports whether current HP has reached maximum HP.
func (c *Character) HPFull() bool {
	values := c.ResourceValues()
	return values.CurrentHP >= values.MaxHP
}

// MaxMPValue returns maximum MP as a floating-point skill-resource value.
func (c *Character) MaxMPValue() float64 { return c.ResourceValues().MaxMP }

// MaxCPValue returns maximum CP as a floating-point skill-resource value.
func (c *Character) MaxCPValue() float64 { return c.ResourceValues().MaxCP }

// HPRegenRate returns c's current HP regeneration rate.
func (c *Character) HPRegenRate() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return c.calcStat(stat.RegenerateHPRate, 0)
	}
	return c.calcStat(stat.RegenerateHPRate, c.levelTableValue(tmpl.HPRegenTable, 0)) * c.weightPenaltyRegenMultiplier()
}

// MPRegenRate returns c's current MP regeneration rate.
func (c *Character) MPRegenRate() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return c.calcStat(stat.RegenerateMPRate, 0)
	}
	return c.calcStat(stat.RegenerateMPRate, c.levelTableValue(tmpl.MPRegenTable, 0)) * c.weightPenaltyRegenMultiplier()
}

// CPRegenRate returns c's current CP regeneration rate.
func (c *Character) CPRegenRate() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return c.calcStat(stat.RegenerateCPRate, 0)
	}
	return c.calcStat(stat.RegenerateCPRate, c.levelTableValue(tmpl.CPRegenTable, 0)) * c.weightPenaltyRegenMultiplier()
}

func (c *Character) levelTableValue(values []float64, fallback float64) float64 {
	idx := max(c.Level(), 1) - 1
	if idx < 0 || idx >= len(values) {
		return fallback
	}
	return values[idx]
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount. A
// dead character gains nothing: the check shares vitalsMu with MarkDead, so a
// caller's earlier liveness check cannot race a death into a corpse heal.
// Callers that change HP outside a client request broadcast the resulting
// status themselves; the cast and item paths already send their own batched
// StatusUpdate at the call site.
func (c *Character) AddHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := c.MaxHPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() || c.curHP >= maxHP {
		return 0
	}
	if c.curHP+amount > maxHP {
		amount = maxHP - c.curHP
	}
	c.curHP += amount
	return amount
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount. Like
// AddHP it leaves a dead character's MP alone, under the lock MarkDead holds.
func (c *Character) AddMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := c.MaxMPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() || c.curMP >= maxMP {
		return 0
	}
	if c.curMP+amount > maxMP {
		amount = maxMP - c.curMP
	}
	c.curMP += amount
	return amount
}

// AddCP restores CP, clamped to MaxCP, and returns the applied amount. A
// dead character gains nothing.
func (c *Character) AddCP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxCP := c.MaxCPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() || c.curCP >= maxCP {
		return 0
	}
	if c.curCP+amount > maxCP {
		amount = maxCP - c.curCP
	}
	c.curCP += amount
	return amount
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount. A
// dead character loses nothing.
func (c *Character) ReduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() || c.curMP <= 0 {
		return 0
	}
	if amount > c.curMP {
		amount = c.curMP
	}
	c.curMP -= amount
	return amount
}

// hitOutcome is how one hit landed on a character's vitals.
type hitOutcome struct {
	// applied is false when the character was already dead.
	applied bool
	// cpOnly marks a hit CP absorbed whole: HP did not change.
	cpOnly bool
	dead   bool
}

// absorbCPThenReduceHP applies PlayerStatus.reduceHp's CP-first absorption
// (PlayerStatus.java:166-184): a Playable attacker other than the actor
// itself (PvP, pet/summon damage) drains CP before HP, unless ignoreCP is
// set (Player.java:6152's ignoreCP argument, sourced from the skill's
// dmgDirectlyToHp for skill-cast and DOT damage, and always false for
// melee auto-attack, which passes no skill). Every damage route in the
// reference converges on this block — melee (CreatureAttack.java:263), DOT
// (EffectDamOverTime.java:48), and skill-cast (Player.java:6152/6154) — so
// ReduceHP, ReduceHPByDOT, and TakeDamage all reach it through landHit,
// after the servitor's damage share and under vitalsMu, and after
// applyNonConsumptionDamageEffects's sleep/immobile-stop, stand-up, and
// stun-break side effects (PlayerStatus.java:118-134), which run first in
// the reference. Already-dead is a no-op, matching PlayerStatus.reduceHp.
func (c *Character) absorbCPThenReduceHP(amount float64, attacker attackable.Combatant, ignoreCP bool) hitOutcome {
	if amount < 0 {
		amount = 0
	}
	if c.Dead() {
		return hitOutcome{}
	}
	hit := hitOutcome{applied: true}
	if !ignoreCP && c.hitByOther(attacker) && attacker.Kind().Playable() {
		hit.cpOnly = c.curCP >= amount
		drained := math.Min(c.curCP, amount)
		c.curCP -= drained
		amount -= drained
	}
	c.curHP -= amount
	hit.dead = c.curHP < creature.DeathHP
	if hit.dead {
		c.curHP = 0
	}
	return hit
}

// hitByOther reports whether attacker is a creature other than c. It
// compares object ids, not interface values: a caller may hand in a wrapper
// that embeds c (drowning passes the session's live player as its own
// attacker), and that is still c attacking itself.
func (c *Character) hitByOther(attacker attackable.Combatant) bool {
	return attacker != nil && attacker.ObjectID() != c.ObjectID()
}

// hitBySelf reports whether attacker is c itself, by object id like
// hitByOther. A nil attacker (a fall) is neither.
func (c *Character) hitBySelf(attacker attackable.Combatant) bool {
	return attacker != nil && attacker.ObjectID() == c.ObjectID()
}

// invulnerableTo reports whether c's invulnerability drops this damage: all
// damage from anyone else or from no attacker, and c's own damage unless it
// is a damage-over-time tick.
func (c *Character) invulnerableTo(attacker attackable.Combatant, isDOT bool) bool {
	return c.Invul() && (!c.hitBySelf(attacker) || !isDOT)
}

// damagePermitted reports whether attacker's damage permission lets its
// damage change c's CP and HP. Only another creature's permission counts:
// c's own damage (drowning, its own damage over time) always lands.
func (c *Character) damagePermitted(attacker attackable.Combatant) bool {
	return c.hitBySelf(attacker) || creature.CanDealDamage(attacker)
}

// servitorDamageShareRadius is how close c's servitor must be to take its
// share of c's damage.
const servitorDamageShareRadius = 900

// damageShareServitor is the servitor surface a damage share lands through.
type damageShareServitor interface {
	IsPet() bool
	Position() (x, y, z int)
	HP() float64
	TakeDamage(damage int, attacker attackable.Combatant) bool
}

// shareDamageWithServitor moves c's damage-transfer share of a hit another
// creature dealt onto c's servitor (never a pet) when it is within
// servitorDamageShareRadius, and returns that share. The share never takes
// the servitor below 1 HP, and it lands through the servitor's own damage
// path with the original attacker. It runs outside vitalsMu: the servitor
// reports its damage to c.
func (c *Character) shareDamageWithServitor(amount float64, attacker attackable.Combatant) int {
	if !c.hitByOther(attacker) {
		return 0
	}
	obj, ok := c.Summon()
	if !ok {
		return 0
	}
	servitor, ok := obj.(damageShareServitor)
	if !ok || servitor.IsPet() {
		return 0
	}
	sx, sy, sz := servitor.Position()
	x, y, z := c.Position()
	if !location.In3DRadius(x, y, z, sx, sy, sz, servitorDamageShareRadius) {
		return 0
	}
	share := int(commons.JavaInt(amount * c.calcStat(stat.TransferDamagePercent, 0) / 100))
	share = min(int(servitor.HP())-1, share)
	if share <= 0 {
		return 0
	}
	servitor.TakeDamage(share, attacker)
	return share
}

// servitorShareRecipient is a player told how a hit it dealt was split
// between its target and the target's servitor.
type servitorShareRecipient interface {
	NotifyServitorDamageShare(targetDamage, servitorDamage int)
}

// NotifyServitorDamageShare tells c that a hit it dealt, or its summon
// dealt, landed targetDamage on its target and servitorDamage on the
// target's servitor.
func (c *Character) NotifyServitorDamageShare(targetDamage, servitorDamage int) {
	c.emit(event.ServitorDamageShared{TargetDamage: targetDamage, ServitorDamage: servitorDamage})
}

// landHit applies one hit that got past the invulnerability, wake-up and
// damage-permission steps: the servitor's share first, then CP before HP,
// then the hit's feedback. It reports whether the hit left c dead; the
// caller runs the death.
func (c *Character) landHit(amount float64, attacker attackable.Combatant, ignoreCP, isDOT bool) bool {
	shared := c.shareDamageWithServitor(amount, attacker)
	amount -= float64(shared)
	c.vitalsMu.Lock()
	hit := c.absorbCPThenReduceHP(amount, attacker, ignoreCP)
	c.vitalsMu.Unlock()
	if !hit.applied {
		return false
	}
	c.sendHitFeedback(amount, attacker, hit, isDOT, shared)
	return hit.dead
}

// sendHitFeedback sends the damaged character what one applied hit owes
// it: its status and, for a hit of at least one point another creature
// dealt outside a damage-over-time tick, S1_GAVE_YOU_S2_DMG naming the
// attacker with the damage left after the servitor's share and before CP
// absorbed any of it. When the servitor took a share, the attacking player
// (a summon's owner) is told of the split next. The damage report sits
// between the CP write and the HP write; only the write that ends the hit
// reports the status, so a hit CP absorbed whole reports its status first
// and any other hit reports it last.
func (c *Character) sendHitFeedback(amount float64, attacker attackable.Combatant, hit hitOutcome, isDOT bool, shared int) {
	if hit.cpOnly {
		c.BroadcastStatus()
	}
	if full := int(commons.JavaInt(amount)); full > 0 && !isDOT && c.hitByOther(attacker) {
		c.emit(event.DamageReceived{AttackerName: attacker.CharacterName(), Amount: full})
		if shared > 0 {
			if recipient, ok := actingPlayer(attacker).(servitorShareRecipient); ok {
				recipient.NotifyServitorDamageShare(full, shared)
			}
		}
	}
	if !hit.cpOnly {
		c.BroadcastStatus()
	}
}

// actingPlayer returns the creature a acts for: a summon's owner, or a
// itself. Only a player among them takes player-addressed feedback.
func actingPlayer(a attackable.Combatant) attackable.Combatant {
	if a.Kind() == actor.KindSummon {
		owner, _ := a.Owner()
		return owner
	}
	return a
}

// ReduceHP applies skill HP damage and runs the once-only death path.
func (c *Character) ReduceHP(amount float64, attacker attackable.Combatant, skill modelskill.Definition) {
	c.reduceSkillHP(amount, attacker, skill, true)
}

// ReduceHPWithoutCastBreak is ReduceHP for a hit whose cast-break roll the
// caller already ran through BreakCastOnDamage, ahead of other per-hit work
// that must come between the two: DRAIN rolls the break, then lands its
// effects, and only then takes the HP.
func (c *Character) ReduceHPWithoutCastBreak(amount float64, attacker attackable.Combatant, skill modelskill.Definition) {
	c.reduceSkillHP(amount, attacker, skill, false)
}

// reduceSkillHP rolls a skill hit's cast break before the hit touches the
// character, on the full damage and whatever the attacker's damage
// permission: the break roll only exempts an invulnerable target. The
// wake-up side effects follow, and only then does another attacker without
// damage permission stop short of the CP and HP change. A hit that works
// out to no damage still rolls the break and runs the wake-up side
// effects; it then writes nothing unless it is another playable's hit
// through CP, which rewrites CP unchanged and reports the status.
func (c *Character) reduceSkillHP(amount float64, attacker attackable.Combatant, skill modelskill.Definition, breakCast bool) {
	// A NaN amount (a zero-defence hit scaled by a zero multiplier) takes
	// nothing, like a negative one.
	if !(amount > 0) {
		amount = 0
	}
	if c.Invul() || c.Dead() {
		return
	}
	if breakCast {
		c.breakCastOnDamage(amount)
	}
	c.applyNonConsumptionDamageEffects(false)
	if !c.damagePermitted(attacker) {
		return
	}
	if amount == 0 && (skill.DirectHPDamage || !c.hitByOther(attacker) || !attacker.Kind().Playable()) {
		return
	}
	if c.landHit(amount, attacker, skill.DirectHPDamage, false) {
		c.Die(attacker)
	}
}

// ReduceHPByDOT applies periodic damage without the normal-hit cast
// interruption. isDOT distinguishes a real damage-over-time skill tick
// (true, e.g. Poison/Bleed — Creature.reduceCurrentHpByDOT hardcodes this)
// from other periodic, non-attack damage sources the reference still routes
// through reduceCurrentHp with isDOT=false, such as drowning
// (WaterTaskManager.java calls reduceCurrentHp(hp, player, false, false,
// null)): both skip cast interruption, but only isDOT=false allows the
// 1-in-10 STUN-break roll. No datapack DOT effect sets dmgDirectlyToHp (the
// only skill that does, Backstab, is a BLOW burst hit, never delivered
// through EffectDamOverTime), so ignoreCP is always false here.
//
// Invulnerability drops the damage unless it is c's own damage-over-time
// tick; drowning, which is c's own damage but not such a tick, is dropped
// too. The wake-up side effects run next, and only then does another
// attacker's damage permission gate the CP and HP change: c's own damage is
// never gated by c's own permission.
//
// A tick that works out to no damage (a DamOverTime template value of 0)
// still runs the wake-up side effects. It then writes nothing unless it is
// another playable's tick through CP, which rewrites CP unchanged and
// reports the status, as reduceSkillHP does for a zero skill hit.
func (c *Character) ReduceHPByDOT(amount float64, attacker effect.Actor, isDOT bool) {
	c.reducePeriodicHP(amount, attacker, isDOT, false)
}

// ReduceHPByToggleUpkeep applies a toggle skill's own damage-over-time
// upkeep tick. It is ReduceHPByDOT's real DOT tick, except the HP counts as
// spent rather than taken in a hit: the wake-up side effects never run, so
// the tick leaves SLEEP, IMMOBILE_UNTIL_ATTACKED and a seated character
// alone. Invulnerability still lets it through, as it is c's own tick.
func (c *Character) ReduceHPByToggleUpkeep(amount float64, effector effect.Actor) {
	c.reducePeriodicHP(amount, effector, true, true)
}

// reducePeriodicHP is ReduceHPByDOT and ReduceHPByToggleUpkeep's shared
// path; hpConsumption skips the wake-up side effects.
func (c *Character) reducePeriodicHP(amount float64, attacker effect.Actor, isDOT, hpConsumption bool) {
	killer, _ := attacker.(attackable.Combatant)
	if amount < 0 {
		amount = 0
	}
	if c.Dead() || c.invulnerableTo(killer, isDOT) {
		return
	}
	if !hpConsumption {
		c.applyNonConsumptionDamageEffects(isDOT)
	}
	if !c.damagePermitted(killer) {
		return
	}
	if amount == 0 && (!c.hitByOther(killer) || !killer.Kind().Playable()) {
		return
	}
	if c.landHit(amount, killer, false, isDOT) {
		c.Die(killer)
	}
}

// applyNonConsumptionDamageEffects mirrors PlayerStatus.reduceHp's
// !isHPConsumption block: every normal-hit or DOT damage source stops SLEEP
// and IMMOBILE_UNTIL_ATTACKED, stands the character up when it has finished
// sitting down (an ordinary sit or a fake-death lie-down) unless it is in
// shop mode, and — for non-DOT damage only — has a 1-in-10 chance to break
// STUN. A sit-down or lie-down still under way is left to finish. HP spent
// as a skill's own resource cost, a toggle's upkeep tick among it
// (ReduceHPByToggleUpkeep), never routes through here.
func (c *Character) applyNonConsumptionDamageEffects(isDOT bool) {
	live := c.liveLocked()
	list := live.EffectList()
	list.StopByType(effect.TypeSleep)
	list.StopByType(effect.TypeImmobileUntilAttacked)

	if c.Seated() && !c.InStoreMode() {
		c.StandUp()
	}

	if !isDOT && live.Stunned() && c.Roll(10) == 0 {
		list.StopByType(effect.TypeStun)
	}
}

// SetHP sets current HP, clamped to [0, MaxHP]. It has no effect on a dead
// character; Revive is the only way back to positive HP.
func (c *Character) SetHP(value float64) {
	maxHP := c.MaxHPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	c.curHP = value
}

// SetCP sets current CP, clamped to [0, MaxCP]. It has no effect on a dead
// character.
func (c *Character) SetCP(value float64) {
	maxCP := c.MaxCPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.dead.Load() {
		return
	}
	if value < 0 {
		value = 0
	}
	if value > maxCP {
		value = maxCP
	}
	c.curCP = value
}

// CanBeHealed reports whether c may receive HP/MP restoration.
func (c *Character) CanBeHealed() bool {
	return !c.Dead() && !c.Invul()
}

// Invulnerable reports whether c ignores direct resource effects.
func (c *Character) Invulnerable() bool { return c.Invul() }

// HealEffectiveness returns the percentage multiplier applied to incoming heals.
func (c *Character) HealEffectiveness() float64 {
	return c.calcStat(stat.HealEffectiveness, 100)
}

// HealProficiency returns the flat heal-power bonus c contributes.
func (c *Character) HealProficiency() float64 {
	return c.calcStat(stat.HealProficiency, 0)
}

// RechargeMP applies c's MP recharge multiplier to amount.
func (c *Character) RechargeMP(amount float64) float64 {
	return c.calcStat(stat.RechargeMPRate, amount)
}

// HealInput resolves c's side of an outgoing HEAL. A mage-class player's
// charged spiritshot scales its M.Atk term; a fighter's does not.
func (c *Character) HealInput(def modelskill.Definition) (formulas.HealInput, bool) {
	scaling := formulas.HealShotScalingNone
	if ClassMage(c.ClassID) {
		scaling = formulas.HealShotScalingMage
	}
	return creature.ResolveHealInput(def, c.HealProficiency(), c.MAtk(), scaling), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against c.
func (c *Character) PhysicalSkillInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	return creature.ResolvePhysicalSkillInput(caster, c, def, creature.Playable(caster), 1)
}

// MagicDamageInput resolves the damage formula input for a magic skill cast
// by caster against c, rolling resist when magicFailures is set.
func (c *Character) MagicDamageInput(caster creature.FormulaActor, def modelskill.Definition, magicFailures bool) (formulas.MagicDamageInput, bool) {
	return creature.ResolveMagicDamageInput(caster, c, def, creature.Playable(caster), magicFailures)
}

// BlowInput resolves the damage formula input for a blow skill cast by
// caster against c.
func (c *Character) BlowInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.BlowInput, bool) {
	return creature.ResolveBlowInput(caster, c, def, creature.Playable(caster))
}

func (c *Character) CounterSkillPhysical() float64 {
	return c.CalcStat(stat.CounterSkillPhysical, 0)
}

// CancelVulnerability returns c's CANCEL_VULN multiplier for the cancel and
// cancel-debuff success-rate formulas (Formulas.java:949-951). classification
// is unused: the reference applies CANCEL_VULN uniformly, without the
// per-classification switch it uses for the other _VULN stats.
func (c *Character) CancelVulnerability(_ string) float64 {
	return c.CalcStat(stat.CancelVuln, 1)
}

// SkillReflectInput resolves c's reflected-skill chance for def.
func (c *Character) SkillReflectInput(def modelskill.Definition) formulas.SkillReflectInput {
	reflectStat := stat.ReflectSkillPhysic
	if def.Magic {
		reflectStat = stat.ReflectSkillMagic
	}
	return formulas.SkillReflectInput{
		IgnoreResists:  def.IgnoreResists,
		CanBeReflected: def.CanBeReflected,
		Magic:          def.Magic,
		CastRange:      def.CastRange,
		ReflectChance:  c.CalcStat(reflectStat, 0),
	}
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill
// cast by caster against c.
func (c *Character) ManaDamageInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	return creature.ResolveManaDamageInput(caster, c, c.MaxMPValue(), def)
}

// LethalRate returns c's lethal-strike rate multiplier.
func (c *Character) LethalRate() float64 {
	return c.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against c.
func (c *Character) LethalInput(caster creature.FormulaActor, def modelskill.Definition) (formulas.LethalInput, bool) {
	if c.Invul() || !creature.CanDealDamage(caster) {
		return formulas.LethalInput{}, false
	}
	if caster == nil {
		return formulas.LethalInput{}, false
	}
	attacker := caster
	return formulas.LethalInput{
		Chance1:       def.LethalChance1,
		Chance2:       def.LethalChance2,
		MagicLevel:    def.MagicLevel,
		AttackerLevel: attacker.Level(),
		TargetLevel:   c.Level(),
		LethalMul:     attacker.LethalRate(),
	}, true
}

// ApplyLethalOutcome applies a lethal-strike tier to c.
func (c *Character) ApplyLethalOutcome(outcome formulas.LethalOutcome, _ attackable.Combatant, _ modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		c.SetHP(1)
		c.SetCP(1)
	case formulas.LethalHalf:
		c.SetCP(1)
	}
}
