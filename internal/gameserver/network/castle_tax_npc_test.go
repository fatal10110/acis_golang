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

// TestNpcCastle pins Npc.getCastle as the tax paths read it: the template
// residence (the first castle whose npcs list holds the id), overridden by
// a maker's spawn-time residence (MultiSpawn.doSpawn -> Npc.setResidence):
// a castle id gives that castle, a siegable clan hall id gives no castle,
// and anything else leaves the template's.
func TestNpcCastle(t *testing.T) {
	const (
		townNPC  = 30001 // Gludio's (castle 1) npcs list
		strayNPC = 99999 // no castle's
	)
	newCastle := func(id int, npcs ...int) *castledata.Castle {
		c, err := castledata.NewCastle(castledata.CastleAttrs{ID: id, Alias: "c" + strconv.Itoa(id), Name: "C", NPCs: npcs}, nil, nil, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	data, err := castledata.NewTable([]*castledata.Castle{newCastle(1, townNPC), newCastle(7, townNPC)})
	if err != nil {
		t.Fatal(err)
	}
	newHall := func(id int, siegable bool) *hallmodel.Hall {
		h, err := hallmodel.NewHall(hallmodel.HallAttrs{ID: id, Alias: "h" + strconv.Itoa(id), Name: "H", Description: "d", Town: "t", Siegable: siegable, Tax: residence.Tax{}}, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	halls, err := hallmodel.NewTable([]*hallmodel.Hall{newHall(21, false), newHall(34, true)})
	if err != nil {
		t.Fatal(err)
	}
	l := &GameClientLink{castles: castle.NewManager(data, clan.NewTable(), nil, nil, zerolog.Nop()), clanHallData: halls}

	for _, tt := range []struct {
		name      string
		npcID     int
		spawnTime string
		want      int // 0 for none
	}{
		{"template castle", townNPC, "", 1},
		{"no castle", strayNPC, "", 0},
		{"maker castle", strayNPC, "siege_warfare_start(7)", 7},
		{"maker castle over the template's", townNPC, "pc_siege_warfare_start(7)", 7},
		{"siegable hall clears the castle", townNPC, "agit_defend_warfare_start(34)", 0},
		{"plain hall keeps the template's", townNPC, "agit_defend_warfare_start(21)", 1},
		{"unknown residence keeps the template's", townNPC, "siege_warfare_start(99)", 1},
		{"door_open keeps the template's", townNPC, "door_open(7)", 1},
	} {
		t.Run(tt.name, func(t *testing.T) {
			inst := &npc.Instance{Template: &npc.Template{ID: tt.npcID}}
			if tt.spawnTime != "" {
				inst.Maker = &spawn.Maker{Name: "m", SpawnTime: tt.spawnTime}
			}
			c, ok := l.npcCastle(inst)
			got := 0
			if ok {
				got = c.ID
			}
			if got != tt.want {
				t.Fatalf("npcCastle = %d, want %d", got, tt.want)
			}
		})
	}

	if _, ok := (&GameClientLink{}).npcCastle(&npc.Instance{Template: &npc.Template{ID: townNPC}}); ok {
		t.Fatal("a server without castles gives an NPC a castle")
	}
}
