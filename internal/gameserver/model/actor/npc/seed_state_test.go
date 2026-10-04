package npc

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/manor"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// TestHarvestedCropStrongTypeMultiplier pins the strong-type crop base: the
// first template passive in 4303..4310 yields (id - 4301) crops per harvest,
// any other passive leaves the base at one.
func TestHarvestedCropStrongTypeMultiplier(t *testing.T) {
	const cropID = 5073
	seed := manor.Seed{CropID: cropID, SeedID: 5016, MatureID: 5103, Level: 20}
	cases := []struct {
		name     string
		passives []skill.Ref
		want     int
	}{
		{"no passive", nil, 1},
		{"non-strong passive", []skill.Ref{{ID: 4302, Level: 1}}, 1},
		{"above strong range", []skill.Ref{{ID: 4311, Level: 1}}, 1},
		{"first strong 4303", []skill.Ref{{ID: 4303, Level: 1}}, 2},
		{"strong 4305", []skill.Ref{{ID: 4305, Level: 1}}, 4},
		{"last strong 4310", []skill.Ref{{ID: 4310, Level: 1}}, 9},
		{"first strong passive wins", []skill.Ref{{ID: 4416, Level: 1}, {ID: 4306, Level: 1}, {ID: 4309, Level: 1}}, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Monster level equals the seed level, so the level bonus is zero
			// and the count is the strong-type base alone.
			h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 20, Passives: tc.passives})
			if !h.SeedState().Sow(100, seed) {
				t.Fatal("Sow() = false on a fresh life")
			}
			gotCrop, gotCount := h.SeedState().HarvestedCrop(1)
			if gotCrop != cropID || gotCount != tc.want {
				t.Fatalf("HarvestedCrop(1) = (%d, %d), want (%d, %d)", gotCrop, gotCount, cropID, tc.want)
			}
		})
	}
}

// TestHarvestedCropAddsLevelBonusAndRate adds one crop per level the owner
// stands more than five above the seed on top of the strong base, then
// multiplies by the manor drop rate.
func TestHarvestedCropAddsLevelBonusAndRate(t *testing.T) {
	seed := manor.Seed{CropID: 5073, Level: 10}
	h := newCombatHostile(t, 1, &Template{ID: 1, Type: "Monster", Level: 20, Passives: []skill.Ref{{ID: 4305, Level: 1}}})
	h.SeedState().Sow(100, seed)
	// base 4 + (20 - 10 - 5) = 9, times rate 3.
	if _, got := h.SeedState().HarvestedCrop(3); got != 27 {
		t.Fatalf("HarvestedCrop(3) count = %d, want 27", got)
	}
}
