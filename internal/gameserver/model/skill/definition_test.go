package skill

import "testing"

// minimalAttrs returns the attributes a level carries when its data sets
// only the classification tags every skill has.
func minimalAttrs() DefinitionAttrs {
	return DefinitionAttrs{
		Activation:            ActivationActive,
		Target:                TargetOne,
		SkillType:             "BUFF",
		EffectRange:           -1,
		AbnormalLevel:         -1,
		EffectAbnormalLevel:   -1,
		NegateLevel:           -1,
		Radius:                80,
		EffectNpcID:           -1,
		Element:               ElementNone,
		CubicActivationTime:   8,
		CubicActivationChance: 30,
		SummonTotalLifeTime:   1200000,
		ActivationChance:      -1,
		CanBeReflected:        true,
		CanBeDispelled:        true,
	}
}

func TestNewDefinitionCarriesAttrs(t *testing.T) {
	d := NewDefinition(7, 1, "Test Skill", minimalAttrs())
	if d.ID != 7 || d.Level != 1 || d.Name != "Test Skill" {
		t.Fatalf("NewDefinition() identity = %+v", d)
	}
	if d.Target != TargetOne || d.SkillType != "BUFF" || d.Activation != ActivationActive {
		t.Fatalf("NewDefinition() tags = %+v", d)
	}
	if d.EffectRange != -1 || d.AbnormalLevel != -1 || d.NegateLevel != -1 {
		t.Fatalf("NewDefinition() (-1)-carrying fields = %+v", d)
	}
	if d.Radius != 80 {
		t.Fatalf("NewDefinition() Radius = %d, want 80", d.Radius)
	}
	if d.Element != ElementNone {
		t.Fatalf("NewDefinition() Element = %v, want ElementNone", d.Element)
	}
	if !d.CanBeReflected || !d.CanBeDispelled {
		t.Fatalf("NewDefinition() reflect/dispel = %+v", d)
	}
	// BUFF isn't a classified-offensive type, and target isn't CORPSE_MOB.
	if d.Offensive {
		t.Fatal("NewDefinition() Offensive = true, want false")
	}
	// Not PDAM/BLOW.
	if d.BaseCritRate != -1 {
		t.Fatalf("NewDefinition() BaseCritRate = %d, want -1", d.BaseCritRate)
	}
	if d.Flight != nil {
		t.Fatalf("NewDefinition() Flight = %v, want nil", d.Flight)
	}
	if d.SharedReuse != nil {
		t.Fatalf("NewDefinition() SharedReuse = %v, want nil", d.SharedReuse)
	}
}

func TestNewDefinitionOffensiveAndCritDefaults(t *testing.T) {
	a := minimalAttrs()
	a.SkillType = "PDAM"
	d := NewDefinition(1, 1, "x", a)
	if !d.Offensive {
		t.Fatal("PDAM: Offensive = false, want true")
	}
	if d.BaseCritRate != 0 {
		t.Fatalf("PDAM: BaseCritRate = %d, want 0", d.BaseCritRate)
	}

	// A debuff or a corpse-mob target is offensive whatever its skill type.
	a = minimalAttrs()
	a.Debuff = true
	if !NewDefinition(1, 1, "x", a).Offensive {
		t.Fatal("debuff: Offensive = false, want true")
	}
	a = minimalAttrs()
	a.Target = TargetCorpseMob
	if !NewDefinition(1, 1, "x", a).Offensive {
		t.Fatal("CORPSE_MOB target: Offensive = false, want true")
	}
}

func TestNewDefinitionExplicitOverridesDefault(t *testing.T) {
	a := minimalAttrs()
	a.SkillType = "PDAM"
	offensive, rate := false, 42
	a.Offensive, a.BaseCritRate = &offensive, &rate

	d := NewDefinition(1, 1, "x", a)
	if d.Offensive {
		t.Fatal("explicit offensive=false was overridden by the PDAM default")
	}
	if d.BaseCritRate != 42 {
		t.Fatalf("BaseCritRate = %d, want 42 (explicit)", d.BaseCritRate)
	}
}

func TestNewDefinitionHeroSkill(t *testing.T) {
	if !NewDefinition(395, 1, "Hero Skill", minimalAttrs()).HeroSkill {
		t.Fatal("skill 395: HeroSkill = false, want true")
	}
	if NewDefinition(1, 1, "Not Hero", minimalAttrs()).HeroSkill {
		t.Fatal("skill 1: HeroSkill = true, want false")
	}
}

func TestParseRef(t *testing.T) {
	ref, err := ParseRef("10-2")
	if err != nil {
		t.Fatalf("ParseRef(\"10-2\") error: %v", err)
	}
	if ref.ID != 10 || ref.Level != 2 {
		t.Fatalf("ParseRef(\"10-2\") = %+v, want {10 2}", ref)
	}
	for _, raw := range []string{"", "10", "not-a-pair-of-ints", "10-2-3", "x-2", "10-x", "100--1", "-1-2"} {
		if _, err := ParseRef(raw); err == nil {
			t.Fatalf("ParseRef(%q) = nil error, want an error", raw)
		}
	}
}

func TestParseEnums(t *testing.T) {
	if a, err := ParseActivation("TOGGLE"); err != nil || a != ActivationToggle {
		t.Fatalf("ParseActivation(\"TOGGLE\") = %v, %v", a, err)
	}
	if tgt, err := ParseTarget("CORPSE_MOB"); err != nil || tgt != TargetCorpseMob {
		t.Fatalf("ParseTarget(\"CORPSE_MOB\") = %v, %v", tgt, err)
	}
	if e, err := ParseElement("VALAKAS"); err != nil || e != ElementValakas {
		t.Fatalf("ParseElement(\"VALAKAS\") = %v, %v", e, err)
	}
	if f, err := ParseFlight("CHARGE"); err != nil || f != FlightCharge {
		t.Fatalf("ParseFlight(\"CHARGE\") = %v, %v", f, err)
	}
	if _, err := ParseTarget("NOT_A_TARGET"); err == nil {
		t.Fatal("ParseTarget(\"NOT_A_TARGET\") = nil error, want an error")
	}
}
