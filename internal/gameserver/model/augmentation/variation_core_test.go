package augmentation_test

import (
	"path/filepath"
	"slices"
	"testing"

	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// shippedTable loads the datapack's augmentation directory.
func shippedTable(t *testing.T) *augmentation.Table {
	t.Helper()
	table, err := gamexml.LoadAugmentations(filepath.Join(datapack.Require(t), "data", "xml", "augmentation"))
	if err != nil {
		t.Fatalf("LoadAugmentations: %v", err)
	}
	return table
}

// TestBonusesDecodeShippedStatTables checks augmentation ids against stat
// values read by hand out of data/xml/augmentation/stats.xml, following the
// id layout of the reference's AugmentationData.getAugStatsById: an option
// id from 1 is (id-1) = color*3640 + level*91 + offset, offsets 0-12 the
// solo stats of the color's <set order>, 13-90 every pair (i<j) at their
// combined values, 16341-16344 STR/CON/INT/MEN +1, the high half stat34.
func TestBonusesDecodeShippedStatTables(t *testing.T) {
	table := shippedTable(t)
	cases := []struct {
		name string
		id   int32
		want []augmentation.Bonus
	}{
		// Order 0 pDef solo[0].
		{"first solo", 1, []augmentation.Bonus{{"pDef", 15.4}}},
		// 196-1 = level 2, offset 13: pDef with mDef, combined[2].
		{"first pair at level 2", 196, []augmentation.Bonus{{"pDef", 8.6}, {"mDef", 6.3}}},
		// 14482-1 = 3*3640 + 39*91 + 12: order 3 rCrit solo[39].
		{"last solo of the last block", 14482, []augmentation.Bonus{{"rCrit", 21.8}}},
		// 7371-1 = 2*3640 + 90: the last pair, accCombat with rCrit,
		// order 2 combined[0].
		{"last pair", 7371, []augmentation.Bonus{{"accCombat", 0.5}, {"rCrit", 5.2}}},
		// stat34 16343 is INT +1, after stat12's pDef.
		{"base stat", 16343<<16 + 1, []augmentation.Bonus{{"pDef", 15.4}, {"INT", 1}}},
		// stat34 14561 is a skill option: no bonus of its own.
		{"skill option", 14561<<16 + 1, []augmentation.Bonus{{"pDef", 15.4}}},
	}
	for _, tc := range cases {
		if got := table.Bonuses(tc.id); !slices.Equal(got, tc.want) {
			t.Errorf("%s: Bonuses(%d) = %v, want %v", tc.name, tc.id, got, tc.want)
		}
	}
}

// draw is one scripted random int: the bounds the roll must ask for and
// the value it gets.
type draw struct{ min, max, value int }

func scripted(t *testing.T, draws []draw) augmentation.Rand {
	t.Helper()
	i := 0
	t.Cleanup(func() {
		if i != len(draws) {
			t.Errorf("used %d of %d scripted draws", i, len(draws))
		}
	})
	return func(lo, hi int) int {
		if i >= len(draws) {
			t.Fatalf("unscripted draw %d [%d,%d]", i, lo, hi)
		}
		d := draws[i]
		i++
		if lo != d.min || hi != d.max {
			t.Fatalf("draw %d asks [%d,%d], want [%d,%d]", i, lo, hi, d.min, d.max)
		}
		return d.value
	}
}

// TestGenerateFollowsReferenceDrawOrder walks the reference's
// generateRandomAugmentation by hand for two life stones: the bounds of
// every draw and the id it builds are derived from that method's
// arithmetic, not from this package.
func TestGenerateFollowsReferenceDrawOrder(t *testing.T) {
	table := shippedTable(t)

	t.Run("no-grade stone without skill", func(t *testing.T) {
		// No skill (50 > 15), no glow (50 > 0), no base stat (50 > 1); color
		// 30 <= 40 is blue (1). temp 2: stat34 from 1*910 + 2*3640 + 1 =
		// 8191; stat12's block 1*910 + 1 = 911.
		rnd := scripted(t, []draw{
			{1, 100, 50},
			{1, 100, 50},
			{1, 100, 50},
			{0, 100, 30},
			{2, 3, 2},
			{8191, 8281, 8200},
			{0, 1, 1},
			{911, 1001, 950},
		})
		got := table.Generate(0, augmentation.GradeNone, augmentation.DefaultChances(), rnd)
		if want := int32(8200<<16 + 950); got.ID != want || got.Skill != nil {
			t.Fatalf("Generate = %+v, want id %d without skill", got, want)
		}
	})

	t.Run("top-grade stone with skill", func(t *testing.T) {
		// Level 12 clamps to 9. Skill (10 <= 60), glow (100 <= 100), no base
		// roll; color 20 <= 35 is red. The first of level 9's 54 red skills
		// is 16287 (skill 3202 level 3). Glow block: 9*91 + 0*3640 +
		// (3+3)/2*910 + 1 = 3550.
		rnd := scripted(t, []draw{
			{1, 100, 10},
			{1, 100, 100},
			{0, 100, 20},
			{0, 53, 0},
			{0, 1, 0},
			{3550, 3640, 3600},
		})
		got := table.Generate(12, augmentation.GradeTop, augmentation.DefaultChances(), rnd)
		if want := int32(16287<<16 + 3600); got.ID != want {
			t.Fatalf("Generate id = %d, want %d", got.ID, want)
		}
		if got.Skill == nil || got.Skill.SkillID != 3202 || got.Skill.SkillLevel != 3 {
			t.Fatalf("Generate skill = %+v, want 3202 level 3", got.Skill)
		}
	})

	t.Run("base stat is always red", func(t *testing.T) {
		// No skill, base stat (1 <= 1) STR; color 90 still turns red (3).
		// stat34 is set: no glow, block rnd(0,1)*3640 + 1.
		rnd := scripted(t, []draw{
			{1, 100, 90},
			{1, 100, 90},
			{1, 100, 1},
			{16341, 16344, 16341},
			{0, 100, 90},
			{0, 1, 1},
			{3641, 3731, 3641},
		})
		got := table.Generate(0, augmentation.GradeMid, augmentation.DefaultChances(), rnd)
		if want := int32(16341<<16 + 3641); got.ID != want || got.Skill != nil {
			t.Fatalf("Generate = %+v, want id %d without skill", got, want)
		}
	})
}

// TestLifeStonesByItemID checks the life stone item ranges: 8723-8732 no
// grade, then mid, high and top, ten levels each, with the level a player
// needs.
func TestLifeStonesByItemID(t *testing.T) {
	cases := []struct {
		itemID      int32
		grade, lvl  int
		playerLevel int
	}{
		{8723, augmentation.GradeNone, 0, 46},
		{8732, augmentation.GradeNone, 9, 76},
		{8733, augmentation.GradeMid, 0, 46},
		{8752, augmentation.GradeHigh, 9, 76}, // 8723 + 29
		{8762, augmentation.GradeTop, 9, 76},
	}
	for _, tc := range cases {
		ls, ok := augmentation.LifeStoneByItemID(tc.itemID)
		if !ok || ls.Grade != tc.grade || ls.Level != tc.lvl || ls.PlayerLevel() != tc.playerLevel {
			t.Errorf("LifeStoneByItemID(%d) = %+v %v (player level %d), want grade %d level %d player level %d",
				tc.itemID, ls, ok, ls.PlayerLevel(), tc.grade, tc.lvl, tc.playerLevel)
		}
	}
	for _, id := range []int32{8722, 8763} {
		if _, ok := augmentation.LifeStoneByItemID(id); ok {
			t.Errorf("LifeStoneByItemID(%d) resolved, want no life stone", id)
		}
	}
}
