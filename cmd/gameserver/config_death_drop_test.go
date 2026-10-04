package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
)

// TestLoadDeathDropRules reads the death drop rates from server.properties
// and the PK threshold, the game-master switch and both kept item lists
// from players.properties; files without the keys keep the defaults.
func TestLoadDeathDropRules(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	server := write("server.properties", `
PlayerDropLimit = 1
PlayerRateDrop = 2
PlayerRateDropItem = 3
PlayerRateDropEquip = 4
PlayerRateDropEquipWeapon = 6
KarmaDropLimit = 7
KarmaRateDrop = 8
KarmaRateDropItem = 9
KarmaRateDropEquip = 11
KarmaRateDropEquipWeapon = 12
`)
	players := write("players.properties", `
CanGMDropEquipment = True
MinimumPKRequiredToDrop = 3
ListOfPetItems = 2375,3500
ListOfNonDroppableItemsForPK = 57, 1147;425
`)
	empty := write("empty.properties", "")

	got, err := loadDeathDropRules(gameServerPaths{ConfigPath: server, PlayersConfigPath: players})
	if err != nil {
		t.Fatalf("loadDeathDropRules() error = %v", err)
	}
	want := player.DeathDropRules{
		Monster:      player.DeathDropRates{Chance: 2, Item: 3, Equip: 4, EquipWeapon: 6, Limit: 1},
		Karma:        player.DeathDropRates{Chance: 8, Item: 9, Equip: 11, EquipWeapon: 12, Limit: 7},
		KarmaPKLimit: 3,
		GMDrops:      true,
		Kept:         []int32{57, 1147, 425, 2375, 3500},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadDeathDropRules() = %+v, want %+v", got, want)
	}

	got, err = loadDeathDropRules(gameServerPaths{ConfigPath: empty, PlayersConfigPath: empty})
	if err != nil {
		t.Fatalf("loadDeathDropRules(defaults) error = %v", err)
	}
	want = player.DeathDropRules{
		Monster:      player.DeathDropRates{Chance: 5, Item: 70, Equip: 25, EquipWeapon: 5, Limit: 3},
		Karma:        player.DeathDropRates{Chance: 70, Item: 50, Equip: 40, EquipWeapon: 10, Limit: 10},
		KarmaPKLimit: 5,
		Kept: []int32{
			1147, 425, 1146, 461, 10, 2368, 7, 6, 2370, 2369,
			2375, 3500, 3501, 3502, 4422, 4423, 4424, 4425, 6648, 6649, 6650,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("loadDeathDropRules(defaults) = %+v, want %+v", got, want)
	}

	bad := write("bad.properties", "ListOfPetItems = 2375,collar\n")
	if _, err := loadDeathDropRules(gameServerPaths{ConfigPath: empty, PlayersConfigPath: bad}); err == nil {
		t.Fatal("loadDeathDropRules(malformed list) error = nil, want one")
	}
}
