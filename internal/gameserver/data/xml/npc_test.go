package xml

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	itemmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
)

// gremlinDropItemIDs are the item ids Gremlin's (npc 20001) drop table
// references, used to build a fixture item.Table that lets those drops
// validate.
var gremlinDropItemIDs = []int32{8600, 8601, 8602, 8603, 8604, 8605, 8606, 8607, 8608, 8609, 8610, 8611, 8612, 8613, 8614}

func itemTableWithIDs(ids []int32) *itemmodel.Table {
	templates := make([]*itemmodel.Template, len(ids))
	for i, id := range ids {
		templates[i] = &itemmodel.Template{ID: id}
	}
	return itemmodel.NewTable(templates)
}

func emptySkillTable() *skill.Table {
	return skill.NewTable(nil)
}

func skillTableWith(refs ...skill.Ref) *skill.Table {
	defs := make([]skill.Definition, len(refs))
	for i, ref := range refs {
		defs[i] = skill.Definition{ID: ref.ID, Level: ref.Level}
	}
	return skill.NewTable(defs)
}

const npcRequiredSets = `<set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/>`

func loadNPCFixture(t *testing.T, body string, skills *skill.Table) *npc.Table {
	t.Helper()
	dir := t.TempDir()
	writeXMLFixture(t, filepath.Join(dir, "fixture.xml"), body)
	table, err := LoadNPCTemplates(dir, itemTableWithIDs(nil), skills, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadNPCTemplates: %v", err)
	}
	return table
}

func TestLoadNPCTemplates(t *testing.T) {
	dir := datapackPath(t, filepath.Join("data", "xml", "npcs"))
	skills, err := LoadSkillDefinitions(datapackPath(t, filepath.Join("data", "xml", "skills")), zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadSkillDefinitions: %v", err)
	}

	// The full 16-file datapack takes a few seconds to parse; load it once
	// per item-table variant and share the result across subtests instead
	// of reloading it for every assertion.
	withGremlinItems, err := LoadNPCTemplates(dir, itemTableWithIDs(gremlinDropItemIDs), skills, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadNPCTemplates(%q) error: %v", dir, err)
	}
	withNoItems, err := LoadNPCTemplates(dir, itemTableWithIDs(nil), skills, zerolog.Nop())
	if err != nil {
		t.Fatalf("LoadNPCTemplates(%q) error: %v", dir, err)
	}

	t.Run("base fields and drops validate against a loaded item table", func(t *testing.T) {
		table := withGremlinItems

		gremlin, ok := table.Get(20001)
		if !ok {
			t.Fatal("npc 20001 (Gremlin) not loaded")
		}
		if gremlin.Name != "Gremlin" || gremlin.Title != "" || gremlin.Alias != "gremlin" {
			t.Fatalf("Gremlin identity = %+v", gremlin)
		}
		if gremlin.Type != "Monster" || gremlin.Level != 1 {
			t.Fatalf("Gremlin type/level = %q/%d", gremlin.Type, gremlin.Level)
		}
		if gremlin.CollisionRadius != 10.0 || gremlin.CollisionHeight != 15.0 {
			t.Fatalf("Gremlin collision = %v/%v", gremlin.CollisionRadius, gremlin.CollisionHeight)
		}
		if gremlin.RewardExp != 29.0 || gremlin.RewardSp != 2.0 {
			t.Fatalf("Gremlin exp/sp = %v/%v", gremlin.RewardExp, gremlin.RewardSp)
		}
		if gremlin.HPMax != 39.0 || gremlin.MPMax != 44.0 || gremlin.HPRegen != 3.16 || gremlin.MPRegen != 0.91 {
			t.Fatalf("Gremlin hp/mp = %+v", gremlin)
		}
		wantDamageRange := []int{0, 0, 80, 120}
		if len(gremlin.BaseDamageRange) != len(wantDamageRange) {
			t.Fatalf("Gremlin BaseDamageRange = %v, want %v", gremlin.BaseDamageRange, wantDamageRange)
		}
		for i, v := range wantDamageRange {
			if gremlin.BaseDamageRange[i] != v {
				t.Fatalf("Gremlin BaseDamageRange = %v, want %v", gremlin.BaseDamageRange, wantDamageRange)
			}
		}
		if gremlin.STR != 40 || gremlin.CON != 43 || gremlin.DEX != 30 || gremlin.INT != 21 || gremlin.WIT != 20 || gremlin.MEN != 10 {
			t.Fatalf("Gremlin base stats = %+v", gremlin)
		}
		if gremlin.PAtk != 7.56 || gremlin.PDef != 39.0 || gremlin.MAtk != 6.48 || gremlin.MDef != 29.44 {
			t.Fatalf("Gremlin combat stats = %+v", gremlin)
		}
		if gremlin.CorpseTime != 7 {
			t.Fatalf("Gremlin CorpseTime = %d, want default 7", gremlin.CorpseTime)
		}
		if gremlin.AggroRange != 1000 || !gremlin.Seedable || gremlin.CanSeeThrough {
			t.Fatalf("Gremlin aggro/seedable/seeThrough = %+v", gremlin)
		}

		// Race is encoded via the dedicated race skill (id 4416) at level
		// 13, not a "race" attribute.
		if gremlin.Race != npc.RaceFairy {
			t.Fatalf("Gremlin Race = %v, want RaceFairy", gremlin.Race)
		}

		wantAI := map[string]string{"MoveAroundSocial": "0", "MoveAroundSocial1": "0", "MoveAroundSocial2": "0"}
		for k, v := range wantAI {
			got, err := gremlin.AIParams.GetString(k)
			if err != nil || got != v {
				t.Fatalf("Gremlin AIParams[%q] = %q, %v, want %q", k, got, err, v)
			}
		}

		if len(gremlin.Drops) != 6 {
			t.Fatalf("Gremlin Drops category count = %d, want 6", len(gremlin.Drops))
		}
		first := gremlin.Drops[0]
		if first.Kind != itemmodel.DropHerb || first.Chance != 42.0 {
			t.Fatalf("Gremlin Drops[0] = %+v", first)
		}
		wantFirstDrops := []itemmodel.Drop{
			{ItemID: 8600, Min: 1, Max: 1, Chance: 55.0},
			{ItemID: 8601, Min: 1, Max: 1, Chance: 38.0},
			{ItemID: 8602, Min: 1, Max: 1, Chance: 7.0},
		}
		if len(first.Drops) != len(wantFirstDrops) {
			t.Fatalf("Gremlin Drops[0].Drops = %+v, want %+v", first.Drops, wantFirstDrops)
		}
		for i, d := range wantFirstDrops {
			if first.Drops[i] != d {
				t.Fatalf("Gremlin Drops[0].Drops[%d] = %+v, want %+v", i, first.Drops[i], d)
			}
		}
	})

	t.Run("drops referencing an unloaded item are skipped, not an error", func(t *testing.T) {
		table := withNoItems
		gremlin, ok := table.Get(20001)
		if !ok {
			t.Fatal("npc 20001 (Gremlin) not loaded")
		}
		if len(gremlin.Drops) != 6 {
			t.Fatalf("Gremlin Drops category count = %d, want 6 (categories survive even when every drop is skipped)", len(gremlin.Drops))
		}
		for i, c := range gremlin.Drops {
			if len(c.Drops) != 0 {
				t.Fatalf("Gremlin Drops[%d].Drops = %+v, want empty (no items loaded)", i, c.Drops)
			}
		}
	})

	t.Run("pet template without mount data defaults mount fields to zero", func(t *testing.T) {
		table := withNoItems
		wolf, ok := table.Get(12077)
		if !ok {
			t.Fatal("npc 12077 (Wolf) not loaded")
		}
		if wolf.Pet == nil {
			t.Fatal("Wolf.Pet is nil, want populated")
		}
		if wolf.Pet.Food1 != 2515 || wolf.Pet.Food2 != 0 {
			t.Fatalf("Wolf.Pet food = %d/%d", wolf.Pet.Food1, wolf.Pet.Food2)
		}
		if wolf.Pet.AutoFeedLimit != 0.55 || wolf.Pet.HungryLimit != 0.5 || wolf.Pet.UnsummonLimit != 0.4 {
			t.Fatalf("Wolf.Pet limits = %+v", wolf.Pet)
		}
		if len(wolf.Pet.Levels) != 81 {
			t.Fatalf("Wolf.Pet.Levels count = %d, want 81", len(wolf.Pet.Levels))
		}
		lvl1, ok := wolf.Pet.Levels[1]
		if !ok {
			t.Fatal("Wolf.Pet.Levels[1] missing")
		}
		want := npc.PetLevelStats{
			MaxExp: 0, MaxMeal: 248, ExpType: -1, MealInBattle: 2, MealInNormal: 2,
			PAtk: 2.11864406779661, PDef: 11.1111111111111, MAtk: 1.44675925925926, MDef: 8.13062889692864,
			MaxHP: 19.8725961538461, MaxMP: 20.0, HPRegen: 2.0, MPRegen: 0.9, SSCount: 1, SPSCount: 1,
		}
		if lvl1 != want {
			t.Fatalf("Wolf.Pet.Levels[1] = %+v, want %+v", lvl1, want)
		}

		// Race is encoded via the dedicated race skill (id 4416) at level 4.
		if wolf.Race != npc.RaceAnimal {
			t.Fatalf("Wolf.Race = %v, want RaceAnimal", wolf.Race)
		}
	})

	t.Run("pet template with mount data", func(t *testing.T) {
		table := withNoItems
		strider, ok := table.Get(12526)
		if !ok {
			t.Fatal("npc 12526 (Wind Strider) not loaded")
		}
		if strider.Pet == nil {
			t.Fatal("Wind Strider.Pet is nil, want populated")
		}
		lvl1, ok := strider.Pet.Levels[1]
		if !ok {
			t.Fatal("Wind Strider.Pet.Levels[1] missing")
		}
		if lvl1.MountMealInBattle != 2 || lvl1.MountMealInNormal != 1 {
			t.Fatalf("Wind Strider mount meal = %+v", lvl1)
		}
		if lvl1.MountAtkSpd != 350.0 || lvl1.MountPAtk != 5.45866426485439 || lvl1.MountMAtk != 5.45866426485439 {
			t.Fatalf("Wind Strider mount combat = %+v", lvl1)
		}
		if lvl1.MountBaseSpeed != 130 || lvl1.MountWaterSpeed != 70 || lvl1.MountFlySpeed != 0 {
			t.Fatalf("Wind Strider mount speed = %+v", lvl1)
		}
	})

	t.Run("clan grouping with ignored ids", func(t *testing.T) {
		table := withNoItems
		bq, ok := table.Get(18001)
		if !ok {
			t.Fatal("npc 18001 (Blood Queen) not loaded")
		}
		if len(bq.Clans) != 1 || bq.Clans[0] != "cave_servant_clan" {
			t.Fatalf("Blood Queen Clans = %v", bq.Clans)
		}
		if bq.ClanRange != 400 {
			t.Fatalf("Blood Queen ClanRange = %d, want 400", bq.ClanRange)
		}
		wantIgnored := []int{20236, 20272, 20237, 20273, 20238, 20274, 20239, 20275, 20240, 20276, 20246, 20277, 20134, 20287}
		if len(bq.IgnoredIDs) != len(wantIgnored) {
			t.Fatalf("Blood Queen IgnoredIDs = %v, want %v", bq.IgnoredIDs, wantIgnored)
		}
		for i, v := range wantIgnored {
			if bq.IgnoredIDs[i] != v {
				t.Fatalf("Blood Queen IgnoredIDs = %v, want %v", bq.IgnoredIDs, wantIgnored)
			}
		}

		gw, ok := table.Get(22001)
		if !ok {
			t.Fatal("npc 22001 (Grim Wolf) not loaded")
		}
		if len(gw.Clans) != 1 || gw.Clans[0] != "npc_clan_22001" || gw.ClanRange != 300 {
			t.Fatalf("Grim Wolf clan/range = %v/%d", gw.Clans, gw.ClanRange)
		}
		if len(gw.IgnoredIDs) != 0 {
			t.Fatalf("Grim Wolf IgnoredIDs = %v, want empty", gw.IgnoredIDs)
		}
	})

	t.Run("multi-value clan splits on semicolon", func(t *testing.T) {
		table := withNoItems
		buffalo, ok := table.Get(16013)
		if !ok {
			t.Fatal("npc 16013 (Trained Buffalo) not loaded")
		}
		want := []string{"pet_clan", "nonpet_clan"}
		if len(buffalo.Clans) != len(want) {
			t.Fatalf("Trained Buffalo Clans = %v, want %v", buffalo.Clans, want)
		}
		for i, v := range want {
			if buffalo.Clans[i] != v {
				t.Fatalf("Trained Buffalo Clans = %v, want %v", buffalo.Clans, want)
			}
		}
	})

	t.Run("teachTo profession list", func(t *testing.T) {
		table := withNoItems
		auron, ok := table.Get(30010)
		if !ok {
			t.Fatal("npc 30010 (Auron) not loaded")
		}
		want := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
		if len(auron.TeachTo) != len(want) {
			t.Fatalf("Auron TeachTo = %v, want %v", auron.TeachTo, want)
		}
		for i, v := range want {
			if auron.TeachTo[i] != v {
				t.Fatalf("Auron TeachTo = %v, want %v", auron.TeachTo, want)
			}
		}
	})

	t.Run("skills map excludes passive and race-marker skills; passives keep resolved PASSIVE entries", func(t *testing.T) {
		table := withNoItems
		sinEater, ok := table.Get(12564)
		if !ok {
			t.Fatal("npc 12564 (Sin Eater) not loaded")
		}
		// Sin Eater's <skills> block is 4121 (type="PASSIVE") and 4416
		// (type="PASSIVE", also the race-marker skill). Race markers
		// resolve race and never enter either list; PASSIVE entries that
		// resolve in the skill table go to Passives, not Skills.
		if len(sinEater.Skills) != 0 {
			t.Fatalf("Sin Eater Skills = %v, want empty", sinEater.Skills)
		}
		wantPassive := skill.Ref{ID: 4121, Level: 1}
		if len(sinEater.Passives) != 1 || sinEater.Passives[0] != wantPassive {
			t.Fatalf("Sin Eater Passives = %v, want [%v]", sinEater.Passives, wantPassive)
		}
		if sinEater.Race != npc.RaceFairy {
			t.Fatalf("Sin Eater Race = %v, want RaceFairy", sinEater.Race)
		}
	})

	t.Run("idTemplate defaults to id when absent", func(t *testing.T) {
		table := withNoItems
		gremlin, _ := table.Get(20001)
		if gremlin.TemplateID != 20001 {
			t.Fatalf("Gremlin TemplateID = %d, want 20001", gremlin.TemplateID)
		}

		hasha, ok := table.Get(50000)
		if !ok {
			t.Fatal("npc 50000 (Hasha) not loaded")
		}
		if hasha.TemplateID != 31228 {
			t.Fatalf("Hasha TemplateID = %d, want 31228", hasha.TemplateID)
		}
	})
}

func TestLoadNPCTemplatesErrors(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		content string
	}{
		{
			name:    "malformed xml",
			content: `<list><npc id="1" name="x" <set name="type" val="Monster"/></npc></list>`,
		},
		{
			name:    "missing required name attribute",
			content: `<list><npc id="1"><set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/></npc></list>`,
		},
		{
			name:    "missing required radius attribute",
			content: `<list><npc id="1" name="x"><set name="type" val="Monster"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/></npc></list>`,
		},
		{
			name:    "malformed drop chance",
			content: `<list><npc id="1" name="x"><set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/><drops><category type="DROP"><drop itemid="1" min="1" max="1" chance="oops"/></category></drops></npc></list>`,
		},
		{
			name:    "drop missing itemid",
			content: `<list><npc id="1" name="x"><set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/><drops><category type="DROP"><drop min="1" max="1" chance="100"/></category></drops></npc></list>`,
		},
		{
			name:    "teachTo class id past the ClassId.VALUES boundary",
			content: `<list><npc id="1" name="x"><set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/><teachTo classes="119"/></npc></list>`,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(dir, "fixture.xml")
			writeXMLFixture(t, path, c.content)
			if _, err := LoadNPCTemplates(dir, itemTableWithIDs([]int32{1}), emptySkillTable(), zerolog.Nop()); err == nil {
				t.Fatalf("expected an error for %s, got nil", c.name)
			}
		})
	}

	t.Run("teachTo class id at the ClassId.VALUES boundary is accepted", func(t *testing.T) {
		boundaryDir := t.TempDir()
		path := filepath.Join(boundaryDir, "fixture.xml")
		writeXMLFixture(t, path, `<list><npc id="1" name="x"><set name="type" val="Monster"/><set name="radius" val="1"/><set name="height" val="1"/><set name="pAtk" val="1"/><set name="mAtk" val="1"/><set name="pDef" val="1"/><set name="mDef" val="1"/><set name="baseDamageRange" val="0;0;1;1"/><teachTo classes="118"/></npc></list>`)
		if _, err := LoadNPCTemplates(boundaryDir, itemTableWithIDs(nil), emptySkillTable(), zerolog.Nop()); err != nil {
			t.Fatalf("LoadNPCTemplates with teachTo classes=\"118\" error: %v", err)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		empty := t.TempDir()
		if _, err := LoadNPCTemplates(empty, itemTableWithIDs(nil), emptySkillTable(), zerolog.Nop()); err == nil {
			t.Fatal("expected an error for an empty directory, got nil")
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		if _, err := LoadNPCTemplates(filepath.Join(dir, "does-not-exist"), itemTableWithIDs(nil), emptySkillTable(), zerolog.Nop()); err == nil {
			t.Fatal("expected an error for a missing directory, got nil")
		}
	})

	t.Run("missing skill table", func(t *testing.T) {
		okDir := t.TempDir()
		writeXMLFixture(t, filepath.Join(okDir, "fixture.xml"), `<list><npc id="1" name="x">`+npcRequiredSets+`</npc></list>`)
		if _, err := LoadNPCTemplates(okDir, itemTableWithIDs(nil), nil, zerolog.Nop()); err == nil {
			t.Fatal("expected an error for a missing skill table, got nil")
		}
	})

	t.Run("skill id overflowing int32 fails the load", func(t *testing.T) {
		overflowDir := t.TempDir()
		writeXMLFixture(t, filepath.Join(overflowDir, "fixture.xml"), `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="2147483648" level="1" type="PASSIVE"/></skills></npc></list>`)
		_, err := LoadNPCTemplates(overflowDir, itemTableWithIDs(nil), emptySkillTable(), zerolog.Nop())
		if err == nil {
			t.Fatal("expected an error for skill id 2147483648, got nil")
		}
		if !strings.Contains(err.Error(), "overflows int32") {
			t.Fatalf("error = %v, want overflows int32", err)
		}
	})
}

func TestLoadNPCTemplateSkills(t *testing.T) {
	known := skill.Ref{ID: 4067, Level: 5}
	passive := skill.Ref{ID: 4121, Level: 1}
	table := skillTableWith(known, passive)

	t.Run("PASSIVE entry is stored separately from the skills map", func(t *testing.T) {
		tpls := loadNPCFixture(t, `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="4121" level="1" type="PASSIVE"/></skills></npc></list>`, table)
		got, ok := tpls.Get(1)
		if !ok {
			t.Fatal("npc 1 not loaded")
		}
		if len(got.Skills) != 0 {
			t.Fatalf("Skills = %v, want empty", got.Skills)
		}
		if len(got.Passives) != 1 || got.Passives[0] != passive {
			t.Fatalf("Passives = %v, want [%v]", got.Passives, passive)
		}
	})

	t.Run("non-PASSIVE entry is stored in the skills map", func(t *testing.T) {
		tpls := loadNPCFixture(t, `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="4067" level="5" type="SKILL01_ID"/></skills></npc></list>`, table)
		got, ok := tpls.Get(1)
		if !ok {
			t.Fatal("npc 1 not loaded")
		}
		if got.Skills[4067] != 5 {
			t.Fatalf("Skills = %v, want 4067=5", got.Skills)
		}
		if len(got.Passives) != 0 {
			t.Fatalf("Passives = %v, want empty", got.Passives)
		}
	})

	t.Run("unknown id/level is skipped without failing the load", func(t *testing.T) {
		tpls := loadNPCFixture(t, `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="4067" level="5" type="SKILL01_ID"/><skill id="99999" level="1" type="SKILL01_ID"/><skill id="4121" level="99" type="PASSIVE"/></skills></npc></list>`, table)
		got, ok := tpls.Get(1)
		if !ok {
			t.Fatal("npc 1 not loaded")
		}
		if got.Skills[4067] != 5 || len(got.Skills) != 1 {
			t.Fatalf("Skills = %v, want only 4067=5", got.Skills)
		}
		if len(got.Passives) != 0 {
			t.Fatalf("Passives = %v, want empty", got.Passives)
		}
	})

	t.Run("race-marker skills never enter skills or passives", func(t *testing.T) {
		tpls := loadNPCFixture(t, `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="4416" level="13" type="PASSIVE"/><skill id="4290" level="1" type="PASSIVE"/></skills></npc></list>`, table)
		got, ok := tpls.Get(1)
		if !ok {
			t.Fatal("npc 1 not loaded")
		}
		if len(got.Skills) != 0 || len(got.Passives) != 0 {
			t.Fatalf("Skills = %v Passives = %v, want both empty", got.Skills, got.Passives)
		}
		if got.Race != npc.RaceUndead {
			t.Fatalf("Race = %v, want RaceUndead (secondary marker 4290 wins last)", got.Race)
		}
	})

	t.Run("semicolon type list puts PASSIVE in passives and other tokens in the skills map", func(t *testing.T) {
		tpls := loadNPCFixture(t, `<list><npc id="1" name="x">`+npcRequiredSets+`<skills><skill id="4067" level="5" type="PASSIVE;SKILL01_ID"/></skills></npc></list>`, table)
		got, ok := tpls.Get(1)
		if !ok {
			t.Fatal("npc 1 not loaded")
		}
		if got.Skills[4067] != 5 {
			t.Fatalf("Skills = %v, want 4067=5", got.Skills)
		}
		want := skill.Ref{ID: 4067, Level: 5}
		if len(got.Passives) != 1 || got.Passives[0] != want {
			t.Fatalf("Passives = %v, want [%v]", got.Passives, want)
		}
	})
}
