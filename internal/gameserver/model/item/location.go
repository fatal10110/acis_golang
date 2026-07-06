package item

import "fmt"

// Location identifies which container (or equip slot) an item instance
// occupies, matching the items table's "loc" column values exactly.
type Location int

const (
	LocationVoid Location = iota
	LocationInventory
	LocationPaperdoll
	LocationWarehouse
	LocationClanWarehouse
	LocationPet
	LocationPetEquip
	LocationFreight
)

// locationNames maps a Location to its items.loc column spelling.
var locationNames = [...]string{
	LocationVoid:          "VOID",
	LocationInventory:     "INVENTORY",
	LocationPaperdoll:     "PAPERDOLL",
	LocationWarehouse:     "WAREHOUSE",
	LocationClanWarehouse: "CLANWH",
	LocationPet:           "PET",
	LocationPetEquip:      "PET_EQUIP",
	LocationFreight:       "FREIGHT",
}

// String returns the items.loc column spelling for l.
func (l Location) String() string {
	if int(l) < 0 || int(l) >= len(locationNames) {
		return fmt.Sprintf("Location(%d)", int(l))
	}
	return locationNames[l]
}

// ParseLocation resolves an items.loc column value to a Location. It
// returns an error for any value outside the shipped set rather than
// guessing.
func ParseLocation(s string) (Location, error) {
	for l, name := range locationNames {
		if name == s {
			return Location(l), nil
		}
	}
	return 0, fmt.Errorf("item: unknown location %q", s)
}
