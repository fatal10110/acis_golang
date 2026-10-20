package quest

import (
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q216"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

const (
	q216Path                  = "quest.Q216_TrialOfTheGuildsman"
	q216Name                  = "Q216_TrialOfTheGuildsman"
	q216AntID                 = 20079
	q216PinterOrdersID        = 3135
	q216AmberBeadID           = 3136
	q216AmberLumpID           = 3137
	q216DwarvenFighterClassID = 53
	q216ScavengerClassID      = 54
	q216ArtisanClassID        = 56
)

// bootQ216Ants boots a level 36 character of class at cond 5 of the trial,
// holding Pinter's instructions, spawns an ant spoiled by spoiler (0:
// nobody; -1: the character itself) and has the character kill it.
func bootQ216Ants(t *testing.T, class int, spoiler int32) (*gameservertest.Server, int32) {
	t.Helper()
	// The dwarven fighter is the parent class both occupations need.
	dwarf, tmpl := gameservertest.ClassTemplate(), gameservertest.ClassTemplate()
	dwarf.ID, tmpl.ID = q216DwarvenFighterClassID, class
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{q216PinterOrdersID, q216AmberBeadID, q216AmberLumpID} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	srv := gameservertest.Boot(t,
		gameservertest.WithClassCharacter("Gatherer", 36, 0, class),
		gameservertest.WithClassTemplates(dwarf, tmpl),
		gameservertest.WithWantChars(1),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q216AntID: script.KindHostile},
			[]script.Listing{{Path: q216Path}},
			script.Catalog{q216Path: q216.New},
		),
	)
	objID := srv.SoleObjectID(t)
	for _, r := range [][2]string{{"<state>", "STARTED"}, {"<cond>", "5"}} {
		if _, err := srv.DB.Exec("INSERT INTO character_quests (charId,name,var,value) VALUES (?,?,?,?)", objID, q216Name, r[0], r[1]); err != nil {
			t.Fatal(err)
		}
	}
	srv.GiveItem(t, objID, q216PinterOrdersID, 1)
	enterWorld(t, srv)

	x, y, z := srv.PlayerPosition(t, objID)
	ant := srv.SpawnHostileNPCTemplateAt(t, proofMonster(q216AntID), location.Location{X: x + 40, Y: y, Z: z})
	switch spoiler {
	case 0:
	case -1:
		if !ant.SpoilPool().Mark(objID) {
			t.Fatal("a fresh ant is already spoiled")
		}
	default:
		if !ant.SpoilPool().Mark(spoiler) {
			t.Fatal("a fresh ant is already spoiled")
		}
	}
	killOnPlayerQueue(t, srv, objID, ant)
	return srv, objID
}

// TestQ216AntBeadsFollowTheSpoiler pins the ant drop of the trial: a
// Scavenger gets ten Amber Beads off an ant it spoiled itself and five off
// one nobody, or someone else, spoiled; an Artisan gets five whoever
// spoiled it. No drop reaches the seventy, so no Amber Lump is given.
func TestQ216AntBeadsFollowTheSpoiler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		class   int
		spoiler int32
		want    int
	}{
		{"scavenger spoiled it", q216ScavengerClassID, -1, 10},
		{"scavenger, nobody spoiled it", q216ScavengerClassID, 0, 5},
		{"scavenger, another spoiled it", q216ScavengerClassID, 0x7fff0001, 5},
		{"artisan spoiled it", q216ArtisanClassID, -1, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, objID := bootQ216Ants(t, tc.class, tc.spoiler)
			if got := srv.PlayerItemCount(t, objID, q216AmberBeadID); got != tc.want {
				t.Fatalf("Amber Beads after the kill = %d, want %d", got, tc.want)
			}
			if got := srv.PlayerItemCount(t, objID, q216AmberLumpID); got != 0 {
				t.Fatalf("Amber Lumps after the kill = %d, want 0", got)
			}
		})
	}
}
