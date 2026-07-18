package player

import (
	"reflect"
	"testing"
)

func TestCharacterResourcesAreNotExportedFields(t *testing.T) {
	typ := reflect.TypeOf(Character{})
	for _, name := range []string{"MaxHP", "CurHP", "MaxMP", "CurMP", "MaxCP", "CurCP"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("Character exports mutable resource field %s", name)
		}
	}
}

func TestCharacterVitals(t *testing.T) {
	ch := &Character{}
	ch.SetResourceValues(Resources{CurrentHP: 12.9, CurrentMP: 7.1})

	got := ch.Vitals()
	want := Vitals{HP: 12, MP: 7}
	if got != want {
		t.Fatalf("Vitals() = %+v, want %+v", got, want)
	}
}

func TestVitalsChangesTo(t *testing.T) {
	before := Vitals{HP: 100, MP: 50}

	got := before.ChangesTo(Vitals{HP: 75, MP: 50})
	want := VitalsChange{HP: 75, HPChanged: true, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo() = %+v, want %+v", got, want)
	}
	if !got.Changed() {
		t.Fatal("Changed() = false, want true")
	}

	got = before.ChangesTo(before)
	want = VitalsChange{HP: 100, MP: 50}
	if got != want {
		t.Fatalf("ChangesTo(unchanged) = %+v, want %+v", got, want)
	}
	if got.Changed() {
		t.Fatal("Changed() unchanged = true, want false")
	}
}
