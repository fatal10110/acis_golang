package npcs

import (
	"slices"
	"strconv"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestOlympiadClassRankingHoldsNoQueue holds the month's standings while the
// player asks the manager for a class ranking. While the read waits, the
// player's queue still runs work posted to it; once the lock lifts the
// ranking page arrives, then the closing ActionFailed, as without the hold.
// Were the read made on the queue, the probe would wait behind it and the
// read would time out, ranking nobody.
func TestOlympiadClassRankingHoldsNoQueue(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t,
		`INSERT INTO olympiad_nobles_eom (char_id, class_id, olympiad_points, competitions_done, competitions_won, competitions_lost, competitions_drawn)
			SELECT obj_Id, 88, 40, 6, 3, 3, 0 FROM characters WHERE char_name = 'Talker'`))
	fill := []string{"%place1%", "1", "%rank1%", "Talker"}
	for i := 2; i <= 10; i++ {
		fill = append(fill, "%place"+strconv.Itoa(i)+"%", "", "%rank"+strconv.Itoa(i)+"%", "")
	}
	w.offer(t, manager, "Olympiad 2_88")

	release := w.srv.LockTable(t, "olympiad_nobles_eom")
	w.c.Send(encodeBypass(npcCommand(manager, "Olympiad 2_88")))
	w.srv.AwaitLockWait(t, "SELECT characters.char_name")
	w.srv.AwaitQueueRuns(t, w.player)
	release()

	frames := drainFrames(t, w.c)
	if got, want := opcodes(frames), []byte{serverpackets.OpcodeNpcHtmlMessage, serverpackets.OpcodeActionFailed}; !slices.Equal(got, want) {
		t.Fatalf("ranking answer = %x, want %x", got, want)
	}
	if _, page := pageOf(t, frames); page != wantOlympiadPage(t, manager, "noble_ranking.htm", fill...) {
		t.Fatalf("ranking page = %q, want Talker first, the other places blank", page)
	}
}
