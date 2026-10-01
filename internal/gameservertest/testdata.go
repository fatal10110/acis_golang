package gameservertest

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/augmentation"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/henna"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/recipe"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Geo is an always-passable move.Geo double for suites that don't exercise
// geodata behavior.
type Geo struct{}

func (Geo) CanMove(int, int, int, int, int, int) bool { return true }

// Height answers the probe height itself, clamped to the int16 range a
// geodata height holds.
func (Geo) Height(_, _, z int) int16 {
	if z > math.MaxInt16 {
		return math.MaxInt16
	}
	if z < math.MinInt16 {
		return math.MinInt16
	}
	return int16(z)
}

// GateGeo is an always-passable Geo until Block closes every straight-line
// walk, so a suite can start a move then fire the in-flight blocked path.
type GateGeo struct {
	Geo
	blocked bool
}

// Block makes later CanMove checks fail.
func (g *GateGeo) Block() { g.blocked = true }

func (g *GateGeo) CanMove(int, int, int, int, int, int) bool { return !g.blocked }

func (g *GateGeo) CanFly(int, int, int, float64, int, int, int) bool { return !g.blocked }

func (Geo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (Geo) ValidLocation(_, _, _, tx, ty, tz int) location.Location {
	return location.Location{X: tx, Y: ty, Z: tz}
}
func (Geo) Walkable(int, int, int) bool { return true }

func (g Geo) CanFly(ox, oy, oz int, _ float64, tx, ty, tz int) bool {
	return g.CanMove(ox, oy, oz, tx, ty, tz)
}

func (g Geo) ValidFlyLocation(ox, oy, oz int, _ float64, tx, ty, tz int) location.Location {
	return g.ValidLocation(ox, oy, oz, tx, ty, tz)
}

// SpawnZ is the height the class template spawns new characters at.
const SpawnZ = 30

// FlatGeo is Geo over one flat floor at Z. Geo, like a region with no
// geodata, answers the probe height itself, so a ground walk, which reads
// each step's floor from above the walker (curZ + 2*CellHeight), climbs with
// every step it takes; FlatGeo's walks keep to the floor.
type FlatGeo struct {
	Geo
	Z int
}

func (g FlatGeo) Height(int, int, int) int16 { return int16(g.Z) }

// Templates builds the single class template (id 0) every suite's characters
// use: level-1 human fighter stats with the shared acquire-skill grants.
func Templates(t testing.TB) *player.TemplateTable {
	t.Helper()
	return templatesWith(t, ClassTemplate())
}

// templatesWith is Templates with class 0 replaced by class0 and extra
// added, each replacing a default of its id.
func templatesWith(t testing.TB, class0 *player.Template, extra ...*player.Template) *player.TemplateTable {
	t.Helper()
	tmpls := map[int]*player.Template{
		0:  class0,
		1:  fighterLineTemplate(1),
		2:  fighterLineTemplate(2),
		88: duelistTemplate(),
	}
	for _, tmpl := range extra {
		tmpls[tmpl.ID] = tmpl
	}
	table, err := player.NewTemplateTable(tmpls)
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

// HennaTemplates returns the dye symbols behavior suites resolve on restore.
func HennaTemplates(t testing.TB) *henna.Table {
	t.Helper()
	return henna.NewTable([]henna.Henna{
		{SymbolID: 1, DyeID: 4445, DrawPrice: 37000, STR: 1, CON: -3, Classes: []int{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}},
		{SymbolID: 2, DyeID: 4446, DrawPrice: 37000, STR: 1, DEX: -3, Classes: []int{1, 4, 7, 11, 15, 19, 22, 26, 29, 32, 35, 39, 42, 45, 47, 50, 54, 56}},
	})
}

// RecipeTemplates is the recipe table the behavior suites boot with: rows
// copied from the shipped recipes.xml, the dwarven mk_wooden_arrow (id 1)
// and mk_broad_sword (id 2) and the common mk_lesser_healing_potion (id 686).
func RecipeTemplates() *recipe.Table {
	return recipe.NewTable([]recipe.Recipe{
		{
			ID: 1, Alias: "mk_wooden_arrow", ItemID: 1666, Level: 1, MPCost: 30, SuccessRate: 100, Dwarven: true,
			Materials: []recipe.Ingredient{{ItemID: 1864, Count: 4}, {ItemID: 1869, Count: 2}},
			Product:   recipe.Ingredient{ItemID: 17, Count: 500},
		},
		{
			ID: 2, Alias: "mk_broad_sword", ItemID: 1786, Level: 1, MPCost: 30, SuccessRate: 100, Dwarven: true,
			Materials: []recipe.Ingredient{{ItemID: 2005, Count: 1}, {ItemID: 1869, Count: 18}, {ItemID: 1870, Count: 18}},
			Product:   recipe.Ingredient{ItemID: 3, Count: 1},
		},
		{
			ID: 686, Alias: "mk_lesser_healing_potion", ItemID: 6926, Level: 1, MPCost: 30, SuccessRate: 100,
			Materials: []recipe.Ingredient{{ItemID: 6908, Count: 2}},
			Product:   recipe.Ingredient{ItemID: 1060, Count: 1},
		},
	})
}

// The fixture classes keep every base attribute at 0, so a base of 1 would
// finalize P.Atk. and M.Atk. below 1, and the getters' int truncation would
// leave the fixture characters hitting and casting for nothing. fixturePAtk
// and fixtureMAtk are the shipped human fighter's bases
// (classes/humanFighter.xml), which finalize to 1 at low levels; the small
// defence bases keep the suites' hits in the same range they had before the
// getters truncated.
const (
	fixturePAtk = 4
	fixturePDef = 4
	fixtureMAtk = 6
	fixtureMDef = 4
)

// ClassTemplate is the shared human-fighter class template every seeded
// character selects, usable without a testing context.
func ClassTemplate() *player.Template {
	return &player.Template{
		ID:                   0,
		BaseLevel:            1,
		HPTable:              []float64{80},
		MPTable:              []float64{30},
		CPTable:              []float64{32},
		PAtk:                 fixturePAtk,
		PDef:                 fixturePDef,
		MAtk:                 fixtureMAtk,
		MDef:                 fixtureMDef,
		Spawns:               []location.Location{{X: 10, Y: 20, Z: SpawnZ}},
		RunSpeed:             120,
		WalkSpeed:            60,
		SwimSpeed:            50,
		SafeFallHeightFemale: 270,
		SafeFallHeightMale:   250,
		Skills: []player.SkillGrant{
			{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
			{SkillID: 900001, Level: 1, MinLevel: 50, Cost: 0},
		},
	}
}

// fighterLineTemplate fills the intermediate warrior-profession templates
// the duelist line requires the table to carry, with the shipped human
// fighter base attributes the symbol windows report.
func fighterLineTemplate(id int) *player.Template {
	return &player.Template{
		ID:                   id,
		BaseLevel:            1,
		STR:                  40,
		CON:                  43,
		DEX:                  30,
		INT:                  21,
		WIT:                  11,
		MEN:                  25,
		HPTable:              []float64{80},
		MPTable:              []float64{30},
		CPTable:              []float64{32},
		PAtk:                 fixturePAtk,
		PDef:                 fixturePDef,
		MAtk:                 fixtureMAtk,
		MDef:                 fixtureMDef,
		Spawns:               []location.Location{{X: 10, Y: 20, Z: SpawnZ}},
		RunSpeed:             120,
		WalkSpeed:            60,
		SwimSpeed:            50,
		SafeFallHeightFemale: 270,
		SafeFallHeightMale:   250,
	}
}

// duelistTemplate is the enchant-eligible third-profession template the
// skill-enchant flows select; it shares the fighter spawn point so both
// classes land in the same world region.
func duelistTemplate() *player.Template {
	return &player.Template{
		ID:                   88,
		BaseLevel:            76,
		HPTable:              []float64{80},
		MPTable:              []float64{30},
		CPTable:              []float64{32},
		PAtk:                 fixturePAtk,
		PDef:                 fixturePDef,
		MAtk:                 fixtureMAtk,
		MDef:                 fixtureMDef,
		Spawns:               []location.Location{{X: 10, Y: 20, Z: SpawnZ}},
		RunSpeed:             120,
		WalkSpeed:            60,
		SwimSpeed:            50,
		SafeFallHeightFemale: 270,
		SafeFallHeightMale:   250,
	}
}

// TwoSkillScrollID is a synthetic ItemSkills etc-item with two non-instant
// attached skills. Three shipped ItemSkills templates carry multiple
// attached skills (8612/8613/8614), but every one of their skills is
// isPotion, so ResolveAICastSkills filters them out. This fixture is the
// only way to drive the later-skill next-intention path.
const TwoSkillScrollID int32 = 9700

// UnlockableKeyID is a non-potion ItemSkills fixture for target-validation tests.
const UnlockableKeyID int32 = 9701

// PetResurrectionScrollID is the Blessed Scroll of Resurrection for Pets
// (datapack item 6387): a ScrollsOfResurrection item whose skill, 2179,
// consumes the scroll itself.
const PetResurrectionScrollID int32 = 6387

// SelfConsumingScrollID is the Petrification Scroll (datapack item 8379): an
// ItemSkills scroll whose skill, 2239, also names the scroll as its own
// consume item, so one use destroys the carrier and then the skill's item.
const SelfConsumingScrollID int32 = 8379

// StormbringerID is a C-grade sword (datapack item 72, 916 crystals): the
// lowest grade a life stone augments.
const StormbringerID int32 = 72

// LifeStone46ID is the no-grade level 46 life stone (datapack item 8723).
const LifeStone46ID int32 = 8723

// GemstoneDID is Gemstone D (datapack item 2130), the gemstone augmenting a
// C- or B-grade weapon consumes.
const GemstoneDID int32 = 2130

// FormalWearID is the full-body formal dress (bodypart alldress) that forbids
// item and skill use while worn.
const FormalWearID int32 = 6408

// ItemTemplates builds the item catalog shared by the behavior suites: adena,
// potions, shots, a weapon, crystals, enchant scrolls, escape scrolls, quest
// and summon items.
func ItemTemplates() *item.Table {
	return item.NewTable([]*item.Template{
		{
			ID:          item.AdenaID,
			Name:        "Adena",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{},
		},
		{
			ID:          20,
			Name:        "Potion",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemPotion},
		},
		{
			ID:             1463,
			Name:           "Soulshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2150, Level: 1}},
		},
		{
			ID:             2509,
			Name:           "Spiritshot: No Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalD,
			DefaultAction:  item.ActionSpiritshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2047, Level: 1}},
		},
		{
			ID:             1464,
			Name:           "Soulshot: C Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			Crystal:        item.CrystalC,
			DefaultAction:  item.ActionSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "SoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2151, Level: 1}},
		},
		{
			ID:             6645,
			Name:           "Beast Soulshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			DefaultAction:  item.ActionSummonSoulshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSoulShots"},
			AttachedSkills: []item.SkillRef{{ID: 2033, Level: 1}},
		},
		{
			ID:             6646,
			Name:           "Beast Spiritshot",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			DefaultAction:  item.ActionSummonSpiritshot,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemShot, Handler: "BeastSpiritShots"},
			AttachedSkills: []item.SkillRef{{ID: 2008, Level: 1}},
		},
		{
			ID:           30,
			Name:         "Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Dropable:     true,
			Tradable:     true,
			Destroyable:  true,
			Depositable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			ID:            14,
			Name:          "Bow",
			Kind:          item.KindWeapon,
			Slot:          item.SlotLRHand,
			Duration:      -1,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			Weapon: &item.WeaponDetail{
				Type:       item.WeaponBow,
				MPConsume:  1,
				ReuseDelay: 1500,
			},
		},
		{
			ID:            17,
			Name:          "Wooden Arrow",
			Kind:          item.KindEtcItem,
			Slot:          item.SlotLHand,
			Duration:      -1,
			Stackable:     true,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			EtcItem:       &item.EtcItemDetail{Type: item.EtcItemArrow},
		},
		{
			// Demonic Sword Zariche, the reference server's cursed weapon
			// item id, kept minimal so cursed-weapon-exclusion tests can
			// seed a real ground drop without loading cursedWeapons.xml.
			ID:           8190,
			Name:         "Demonic Sword Zariche",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalS,
			CrystalCount: 10,
			Dropable:     true,
			Destroyable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			// A time-limited shadow weapon: Duration minutes of mana decay
			// while equipped (Duration 5 → 300 seconds of initial mana).
			ID:           7884,
			Name:         "Shadow Sword",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     5,
			Crystal:      item.CrystalD,
			CrystalCount: 10,
			Destroyable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
		{
			ID:          955,
			Name:        "Scroll: Enchant Weapon (D)",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:          6575,
			Name:        "Blessed Scroll: Enchant Weapon (D)",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemBlessedScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:          40,
			Name:        "Tunic",
			Kind:        item.KindArmor,
			Slot:        item.SlotChest,
			Duration:    -1,
			Crystal:     item.CrystalD,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			Depositable: true,
			Armor:       &item.ArmorDetail{Type: item.ArmorMagic},
		},
		{
			ID:            FormalWearID,
			Name:          "Formal Wear",
			Kind:          item.KindArmor,
			Slot:          item.SlotAllDress,
			Duration:      -1,
			Destroyable:   true,
			DefaultAction: item.ActionEquip,
			Armor:         &item.ArmorDetail{Type: item.ArmorNone},
		},
		{
			ID:             1060,
			Name:           "Lesser Healing Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 10000, SharedReuseGroup: 8},
			AttachedSkills: []item.SkillRef{{ID: 2031, Level: 1}},
			UseConditions: []item.UseCondition{{
				Root:      item.Condition{Kind: "player", Attrs: map[string]string{"flying": "False"}},
				MessageID: int32(serverpackets.SystemMessageS1CannotBeUsed),
				AddName:   true,
			}},
		},
		{
			ID:             736,
			Name:           "Scroll: Escape",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2013, Level: 1}},
		},
		{
			ID:             PetResurrectionScrollID,
			Name:           "Blessed Scroll of Resurrection for Pets",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ScrollsOfResurrection", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2179, Level: 1}},
		},
		{
			ID:             5593,
			Name:           "SP Scroll: Low Grade",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2167, Level: 1}},
		},
		{
			// Synthetic two-skill ItemSkills template. Three shipped
			// ItemSkills templates carry multiple attached skills
			// (8612/8613/8614), but every one of their skills is isPotion,
			// so ResolveAICastSkills filters them out. This fixture is the
			// only way to drive the later-skill next-intention path.
			ID:             TwoSkillScrollID,
			Name:           "Two-Skill Scroll",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2014, Level: 1}, {ID: 2015, Level: 1}},
		},
		{
			ID:             SelfConsumingScrollID,
			Name:           "Petrification Scroll",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2239, Level: 1}},
		},
		{
			ID:             UnlockableKeyID,
			Name:           "Unlockable Key",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemScroll, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2236, Level: 1}},
		},
		{
			ID:        9001,
			Name:      "Quest Token",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemQuest},
		},
		{
			ID:             728,
			Name:           "Mana Potion",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			Depositable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", ReuseDelay: 10000, SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2279, Level: 2}},
		},
		{
			ID:             5589,
			Name:           "Energy Stone",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemPotion, Handler: "ItemSkills", SharedReuseGroup: -1},
			AttachedSkills: []item.SkillRef{{ID: 2165, Level: 1}},
		},
		{
			ID:             8600,
			Name:           "Herb of Life",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			EtcItem:        &item.EtcItemDetail{Type: item.EtcItemHerb, Handler: "ItemSkills"},
			AttachedSkills: []item.SkillRef{{ID: 2278, Level: 1}},
		},
		{
			ID:            6535,
			Name:          "Fishing Shot",
			Kind:          item.KindEtcItem,
			Duration:      -1,
			Stackable:     true,
			DefaultAction: item.ActionFishingShot,
			EtcItem:       &item.EtcItemDetail{Type: item.EtcItemShot},
		},
		{
			ID:          9500,
			Name:        "Heavy Ingot",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Destroyable: true,
			Tradable:    true,
			EtcItem:     &item.EtcItemDetail{},
			Weight:      10,
		},
		{
			ID:          9600,
			Name:        "Wolf Collar",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Destroyable: true,
			// The datapack's wolf collar (2375) is a PET_COLLAR and sets no
			// is_tradable, so it trades like any item.
			Tradable: true,
			EtcItem:  &item.EtcItemDetail{Type: item.EtcItemPetCollar, Handler: "SummonItems"},
		},
		{
			// The wyvern control item the mount flow uses; the summon-item
			// table maps it to the wyvern summon type.
			ID:          9601,
			Name:        "Wyvern Collar",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: "SummonItems"},
		},
		{
			// A decorative-summon item (Christmas Tree); the summon-item
			// table maps it to the decorative summon type.
			ID:          9602,
			Name:        "Decoration Kit",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: "SummonItems"},
		},
		{
			// Wolf's food, the PetFoods-handler item the pet feeding path
			// dispatches through its hardcoded feed-skill mapping.
			ID:             2515,
			Name:           "Wolf Food",
			Kind:           item.KindEtcItem,
			Duration:       -1,
			Stackable:      true,
			Dropable:       true,
			Tradable:       true,
			Destroyable:    true,
			EtcItem:        &item.EtcItemDetail{Handler: "PetFoods"},
			AttachedSkills: []item.SkillRef{{ID: 2048, Level: 1}},
			Weight:         40,
		},
		{
			ID:           StormbringerID,
			Name:         "Stormbringer",
			Kind:         item.KindWeapon,
			Slot:         item.SlotRHand,
			Duration:     -1,
			Crystal:      item.CrystalC,
			CrystalCount: 916,
			Dropable:     true,
			Tradable:     true,
			Sellable:     true,
			Destroyable:  true,
			Depositable:  true,
			Weapon:       &item.WeaponDetail{Type: item.WeaponSword, SoulshotCount: 1, SpiritshotCount: 1},
		},
		{
			ID:          LifeStone46ID,
			Name:        "Life Stone: level 46",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemMaterial},
		},
		{
			ID:          GemstoneDID,
			Name:        "Gemstone D",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Dropable:    true,
			Tradable:    true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Type: item.EtcItemMaterial},
		},
	})
}

// HTMLCache writes the given pages into a temp dir and loads them through the
// production HTML cache loader.
func HTMLCache(t testing.TB, pages map[string]string) *cache.HTML {
	t.Helper()
	dir := t.TempDir()
	for name, content := range pages {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	html, err := cache.LoadHTML(dir)
	if err != nil {
		t.Fatalf("LoadHTML: %v", err)
	}
	return html
}

// seedCharacter inserts a character through the real SQL character store so
// it is selectable at the next CharSelectInfo. Returns the persisted
// character (with its object id).
func (s *Server) seedCharacter(tb testing.TB, account, name string, level, sp int) *player.Character {
	tb.Helper()
	tmpl, ok := s.templates.Get(0)
	if !ok {
		tb.Fatal("missing test class template")
	}
	ch, err := player.NewCharacter(s.ids.nextID(), tmpl, account, name, 1, 0, 0, player.SexMale)
	if err != nil {
		tb.Fatalf("seed character: %v", err)
	}
	ch.CharLevel = level
	ch.SP = sp
	if err := s.Chars.Create(context.Background(), ch); err != nil {
		tb.Fatalf("seed character store: %v", err)
	}
	return ch
}

// giveItem inserts an inventory item through the real SQL item store,
// mirroring what a reward or purchase path persists.
func (s *Server) giveItem(tb testing.TB, ownerID, templateID, count int32) int32 {
	tb.Helper()
	objectID := s.NewObjectID()
	inst := item.Instance{
		ObjectID:   objectID,
		TemplateID: templateID,
		OwnerID:    ownerID,
		Count:      int(count),
		Location:   item.LocationInventory,
	}
	if err := s.Items.Create(context.Background(), ownerID, inst); err != nil {
		tb.Fatalf("give item: %v", err)
	}
	return objectID
}

// Augmentation skill ids of AugmentationTable, by color: every life stone
// level's one blue, purple and red option names one of them at level 1.
const (
	AugmentBlueSkillID   int32 = 3203
	AugmentPurpleSkillID int32 = 3243
	AugmentRedSkillID    int32 = 3256
)

// augmentationStatNames are a color block's 13 stat tables in the shipped
// order.
var augmentationStatNames = [...]string{"pDef", "mDef", "maxHp", "maxMp", "maxCp", "pAtk", "mAtk", "regHp", "regMp", "regCp", "rEvas", "accCombat", "rCrit"}

// AugmentationStatValue is AugmentationTable's solo value of stat table
// index stat (0 pDef .. 12 rCrit) in color block color; its combined value
// is half of it. Every level reads the same value.
func AugmentationStatValue(color, stat int) float32 {
	return float32(10*(stat+1) + color)
}

// AugmentationTable builds a small augmentation table shaped like the
// shipped one: four color blocks of the 13 stat tables, each holding one
// value (AugmentationStatValue) every level reads, and per life stone level
// one blue, purple and red skill option, ids 14561 + level*178 + 0, 1, 2.
func AugmentationTable(t testing.TB) *augmentation.Table {
	t.Helper()
	var groups []augmentation.StatGroup
	for color := range 4 {
		var stats []augmentation.Stat
		for i, name := range augmentationStatNames {
			v := AugmentationStatValue(color, i)
			stat, err := augmentation.NewStat(name, []float32{v}, []float32{v / 2})
			if err != nil {
				t.Fatalf("augmentation stat: %v", err)
			}
			stats = append(stats, stat)
		}
		group, err := augmentation.NewStatGroup(color, stats)
		if err != nil {
			t.Fatalf("augmentation stat group: %v", err)
		}
		groups = append(groups, group)
	}
	var skills []augmentation.Skill
	for level := range 10 {
		for i, c := range []struct {
			color   string
			skillID int32
		}{{"blue", AugmentBlueSkillID}, {"purple", AugmentPurpleSkillID}, {"red", AugmentRedSkillID}} {
			s, err := augmentation.NewSkill(14561+level*178+i, c.skillID, 1, c.color)
			if err != nil {
				t.Fatalf("augmentation skill: %v", err)
			}
			skills = append(skills, s)
		}
	}
	table, err := augmentation.NewTable(groups, skills)
	if err != nil {
		t.Fatalf("augmentation table: %v", err)
	}
	return table
}
