package player

import "testing"

func TestCharacterVitals(t *testing.T) {
	ch := &Character{CurHP: 12.9, CurMP: 7.1}

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
