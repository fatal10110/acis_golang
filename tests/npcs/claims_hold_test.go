package npcs

import (
	"context"
	"strings"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/derby"
	"github.com/fatal10110/acis_golang/internal/gameserver/fishchamp"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sevensigns"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The claims and stakes below are stored while their dialog runs. Each test
// holds the table the store writes: while the store waits, the player's
// queue still runs work posted to it, and once the lock lifts the answer
// arrives as without the hold. Were the store made on the queue, the probe
// would wait behind it.

// TestOlympiadNoblessePassClaimHoldsNoQueue holds the nobles table while
// the noble trades its points for passes.
func TestOlympiadNoblessePassClaimHoldsNoQueue(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(120, false)), gameservertest.WithOlympiadValidation())
	w.managerPage(t, manager, "OlympiadNoble 6")

	release := w.srv.LockTable(t, "olympiad_nobles")
	w.c.Send(encodeBypass(npcCommand(manager, "OlympiadNoble 10")))
	w.srv.AwaitLockWait(t, "INSERT INTO olympiad_nobles")
	w.srv.AwaitQueueRuns(t, w.player)
	release()

	frames := drainFrames(t, w.c)
	if got := systemMessageIDs(frames); len(got) != 1 || got[0] != smYouPickedUpS2S1 {
		t.Fatalf("trade messages = %v, want the pick-up message alone", got)
	}
	if frames[len(frames)-1][0] != serverpackets.OpcodeActionFailed {
		t.Fatalf("trade answer = %x, want the closing ActionFailed last", opcodes(frames))
	}
	if got := w.savedCount(t, noblesseGatePass); got != 120000 {
		t.Fatalf("passes held = %d, want 120000", got)
	}
}

// TestFishingChampionshipClaimHoldsNoQueue holds the championship table
// while a winner claims its prize.
func TestFishingChampionshipClaimHoldsNoQueue(t *testing.T) {
	t.Parallel()
	w, f := bootFisherman(t, fishchamp.DefaultConfig(), []any{"Talker", 81.5, 1}, []any{"Other", 88.25, 1})
	w.talkTo(t, f)
	w.dialogFrames(t, npcCommand(f, "FishingChampionship"))

	release := w.srv.LockTable(t, "fishing_championship")
	w.c.Send(encodeBypass(npcCommand(f, "FishingReward")))
	w.srv.AwaitLockWait(t, "DELETE FROM fishing_championship")
	w.srv.AwaitQueueRuns(t, w.player)
	release()

	var frames [][]byte
	for _, frame := range drainFrames(t, w.c) {
		switch frame[0] {
		case serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeSystemMessage, serverpackets.OpcodeActionFailed:
			frames = append(frames, frame)
		}
	}
	assertFrames(t, "claim", frames[:1], sysMsg(serverpackets.SystemMessageYouPickedUpS2S1, itemNameParam(item.AdenaID), itemNumberParam(500000)))
	if got := opcodes(frames[1:]); string(got) != string(releaseAnswer) {
		t.Fatalf("claim answer = %x, want %x", got, releaseAnswer)
	}
}

// TestDerbyBetSaveHoldsNoQueue holds the stakes table while a ticket is
// bought; the stake is stored once the lock lifts. The store's own bound
// would free a queue it held, so the log shows the store never gave up.
func TestDerbyBetSaveHoldsNoQueue(t *testing.T) {
	t.Parallel()
	w := bootDerby(t, 5000, gameservertest.WithCapturedLog())
	w.tick(1)
	w.manager = w.spawnFolk(t, folkTemplate("DerbyTrackManagerNpc", raceManagerID), 60)
	w.talkTo(t, w.manager)
	for _, step := range []string{"BuyTicket 0", "BuyTicket 5", "BuyTicket 10", "BuyTicket 13", "BuyTicket 20"} {
		html(t, w.dialog(t, step))
	}

	release := w.srv.LockTable(t, "mdt_bets")
	w.c.Send(encodeBypass(npcCommand(w.manager, "BuyTicket 21")))
	w.srv.AwaitLockWait(t, "REPLACE INTO mdt_bets")
	w.srv.AwaitQueueRuns(t, w.player)
	release()

	var frames [][]byte
	for _, f := range drainFrames(t, w.c) {
		if f[0] != serverpackets.OpcodeInventoryUpdate && f[0] != serverpackets.OpcodeStatusUpdate {
			frames = append(frames, f)
		}
	}
	w.assertChat(t, "buy under the hold", frames,
		sysMsg(serverpackets.SystemMessageS1DisappearedAdena, numberParam(1000)),
		sysMsg(serverpackets.SystemMessageAcquiredS1S2, numberParam(1), itemNameParam(derby.TicketItemID)))
	var bet int64
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT bet FROM mdt_bets WHERE lane_id = 5`).Scan(&bet); err != nil || bet != 1000 {
		t.Fatalf("stored lane 5 stake = %d, %v; want 1000", bet, err)
	}
	if logs := w.srv.LogText(); strings.Contains(logs, "save derby bet") {
		t.Fatalf("the stake's store gave up: %s", logs)
	}
}

// TestSignsPriestSignUpAnswersWhileInsertWaits holds the Seven Signs table
// while a talker signs up. The new row's insert is left to the talker's
// persistence lane, so while it waits the talker's queue still runs; once
// the lock lifts the dialog's answer is whole, the row lands, and the insert
// never gave up. Were the insert made on the queue, the probe would wait
// behind it until the insert's bound ran out.
func TestSignsPriestSignUpAnswersWhileInsertWaits(t *testing.T) {
	t.Parallel()
	w := bootPriestWorldWith(t, map[int32]int32{item.AdenaID: 60000}, []gameservertest.Option{gameservertest.WithCapturedLog()},
		`UPDATE characters SET classid = 2, base_class = 2 WHERE char_name = 'Talker'`)
	dawn := w.selectedPriest(t, "DawnPriest", dawnPriestID, 30)
	setSevenSigns(t, w, sevensigns.Competition, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal, sevensigns.NoCabal)
	w.openAnyNpcPage(t)

	release := w.srv.LockTable(t, "seven_signs")
	w.c.Send(encodeBypass(npcCommand(dawn, "SevenSigns 4 1 3")))
	w.srv.AwaitLockWait(t, "INSERT INTO seven_signs")
	w.srv.AwaitQueueRuns(t, w.player)
	release()

	frames := drainFrames(t, w.c)

	if _, ok := firstOpcode(frames, serverpackets.OpcodeNpcHtmlMessage); !ok {
		t.Fatalf("sign-up answer = %x, want its page before the insert lands", opcodes(frames))
	}
	if got := systemMessageIDs(frames); len(got) != 2 || got[0] != msgJoinedDusk || got[1] != msgFightForStrife {
		t.Fatalf("sign-up messages = %v, want joined Dusk then the seal", got)
	}
	if row, ok := w.savedSignsRow(t); !ok || row.cabal != "DUSK" || row.seal != "STRIFE" {
		t.Fatalf("seven_signs row = %+v (found %v), want DUSK STRIFE", row, ok)
	}
	if logs := w.srv.LogText(); strings.Contains(logs, "insert sign-up") {
		t.Fatalf("the sign-up's insert gave up: %s", logs)
	}
}
