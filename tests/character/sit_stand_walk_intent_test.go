package character

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/privatestore"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// encodeSitStandActionUse is RequestActionUse for action 0, the action-bar
// sit/stand toggle.
func encodeSitStandActionUse() []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestActionUse)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// sitPosture is the posture a world player reports.
type sitPosture interface {
	Standing() bool
	SittingNow() bool
}

func playerPosture(t *testing.T, srv *gameservertest.Server, objID int32) sitPosture {
	t.Helper()
	actor, ok := srv.State.Player(objID)
	if !ok {
		t.Fatal("player missing from world state")
	}
	return actor.(sitPosture)
}

// TestStandRequestWhileWalkingGoesIdle pins #4058: a stand request from a
// standing player who is walking is refused in the think step, which sends
// the player idle first: the walk stops (StopMove to the player and every
// observer), then ActionFailed, and no ChangeWaitType. A sit queued behind
// the walk is dropped with it, so the arrival sits nobody.
func TestStandRequestWhileWalkingGoesIdle(t *testing.T) {
	t.Parallel()
	for _, queuedSit := range []bool{false, true} {
		name := "walking"
		if queuedSit {
			name = "walking with a queued sit"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			srv, c, observer, objID, _ := bootSitObserverPair(t)
			startSitWalk(t, c, observer)
			if queuedSit {
				c.Send(encodeRequestChangeWaitType(false))
				if got := sitResponseOpcodes(c.ReadQueued()); len(got) != 0 {
					t.Fatalf("sit during the walk answered %#x, want nothing", got)
				}
			}

			c.Send(encodeRequestChangeWaitType(true))
			want := []byte{serverpackets.OpcodeStopMove, serverpackets.OpcodeActionFailed}
			if got := sitResponseOpcodes(c.ReadQueued()); !slices.Equal(got, want) {
				t.Fatalf("stand while walking = %#x, want %#x", got, want)
			}
			if got := sitResponseOpcodes(observer.ReadQueued()); !slices.Equal(got, want[:1]) {
				t.Fatalf("observer frames = %#x, want %#x", got, want[:1])
			}
			if srv.PlayerMove(t, objID).Moving() {
				t.Fatal("player still walking after the refused stand")
			}
			srv.SettlePosture(t, objID)
			if got := sitResponseOpcodes(c.ReadQueued()); len(got) != 0 {
				t.Fatalf("after the refused stand the player got %#x, want nothing", got)
			}
			if !playerPosture(t, srv, objID).Standing() {
				t.Fatal("player not standing after the refused stand")
			}
		})
	}
}

// TestSitRequestDuringWalkRunsOnArrival pins #4063: a sit request, by the
// sit key or the action-bar toggle, from a player walking to a point waits
// as the next intention: the walk goes on with no StopMove and no answer,
// and the player sits down where the walk ends, observers seeing the
// ChangeWaitType alone.
func TestSitRequestDuringWalkRunsOnArrival(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		request []byte
	}{
		{"RequestChangeWaitType(sit)", encodeRequestChangeWaitType(false)},
		{"RequestActionUse(0)", encodeSitStandActionUse()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv, c, observer, objID, _ := bootSitObserverPair(t)
			startSitWalk(t, c, observer)

			c.Send(tc.request)
			if got := sitResponseOpcodes(c.ReadQueued()); len(got) != 0 {
				t.Fatalf("sit during the walk answered %#x, want nothing", got)
			}
			if got := sitResponseOpcodes(observer.ReadQueued()); len(got) != 0 {
				t.Fatalf("observer saw %#x during the walk, want nothing", got)
			}
			if !srv.PlayerMove(t, objID).Moving() {
				t.Fatal("the sit request stopped the walk")
			}
			if !playerPosture(t, srv, objID).Standing() {
				t.Fatal("player sat down while walking")
			}

			waitForWorldPosition(t, srv, objID, sitWalkTarget)
			srv.AdvanceUntil(t, "walk end", func() bool { return !srv.PlayerMove(t, objID).Moving() })
			want := []byte{serverpackets.OpcodeChangeWaitType}
			if got := sitResponseOpcodes(c.ReadQueued()); !slices.Equal(got, want) {
				t.Fatalf("arrival frames = %#x, want %#x", got, want)
			}
			if got := sitResponseOpcodes(observer.ReadQueued()); !slices.Equal(got, want) {
				t.Fatalf("observer arrival frames = %#x, want %#x", got, want)
			}
			posture := playerPosture(t, srv, objID)
			if posture.Standing() || !posture.SittingNow() {
				t.Fatalf("after arrival standing=%v sittingNow=%v, want sitting down", posture.Standing(), posture.SittingNow())
			}
			if x, y, z := srv.PlayerPosition(t, objID); x != sitWalkTarget.X || y != sitWalkTarget.Y || z != sitWalkTarget.Z {
				t.Fatalf("sat at (%d,%d,%d), want the walk destination %+v", x, y, z, sitWalkTarget)
			}
		})
	}
}

// TestSitRequestRefusedDuringStoreSetUp pins the operating refusal of a sit:
// a player setting up a private store is answered ActionFailed alone (it
// goes idle, with nothing under way to stop) and keeps standing in the
// set-up. A stand in the same state is refused only as a stand while not
// seated, the store set-up not counting for a stand.
func TestSitRequestRefusedDuringStoreSetUp(t *testing.T) {
	t.Parallel()
	srv, c, observer, objID, _ := bootSitObserverPair(t)
	c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreManageSell).Bytes())
	if !slices.ContainsFunc(c.ReadQueued(), func(f []byte) bool { return f[0] == serverpackets.OpcodePrivateStoreManageListSell }) {
		t.Fatal("sell set-up window did not open")
	}
	if got := srv.PlayerOperateType(t, objID); got != privatestore.OperateSellManage {
		t.Fatalf("operate type = %v, want sell set-up", got)
	}
	observer.ReadQueued()

	for _, stand := range []bool{false, true} {
		c.Send(encodeRequestChangeWaitType(stand))
		want := []byte{serverpackets.OpcodeActionFailed}
		if got := sitResponseOpcodes(c.ReadQueued()); !slices.Equal(got, want) {
			t.Fatalf("stand=%v during the store set-up = %#x, want %#x", stand, got, want)
		}
		if got := sitResponseOpcodes(observer.ReadQueued()); len(got) != 0 {
			t.Fatalf("stand=%v: observer saw %#x, want nothing", stand, got)
		}
		if posture := playerPosture(t, srv, objID); !posture.Standing() || posture.SittingNow() {
			t.Fatalf("stand=%v: standing=%v sittingNow=%v, want standing", stand, posture.Standing(), posture.SittingNow())
		}
		if got := srv.PlayerOperateType(t, objID); got != privatestore.OperateSellManage {
			t.Fatalf("stand=%v: operate type = %v, want sell set-up kept", stand, got)
		}
	}
}
