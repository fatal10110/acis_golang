package clan

import (
	"context"
	"database/sql"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/clan"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestInviteAcceptedAfterInviterChangedClan (#5105): Founder invites
// Recruit into Knights, then dissolves Knights and founds Rivals inside the
// invitation window. Recruit's acceptance names Knights, which Founder no
// longer belongs to: it is refused with ActionFailed, Recruit joins no clan,
// and Rivals keeps Founder alone, in memory and stored.
func TestInviteAcceptedAfterInviterChangedClan(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.DissolveDays = 0
	cfg.CreateDays = 0
	// Founder starts mid-level 10: the dissolution's experience loss leaves
	// it the level 10 a founding needs.
	midLevel := gameservertest.WithClanSeed(func(db *sql.DB) {
		if _, err := db.ExecContext(context.Background(), `UPDATE characters SET exp = 60000 WHERE char_name = 'Founder'`); err != nil {
			t.Fatalf("seed exp: %v", err)
		}
	})
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanConfig(cfg), dissolutionPages(t), gameservertest.WithLevels(warLevels(t)), midLevel)
	w.found(t, "Knights")

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("recruit was not asked")
	}
	wantDispersed(t, "founder", w.masterCommandBy(t, w.leader, "dissolve_clan"))
	rivals := w.found(t, "Rivals")
	drainFrames(t, w.member)

	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if got := opcodes(drainFrames(t, w.member)); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("recruit's stale acceptance = %x, want ActionFailed", got)
	}
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("founder after the stale acceptance = %x, want nothing", opcodes(frames))
	}
	if cl, ok := w.srv.Clans.Table().MemberClan(w.memberID); ok {
		t.Fatalf("recruit joined %q", cl.Name())
	}
	cl, ok := w.srv.Clans.Table().Get(rivals)
	if !ok {
		t.Fatal("Rivals is gone")
	}
	if n := len(cl.Members()); n != 1 {
		t.Fatalf("Rivals members = %d, want the founder alone", n)
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COALESCE(clanid, 0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("recruit's stored clanid = %d, want 0", got)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM characters WHERE clanid = ?`, rivals); got != 1 {
		t.Fatalf("stored Rivals members = %d, want 1", got)
	}
}

// TestInviteAcceptedAfterInviterLeftClan (#5105): Founder invites Recruit
// into Knights, then dissolves Knights and founds nothing. Recruit's
// acceptance names a clan its inviter no longer belongs to: it is refused
// with ActionFailed, Recruit joins no clan, and its side of the invitation
// is cleared so it can be invited again.
func TestInviteAcceptedAfterInviterLeftClan(t *testing.T) {
	cfg := clan.DefaultConfig()
	cfg.DissolveDays = 0
	w := bootClanWorld(t, 10, 0, 0, gameservertest.WithClanConfig(cfg), dissolutionPages(t), gameservertest.WithLevels(warLevels(t)))
	knights := w.found(t, "Knights")

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("recruit was not asked")
	}
	wantDispersed(t, "founder", w.masterCommandBy(t, w.leader, "dissolve_clan"))
	drainFrames(t, w.member)

	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if got := opcodes(drainFrames(t, w.member)); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("recruit's stale acceptance = %x, want ActionFailed", got)
	}
	if frames := drainFrames(t, w.leader); len(frames) != 0 {
		t.Fatalf("founder after the stale acceptance = %x, want nothing", opcodes(frames))
	}
	if cl, ok := w.srv.Clans.Table().MemberClan(w.memberID); ok {
		t.Fatalf("recruit joined %q", cl.Name())
	}
	if _, ok := w.srv.Clans.Invites().Partner(w.memberID); ok {
		t.Fatal("recruit's side of the refused invitation is still pending")
	}
	w.srv.FlushPersistence(t)
	if got := queryInt(t, w, `SELECT COALESCE(clanid, 0) FROM characters WHERE obj_Id = ?`, w.memberID); got != 0 {
		t.Fatalf("recruit's stored clanid = %d, want 0", got)
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM characters WHERE clanid = ?`, knights); got != 0 {
		t.Fatalf("stored Knights members = %d, want 0", got)
	}
}

// TestInviteAcceptedAfterInviterInvitedAnother (#5105): Founder invites
// Recruit, logs out and back in, and invites Third, which its gone login's
// invitation no longer keeps it from. Recruit's acceptance answers an
// invitation that now names Third: it is refused with ActionFailed and
// Recruit stays clanless, while Third's acceptance joins Knights.
func TestInviteAcceptedAfterInviterInvitedAnother(t *testing.T) {
	w := bootClanWorld(t, 10, 0, 0)
	knights := w.found(t, "Knights")
	thirdID := w.srv.SeedCharacterFor(t, "player3", "Third", 1, 0).ID
	third := w.srv.DialClient(t, "player3", 1)
	startInWorld(t, third)

	w.leader.Send(encodeRequestJoinPledge(w.memberID, 0))
	if _, ok := firstOpcode(drainFrames(t, w.member), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("recruit was not asked")
	}
	w.leaveWorld(t, w.leader)
	w.leader = w.srv.DialClient(t, "player1", 1)
	startInWorld(t, w.leader)
	drainFrames(t, w.member)
	drainFrames(t, third)
	w.leader.Send(encodeRequestJoinPledge(thirdID, 0))
	if _, ok := firstOpcode(drainFrames(t, third), serverpackets.OpcodeAskJoinPledge); !ok {
		t.Fatal("Third was not asked")
	}

	w.member.Send(encodeRequestAnswerJoinPledge(1))
	if got := opcodes(drainFrames(t, w.member)); !slices.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("recruit's stale acceptance = %x, want ActionFailed", got)
	}
	if cl, ok := w.srv.Clans.Table().MemberClan(w.memberID); ok {
		t.Fatalf("recruit joined %q", cl.Name())
	}
	third.Send(encodeRequestAnswerJoinPledge(1))
	if _, ok := firstOpcode(drainFrames(t, third), serverpackets.OpcodeJoinPledge); !ok {
		t.Fatal("Third did not join")
	}
	w.srv.FlushPersistence(t)
	for id, want := range map[int32]int64{w.memberID: 0, thirdID: int64(knights)} {
		if got := queryInt(t, w, `SELECT COALESCE(clanid, 0) FROM characters WHERE obj_Id = ?`, id); got != want {
			t.Fatalf("stored clanid of %d = %d, want %d", id, got, want)
		}
	}
}
