package main

import "github.com/fatal10110/acis_golang/internal/config"

// partyRange is players.properties PartyRange: how near a party member must
// be to share a kill or loot, body to body; -1 is unlimited.
type partyRange int

func loadPartyRange(paths gameServerPaths) (partyRange, error) {
	props, err := config.LoadFile(paths.PlayersConfigPath)
	if err != nil {
		return 0, err
	}
	n, err := props.Int("PartyRange", 1500)
	return partyRange(n), err
}
