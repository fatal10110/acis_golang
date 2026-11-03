package quest

import (
	"context"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q421"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The flutes of Little Wing's Big Adventure (Q421_LittleWingsBigAdventure):
// the quest registers the object id of the enchanted flute and gives a
// stronger flute of the same enchant back. A scenario cannot enchant a held
// flute nor read an object id, so this Boot test pins what rides on them.
const (
	q421Name = "Q421_LittleWingsBigAdventure"
	q421Path = "quest.Q421_LittleWingsBigAdventure"

	q421Cronos = 30610
	q421Mimyu  = 30747

	q421FluteOfWind = 3500
	q421FluteOfStar = 3501
	q421StrongStar  = 3501 + 922
)

// q421FluteWorld boots a level 45 character holding flutes (template id to
// enchant level, one each), over Cronos and Mimyu. A positive registered
// starts the quest at cond 3 with iCond 100 and the object id of the flute
// of that template as summonOid. A flute of enchant -1 is registered then
// lost: it is not in the inventory. It returns the flutes' object ids.
func q421FluteWorld(t *testing.T, registered int32, flutes map[int32]int) (*dialogWorld, map[int32]int32) {
	t.Helper()
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{3500, 3501, 3502, 4422, 4423, 4424} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Dragonflute", Kind: item.KindEtcItem, Duration: -1, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	templates = append(templates, &item.Template{
		ID: 4325, Name: "Fairy Leaf", Kind: item.KindEtcItem, Duration: -1, Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
	})
	pages := chatPages(questPages(q421Name, map[string][]string{
		"30610-02.htm": nil, "30610-03.htm": nil, "30610-04.htm": {"30610-06.htm"}, "30610-05.htm": nil,
		"30747-16.htm": nil, "30747-17.htm": nil, "30747-18.htm": nil,
	}), q421Cronos, q421Mimyu)
	held := map[int32]int32{}
	w := bootSeamQuest(t, 45, pages, func(srv *gameservertest.Server, objID int32) {
		for id, enchant := range flutes {
			held[id] = srv.GiveItem(t, objID, id, 1)
			if enchant < 0 {
				continue
			}
			if _, err := srv.DB.ExecContext(context.Background(), "UPDATE items SET enchant_level = ? WHERE object_id = ?", enchant, held[id]); err != nil {
				t.Fatalf("enchant the flute: %v", err)
			}
		}
		if registered > 0 {
			insertJournal(t, srv.DB,
				journalRow{objID, q421Name, "<state>", val("STARTED")},
				journalRow{objID, q421Name, "<cond>", val("3")},
				journalRow{objID, q421Name, "iCond", val("100")},
				journalRow{objID, q421Name, "summonOid", val(strconv.Itoa(int(held[registered])))})
		}
		for id, enchant := range flutes {
			if enchant < 0 {
				if _, err := srv.DB.ExecContext(context.Background(), "DELETE FROM items WHERE object_id = ?", held[id]); err != nil {
					t.Fatalf("lose the flute: %v", err)
				}
			}
		}
	},
		gameservertest.WithItemTemplates(item.NewTable(templates)),
		gameservertest.WithNPCs(npc.NewTable([]*npc.Template{
			gameservertest.FolkTemplate("Folk", q421Cronos), gameservertest.FolkTemplate("Folk", q421Mimyu),
		})),
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q421Cronos: script.KindFolk, q421Mimyu: script.KindFolk},
			[]script.Listing{{Path: q421Path}}, script.Catalog{q421Path: q421.New}),
	)
	w.roles[w.player] = "player"
	w.spawnFolk(t, "cronos", q421Cronos, 40, 0, 0)
	w.spawnFolk(t, "mimyu", q421Mimyu, 50, 0, 0)
	return w, held
}

// fluteEnchant is the enchant level of the character's flute of template id,
// from the saved item rows.
func fluteEnchant(t *testing.T, w *dialogWorld, id int32) (int, bool) {
	t.Helper()
	w.srv.FlushItems(t)
	var enchant int
	err := w.srv.DB.QueryRowContext(context.Background(), "SELECT enchant_level FROM items WHERE owner_id = ? AND item_id = ?", w.player, id).Scan(&enchant)
	return enchant, err == nil
}

// TestQ421CronosStartsOnlyForOneStrongFlute: Cronos starts the quest for a
// single flute enchanted to 55 or more, and the quest remembers that flute's
// object id; a weaker flute, or a second one, is turned away.
func TestQ421CronosStartsOnlyForOneStrongFlute(t *testing.T) {
	t.Parallel()
	t.Run("strong flute", func(t *testing.T) {
		t.Parallel()
		w, held := q421FluteWorld(t, 0, map[int32]int{q421FluteOfStar: 55})
		if got := w.talk(t, "cronos"); !shows(got, "30610-04.htm") {
			t.Fatalf("Cronos showed %q, want 30610-04.htm", got)
		}
		if got := w.send(t, encodeBypass("Quest "+q421Name+" 30610-06.htm")); !shows(got, "30610-05.htm") {
			t.Fatalf("starting showed %q, want 30610-05.htm", got)
		}
		vars := w.srv.QuestVars(t, w.player, q421Name)
		want := map[string]string{"<state>": "STARTED", "<cond>": "1", "iCond": "1", "summonOid": strconv.Itoa(int(held[q421FluteOfStar]))}
		for k, v := range want {
			if vars[k] != v {
				t.Fatalf("%s = %q, want %q (all: %v)", k, vars[k], v, vars)
			}
		}
	})
	t.Run("weak flute", func(t *testing.T) {
		t.Parallel()
		w, _ := q421FluteWorld(t, 0, map[int32]int{q421FluteOfStar: 54})
		if got := w.talk(t, "cronos"); !shows(got, "30610-03.htm") {
			t.Fatalf("Cronos showed %q, want 30610-03.htm", got)
		}
	})
	t.Run("two flutes", func(t *testing.T) {
		t.Parallel()
		w, _ := q421FluteWorld(t, 0, map[int32]int{q421FluteOfStar: 60, q421FluteOfWind: 60})
		if got := w.talk(t, "cronos"); !shows(got, "30610-02.htm") {
			t.Fatalf("Cronos showed %q, want 30610-02.htm", got)
		}
	})
}

// TestQ421MimyuGivesTheStrongerFluteForTheRegisteredOne: with the fairy
// acknowledged (iCond 100) and no summon out, Mimyu trades the registered
// flute for the flute 922 ids on with the same enchant and ends the quest; a
// registered flute that is gone gets the curse page, and a second flute the
// page of the extra flute.
func TestQ421MimyuGivesTheStrongerFluteForTheRegisteredOne(t *testing.T) {
	t.Parallel()
	t.Run("registered flute", func(t *testing.T) {
		t.Parallel()
		w, _ := q421FluteWorld(t, q421FluteOfStar, map[int32]int{q421FluteOfStar: 61})
		if got := w.talk(t, "mimyu"); !shows(got, "30747-16.htm") {
			t.Fatalf("Mimyu showed %q, want 30747-16.htm", got)
		}
		if _, ok := fluteEnchant(t, w, q421FluteOfStar); ok {
			t.Fatal("the registered flute is still held")
		}
		if enchant, ok := fluteEnchant(t, w, q421StrongStar); !ok || enchant != 61 {
			t.Fatalf("stronger flute held %v with enchant %d, want held with 61", ok, enchant)
		}
		if vars := w.srv.QuestVars(t, w.player, q421Name); len(vars) != 0 {
			t.Fatalf("the finished quest keeps %v", vars)
		}
	})
	t.Run("registered flute lost", func(t *testing.T) {
		t.Parallel()
		w, _ := q421FluteWorld(t, q421FluteOfStar, map[int32]int{q421FluteOfStar: -1, q421FluteOfWind: 60})
		if got := w.talk(t, "mimyu"); !shows(got, "30747-18.htm") {
			t.Fatalf("Mimyu showed %q, want 30747-18.htm", got)
		}
		if _, ok := fluteEnchant(t, w, q421FluteOfWind); !ok {
			t.Fatal("the other flute is gone")
		}
		if vars := w.srv.QuestVars(t, w.player, q421Name); vars["iCond"] != "100" {
			t.Fatalf("quest rows %v, want the quest kept", vars)
		}
	})
	t.Run("two flutes", func(t *testing.T) {
		t.Parallel()
		w, _ := q421FluteWorld(t, q421FluteOfStar, map[int32]int{q421FluteOfStar: 61, q421FluteOfWind: 60})
		if got := w.talk(t, "mimyu"); !shows(got, "30747-17.htm") {
			t.Fatalf("Mimyu showed %q, want 30747-17.htm", got)
		}
	})
}
