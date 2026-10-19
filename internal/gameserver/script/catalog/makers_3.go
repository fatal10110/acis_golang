package catalog

import (
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/maker"
)

// makers3 is the maker catalog of batch 3 of 3 (#214): the boss-lair makers, by npcmaker type.
func makers3() script.MakerCatalog {
	return script.MakerCatalog{
		"benom_maker":                        maker.Benom,
		"default_use_db_maker_for_frintessa": nil,
		"freya_deacon_keeper_maker":          maker.FreyaDeaconKeeper,
		"frintessa_evilate_maker":            maker.FrintezzaEvilate,
		"ice_fairy_sirr_maker":               nil,
		"rhamphorhynchus_maker":              nil,
		"sailren_dummy_maker":                maker.SailrenDummy,
		"sailren_maker":                      nil,
		"sculpture_garden_maker":             maker.SculptureGarden,
		"sculpture_ice_fairy_maker":          maker.SculptureIceFairy,
		"statue_of_shilen_maker":             maker.StatueOfShilen,
		"tyrannosaurus_maker":                nil,
		"velociraptor_maker":                 maker.Velociraptor,
	}
}
