// Package stat identifies the named quantities the calculation chain
// (basefunc/funcs) and skill/item data can target: hp/mp/cp, attack and
// defense values, rates, resistances, and the six base attributes. A Stat is
// just a key — computing or applying one is the calculation chain's job.
package stat

import "fmt"

// Stat is one calculable quantity. Its zero value, MaxHP, is never a valid
// "unset" sentinel — every Stat is a deliberate value from the table below.
type Stat uint8

const (
	MaxHP Stat = iota
	MaxMP
	MaxCP
	RegenerateHPRate
	RegenerateCPRate
	RegenerateMPRate
	RechargeMPRate
	HealEffectiveness
	HealProficiency

	PowerDefence
	MagicDefence
	PowerAttack
	MagicAttack
	PowerAttackSpeed
	MagicAttackSpeed
	MagicReuseRate
	PReuse
	ShieldDefence
	ShieldDefenceAngle
	ShieldRate

	CriticalDamage
	CriticalDamagePos
	CriticalDamageAdd

	PvPPhysicalDmg
	PvPMagicalDmg
	PvPPhysSkillDmg
	PvPPhysSkillDef

	EvasionRate
	PSkillEvasion
	CriticalRate
	BlowRate
	LethalRate
	MCriticalRate
	AttackCancel

	AccuracyCombat
	PowerAttackRange
	PowerAttackAngle
	AttackCountMax

	RunSpeed

	StatSTR
	StatCON
	StatDEX
	StatINT
	StatWIT
	StatMEN

	Breath
	Fall

	Aggression
	Bleed
	Poison
	Stun
	Root
	Movement
	Confusion
	Sleep

	FireRes
	WaterRes
	WindRes
	EarthRes
	HolyRes
	DarkRes
	ValakasRes

	FirePower
	WaterPower
	WindPower
	EarthPower
	HolyPower
	DarkPower
	ValakasPower

	BleedVuln
	PoisonVuln
	StunVuln
	ParalyzeVuln
	RootVuln
	SleepVuln
	DamageZoneVuln
	CritVuln
	CancelVuln
	DerangementVuln
	DebuffVuln

	SwordWpnVuln
	BluntWpnVuln
	DaggerWpnVuln
	BowWpnVuln
	PoleWpnVuln
	DualWpnVuln
	DualFistWpnVuln
	BigSwordWpnVuln
	BigBluntWpnVuln

	ReflectDamagePercent
	ReflectSkillMagic
	ReflectSkillPhysic
	CounterSkillPhysical
	AbsorbDamagePercent
	TransferDamagePercent

	PAtkPlants
	PAtkInsects
	PAtkAnimals
	PAtkBeasts
	PAtkDragons
	PAtkGiants
	PAtkMCreatures

	PDefPlants
	PDefInsects
	PDefAnimals
	PDefBeasts
	PDefDragons
	PDefGiants
	PDefMCreatures

	WeightLimit
	WeightPenalty
	InvLim
	WhLim
	FreightLim
	PSellLim
	PBuyLim
	RecDLim
	RecCLim

	PhysicalMpConsumeRate
	MagicalMpConsumeRate
	DanceMpConsumeRate

	SkillMastery

	numStats
)

// info is the per-Stat metadata: its data-file spelling and whether a
// calculated value can legally go negative.
type info struct {
	name           string
	cantBeNegative bool
}

// table is indexed by Stat; every entry from MaxHP..SkillMastery must be
// present, in the same order as the const block above, so Stat(i)'s data
// lines up with table[i].
var table = [numStats]info{
	MaxHP:              {"maxHp", true},
	MaxMP:              {"maxMp", true},
	MaxCP:              {"maxCp", true},
	RegenerateHPRate:   {"regHp", false},
	RegenerateCPRate:   {"regCp", false},
	RegenerateMPRate:   {"regMp", false},
	RechargeMPRate:     {"gainMp", false},
	HealEffectiveness:  {"gainHp", false},
	HealProficiency:    {"giveHp", false},
	PowerDefence:       {"pDef", true},
	MagicDefence:       {"mDef", true},
	PowerAttack:        {"pAtk", true},
	MagicAttack:        {"mAtk", true},
	PowerAttackSpeed:   {"pAtkSpd", true},
	MagicAttackSpeed:   {"mAtkSpd", true},
	MagicReuseRate:     {"mReuse", false},
	PReuse:             {"pReuse", false},
	ShieldDefence:      {"sDef", true},
	ShieldDefenceAngle: {"shieldDefAngle", false},
	ShieldRate:         {"rShld", false},

	CriticalDamage:    {"cAtk", false},
	CriticalDamagePos: {"cAtkPos", false},
	CriticalDamageAdd: {"cAtkAdd", false},

	PvPPhysicalDmg:  {"pvpPhysDmg", false},
	PvPMagicalDmg:   {"pvpMagicalDmg", false},
	PvPPhysSkillDmg: {"pvpPhysSkillsDmg", false},
	PvPPhysSkillDef: {"pvpPhysSkillsDef", false},

	EvasionRate:   {"rEvas", false},
	PSkillEvasion: {"pSkillEvas", false},
	CriticalRate:  {"rCrit", false},
	BlowRate:      {"blowRate", false},
	LethalRate:    {"lethalRate", false},
	MCriticalRate: {"mCritRate", false},
	AttackCancel:  {"cancel", false},

	AccuracyCombat:   {"accCombat", false},
	PowerAttackRange: {"pAtkRange", false},
	PowerAttackAngle: {"pAtkAngle", false},
	AttackCountMax:   {"atkCountMax", false},

	RunSpeed: {"runSpd", false},

	StatSTR: {"STR", true},
	StatCON: {"CON", true},
	StatDEX: {"DEX", true},
	StatINT: {"INT", true},
	StatWIT: {"WIT", true},
	StatMEN: {"MEN", true},

	Breath: {"breath", false},
	Fall:   {"fall", false},

	Aggression: {"aggression", false},
	Bleed:      {"bleed", false},
	Poison:     {"poison", false},
	Stun:       {"stun", false},
	Root:       {"root", false},
	Movement:   {"movement", false},
	Confusion:  {"confusion", false},
	Sleep:      {"sleep", false},

	FireRes:    {"fireRes", false},
	WaterRes:   {"waterRes", false},
	WindRes:    {"windRes", false},
	EarthRes:   {"earthRes", false},
	HolyRes:    {"holyRes", false},
	DarkRes:    {"darkRes", false},
	ValakasRes: {"valakasRes", false},

	FirePower:    {"firePower", false},
	WaterPower:   {"waterPower", false},
	WindPower:    {"windPower", false},
	EarthPower:   {"earthPower", false},
	HolyPower:    {"holyPower", false},
	DarkPower:    {"darkPower", false},
	ValakasPower: {"valakasPower", false},

	BleedVuln:       {"bleedVuln", false},
	PoisonVuln:      {"poisonVuln", false},
	StunVuln:        {"stunVuln", false},
	ParalyzeVuln:    {"paralyzeVuln", false},
	RootVuln:        {"rootVuln", false},
	SleepVuln:       {"sleepVuln", false},
	DamageZoneVuln:  {"damageZoneVuln", false},
	CritVuln:        {"critVuln", false},
	CancelVuln:      {"cancelVuln", false},
	DerangementVuln: {"derangementVuln", false},
	DebuffVuln:      {"debuffVuln", false},

	SwordWpnVuln:    {"swordWpnVuln", false},
	BluntWpnVuln:    {"bluntWpnVuln", false},
	DaggerWpnVuln:   {"daggerWpnVuln", false},
	BowWpnVuln:      {"bowWpnVuln", false},
	PoleWpnVuln:     {"poleWpnVuln", false},
	DualWpnVuln:     {"dualWpnVuln", false},
	DualFistWpnVuln: {"dualFistWpnVuln", false},
	BigSwordWpnVuln: {"bigSwordWpnVuln", false},
	BigBluntWpnVuln: {"bigBluntWpnVuln", false},

	ReflectDamagePercent:  {"reflectDam", false},
	ReflectSkillMagic:     {"reflectSkillMagic", false},
	ReflectSkillPhysic:    {"reflectSkillPhysic", false},
	CounterSkillPhysical:  {"counterSkill", false},
	AbsorbDamagePercent:   {"absorbDam", false},
	TransferDamagePercent: {"transDam", false},

	PAtkPlants:     {"pAtk-plants", false},
	PAtkInsects:    {"pAtk-insects", false},
	PAtkAnimals:    {"pAtk-animals", false},
	PAtkBeasts:     {"pAtk-beasts", false},
	PAtkDragons:    {"pAtk-dragons", false},
	PAtkGiants:     {"pAtk-giants", false},
	PAtkMCreatures: {"pAtk-magicCreature", false},

	PDefPlants:     {"pDef-plants", false},
	PDefInsects:    {"pDef-insects", false},
	PDefAnimals:    {"pDef-animals", false},
	PDefBeasts:     {"pDef-beasts", false},
	PDefDragons:    {"pDef-dragons", false},
	PDefGiants:     {"pDef-giants", false},
	PDefMCreatures: {"pDef-magicCreature", false},

	WeightLimit:   {"weightLimit", false},
	WeightPenalty: {"weightPenalty", false},
	InvLim:        {"inventoryLimit", false},
	WhLim:         {"whLimit", false},
	FreightLim:    {"FreightLimit", false},
	PSellLim:      {"PrivateSellLimit", false},
	PBuyLim:       {"PrivateBuyLimit", false},
	RecDLim:       {"DwarfRecipeLimit", false},
	RecCLim:       {"CommonRecipeLimit", false},

	PhysicalMpConsumeRate: {"PhysicalMpConsumeRate", false},
	MagicalMpConsumeRate:  {"MagicalMpConsumeRate", false},
	DanceMpConsumeRate:    {"DanceMpConsumeRate", false},

	SkillMastery: {"skillMastery", false},
}

// byName resolves a data-file spelling back to its Stat, built once from
// table so ByName never has to scan linearly.
var byName = func() map[string]Stat {
	m := make(map[string]Stat, numStats)
	for i := Stat(0); i < numStats; i++ {
		m[table[i].name] = i
	}
	return m
}()

// Name returns the data-file spelling of s (e.g. "pAtk" for PowerAttack).
func (s Stat) Name() string {
	if s >= numStats {
		return fmt.Sprintf("Stat(%d)", uint8(s))
	}
	return table[s].name
}

// String supports fmt printing; it returns the same value as Name.
func (s Stat) String() string { return s.Name() }

// CantBeNegative reports whether a calculated value for s must never drop
// below zero (true for the "hard" stats: hp/mp/cp pools, atk/def, attack
// speed, and the six base attributes).
func (s Stat) CantBeNegative() bool {
	if s >= numStats {
		return false
	}
	return table[s].cantBeNegative
}

// ByName resolves name (an exact, case-sensitive data-file spelling, e.g.
// from a skill or item XML attribute) to its Stat. It reports an error for
// any name not in the table, matching how a malformed data file should fail
// loudly rather than default to an arbitrary Stat.
func ByName(name string) (Stat, error) {
	if s, ok := byName[name]; ok {
		return s, nil
	}
	return 0, fmt.Errorf("stat: unknown name %q", name)
}
