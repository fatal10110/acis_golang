package network

import (
	"testing"

	"github.com/rs/zerolog"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/staticobject"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/skill/effect"
	"github.com/fatal10110/acis_golang/internal/gameserver/world"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestRequestChangeWaitTypeRejectionSendsActionFailed pins Gap 1: a rejected
// sit/stand request (here, standing up while already standing, mirroring the
// reference's thinkStand guard) must release the client with ActionFailed
// instead of silently dropping the frame.
func TestRequestChangeWaitTypeRejectionSendsActionFailed(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	gcl := &GameClientLink{log: zerolog.Nop()}

	gcl.requestChangeWaitType(live, true)

	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("rejected stand opcodes = %x, want ActionFailed", got)
	}
	if !live.Standing() {
		t.Fatal("live player stopped standing after a rejected stand request")
	}
}

// TestRequestChangeWaitTypeSitTargetsClaimableChair pins Gap 2: the sit key
// (RequestChangeWaitType with Stand == false) must bind a claimable targeted
// throne exactly like the click path does.
func TestRequestChangeWaitTypeSitTargetsClaimableChair(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	testsupport.ResetCapture(frames)
	live.SetTargetTracked(chair)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.requestChangeWaitType(live, false)

	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeChairSit}) {
		t.Fatalf("sit-key chair opcodes = %x, want ChangeWaitType, ChairSit", got)
	}
	if !chair.Busy() {
		t.Fatal("chair was not marked busy after sit key targeted it")
	}
}

// TestRequestChangeWaitTypeSitFallsBackWhenChairUnclaimable pins the
// reference's unconditional sitDown() ahead of its chair check: an
// unclaimable targeted chair (here, already busy) must not block the sit,
// it just leaves the player sitting on the ground instead of the throne.
func TestRequestChangeWaitTypeSitFallsBackWhenChairUnclaimable(t *testing.T) {
	state := world.New()
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	chair, err := staticobject.NewObject(2, &staticobject.Template{
		ID:       777,
		Location: location.Location{X: 100, Y: 0, Z: 0},
		Type:     1,
	})
	if err != nil {
		t.Fatalf("NewObject: %v", err)
	}
	chair.SetBusy(true)

	state.Spawn(live, 0, 0, 0, 0)
	state.Spawn(chair, 100, 0, 0, 0)
	testsupport.ResetCapture(frames)
	live.SetTargetTracked(chair)

	gcl := &GameClientLink{world: state, log: zerolog.Nop()}
	gcl.requestChangeWaitType(live, false)

	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType}) {
		t.Fatalf("sit-key opcodes with unclaimable chair = %x, want plain ChangeWaitType only", got)
	}
	if live.Standing() {
		t.Fatal("live player did not sit after an unclaimable chair target")
	}
}

// TestRequestChangeWaitTypeStandStopsFakeDeath pins the reference's
// thinkStand fake-death branch: a stand request while fake-dead must stop
// the FAKE_DEATH toggle effect (broadcasting the revive visual) instead of
// being rejected as if the player were really dead. The fake-death
// stance/revive broadcasts run through the hooks attachLivePlayer wires in
// production (character_flow.go); wire them the same way here rather than
// through the full client-packet dispatch loop, to keep the frame sequence
// deterministic.
func TestRequestChangeWaitTypeStandStopsFakeDeath(t *testing.T) {
	frames := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, frames)
	gcl := &GameClientLink{log: zerolog.Nop()}
	live.SetStanceBroadcaster(func(stance player.Stance) {
		waitType := serverpackets.WaitSitting
		switch stance {
		case player.StanceStanding:
			waitType = serverpackets.WaitStanding
		case player.StanceFakeDeathStart:
			waitType = serverpackets.WaitFakeDeathStart
		case player.StanceFakeDeathStop:
			waitType = serverpackets.WaitFakeDeathStop
		}
		x, y, z := live.Position()
		gcl.broadcastLiveFrame(live, func() wire.Frame {
			return serverpackets.FrameChangeWaitType(live.ObjectID(), waitType, location.Location{X: x, Y: y, Z: z})
		})
	})
	live.SetFakeDeathReviveBroadcaster(func() { gcl.broadcastLiveRevive(live) })

	e, err := effect.New(effect.Skill{}, modelskill.EffectTemplate{Name: "FakeDeath"})
	if err != nil {
		t.Fatalf("effect.New(FakeDeath): %v", err)
	}
	e.Effected = live.Character
	live.EffectList().Add(e)
	if !live.FakeDead() {
		t.Fatal("live player not fake-dead after FakeDeath effect start")
	}
	testsupport.ResetCapture(frames)

	gcl.requestChangeWaitType(live, true)

	if got := testsupport.FrameOpcodes(frames.Frames()); string(got) != string([]byte{serverpackets.OpcodeChangeWaitType, serverpackets.OpcodeRevive}) {
		t.Fatalf("stand-during-fake-death opcodes = %x, want ChangeWaitType, Revive", got)
	}
	r := wire.NewReader(frames.Frames()[0][1:])
	r.ReadInt32()
	if got := r.ReadInt32(); got != int32(serverpackets.WaitFakeDeathStop) {
		t.Fatalf("stand-during-fake-death wait type = %d, want %d", got, serverpackets.WaitFakeDeathStop)
	}
	if live.FakeDead() {
		t.Fatal("live player still fake-dead after a stand request")
	}
	if !live.Standing() {
		t.Fatal("live player did not stand after stopping fake death")
	}
}

func TestGameClientLinkAutoAttackStanceRefreshAndStop(t *testing.T) {
	capture := &testsupport.FrameCapture{}
	live := newTestLivePlayer(t, 1, capture)
	tracker := &attackStanceRecorder{}
	link := &GameClientLink{attackStance: tracker}

	link.startLiveAutoAttack(live)
	if len(tracker.actors) != 1 || tracker.actors[0].ObjectID() != live.ObjectID() {
		t.Fatalf("attack stance actors = %+v, want live player", tracker.actors)
	}
	if !live.InCombat() {
		t.Fatal("live player not marked in combat after AutoAttackStart")
	}
	if len(capture.Frames()) != 1 || capture.Frames()[0][0] != serverpackets.OpcodeAutoAttackStart {
		t.Fatalf("start frames = %x, want one AutoAttackStart", capture.Frames())
	}

	link.startLiveAutoAttack(live)
	if len(tracker.actors) != 2 {
		t.Fatalf("attack stance refresh count = %d, want 2", len(tracker.actors))
	}
	if len(capture.Frames()) != 1 {
		t.Fatalf("second start emitted %d frames, want no duplicate AutoAttackStart", len(capture.Frames())-1)
	}

	link.stopLiveAutoAttack(live)
	if live.InCombat() {
		t.Fatal("live player still marked in combat after AutoAttackStop")
	}
	if len(capture.Frames()) != 2 || capture.Frames()[1][0] != serverpackets.OpcodeAutoAttackStop {
		t.Fatalf("stop frames = %x, want AutoAttackStop", capture.Frames())
	}

	link.stopLiveAutoAttack(live)
	if len(capture.Frames()) != 2 {
		t.Fatalf("second stop emitted %d frames, want no duplicate AutoAttackStop", len(capture.Frames())-2)
	}
}
