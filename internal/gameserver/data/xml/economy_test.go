package xml

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
)

func TestLoadEconomyData(t *testing.T) {
	xmlDir := datapackPath(t, filepath.Join("data", "xml"))

	t.Run("recipes", func(t *testing.T) {
		table, err := LoadRecipes(filepath.Join(xmlDir, "recipes.xml"))
		if err != nil {
			t.Fatalf("LoadRecipes error: %v", err)
		}
		if got, want := table.Len(), 871; got != want {
			t.Fatalf("Len() = %d, want %d", got, want)
		}
		r, ok := table.Find(1)
		if !ok {
			t.Fatal("recipe 1 not loaded")
		}
		if r.Alias != "mk_wooden_arrow" || r.ItemID != 1666 || r.Level != 1 || r.MPCost != 30 || r.SuccessRate != 100 || !r.Dwarven {
			t.Fatalf("recipe 1 = %+v", r)
		}
		if len(r.Materials) != 2 || r.Materials[0].ItemID != 1864 || r.Materials[0].Count != 4 || r.Product.ItemID != 17 || r.Product.Count != 500 {
			t.Fatalf("recipe 1 ingredients/product = %+v / %+v", r.Materials, r.Product)
		}
		if byItem, ok := table.FindByItemID(1666); !ok || byItem.ID != 1 {
			t.Fatalf("FindByItemID(1666) = %+v, %v", byItem, ok)
		}
	})

	t.Run("buylists", func(t *testing.T) {
		table, err := LoadBuyLists(filepath.Join(xmlDir, "buyLists.xml"))
		if err != nil {
			t.Fatalf("LoadBuyLists error: %v", err)
		}
		if got, want := table.Len(), 687; got != want {
			t.Fatalf("Len() = %d, want %d", got, want)
		}
		if got, want := table.ProductCount(), 18812; got != want {
			t.Fatalf("ProductCount() = %d, want %d", got, want)
		}
		list, ok := table.Find(1)
		if !ok {
			t.Fatal("buylist 1 not loaded")
		}
		if list.NPCID != 30001 || !list.AllowsNPC(30001) || len(list.Products) != 23 {
			t.Fatalf("buylist 1 = %+v", list)
		}
		product, ok := list.FindProduct(1)
		if !ok {
			t.Fatal("buylist 1 product 1 not loaded")
		}
		if product.Price != 883 || product.LimitedStock() || product.MaxCount != -1 || product.RestockDelayMillis != -60000 {
			t.Fatalf("buylist 1 product 1 = %+v", product)
		}
	})

	t.Run("hennas", func(t *testing.T) {
		table, err := LoadHennas(filepath.Join(xmlDir, "hennas.xml"))
		if err != nil {
			t.Fatalf("LoadHennas error: %v", err)
		}
		if got, want := table.Len(), 180; got != want {
			t.Fatalf("Len() = %d, want %d", got, want)
		}
		h, ok := table.Find(1)
		if !ok {
			t.Fatal("henna 1 not loaded")
		}
		if h.DyeID != 4445 || h.DrawPrice != 37000 || h.STR != 1 || h.CON != -3 || h.RemovePrice() != 7400 || !h.UsableByClass(1) {
			t.Fatalf("henna 1 = %+v", h)
		}
		if len(h.Classes) != 18 {
			t.Fatalf("henna 1 class count = %d, want 18", len(h.Classes))
		}
	})

	t.Run("armor sets", func(t *testing.T) {
		table, err := LoadArmorSets(filepath.Join(xmlDir, "armorSets.xml"))
		if err != nil {
			t.Fatalf("LoadArmorSets error: %v", err)
		}
		if got, want := table.Len(), 51; got != want {
			t.Fatalf("Len() = %d, want %d", got, want)
		}
		set, ok := table.FindByChest(23)
		if !ok {
			t.Fatal("armor set chest 23 not loaded")
		}
		if set.Name != "Wooden Set" || set.Legs != 2386 || set.Head != 43 || set.SkillID != 3500 {
			t.Fatalf("armor set chest 23 = %+v", set)
		}
		if got := set.PieceIDs(); got != [5]int32{23, 2386, 43, 0, 0} {
			t.Fatalf("PieceIDs() = %+v", got)
		}
	})

	t.Run("fish", func(t *testing.T) {
		table, err := LoadFish(filepath.Join(xmlDir, "fish.xml"))
		if err != nil {
			t.Fatalf("LoadFish error: %v", err)
		}
		if got, want := table.Len(), 270; got != want {
			t.Fatalf("Len() = %d, want %d", got, want)
		}
		f, ok := table.Find(6411)
		if !ok {
			t.Fatal("fish 6411 not loaded")
		}
		if f.Level != 1 || f.HP != 100 || f.HPRegen != 4 || f.Type != 1 || f.Group != 1 || f.Guts != 500 || f.GutsCheckTime != 5000 || f.WaitTime != 20000 || f.CombatTime != 24000 {
			t.Fatalf("fish 6411 = %+v", f)
		}
	})

	t.Run("augmentations", func(t *testing.T) {
		table, err := LoadAugmentations(filepath.Join(xmlDir, "augmentation"))
		if err != nil {
			t.Fatalf("LoadAugmentations error: %v", err)
		}
		if got, want := table.SkillCount(), 1780; got != want {
			t.Fatalf("SkillCount() = %d, want %d", got, want)
		}
		if got, want := len(table.StatGroups), 4; got != want {
			t.Fatalf("len(StatGroups) = %d, want %d", got, want)
		}
		if got, want := table.StatCount(), 52; got != want {
			t.Fatalf("StatCount() = %d, want %d", got, want)
		}
		if got, want := bucketCount(table.Blue), 170; got != want {
			t.Fatalf("blue skill count = %d, want %d", got, want)
		}
		if got, want := bucketCount(table.Purple), 1070; got != want {
			t.Fatalf("purple skill count = %d, want %d", got, want)
		}
		if got, want := bucketCount(table.Red), 540; got != want {
			t.Fatalf("red skill count = %d, want %d", got, want)
		}
		skill, ok := table.FindSkill(14561)
		if !ok {
			t.Fatal("augmentation skill 14561 not loaded")
		}
		if skill.SkillID != 3203 || skill.SkillLevel != 1 || skill.Color != augmentation.Blue || skill.Level != 0 {
			t.Fatalf("augmentation skill 14561 = %+v", skill)
		}
		red, ok := table.FindSkill(16340)
		if !ok {
			t.Fatal("augmentation skill 16340 not loaded")
		}
		if red.SkillID != 3256 || red.SkillLevel != 3 || red.Color != augmentation.Red || red.Level != 9 {
			t.Fatalf("augmentation skill 16340 = %+v", red)
		}
		group := table.StatGroups[0]
		if group.Order != 0 || len(group.Stats) != 13 {
			t.Fatalf("first stat group = %+v", group)
		}
		stat := group.Stats[0]
		if stat.Name != "pDef" || len(stat.SoloValues) != 40 || len(stat.CombinedValues) != 40 || stat.SoloValues[0] != 15.4 || stat.CombinedValues[0] != 7.7 {
			t.Fatalf("first augmentation stat = %+v", stat)
		}
	})
}

func TestLoadEconomyDataErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		file    string
		content string
		load    func(string) error
	}{
		{
			name:    "recipe missing isDwarven",
			file:    "recipes.xml",
			content: `<list><recipe alias="x" id="1" material="1-1" product="2-1" itemId="3" level="1" mpConsume="1" successRate="100"/></list>`,
			load: func(path string) error {
				_, err := LoadRecipes(path)
				return err
			},
		},
		{
			name:    "buylist missing npcId",
			file:    "buyLists.xml",
			content: `<list><buyList id="1"><product id="1"/></buyList></list>`,
			load: func(path string) error {
				_, err := LoadBuyLists(path)
				return err
			},
		},
		{
			name:    "henna missing classes",
			file:    "hennas.xml",
			content: `<list><henna symbolId="1" dyeId="4445"/></list>`,
			load: func(path string) error {
				_, err := LoadHennas(path)
				return err
			},
		},
		{
			name:    "armor set missing chest",
			file:    "armorSets.xml",
			content: `<list><armorset name="x" legs="0" head="0" gloves="0" feet="0" skillId="1" shield="0" shieldSkillId="0" enchant6Skill="0"/></list>`,
			load: func(path string) error {
				_, err := LoadArmorSets(path)
				return err
			},
		},
		{
			name:    "fish missing hp",
			file:    "fish.xml",
			content: `<list><fish id="1" level="1" hpRegen="1" type="1" group="1" guts="1" gutsCheckTime="1" waitTime="1" combatTime="1"/></list>`,
			load: func(path string) error {
				_, err := LoadFish(path)
				return err
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, c.file)
			writeXMLFixture(t, path, c.content)
			if err := c.load(path); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}

	t.Run("augmentation missing skillLevel", func(t *testing.T) {
		augDir := filepath.Join(dir, "augmentation")
		if err := os.MkdirAll(augDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeXMLFixture(t, filepath.Join(augDir, "skills.xml"), `<list><augmentation id="14561" skillId="3203" type="blue"/></list>`)
		if _, err := LoadAugmentations(augDir); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func bucketCount(buckets [10][]int) int {
	n := 0
	for _, bucket := range buckets {
		n += len(bucket)
	}
	return n
}
