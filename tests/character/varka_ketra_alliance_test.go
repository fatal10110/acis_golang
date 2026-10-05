package character

import (
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// varkaKetraAllyColumn reads objID's stored characters.varka_ketra_ally.
func varkaKetraAllyColumn(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var level int
	if err := srv.DB.QueryRowContext(context.Background(), `SELECT varka_ketra_ally FROM characters WHERE obj_Id = ?`, objID).Scan(&level); err != nil {
		t.Fatalf("read varka_ketra_ally of %d: %v", objID, err)
	}
	return level
}

// TestVarkaKetraAllianceSurvivesRelog pins characters.varka_ketra_ally: the
// stored standing loads with the character, a change is written by the
// save restart runs and survives the next login, and the two ally checks
// follow the sign — negative is Varka, positive Ketra, 0 neither.
func TestVarkaKetraAllianceSurvivesRelog(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1), gameservertest.WithReuseDelays(0, 0))
	objID := srv.SoleObjectID(t)
	if _, err := srv.DB.ExecContext(context.Background(), `UPDATE characters SET varka_ketra_ally = 3 WHERE obj_Id = ?`, objID); err != nil {
		t.Fatalf("seed varka_ketra_ally: %v", err)
	}

	assertStanding := func(when string, want int) {
		t.Helper()
		c := onlineChar(t, srv, objID)
		if got := c.VarkaKetraAlliance(); got != want {
			t.Fatalf("%s: standing = %d, want %d", when, got, want)
		}
		if c.AlliedWithVarka() != (want < 0) || c.AlliedWithKetra() != (want > 0) {
			t.Fatalf("%s: allied with Varka %v, Ketra %v at standing %d", when, c.AlliedWithVarka(), c.AlliedWithKetra(), want)
		}
	}
	// Restart leaves the world for character selection, which saves.
	restart := func() {
		t.Helper()
		srv.Client.Send(encodeSingleOpcode(clientpackets.OpcodeRequestRestart))
		if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeRestartResponse {
			t.Fatalf("restart opcode = %#x, want RestartResponse", reply[0])
		}
		if reply := srv.Client.Read(); reply[0] != serverpackets.OpcodeCharSelectInfo {
			t.Fatalf("post-restart opcode = %#x, want CharSelectInfo", reply[0])
		}
	}

	enterWorld(t, srv.Client)
	assertStanding("stored Ketra 3", 3)

	onlineChar(t, srv, objID).SetVarkaKetraAlliance(-5)
	assertStanding("set Varka 5", -5)
	restart()
	if got := varkaKetraAllyColumn(t, srv, objID); got != -5 {
		t.Fatalf("saved varka_ketra_ally = %d, want -5", got)
	}

	enterWorld(t, srv.Client)
	assertStanding("relog Varka 5", -5)

	onlineChar(t, srv, objID).SetVarkaKetraAlliance(0)
	assertStanding("reset", 0)
	restart()
	if got := varkaKetraAllyColumn(t, srv, objID); got != 0 {
		t.Fatalf("saved varka_ketra_ally = %d, want 0", got)
	}
}
