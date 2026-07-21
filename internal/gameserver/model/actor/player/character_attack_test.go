package player

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attack"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

func zeroRoll(int) int { return 0 }

func combatTemplate() *Template {
	return &Template{
		ID: 0, FistsItemID: 1,
		STR: 40, CON: 43, DEX: 30, INT: 21, WIT: 11, MEN: 25,
		PAtk: 5, PDef: 50,
		CollisionRadius: 9, CollisionHeight: 23,
		HPTable: []float64{100}, MPTable: []float64{30}, CPTable: []float64{0},
		Spawns: []location.Location{{X: 0, Y: 0, Z: 0}},
	}
}

func combatItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 5},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 300},
		}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{Type: item.WeaponSword, ReuseDelay: 1200, RandomDamage: 0}, Modifiers: []item.StatModifier{
			{Op: item.FuncSet, Stat: "pAtk", Value: 100},
			{Op: item.FuncSet, Stat: "pAtkSpd", Value: 433},
			{Op: item.FuncSet, Stat: "rCrit", Value: 4},
		}},
		{ID: 3, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: item.CrystalD, Weapon: &item.WeaponDetail{
			Type: item.WeaponSword, SoulshotCount: 2, SpiritshotCount: 1,
		}},
	})
}

func liveCharacter(id int32, tmpl *Template, items *item.Table, equipped ...*item.Instance) *Character {
	c := &Character{
		ID: id, Name: "char", ClassID: tmpl.ID, BaseClassID: tmpl.ID,
		Race: RaceHuman, Sex: SexMale, CharLevel: 1,
		Location: location.Location{X: int(id) * 100, Y: 0, Z: 0},
	}
	c.SetResourceValues(Resources{MaxHP: 100, CurrentHP: 100, MaxMP: 30, CurrentMP: 30})
	c.AttachRuntime(tmpl, itemcontainer.RestorePlayerInventory(c.ID, items, equipped))
	c.SetRollSource(zeroRoll)
	return c
}

func TestCharacterAttackUsesEquippedRightHandWeapon(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items, &item.Instance{
		ObjectID: 10, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
	})

	if got := c.AttackType(); got != item.WeaponSword {
		t.Fatalf("AttackType() = %v, want equipped sword", got)
	}
	if got := c.AttackSpeed(); got != 476 {
		t.Fatalf("AttackSpeed() = %d, want DEX-adjusted equipped weapon speed", got)
	}
	if got := c.WeaponReuseDelay(); got != 1200*time.Millisecond {
		t.Fatalf("WeaponReuseDelay() = %s, want 1200ms", got)
	}
	if got := c.WeaponGrade(); got != int(item.CrystalD) {
		t.Fatalf("WeaponGrade() = %d, want D grade", got)
	}
}

func TestCharacterAttackFallsBackToTemplateFists(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)

	if got := c.AttackType(); got != item.WeaponFist {
		t.Fatalf("AttackType() = %v, want template fists", got)
	}
	if got := c.AttackSpeed(); got != 330 {
		t.Fatalf("AttackSpeed() = %d, want DEX-adjusted fists speed", got)
	}
}

func TestCharacterPhysicalAttackResolvesLethalHit(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	attacker := liveCharacter(1, tmpl, items, &item.Instance{
		ObjectID: 10, TemplateID: 2, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
	})
	defender := liveCharacter(2, tmpl, items)
	defender.SetHP(100)

	state := world.New()
	state.Spawn(attacker, 0, 0, 0, 0)
	state.Spawn(defender, 100, 0, 0, 0)

	if !attack.NewPlayer(attacker).CanAttack(defender) {
		t.Fatal("CanAttack() = false for known live player target")
	}

	hit := attacker.MakeAttackHit(defender, false)
	if hit.Miss || hit.Damage <= 0 {
		t.Fatalf("MakeAttackHit() = %+v, want damaging hit", hit)
	}
	defender.TakeDamage(hit.Damage, attacker)
	if !defender.Dead() {
		t.Fatal("defender.Dead() = false after lethal player attack")
	}
	if got := defender.HP(); got != 0 {
		t.Fatalf("defender HP = %v, want 0 after lethal attack", got)
	}
}

// fakeLineOfSight is a LineOfSight double that records the query it
// received and returns a fixed result.
type fakeLineOfSight struct {
	result bool
	got    struct {
		ox, oy, oz       int
		oCollisionHeight float64
		tx, ty, tz       int
		tCollisionHeight float64
	}
}

func (f *fakeLineOfSight) CanSeeActor(ox, oy, oz int, oCollisionHeight float64, tx, ty, tz int, tCollisionHeight float64) bool {
	f.got.ox, f.got.oy, f.got.oz, f.got.oCollisionHeight = ox, oy, oz, oCollisionHeight
	f.got.tx, f.got.ty, f.got.tz, f.got.tCollisionHeight = tx, ty, tz, tCollisionHeight
	return f.result
}

func TestCharacterCanSeeDefaultsToVisibleWithoutLineOfSight(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items)

	if !c.CanSee(target) {
		t.Fatal("CanSee() = false with no line-of-sight query attached, want true")
	}
}

func TestCharacterCanSeeQueriesLineOfSightWithActorHeights(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items)

	los := &fakeLineOfSight{result: false}
	c.SetLineOfSight(los)

	if got := c.CanSee(target); got != false {
		t.Fatalf("CanSee() = %v, want false (from line-of-sight query result)", got)
	}

	ox, oy, oz := c.Position()
	tx, ty, tz := target.Position()
	if los.got.ox != ox || los.got.oy != oy || los.got.oz != oz {
		t.Fatalf("CanSeeActor() origin = (%d,%d,%d), want (%d,%d,%d)", los.got.ox, los.got.oy, los.got.oz, ox, oy, oz)
	}
	if los.got.tx != tx || los.got.ty != ty || los.got.tz != tz {
		t.Fatalf("CanSeeActor() target = (%d,%d,%d), want (%d,%d,%d)", los.got.tx, los.got.ty, los.got.tz, tx, ty, tz)
	}
	if los.got.oCollisionHeight != c.CollisionHeight() {
		t.Fatalf("CanSeeActor() origin collision height = %v, want %v", los.got.oCollisionHeight, c.CollisionHeight())
	}
	if los.got.tCollisionHeight != target.CollisionHeight() {
		t.Fatalf("CanSeeActor() target collision height = %v, want %v", los.got.tCollisionHeight, target.CollisionHeight())
	}

	los.result = true
	if got := c.CanSee(target); got != true {
		t.Fatalf("CanSee() = %v, want true (from line-of-sight query result)", got)
	}
}

func TestCharacterDieBroadcastsDieOnceOnly(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)
	c.SetHP(1)

	var calls int
	c.SetDieBroadcaster(func() { calls++ })

	if !c.Die(nil) {
		t.Fatal("Die() = false on a live character, want true")
	}
	if calls != 1 {
		t.Fatalf("die broadcast calls = %d, want 1", calls)
	}

	// A repeated kill is a no-op per Die's once-only contract: no second
	// Die packet.
	if c.Die(nil) {
		t.Fatal("Die() = true on an already-dead character, want false")
	}
	if calls != 1 {
		t.Fatalf("die broadcast calls after repeat kill = %d, want still 1", calls)
	}
}

func TestCharacterRevive(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)
	c.SetHP(1)
	c.Die(nil)
	if !c.Dead() {
		t.Fatal("precondition: character should be dead after Die()")
	}

	maxHP := c.ResourceValues().MaxHP
	if !c.Revive(0.5) {
		t.Fatal("Revive() = false on a dead character, want true")
	}
	if c.Dead() {
		t.Fatal("Dead() = true after Revive, want false")
	}
	if got, want := c.CurrentHP(), int(maxHP*0.5); got != want {
		t.Fatalf("CurrentHP() after Revive(0.5) = %d, want %d (half of calculated max HP %v)", got, want, maxHP)
	}

	if c.Revive(0.5) {
		t.Fatal("Revive() = true on an already-alive character, want false")
	}
}

// TestCharacterTakeDamageBroadcastsStatusOnEveryHit is the regression test
// for a target's HP bar never visibly dropping: TakeDamage applied damage
// and ran the death check, but never told any client the target's HP had
// changed until the corpse appeared. A non-lethal hit must broadcast once;
// a hit against an already-dead character (a stray late multi-hit landing
// after death) must not broadcast at all, since no damage was applied.
func TestCharacterTakeDamageBroadcastsStatusOnEveryHit(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	defender := liveCharacter(2, tmpl, items)
	defender.SetHP(100)

	var broadcasts int
	defender.SetStatusBroadcaster(func() { broadcasts++ })

	if defender.TakeDamage(30, nil) {
		t.Fatal("TakeDamage(30) on a 100 HP defender reported a kill")
	}
	if got := defender.HP(); got != 70 {
		t.Fatalf("defender HP = %v, want 70 after non-lethal hit", got)
	}
	if broadcasts != 1 {
		t.Fatalf("broadcasts after non-lethal hit = %d, want 1", broadcasts)
	}

	if !defender.TakeDamage(70, nil) {
		t.Fatal("TakeDamage(70) on a 70 HP defender did not report a kill")
	}
	if broadcasts != 2 {
		t.Fatalf("broadcasts after lethal hit = %d, want 2", broadcasts)
	}

	if defender.TakeDamage(10, nil) {
		t.Fatal("TakeDamage against an already-dead defender reported a kill")
	}
	if broadcasts != 2 {
		t.Fatalf("broadcasts after a stray hit on a dead defender = %d, want unchanged at 2", broadcasts)
	}
}

// TestCharacterPositionAccessIsRaceFree exercises the exact goroutine
// pairing that produces a live game's data race on Location/LastHeading: a
// position-update ticker calling SyncPosition during an attack chase,
// concurrently with the owning connection's network goroutine calling
// SetLastKnownPosition for a client-reported move, while a third goroutine
// reads the last-known state the way a save or a range check would. Run
// with -race.
func TestCharacterPositionAccessIsRaceFree(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items)

	const iterations = 500
	var wg sync.WaitGroup
	wg.Add(3)

	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SyncPosition(location.Location{X: i, Y: 0, Z: 0})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			c.SetLastKnownPosition(location.Location{X: -i, Y: 0, Z: 0}, i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = c.CurrentLocation()
			_ = c.CurrentHeading()
		}
	}()

	wg.Wait()
}

func shotWeaponCharacter(t *testing.T) *Character {
	t.Helper()
	tmpl := combatTemplate()
	items := combatItems()
	return liveCharacter(1, tmpl, items, &item.Instance{
		ObjectID: 10, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.RHand,
	})
}

func TestCharacterChargeSoulshot(t *testing.T) {
	c := shotWeaponCharacter(t)

	consume, result := c.ChargeSoulshot(item.CrystalD, 0)
	if result != ChargeShotOK {
		t.Fatalf("result = %v, want ChargeShotOK", result)
	}
	if consume != 2 {
		t.Fatalf("consume = %d, want 2 (weapon SoulshotCount)", consume)
	}
	if !c.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = false after a successful charge")
	}

	// Already charged: reported distinctly from the other rejections so
	// the caller can stay silent for this one, matching the reference.
	if _, result := c.ChargeSoulshot(item.CrystalD, 0); result != ChargeShotAlreadyCharged {
		t.Fatalf("result = %v, want ChargeShotAlreadyCharged", result)
	}
}

func TestCharacterChargeSoulshotRejectsGradeMismatchBeforeCharged(t *testing.T) {
	c := shotWeaponCharacter(t)

	if _, result := c.ChargeSoulshot(item.CrystalC, 0); result != ChargeShotGradeMismatch {
		t.Fatalf("result = %v, want ChargeShotGradeMismatch", result)
	}
	if c.SoulshotCharged() {
		t.Fatal("SoulshotCharged() = true after a rejected charge")
	}
}

func TestCharacterChargeSoulshotRejectsNoWeaponCapacity(t *testing.T) {
	tmpl := combatTemplate()
	items := combatItems()
	c := liveCharacter(1, tmpl, items) // fists: no soulshot capacity

	if _, result := c.ChargeSoulshot(item.CrystalD, 0); result != ChargeShotNoCapacity {
		t.Fatalf("result = %v, want ChargeShotNoCapacity", result)
	}
}

func TestCharacterChargeSpiritshot(t *testing.T) {
	c := shotWeaponCharacter(t)

	consume, result := c.ChargeSpiritshot(item.ShotSpirit, item.CrystalD)
	if result != ChargeShotOK {
		t.Fatalf("result = %v, want ChargeShotOK", result)
	}
	if consume != 1 {
		t.Fatalf("consume = %d, want 1 (weapon SpiritshotCount)", consume)
	}
	if !c.SpiritshotCharged() {
		t.Fatal("SpiritshotCharged() = false after a successful charge")
	}

	if _, result := c.ChargeSpiritshot(item.ShotSpirit, item.CrystalD); result != ChargeShotAlreadyCharged {
		t.Fatalf("result = %v, want ChargeShotAlreadyCharged", result)
	}
}

func TestCharacterChargeSpiritshotChecksAlreadyChargedBeforeGrade(t *testing.T) {
	c := shotWeaponCharacter(t)
	if _, result := c.ChargeSpiritshot(item.ShotSpirit, item.CrystalD); result != ChargeShotOK {
		t.Fatal("setup charge failed")
	}

	// Reference order for this shot kind checks already-charged before
	// grade — a mismatched-grade attempt on an already-charged weapon
	// still reports ChargeShotAlreadyCharged, not ChargeShotGradeMismatch.
	if _, result := c.ChargeSpiritshot(item.ShotSpirit, item.CrystalC); result != ChargeShotAlreadyCharged {
		t.Fatalf("result = %v, want ChargeShotAlreadyCharged (checked before grade)", result)
	}
}

func TestCharacterChargeSpiritshotAndBlessedAreIndependentCharges(t *testing.T) {
	c := shotWeaponCharacter(t)

	if _, result := c.ChargeSpiritshot(item.ShotSpirit, item.CrystalD); result != ChargeShotOK {
		t.Fatal("spirit charge setup failed")
	}
	if _, result := c.ChargeSpiritshot(item.ShotBlessedSpirit, item.CrystalD); result != ChargeShotOK {
		t.Fatalf("result = %v, want ChargeShotOK (blessed is a distinct charge slot)", result)
	}
	if !c.BlessedSpiritshotCharged() {
		t.Fatal("BlessedSpiritshotCharged() = false after a successful charge")
	}
}
