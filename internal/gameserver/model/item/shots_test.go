package item

import "testing"

func TestInstance_ChargedShot(t *testing.T) {
	inst := &Instance{}

	if inst.ChargedShot(ShotSoul) {
		t.Fatalf("new instance should not be charged")
	}

	inst.SetChargedShot(ShotSoul, true)
	if !inst.ChargedShot(ShotSoul) {
		t.Errorf("ShotSoul should be charged after SetChargedShot(true)")
	}
	if inst.ChargedShot(ShotSpirit) {
		t.Errorf("charging ShotSoul must not also charge ShotSpirit")
	}

	inst.SetChargedShot(ShotSpirit, true)
	inst.SetChargedShot(ShotSoul, false)
	if inst.ChargedShot(ShotSoul) {
		t.Errorf("ShotSoul should be discharged")
	}
	if !inst.ChargedShot(ShotSpirit) {
		t.Errorf("discharging ShotSoul must not discharge ShotSpirit")
	}

	inst.UnchargeAllShots()
	if inst.ChargedShot(ShotSpirit) {
		t.Errorf("UnchargeAllShots should clear every shot")
	}
}

func TestWeaponDetail_EvaluateSoulshot(t *testing.T) {
	weapon := &WeaponDetail{SoulshotCount: 2, ReducedSoulshotChance: 50, ReducedSoulshotCount: 1}

	tests := []struct {
		name           string
		detail         *WeaponDetail
		weaponCrystal  CrystalType
		shotCrystal    CrystalType
		alreadyCharged bool
		roll           int
		wantConsume    int32
		wantOK         bool
	}{
		{"grade match, no reduced roll", weapon, CrystalD, CrystalD, false, 99, 2, true},
		{"grade match, reduced roll hits", weapon, CrystalD, CrystalD, false, 10, 1, true},
		{"grade mismatch", weapon, CrystalD, CrystalC, false, 99, 0, false},
		{"already charged", weapon, CrystalD, CrystalD, true, 99, 0, false},
		{"no soulshot capacity", &WeaponDetail{}, CrystalNone, CrystalNone, false, 99, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			consume, ok := tt.detail.EvaluateSoulshot(tt.weaponCrystal, tt.shotCrystal, tt.alreadyCharged, tt.roll)
			if consume != tt.wantConsume || ok != tt.wantOK {
				t.Errorf("EvaluateSoulshot() = (%d, %v), want (%d, %v)", consume, ok, tt.wantConsume, tt.wantOK)
			}
		})
	}
}

func TestWeaponDetail_EvaluateSpiritshot(t *testing.T) {
	weapon := &WeaponDetail{SpiritshotCount: 1}

	if consume, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalS, false); !ok || consume != 1 {
		t.Errorf("EvaluateSpiritshot() = (%d, %v), want (1, true)", consume, ok)
	}
	if _, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalA, false); ok {
		t.Errorf("EvaluateSpiritshot() with mismatched grade should not be ok")
	}
	if _, ok := weapon.EvaluateSpiritshot(CrystalS, CrystalS, true); ok {
		t.Errorf("EvaluateSpiritshot() while already charged should not be ok")
	}
	if _, ok := (&WeaponDetail{}).EvaluateSpiritshot(CrystalNone, CrystalNone, false); ok {
		t.Errorf("EvaluateSpiritshot() with zero capacity should not be ok")
	}
}
