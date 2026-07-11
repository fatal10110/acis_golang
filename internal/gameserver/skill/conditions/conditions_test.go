package conditions

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/basefunc"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// Static interface-satisfaction checks: every leaf/logic condition type
// must be usable as a basefunc.Condition (what a Func's Cond() carries).
var (
	_ basefunc.Condition = Level{}
	_ basefunc.Condition = Hp{}
	_ basefunc.Condition = Mp{}
	_ basefunc.Condition = PkCount{}
	_ basefunc.Condition = HasCastle{}
	_ basefunc.Condition = HasClanHall{}
	_ basefunc.Condition = InvSize{}
	_ basefunc.Condition = IsHero{}
	_ basefunc.Condition = PledgeClass{}
	_ basefunc.Condition = Race{}
	_ basefunc.Condition = Sex{}
	_ basefunc.Condition = PlayerState{}
	_ basefunc.Condition = Weight{}
	_ basefunc.Condition = Charges{}
	_ basefunc.Condition = ActiveEffectID{}
	_ basefunc.Condition = ActiveSkillID{}
	_ basefunc.Condition = InsidePoly{}
	_ basefunc.Condition = TargetActiveSkillID{}
	_ basefunc.Condition = TargetHpMinMax{}
	_ basefunc.Condition = TargetNpcID{}
	_ basefunc.Condition = TargetRaceID{}
	_ basefunc.Condition = SkillStats{}
	_ basefunc.Condition = UsingItemType{}
	_ basefunc.Condition = GameChance{}
	_ basefunc.Condition = GameTime{}
	_ basefunc.Condition = ElementSeed{}
	_ basefunc.Condition = ForceBuff{}
	_ basefunc.Condition = &And{}
	_ basefunc.Condition = &Or{}
	_ basefunc.Condition = Not{}
)

type fakeActor struct {
	level   int
	hpRatio float64
	mpRatio float64
	x, y, z int
	moving  bool
	running bool
	riding  bool
	flying  bool
	behind  bool
	front   bool
	skills  map[int]int
	effects map[int]int
}

func (a fakeActor) Level() int             { return a.level }
func (a fakeActor) HPRatio() float64       { return a.hpRatio }
func (a fakeActor) MPRatio() float64       { return a.mpRatio }
func (a fakeActor) X() int                 { return a.x }
func (a fakeActor) Y() int                 { return a.y }
func (a fakeActor) Z() int                 { return a.z }
func (a fakeActor) IsMoving() bool         { return a.moving }
func (a fakeActor) IsRunning() bool        { return a.running }
func (a fakeActor) IsRiding() bool         { return a.riding }
func (a fakeActor) IsFlying() bool         { return a.flying }
func (a fakeActor) IsBehind(Actor) bool    { return a.behind }
func (a fakeActor) IsInFrontOf(Actor) bool { return a.front }
func (a fakeActor) ActiveSkillLevel(id int) (int, bool) {
	lv, ok := a.skills[id]
	return lv, ok
}
func (a fakeActor) ActiveEffectLevel(id int) (int, bool) {
	lv, ok := a.effects[id]
	return lv, ok
}

type fakePlayer struct {
	fakeActor
	sitting     bool
	olympiad    bool
	hero        bool
	pkKills     int
	pledgeClass int
	clanLeader  bool
	hasClan     bool
	clanCastle  int
	anyCastle   bool
	clanHall    int
	anyClanHall bool
	race        player.Race
	sex         player.Sex
	weightTier  int
	invSize     int
	invLimit    int
	charges     int
	wearingMask int
}

func (p fakePlayer) IsSitting() bool             { return p.sitting }
func (p fakePlayer) IsInOlympiadMode() bool      { return p.olympiad }
func (p fakePlayer) IsHero() bool                { return p.hero }
func (p fakePlayer) PkKills() int                { return p.pkKills }
func (p fakePlayer) PledgeClass() int            { return p.pledgeClass }
func (p fakePlayer) IsClanLeader() bool          { return p.clanLeader }
func (p fakePlayer) HasClan() bool               { return p.hasClan }
func (p fakePlayer) ClanCastleID() int           { return p.clanCastle }
func (p fakePlayer) ClanHasAnyCastle() bool      { return p.anyCastle }
func (p fakePlayer) ClanHallID() int             { return p.clanHall }
func (p fakePlayer) ClanHasAnyClanHall() bool    { return p.anyClanHall }
func (p fakePlayer) Race() player.Race           { return p.race }
func (p fakePlayer) Sex() player.Sex             { return p.sex }
func (p fakePlayer) WeightPenalty() int          { return p.weightTier }
func (p fakePlayer) InventorySize() int          { return p.invSize }
func (p fakePlayer) InventoryLimit() int         { return p.invLimit }
func (p fakePlayer) Charges() int                { return p.charges }
func (p fakePlayer) IsWearingType(mask int) bool { return p.wearingMask&mask != 0 }

func TestLevelHpMp(t *testing.T) {
	a := fakeActor{level: 40, hpRatio: 0.3, mpRatio: 0.5}
	if !(Level{Level: 40}).Test(a, nil, nil) {
		t.Error("Level{40} should pass at level 40")
	}
	if (Level{Level: 41}).Test(a, nil, nil) {
		t.Error("Level{41} should fail at level 40")
	}
	if !(Hp{Percent: 30}).Test(a, nil, nil) {
		t.Error("Hp{30} should pass at 30% hp")
	}
	if (Hp{Percent: 29}).Test(a, nil, nil) {
		t.Error("Hp{29} should fail at 30% hp")
	}
	if !(Mp{Percent: 50}).Test(a, nil, nil) {
		t.Error("Mp{50} should pass at 50% mp")
	}
}

func TestPkCountNonPlayerFails(t *testing.T) {
	if (PkCount{Max: 5}).Test(fakeActor{}, nil, nil) {
		t.Error("PkCount should fail for a non-player effector")
	}
	p := fakePlayer{pkKills: 3}
	if !(PkCount{Max: 5}).Test(p, nil, nil) {
		t.Error("PkCount{5} should pass with 3 kills")
	}
	if (PkCount{Max: 2}).Test(p, nil, nil) {
		t.Error("PkCount{2} should fail with 3 kills")
	}
}

func TestHasCastle(t *testing.T) {
	noClan := fakePlayer{}
	if !(HasCastle{CastleID: 0}).Test(noClan, nil, nil) {
		t.Error("CastleID 0 should pass for a clanless player")
	}
	if (HasCastle{CastleID: 3}).Test(noClan, nil, nil) {
		t.Error("CastleID 3 should fail for a clanless player")
	}

	withCastle := fakePlayer{hasClan: true, clanCastle: 3, anyCastle: true}
	if !(HasCastle{CastleID: 3}).Test(withCastle, nil, nil) {
		t.Error("exact castle id should match")
	}
	if (HasCastle{CastleID: 4}).Test(withCastle, nil, nil) {
		t.Error("wrong castle id should not match")
	}
	if !(HasCastle{CastleID: -1}).Test(withCastle, nil, nil) {
		t.Error("-1 (any castle) should match a clan owning one")
	}

	noCastleClan := fakePlayer{hasClan: true, anyCastle: false}
	if (HasCastle{CastleID: -1}).Test(noCastleClan, nil, nil) {
		t.Error("-1 (any castle) should fail for a clan owning none")
	}
}

func TestHasClanHall(t *testing.T) {
	noClan := fakePlayer{}
	if !(HasClanHall{ClanHallIDs: []int{0}}).Test(noClan, nil, nil) {
		t.Error("[0] should pass for a clanless player")
	}

	withHall := fakePlayer{hasClan: true, clanHall: 5, anyClanHall: true}
	if !(HasClanHall{ClanHallIDs: []int{5, 6}}).Test(withHall, nil, nil) {
		t.Error("hall id in list should match")
	}
	if (HasClanHall{ClanHallIDs: []int{6, 7}}).Test(withHall, nil, nil) {
		t.Error("hall id not in list should not match")
	}
	if !(HasClanHall{ClanHallIDs: []int{-1}}).Test(withHall, nil, nil) {
		t.Error("[-1] (any hall) should match a clan owning one")
	}
}

func TestInvSize(t *testing.T) {
	if !(InvSize{Size: 3}).Test(fakeActor{}, nil, nil) {
		t.Error("InvSize should pass for a non-player (always true)")
	}
	p := fakePlayer{invSize: 90, invLimit: 100}
	if !(InvSize{Size: 10}).Test(p, nil, nil) {
		t.Error("90 <= 100-10 should pass")
	}
	if (InvSize{Size: 11}).Test(p, nil, nil) {
		t.Error("90 <= 100-11 should fail")
	}
}

func TestIsHeroPledgeClassRaceSex(t *testing.T) {
	hero := fakePlayer{hero: true}
	if !(IsHero{Want: true}).Test(hero, nil, nil) {
		t.Error("IsHero{true} should pass for a hero")
	}

	leader := fakePlayer{hasClan: true, clanLeader: true}
	if !(PledgeClass{Class: -1}).Test(leader, nil, nil) {
		t.Error("PledgeClass{-1} should pass for the clan leader")
	}
	member := fakePlayer{hasClan: true, pledgeClass: 3}
	if !(PledgeClass{Class: 3}).Test(member, nil, nil) {
		t.Error("PledgeClass{3} should pass at pledge class 3")
	}
	if (PledgeClass{Class: 4}).Test(member, nil, nil) {
		t.Error("PledgeClass{4} should fail at pledge class 3")
	}

	orc := fakePlayer{race: player.RaceOrc}
	if !(Race{Race: player.RaceOrc}).Test(orc, nil, nil) {
		t.Error("Race{Orc} should pass for an orc")
	}
	if (Race{Race: player.RaceElf}).Test(orc, nil, nil) {
		t.Error("Race{Elf} should fail for an orc")
	}

	female := fakePlayer{sex: player.SexFemale}
	if !(Sex{Sex: 1}).Test(female, nil, nil) {
		t.Error("Sex{1} should pass for a female")
	}
}

func TestPlayerState(t *testing.T) {
	a := fakeActor{moving: true, running: true, riding: true, flying: true, behind: true, front: false}
	target := fakeActor{}

	if !(PlayerState{Check: StateMoving, Required: true}).Test(a, target, nil) {
		t.Error("Moving should be true")
	}
	if !(PlayerState{Check: StateRunning, Required: true}).Test(a, target, nil) {
		t.Error("Running should be true")
	}
	if !(PlayerState{Check: StateRiding, Required: true}).Test(a, target, nil) {
		t.Error("Riding should be true")
	}
	if !(PlayerState{Check: StateBehind, Required: true}).Test(a, target, nil) {
		t.Error("Behind should be true")
	}
	if !(PlayerState{Check: StateFront, Required: false}).Test(a, target, nil) {
		t.Error("Front should be false")
	}

	// Resting/Olympiad on a non-player report the opposite of Required.
	if !(PlayerState{Check: StateResting, Required: false}).Test(a, target, nil) {
		t.Error("non-player Resting{false} should pass")
	}
	if (PlayerState{Check: StateResting, Required: true}).Test(a, target, nil) {
		t.Error("non-player Resting{true} should fail")
	}

	sitting := fakePlayer{sitting: true}
	if !(PlayerState{Check: StateResting, Required: true}).Test(sitting, target, nil) {
		t.Error("sitting player Resting{true} should pass")
	}
}

func TestWeightChargesActiveIDs(t *testing.T) {
	if !(Weight{Tier: 3}).Test(fakeActor{}, nil, nil) {
		t.Error("Weight should pass for a non-player")
	}
	p := fakePlayer{weightTier: 1, charges: 4}
	if !(Weight{Tier: 3}).Test(p, nil, nil) {
		t.Error("tier 1 < 3 should pass")
	}
	if (Weight{Tier: 1}).Test(p, nil, nil) {
		t.Error("tier 1 < 1 should fail")
	}
	if !(Charges{Min: 4}).Test(p, nil, nil) {
		t.Error("4 >= 4 charges should pass")
	}
	if (Charges{Min: 5}).Test(p, nil, nil) {
		t.Error("4 >= 5 charges should fail")
	}

	a := fakeActor{skills: map[int]int{100: 3}, effects: map[int]int{200: 2}}
	if !(ActiveSkillID{SkillID: 100, Level: -1}).Test(a, nil, nil) {
		t.Error("ActiveSkillID with Level -1 should pass when known")
	}
	if !(ActiveSkillID{SkillID: 100, Level: 3}).Test(a, nil, nil) {
		t.Error("ActiveSkillID at exact level should pass")
	}
	if (ActiveSkillID{SkillID: 100, Level: 4}).Test(a, nil, nil) {
		t.Error("ActiveSkillID above known level should fail")
	}
	if (ActiveSkillID{SkillID: 999, Level: -1}).Test(a, nil, nil) {
		t.Error("unknown skill id should fail")
	}
	if !(ActiveEffectID{EffectID: 200, Level: -1}).Test(a, nil, nil) {
		t.Error("ActiveEffectID with Level -1 should pass when active")
	}
	if (ActiveEffectID{EffectID: 999, Level: -1}).Test(a, nil, nil) {
		t.Error("inactive effect id should fail")
	}
}

func TestInsidePoly(t *testing.T) {
	poly, err := zone.NewPolygon([]location.Point{{X: 0, Y: 0}, {X: 0, Y: 10}, {X: 10, Y: 10}, {X: 10, Y: 0}}, 0, 100)
	if err != nil {
		t.Fatalf("NewPolygon: %v", err)
	}
	inside := fakeActor{x: 5, y: 5, z: 50}
	outside := fakeActor{x: 50, y: 50, z: 50}

	if !(InsidePoly{Zone: poly, CheckInside: true}).Test(inside, nil, nil) {
		t.Error("point inside polygon should pass CheckInside=true")
	}
	if (InsidePoly{Zone: poly, CheckInside: true}).Test(outside, nil, nil) {
		t.Error("point outside polygon should fail CheckInside=true")
	}
	if !(InsidePoly{Zone: poly, CheckInside: false}).Test(outside, nil, nil) {
		t.Error("point outside polygon should pass CheckInside=false")
	}
}

func TestTargetConditions(t *testing.T) {
	target := fakeActor{hpRatio: 0.4, skills: map[int]int{50: 1}}
	if !(TargetHpMinMax{Min: 30, Max: 50}).Test(nil, target, nil) {
		t.Error("40%% hp should fall within [30,50]")
	}
	if (TargetHpMinMax{Min: 41, Max: 50}).Test(nil, target, nil) {
		t.Error("40%% hp should fall outside [41,50]")
	}
	if (TargetHpMinMax{Min: 0, Max: 100}).Test(nil, nil, nil) {
		t.Error("nil effected should always fail")
	}
	if !(TargetActiveSkillID{SkillID: 50}).Test(nil, target, nil) {
		t.Error("known target skill should pass")
	}
	if (TargetActiveSkillID{SkillID: 51}).Test(nil, target, nil) {
		t.Error("unknown target skill should fail")
	}
}

type fakeNpcTarget struct{ id int32 }

func (f fakeNpcTarget) NpcID() int32 { return f.id }

type fakeDoorTarget struct{ id int }

func (f fakeDoorTarget) DoorID() int { return f.id }

type fakeRaceTarget struct{ race int }

func (f fakeRaceTarget) RaceOrdinal() int { return f.race }

func TestTargetNpcAndRaceID(t *testing.T) {
	if !(TargetNpcID{IDs: []int{20, 30}}).Test(nil, fakeNpcTarget{id: 20}, nil) {
		t.Error("npc id in list should pass")
	}
	if (TargetNpcID{IDs: []int{20, 30}}).Test(nil, fakeNpcTarget{id: 40}, nil) {
		t.Error("npc id not in list should fail")
	}
	if !(TargetNpcID{IDs: []int{7}}).Test(nil, fakeDoorTarget{id: 7}, nil) {
		t.Error("door id in list should pass")
	}
	if (TargetNpcID{IDs: []int{7}}).Test(nil, "not-a-target", nil) {
		t.Error("neither npc nor door should fail")
	}

	if !(TargetRaceID{IDs: []int{3, 4}}).Test(nil, fakeRaceTarget{race: 3}, nil) {
		t.Error("race in list should pass")
	}
	if (TargetRaceID{IDs: []int{3, 4}}).Test(nil, fakeRaceTarget{race: 5}, nil) {
		t.Error("race not in list should fail")
	}
}

type fakeSkill struct{ stat stat.Stat }

func (s fakeSkill) Stat() stat.Stat { return s.stat }

func TestSkillStatsAndUsingItemType(t *testing.T) {
	if !(SkillStats{Stat: stat.PowerAttack}).Test(nil, nil, fakeSkill{stat: stat.PowerAttack}) {
		t.Error("matching skill stat should pass")
	}
	if (SkillStats{Stat: stat.PowerAttack}).Test(nil, nil, fakeSkill{stat: stat.MagicAttack}) {
		t.Error("non-matching skill stat should fail")
	}
	if (SkillStats{Stat: stat.PowerAttack}).Test(nil, nil, nil) {
		t.Error("nil skill should fail")
	}

	p := fakePlayer{wearingMask: 0b0110}
	if !(UsingItemType{Mask: 0b0010}).Test(p, nil, nil) {
		t.Error("overlapping mask should pass")
	}
	if (UsingItemType{Mask: 0b1000}).Test(p, nil, nil) {
		t.Error("non-overlapping mask should fail")
	}
}

func TestGameChanceDistribution(t *testing.T) {
	hits := 0
	const trials = 2000
	for i := 0; i < trials; i++ {
		if (GameChance{Percent: 100}).Test(nil, nil, nil) {
			hits++
		}
	}
	if hits != trials {
		t.Errorf("100%% chance should always pass, got %d/%d", hits, trials)
	}
	if (GameChance{Percent: 0}).Test(nil, nil, nil) {
		t.Error("0%% chance should never pass")
	}
}

type fakeClock struct{ night bool }

func (c fakeClock) IsNight() bool { return c.night }

func TestGameTime(t *testing.T) {
	if !(GameTime{Clock: fakeClock{night: true}, Night: true}).Test(nil, nil, nil) {
		t.Error("night clock + Night:true should pass")
	}
	if (GameTime{Clock: fakeClock{night: false}, Night: true}).Test(nil, nil, nil) {
		t.Error("day clock + Night:true should fail")
	}
}

type fakeSeedActor struct{ powers map[int]int }

func (s fakeSeedActor) SeedPower(id int) int { return s.powers[id] }

func TestElementSeed(t *testing.T) {
	// Fire=1285, Water=1286, Wind=1287. Require 2 fire, 1 water, 0 wind,
	// plus 1 "any" and a total of 2 remaining.
	seeded := fakeSeedActor{powers: map[int]int{1285: 3, 1286: 1, 1287: 0}}
	cond := ElementSeed{Required: [5]int{2, 1, 0, 1, 2}}
	// After flat costs: fire=1, water=0, wind=0. "any 1": consumes fire's
	// remaining charge -> fire=0. Total remaining = 0, which is < 2, so
	// this should actually fail; use a case that clearly passes instead.
	if cond.Test(seeded, nil, nil) {
		t.Error("insufficient total remaining charge after any-1 consumption should fail")
	}

	plenty := fakeSeedActor{powers: map[int]int{1285: 5, 1286: 5, 1287: 5}}
	if !cond.Test(plenty, nil, nil) {
		t.Error("plenty of charge on all three seeds should pass")
	}

	insufficientFlat := ElementSeed{Required: [5]int{10, 0, 0, 0, 0}}
	if insufficientFlat.Test(plenty, nil, nil) {
		t.Error("insufficient flat fire requirement should fail")
	}

	// A non-SeedActor effector reports every seed as uncharged (0), so any
	// positive requirement fails closed.
	if (ElementSeed{Required: [5]int{1, 0, 0, 0, 0}}).Test(fakeActor{}, nil, nil) {
		t.Error("non-seed actor should fail a positive seed requirement")
	}
	if !(ElementSeed{Required: [5]int{0, 0, 0, 0, 0}}).Test(fakeActor{}, nil, nil) {
		t.Error("all-zero requirement should always pass")
	}
}

type fakeForceActor struct{ forces map[int]int }

func (f fakeForceActor) ForceLevel(id int) (int, bool) {
	lv, ok := f.forces[id]
	return lv, ok
}

func TestForceBuff(t *testing.T) {
	withForces := fakeForceActor{forces: map[int]int{battleForceSkillID: 2, spellForceSkillID: 1}}
	if !(ForceBuff{BattleForce: 2}).Test(withForces, nil, nil) {
		t.Error("exact battle force level should pass")
	}
	if (ForceBuff{BattleForce: 3}).Test(withForces, nil, nil) {
		t.Error("battle force above active level should fail")
	}
	if !(ForceBuff{SpellForce: 1}).Test(withForces, nil, nil) {
		t.Error("exact spell force level should pass")
	}
	if !(ForceBuff{}).Test(fakeActor{}, nil, nil) {
		t.Error("zero requirement should always pass, even for a non-force actor")
	}
	if (ForceBuff{BattleForce: 1}).Test(fakeActor{}, nil, nil) {
		t.Error("non-force actor should fail a positive requirement")
	}
}

func TestLogicCombinators(t *testing.T) {
	pass := GameChance{Percent: 100}
	fail := GameChance{Percent: 0}

	var and And
	and.Add(pass)
	and.Add(pass)
	if !and.Test(nil, nil, nil) {
		t.Error("And of two passing conditions should pass")
	}
	and.Add(fail)
	if and.Test(nil, nil, nil) {
		t.Error("And with one failing condition should fail")
	}

	var or Or
	if or.Test(nil, nil, nil) {
		t.Error("empty Or should fail")
	}
	or.Add(fail)
	or.Add(pass)
	if !or.Test(nil, nil, nil) {
		t.Error("Or with one passing condition should pass")
	}

	if (Not{Condition: pass}).Test(nil, nil, nil) {
		t.Error("Not{pass} should fail")
	}
	if !(Not{Condition: fail}).Test(nil, nil, nil) {
		t.Error("Not{fail} should pass")
	}

	var emptyAnd And
	if !emptyAnd.Test(nil, nil, nil) {
		t.Error("empty And should vacuously pass")
	}
}
