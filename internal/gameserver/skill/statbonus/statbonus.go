// Package statbonus holds the precomputed per-attribute-value multiplier
// tables the stat calculation chain and combat formulas both read from: one
// row per possible base attribute value (0..MaxStatValue-1), giving the
// multiplier that value contributes to the attack/defense/regen stat it
// backs. The tables are fixed math, not configuration — recomputed once at
// package init from the same closed-form curve for each attribute.
package statbonus

import "math"

// MaxStatValue is one past the highest base attribute value the tables
// cover; a value here is used as a direct index into every table below.
const MaxStatValue = 100

// curve is the (base, offset) pair the closed form
// floor(base^(i-offset) * 100 + 0.5) / 100 uses to build one attribute's
// bonus table.
type curve struct {
	base   float64
	offset float64
}

var (
	strCurve = curve{1.036, 34.845}
	intCurve = curve{1.020, 31.375}
	dexCurve = curve{1.009, 19.360}
	witCurve = curve{1.050, 20.000}
	conCurve = curve{1.030, 27.632}
	menCurve = curve{1.010, -0.060}
)

// STRBonus, CONBonus, DEXBonus, INTBonus, WITBonus and MENBonus give the
// multiplier a base attribute value of i contributes: STRBonus for P.Atk,
// CONBonus for max HP/CP and HP/CP regen, DEXBonus for evasion/critical
// rate/attack speed/run speed, INTBonus for M.Atk, WITBonus for
// M.Atk-critical/M.Atk-speed, MENBonus for M.Def/max MP/MP regen.
var (
	STRBonus [MaxStatValue]float64
	CONBonus [MaxStatValue]float64
	DEXBonus [MaxStatValue]float64
	INTBonus [MaxStatValue]float64
	WITBonus [MaxStatValue]float64
	MENBonus [MaxStatValue]float64

	// BaseEvasionAccuracy is the shared term both evasion rate and
	// accuracy add on top of level: sqrt(DEX) * 6.
	BaseEvasionAccuracy [MaxStatValue]float64
)

func buildCurve(c curve) [MaxStatValue]float64 {
	var table [MaxStatValue]float64
	for i := range table {
		table[i] = math.Floor(math.Pow(c.base, float64(i)-c.offset)*100+0.5) / 100
	}
	return table
}

func init() {
	STRBonus = buildCurve(strCurve)
	CONBonus = buildCurve(conCurve)
	DEXBonus = buildCurve(dexCurve)
	INTBonus = buildCurve(intCurve)
	WITBonus = buildCurve(witCurve)
	MENBonus = buildCurve(menCurve)

	for i := range BaseEvasionAccuracy {
		BaseEvasionAccuracy[i] = math.Sqrt(float64(i)) * 6
	}
}
