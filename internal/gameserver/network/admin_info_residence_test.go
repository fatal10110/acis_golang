package network

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/castle"
	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/residence"
	castledata "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/castle"
	hallmodel "github.com/fatal10110/acis_golang/internal/gameserver/model/residence/clanhall"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/spawn"
	"github.com/rs/zerolog"
)

// TestNpcResidenceName pins the residence name the //info 2 page prints
// (AdminInfo.sendGeneralInfos page 2, Npc.getResidence): the template's
// residence (the first castle listing the id, else the first clan hall),
// overridden by a maker's spawn-time residence (MultiSpawn.doSpawn ->
// Npc.setResidence(String)) when it names a castle or a siegable hall; a
// plain hall or an unknown id keeps the template's.
func TestNpcResidenceName(t *testing.T) {
	const (
		townNPC  = 30001 // Gludio's (castle 1) npcs list
		hallNPC  = 30002 // the plain hall's npcs list only
		strayNPC = 99999 // no residence's
	)
	newCastle := func(id int, name string, npcs ...int) *castledata.Castle {
		c, err := castledata.NewCastle(castledata.CastleAttrs{ID: id, Alias: "c" + strconv.Itoa(id), Name: name, NPCs: npcs}, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	data, err := castledata.NewTable([]*castledata.Castle{newCastle(1, "Gludio", townNPC), newCastle(7, "Aden")})
	if err != nil {
		t.Fatal(err)
	}
	newHall := func(id int, name string, siegable bool, npcs ...int) *hallmodel.Hall {
		h, err := hallmodel.NewHall(hallmodel.HallAttrs{ID: id, Alias: "h" + strconv.Itoa(id), Name: name, Description: "d", Town: "t", Siegable: siegable, Tax: residence.Tax{}, NPCs: npcs}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{newHall(21, "Moonstone Hall", false, hallNPC, townNPC), newHall(34, "Devastated Castle", true)})
	if err != nil {
		t.Fatal(err)
	}
	l := newBareGameClientLink(t, GameClientLinkConfig{Castles: castle.NewManager(data, clan.NewTable(), nil, nil, zerolog.Nop()), ClanHallData: halls})

	for _, tt := range []struct {
		name      string
		npcID     int
		spawnTime string
		want      string
	}{
		{"template castle", townNPC, "", "Gludio"},
		{"template hall", hallNPC, "", "Moonstone Hall"},
		{"no residence", strayNPC, "", "none"},
		{"maker castle", strayNPC, "siege_warfare_start(7)", "Aden"},
		{"maker castle over the template's", townNPC, "pc_siege_warfare_start(7)", "Aden"},
		{"maker siegable hall over the template's castle", townNPC, "agit_defend_warfare_start(34)", "Devastated Castle"},
		{"maker plain hall keeps the template's castle", townNPC, "agit_defend_warfare_start(21)", "Gludio"},
		{"maker plain hall on no residence", strayNPC, "agit_defend_warfare_start(21)", "none"},
		{"unknown id keeps the template's castle", townNPC, "siege_warfare_start(99)", "Gludio"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inst := &npc.Instance{Template: &npc.Template{ID: tt.npcID}}
			if tt.spawnTime != "" {
				inst.Maker = &spawn.Maker{Name: "m", SpawnTime: tt.spawnTime}
			}
			if got := l.npcResidenceName(inst); got != tt.want {
				t.Fatalf("npcResidenceName = %q, want %q", got, tt.want)
			}
		})
	}
}
