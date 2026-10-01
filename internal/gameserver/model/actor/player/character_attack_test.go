package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// ---- from character_target_test.go ----
func targetCharacter(id int32) *Character {
	return &Character{ID: id, Name: "target", CharLevel: 1}
}

func TestCharacterTargetRoundTrips(t *testing.T) {
	c := targetCharacter(1)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil before any selection", got)
	}

	other := targetCharacter(2)
	c.StoreTarget(other)
	if got := c.Target(); got != world.Tracked(other) {
		t.Fatalf("Target() = %v, want %v", got, other)
	}

	c.StoreTarget(nil)
	if got := c.Target(); got != nil {
		t.Fatalf("Target() = %v, want nil after clearing", got)
	}
}

// TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster
// exercises the retargetableOnAggression contract the AGGDEBUFF continuous
// handler consults: a playable not currently targeting the caster gets
// retargeted onto them via SetTarget, not attacked.
func TestCharacterRetargetableOnAggressionRetargetsWhenNotAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	other := targetCharacter(3)
	target := targetCharacter(2)
	target.StoreTarget(other)

	if got := target.CurrentTarget(); got != world.Tracked(other) {
		t.Fatalf("CurrentTarget() = %v, want %v", got, other)
	}
	target.SetTarget(caster)
	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() after SetTarget with no sink = %v, want caster", got)
	}

	target.StoreTarget(other)
	rec := recordEvents(target)
	target.SetTarget(caster)

	if got := event.Of[event.Retargeted](rec); len(got) != 1 || got[0].Target != event.Object(caster) {
		t.Fatalf("Retargeted events = %+v, want one onto caster", got)
	}
	if event.Count[event.AttackRequested](rec) != 0 {
		t.Fatal("a playable not already targeting the caster must be retargeted, not attacked")
	}
}

// TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster
// exercises the other branch: a playable already targeting the caster is
// provoked into attacking them through the AttackTarget hook instead of
// being retargeted.
func TestCharacterRetargetableOnAggressionAttacksWhenAlreadyTargetingCaster(t *testing.T) {
	caster := targetCharacter(1)
	target := targetCharacter(2)
	target.StoreTarget(caster)

	rec := recordEvents(target)

	target.AttackTarget(caster)

	if got := event.Of[event.AttackRequested](rec); len(got) != 1 || got[0].Target != event.Object(caster) {
		t.Fatalf("AttackRequested events = %+v, want one onto caster", got)
	}
	if got := target.Target(); got != world.Tracked(caster) {
		t.Fatalf("Target() = %v, want unchanged caster (attack, not retarget)", got)
	}
}

func TestCharacterTryToAttackDelegatesToAttackTarget(t *testing.T) {
	caster := targetCharacter(1)
	target := targetCharacter(2)
	rec := recordEvents(target)

	target.TryToAttack(caster)

	if got := event.Of[event.AttackRequested](rec); len(got) != 1 || got[0].Target != event.Object(caster) {
		t.Fatalf("AttackRequested events = %+v, want one onto caster", got)
	}
}

func TestCharacterMakeAttackHitAppliesFacingAndNight(t *testing.T) {
	place := func(t *testing.T, ax, ay int, night bool) (*Character, *Character) {
		t.Helper()
		tmpl := combatTemplate()
		items := combatItems()
		target := liveCharacter(1, tmpl, items)
		attacker := liveCharacter(2, tmpl, items)
		target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
		attacker.SetLastKnownPosition(location.Location{X: ax, Y: ay, Z: 0}, 0)
		live, err := creature.NewLive(location.Location{X: ax, Y: ay, Z: 0}, 0, permissiveGeo{}, attacker, effect.WithEnv(effect.Env{Night: hitNight(night)}))
		if err != nil {
			t.Fatal(err)
		}
		live.SetQueue(idleQueue())
		attacker.Live = live
		return attacker, target
	}

	attacker, target := place(t, 100, 0, false)
	acc := attacker.Accuracy()
	eva := target.Evasion()
	frontRate := formulas.HitRate(acc, eva, 0, false, false, true)
	behindRate := formulas.HitRate(acc, eva, 0, false, true, false)
	nightRate := formulas.HitRate(acc, eva, 0, true, false, true)
	if frontRate >= behindRate {
		t.Fatalf("need positional rate gap, front=%d behind=%d", frontRate, behindRate)
	}
	if nightRate >= frontRate {
		t.Fatalf("need night rate gap, night=%d front=%d", nightRate, frontRate)
	}
	posRoll := (frontRate + behindRate) / 2
	nightRoll := (nightRate + frontRate) / 2

	tests := []struct {
		name     string
		ax, ay   int
		night    bool
		roll     int
		wantMiss bool
	}{
		{"front day misses between front and behind rates", 100, 0, false, posRoll, true},
		{"behind day hits between front and behind rates", -100, 0, false, posRoll, false},
		{"side day hits between front and behind rates", 0, 100, false, posRoll, false},
		{"front night misses between night and front rates", 100, 0, true, nightRoll, true},
		{"front day hits the night-gap roll", 100, 0, false, nightRoll, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker, target := place(t, tt.ax, tt.ay, tt.night)
			attacker.SetRollSource(func(int) int { return tt.roll })
			hit := attacker.MakeAttackHit(target, false)
			if hit.Miss != tt.wantMiss {
				t.Fatalf("Miss = %v, want %v (roll %d acc %d eva %d)", hit.Miss, tt.wantMiss, tt.roll, acc, eva)
			}
		})
	}
}

func attackHitNoCrit() func(int) int {
	n := 0
	return func(int) int {
		n++
		if n == 1 {
			return 0
		}
		if n == 2 {
			return 999
		}
		return 0
	}
}

func TestCharacterMakeAttackHitUsesPosPvpWeaponCritShieldAndSoulshot(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	place := func(ax, ay int, equipped ...*item.Instance) (*Character, *Character) {
		target := liveCharacter(1, tmpl, items)
		attacker := liveCharacter(2, tmpl, items, equipped...)
		target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
		attacker.SetLastKnownPosition(location.Location{X: ax, Y: ay, Z: 0}, 0)
		attacker.SetRollSource(attackHitNoCrit())
		return attacker, target
	}

	frontAtk, frontTgt := place(100, 0)
	front := frontAtk.MakeAttackHit(frontTgt, false)
	if front.Miss || front.Crit {
		t.Fatalf("front hit miss=%v crit=%v, want hit non-crit", front.Miss, front.Crit)
	}
	wantFront := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: frontAtk.PAtk(), Defence: frontTgt.PDef(),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if front.Damage != wantFront {
		t.Fatalf("front damage = %d, want %d", front.Damage, wantFront)
	}

	behindAtk, behindTgt := place(-100, 0)
	behind := behindAtk.MakeAttackHit(behindTgt, false)
	wantBehind := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: behindAtk.PAtk(), Defence: behindTgt.PDef(),
		PosMul: 1.2, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if behind.Damage != wantBehind {
		t.Fatalf("behind damage = %d, want %d", behind.Damage, wantBehind)
	}

	pvpAtk, pvpTgt := place(100, 0)
	pvpAtk.AddStatFuncs([]effect.Mod{{Stat: stat.PvPPhysicalDmg, Op: effect.OpMul, Value: 2, Owner: testModOwner()}})
	pvp := pvpAtk.MakeAttackHit(pvpTgt, false)
	wantPvP := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: pvpAtk.PAtk(), Defence: pvpTgt.PDef(),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 2,
	}))
	if pvp.Damage != wantPvP {
		t.Fatalf("pvp damage = %d, want %d", pvp.Damage, wantPvP)
	}

	sword := &item.Instance{ObjectID: 21, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand}
	wpnAtk, wpnTgt := place(100, 0, sword)
	wpnTgt.AddStatFuncs([]effect.Mod{{Stat: stat.SwordWpnVuln, Op: effect.OpMul, Value: 1.5, Owner: testModOwner()}})
	wpn := wpnAtk.MakeAttackHit(wpnTgt, false)
	wantWpn := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: wpnAtk.PAtk(), Defence: wpnTgt.PDef(),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1.5, PvPMul: 1,
	}))
	if wpn.Damage != wantWpn {
		t.Fatalf("weapon-vuln damage = %d, want %d", wpn.Damage, wantWpn)
	}

	critAtk, critTgt := place(100, 0)
	critAtk.SetRollSource(zeroRoll)
	critAtk.AddStatFuncs([]effect.Mod{{Stat: stat.CriticalDamage, Op: effect.OpMul, Value: 2, Owner: testModOwner()}})
	critTgt.AddStatFuncs([]effect.Mod{{Stat: stat.CritVuln, Op: effect.OpMul, Value: 1.5, Owner: testModOwner()}})
	crit := critAtk.MakeAttackHit(critTgt, false)
	if !crit.Crit {
		t.Fatal("crit hit Crit = false")
	}
	wantCrit := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: critAtk.PAtk(), Defence: critTgt.PDef(), Crit: true,
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
		CritDamageMul: 2, CritDamagePosMul: 1, CritVulnMul: 1.5,
	}))
	if crit.Damage != wantCrit {
		t.Fatalf("crit damage = %d, want %d", crit.Damage, wantCrit)
	}

	ssSword := &item.Instance{ObjectID: 22, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand}
	ssAtk, ssTgt := place(100, 0, ssSword)
	ssAtk.SetChargedShot(item.ShotSoul, true)
	ss := ssAtk.MakeAttackHit(ssTgt, false)
	wantSS := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: ssAtk.PAtk(), Defence: ssTgt.PDef(), SoulShot: true,
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if ss.Damage != wantSS {
		t.Fatalf("soulshot damage = %d, want %d", ss.Damage, wantSS)
	}

	shieldItems := shieldDefenseItems()
	shieldTgt := liveCharacter(1, tmpl, shieldItems, equippedShield())
	shieldAtk := liveCharacter(2, tmpl, shieldItems)
	shieldTgt.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	shieldAtk.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	shieldTgt.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 80, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
		{Stat: stat.ShieldDefence, Op: effect.OpSet, Value: 40, Owner: testModOwner()},
	})
	n := 0
	shieldAtk.SetRollSource(func(int) int {
		n++
		if n == 1 {
			return 0
		}
		if n == 2 {
			return 999
		}
		return 0
	})
	shieldTgt.SetRollSource(func(bound int) int {
		if bound == 100 {
			return 10
		}
		return 0
	})
	blocked := shieldAtk.MakeAttackHit(shieldTgt, false)
	if blocked.Shield != formulas.ShieldSuccess {
		t.Fatalf("shield = %v, want ShieldSuccess", blocked.Shield)
	}
	wantBlocked := int(formulas.PhysicalAttackDamage(formulas.PhysicalAttackInput{
		AttackPower: shieldAtk.PAtk(), Defence: shieldTgt.PDef() + shieldTgt.CalcStat(stat.ShieldDefence, 0),
		PosMul: 1, ElementalMul: 1, RandomMul: 1, RaceMul: 1, WeaponVulnMul: 1, PvPMul: 1,
	}))
	if blocked.Damage != wantBlocked {
		t.Fatalf("blocked damage = %d, want %d", blocked.Damage, wantBlocked)
	}

	perfectTgt := liveCharacter(3, tmpl, shieldItems, equippedShield())
	perfectAtk := liveCharacter(4, tmpl, shieldItems)
	perfectTgt.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	perfectAtk.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	perfectTgt.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 80, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
	})
	perfectAtk.SetRollSource(attackHitNoCrit())
	perfectTgt.SetRollSource(func(int) int { return 0 })
	perfect := perfectAtk.MakeAttackHit(perfectTgt, false)
	if perfect.Shield != formulas.ShieldPerfect {
		t.Fatalf("shield = %v, want ShieldPerfect", perfect.Shield)
	}
	if perfect.Damage != 1 {
		t.Fatalf("perfect-block damage = %d, want 1", perfect.Damage)
	}
}

type hitNight bool

func (n hitNight) IsNight() bool { return bool(n) }

func TestCharacterCombatantReadsSilentMoveAndFakeDeathEffects(t *testing.T) {
	c := &Character{ID: 1}
	attachTestLive(t, c)
	var combatant attackable.Combatant = c
	if combatant.SilentMoving() || combatant.FakeDeath() {
		t.Fatal("SilentMoving/FakeDeath = true before any effect, want false")
	}

	for _, name := range []string{"SilentMove", "FakeDeath"} {
		e, err := effect.New(effect.Skill{ID: 1}, modelskill.EffectTemplate{Name: name})
		if err != nil {
			t.Fatalf("effect.New(%q) error: %v", name, err)
		}
		e.Effector, e.Effected = c, c
		c.EffectList().Add(e)
	}
	if !combatant.SilentMoving() {
		t.Fatal("Combatant.SilentMoving() = false with an active SilentMove effect, want true")
	}
	if !combatant.FakeDeath() {
		t.Fatal("Combatant.FakeDeath() = false with an active FakeDeath effect, want true")
	}
}

// Facing checks read a target's Heading through Combatant, so an unspawned
// player must still report its last-known heading there.
func TestCharacterCombatantHeadingIsCurrentHeading(t *testing.T) {
	c := &Character{ID: 1}
	c.SetLastKnownPosition(location.Location{X: 1, Y: 2, Z: 3}, 16384)
	var combatant attackable.Combatant = c
	if got, want := combatant.Heading(), c.CurrentHeading(); got != want || got != 16384 {
		t.Fatalf("Combatant.Heading() = %d, CurrentHeading() = %d, want both 16384", got, want)
	}
}
