package xml

import (
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// TestShippedRecallDestinations pins the recall destination fields the
// shipped RECALL and TELEPORT skills load with: recallType (Castle,
// ClanHall, otherwise town) and the per-level teleCoords table, a "0;0;0"
// entry included, which still names a location.
func TestShippedRecallDestinations(t *testing.T) {
	t.Parallel()
	dir := datapackPath(t, filepath.Join("data", "xml", "skills"))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", dir, err)
	}
	for _, tc := range []struct {
		id     skill.ID
		level  int
		recall skill.RecallType
		coords *location.Location
	}{
		{2013, 1, skill.RecallTown, nil},     // Scroll Of Escape
		{2099, 1, skill.RecallTown, nil},     // Escape: 5 minutes
		{1255, 2, skill.RecallTown, nil},     // Party Recall
		{2177, 1, skill.RecallClanHall, nil}, // Blessed Scroll of Escape: Clan Hall
		{2178, 1, skill.RecallCastle, nil},   // Blessed Scroll of Escape: Castle
		{2213, 1, skill.RecallTown, &location.Location{X: -84200, Y: 244544, Z: -3728}},
		{2213, 21, skill.RecallTown, &location.Location{X: 107946, Y: -52280, Z: -2408}},
		{2214, 1, skill.RecallTown, &location.Location{X: -84200, Y: 244566, Z: -3728}},
		{2214, 6, skill.RecallTown, &location.Location{}},
	} {
		def, ok := table.Get(tc.id, tc.level)
		if !ok {
			t.Fatalf("skill %d level %d missing", tc.id, tc.level)
		}
		if def.RecallType != tc.recall {
			t.Errorf("skill %d level %d RecallType = %v, want %v", tc.id, tc.level, def.RecallType, tc.recall)
		}
		switch {
		case tc.coords == nil && def.TeleCoords != nil:
			t.Errorf("skill %d level %d TeleCoords = %+v, want none", tc.id, tc.level, *def.TeleCoords)
		case tc.coords != nil && (def.TeleCoords == nil || *def.TeleCoords != *tc.coords):
			t.Errorf("skill %d level %d TeleCoords = %v, want %+v", tc.id, tc.level, def.TeleCoords, *tc.coords)
		}
	}
}

// TestParseRecallAttributes pins the recallType and teleCoords readings:
// recallType compares without case and defaults to town; teleCoords needs
// three integer parts, trims them, ignores extra integer parts and is
// dropped when any part is not an integer.
func TestParseRecallAttributes(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]skill.RecallType{
		"Castle": skill.RecallCastle, "castle": skill.RecallCastle, "CLANHALL": skill.RecallClanHall,
		"ClanHall": skill.RecallClanHall, "": skill.RecallTown, "Town": skill.RecallTown, "SiegeFlag": skill.RecallTown,
	} {
		if got := skill.ParseRecallType(in); got != want {
			t.Errorf("ParseRecallType(%q) = %v, want %v", in, got, want)
		}
	}
	for in, want := range map[string]*location.Location{
		"1;2;3":          {X: 1, Y: 2, Z: 3},
		" -1 ; +2 ; 3 ":  {X: -1, Y: 2, Z: 3},
		"1;2;3;4":        {X: 1, Y: 2, Z: 3},
		"1;2":            nil,
		"1;2;x":          nil,
		"1;2;3;x":        nil,
		"":               nil,
		"#teleCoords":    nil,
		"1;2;9999999999": nil,
	} {
		got, ok := skill.ParseTeleCoords(in)
		switch {
		case want == nil && ok:
			t.Errorf("ParseTeleCoords(%q) = %+v, want none", in, got)
		case want != nil && (!ok || got != *want):
			t.Errorf("ParseTeleCoords(%q) = %+v, %v, want %+v", in, got, ok, *want)
		}
	}
}
