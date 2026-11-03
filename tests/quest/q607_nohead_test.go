package quest

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

func encodeRequestDestroyItem(objectID, count int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestDestroyItem)
	w.WriteInt32(objectID)
	w.WriteInt32(count)
	return w.Bytes()
}

// TestQ607TradeWithoutTheHeadGivesNothing: a Ketra ally who took the quest
// and has no head yet is told to bring it (31370-06.htm). After Shadith he
// is shown the trade (31370-05.htm), destroys the head, and follows the
// trade link anyway: he is told again to bring it, goes back to cond 1 with
// the accept sound, and gets no totem and no experience; the alliance mark
// stays (Q607_ProveYourCourage.onTalk, onAdvEvent).
func TestQ607TradeWithoutTheHeadGivesNothing(t *testing.T) {
	t.Parallel()
	w := bootSeamQuest(t, 75, chatPages(questPages(q607Name, map[string][]string{
		"31370-01.htm": {"31370-04.htm"}, "31370-02.htm": nil, "31370-04.htm": nil,
		"31370-05.htm": {"31370-07.htm"}, "31370-06.htm": nil, "31370-07.htm": nil,
	}), kadunID), func(srv *gameservertest.Server, objID int32) {
		seedStanding(t, srv, objID, 3)
		srv.GiveItem(t, objID, ketraAlliance3ID, 1)
		if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET exp = ? WHERE obj_Id = ?", int64(74)*q607LevelExp, objID); err != nil {
			t.Fatalf("seed experience: %v", err)
		}
	}, q607Scripts(), q607Items(), gameservertest.WithLevels(q607Levels(t)))
	kadun := w.spawnFolk(t, "kadun", kadunID, 0, 30, 0).ObjectID()

	if got := w.questWindow(t, kadun); !shows(got, "31370-01.htm") {
		t.Fatalf("Kadun's window = %q, want 31370-01.htm", got)
	}
	if got := w.bypass(t, "Quest "+q607Name+" 31370-04.htm"); !shows(got, "31370-04.htm") {
		t.Fatalf("the accept link = %q, want 31370-04.htm", got)
	}
	if got := w.questWindow(t, kadun); !shows(got, "31370-06.htm") {
		t.Fatalf("Kadun's window without the head = %q, want 31370-06.htm", got)
	}

	shadith := w.srv.SpawnHostileNPCTemplateAt(t, proofMonster(shadithID), location.Location{X: w.at.X + 40, Y: w.at.Y, Z: w.at.Z})
	killOnPlayerQueue(t, w.srv, w.player, shadith)
	w.srv.ReadQueued(t, w.srv.Client)
	if got := w.questWindow(t, kadun); !shows(got, "31370-05.htm") {
		t.Fatalf("Kadun's window with the head = %q, want 31370-05.htm", got)
	}

	var head int32
	w.srv.RunScript(t, w.player, q607Name, func(_ *script.Script, p *script.Player) {
		for _, held := range p.HeldItems() {
			if held.ItemID == headOfShadithID {
				head = held.ObjectID
			}
		}
	})
	if head == 0 {
		t.Fatal("no head of Shadith after the kill")
	}
	w.srv.Client.Send(encodeRequestDestroyItem(head, 1))
	w.srv.ReadQueued(t, w.srv.Client)
	if n := w.srv.PlayerItemCount(t, w.player, headOfShadithID); n != 0 {
		t.Fatalf("heads after the destroy = %d, want 0", n)
	}

	got := w.bypass(t, "Quest "+q607Name+" 31370-07.htm")
	want := []string{
		// cond 1 with no flags stored: (1<<1)-1 | 0x80000000
		// (QuestState.calculateFlags).
		"S QuestList 607:0x80000001",
		"S ExShowQuestMark quest=607",
		"S PlaySound type=0 file=ItemSound.quest_accept bind=0 obj=0 loc=0,0,0 delay=0",
		`S NpcHtmlMessage obj=kadun item=0 html="<html><body>31370-06.htm</body></html>\n"`,
		"S ActionFailed",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("the trade link without the head = %q, want %q", got, want)
	}
	if vars := w.srv.QuestVars(t, w.player, q607Name); vars["<state>"] != "STARTED" || vars["<cond>"] != "1" {
		t.Fatalf("Q607 after the trade link = %v, want started at cond 1", vars)
	}
	for id, n := range map[int32]int{headOfShadithID: 0, totemOfValorID: 0, ketraAlliance3ID: 1} {
		if c := w.srv.PlayerItemCount(t, w.player, id); c != n {
			t.Errorf("item %d after the trade link = %d, want %d", id, c, n)
		}
	}
}
