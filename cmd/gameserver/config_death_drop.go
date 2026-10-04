package main

import (
	"github.com/fatal10110/acis_golang/internal/config"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// Item templates no PK death drops when players.properties lists none.
var (
	defaultDeathDropKeptItems = []int{1147, 425, 1146, 461, 10, 2368, 7, 6, 2370, 2369}
	defaultDeathDropPetItems  = []int{2375, 3500, 3501, 3502, 4422, 4423, 4424, 4425, 6648, 6649, 6650}
)

// loadDeathDropRules reads what a player's death may cost it in items: the
// rates from server.properties, the PK threshold, the game-master switch
// and the items no death drops from players.properties.
func loadDeathDropRules(paths gameServerPaths) (player.DeathDropRules, error) {
	server, err := config.LoadFile(paths.ConfigPath)
	if err != nil {
		return player.DeathDropRules{}, err
	}
	players, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return player.DeathDropRules{}, err
	}
	sf := config.NewFields(server, "death drop rates")
	rules := player.DeathDropRules{
		Monster: player.DeathDropRates{
			Chance:      sf.Int("PlayerRateDrop", 5),
			Item:        sf.Int("PlayerRateDropItem", 70),
			Equip:       sf.Int("PlayerRateDropEquip", 25),
			EquipWeapon: sf.Int("PlayerRateDropEquipWeapon", 5),
			Limit:       sf.Int("PlayerDropLimit", 3),
		},
		Karma: player.DeathDropRates{
			Chance:      sf.Int("KarmaRateDrop", 70),
			Item:        sf.Int("KarmaRateDropItem", 50),
			Equip:       sf.Int("KarmaRateDropEquip", 40),
			EquipWeapon: sf.Int("KarmaRateDropEquipWeapon", 10),
			Limit:       sf.Int("KarmaDropLimit", 10),
		},
	}
	if err := sf.Err(); err != nil {
		return player.DeathDropRules{}, err
	}
	pf := config.NewFields(players, "death drop rules")
	rules.GMDrops = pf.Bool("CanGMDropEquipment", false)
	rules.KarmaPKLimit = pf.Int("MinimumPKRequiredToDrop", 5)
	if err := pf.Err(); err != nil {
		return player.DeathDropRules{}, err
	}
	for _, list := range []struct {
		key string
		def []int
	}{
		{"ListOfNonDroppableItemsForPK", defaultDeathDropKeptItems},
		{"ListOfPetItems", defaultDeathDropPetItems},
	} {
		ids, err := players.Ints(list.key, list.def)
		if err != nil {
			return player.DeathDropRules{}, err
		}
		for _, id := range ids {
			rules.Kept = append(rules.Kept, int32(id))
		}
	}
	return rules, nil
}
