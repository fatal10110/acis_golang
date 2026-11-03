package npcs

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOlympiadNoblessePassClaimLandsFirst: by the time the passes reach
// the player, the claim's olympiad_nobles row is already in the database,
// with no persistence flushed, so the passes' item row, written later, can
// never land ahead of it (#3454).
func TestOlympiadNoblessePassClaimLandsFirst(t *testing.T) {
	t.Parallel()
	w, manager := bootOlympiadManager(t, nobleDuelist(t, nobleRecord(120, false)), gameservertest.WithOlympiadValidation())
	if got, want := w.managerPage(t, manager, "OlympiadNoble 6"), wantOlympiadPage(t, manager, "noble_settle.htm"); got != want {
		t.Fatalf("offer page = %q, want %q", got, want)
	}
	frames := w.bypass(t, npcCommand(manager, "OlympiadNoble 10"))
	if _, ok := firstOpcode(frames, serverpackets.OpcodeSystemMessage); !ok {
		t.Fatalf("trade answer = %x, want the pick-up message", opcodes(frames))
	}
	var points, rewarded int
	if err := w.srv.DB.QueryRowContext(context.Background(), `SELECT olympiad_points, rewarded FROM olympiad_nobles WHERE char_id = ?`, w.player).Scan(&points, &rewarded); err != nil || points != 0 || rewarded != 1 {
		t.Fatalf("stored record as the passes arrive = %d points, rewarded %d, %v; want 0 and 1", points, rewarded, err)
	}
	if got := w.savedCount(t, noblesseGatePass); got != 120000 {
		t.Fatalf("passes held = %d, want 120000", got)
	}
}
