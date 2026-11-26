package clan

import (
	"context"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// bootLoggingOutLeaver boots the founder leading a level 5 clan owning
// Gludio Castle and the recruit, under a clan creation penalty ending at
// createExpiry and wearing the Circlet of Gludio, in the world; the recruit
// has then joined the clan. It returns the world and the circlet's object
// id.
func bootLoggingOutLeaver(t *testing.T, createExpiry int64) (*clanWorld, int32) {
	t.Helper()
	srv := gameservertest.Boot(t, append(castleClanOptions(t),
		gameservertest.WithCharacter("Founder", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithReuseDelays(0, 0),
		knightsSeed(t, 5, 1),
	)...)
	w := &clanWorld{srv: srv, leader: srv.Client, leaderID: srv.SoleObjectID(t)}
	w.memberID = srv.SeedCharacterFor(t, "player2", "Recruit", 40, 0).ID
	circlet := srv.GiveItem(t, w.memberID, circletOfGludio, 1)
	ctx := context.Background()
	if _, err := srv.DB.ExecContext(ctx, `UPDATE items SET loc = 'PAPERDOLL', loc_data = ? WHERE object_id = ?`, faceSlot, circlet); err != nil {
		t.Fatalf("wear the circlet: %v", err)
	}
	if _, err := srv.DB.ExecContext(ctx, `UPDATE characters SET clan_create_expiry_time = ? WHERE obj_Id = ?`, createExpiry, w.memberID); err != nil {
		t.Fatalf("set the creation penalty: %v", err)
	}
	w.member = srv.DialClient(t, "player2", 1)
	startInWorld(t, w.leader)
	startInWorld(t, w.member)
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.recruit(t)
	return w, circlet
}

// TestOustDuringLogoutWindowWritesOnlineRemoval expels a member its logout
// already took out of the world, its clan still listing it online: the
// world removal is driven on its queue, as its logout does it, before its
// clan marks it offline. The member leaves as an online one does, its
// logout's saves having taken it online: its row ends clanless, untitled,
// at power grade 0, under the join penalty and keeping its own creation
// penalty, and the Circlet of Gludio it wears stays on, its last inventory
// save storing it worn, rather than going back to its inventory against
// that save.
func TestOustDuringLogoutWindowWritesOnlineRemoval(t *testing.T) {
	t.Parallel()
	createExpiry := time.Now().Add(72 * time.Hour).UnixMilli()
	w, circlet := bootLoggingOutLeaver(t, createExpiry)
	w.leader.Send(encodeRequestGiveNickName("Recruit", "Lord"))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	if got := queryInt(t, w, `SELECT power_grade FROM characters WHERE obj_Id = ?`, w.memberID); got == 0 {
		t.Fatal("recruit stored at power grade 0; an online removal would not show")
	}

	// Out of the world, still listed online: the span between the logout's
	// world removal and its clan marking it offline.
	removed := make(chan struct{})
	w.srv.PlayerQueue(t, w.memberID).Post(func() {
		defer close(removed)
		obj, ok := w.srv.State.Player(w.memberID)
		if !ok {
			t.Error("recruit not in the world")
			return
		}
		w.srv.State.RemovePlayer(obj)
	})
	<-removed
	cl, _ := w.srv.Clans.Table().Get(titleClanID)
	if m, ok := cl.Member(w.memberID); !ok || !m.Online {
		t.Fatalf("recruit listed %v, online %v; want listed online", ok, m.Online)
	}

	before := time.Now().UnixMilli()
	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	drainFrames(t, w.leader)
	if cl.IsMember(w.memberID) {
		t.Fatal("the expelled member is still listed in the clan")
	}

	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	if !w.member.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	w.srv.Settle(t)
	w.srv.FlushPersistence(t)

	if got := queryInt(t, w, `SELECT COUNT(*) FROM characters WHERE obj_Id = ? AND COALESCE(clanid, 0) = 0 AND title = '' AND power_grade = 0`, w.memberID); got != 1 {
		t.Fatal("expelled member's row is not clanless, untitled and at power grade 0")
	}
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ?`, w.memberID); got < before+24*3600*1000 {
		t.Fatalf("expelled member's join penalty ends %d, want a day after %d", got, before)
	}
	if got := queryInt(t, w, `SELECT clan_create_expiry_time FROM characters WHERE obj_Id = ?`, w.memberID); got != createExpiry {
		t.Fatalf("expelled member's creation penalty ends %d, want its own %d", got, createExpiry)
	}
	if loc := itemLoc(t, w, circlet); loc != "PAPERDOLL" {
		t.Fatalf("expelled member's circlet = %s, want PAPERDOLL", loc)
	}
}
