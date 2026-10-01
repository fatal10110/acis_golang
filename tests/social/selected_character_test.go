package social

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// selectOnly selects c's character in slot 0 and stops before EnterWorld:
// the character is on its loading screen. It returns once the selection is
// handled, so the character's registration is in place.
func selectOnly(t *testing.T, srv *gameservertest.Server, c *testsupport.ScriptedClient) {
	t.Helper()
	c.Send(encodeRequestGameStart(0))
	assertOpcode(t, c.Read(), serverpackets.OpcodeSSQInfo, "SSQInfo")
	assertOpcode(t, c.Read(), serverpackets.OpcodeCharSelected, "CharSelected")
	srv.AwaitHandled(t)
}

func encodeAction(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAction)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// TestSelectedCharacterIsFoundBeforeEnterWorld pins a character between its
// selection and EnterWorld: lookups by name and id find it, so a whisper
// and a friend's private message reach its client and the friend's
// /friendlist shows it online; it is not spawned yet, so the friend neither sees nor can click it,
// and its friends hear of it only when it enters.
func TestSelectedCharacterIsFoundBeforeEnterWorld(t *testing.T) {
	p := bootPair(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	startInWorld(t, p.alice)
	drainUntilQuiet(t, p.alice)

	selectOnly(t, p.srv, p.bobby)
	if _, ok := p.srv.State.Player(p.bobbyID); !ok {
		t.Fatal("selected character not found by id")
	}
	if obj, ok := p.srv.State.PlayerByName("bobby"); !ok || obj.ObjectID() != p.bobbyID {
		t.Fatal("selected character not found by name")
	}
	if _, ok := p.srv.State.Object(p.bobbyID); ok {
		t.Fatal("selected character spawned before EnterWorld")
	}
	assertSilent(t, p.alice, "friend of a character that only selected")

	p.alice.Send(encodeTell("psst", "bobby"))
	assertSay(t, p.bobby, p.aliceID, sayTell, "Alice", "psst")
	assertSay(t, p.alice, p.aliceID, sayTell, "->Bobby", "psst")
	assertSilent(t, p.alice, "sender of a whisper to a selected character")

	p.alice.Send(encodeFriendSay("hello", "bobby"))
	assertFriendSay(t, p.bobby.Read(), 0, "bobby", "Alice", "hello")
	assertSilent(t, p.alice, "sender of a message to a selected character")

	p.alice.Send(encodeFriendList())
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListHeader)
	assertSystemMessageText(t, p.alice.Read(), serverpackets.SystemMessageS1Online, "Bobby")
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageFriendListFooter)

	p.alice.Send(encodeAction(p.bobbyID))
	assertOpcode(t, p.alice.Read(), serverpackets.OpcodeActionFailed, "click on a selected character")
	assertSilent(t, p.alice, "click on a selected character")

	p.bobby.Send(wire.NewPacketWriter(clientpackets.OpcodeEnterWorld).Bytes())
	readEnterWorldBurst(t, p.bobby)
	if _, ok := p.srv.State.Object(p.bobbyID); !ok {
		t.Fatal("character not spawned after EnterWorld")
	}
	frames := drainFrames(t, p.alice)
	i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeL2FriendStatus })
	if i < 0 || i+1 >= len(frames) {
		t.Fatalf("friend got no L2FriendStatus then message at EnterWorld: %x", opcodes(frames))
	}
	assertFriendStatus(t, frames[i], true, "Bobby", p.bobbyID)
	assertSystemMessageText(t, frames[i+1], serverpackets.SystemMessageFriendS1HasLoggedIn, "Bobby")
}

// TestDisconnectBeforeEnterWorldLeavesNothingRegistered pins a connection
// lost on the loading screen: the selected character leaves the world as a
// logout takes it out, so nothing of it stays registered, its actor queue
// is closed, its row is marked offline, and its friends get the offline
// status a logout sends.
func TestDisconnectBeforeEnterWorldLeavesNothingRegistered(t *testing.T) {
	p := bootPair(t)
	p.srv.Relations.AddFriend(p.aliceID, p.bobbyID)
	startInWorld(t, p.alice)
	drainUntilQuiet(t, p.alice)
	openQueues := p.srv.OpenActorQueues()

	selectOnly(t, p.srv, p.bobby)
	if got := p.srv.OpenActorQueues(); got != openQueues+1 {
		t.Fatalf("open actor queues after the selection = %d, want %d", got, openQueues+1)
	}
	if online := onlineFlag(t, p, p.bobbyID); online != 1 {
		t.Fatalf("online flag after the selection = %d, want 1", online)
	}

	if err := p.bobby.Close(); err != nil {
		t.Fatalf("close the selecting client: %v", err)
	}
	p.awaitOffline(t, p.bobbyID)
	p.srv.Settle(t)
	p.srv.FlushPersistence(t)
	if _, ok := p.srv.State.PlayerByName("Bobby"); ok {
		t.Fatal("character still found by name after its connection dropped")
	}
	if _, ok := p.srv.State.Object(p.bobbyID); ok {
		t.Fatal("character spawned by a connection that dropped before EnterWorld")
	}
	if got := p.srv.OpenActorQueues(); got != openQueues {
		t.Fatalf("open actor queues after the drop = %d, want %d", got, openQueues)
	}
	if online := onlineFlag(t, p, p.bobbyID); online != 0 {
		t.Fatalf("online flag after the drop = %d, want 0", online)
	}
	frames := drainFrames(t, p.alice)
	i := slices.IndexFunc(frames, func(f []byte) bool { return f[0] == serverpackets.OpcodeL2FriendStatus })
	if i < 0 {
		t.Fatalf("friend got no L2FriendStatus at the drop: %x", opcodes(frames))
	}
	assertFriendStatus(t, frames[i], false, "Bobby", p.bobbyID)

	p.alice.Send(encodeTell("psst", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
	p.alice.Send(encodeFriendSay("hello", "Bobby"))
	assertStaticSystemMessage(t, p.alice.Read(), serverpackets.SystemMessageTargetNotFound)
}

// TestSelectionSeesCharacterStillEntering holds a character on its loading
// screen in the world after its account is taken over: selecting it again
// answers nothing until the entering session has left. Once it has, the
// selection goes through.
func TestSelectionSeesCharacterStillEntering(t *testing.T) {
	t.Parallel()
	p := bootPair(t, gameservertest.WithRealPool())
	selectOnly(t, p.srv, p.bobby)

	queue := p.srv.PlayerQueue(t, p.bobbyID)
	held, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(letGo) // a failure below must not leave the queue held at shutdown
	if !queue.Post(func() { close(held); <-release }) {
		t.Fatal("post to the entering character's queue: queue closed")
	}
	<-held

	// The takeover closes the entering session; its detach waits on the
	// held queue, so the character stays registered.
	c := p.srv.DialClient(t, "player2", 1)
	if !p.bobby.AwaitClose(silenceWindow * 10) {
		t.Fatal("account takeover left the entering session open")
	}
	c.Send(encodeRequestGameStart(0))
	p.srv.AwaitHandled(t)
	if frames := drainFrames(t, c); len(frames) != 0 {
		t.Fatalf("selection with the character still entering answered %x, want nothing", opcodes(frames))
	}

	letGo()
	p.srv.AdvanceUntil(t, "entering character out of the world", func() bool {
		_, ok := p.srv.State.Player(p.bobbyID)
		return !ok
	})
	selectOnly(t, p.srv, c)
	if _, ok := p.srv.State.Player(p.bobbyID); !ok {
		t.Fatal("character not registered after its second selection")
	}
}

func onlineFlag(t *testing.T, p *pair, objID int32) int {
	t.Helper()
	var online int
	if err := p.srv.DB.QueryRowContext(context.Background(), "SELECT online FROM characters WHERE obj_Id = ?", objID).Scan(&online); err != nil {
		t.Fatalf("read online flag of %d: %v", objID, err)
	}
	return online
}

func opcodes(frames [][]byte) []byte {
	out := make([]byte, len(frames))
	for i, f := range frames {
		out[i] = f[0]
	}
	return out
}
