package player

import (
	"math"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/funcs"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

const perfectShieldBlockRate = 5

func (c *Character) statCalc(s stat.Stat) *basefunc.Calculator {
	c.statMu.Lock()
	defer c.statMu.Unlock()
	return c.statCalcLocked(s)
}

func (c *Character) statCalcLocked(s stat.Stat) *basefunc.Calculator {
	if c.statCalcs == nil {
		c.statCalcs = make(map[stat.Stat]*basefunc.Calculator)
	}
	if calc := c.statCalcs[s]; calc != nil {
		return calc
	}
	calc := &basefunc.Calculator{}
	for _, fn := range defaultStatFuncs(s) {
		calc.AddFunc(fn)
	}
	c.statCalcs[s] = calc
	return calc
}

func (c *Character) calcStat(s stat.Stat, base float64) float64 {
	value := c.statCalc(s).Calc(characterStatActor{c: c}, c, nil, base)
	if s.CantBeNegative() && value < 0 {
		return 0
	}
	return value
}

func defaultStatFuncs(s stat.Stat) []basefunc.Func {
	switch s {
	case stat.MaxHP:
		return []basefunc.Func{funcs.MaxHpMul}
	case stat.MaxMP:
		return []basefunc.Func{funcs.MaxMpMul}
	case stat.MaxCP:
		return []basefunc.Func{funcs.MaxCpMul}
	case stat.RegenerateHPRate:
		return []basefunc.Func{funcs.RegenHpMul}
	case stat.RegenerateMPRate:
		return []basefunc.Func{funcs.RegenMpMul}
	case stat.RegenerateCPRate:
		return []basefunc.Func{funcs.RegenCpMul}
	case stat.PowerAttack:
		return []basefunc.Func{funcs.PAtkMod}
	case stat.PowerDefence:
		return []basefunc.Func{funcs.PDefMod}
	case stat.MagicAttack:
		return []basefunc.Func{funcs.MAtkMod}
	case stat.MagicDefence:
		return []basefunc.Func{funcs.MDefMod}
	case stat.PowerAttackSpeed:
		return []basefunc.Func{funcs.PAtkSpeed}
	case stat.MagicAttackSpeed:
		return []basefunc.Func{funcs.MAtkSpeed}
	case stat.AccuracyCombat:
		return []basefunc.Func{funcs.AtkAccuracy}
	case stat.EvasionRate:
		return []basefunc.Func{funcs.AtkEvasion}
	case stat.CriticalRate:
		return []basefunc.Func{funcs.AtkCritical}
	case stat.MCriticalRate:
		return []basefunc.Func{funcs.MAtkCritical}
	case stat.RunSpeed:
		return []basefunc.Func{funcs.MoveSpeed}
	case stat.StatSTR:
		return []basefunc.Func{funcs.HennaSTR}
	case stat.StatCON:
		return []basefunc.Func{funcs.HennaCON}
	case stat.StatDEX:
		return []basefunc.Func{funcs.HennaDEX}
	case stat.StatINT:
		return []basefunc.Func{funcs.HennaINT}
	case stat.StatWIT:
		return []basefunc.Func{funcs.HennaWIT}
	case stat.StatMEN:
		return []basefunc.Func{funcs.HennaMEN}
	default:
		return nil
	}
}

type characterStatActor struct {
	c *Character
}

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
	if a.c.CharLevel <= 0 {
		return 1
	}
	return a.c.CharLevel
}

func (a characterStatActor) LevelMod() float64 {
	return (89 + float64(a.Level())) / 100
}

func (a characterStatActor) IsSummon() bool { return false }

func (a characterStatActor) IsMageClass() bool { return false }

func (a characterStatActor) HennaBonus(stat.Stat) float64 { return 0 }

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
func (c *Character) ShieldDefense(caster any, def modelskill.Definition, isCrit bool) formulas.ShieldDefense {
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

	return formulas.ShieldUse(baseRate, c.DEX(), attackerUsesBow(caster), isCrit, perfectShieldBlockRate, c.rollValue(100))
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
	return ok && tmpl != nil && tmpl.Kind == item.KindArmor && tmpl.Armor != nil
}

func (c *Character) facing(caster any, degrees int) bool {
	other, ok := caster.(interface{ Position() (int, int, int) })
	if !ok {
		return false
	}
	x, y, z := other.Position()
	targetFacing := location.OrientedLocation{Location: c.CurrentLocation(), Heading: c.CurrentHeading()}
	return targetFacing.IsFacing(location.Location{X: x, Y: y, Z: z}, degrees)
}

func attackerUsesBow(caster any) bool {
	attacker, ok := caster.(interface{ AttackType() item.WeaponType })
	return ok && attacker.AttackType() == item.WeaponBow
}

// MAtk returns the current magic attack value.
func (c *Character) MAtk() float64 {
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.MAtk > 0 {
		base = tmpl.MAtk
	}
	return c.calcStat(stat.MagicAttack, c.activeWeapon().stat("mAtk", base))
}

// MDef returns the current magic defence value.
func (c *Character) MDef() float64 {
	tmpl := c.template()
	base := 1.0
	if tmpl != nil && tmpl.MDef > 0 {
		base = tmpl.MDef
	}
	return c.calcStat(stat.MagicDefence, base)
}

// HP returns current HP as a floating-point skill-resource value.
func (c *Character) HP() float64 { return c.ResourceValues().CurrentHP }

// MPValue returns current MP as a floating-point skill-resource value.
func (c *Character) MPValue() float64 { return c.ResourceValues().CurrentMP }

// CP returns current CP as a floating-point skill-resource value.
func (c *Character) CP() float64 { return c.ResourceValues().CurrentCP }

// MaxHPValue returns maximum HP as a floating-point skill-resource value.
func (c *Character) MaxHPValue() float64 { return c.ResourceValues().MaxHP }

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
	return c.calcStat(stat.RegenerateHPRate, c.levelTableValue(tmpl.HPRegenTable, 0))
}

// MPRegenRate returns c's current MP regeneration rate.
func (c *Character) MPRegenRate() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return c.calcStat(stat.RegenerateMPRate, 0)
	}
	return c.calcStat(stat.RegenerateMPRate, c.levelTableValue(tmpl.MPRegenTable, 0))
}

// CPRegenRate returns c's current CP regeneration rate.
func (c *Character) CPRegenRate() float64 {
	tmpl := c.template()
	if tmpl == nil {
		return c.calcStat(stat.RegenerateCPRate, 0)
	}
	return c.calcStat(stat.RegenerateCPRate, c.levelTableValue(tmpl.CPRegenTable, 0))
}

func (c *Character) levelTableValue(values []float64, fallback float64) float64 {
	level := c.CharLevel
	if level <= 0 {
		level = 1
	}
	idx := level - 1
	if idx < 0 || idx >= len(values) {
		return fallback
	}
	return values[idx]
}

// AddHP restores HP, clamped to MaxHP, and returns the applied amount.
func (c *Character) AddHP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxHP := c.MaxHPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.curHP >= maxHP {
		return 0
	}
	if c.curHP+amount > maxHP {
		amount = maxHP - c.curHP
	}
	c.curHP += amount
	return amount
}

// AddMP restores MP, clamped to MaxMP, and returns the applied amount.
func (c *Character) AddMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	maxMP := c.MaxMPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.curMP >= maxMP {
		return 0
	}
	if c.curMP+amount > maxMP {
		amount = maxMP - c.curMP
	}
	c.curMP += amount
	return amount
}

// ReduceMP subtracts MP, clamped at zero, and returns the applied amount.
func (c *Character) ReduceMP(amount float64) float64 {
	if amount <= 0 {
		return 0
	}
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if c.curMP <= 0 {
		return 0
	}
	if amount > c.curMP {
		amount = c.curMP
	}
	c.curMP -= amount
	return amount
}

// ReduceHP applies skill HP damage and runs the once-only death path.
func (c *Character) ReduceHP(amount float64, attacker any, _ modelskill.Definition) {
	if amount <= 0 {
		return
	}
	c.vitalsMu.Lock()
	if c.curHP <= 0 {
		c.vitalsMu.Unlock()
		return
	}
	c.curHP -= amount
	dead := c.curHP <= 0
	if dead {
		c.curHP = 0
	}
	c.vitalsMu.Unlock()
	if dead {
		killer, _ := attacker.(creature.DeathActor)
		c.Die(killer)
	}
}

// SetHP sets current HP, clamped to [0, MaxHP].
func (c *Character) SetHP(value float64) {
	maxHP := c.MaxHPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
	if value < 0 {
		value = 0
	}
	if value > maxHP {
		value = maxHP
	}
	c.curHP = value
}

// SetCP sets current CP, clamped to [0, MaxCP].
func (c *Character) SetCP(value float64) {
	maxCP := c.MaxCPValue()
	c.vitalsMu.Lock()
	defer c.vitalsMu.Unlock()
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

// HealAmount resolves c's outgoing HEAL amount before target effectiveness.
func (c *Character) HealAmount(def modelskill.Definition) (float64, bool) {
	amount := float64(def.Power) + c.HealProficiency()
	if skillTypeKey(def.SkillType) == "HEAL_STATIC" {
		return amount, true
	}
	return amount + math.Sqrt(float64(int(c.MAtk()))), true
}

// PhysicalSkillInput resolves the damage formula input for a physical skill
// cast by caster against c.
func (c *Character) PhysicalSkillInput(caster any, def modelskill.Definition) (formulas.PhysicalSkillInput, bool) {
	attacker, ok := caster.(*Character)
	if !ok || attacker == nil {
		return formulas.PhysicalSkillInput{}, false
	}
	soulshot := attacker.SoulshotCharged()
	skillPower := float64(def.Power)
	if soulshot && def.SoulShotBoost > 0 {
		skillPower *= float64(def.SoulShotBoost)
	}
	return formulas.PhysicalSkillInput{
		AttackPower:   attacker.PAtk(),
		SkillPower:    skillPower,
		Defence:       positive(c.PDef()),
		Crit:          attacker.physicalSkillCrit(def),
		SoulShot:      soulshot,
		RandomMul:     attacker.randomDamageMultiplier(def),
		ElementalMul:  c.elementalSkillModifier(def),
		RaceMul:       1,
		WeaponVulnMul: c.weaponVulnerability(attacker),
		PvPMul:        attacker.calcStat(stat.PvPPhysSkillDmg, 1),
	}, true
}

// MagicDamageInput resolves the damage formula input for a magic skill cast
// by caster against c.
func (c *Character) MagicDamageInput(caster any, def modelskill.Definition) (formulas.MagicDamageInput, bool) {
	attacker, ok := caster.(*Character)
	if !ok || attacker == nil {
		return formulas.MagicDamageInput{}, false
	}
	sps, bsps := attacker.spiritshotFlags()
	return formulas.MagicDamageInput{
		MAtk:            attacker.MAtk(),
		MDef:            positive(c.MDef()),
		SkillPower:      float64(def.Power),
		PvPMul:          attacker.magicPvPMul(def),
		ElementalMul:    c.elementalSkillModifier(def),
		MagicCrit:       formulas.MCritSucceeds(int(attacker.MagicCriticalRate()), attacker.rollValue(1000)),
		SoulShot:        sps,
		BlessedSoulShot: bsps,
	}, true
}

// BlowInput resolves the damage formula input for a blow skill cast by
// caster against c.
func (c *Character) BlowInput(caster any, def modelskill.Definition) (formulas.BlowInput, bool) {
	attacker, ok := caster.(*Character)
	if !ok || attacker == nil {
		return formulas.BlowInput{}, false
	}
	soulshot := attacker.SoulshotCharged()
	skillPower := float64(def.Power)
	if soulshot && def.SoulShotBoost > 0 {
		skillPower *= float64(def.SoulShotBoost)
	}
	return formulas.BlowInput{
		AttackPower:       attacker.PAtk(),
		SkillPower:        skillPower,
		Defence:           positive(c.PDef()),
		SoulShot:          soulshot,
		IsPvP:             true,
		RandomMul:         float64(95+attacker.rollValue(11)) / 100,
		PosMul:            c.positionMultiplierFrom(attacker, true),
		PvPMul:            attacker.calcStat(stat.PvPPhysSkillDmg, 1),
		CritDamageMul:     attacker.calcStat(stat.CriticalDamage, 1),
		CritDamagePosMul:  (attacker.calcStat(stat.CriticalDamagePos, 1)-1)/2 + 1,
		CritVulnMul:       c.calcStat(stat.CritVuln, 1),
		DaggerVulnMul:     c.calcStat(stat.DaggerWpnVuln, 1),
		CritDamageAddBase: attacker.calcStat(stat.CriticalDamageAdd, 0),
	}, true
}

// ManaDamageInput resolves the MP-damage formula input for a magic skill
// cast by caster against c.
func (c *Character) ManaDamageInput(caster any, def modelskill.Definition) (formulas.ManaDamageInput, bool) {
	attacker, ok := caster.(*Character)
	if !ok || attacker == nil {
		return formulas.ManaDamageInput{}, false
	}
	sps, bsps := attacker.spiritshotFlags()
	return formulas.ManaDamageInput{
		MAtk:            attacker.MAtk(),
		MDef:            positive(c.MDef()),
		SkillPower:      float64(def.Power),
		TargetMaxMp:     c.MaxMPValue(),
		SoulShot:        sps,
		BlessedSoulShot: bsps,
		VulnMul:         c.skillVulnerability(def.SkillType, def),
	}, true
}

// LethalRate returns c's lethal-strike rate multiplier.
func (c *Character) LethalRate() float64 {
	return c.calcStat(stat.LethalRate, 1)
}

// LethalInput resolves a lethal-strike roll against c.
func (c *Character) LethalInput(caster any, def modelskill.Definition) (formulas.LethalInput, bool) {
	attacker, ok := caster.(interface {
		Level() int
		LethalRate() float64
	})
	if !ok {
		return formulas.LethalInput{}, false
	}
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
func (c *Character) ApplyLethalOutcome(outcome formulas.LethalOutcome, _ any, _ modelskill.Definition) {
	switch outcome {
	case formulas.LethalFull:
		c.SetHP(1)
		c.SetCP(1)
	case formulas.LethalHalf:
		c.SetCP(1)
	}
}

func (c *Character) elementalSkillModifier(def modelskill.Definition) float64 {
	s, ok := elementResistanceStat(def.Element)
	if !ok {
		return 1
	}
	return c.calcStat(s, 1)
}

func elementResistanceStat(element modelskill.Element) (stat.Stat, bool) {
	switch element {
	case modelskill.ElementWind:
		return stat.WindRes, true
	case modelskill.ElementFire:
		return stat.FireRes, true
	case modelskill.ElementWater:
		return stat.WaterRes, true
	case modelskill.ElementEarth:
		return stat.EarthRes, true
	case modelskill.ElementHoly:
		return stat.HolyRes, true
	case modelskill.ElementDark:
		return stat.DarkRes, true
	case modelskill.ElementValakas:
		return stat.ValakasRes, true
	default:
		return 0, false
	}
}

func (c *Character) spiritshotFlags() (sps, bsps bool) {
	bsps = c.BlessedSpiritshotCharged()
	if bsps {
		return false, true
	}
	return c.SpiritshotCharged(), false
}

func (c *Character) magicPvPMul(def modelskill.Definition) float64 {
	if def.Magic {
		return c.calcStat(stat.PvPMagicalDmg, 1)
	}
	return c.calcStat(stat.PvPPhysSkillDmg, 1)
}

func (c *Character) positionMultiplierFrom(attacker *Character, crit bool) float64 {
	targetFacing := location.OrientedLocation{Location: c.CurrentLocation(), Heading: c.CurrentHeading()}
	attackerLoc := attacker.CurrentLocation()
	return formulas.PosMul(targetFacing.IsBehind(attackerLoc), targetFacing.IsInFrontOf(attackerLoc), crit)
}

func (c *Character) physicalSkillCrit(def modelskill.Definition) bool {
	if def.BaseCritRate <= 0 {
		return false
	}
	rate := float64(def.BaseCritRate) * 10 * statbonus.STRBonus[c.STR()]
	return formulas.CritSucceeds(rate, c.rollValue(1000))
}

func (c *Character) randomDamageMultiplier(def modelskill.Definition) float64 {
	if skillTypeKey(def.EffectType) == "CHARGEDAM" {
		return 1
	}
	weapon := c.activeWeapon()
	if weapon.tmpl == nil || weapon.tmpl.Weapon == nil || weapon.tmpl.Weapon.RandomDamage <= 0 {
		return 1
	}
	spread := int(weapon.tmpl.Weapon.RandomDamage)
	return 1 + float64(c.rollValue(2*spread+1)-spread)/100
}

func (c *Character) weaponVulnerability(attacker *Character) float64 {
	switch attacker.AttackType() {
	case item.WeaponSword:
		return c.calcStat(stat.SwordWpnVuln, 1)
	case item.WeaponBlunt:
		return c.calcStat(stat.BluntWpnVuln, 1)
	case item.WeaponDagger:
		return c.calcStat(stat.DaggerWpnVuln, 1)
	case item.WeaponBow:
		return c.calcStat(stat.BowWpnVuln, 1)
	case item.WeaponPole:
		return c.calcStat(stat.PoleWpnVuln, 1)
	case item.WeaponDual:
		return c.calcStat(stat.DualWpnVuln, 1)
	case item.WeaponDualFist:
		return c.calcStat(stat.DualFistWpnVuln, 1)
	case item.WeaponBigSword:
		return c.calcStat(stat.BigSwordWpnVuln, 1)
	case item.WeaponBigBlunt:
		return c.calcStat(stat.BigBluntWpnVuln, 1)
	default:
		return 1
	}
}

func positive(v float64) float64 {
	if v <= 0 {
		return 1
	}
	return v
}
