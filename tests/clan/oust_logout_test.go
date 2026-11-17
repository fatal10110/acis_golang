package clan

import (
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
)

// postPark posts a task that blocks q until release is called (also on
// cleanup); parked closes once it runs.
func postPark(t *testing.T, q *sim.Queue) (parked <-chan struct{}, release func()) {
	t.Helper()
	gate, started := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	q.Post(func() {
		close(started)
		<-gate
	})
	return started, release
}

// TestOustDuringLogoutKeepsPenalty expels a titled member whose logout
// check passed, with its detach queued ahead of the leave the expulsion
// posts to it: its logout save copies the character still in the clan,
// titled and free to join, and lands after the expulsion's row. The row
// still ends clanless, untitled and under the join penalty, so leaving the
// world as the clan expels it escapes neither.
func TestOustDuringLogoutKeepsPenalty(t *testing.T) {
	t.Parallel()
	w := bootSeededClan(t, 3, 0, false)
	if !w.srv.DrivesClock() {
		t.Skip("ordering the expulsion against a parked detach needs the one-FIFO inline executor")
	}
	w.recruit(t)
	w.leader.Send(encodeRequestGiveNickName("Recruit", "Lord"))
	drainFrames(t, w.leader)
	drainFrames(t, w.member)
	w.srv.FlushPersistence(t)

	// The executor runs every queue's tasks in one FIFO, so a parked task
	// holds them all: the expulsion is accepted ahead of the detach the
	// logout check's connection posts, and the leave it posts to the
	// member is accepted after that detach.
	memberQueue, leaderQueue := w.srv.PlayerQueue(t, w.memberID), w.srv.PlayerQueue(t, w.leaderID)
	parked, first := postPark(t, memberQueue)
	<-parked
	n := memberQueue.Accepted()
	w.member.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	w.srv.AwaitQueueAccepted(t, memberQueue, n) // the logout check
	parked, second := postPark(t, memberQueue)
	n = leaderQueue.Accepted()
	before := time.Now().UnixMilli()
	w.leader.Send(encodeRequestOustPledgeMember("Recruit"))
	w.srv.AwaitQueueAccepted(t, leaderQueue, n) // the expulsion
	n = memberQueue.Accepted()
	first()
	<-parked
	w.srv.AwaitQueueAccepted(t, memberQueue, n) // the detach
	second()

	if !w.member.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	w.srv.Settle(t)
	w.srv.FlushPersistence(t)
	drainFrames(t, w.leader)
	if cl, _ := w.srv.Clans.Table().Get(titleClanID); cl.IsMember(w.memberID) {
		t.Fatal("the expelled member is still listed in the clan")
	}
	if got := queryInt(t, w, `SELECT COUNT(*) FROM characters WHERE obj_Id = ? AND COALESCE(clanid, 0) = 0 AND title = ''`, w.memberID); got != 1 {
		t.Fatal("expelled member's row is not clanless and untitled")
	}
	if got := queryInt(t, w, `SELECT clan_join_expiry_time FROM characters WHERE obj_Id = ?`, w.memberID); got < before+24*3600*1000 {
		t.Fatalf("expelled member's join penalty ends %d, want a day after %d", got, before)
	}
}
