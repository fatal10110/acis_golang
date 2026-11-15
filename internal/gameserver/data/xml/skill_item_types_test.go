package xml

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/rs/zerolog"
)

// TestShippedItemSkillTypeAttrs pins, against the shipped XML, the
// attributes the CREATE_ITEM, CHANGE_APPEARANCE and SPAWN skills read:
// create_item_id (a ";" list), create_item_count (default 0) and
// random_count (default 1); faceId, hairColorId and hairStyleId (default
// -1 each); npcId and despawnDelay (default 0). It also pins the shipped
// skill 2215 level 1 capsule rows kept with their even and single field
// groups.
func TestShippedItemSkillTypeAttrs(t *testing.T) {
	t.Parallel()
	dir := datapackPath(t, filepath.Join("data", "xml", "skills"))
	table, err := LoadSkillDefinitions(dir, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", dir, err)
	}
	get := func(id skill.ID, level int) skill.Definition {
		t.Helper()
		def, ok := table.Get(id, level)
		if !ok {
			t.Fatalf("skill %d level %d missing", id, level)
		}
		return *def
	}

	for _, tt := range []struct {
		id            skill.ID
		level         int
		ids           []int32
		count, random int
	}{
		{2104, 1, []int32{1835}, 1000, 1},
		{1387, 3, []int32{2508}, 100, 300},
	} {
		def := get(tt.id, tt.level)
		if !reflect.DeepEqual(def.CreateItemIDs, tt.ids) || def.CreateItemCount != tt.count || def.CreateItemRandomCount != tt.random {
			t.Errorf("skill %d-%d create ids/count/random = %v/%d/%d, want %v/%d/%d", tt.id, tt.level,
				def.CreateItemIDs, def.CreateItemCount, def.CreateItemRandomCount, tt.ids, tt.count, tt.random)
		}
	}
	if def := get(2003, 1); def.CreateItemIDs != nil || def.CreateItemCount != 0 || def.CreateItemRandomCount != 1 {
		t.Errorf("non CREATE_ITEM skill 2003 create ids/count/random = %v/%d/%d, want nil/0/1", def.CreateItemIDs, def.CreateItemCount, def.CreateItemRandomCount)
	}

	for _, tt := range []struct {
		id                     skill.ID
		face, hairColor, style int
	}{
		{2291, 0, -1, -1},
		{2135, -1, -1, 6},
	} {
		def := get(tt.id, 1)
		if def.AppearanceFace != tt.face || def.AppearanceHairColor != tt.hairColor || def.AppearanceHairStyle != tt.style {
			t.Errorf("skill %d face/hair color/hair style = %d/%d/%d, want %d/%d/%d", tt.id,
				def.AppearanceFace, def.AppearanceHairColor, def.AppearanceHairStyle, tt.face, tt.hairColor, tt.style)
		}
	}

	for _, tt := range []struct {
		id      skill.ID
		npcID   int
		despawn int
	}{
		{2003, 12774, 120000},
		{2137, 13006, 1200000},
	} {
		def := get(tt.id, 1)
		if def.NpcID != tt.npcID || def.DespawnDelay != tt.despawn {
			t.Errorf("skill %d npc/despawn = %d/%d, want %d/%d", tt.id, def.NpcID, def.DespawnDelay, tt.npcID, tt.despawn)
		}
	}
	if def := get(2104, 1); def.DespawnDelay != 0 || def.AppearanceFace != -1 {
		t.Errorf("skill 2104 despawn/face = %d/%d, want the defaults 0/-1", def.DespawnDelay, def.AppearanceFace)
	}

	products := skill.ParseExtractableItems(get(2215, 1).ExtractableItems)
	if len(products) != 9 {
		t.Fatalf("skill 2215-1 rows = %d, want 9", len(products))
	}
	want := []skill.ExtractableProduct{
		{Items: []skill.ExtractableItem{}, Chance: 57},
		{Items: []skill.ExtractableItem{{ItemID: 100000, Quantity: 8}}, Chance: 8},
		{Items: []skill.ExtractableItem{{ItemID: 3456, Quantity: 1}}, Chance: 10},
	}
	if !reflect.DeepEqual(products[:3], want) {
		t.Errorf("skill 2215-1 first rows = %#v, want %#v", products[:3], want)
	}
}
