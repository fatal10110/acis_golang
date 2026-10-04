package player

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/event"
)

// TestReportDeathDropPvPZone pins the PvP-zone gate of the reference's
// onDieDropItem, `(!isInsideZone(ZoneId.PVP) || pk == null)`: a death a
// player caused inside a PvP zone drops nothing, even for a PK at the
// threshold, while a monster kill inside one still rolls, and so does a
// PK's death to a player outside one.
func TestReportDeathDropPvPZone(t *testing.T) {
	karma := DeathDropRates{Chance: 70, Equip: 40, EquipWeapon: 10, Item: 50, Limit: 10}
	monster := DeathDropRates{Chance: 5, Equip: 25, EquipWeapon: 5, Item: 70, Limit: 3}
	for _, tt := range []struct {
		name         string
		karma, pks   int
		byPlayer     bool
		inPvP        bool
		wantRatesFor string
	}{
		{name: "PK killed by a player inside a PvP zone", karma: 240, pks: 5, byPlayer: true, inPvP: true},
		{name: "PK killed by a player outside a PvP zone", karma: 240, pks: 5, byPlayer: true, wantRatesFor: "karma"},
		{name: "PK killed by a monster inside a PvP zone", karma: 240, pks: 5, inPvP: true, wantRatesFor: "karma"},
		{name: "player killed by a monster inside a PvP zone", inPvP: true, wantRatesFor: "monster"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &Character{ID: 1, CharLevel: 5, KarmaPoints: tt.karma, PKKills: tt.pks}
			c.deathDrop = DeathDropRules{Karma: karma, Monster: monster, KarmaPKLimit: 5}
			c.SetInPvPZone(tt.inPvP)
			rec := recordEvents(c)

			var killer attackable.Combatant = deathPenaltyKiller{}
			if tt.byPlayer {
				killer = &Character{ID: 2}
			}
			c.reportDeathDrop(killer)

			got := event.Of[event.DeathItemDrop](rec)
			var want []event.DeathItemDrop
			switch tt.wantRatesFor {
			case "karma":
				want = []event.DeathItemDrop{{Chance: 70, EquipChance: 40, WeaponChance: 10, ItemChance: 50, Limit: 10}}
			case "monster":
				want = []event.DeathItemDrop{{Chance: 5, EquipChance: 25, WeaponChance: 5, ItemChance: 70, Limit: 3}}
			}
			if len(got) != len(want) || (len(want) == 1 && got[0] != want[0]) {
				t.Fatalf("death drop reports = %+v, want %+v", got, want)
			}
		})
	}
}
