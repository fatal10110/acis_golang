package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// minimalSet returns a StatSet holding only the attributes NewDefinition
// requires unconditionally.
func minimalSet() *commons.StatSet {
	set := commons.NewStatSet()
	set.Set("target", "ONE")
	set.Set("skillType", "BUFF")
	set.Set("operateType", "ACTIVE")
	return set
}

func TestNewDefinitionDefaults(t *testing.T) {
	d, err := NewDefinition(7, 1, "Test Skill", minimalSet())
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if d.ID != 7 || d.Level != 1 || d.Name != "Test Skill" {
		t.Fatalf("NewDefinition() identity = %+v", d)
	}
	if d.Target != TargetOne || d.SkillType != "BUFF" || d.Activation != ActivationActive {
		t.Fatalf("NewDefinition() tags = %+v", d)
	}
	if d.EffectRange != -1 || d.AbnormalLevel != -1 || d.NegateLevel != -1 {
		t.Fatalf("NewDefinition() (-1)-defaulted fields = %+v", d)
	}
	if d.Radius != 80 {
		t.Fatalf("NewDefinition() Radius = %d, want 80", d.Radius)
	}
	if d.Element != ElementNone {
		t.Fatalf("NewDefinition() Element = %v, want ElementNone", d.Element)
	}
	if d.CanBeReflected != true || d.CanBeDispelled != true {
		t.Fatalf("NewDefinition() reflect/dispel defaults = %+v", d)
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
	set := minimalSet()
	set.Set("skillType", "PDAM")
	d, err := NewDefinition(1, 1, "x", set)
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if !d.Offensive {
		t.Fatal("PDAM: Offensive = false, want true")
	}
	if d.BaseCritRate != 0 {
		t.Fatalf("PDAM: BaseCritRate = %d, want 0", d.BaseCritRate)
	}
}

func TestNewDefinitionExplicitOverridesDefault(t *testing.T) {
	set := minimalSet()
	set.Set("skillType", "PDAM")
	set.Set("offensive", "false")
	set.Set("baseCritRate", "42")
	d, err := NewDefinition(1, 1, "x", set)
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if d.Offensive {
		t.Fatal("explicit offensive=false was overridden by the PDAM default")
	}
	if d.BaseCritRate != 42 {
		t.Fatalf("BaseCritRate = %d, want 42 (explicit)", d.BaseCritRate)
	}
}

func TestNewDefinitionOptionalReferences(t *testing.T) {
	set := minimalSet()
	set.Set("sharedReuse", "10-2")
	set.Set("negateId", "1,2,3")
	set.Set("negateStats", "STUN ROOT")
	set.Set("flyType", "CHARGE")

	d, err := NewDefinition(1, 1, "x", set)
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if d.SharedReuse == nil || d.SharedReuse.ID != 10 || d.SharedReuse.Level != 2 {
		t.Fatalf("SharedReuse = %+v, want {10 2}", d.SharedReuse)
	}
	if len(d.NegateIDs) != 3 || d.NegateIDs[0] != 1 || d.NegateIDs[2] != 3 {
		t.Fatalf("NegateIDs = %v, want [1 2 3]", d.NegateIDs)
	}
	if len(d.NegateTypes) != 2 || d.NegateTypes[0] != "STUN" || d.NegateTypes[1] != "ROOT" {
		t.Fatalf("NegateTypes = %v, want [STUN ROOT]", d.NegateTypes)
	}
	if d.Flight == nil || *d.Flight != FlightCharge {
		t.Fatalf("Flight = %v, want FlightCharge", d.Flight)
	}
}

func TestNewDefinitionHeroSkill(t *testing.T) {
	d, err := NewDefinition(395, 1, "Hero Skill", minimalSet())
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if !d.HeroSkill {
		t.Fatal("skill 395: HeroSkill = false, want true")
	}

	d2, err := NewDefinition(1, 1, "Not Hero", minimalSet())
	if err != nil {
		t.Fatalf("NewDefinition() error: %v", err)
	}
	if d2.HeroSkill {
		t.Fatal("skill 1: HeroSkill = true, want false")
	}
}

func TestNewDefinitionRequiredFields(t *testing.T) {
	cases := []struct {
		name string
		set  func() *commons.StatSet
	}{
		{"missing target", func() *commons.StatSet {
			s := commons.NewStatSet()
			s.Set("skillType", "BUFF")
			s.Set("operateType", "ACTIVE")
			return s
		}},
		{"missing skillType", func() *commons.StatSet {
			s := commons.NewStatSet()
			s.Set("target", "ONE")
			s.Set("operateType", "ACTIVE")
			return s
		}},
		{"missing operateType", func() *commons.StatSet {
			s := commons.NewStatSet()
			s.Set("target", "ONE")
			s.Set("skillType", "BUFF")
			return s
		}},
		{"unknown target tag", func() *commons.StatSet {
			s := minimalSet()
			s.Set("target", "NOT_REAL")
			return s
		}},
		{"malformed sharedReuse", func() *commons.StatSet {
			s := minimalSet()
			s.Set("sharedReuse", "not-a-pair-of-ints")
			return s
		}},
		{"malformed negateId", func() *commons.StatSet {
			s := minimalSet()
			s.Set("negateId", "1,oops")
			return s
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewDefinition(1, 1, "x", c.set()); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}
}
