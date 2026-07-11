package funcs

import (
	"math"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/statbonus"
)

// fakeActor is a minimal Actor for exercising the finalize funcs without a
// real creature runtime.
type fakeActor struct {
	str, con, dex, intv, wit, men int
	level                         int
	levelMod                      float64
	isSummon                      bool
}

func (a fakeActor) STR() int          { return a.str }
func (a fakeActor) CON() int          { return a.con }
func (a fakeActor) DEX() int          { return a.dex }
func (a fakeActor) INT() int          { return a.intv }
func (a fakeActor) WIT() int          { return a.wit }
func (a fakeActor) MEN() int          { return a.men }
func (a fakeActor) Level() int        { return a.level }
func (a fakeActor) LevelMod() float64 { return a.levelMod }
func (a fakeActor) IsSummon() bool    { return a.isSummon }

// fakePlayer adds the PlayerActor surface on top of fakeActor.
type fakePlayer struct {
	fakeActor
	mage      bool
	henna     map[stat.Stat]float64
	equipped  int
	hasWeapon bool
}

func (p fakePlayer) IsMageClass() bool { return p.mage }
func (p fakePlayer) HennaBonus(s stat.Stat) float64 {
	return p.henna[s]
}
func (p fakePlayer) HasEquipped(slotMask int) bool { return p.equipped&slotMask != 0 }
func (p fakePlayer) HasWeaponEquipped() bool       { return p.hasWeapon }

func almostEqual(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestAtkAccuracy(t *testing.T) {
	npc := fakeActor{dex: 40, level: 20}
	// BaseEvasionAccuracy[40] = sqrt(40)*6.
	want := math.Sqrt(40)*6 + 20
	got := AtkAccuracy.Calc(npc, nil, nil, 0, 0)
	if !almostEqual(got, want) {
		t.Errorf("npc: Calc() = %v, want %v", got, want)
	}

	summonLow := fakeActor{dex: 40, level: 59, isSummon: true}
	wantSummonLow := math.Sqrt(40)*6 + 59 + 4
	if got := AtkAccuracy.Calc(summonLow, nil, nil, 0, 0); !almostEqual(got, wantSummonLow) {
		t.Errorf("summon<60: Calc() = %v, want %v", got, wantSummonLow)
	}

	summonHigh := fakeActor{dex: 40, level: 60, isSummon: true}
	wantSummonHigh := math.Sqrt(40)*6 + 60 + 5
	if got := AtkAccuracy.Calc(summonHigh, nil, nil, 0, 0); !almostEqual(got, wantSummonHigh) {
		t.Errorf("summon>=60: Calc() = %v, want %v", got, wantSummonHigh)
	}
}

func TestAtkCritical(t *testing.T) {
	npc := fakeActor{dex: 40}
	// value(1) * DEXBonus[40] * 10.
	got := AtkCritical.Calc(npc, nil, nil, 0, 1)
	want := statbonus.DEXBonus[40] * 10
	if !almostEqual(got, want) {
		t.Errorf("npc: Calc() = %v, want %v", got, want)
	}

	summon := fakeActor{dex: 40, isSummon: true}
	if got := AtkCritical.Calc(summon, nil, nil, 0, 1); got != 10 {
		t.Errorf("summon: Calc() = %v, want 10 (no DEX multiplier)", got)
	}
}

func TestMAtkCriticalWeaponGate(t *testing.T) {
	npc := fakeActor{wit: 30}
	want := statbonus.WITBonus[30]
	if got := MAtkCritical.Calc(npc, nil, nil, 0, 1); !almostEqual(got, want) {
		t.Errorf("npc: Calc() = %v, want %v", got, want)
	}

	armedPlayer := fakePlayer{fakeActor: fakeActor{wit: 30}, hasWeapon: true}
	if got := MAtkCritical.Calc(armedPlayer, nil, nil, 0, 1); !almostEqual(got, want) {
		t.Errorf("armed player: Calc() = %v, want %v", got, want)
	}

	bareHandedPlayer := fakePlayer{fakeActor: fakeActor{wit: 30}, hasWeapon: false}
	if got := MAtkCritical.Calc(bareHandedPlayer, nil, nil, 0, 1); got != 1 {
		t.Errorf("bare-handed player: Calc() = %v, want unchanged 1", got)
	}
}

func TestMDefModAccessoryPenalty(t *testing.T) {
	bare := fakePlayer{fakeActor: fakeActor{men: 10, levelMod: 1}}
	baseGot := MDefMod.Calc(bare, nil, nil, 0, 100)

	ringed := fakePlayer{fakeActor: fakeActor{men: 10, levelMod: 1}, equipped: SlotLFinger | SlotRFinger}
	ringedGot := MDefMod.Calc(ringed, nil, nil, 0, 100)

	wantDelta := -10 * statbonus.MENBonus[10] * 1 // two rings, -5 each, times the MEN/level multiplier
	if !almostEqual(ringedGot-baseGot, wantDelta) {
		t.Errorf("ring penalty delta = %v, want %v", ringedGot-baseGot, wantDelta)
	}
}

func TestPDefModMageVsFighter(t *testing.T) {
	mage := fakePlayer{fakeActor: fakeActor{levelMod: 1}, mage: true, equipped: SlotChest}
	fighter := fakePlayer{fakeActor: fakeActor{levelMod: 1}, mage: false, equipped: SlotChest}

	mageGot := PDefMod.Calc(mage, nil, nil, 0, 100)
	fighterGot := PDefMod.Calc(fighter, nil, nil, 0, 100)

	if !almostEqual(mageGot, 85) {
		t.Errorf("mage chest: Calc() = %v, want 85", mageGot)
	}
	if !almostEqual(fighterGot, 69) {
		t.Errorf("fighter chest: Calc() = %v, want 69", fighterGot)
	}
}

func TestPDefModFullBodyArmorAddsBothPenalties(t *testing.T) {
	// A full-body piece occupies the chest slot (triggering the chest
	// penalty) and additionally triggers the legs-equivalent penalty — the
	// two stack, they don't substitute for each other.
	fullBody := fakePlayer{fakeActor: fakeActor{levelMod: 1}, equipped: SlotChest | FullBodyArmor}
	legsOnly := fakePlayer{fakeActor: fakeActor{levelMod: 1}, equipped: SlotLegs}

	fullBodyGot := PDefMod.Calc(fullBody, nil, nil, 0, 100)
	legsGot := PDefMod.Calc(legsOnly, nil, nil, 0, 100)

	// Fighter: chest -31, plus the legs-equivalent -18 = 100-49 = 51.
	if !almostEqual(fullBodyGot, 51) {
		t.Errorf("full-body armor: Calc() = %v, want 51", fullBodyGot)
	}
	// Legs-only, no chest item: just -18 = 82.
	if !almostEqual(legsGot, 82) {
		t.Errorf("legs-only: Calc() = %v, want 82", legsGot)
	}
}

func TestHennaBonus(t *testing.T) {
	p := fakePlayer{henna: map[stat.Stat]float64{stat.StatSTR: 3}}
	if got := HennaSTR.Calc(p, nil, nil, 0, 40); got != 43 {
		t.Errorf("player with henna: Calc() = %v, want 43", got)
	}

	npc := fakeActor{}
	if got := HennaSTR.Calc(npc, nil, nil, 0, 40); got != 40 {
		t.Errorf("npc: Calc() = %v, want unchanged 40", got)
	}
}

func TestVitalsMultipliers(t *testing.T) {
	a := fakeActor{con: 20, men: 30, levelMod: 1.5}

	if got, want := MaxHpMul.Calc(a, nil, nil, 0, 100), 100*statbonus.CONBonus[20]; !almostEqual(got, want) {
		t.Errorf("MaxHpMul: Calc() = %v, want %v", got, want)
	}
	if got, want := MaxMpMul.Calc(a, nil, nil, 0, 100), 100*statbonus.MENBonus[30]; !almostEqual(got, want) {
		t.Errorf("MaxMpMul: Calc() = %v, want %v", got, want)
	}
	if got, want := RegenCpMul.Calc(a, nil, nil, 0, 100), 100*statbonus.CONBonus[20]*1.5; !almostEqual(got, want) {
		t.Errorf("RegenCpMul: Calc() = %v, want %v", got, want)
	}
}

func TestMoveSpeed(t *testing.T) {
	a := fakeActor{dex: 50}
	want := 1.0 * statbonus.DEXBonus[50]
	if got := MoveSpeed.Calc(a, nil, nil, 0, 1); !almostEqual(got, want) {
		t.Errorf("Calc() = %v, want %v", got, want)
	}
}
