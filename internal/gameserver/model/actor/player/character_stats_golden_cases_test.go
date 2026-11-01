package player

import (
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/itemcontainer"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/stat"
)

// The cases below mirror StatPipelineProbe.java in summon/testdata/oracle:
// a case added or changed on one side needs its twin on the other, and the
// golden is regenerated with that directory's run.sh.

// goldenTemplate is the combat template with the M.Def, M.Atk and run speed
// the probe's player reads.
func goldenTemplate() *Template {
	tmpl := combatTemplate()
	tmpl.MDef = 60
	tmpl.MAtk = 20
	tmpl.RunSpeed = 100
	return tmpl
}

// goldenPlayer is a character of class and level with the instances worn.
func goldenPlayer(id int32, class, level int, items *item.Table, worn ...*item.Instance) *Character {
	tmpl := goldenTemplate()
	tmpl.ID = class
	c := liveCharacter(id, tmpl, items, worn...)
	c.CharLevel = level
	return c
}

const (
	goldenFighterClass = 0  // Human Fighter
	goldenMageClass    = 10 // Human Mystic
	goldenPenaltyLevel = 40
)

// goldenPlayerBuiltins reads every builtin on its own, with nothing worn and
// no func attached.
func goldenPlayerBuiltins(out map[string]float64) {
	c := goldenPlayer(20, goldenFighterClass, goldenPenaltyLevel, combatItems())
	out["builtin_patk"] = rawPAtk(c)
	out["builtin_matk"] = rawMAtk(c)
	out["builtin_pdef"] = rawPDef(c)
	out["builtin_mdef"] = rawMDef(c)
	out["builtin_runspeed"] = c.RunSpeed()
}

// outfitPiece is the template slot and paperdoll position of a worn piece.
type outfitPiece struct {
	slot      item.Slot
	paperdoll int
}

var outfitPieces = map[string]outfitPiece{
	"head":     {item.SlotHead, itemcontainer.Head},
	"chest":    {item.SlotChest, itemcontainer.Chest},
	"fullbody": {item.SlotFullArmor, itemcontainer.Chest},
	"alldress": {item.SlotAllDress, itemcontainer.Chest},
	"legs":     {item.SlotLegs, itemcontainer.Legs},
	"gloves":   {item.SlotGloves, itemcontainer.Gloves},
	"feet":     {item.SlotFeet, itemcontainer.Feet},
	"lfinger":  {item.SlotLFinger, itemcontainer.LFinger},
	"rfinger":  {item.SlotRFinger, itemcontainer.RFinger},
	"lear":     {item.SlotLEar, itemcontainer.LEar},
	"rear":     {item.SlotREar, itemcontainer.REar},
	"neck":     {item.SlotNeck, itemcontainer.Neck},
}

const (
	goldenArmor   = "head,chest,legs,gloves,feet"
	goldenJewelry = "lfinger,rfinger,lear,rear,neck"
)

// goldenPlayerPenalties reads P.Def and M.Def of a level-40 fighter or mage
// wearing the comma-separated pieces.
func goldenPlayerPenalties(out map[string]float64) {
	cases := []struct {
		name   string
		mage   bool
		pieces string
	}{
		{"head", false, "head"},
		{"chest_fighter", false, "chest"},
		{"chest_mage", true, "chest"},
		{"chest_legs_mage", true, "chest,legs"},
		{"fullbody_fighter", false, "fullbody"},
		{"fullbody_mage", true, "fullbody"},
		{"alldress_fighter", false, "alldress"},
		{"legs_fighter", false, "legs"},
		{"legs_mage", true, "legs"},
		{"gloves_feet", false, "gloves,feet"},
		{"armor_fighter", false, goldenArmor},
		{"armor_mage", true, goldenArmor},
		{"fingers", false, "lfinger,rfinger"},
		{"ears", false, "lear,rear"},
		{"neck", false, "neck"},
		{"jewelry", false, goldenJewelry},
		{"all_fighter", false, goldenArmor + "," + goldenJewelry},
		{"all_mage", true, goldenArmor + "," + goldenJewelry},
	}
	for i, tc := range cases {
		var templates []*item.Template
		var worn []*item.Instance
		for j, token := range strings.Split(tc.pieces, ",") {
			piece, ok := outfitPieces[token]
			if !ok {
				panic("unknown outfit piece " + token)
			}
			id := int32(100 + j)
			templates = append(templates, &item.Template{ID: id, Kind: item.KindArmor, Slot: piece.slot, Armor: &item.ArmorDetail{}})
			worn = append(worn, &item.Instance{
				ObjectID: 5000 + id, TemplateID: id,
				Location: item.LocationPaperdoll, LocationData: piece.paperdoll,
			})
		}
		class := goldenFighterClass
		if tc.mage {
			class = goldenMageClass
		}
		c := goldenPlayer(int32(30+i), class, goldenPenaltyLevel, item.NewTable(templates), worn...)
		out["penalty_"+tc.name+"_pdef"] = rawPDef(c)
		out["penalty_"+tc.name+"_mdef"] = rawMDef(c)
	}
}

var goldenGrades = []struct {
	name  string
	grade item.CrystalType
}{
	{"none", item.CrystalNone},
	{"d", item.CrystalD},
	{"c", item.CrystalC},
	{"b", item.CrystalB},
	{"a", item.CrystalA},
	{"s", item.CrystalS},
}

var goldenWeaponTypes = []struct {
	name string
	kind item.WeaponType
}{
	{"sword", item.WeaponSword},
	{"bow", item.WeaponBow},
	{"bigsword", item.WeaponBigSword},
	{"bigblunt", item.WeaponBigBlunt},
	{"dual", item.WeaponDual},
	{"dualfist", item.WeaponDualFist},
}

// enchantedPlayer is a level-1 fighter with an Enchant func on s owned by an
// unworn instance of tmpl at the enchant level. The instance is only the
// func's owner: whether the item is worn never reaches the func.
func enchantedPlayer(tmpl *item.Template, enchant int, s stat.Stat) *Character {
	items := item.NewTable([]*item.Template{tmpl})
	inst := &item.Instance{ObjectID: 900, TemplateID: tmpl.ID, EnchantLevel: enchant}
	c := goldenPlayer(60, goldenFighterClass, 1, items)
	owner := effect.ModOwnerItem(effect.ItemOwner{Inst: inst, Tmpl: tmpl})
	c.AddStatFuncs([]effect.Mod{{Stat: s, Op: effect.OpEnchant, Owner: owner}})
	return c
}

func goldenWeapon(grade item.CrystalType, kind item.WeaponType) *item.Template {
	return &item.Template{
		ID: 50, Kind: item.KindWeapon, Slot: item.SlotRHand, Crystal: grade,
		Weapon: &item.WeaponDetail{Type: kind},
	}
}

func goldenArmorPiece(grade item.CrystalType) *item.Template {
	return &item.Template{ID: 50, Kind: item.KindArmor, Slot: item.SlotChest, Crystal: grade, Armor: &item.ArmorDetail{}}
}

// goldenPlayerEnchants reads the Enchant func by crystal grade, weapon type
// and enchant level, for P.Atk, M.Atk, P.Def and M.Def.
func goldenPlayerEnchants(out map[string]float64) {
	for _, g := range goldenGrades {
		for _, enchant := range []int{2, 7} {
			for _, w := range goldenWeaponTypes {
				c := enchantedPlayer(goldenWeapon(g.grade, w.kind), enchant, stat.PowerAttack)
				out[fmt.Sprintf("enchant_patk_%s_%s_e%d", g.name, w.name, enchant)] = rawPAtk(c)
			}
			c := enchantedPlayer(goldenWeapon(g.grade, item.WeaponSword), enchant, stat.MagicAttack)
			out[fmt.Sprintf("enchant_matk_%s_e%d", g.name, enchant)] = rawMAtk(c)
		}
	}
	for _, enchant := range []int{0, 3, 4} {
		c := enchantedPlayer(goldenWeapon(item.CrystalS, item.WeaponSword), enchant, stat.PowerAttack)
		out[fmt.Sprintf("enchant_patk_s_sword_e%d", enchant)] = rawPAtk(c)
	}
	for _, enchant := range []int{2, 7} {
		armor := goldenArmorPiece(item.CrystalA)
		out[fmt.Sprintf("enchant_pdef_e%d", enchant)] = rawPDef(enchantedPlayer(armor, enchant, stat.PowerDefence))
		out[fmt.Sprintf("enchant_mdef_e%d", enchant)] = rawMDef(enchantedPlayer(armor, enchant, stat.MagicDefence))
		// A non-weapon item leaves P.Atk alone.
		out[fmt.Sprintf("enchant_patk_armor_e%d", enchant)] = rawPAtk(enchantedPlayer(armor, enchant, stat.PowerAttack))
	}
}
