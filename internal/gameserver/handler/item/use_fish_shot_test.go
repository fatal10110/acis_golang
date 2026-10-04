package item

import (
	"testing"

	modelitem "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
)

func fishingRod(crystal modelitem.CrystalType) *modelitem.Template {
	return &modelitem.Template{
		ID: shotWeaponID, Kind: modelitem.KindWeapon, Slot: modelitem.SlotLRHand, Crystal: crystal,
		Weapon: &modelitem.WeaponDetail{Type: modelitem.WeaponFishingRod},
	}
}

// TestUseFishShot pins FishShots.useItem: a rod must be in hand, not
// already charged and of the shot's grade; one shot is used up and the rod
// carries the fishing shot charge alone.
func TestUseFishShot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		weapon    *modelitem.Template
		count     int
		charged   bool
		want      FishShotOutcome
		skillID   int32
		left      int
		fishShot  bool
		shotGrade modelitem.CrystalType
	}{
		{name: "applied", weapon: fishingRod(modelitem.CrystalD), count: 3, want: FishShotApplied, skillID: 2182, left: 2, fishShot: true, shotGrade: modelitem.CrystalD},
		{name: "no weapon", count: 3, want: FishShotNoRod, left: 3, shotGrade: modelitem.CrystalD},
		{name: "sword", weapon: shotWeapon(modelitem.CrystalD), count: 3, want: FishShotNoRod, left: 3, shotGrade: modelitem.CrystalD},
		{name: "already charged", weapon: fishingRod(modelitem.CrystalD), count: 3, charged: true, want: FishShotAlreadyCharged, left: 3, fishShot: true, shotGrade: modelitem.CrystalD},
		{name: "grade mismatch", weapon: fishingRod(modelitem.CrystalD), count: 3, want: FishShotGradeMismatch, left: 3, shotGrade: modelitem.CrystalC},
		{name: "already charged ahead of grade", weapon: fishingRod(modelitem.CrystalD), count: 3, charged: true, want: FishShotAlreadyCharged, left: 3, fishShot: true, shotGrade: modelitem.CrystalC},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tmpl := shotTemplate(FishShotsHandler, tc.shotGrade, 2182)
			caster, inv, inst := newShooter(tmpl, tc.weapon, tc.count)
			if tc.charged {
				caster.SetChargedShot(modelitem.ShotFishSoul, true)
			}
			got, skillID := UseFishShot(FishShotUseRequest{Caster: caster, Inventory: inv, Item: inst, Template: tmpl, Destroyer: destroyer()})
			if got != tc.want || skillID != tc.skillID {
				t.Fatalf("UseFishShot = %v, %d; want %v, %d", got, skillID, tc.want, tc.skillID)
			}
			if left := stackCount(inv, 10); left != tc.left {
				t.Fatalf("shots left = %d, want %d", left, tc.left)
			}
			if caster.ChargedShot(modelitem.ShotFishSoul) != tc.fishShot || caster.ChargedShot(modelitem.ShotSoul) {
				t.Fatalf("fishing shot charged = %v, soulshot %v; want %v, false", caster.ChargedShot(modelitem.ShotFishSoul), caster.ChargedShot(modelitem.ShotSoul), tc.fishShot)
			}
		})
	}
}
