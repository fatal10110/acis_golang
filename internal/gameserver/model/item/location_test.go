package item

import "testing"

func TestLocationStringRoundTrip(t *testing.T) {
	locations := []Location{
		LocationVoid, LocationInventory, LocationPaperdoll, LocationWarehouse,
		LocationClanWarehouse, LocationPet, LocationPetEquip, LocationFreight,
	}
	for _, l := range locations {
		s := l.String()
		got, err := ParseLocation(s)
		if err != nil {
			t.Errorf("ParseLocation(%q) unexpected error: %v", s, err)
			continue
		}
		if got != l {
			t.Errorf("ParseLocation(%q) = %v, want %v", s, got, l)
		}
	}
}

func TestParseLocation_Unknown(t *testing.T) {
	if _, err := ParseLocation("NOT_A_LOCATION"); err == nil {
		t.Fatal("ParseLocation() with unknown value: want error, got nil")
	}
}

func TestParseLocation_ExactSpelling(t *testing.T) {
	tests := map[string]Location{
		"VOID":      LocationVoid,
		"INVENTORY": LocationInventory,
		"PAPERDOLL": LocationPaperdoll,
		"WAREHOUSE": LocationWarehouse,
		"CLANWH":    LocationClanWarehouse,
		"PET":       LocationPet,
		"PET_EQUIP": LocationPetEquip,
		"FREIGHT":   LocationFreight,
	}
	for s, want := range tests {
		got, err := ParseLocation(s)
		if err != nil {
			t.Errorf("ParseLocation(%q) unexpected error: %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("ParseLocation(%q) = %v, want %v", s, got, want)
		}
	}
}
