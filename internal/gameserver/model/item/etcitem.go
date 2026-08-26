package item

import (
	"fmt"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// EtcItemType further classifies a KindEtcItem template beyond the generic
// "etc item" bucket: what kind of consumable, currency, or quest object it
// is.
type EtcItemType uint8

const (
	EtcItemNone EtcItemType = iota
	EtcItemArrow
	EtcItemPotion
	EtcItemScrollEnchantWeapon
	EtcItemScrollEnchantArmor
	EtcItemScroll
	EtcItemRecipe
	EtcItemMaterial
	EtcItemPetCollar
	EtcItemCastleGuard
	EtcItemLotto
	EtcItemRaceTicket
	EtcItemDye
	EtcItemSeed
	EtcItemCrop
	EtcItemMatureCrop
	EtcItemHarvest
	EtcItemSeed2
	EtcItemTicketOfLord
	EtcItemLure
	EtcItemBlessedScrollEnchantWeapon
	EtcItemBlessedScrollEnchantArmor
	EtcItemCoupon
	EtcItemElixir
	EtcItemShot
	EtcItemHerb
	EtcItemQuest
)

// String returns the canonical XML spelling for e.
func (e EtcItemType) String() string {
	name, ok := etcItemTypeStrings[e]
	if !ok {
		return fmt.Sprintf("EtcItemType(%d)", uint8(e))
	}
	return name
}

var etcItemTypeStrings = map[EtcItemType]string{
	EtcItemNone:                       "NONE",
	EtcItemArrow:                      "ARROW",
	EtcItemPotion:                     "POTION",
	EtcItemScrollEnchantWeapon:        "SCRL_ENCHANT_WP",
	EtcItemScrollEnchantArmor:         "SCRL_ENCHANT_AM",
	EtcItemScroll:                     "SCROLL",
	EtcItemRecipe:                     "RECIPE",
	EtcItemMaterial:                   "MATERIAL",
	EtcItemPetCollar:                  "PET_COLLAR",
	EtcItemCastleGuard:                "CASTLE_GUARD",
	EtcItemLotto:                      "LOTTO",
	EtcItemRaceTicket:                 "RACE_TICKET",
	EtcItemDye:                        "DYE",
	EtcItemSeed:                       "SEED",
	EtcItemCrop:                       "CROP",
	EtcItemMatureCrop:                 "MATURECROP",
	EtcItemHarvest:                    "HARVEST",
	EtcItemSeed2:                      "SEED2",
	EtcItemTicketOfLord:               "TICKET_OF_LORD",
	EtcItemLure:                       "LURE",
	EtcItemBlessedScrollEnchantWeapon: "BLESS_SCRL_ENCHANT_WP",
	EtcItemBlessedScrollEnchantArmor:  "BLESS_SCRL_ENCHANT_AM",
	EtcItemCoupon:                     "COUPON",
	EtcItemElixir:                     "ELIXIR",
	EtcItemShot:                       "SHOT",
	EtcItemHerb:                       "HERB",
	EtcItemQuest:                      "QUEST",
}

var etcItemTypeNames = commons.ReverseMap(etcItemTypeStrings)

// shotActions is the set of default actions that reclassify a template as
// EtcItemShot regardless of its own declared etcitem_type.
var shotActions = map[ActionType]bool{
	ActionSoulshot:         true,
	ActionSpiritshot:       true,
	ActionSummonSoulshot:   true,
	ActionSummonSpiritshot: true,
}

// ParseEtcItemType resolves a template's "etcitem_type" attribute to an
// EtcItemType. It returns an error for any value outside the shipped set
// rather than guessing.
func ParseEtcItemType(s string) (EtcItemType, error) {
	t, ok := etcItemTypeNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown etc item type %q", s)
	}
	return t, nil
}

// EtcItemDetail is the etc-item-specific data a KindEtcItem Template
// carries; nil for every other Kind.
type EtcItemDetail struct {
	Type EtcItemType

	// Handler names the use-item behavior this template invokes; empty
	// when the template defines none.
	Handler string

	SharedReuseGroup int32
	ReuseDelay       int32
}

// NewEtcItemDetail builds the EtcItemDetail for a KindEtcItem template
// declaring etcType, handler, sharedReuseGroup and reuseDelay, with
// defaultAction, the template's own default action. A soulshot/spiritshot
// default action always reports EtcItemShot, overriding whatever
// etcitem_type the data declares.
func NewEtcItemDetail(etcType EtcItemType, handler string, sharedReuseGroup, reuseDelay int32, defaultAction ActionType) *EtcItemDetail {
	if shotActions[defaultAction] {
		etcType = EtcItemShot
	}
	return &EtcItemDetail{
		Type:             etcType,
		Handler:          handler,
		SharedReuseGroup: sharedReuseGroup,
		ReuseDelay:       reuseDelay,
	}
}

// IsQuestItem reports whether d classifies its item as a quest item.
func (d *EtcItemDetail) IsQuestItem() bool {
	return d.Type == EtcItemQuest
}
