package skill

import (
	"testing"
)

func TestTriggerTypeString(t *testing.T) {
	for _, name := range triggerTypeStrings {
		trigger, ok := triggerTypeNames[name]
		if !ok {
			t.Fatalf("triggerTypeNames missing %q", name)
		}
		if got := trigger.String(); got != name {
			t.Fatalf("String() = %q, want %q", got, name)
		}
	}
}

func TestParseChanceConditionEmptyTypeIsNoCondition(t *testing.T) {
	cond, ok, err := ParseChanceCondition("", -1)
	if err != nil {
		t.Fatalf("ParseChanceCondition() error = %v, want nil", err)
	}
	if ok {
		t.Fatalf("ParseChanceCondition() ok = true, want false for an empty chanceType")
	}
	if cond != (ChanceCondition{}) {
		t.Fatalf("ParseChanceCondition() cond = %+v, want zero value", cond)
	}
}

func TestParseChanceConditionUnknownTypeIsAnError(t *testing.T) {
	if _, ok, err := ParseChanceCondition("BOGUS", 50); err == nil || ok {
		t.Fatalf("ParseChanceCondition(BOGUS) = ok %v err %v, want an error and ok=false", ok, err)
	}
}

func TestParseChanceConditionKnownType(t *testing.T) {
	cond, ok, err := ParseChanceCondition("ON_ATTACKED", 80)
	if err != nil || !ok {
		t.Fatalf("ParseChanceCondition() = %+v, %v, %v, want ok with no error", cond, ok, err)
	}
	if cond.Trigger != TriggerOnAttacked || cond.Chance != 80 {
		t.Fatalf("cond = %+v, want {TriggerOnAttacked 80}", cond)
	}
}

// TestChanceConditionFires reproduces the reference chance-condition roll:
// trigger must match, and a non-negative chance only fires below its own
// value out of a [0,100) roll, while a negative chance always fires once
// the trigger matches.
func TestChanceConditionFires(t *testing.T) {
	tests := []struct {
		name    string
		cond    ChanceCondition
		trigger TriggerType
		roll    int
		want    bool
	}{
		{"trigger mismatch never fires", ChanceCondition{Trigger: TriggerOnCrit, Chance: -1}, TriggerOnHit, 0, false},
		{"negative chance always fires on match", ChanceCondition{Trigger: TriggerOnHit, Chance: -1}, TriggerOnHit, 99, true},
		{"roll under chance fires", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 49, true},
		{"roll at chance does not fire", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 50, false},
		{"roll over chance does not fire", ChanceCondition{Trigger: TriggerOnHit, Chance: 50}, TriggerOnHit, 51, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.cond.Fires(tt.trigger, tt.roll); got != tt.want {
				t.Fatalf("Fires(%v, %d) = %v, want %v", tt.trigger, tt.roll, got, tt.want)
			}
		})
	}
}
