package quest

import (
	"encoding/binary"
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/quest/q127"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Kamael: A Window to the Future's story (Q127_KamaelAWindowToTheFuture
// onAdvEvent "kamaelstory"). The scenario runner leaves out every extended
// packet but the quest mark, so the scenario q127_kamael_a_window_to_the_future
// plays the step without its ExPlayScene; this Boot test pins the scene, in
// the place the reference trace of that scenario sends it.
const (
	q127Name    = "Q127_KamaelAWindowToTheFuture"
	q127Path    = "quest.Q127_KamaelAWindowToTheFuture"
	q127Rodemai = 30756
)

// TestQ127StoryPlaysTheScene: at cond 7, Rodemai's story sets cond 8, plays
// the middle sound and then the scene for the character, and shows no page.
func TestQ127StoryPlaysTheScene(t *testing.T) {
	t.Parallel()
	pages := chatPages(questPages(q127Name, map[string][]string{
		"30756-01.htm": {"30756-02.htm"}, "30756-02.htm": {"30756-03.htm"}, "30756-03.htm": {"kamaelstory"},
	}), q127Rodemai)
	w := bootSeamQuest(t, 20, pages, func(srv *gameservertest.Server, objID int32) {
		var journal []journalRow
		for k, v := range started(7) {
			journal = append(journal, journalRow{objID, q127Name, k, val(v)})
		}
		insertJournal(t, srv.DB, journal...)
	},
		gameservertest.WithNpcSpawns(nil),
		gameservertest.WithNPCScripts(
			map[int32]script.NPCKind{q127Rodemai: script.KindFolk},
			[]script.Listing{{Path: q127Path}},
			script.Catalog{q127Path: q127.New},
		),
	)
	w.spawnFolk(t, "rodemai", q127Rodemai, 40, 0, 0)

	wantQ118Page(t, "cond 7", w.talk(t, "rodemai"), "30756-01.htm")
	wantQ118Page(t, "story 1", w.send(t, encodeBypass("Quest "+q127Name+" 30756-02.htm")), "30756-02.htm")
	wantQ118Page(t, "story 2", w.send(t, encodeBypass("Quest "+q127Name+" 30756-03.htm")), "30756-03.htm")

	w.srv.Client.Send(encodeBypass("Quest " + q127Name + " kamaelstory"))
	var got []string
	for _, f := range w.srv.ReadQueued(t, w.srv.Client) {
		switch {
		case f[0] == serverpackets.OpcodeQuestList:
			got = append(got, "QuestList")
		case f[0] == serverpackets.OpcodePlaySound:
			got = append(got, "PlaySound")
		case f[0] == serverpackets.OpcodeNpcHtmlMessage:
			got = append(got, "NpcHtmlMessage")
		case f[0] == serverpackets.OpcodeActionFailed:
			got = append(got, "ActionFailed")
		case f[0] == 0xfe && len(f) >= 3:
			ex := "Ex" + strconv.FormatUint(uint64(binary.LittleEndian.Uint16(f[1:3])), 16)
			if ex == "Ex5b" && len(f) != 3 {
				t.Fatalf("ExPlayScene % x has a body", f)
			}
			got = append(got, ex)
		}
	}
	// ExShowQuestMark is the extended 0x1a, ExPlayScene the extended 0x5b
	// with no body.
	want := []string{"QuestList", "Ex1a", "PlaySound", "Ex5b"}
	if !slices.Equal(got, want) {
		t.Fatalf("kamaelstory: frames %q, want %q", got, want)
	}
	rows := questRows(t, w.srv, w.player, q127Name)
	if rows["<cond>"] != "8" || rows["<state>"] != "STARTED" {
		t.Fatalf("kamaelstory: rows %v, want cond 8 started", rows)
	}
}
