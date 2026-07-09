package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// MaterialType is the substance an item is made of, used for on-hit sound
// and visual effects.
type MaterialType uint8

const (
	MaterialSteel MaterialType = iota
	MaterialFineSteel
	MaterialCotton
	MaterialBloodSteel
	MaterialBronze
	MaterialSilver
	MaterialGold
	MaterialMithril
	MaterialOriharukon
	MaterialPaper
	MaterialWood
	MaterialCloth
	MaterialLeather
	MaterialBone
	MaterialHorn
	MaterialDamascus
	MaterialAdamantaite
	MaterialChrysolite
	MaterialCrystal
	MaterialLiquid
	MaterialScaleOfDragon
	MaterialDyestuff
	MaterialCobweb
)

// String returns the canonical XML spelling for m.
func (m MaterialType) String() string {
	name, ok := materialTypeStrings[m]
	if !ok {
		return fmt.Sprintf("MaterialType(%d)", uint8(m))
	}
	return name
}

var materialTypeStrings = map[MaterialType]string{
	MaterialSteel:         "STEEL",
	MaterialFineSteel:     "FINE_STEEL",
	MaterialCotton:        "COTTON",
	MaterialBloodSteel:    "BLOOD_STEEL",
	MaterialBronze:        "BRONZE",
	MaterialSilver:        "SILVER",
	MaterialGold:          "GOLD",
	MaterialMithril:       "MITHRIL",
	MaterialOriharukon:    "ORIHARUKON",
	MaterialPaper:         "PAPER",
	MaterialWood:          "WOOD",
	MaterialCloth:         "CLOTH",
	MaterialLeather:       "LEATHER",
	MaterialBone:          "BONE",
	MaterialHorn:          "HORN",
	MaterialDamascus:      "DAMASCUS",
	MaterialAdamantaite:   "ADAMANTAITE",
	MaterialChrysolite:    "CHRYSOLITE",
	MaterialCrystal:       "CRYSTAL",
	MaterialLiquid:        "LIQUID",
	MaterialScaleOfDragon: "SCALE_OF_DRAGON",
	MaterialDyestuff:      "DYESTUFF",
	MaterialCobweb:        "COBWEB",
}

// materialTypeNames maps a template's "material" attribute to the
// MaterialType it selects.
var materialTypeNames = commons.ReverseMap(materialTypeStrings)

// CrystalType is the enchant-crystallization grade an item belongs to; NONE
// means the item cannot be crystallized.
type CrystalType uint8

const (
	CrystalNone CrystalType = iota
	CrystalD
	CrystalC
	CrystalB
	CrystalA
	CrystalS
)

// String returns the canonical XML spelling for c.
func (c CrystalType) String() string {
	name, ok := crystalTypeStrings[c]
	if !ok {
		return fmt.Sprintf("CrystalType(%d)", uint8(c))
	}
	return name
}

var crystalTypeStrings = map[CrystalType]string{
	CrystalNone: "NONE",
	CrystalD:    "D",
	CrystalC:    "C",
	CrystalB:    "B",
	CrystalA:    "A",
	CrystalS:    "S",
}

var crystalTypeNames = commons.ReverseMap(crystalTypeStrings)

// ActionType is the client-side action bound to double-clicking or using an
// item, selecting the icon and default handler the client offers for it.
type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionCalc
	ActionCallSkill
	ActionCapsule
	ActionCreateMPCC
	ActionDice
	ActionEquip
	ActionFishingShot
	ActionHarvest
	ActionHideName
	ActionKeepExp
	ActionNickColor
	ActionPeel
	ActionRecipe
	ActionSeed
	ActionShowAdventurerGuideBook
	ActionShowHTML
	ActionShowSSQStatus
	ActionSkillMaintain
	ActionSkillReduce
	ActionSoulshot
	ActionSpiritshot
	ActionStartQuest
	ActionSummonSoulshot
	ActionSummonSpiritshot
	ActionXmasOpen
)

// String returns the canonical XML spelling for a.
func (a ActionType) String() string {
	name, ok := actionTypeStrings[a]
	if !ok {
		return fmt.Sprintf("ActionType(%d)", uint8(a))
	}
	return name
}

var actionTypeStrings = map[ActionType]string{
	ActionNone:                    "none",
	ActionCalc:                    "calc",
	ActionCallSkill:               "call_skill",
	ActionCapsule:                 "capsule",
	ActionCreateMPCC:              "create_mpcc",
	ActionDice:                    "dice",
	ActionEquip:                   "equip",
	ActionFishingShot:             "fishingshot",
	ActionHarvest:                 "harvest",
	ActionHideName:                "hide_name",
	ActionKeepExp:                 "keep_exp",
	ActionNickColor:               "nick_color",
	ActionPeel:                    "peel",
	ActionRecipe:                  "recipe",
	ActionSeed:                    "seed",
	ActionShowAdventurerGuideBook: "show_adventurer_guide_book",
	ActionShowHTML:                "show_html",
	ActionShowSSQStatus:           "show_ssq_status",
	ActionSkillMaintain:           "skill_maintain",
	ActionSkillReduce:             "skill_reduce",
	ActionSoulshot:                "soulshot",
	ActionSpiritshot:              "spiritshot",
	ActionStartQuest:              "start_quest",
	ActionSummonSoulshot:          "summon_soulshot",
	ActionSummonSpiritshot:        "summon_spiritshot",
	ActionXmasOpen:                "xmas_open",
}

var actionTypeNames = commons.ReverseMap(actionTypeStrings)
