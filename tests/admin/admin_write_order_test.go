package admin

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// readAccessLevel reads objID's stored access level once.
func readAccessLevel(t *testing.T, srv *gameservertest.Server, objID int32) int {
	t.Helper()
	var got int
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT accesslevel FROM characters WHERE obj_Id = ?", objID).Scan(&got); err != nil {
		t.Fatalf("read access level: %v", err)
	}
	return got
}

func encodeSelectSlot0() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestGameStart)
	w.WriteInt32(0)
	w.WriteUint16(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	return w.Bytes()
}

func encodeLogout() []byte { return wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes() }

// TestAdminGMOffLevelStoredWithSave pins //gmoff's row timing
// (Player.setAccessLevel changes memory only; storeCharBase writes
// accesslevel): the dropped level reaches the row with the next save, not
// at once, so a server stopped inside the window keeps the GM's level.
func TestAdminGMOffLevelStoredWithSave(t *testing.T) {
	t.Parallel()
	srv, gmID := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)

	exchange(t, gm, encodeBuildCmd("gmoff 5"))
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, gmID); got != adminLevel {
		t.Fatalf("stored access level after //gmoff = %d, want %d until the next save", got, adminLevel)
	}
	srv.TickAutosave(t)
	if got := readAccessLevel(t, srv, gmID); got != userLevel {
		t.Fatalf("stored access level after the autosave = %d, want %d", got, userLevel)
	}
}

// TestOfflineAccessDuringSelectionReachesSession pins an offline //set
// access landing while the character is being selected: the write waits for
// the selection instead of racing its row read, and the session it starts
// plays under the new level, which its logout save then stores.
func TestOfflineAccessDuringSelectionReachesSession(t *testing.T) {
	t.Parallel()
	srv, _ := bootAdmin(t, adminLevel)
	gm := srv.Client
	enterWorld(t, gm)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	user := srv.DialClient(t, "player2", 1)

	release := srv.HoldPersistenceLane(t, ch.ID)
	user.Send(encodeSelectSlot0())
	if got := srv.ReadQueued(t, user); len(got) != 0 {
		t.Fatalf("selection answered %x while its saves were held, want it waiting", testsupport.FrameOpcodes(got))
	}
	assertTexts(t, exchange(t, gm, encodeBuildCmd("set access Player 7")), "Player's access level is now set to 7.")
	if got := readAccessLevel(t, srv, ch.ID); got != userLevel {
		t.Fatalf("stored access level while the selection waits = %d, want %d: the write must not pass the selection", got, userLevel)
	}
	release()

	user.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	user.ReadQueued()
	drain(t, gm)
	assertGMs(t, exchange(t, gm, encodeGmList()), "Admin", "Player")

	user.Send(encodeLogout())
	if !user.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	srv.FlushPersistence(t)
	if got := readAccessLevel(t, srv, ch.ID); got != adminLevel {
		t.Fatalf("stored access level after logout = %d, want %d", got, adminLevel)
	}
}

// TestOfflineUnjailDuringLogoutLandsAfterSave pins an offline //unjail of a
// character whose logout saves are still queued: the release, Floran
// village included, lands after them instead of being overwritten by the
// logout's jail position.
func TestOfflineUnjailDuringLogoutLandsAfterSave(t *testing.T) {
	t.Parallel()
	srv := bootPunish(t)
	gm := srv.Client
	enterWorld(t, gm)
	ch := srv.SeedCharacterFor(t, "player2", "Player", 1, 0)
	setPunishment(t, srv, ch.ID, punishJail, 0)
	if _, err := srv.DB.ExecContext(context.Background(), "UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?", jailAt[0], jailAt[1], jailAt[2], ch.ID); err != nil {
		t.Fatalf("seed position: %v", err)
	}
	user := srv.DialClient(t, "player2", 1)
	burst(t, user)
	drain(t, gm)

	release := srv.HoldPersistenceLane(t, ch.ID)
	user.Send(encodeLogout())
	leftWorld(t, srv, "Player")
	assertTexts(t, exchange(t, gm, encodeBuildCmd("unjail Player")), "Player has been unjailed.")
	release()
	srv.FlushPersistence(t)

	if level, _ := storedPunishment(t, srv, ch.ID); level != punishNone {
		t.Fatalf("stored punishment = %d, want %d", level, punishNone)
	}
	if at := storedPosition(t, srv, ch.ID); at != floranAt {
		t.Fatalf("stored position = %v, want Floran %v", at, floranAt)
	}
}

// storedAdena reads the adena objID's item rows hold.
func storedAdena(t *testing.T, srv *gameservertest.Server, objID int32) int64 {
	t.Helper()
	var n int64
	if err := srv.DB.QueryRowContext(context.Background(), "SELECT COALESCE(SUM(count), 0) FROM items WHERE owner_id = ? AND item_id = 57", objID).Scan(&n); err != nil {
		t.Fatalf("read adena: %v", err)
	}
	return n
}

// parkQueue posts a task that blocks q until the returned release is
// called (also on cleanup), and waits for it to start.
func parkQueue(t *testing.T, q *sim.Queue) (release func()) {
	t.Helper()
	gate, parked := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release = func() { once.Do(func() { close(gate) }) }
	t.Cleanup(release)
	q.Post(func() {
		close(parked)
		<-gate
	})
	<-parked
	return release
}

// commandBehindLogout boots the GM "Admin" and the online "Player", selected
// by the GM, and has the GM's command line run after Player's logout check
// and before its detach, so what the command posts to Player's queue runs
// once the detach began. The executor is inline, one FIFO for every queue,
// and its clock is frozen, so a queue's accepted count moves only for the
// work the test drives: Player's queue is parked ahead of the logout check,
// then again ahead of the command, until the detach is queued behind both.
// It returns the server and Player's object id once Player left the world.
func commandBehindLogout(t *testing.T, line string) (*gameservertest.Server, int32) {
	t.Helper()
	srv, userID, _ := commandsBehindLogout(t, nil, line)
	return srv, userID
}

// commandsBehindLogout is commandBehindLogout running the GM's command lines
// before, answers drained, while Player is still in the world, then lines
// behind Player's logout check, each sent by a GM of its own: the first by
// "Admin", the others by GMs entered for them, which name their target, as
// a connection reads its next packet only once its queue ran the last one.
// It also returns every frame the GMs were sent from lines on, GM by GM.
func commandsBehindLogout(t *testing.T, before []string, lines ...string) (*gameservertest.Server, int32, [][]byte) {
	t.Helper()
	srv, gmID := bootAdmin(t, adminLevel)
	if !srv.DrivesClock() {
		t.Skip("counting queue posts across a parked detach needs the one-FIFO inline executor")
	}
	gm := srv.Client
	enterWorld(t, gm)
	user, userID := addPlayer(t, srv, "player2", "Player", userLevel)
	gms, gmQueues := []*testsupport.ScriptedClient{gm}, []*sim.Queue{srv.PlayerQueue(t, gmID)}
	for i := 1; i < len(lines); i++ {
		c, id := addPlayer(t, srv, "gm"+strconv.Itoa(i), "Admin"+strconv.Itoa(i), adminLevel)
		gms, gmQueues = append(gms, c), append(gmQueues, srv.PlayerQueue(t, id))
	}
	for _, c := range gms {
		drain(t, c)
	}
	exchange(t, gm, encodeAction(userID))
	for _, line := range before {
		exchange(t, gm, encodeBuildCmd(line))
	}
	drain(t, user)

	// Player's connection waits on its logout check, so it never returns to
	// reading while the queue is parked: the waits below count queue posts.
	userQueue := srv.PlayerQueue(t, userID)
	first := parkQueue(t, userQueue)
	n := userQueue.Accepted()
	user.Send(encodeLogout())
	srv.AwaitQueueAccepted(t, userQueue, n) // the logout check
	gate, parked := make(chan struct{}), make(chan struct{})
	var opened sync.Once
	second := func() { opened.Do(func() { close(gate) }) }
	t.Cleanup(second)
	userQueue.Post(func() {
		close(parked)
		<-gate
	})
	for i, line := range lines {
		n = gmQueues[i].Accepted()
		gms[i].Send(encodeBuildCmd(line))
		srv.AwaitQueueAccepted(t, gmQueues[i], n) // the command
	}
	n = userQueue.Accepted()
	first()
	<-parked
	srv.AwaitQueueAccepted(t, userQueue, n) // the detach
	second()

	if !user.AwaitClose(5 * time.Second) {
		t.Fatal("server kept the connection open after Logout")
	}
	leftWorld(t, srv, "Player")
	srv.Settle(t)
	srv.FlushPersistence(t)
	var frames [][]byte
	for _, c := range gms {
		frames = append(frames, c.ReadQueued()...)
	}
	return srv, userID, frames
}

// TestAdminPromotionBehindLogoutStored pins a promotion reaching a player
// whose detach began: it does not run on the departed player, which stays
// off the GM list, and the level is stored after its logout save.
func TestAdminPromotionBehindLogoutStored(t *testing.T) {
	t.Parallel()
	srv, userID := commandBehindLogout(t, "set access 7")
	assertGMs(t, exchange(t, srv.Client, encodeGmList()), "Admin")
	if got := readAccessLevel(t, srv, userID); got != adminLevel {
		t.Fatalf("stored access level = %d, want %d", got, adminLevel)
	}
}

// TestAdminGiveBehindLogoutRefused pins an item given to a player whose
// detach began: it is refused, not left in an inventory whose logout
// already flushed it.
func TestAdminGiveBehindLogoutRefused(t *testing.T) {
	t.Parallel()
	srv, userID := commandBehindLogout(t, "give 57 10")
	if got := storedAdena(t, srv, userID); got != 0 {
		t.Fatalf("stored adena of the departed player = %d, want none", got)
	}
}
