package skills

import (
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// Owner decision on #5062 (a deliberate divergence from the reference,
// which keeps the request forever and never re-checks the answerer): an
// accepted summon-friend dialog re-runs the summoned player's checks on the
// answering player, refused silently, and the pending request is dropped
// once the dialog's 30 s have passed or the summoner has left the world, so
// it no longer blocks another caster either.

const (
	summonConfirmTimeout = 30 * time.Second
	summonThirdName      = "Third"
)

// festivalOf signs up one object id for the festival.
type festivalOf int32

func (f festivalOf) IsParticipant(objectID int32) bool { return objectID == int32(f) }

// openSummonDialog has the caster cast the confirm summon at the friend and
// returns the friend's item count once the dialog is open.
func (w summonWorld) openSummonDialog(t *testing.T) int {
	t.Helper()
	w.selectTarget(t, w.targetID)
	w.cast(t, summonConfirmSkill)
	if !hasOpcode(w.target.ReadQueued(), serverpackets.OpcodeConfirmDlg) {
		t.Fatal("the friend got no summon dialog")
	}
	w.drain(t)
	return w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID)
}

// acceptSummon has the friend accept requesterID's dialog and asserts
// whether it was pulled, paying its item only then, with no summon-gate
// message told either side.
func (w summonWorld) acceptSummon(t *testing.T, requesterID int32, itemsBefore int, wantPulled bool) {
	t.Helper()
	w.target.Send(encodeSummonDlgAnswer(1, requesterID))
	w.srv.Settle(t)
	friend := w.target.ReadQueued()
	if got := teleported(friend); got != wantPulled {
		t.Fatalf("friend teleported = %v, want %v", got, wantPulled)
	}
	if !wantPulled {
		assertSummonMessage(t, "friend", friend, nil)
	}
	wantItems := itemsBefore
	if wantPulled {
		wantItems--
	}
	if got := w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID); got != wantItems {
		t.Fatalf("friend item = %d, want %d", got, wantItems)
	}
}

func hasOpcode(frames [][]byte, op byte) bool {
	for _, f := range frames {
		if f[0] == op {
			return true
		}
	}
	return false
}

// TestSummonFriendAcceptRechecksAnswerer opens the dialog, then has the
// friend die, open a store, fight, get rooted or sign up for the festival
// before accepting: it stays put and keeps its item, told nothing.
func TestSummonFriendAcceptRechecksAnswerer(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name  string
		setup func(*testing.T, *player.Character)
	}{
		{"dead", func(t *testing.T, pc *player.Character) { pc.MarkDead() }},
		{"operating", func(t *testing.T, pc *player.Character) { pc.SetOperateType(privatestore.OperateSell) }},
		{"in combat", func(t *testing.T, pc *player.Character) { pc.SetInCombat(true) }},
		{"rooted", addRoot},
		{"festival", func(t *testing.T, pc *player.Character) { pc.SetFestivalRoster(festivalOf(pc.ObjectID())) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			w := bootSummonWorld(t)
			items := w.openSummonDialog(t)
			w.on(t, w.targetID, func(pc *player.Character) { tt.setup(t, pc) })
			w.drain(t)

			w.acceptSummon(t, w.casterID, items, false)
			assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
		})
	}
}

// TestSummonFriendRequestLapses lets the dialog's 30 s pass: an accept a
// second before still pulls the friend, one after no longer does, and the caster may summon it again, the friend
// getting a fresh dialog whose accept does pull it.
func TestSummonFriendRequestLapses(t *testing.T) {
	t.Parallel()
	t.Run("accept in time", func(t *testing.T) {
		t.Parallel()
		w := bootSummonWorld(t)
		items := w.openSummonDialog(t)
		w.srv.Advance(t, summonConfirmTimeout-time.Second)
		w.drain(t)

		w.acceptSummon(t, w.casterID, items, true)
	})
	t.Run("accept", func(t *testing.T) {
		t.Parallel()
		w := bootSummonWorld(t)
		items := w.openSummonDialog(t)
		w.srv.Advance(t, summonConfirmTimeout)
		w.drain(t)

		w.acceptSummon(t, w.casterID, items, false)
	})
	t.Run("new request", func(t *testing.T) {
		t.Parallel()
		w := bootSummonWorld(t)
		w.openSummonDialog(t)
		w.srv.Advance(t, summonConfirmTimeout)
		w.drain(t)

		w.cast(t, summonConfirmSkill)
		assertSummonMessage(t, "caster", w.caster.ReadQueued(), nil)
		if !hasOpcode(w.target.ReadQueued(), serverpackets.OpcodeConfirmDlg) {
			t.Fatal("the friend got no fresh summon dialog")
		}
		w.drain(t)
		w.acceptSummon(t, w.casterID, w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID), true)
	})
}

// TestSummonFriendSummonerLeavingDropsRequest has the caster log out with
// the dialog open: an accept no longer pulls the friend, and a third player
// may summon it, the friend getting that player's dialog.
func TestSummonFriendSummonerLeavingDropsRequest(t *testing.T) {
	t.Parallel()
	t.Run("accept", func(t *testing.T) {
		t.Parallel()
		w := bootSummonWorld(t)
		items := w.openSummonDialog(t)
		w.logOutCaster(t)

		w.acceptSummon(t, w.casterID, items, false)
	})
	t.Run("new caster", func(t *testing.T) {
		t.Parallel()
		w := bootSummonWorld(t)
		w.openSummonDialog(t)
		w.logOutCaster(t)
		third, thirdID := w.enterThirdCaster(t)

		x, y, z := w.srv.PlayerPosition(t, w.targetID)
		third.Send(encodeAction(w.targetID, int32(x), int32(y), int32(z), false))
		drainUntilQuiet(t, third)
		drainUntilQuiet(t, w.target)
		third.Send(encodeRequestMagicSkillUse(summonConfirmSkill, false, false))
		w.srv.Advance(t, summonFriendCastSettle)
		assertSummonMessage(t, "third caster", third.ReadQueued(), nil)
		if !hasOpcode(w.target.ReadQueued(), serverpackets.OpcodeConfirmDlg) {
			t.Fatal("the friend got no summon dialog from the third caster")
		}
		drainUntilQuiet(t, third)
		drainUntilQuiet(t, w.target)
		w.acceptSummon(t, thirdID, w.srv.PlayerItemCount(t, w.targetID, summonTargetItemID), true)
	})
}

func (w summonWorld) logOutCaster(t *testing.T) {
	t.Helper()
	w.caster.Send(encodeLogout())
	if reply := w.caster.Read(); reply[0] != serverpackets.OpcodeLeaveWorld {
		t.Fatalf("logout reply opcode = %#x, want LeaveWorld", reply[0])
	}
	w.caster.ExpectClosed()
	w.srv.Settle(t)
	if _, ok := w.srv.State.Player(w.casterID); ok {
		t.Fatal("the caster is still in the world after logging out")
	}
	drainUntilQuiet(t, w.target)
}

// enterThirdCaster logs in a third player knowing the confirm summon,
// holding its consume item, next to the friend.
func (w summonWorld) enterThirdCaster(t *testing.T) (*testsupport.ScriptedClient, int32) {
	t.Helper()
	id := w.srv.SeedCharacterFor(t, "player3", summonThirdName, summonFriendLevel, 0).ID
	seedKnownSkill(t, w.srv, id, int(summonConfirmSkill), 1)
	w.srv.GiveItem(t, id, summonCasterItemID, summonFriendHeldItemCount)
	c := w.srv.DialClient(t, "player3", 1)
	startInWorldAmongPlayers(t, c)
	drainUntilQuiet(t, c)
	drainUntilQuiet(t, w.target)
	return c, id
}
