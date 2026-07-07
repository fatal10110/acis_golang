package item

import (
	"fmt"
	"sort"

	"github.com/fatal10110/acis_golang/internal/commons"
)

// Kind distinguishes the three item template categories a data file can
// define. A Template's other fields are interpreted the same way regardless
// of Kind; only construction (which XML element produced it) differs.
type Kind uint8

const (
	KindEtcItem Kind = iota
	KindArmor
	KindWeapon
)

// String returns the canonical XML spelling for k.
func (k Kind) String() string {
	switch k {
	case KindEtcItem:
		return "EtcItem"
	case KindArmor:
		return "Armor"
	case KindWeapon:
		return "Weapon"
	default:
		return fmt.Sprintf("Kind(%d)", uint8(k))
	}
}

// kindNames maps a template's XML type attribute to the Kind it selects.
var kindNames = map[string]Kind{
	"EtcItem": KindEtcItem,
	"Armor":   KindArmor,
	"Weapon":  KindWeapon,
}

// ParseKind resolves the XML type attribute of an <item> element (one of
// "EtcItem", "Armor", "Weapon") to a Kind. It returns an error for any other
// value rather than guessing.
func ParseKind(s string) (Kind, error) {
	k, ok := kindNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown template kind %q", s)
	}
	return k, nil
}

// Slot identifies which equipment position (or combination of positions,
// for paired slots) an item occupies. SlotNone means the item cannot be
// equipped. The pet slots are negative sentinels rather than bitmask bits.
type Slot int32

const (
	SlotNone      Slot = 0x0000
	SlotUnderwear Slot = 0x0001
	SlotREar      Slot = 0x0002
	SlotLEar      Slot = 0x0004
	SlotLREar     Slot = SlotREar | SlotLEar
	SlotNeck      Slot = 0x0008
	SlotRFinger   Slot = 0x0010
	SlotLFinger   Slot = 0x0020
	SlotLRFinger  Slot = SlotRFinger | SlotLFinger
	SlotHead      Slot = 0x0040
	SlotRHand     Slot = 0x0080
	SlotLHand     Slot = 0x0100
	SlotGloves    Slot = 0x0200
	SlotChest     Slot = 0x0400
	SlotLegs      Slot = 0x0800
	SlotFeet      Slot = 0x1000
	SlotBack      Slot = 0x2000
	SlotLRHand    Slot = 0x4000
	SlotFullArmor Slot = 0x8000
	SlotFace      Slot = 0x010000
	SlotAllDress  Slot = 0x020000
	SlotHair      Slot = 0x040000
	SlotHairAll   Slot = 0x080000

	SlotWolf      Slot = -100
	SlotHatchling Slot = -101
	SlotStrider   Slot = -102
	SlotBabyPet   Slot = -103
)

// slotNames maps a template's "bodypart" attribute to the Slot it selects.
var slotNames = map[string]Slot{
	"chest":           SlotChest,
	"fullarmor":       SlotFullArmor,
	"alldress":        SlotAllDress,
	"head":            SlotHead,
	"hair":            SlotHair,
	"face":            SlotFace,
	"hairall":         SlotHairAll,
	"underwear":       SlotUnderwear,
	"back":            SlotBack,
	"neck":            SlotNeck,
	"legs":            SlotLegs,
	"feet":            SlotFeet,
	"gloves":          SlotGloves,
	"chest,legs":      SlotChest | SlotLegs,
	"rhand":           SlotRHand,
	"lhand":           SlotLHand,
	"lrhand":          SlotLRHand,
	"rear;lear":       SlotREar | SlotLEar,
	"rfinger;lfinger": SlotRFinger | SlotLFinger,
	"none":            SlotNone,
	"wolf":            SlotWolf,
	"hatchling":       SlotHatchling,
	"strider":         SlotStrider,
	"babypet":         SlotBabyPet,
}

// ParseSlot resolves the "bodypart" attribute of an <item> element to a
// Slot. It returns an error for any value outside the shipped set rather
// than guessing.
func ParseSlot(s string) (Slot, error) {
	slot, ok := slotNames[s]
	if !ok {
		return 0, fmt.Errorf("item: unknown equip slot %q", s)
	}
	return slot, nil
}

// paperdollIndex maps a Slot to the equip-array position an item occupying
// it is actually stored at. Several slots share a position: a two-handed
// weapon (SlotLRHand) shows in the same position as a one-handed weapon
// (SlotRHand), and full-body armor (SlotFullArmor, SlotAllDress) shows in
// the same position as a chest piece. A face accessory and a full-hair
// accessory also share one position; the equip array has a further,
// separate position for full-hair display that no single Slot resolves to
// here, since nothing character creation grants ever occupies it.
var paperdollIndex = map[Slot]int{
	SlotUnderwear: 0,
	SlotLEar:      1,
	SlotREar:      2,
	SlotNeck:      3,
	SlotLFinger:   4,
	SlotRFinger:   5,
	SlotHead:      6,
	SlotRHand:     7,
	SlotLRHand:    7,
	SlotLHand:     8,
	SlotGloves:    9,
	SlotChest:     10,
	SlotFullArmor: 10,
	SlotAllDress:  10,
	SlotLegs:      11,
	SlotFeet:      12,
	SlotBack:      13,
	SlotFace:      14,
	SlotHairAll:   14,
	SlotHair:      15,
}

// PaperdollIndex returns the equip-array position an item occupying s is
// stored at, and whether s resolves to one at all (paired slots such as
// SlotLREar and the pet slots don't; a caller equipping into one of those
// must resolve the specific side itself).
func (s Slot) PaperdollIndex() (int, bool) {
	idx, ok := paperdollIndex[s]
	return idx, ok
}

// Template is one item's static definition as read from a shipped item data
// file: identity, equip/trade/drop flags, passive stat bonuses, a use
// precondition, and — depending on Kind — the weapon, armor, or etc-item
// detail specific to that category.
type Template struct {
	ID   int32
	Name string
	Kind Kind
	Slot Slot

	Weight         int32
	Material       MaterialType
	Duration       int32 // seconds the item lasts once used; -1 means unlimited
	ReferencePrice int32
	Crystal        CrystalType
	CrystalCount   int32

	Stackable     bool
	Sellable      bool
	Dropable      bool
	Destroyable   bool
	Tradable      bool
	Depositable   bool
	OlyRestricted bool

	DefaultAction ActionType

	// AttachedSkills are passive skills granted merely by holding or
	// wearing the item; nil when the template attaches none.
	AttachedSkills []SkillRef

	// Modifiers are the item's passive stat bonuses while equipped.
	Modifiers []StatModifier

	// UseConditions must all hold for the item to be usable; nil means the
	// template defines no precondition.
	UseConditions []UseCondition

	// Weapon, Armor and EtcItem carry the fields specific to their Kind.
	// Exactly one is non-nil, selected by Kind.
	Weapon  *WeaponDetail
	Armor   *ArmorDetail
	EtcItem *EtcItemDetail
}

// NewTemplate builds a Template from set, the merged attributes and <set>
// children of one <item> element plus the "modifiers" and "useConditions"
// values the loader packed in.
func NewTemplate(set *commons.StatSet) (*Template, error) {
	id, err := set.GetInt32("id")
	if err != nil {
		return nil, fmt.Errorf("item template: %w", err)
	}
	wrap := func(err error) error { return fmt.Errorf("item template %d: %w", id, err) }

	t := &Template{ID: id}

	if t.Name, err = set.GetString("name"); err != nil {
		return nil, wrap(err)
	}

	kindStr, err := set.GetString("type")
	if err != nil {
		return nil, wrap(err)
	}
	if t.Kind, err = ParseKind(kindStr); err != nil {
		return nil, wrap(err)
	}

	if t.Slot, err = ParseSlot(set.GetStringDefault("bodypart", "none")); err != nil {
		return nil, wrap(err)
	}

	if t.Weight, err = set.GetInt32Default("weight", 0); err != nil {
		return nil, wrap(err)
	}
	if t.Material, err = commons.GetEnumDefault(set, "material", materialTypeNames, MaterialSteel); err != nil {
		return nil, wrap(err)
	}
	if t.Duration, err = set.GetInt32Default("duration", -1); err != nil {
		return nil, wrap(err)
	}
	if t.ReferencePrice, err = set.GetInt32Default("price", 0); err != nil {
		return nil, wrap(err)
	}
	if t.Crystal, err = commons.GetEnumDefault(set, "crystal_type", crystalTypeNames, CrystalNone); err != nil {
		return nil, wrap(err)
	}
	if t.CrystalCount, err = set.GetInt32Default("crystal_count", 0); err != nil {
		return nil, wrap(err)
	}

	t.Stackable = set.GetBoolDefault("is_stackable", false)
	t.Sellable = set.GetBoolDefault("is_sellable", true)
	t.Dropable = set.GetBoolDefault("is_dropable", true)
	t.Destroyable = set.GetBoolDefault("is_destroyable", true)
	t.Tradable = set.GetBoolDefault("is_tradable", true)
	t.Depositable = set.GetBoolDefault("is_depositable", true)
	t.OlyRestricted = set.GetBoolDefault("is_oly_restricted", false)

	if t.DefaultAction, err = commons.GetEnumDefault(set, "default_action", actionTypeNames, ActionNone); err != nil {
		return nil, wrap(err)
	}

	if set.Has("item_skill") {
		raw, err := set.GetString("item_skill")
		if err != nil {
			return nil, wrap(err)
		}
		if t.AttachedSkills, err = ParseSkillRefs(raw); err != nil {
			return nil, wrap(err)
		}
	}

	if t.Modifiers, err = commons.GetList[StatModifier](set, "modifiers"); err != nil {
		return nil, wrap(err)
	}
	if t.UseConditions, err = commons.GetList[UseCondition](set, "useConditions"); err != nil {
		return nil, wrap(err)
	}

	switch t.Kind {
	case KindWeapon:
		if t.Weapon, err = NewWeaponDetail(set); err != nil {
			return nil, wrap(err)
		}
	case KindArmor:
		if t.Armor, err = NewArmorDetail(set, t.Slot); err != nil {
			return nil, wrap(err)
		}
	case KindEtcItem:
		if t.EtcItem, err = NewEtcItemDetail(set, t.DefaultAction); err != nil {
			return nil, wrap(err)
		}
	}

	return t, nil
}

// Equipable reports whether the template can occupy an equipment slot.
func (t *Template) Equipable() bool {
	return t.Slot != SlotNone && t.Kind != KindEtcItem
}

// HeroItem reports whether the template is one of the fixed hero-only
// weapon ids, or the hero circlet. This range is a client-side constant, not
// something any shipped data file flags.
func (t *Template) HeroItem() bool {
	return (t.ID >= 6611 && t.ID <= 6621) || t.ID == 6842
}

// AdenaID and AncientAdenaID are the two currency item ids the inventory
// list classifies as money rather than a generic etc-item.
const (
	AdenaID        int32 = 57
	AncientAdenaID int32 = 5575
)

// Category values group items the way the client's inventory list does:
// which icon set and sort bucket an item belongs to.
type Category int32

const (
	CategoryWeaponOrJewelry Category = 0
	CategoryArmor           Category = 1
	CategoryMoneyOrEtcItem  Category = 4
)

// SubCategory further splits Category into the client's inventory-list
// sub-groups.
type SubCategory int32

const (
	SubCategoryWeapon    SubCategory = 0
	SubCategoryArmor     SubCategory = 1
	SubCategoryAccessory SubCategory = 2
	SubCategoryMoney     SubCategory = 4
	SubCategoryOther     SubCategory = 5
)

// isJewelrySlot reports whether s is an equip slot whose item displays as
// an accessory rather than as armor, regardless of Kind.
func isJewelrySlot(s Slot) bool {
	switch s {
	case SlotNeck, SlotFace, SlotHair, SlotHairAll:
		return true
	}
	return s&(SlotLEar|SlotLFinger|SlotBack) != 0
}

// Category classifies t the way the inventory list groups items: by Kind,
// with armor-shaped jewelry (rings, earrings, necklaces, and similar
// accessory slots) reported as an accessory rather than as armor. An
// etc-item that isn't currency always reports SubCategoryOther: this method
// doesn't split out quest items into their own sub-category (see
// EtcItemDetail.IsQuestItem for that classification), since no inventory
// list caller needs the distinction yet.
func (t *Template) Category() (Category, SubCategory) {
	switch t.Kind {
	case KindWeapon:
		return CategoryWeaponOrJewelry, SubCategoryWeapon
	case KindArmor:
		if isJewelrySlot(t.Slot) {
			return CategoryWeaponOrJewelry, SubCategoryAccessory
		}
		return CategoryArmor, SubCategoryArmor
	default: // KindEtcItem
		if t.ID == AdenaID || t.ID == AncientAdenaID {
			return CategoryMoneyOrEtcItem, SubCategoryMoney
		}
		return CategoryMoneyOrEtcItem, SubCategoryOther
	}
}

// Table is an in-memory lookup of item templates keyed by id, built once at
// boot and read for the remainder of the process lifetime. The zero value
// is not usable; construct with NewTable.
type Table struct {
	templates map[int32]*Template
}

// NewTable returns a Table backed by templates, keyed by each template's ID.
// A later entry silently overwrites an earlier one with the same ID.
func NewTable(templates []*Template) *Table {
	t := &Table{templates: make(map[int32]*Template, len(templates))}
	for _, tpl := range templates {
		t.templates[tpl.ID] = tpl
	}
	return t
}

// Get returns the template with the given id, or false if none was loaded.
func (t *Table) Get(id int32) (*Template, bool) {
	tpl, ok := t.templates[id]
	return tpl, ok
}

// Len returns the number of templates in the table.
func (t *Table) Len() int {
	return len(t.templates)
}

// All returns every loaded template, ordered ascending by ID.
func (t *Table) All() []*Template {
	templates := make([]*Template, 0, len(t.templates))
	for _, tpl := range t.templates {
		templates = append(templates, tpl)
	}
	sort.Slice(templates, func(i, j int) bool { return templates[i].ID < templates[j].ID })
	return templates
}
