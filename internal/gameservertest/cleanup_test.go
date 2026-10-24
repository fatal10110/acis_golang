package gameservertest

import (
	"runtime"
	"testing"
	"weak"

	"github.com/fatal10110/acis_golang/internal/gameserver/world"
)

// A finished test's stack, its character in the world, becomes garbage even
// while the test itself is still held, as testing holds every finished
// parallel test until its parent ends: nothing Boot registered on the test
// keeps the world alive past the cleanups.
func TestFinishedTestLetsGoOfItsWorld(t *testing.T) {
	var held *testing.T
	var state weak.Pointer[world.State]
	t.Run("boot", func(t *testing.T) {
		held = t
		srv := Boot(t, WithCharacter("Newbie", 1, 0), WithWantChars(1))
		state = weak.Make(srv.State)
		srv.Client.Send(encodeRequestGameStart(0))
		srv.Client.Read() // SSQInfo
		srv.Client.Read() // CharSelected
		srv.Client.Send(encodeEnterWorld())
		readEnterWorldBurst(t, srv.Client)
	})
	// A goroutine that signalled its stop may not have returned yet, its
	// stack still holding the world for a moment; give it a few rounds.
	for i := 0; i < 10 && state.Value() != nil; i++ {
		runtime.GC()
		runtime.Gosched()
	}
	if state.Value() != nil {
		t.Fatal("the finished test's world is still reachable")
	}
	runtime.KeepAlive(held)
}
