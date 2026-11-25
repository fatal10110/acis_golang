package clan

import (
	"bytes"
	"testing"

	datacache "github.com/fatal10110/acis_golang/internal/gameserver/data/cache"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// Owner-approved divergences of #5148, each stricter than the reference.

// TestCrestOfTheWrongSizeDrawsNoObjectID pins that a pledge, large or
// alliance crest upload refused for its size takes no object id: the next
// id the server hands out is the one after the id drawn before the
// uploads.
func TestCrestOfTheWrongSizeDrawsNoObjectID(t *testing.T) {
	t.Run("pledge crests", func(t *testing.T) {
		w := bootCrestWorld(t, clanSeed{level: 3})
		before := w.srv.NewObjectID()
		drainSend(t, w, encodeSetPledgeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize-1)))
		drainSend(t, w, encodeSetLargeCrest(bytes.Repeat([]byte{1}, pledgeCrestSize)))
		if after := w.srv.NewObjectID(); after != before+1 {
			t.Fatalf("object id after the refused uploads = %d, want %d", after, before+1)
		}
	})
	t.Run("alliance crest", func(t *testing.T) {
		w := bootAllianceWorld(t, gameservertest.WithCrests(datacache.NewCrestsIn(t.TempDir())))
		w.formAlliance(t)
		before := w.srv.NewObjectID()
		w.leader.Send(encodeSetAllyCrest(bytes.Repeat([]byte{1}, clientpackets.AllyCrestMaxLength-1)))
		if frames := drainFrames(t, w.leader); len(frames) != 0 {
			t.Fatalf("refused alliance crest answered %x", opcodes(frames))
		}
		if after := w.srv.NewObjectID(); after != before+1 {
			t.Fatalf("object id after the refused upload = %d, want %d", after, before+1)
		}
		if got := storedAlly(t, w, knightsClanID).allyCrestID; got != 0 {
			t.Fatalf("stored alliance crest = %d", got)
		}
	})
}

// TestSubunitRenameToATakenName pins that a sub-unit may not be renamed to
// the name another sub-unit has, as at its founding; the unit keeps its
// name. It may take its own name in another case.
func TestSubunitRenameToATakenName(t *testing.T) {
	w, _ := bootSubunitWorld(t, 1, seedSubunitClan(t))
	w.recruit(t)
	w.masterCommandBy(t, w.leader, "create_academy School")
	w.masterCommandBy(t, w.leader, "create_royal Guards Squire")

	wantMessage(t, "rename to a taken name", w.masterCommandBy(t, w.leader, "rename_pledge 100 school"), serverpackets.SystemMessageMilitaryUnitNameTaken)
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_subpledges WHERE clan_id = ? AND sub_pledge_id = 100 AND name = 'Guards'`, subunitClanID); got != 1 {
		t.Fatal("the royal guard lost its name to a refused rename")
	}

	w.masterCommandBy(t, w.leader, "rename_pledge 100 GUARDS")
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COUNT(*) FROM clan_subpledges WHERE clan_id = ? AND sub_pledge_id = 100 AND name = BINARY 'GUARDS'`, subunitClanID); got != 1 {
		t.Fatal("the royal guard could not take its own name in capitals")
	}
}
