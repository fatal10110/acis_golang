package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/creature"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/formulas"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

type shieldDefenseResolver interface {
	ShieldDefense(caster creature.DeathActor, def modelskill.Definition, isCrit bool) formulas.ShieldDefense
}

func TestCharacterShieldDefenseUsesLiveShieldStatsFacingAndRoll(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	caster := liveCharacter(1, tmpl, items)
	target := liveCharacter(2, tmpl, items, equippedShield())
	caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
	target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]effect.Mod{
		{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
		{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
	})

	src, ok := any(target).(shieldDefenseResolver)
	if !ok {
		t.Fatal("Character must resolve live shield defense")
	}

	tests := []struct {
		name string
		roll int
		want formulas.ShieldDefense
	}{
		{name: "perfect block", roll: 0, want: formulas.ShieldPerfect},
		{name: "ordinary block", roll: 5, want: formulas.ShieldSuccess},
		{name: "failed block", roll: 99, want: formulas.ShieldFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target.SetRollSource(func(n int) int {
				if n != 100 {
					t.Fatalf("shield roll bound = %d, want 100", n)
				}
				return tt.roll
			})
			if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != tt.want {
				t.Fatalf("ShieldDefense() = %v, want %v", got, tt.want)
			}
		})
	}

	caster.SetLastKnownPosition(location.Location{X: -80, Y: 0, Z: 0}, 0)
	target.AddStatFuncs([]effect.Mod{{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 360, Owner: testModOwner()}})
	target.SetRollSource(func(int) int { return 0 })
	if got := src.ShieldDefense(caster, modelskill.Definition{SkillType: "STUN"}, false); got != formulas.ShieldPerfect {
		t.Fatalf("ShieldDefense() with 360-degree stat = %v, want ShieldPerfect", got)
	}
}

func TestCharacterShieldDefenseGatesEquipStatsAndFacing(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	def := modelskill.Definition{SkillType: "STUN"}

	tests := []struct {
		name      string
		equipped  []*item.Instance
		rate      float64
		angle     float64
		casterLoc location.Location
		def       modelskill.Definition
	}{
		{
			name:      "no shield equipped",
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "left hand is not armor",
			equipped:  []*item.Instance{equippedArrow()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "left hand armor is not a shield",
			equipped:  []*item.Instance{equippedLightArmor()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "zero shield rate",
			equipped:  []*item.Instance{equippedShield()},
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "outside shield angle",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: -80, Y: 0, Z: 0},
			def:       def,
		},
		{
			name:      "skill ignores shield",
			equipped:  []*item.Instance{equippedShield()},
			rate:      20,
			angle:     120,
			casterLoc: location.Location{X: 80, Y: 0, Z: 0},
			def:       modelskill.Definition{SkillType: "STUN", IgnoreShield: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items, tt.equipped...)
			caster.SetLastKnownPosition(tt.casterLoc, 0)
			target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
			target.SetRollSource(func(int) int { return 0 })
			target.AddStatFuncs([]effect.Mod{
				{Stat: stat.ShieldRate, Op: effect.OpSet, Value: tt.rate, Owner: testModOwner()},
				{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: tt.angle, Owner: testModOwner()},
			})

			src, ok := any(target).(shieldDefenseResolver)
			if !ok {
				t.Fatal("Character must resolve live shield defense")
			}
			if got := src.ShieldDefense(caster, tt.def, false); got != formulas.ShieldFailed {
				t.Fatalf("ShieldDefense() = %v, want ShieldFailed", got)
			}
		})
	}
}

func TestCharacterShieldDefenseNotifiesDefendingPlayerBySDefOnly(t *testing.T) {
	tmpl := combatTemplate()
	items := shieldDefenseItems()
	def := modelskill.Definition{SkillType: "STUN"}

	tests := []struct {
		name        string
		roll        int
		wantSuccess bool
		wantPerfect bool
	}{
		{name: "perfect block notifies excellent message", roll: 0, wantPerfect: true},
		{name: "ordinary block notifies success message", roll: 5, wantSuccess: true},
		{name: "failed block sends no message", roll: 99},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caster := liveCharacter(1, tmpl, items)
			target := liveCharacter(2, tmpl, items, equippedShield())
			caster.SetLastKnownPosition(location.Location{X: 80, Y: 0, Z: 0}, 0)
			target.SetLastKnownPosition(location.Location{X: 0, Y: 0, Z: 0}, 0)
			target.AddStatFuncs([]effect.Mod{
				{Stat: stat.ShieldRate, Op: effect.OpSet, Value: 20, Owner: testModOwner()},
				{Stat: stat.ShieldDefenceAngle, Op: effect.OpSet, Value: 120, Owner: testModOwner()},
			})
			target.SetRollSource(func(int) int { return tt.roll })

			var gotSuccess, gotPerfect bool
			target.SetShieldBlockNotifiers(func() { gotSuccess = true }, func() { gotPerfect = true })

			target.ShieldDefense(caster, def, false)

			if gotSuccess != tt.wantSuccess {
				t.Fatalf("shield block success notice fired = %v, want %v", gotSuccess, tt.wantSuccess)
			}
			if gotPerfect != tt.wantPerfect {
				t.Fatalf("shield block perfect notice fired = %v, want %v", gotPerfect, tt.wantPerfect)
			}
		})
	}
}

func shieldDefenseItems() *item.Table {
	return item.NewTable([]*item.Template{
		{ID: 1, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponFist}},
		{ID: 2, Kind: item.KindWeapon, Slot: item.SlotRHand, Weapon: &item.WeaponDetail{Type: item.WeaponSword}},
		{ID: 3, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorShield}},
		{ID: 4, Kind: item.KindEtcItem, Slot: item.SlotLHand, EtcItem: &item.EtcItemDetail{Type: item.EtcItemArrow}},
		{ID: 5, Kind: item.KindArmor, Slot: item.SlotLHand, Armor: &item.ArmorDetail{Type: item.ArmorLight}},
	})
}

func equippedShield() *item.Instance {
	return &item.Instance{ObjectID: 30, TemplateID: 3, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func equippedArrow() *item.Instance {
	return &item.Instance{ObjectID: 40, TemplateID: 4, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}

func equippedLightArmor() *item.Instance {
	return &item.Instance{ObjectID: 50, TemplateID: 5, Location: item.LocationPaperdoll, LocationData: itemcontainer.LHand}
}
