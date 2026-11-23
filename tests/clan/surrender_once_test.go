package clan

import (
	"context"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSimultaneousSurrendersPayOnce (#5106): Rivals declared war on
// Knights, and its leader and a second member holding the war privilege
// both surrender at once, each on its own queue. Only the surrender that
// ends the war pays a death's experience loss and is told so; the other
// finds no war left and is refused as not involved, keeping its experience.
func TestSimultaneousSurrendersPayOnce(t *testing.T) {
	w := bootWarWorld(t, gameservertest.WithRealPool())
	secondID := w.srv.SeedCharacterFor(t, "player3", "Second", 10, toLevel3SP).ID
	if _, err := w.srv.DB.ExecContext(context.Background(), `UPDATE characters SET exp = 60000 WHERE obj_Id = ?`, secondID); err != nil {
		t.Fatalf("seed exp: %v", err)
	}
	second := w.srv.DialClient(t, "player3", 1)
	startInWorld(t, second)
	drainAll := func() {
		for _, c := range []*testsupport.ScriptedClient{w.leader, w.member, second} {
			drainFrames(t, c)
		}
	}
	drainAll()

	w.found(t, "Knights")
	w.masterCommandBy(t, w.member, "create_clan Rivals")
	w.raiseToLevel3(t, w.leader)
	w.raiseToLevel3(t, w.member)
	w.member.Send(encodeWarName(clientpackets.OpcodeRequestStartPledgeWar, "Knights"))
	drainAll()
	w.member.Send(encodeRequestJoinPledge(secondID, 0))
	drainFrames(t, second)
	second.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, second), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Second did not join Rivals")
	}
	w.member.Send(encodeRequestPledgePower(6, 2, int32(clan.PrivClanWar)))
	drainAll()

	w.member.Send(encodeWarName(clientpackets.OpcodeRequestSurrenderPledgeWar, "Knights"))
	second.Send(encodeWarName(clientpackets.OpcodeRequestSurrenderPledgeWar, "Knights"))
	answers := map[int32][]int{w.memberID: messages(t, drainFrames(t, w.member)), secondID: messages(t, drainFrames(t, second))}
	drainFrames(t, w.leader)

	var payer, refused int32
	for id, ids := range answers {
		switch {
		case slices.Contains(ids, serverpackets.SystemMessageYouHaveSurrenderedToS1Clan) && !slices.Contains(ids, serverpackets.SystemMessageNotInvolvedInWar):
			payer = id
		case slices.Contains(ids, serverpackets.SystemMessageNotInvolvedInWar) && !slices.Contains(ids, serverpackets.SystemMessageYouHaveSurrenderedToS1Clan):
			refused = id
		}
	}
	if payer == 0 || refused == 0 {
		t.Fatalf("surrender answers = %v, want one surrender and one NOT_INVOLVED_IN_WAR", answers)
	}

	w.leaveWorld(t, w.member)
	w.leaveWorld(t, second)
	// Level 10 spans 48229-71201: 8.875% of 22972 is 2039.
	for id, want := range map[int32]int64{payer: 60000 - 2039, refused: 60000} {
		if got := queryInt(t, w, `SELECT exp FROM characters WHERE obj_Id = ?`, id); got != want {
			t.Fatalf("exp of %d after the surrenders = %d, want %d", id, got, want)
		}
	}
}
