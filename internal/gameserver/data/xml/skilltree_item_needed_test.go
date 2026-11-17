package xml

import (
	"fmt"
	"path/filepath"
	"testing"
)

const enchantRow = `<list><enchantSkill id="1" lvl="101" exp="1" sp="1" rate76="1" rate77="1" rate78="1" rate79="1" rate80="1" itemNeeded="%s"/></list>`

// itemNeeded is a "left-right" pair read as two integers: the forms the
// reference's integer parsing takes (a plus sign, leading zeros) load, those
// it throws on (blanks, fractions, a missing half, a number past 32 bits, a
// negative half) fail the load.
func TestLoadSkillTreesItemNeededNumberForms(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		value     string
		wantID    int32
		wantCount int
		wantErr   bool
	}{
		{value: "6622-1", wantID: 6622, wantCount: 1},
		{value: "+6622-+3", wantID: 6622, wantCount: 3},
		{value: "06622-003", wantID: 6622, wantCount: 3},
		{value: "2147483647-1", wantID: 2147483647, wantCount: 1},
		{value: " 6622-1", wantErr: true},
		{value: "6622 -1", wantErr: true},
		{value: "6622-1.0", wantErr: true},
		{value: "6622-", wantErr: true},
		{value: "-1", wantErr: true},
		{value: "6622", wantErr: true},
		{value: "2147483648-1", wantErr: true},
		{value: "0x1A-1", wantErr: true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			writeXMLFixture(t, filepath.Join(dir, "enchantSkills.xml"), fmt.Sprintf(enchantRow, tc.value))
			trees, err := LoadSkillTrees(dir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("itemNeeded=%q loaded, want an error", tc.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("itemNeeded=%q: %v", tc.value, err)
			}
			if len(trees.Enchant) != 1 || trees.Enchant[0].ItemID != tc.wantID || trees.Enchant[0].ItemCount != tc.wantCount {
				t.Fatalf("itemNeeded=%q = %+v, want item %d x%d", tc.value, trees.Enchant, tc.wantID, tc.wantCount)
			}
		})
	}
}
