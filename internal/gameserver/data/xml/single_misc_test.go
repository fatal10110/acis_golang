package xml

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSoulCrystalData(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "soulCrystals.xml"))

	table, err := LoadSoulCrystalData(path)
	if err != nil {
		t.Fatalf("LoadSoulCrystalData(%q) error: %v", path, err)
	}

	if got := table.CrystalCount(); got != 39 {
		t.Fatalf("CrystalCount() = %d, want 39", got)
	}
	if got := table.LevelingInfoCount(); got != 124 {
		t.Fatalf("LevelingInfoCount() = %d, want 124", got)
	}
	crystal, ok := table.Crystal(5582)
	if !ok || crystal.StagedItemID != 5914 || crystal.Level != 12 {
		t.Fatalf("Crystal(5582) = %+v, %v", crystal, ok)
	}
	info, ok := table.LevelingInfo(22215)
	if !ok || info.AbsorbType != "PARTY_ONE_RANDOM" || len(info.Levels) != 2 || info.Levels[0] != 10 {
		t.Fatalf("LevelingInfo(22215) = %+v, %v", info, ok)
	}
}

func TestLoadSpellbooks(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "spellbooks.xml"))

	table, err := LoadSpellbooks(path)
	if err != nil {
		t.Fatalf("LoadSpellbooks(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 334 {
		t.Fatalf("Count() = %d, want 334", got)
	}
	if got := table.BookForSkill(2, 1, true, true); got != 1512 {
		t.Fatalf("BookForSkill(2, 1, true, true) = %d, want 1512", got)
	}
}

func TestLoadSummonItems(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "summonItems.xml"))

	table, err := LoadSummonItems(path)
	if err != nil {
		t.Fatalf("LoadSummonItems(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 13 {
		t.Fatalf("Count() = %d, want 13", got)
	}
	entry, ok := table.Item(2375)
	if !ok || entry.NPCID != 12077 || entry.SummonType != 1 {
		t.Fatalf("Item(2375) = %+v, %v", entry, ok)
	}
}

func TestLoadHealSps(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "healSps.xml"))

	table, err := LoadHealSps(path)
	if err != nil {
		t.Fatalf("LoadHealSps(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 29 {
		t.Fatalf("Count() = %d, want 29", got)
	}
	if got := table.Calculate(1401, 11, 76, 875); got != 286 {
		t.Fatalf("Calculate(1401, 11, 76, 875) = %v, want 286", got)
	}
}

func TestLoadNewbieBuffs(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "newbieBuffs.xml"))

	table, err := LoadNewbieBuffs(path)
	if err != nil {
		t.Fatalf("LoadNewbieBuffs(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 14 {
		t.Fatalf("Count() = %d, want 14", got)
	}
	if got := table.LowestBuffLevel(false); got != 8 {
		t.Fatalf("LowestBuffLevel(false) = %d, want 8", got)
	}
	if got := len(table.ValidBuffs(true, 12)); got != 3 {
		t.Fatalf("len(ValidBuffs(true, 12)) = %d, want 3", got)
	}
}

func TestLoadAdminData(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml"))

	data, err := LoadAdminData(dir)
	if err != nil {
		t.Fatalf("LoadAdminData(%q) error: %v", dir, err)
	}

	if got := data.AccessLevelCount(); got != 10 {
		t.Fatalf("AccessLevelCount() = %d, want 10", got)
	}
	if got := data.CommandCount(); got != 93 {
		t.Fatalf("CommandCount() = %d, want 93", got)
	}
	level, ok := data.AccessLevel(7)
	if !ok || !level.IsGM || level.Name != "Admin" {
		t.Fatalf("AccessLevel(7) = %+v, %v", level, ok)
	}
	cmd, ok := data.Command("admin_ann")
	if !ok || cmd.AccessLevel != 7 {
		t.Fatalf("Command(admin_ann) = %+v, %v", cmd, ok)
	}
}

func TestLoadAnnouncements(t *testing.T) {
	path := datapackPath(t, filepath.Join("data", "xml", "announcements.xml"))

	list, err := LoadAnnouncements(path)
	if err != nil {
		t.Fatalf("LoadAnnouncements(%q) error: %v", path, err)
	}
	if got := len(list); got != 0 {
		t.Fatalf("len(announcements) = %d, want 0", got)
	}
}

func TestLoadCursedWeapons(t *testing.T) {
	skillsPath := datapackPath(t, filepath.Join("data", "xml", "skills"))
	skills, err := LoadSkillDefinitions(skillsPath)
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", skillsPath, err)
	}

	path := datapackPath(t, filepath.Join("data", "xml", "cursedWeapons.xml"))
	table, err := LoadCursedWeapons(path, skills)
	if err != nil {
		t.Fatalf("LoadCursedWeapons(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 2 {
		t.Fatalf("Count() = %d, want 2", got)
	}
	weapon, ok := table.Weapon(8190)
	if !ok {
		t.Fatal("Weapon(8190) missing")
	}
	if weapon.Name != "Demonic Sword Zariche" || weapon.Skill.Level != skills.MaxLevel(3603) || weapon.StageKills != 10 {
		t.Fatalf("Weapon(8190) = %+v", weapon)
	}
}

func TestLoadBufferSkills(t *testing.T) {
	skillsPath := datapackPath(t, filepath.Join("data", "xml", "skills"))
	skills, err := LoadSkillDefinitions(skillsPath)
	if err != nil {
		t.Fatalf("LoadSkillDefinitions(%q) error: %v", skillsPath, err)
	}

	path := datapackPath(t, filepath.Join("data", "xml", "bufferSkills.xml"))
	table, err := LoadBufferSkills(path, skills)
	if err != nil {
		t.Fatalf("LoadBufferSkills(%q) error: %v", path, err)
	}

	if got := table.Count(); got != 60 {
		t.Fatalf("Count() = %d, want 60", got)
	}
	if got := table.Categories(); len(got) != 3 || got[0] != "Buffs" || got[1] != "Dances" || got[2] != "Songs" {
		t.Fatalf("Categories() = %#v, want [Buffs Dances Songs]", got)
	}
	entry, ok := table.Skill(1035)
	if !ok {
		t.Fatal("Skill(1035) missing")
	}
	if entry.Skill.Level != skills.MaxLevel(1035) || entry.Category != "Buffs" || entry.Description != "Increases resistance to mental attacks." {
		t.Fatalf("Skill(1035) = %+v", entry)
	}
}

func TestSingleMiscLoadersErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		path    string
		content string
		load    func(string) error
	}{
		{
			name:    "soul crystal missing level",
			path:    filepath.Join(dir, "soulCrystals.xml"),
			content: `<list><crystals><crystal initial="4629" staged="4630" broken="4662"/></crystals></list>`,
			load: func(path string) error {
				_, err := LoadSoulCrystalData(path)
				return err
			},
		},
		{
			name:    "spellbook malformed xml",
			path:    filepath.Join(dir, "spellbooks.xml"),
			content: `<list><book skillId="2" itemId="1512"></list>`,
			load: func(path string) error {
				_, err := LoadSpellbooks(path)
				return err
			},
		},
		{
			name:    "summon item missing npcId",
			path:    filepath.Join(dir, "summonItems.xml"),
			content: `<list><item id="2375" summonType="1"/></list>`,
			load: func(path string) error {
				_, err := LoadSummonItems(path)
				return err
			},
		},
		{
			name:    "heal sps missing correction",
			path:    filepath.Join(dir, "healSps.xml"),
			content: `<list><healSps magicLevel="1" neededMatk="6"/></list>`,
			load: func(path string) error {
				_, err := LoadHealSps(path)
				return err
			},
		},
		{
			name:    "newbie buff missing upperLevel",
			path:    filepath.Join(dir, "newbieBuffs.xml"),
			content: `<list><buff skillId="4322" skillLevel="1" lowerLevel="8" isMagicClass="false"/></list>`,
			load: func(path string) error {
				_, err := LoadNewbieBuffs(path)
				return err
			},
		},
		{
			name:    "announcement empty message",
			path:    filepath.Join(dir, "announcements.xml"),
			content: `<list><announcement message="" /></list>`,
			load: func(path string) error {
				_, err := LoadAnnouncements(path)
				return err
			},
		},
		{
			name:    "cursed weapon missing skillId",
			path:    filepath.Join(dir, "cursedWeapons.xml"),
			content: `<list><item id="8190" name="Zariche" dropRate="1" duration="72" durationLost="24" dissapearChance="50" stageKills="10"/></list>`,
			load: func(path string) error {
				_, err := LoadCursedWeapons(path, nil)
				return err
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			writeXMLFixture(t, c.path, c.content)
			if err := c.load(c.path); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}

	t.Run("admin data missing command file", func(t *testing.T) {
		adminDir := filepath.Join(dir, "admin")
		if err := os.MkdirAll(adminDir, 0o755); err != nil {
			t.Fatal(err)
		}
		writeXMLFixture(t, filepath.Join(adminDir, "accessLevels.xml"), `<list></list>`)
		if _, err := LoadAdminData(adminDir); err == nil {
			t.Fatal("expected an error for missing adminCommands.xml, got nil")
		}
	})

	t.Run("buffer skills missing skill definition for default level", func(t *testing.T) {
		path := filepath.Join(dir, "bufferSkills.xml")
		writeXMLFixture(t, path, `<list><category type="Buffs"><buff id="1035" desc="desc"/></category></list>`)
		if _, err := LoadBufferSkills(path, nil); err == nil {
			t.Fatal("expected an error for missing skill table, got nil")
		}
	})
}
