package npcs

import (
	"context"
	"database/sql"
	"strconv"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The quests a subclass reads: the two that open the add, and the one a
// class change ends.
const (
	fatesWhisper   = "Q234_FatesWhisper"
	mimirsElixir   = "Q235_MimirsElixir"
	repentYourSins = "Q422_RepentYourSins"
	// repentSkull and repentManacles are two of Repent Your Sins' quest
	// items, which its exit takes.
	repentSkull    int32 = 4326
	repentManacles int32 = 4425
)

// subclassQuestOptions registers the three quests, so their journal rows
// load, and gives Repent Your Sins its quest items.
func subclassQuestOptions() []gameservertest.Option {
	quest := func(id int32, items ...int32) func() script.Script {
		return func() script.Script { return script.Script{QuestID: id, Items: items} }
	}
	catalog := script.Catalog{
		"quest." + fatesWhisper:   quest(234),
		"quest." + mimirsElixir:   quest(235),
		"quest." + repentYourSins: quest(422, repentSkull, repentManacles),
	}
	var list []script.Listing
	for path := range catalog {
		list = append(list, script.Listing{Path: path})
	}
	templates := gameservertest.ItemTemplates().All()
	for _, id := range []int32{repentSkull, repentManacles} {
		templates = append(templates, &item.Template{
			ID: id, Name: "Quest Item " + strconv.Itoa(int(id)), Kind: item.KindEtcItem, Duration: -1,
			Stackable: true, Destroyable: true, EtcItem: &item.EtcItemDetail{},
		})
	}
	return []gameservertest.Option{
		gameservertest.WithScripts(list, catalog),
		gameservertest.WithItemTemplates(item.NewTable(templates)),
	}
}

// questRow is one character_quests row a scenario seeds.
type questRow struct{ quest, variable, value string }

func seedQuests(t *testing.T, srv *gameservertest.Server, objID int32, rows ...questRow) {
	t.Helper()
	for _, r := range rows {
		exec(t, srv, `INSERT INTO character_quests (charId, name, var, value) VALUES (?, ?, ?, ?)`, objID, r.quest, r.variable, r.value)
	}
}

// questRows returns objID's character_quests rows of quest as var=value.
func questRows(t *testing.T, srv *gameservertest.Server, objID int32, quest string) []string {
	t.Helper()
	rows, err := srv.DB.QueryContext(context.Background(), `SELECT var, value FROM character_quests WHERE charId = ? AND name = ? ORDER BY var`, objID, quest)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var key string
		var value sql.NullString
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		out = append(out, key+"="+value.String)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func completed(quest string) questRow { return questRow{quest, "<state>", "COMPLETED"} }

// TestSubclassAddQuestGate adds a subclass with SubClassWithoutQuests off.
// A character who completed both Fate's Whisper and Mimir's Elixir adds
// it; one missing either quest, or holding one only started, is answered
// SubClass_Fail; a noble adds it with neither.
func TestSubclassAddQuestGate(t *testing.T) {
	cases := []struct {
		name  string
		rows  []questRow
		noble bool
		added bool
	}{
		{name: "both quests completed", rows: []questRow{completed(fatesWhisper), completed(mimirsElixir)}, added: true},
		{name: "Mimir's Elixir missing", rows: []questRow{completed(fatesWhisper)}},
		{name: "Fate's Whisper missing", rows: []questRow{completed(mimirsElixir)}},
		{name: "Fate's Whisper only started", rows: []questRow{{fatesWhisper, "<state>", "STARTED"}, {fatesWhisper, "<cond>", "1"}, completed(mimirsElixir)}},
		{name: "noble", noble: true, added: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := append(subclassQuestOptions(), gameservertest.WithSubclassRules(false, 0))
			w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
				seedQuests(t, srv, objID, tc.rows...)
				if tc.noble {
					exec(t, srv, `UPDATE characters SET nobless = 1 WHERE obj_Id = ?`, objID)
				}
			}, opts...)
			_, html := pageOf(t, w.addSpellhowler(t))
			switch {
			case tc.added && !strings.Contains(html, "You've added a new subclass"):
				t.Fatalf("add = %q, want SubClass_AddOk", html)
			case !tc.added && !strings.Contains(html, "You aren't eligible to add a subclass"):
				t.Fatalf("add = %q, want SubClass_Fail", html)
			}
		})
	}
}

// TestSubclassChangeExitsRepentYourSins changes class with Repent Your Sins
// started. The switch exits it as a repeatable quest where the reference
// does, after the effects come back and before the vitals are clamped: the
// quest window is sent without it, its items are taken and its rows are
// deleted. The completed quests stay. A later change finds nothing to exit.
func TestSubclassChangeExitsRepentYourSins(t *testing.T) {
	w := bootSubclassWorldWith(t, func(srv *gameservertest.Server, objID int32) {
		seedQuests(t, srv, objID,
			questRow{repentYourSins, "<state>", "STARTED"}, questRow{repentYourSins, "<cond>", "2"},
			completed(fatesWhisper),
		)
		srv.GiveItem(t, objID, repentSkull, 3)
		srv.GiveItem(t, objID, repentManacles, 1)
	}, subclassQuestOptions()...)

	frames := w.addSpellhowler(t)
	etc, list, henna := firstIndex(frames, serverpackets.OpcodeEtcStatusUpdate), firstIndex(frames, serverpackets.OpcodeQuestList), firstIndex(frames, serverpackets.OpcodeHennaInfo)
	if etc < 0 || list < etc || henna < list {
		t.Fatalf("add answer = %x, want QuestList between EtcStatusUpdate and HennaInfo", opcodes(frames))
	}
	if ids := questListIDs(frames[list]); len(ids) != 1 || ids[0] != 234 {
		t.Fatalf("quest window = %v, want the completed quest 234 alone", ids)
	}
	if _, html := pageOf(t, frames); !strings.Contains(html, "You've added a new subclass") {
		t.Fatalf("page = %q, want SubClass_AddOk", html)
	}
	for _, id := range []int32{repentSkull, repentManacles} {
		if n := w.srv.PlayerItemCount(t, w.player, id); n != 0 {
			t.Fatalf("quest item %d count = %d, want taken", id, n)
		}
	}
	if vars := w.srv.QuestVars(t, w.player, repentYourSins); vars != nil {
		t.Fatalf("Repent Your Sins after the change = %v, want no state", vars)
	}
	w.srv.FlushPersistence(t)
	if rows := questRows(t, w.srv, w.player, repentYourSins); len(rows) != 0 {
		t.Fatalf("Repent Your Sins rows = %v, want deleted", rows)
	}
	if rows := questRows(t, w.srv, w.player, fatesWhisper); len(rows) != 1 || rows[0] != "<state>=COMPLETED" {
		t.Fatalf("Fate's Whisper rows = %v, want kept completed", rows)
	}

	// Back to the base class: no quest is started, so none is exited and
	// no quest window is sent.
	frames = w.changeTo(t, 0)
	if i := firstIndex(frames, serverpackets.OpcodeQuestList); i >= 0 {
		t.Fatalf("change answer = %x, want no QuestList", opcodes(frames))
	}
}

// questListIDs returns the quest ids of a QuestList frame.
func questListIDs(frame []byte) []int32 {
	r := wire.NewReader(frame[1:])
	n := int(r.ReadUint16())
	ids := make([]int32, n)
	for i := range ids {
		ids[i] = r.ReadInt32()
		r.ReadInt32()
	}
	return ids
}
