package skill

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons"
)

func TestActivationFromStatSet(t *testing.T) {
	set := commons.NewStatSet()
	set.Set("operateType", "TOGGLE")
	got, err := commons.GetEnum[Activation](set, "operateType", activationNames)
	if err != nil || got != ActivationToggle {
		t.Fatalf("Activation = %v, %v, want ActivationToggle", got, err)
	}
	if got.String() != "TOGGLE" {
		t.Fatalf("String() = %q, want TOGGLE", got.String())
	}

	set.Set("operateType", "BOGUS")
	if _, err := commons.GetEnum[Activation](set, "operateType", activationNames); err == nil {
		t.Fatal("expected an error for an unknown operateType tag, got nil")
	}
}

func TestTargetFromStatSet(t *testing.T) {
	for _, name := range targetStrings {
		set := commons.NewStatSet()
		set.Set("target", name)
		got, err := commons.GetEnum[Target](set, "target", targetNames)
		if err != nil {
			t.Fatalf("target %q: %v", name, err)
		}
		if got.String() != name {
			t.Fatalf("target %q round-trip = %q", name, got.String())
		}
	}
}

func TestElementDefault(t *testing.T) {
	set := commons.NewStatSet()
	got, err := commons.GetEnumDefault[Element](set, "element", elementNames, ElementNone)
	if err != nil || got != ElementNone {
		t.Fatalf("Element default = %v, %v, want ElementNone", got, err)
	}

	set.Set("element", "FIRE")
	got, err = commons.GetEnumDefault[Element](set, "element", elementNames, ElementNone)
	if err != nil || got != ElementFire {
		t.Fatalf("Element = %v, %v, want ElementFire", got, err)
	}
}

func TestUnknownEnumValueStringsFallBack(t *testing.T) {
	if got := Activation(99).String(); got != "Activation(99)" {
		t.Fatalf("Activation(99).String() = %q", got)
	}
	if got := Target(99).String(); got != "Target(99)" {
		t.Fatalf("Target(99).String() = %q", got)
	}
	if got := Element(99).String(); got != "Element(99)" {
		t.Fatalf("Element(99).String() = %q", got)
	}
	if got := Flight(99).String(); got != "Flight(99)" {
		t.Fatalf("Flight(99).String() = %q", got)
	}
}
