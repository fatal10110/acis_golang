package stat

import "testing"

// TestStatCount pins the table size to the reference enum's entry count so
// an accidentally dropped or duplicated entry fails loudly.
func TestStatCount(t *testing.T) {
	const wantCount = 122
	if int(numStats) != wantCount {
		t.Fatalf("numStats = %d, want %d", numStats, wantCount)
	}
}

// TestNameAndCantBeNegative checks a representative sample spanning every
// section of the table (hp/mp pools, atk/def, rates, base attributes,
// resistances, vulnerabilities, weapon vulnerabilities, race-specific
// atk/def, and limits) against the reference values.
func TestNameAndCantBeNegative(t *testing.T) {
	tests := []struct {
		s              Stat
		name           string
		cantBeNegative bool
	}{
		{MaxHP, "maxHp", true},
		{MaxMP, "maxMp", true},
		{MaxCP, "maxCp", true},
		{RegenerateHPRate, "regHp", false},
		{RechargeMPRate, "gainMp", false},
		{HealProficiency, "giveHp", false},
		{PowerDefence, "pDef", true},
		{MagicDefence, "mDef", true},
		{PowerAttack, "pAtk", true},
		{MagicAttack, "mAtk", true},
		{PowerAttackSpeed, "pAtkSpd", true},
		{MagicAttackSpeed, "mAtkSpd", true},
		{ShieldDefence, "sDef", true},
		{ShieldDefenceAngle, "shieldDefAngle", false},
		{ShieldRate, "rShld", false},
		{CriticalDamage, "cAtk", false},
		{CriticalDamagePos, "cAtkPos", false},
		{CriticalDamageAdd, "cAtkAdd", false},
		{PvPPhysicalDmg, "pvpPhysDmg", false},
		{PvPPhysSkillDmg, "pvpPhysSkillsDmg", false},
		{EvasionRate, "rEvas", false},
		{CriticalRate, "rCrit", false},
		{BlowRate, "blowRate", false},
		{LethalRate, "lethalRate", false},
		{MCriticalRate, "mCritRate", false},
		{AttackCancel, "cancel", false},
		{AccuracyCombat, "accCombat", false},
		{RunSpeed, "runSpd", false},
		{StatSTR, "STR", true},
		{StatCON, "CON", true},
		{StatDEX, "DEX", true},
		{StatINT, "INT", true},
		{StatWIT, "WIT", true},
		{StatMEN, "MEN", true},
		{Breath, "breath", false},
		{Fall, "fall", false},
		{FireRes, "fireRes", false},
		{ValakasRes, "valakasRes", false},
		{ValakasPower, "valakasPower", false},
		{BleedVuln, "bleedVuln", false},
		{CritVuln, "critVuln", false},
		{DebuffVuln, "debuffVuln", false},
		{SwordWpnVuln, "swordWpnVuln", false},
		{DaggerWpnVuln, "daggerWpnVuln", false},
		{BowWpnVuln, "bowWpnVuln", false},
		{BigBluntWpnVuln, "bigBluntWpnVuln", false},
		{ReflectDamagePercent, "reflectDam", false},
		{CounterSkillPhysical, "counterSkill", false},
		{PAtkPlants, "pAtk-plants", false},
		{PAtkMCreatures, "pAtk-magicCreature", false},
		{PDefDragons, "pDef-dragons", false},
		{WeightLimit, "weightLimit", false},
		{InvLim, "inventoryLimit", false},
		{FreightLim, "FreightLimit", false},
		{RecDLim, "DwarfRecipeLimit", false},
		{RecCLim, "CommonRecipeLimit", false},
		{PhysicalMpConsumeRate, "PhysicalMpConsumeRate", false},
		{DanceMpConsumeRate, "DanceMpConsumeRate", false},
		{SkillMastery, "skillMastery", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Name(); got != tt.name {
				t.Errorf("Name() = %q, want %q", got, tt.name)
			}
			if got := tt.s.CantBeNegative(); got != tt.cantBeNegative {
				t.Errorf("CantBeNegative() = %v, want %v", got, tt.cantBeNegative)
			}
		})
	}
}

// TestByName checks the reverse lookup roundtrips for every entry and
// rejects unknown spellings, matching valueOfXml's exact (not
// case-insensitive) match and its failure-on-unknown behavior.
func TestByName(t *testing.T) {
	for i := Stat(0); i < numStats; i++ {
		name := i.Name()
		got, err := ByName(name)
		if err != nil {
			t.Fatalf("ByName(%q) unexpected error: %v", name, err)
		}
		if got != i {
			t.Errorf("ByName(%q) = %v, want %v", name, got, i)
		}
	}

	if _, err := ByName("notAStat"); err == nil {
		t.Error("ByName(\"notAStat\") expected error, got nil")
	}

	// valueOfXml is exact-match; the lowercase spelling of a stat whose
	// canonical name is uppercase must not resolve.
	if _, err := ByName("str"); err == nil {
		t.Error(`ByName("str") expected error (canonical spelling is "STR"), got nil`)
	}
}

// TestNoDuplicateNames pins that every Stat has a distinct data-file
// spelling, since ByName's map would otherwise silently drop one.
func TestNoDuplicateNames(t *testing.T) {
	seen := make(map[string]Stat, numStats)
	for i := Stat(0); i < numStats; i++ {
		name := i.Name()
		if prev, ok := seen[name]; ok {
			t.Errorf("duplicate name %q for %v and %v", name, prev, i)
		}
		seen[name] = i
	}
}
