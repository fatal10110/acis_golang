package xml

import (
	"path/filepath"
	"testing"
)

func TestLoadSkillTrees(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "skillstrees"))

	trees, err := LoadSkillTrees(dir)
	if err != nil {
		t.Fatalf("LoadSkillTrees(%q) error: %v", dir, err)
	}

	if got, want := len(trees.Fishing), 117; got != want {
		t.Errorf("len(Fishing) = %d, want %d", got, want)
	}
	if got, want := len(trees.Clan), 64; got != want {
		t.Errorf("len(Clan) = %d, want %d", got, want)
	}
	if got, want := len(trees.Enchant), 14550; got != want {
		t.Errorf("len(Enchant) = %d, want %d", got, want)
	}

	t.Run("fishing skill", func(t *testing.T) {
		for _, fs := range trees.Fishing {
			if fs.ID != 1312 {
				continue
			}
			if fs.Level != 1 || fs.MinLevel != 1 || fs.ItemID != 57 || fs.ItemCount != 1000 || fs.Dwarven {
				t.Fatalf("fishing skill 1312 = %+v", fs)
			}
			return
		}
		t.Fatal("fishing skill 1312 not loaded")
	})

	t.Run("dwarven fishing skill", func(t *testing.T) {
		for _, fs := range trees.Fishing {
			if fs.ID != 1368 || fs.Level != 1 {
				continue
			}
			if !fs.Dwarven {
				t.Fatalf("fishing skill 1368 level 1 Dwarven = false, want true (%+v)", fs)
			}
			return
		}
		t.Fatal("fishing skill 1368 level 1 not loaded")
	})

	t.Run("clan skill", func(t *testing.T) {
		for _, cs := range trees.Clan {
			if cs.ID != 370 || cs.Level != 1 {
				continue
			}
			if cs.MinLevel != 5 || cs.Cost != 500 || cs.ItemID != 8166 {
				t.Fatalf("clan skill 370 level 1 = %+v", cs)
			}
			return
		}
		t.Fatal("clan skill 370 level 1 not loaded")
	})

	t.Run("enchant skill with an item requirement", func(t *testing.T) {
		for _, es := range trees.Enchant {
			if es.ID != 1 || es.Level != 101 {
				continue
			}
			if es.Exp != 5500000 || es.SP != 550000 {
				t.Fatalf("enchant skill 1 level 101 exp/sp = %+v", es)
			}
			if es.Rate76 != 82 || es.Rate77 != 92 || es.Rate78 != 97 || es.Rate79 != 100 || es.Rate80 != 100 {
				t.Fatalf("enchant skill 1 level 101 rates = %+v", es)
			}
			if es.ItemID != 6622 || es.ItemCount != 1 {
				t.Fatalf("enchant skill 1 level 101 item = id=%d count=%d, want 6622/1", es.ItemID, es.ItemCount)
			}
			return
		}
		t.Fatal("enchant skill 1 level 101 not loaded")
	})

	t.Run("enchant skill without an item requirement", func(t *testing.T) {
		for _, es := range trees.Enchant {
			if es.ID != 1 || es.Level != 102 {
				continue
			}
			if es.ItemID != 0 || es.ItemCount != 0 {
				t.Fatalf("enchant skill 1 level 102 item = id=%d count=%d, want 0/0", es.ItemID, es.ItemCount)
			}
			return
		}
		t.Fatal("enchant skill 1 level 102 not loaded")
	})
}

func TestLoadSkillTreesErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed xml",
			content: `<list><fishingSkill id="1" <lvl="1" minLvl="1" itemId="1" itemCount="1"/></list>`,
		},
		{
			name:    "fishing skill missing required itemCount attribute",
			content: `<list><fishingSkill id="1" lvl="1" minLvl="1" itemId="1"/></list>`,
		},
		{
			name:    "clan skill missing required cost attribute",
			content: `<list><clanSkill id="1" lvl="1" minLvl="1" itemId="1"/></list>`,
		},
		{
			name:    "enchant skill missing a required rate attribute",
			content: `<list><enchantSkill id="1" lvl="101" exp="1" sp="1" rate76="1" rate77="1" rate78="1" rate79="1"/></list>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "fixture.xml")
			writeXMLFixture(t, path, c.content)
			if _, err := LoadSkillTrees(dir); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := LoadSkillTrees(empty); err == nil {
			t.Fatal("expected an error for an empty directory, got nil")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadSkillTrees(filepath.Join(dir, "does-not-exist")); err == nil {
			t.Fatal("expected an error for a missing directory, got nil")
		}
	})
}
