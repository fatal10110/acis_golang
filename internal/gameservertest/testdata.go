package gameservertest

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// Geo is an always-passable move.Geo double for suites that don't exercise
// geodata behavior.
type Geo struct{}

func (Geo) CanMove(int, int, int, int, int, int) bool { return true }
func (Geo) Height(_, _, z int) int16                  { return int16(z) }

func (Geo) FindPath(_, _ location.Location) ([]location.Location, bool) { return nil, false }
func (Geo) ValidLocation(ox, oy, oz, _, _, _ int) location.Location {
	return location.Location{X: ox, Y: oy, Z: oz}
}

// Templates builds the single class template (id 0) every suite's characters
// use: level-1 human fighter stats with the shared acquire-skill grants.
func Templates(t testing.TB) *player.TemplateTable {
	t.Helper()
	tmpl := &player.Template{
		ID:        0,
		BaseLevel: 1,
		HPTable:   []float64{80},
		MPTable:   []float64{30},
		CPTable:   []float64{32},
		Spawns:    []location.Location{{X: 10, Y: 20, Z: 30}},
		RunSpeed:  120,
		WalkSpeed: 60,
		SwimSpeed: 50,
		Skills: []player.SkillGrant{
			{SkillID: 3, Level: 1, MinLevel: 5, Cost: 50},
			{SkillID: 900001, Level: 1, MinLevel: 50, Cost: 0},
		},
	}
	table, err := player.NewTemplateTable(map[int]*player.Template{0: tmpl})
	if err != nil {
		t.Fatalf("build template table: %v", err)
	}
	return table
}

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
			ID:        item.CrystalD.ItemID(),
			Name:      "D-grade Crystal",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
		},
		{
			ID:        955,
			Name:      "Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:        6575,
			Name:      "Blessed Scroll: Enchant Weapon (D)",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemBlessedScrollEnchantWeapon, Handler: "EnchantScrolls"},
		},
		{
			ID:       40,
			Name:     "Tunic",
			Kind:     item.KindArmor,
			Slot:     item.SlotChest,
			Duration: -1,
			Crystal:  item.CrystalD,
			Armor:    &item.ArmorDetail{Type: item.ArmorMagic},
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
			ID:        9001,
			Name:      "Quest Token",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{Type: item.EtcItemQuest},
		},
		{
			ID:        9500,
			Name:      "Heavy Ingot",
			Kind:      item.KindEtcItem,
			Duration:  -1,
			Stackable: true,
			EtcItem:   &item.EtcItemDetail{},
			Weight:    10,
		},
		{
			ID:          91000,
			Name:        "Wolf Collar",
			Kind:        item.KindEtcItem,
			Duration:    -1,
			Stackable:   true,
			Destroyable: true,
			EtcItem:     &item.EtcItemDetail{Handler: "SummonItems"},
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
