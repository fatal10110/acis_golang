package character

import (
	"bytes"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

func encodeRequestChangeMoveType(run bool) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeRequestChangeMoveType)
	w.WriteInt32(wire.BoolInt32(run))
	return w.Bytes()
}

func assertStanceRefresh(t *testing.T, self, observer *testsupport.ScriptedClient, objectID int32, run bool) {
	t.Helper()
	ownMove := self.Read()
	otherMove := observer.Read()
	var wantRunning int32
	if run {
		wantRunning = 1
	}
	for _, frame := range [][]byte{ownMove, otherMove} {
		if frame[0] != serverpackets.OpcodeChangeMoveType {
			t.Fatalf("first stance frame opcode = %#x, want ChangeMoveType", frame[0])
		}
		r := wire.NewReader(frame[1:])
		if id, running, swimming := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != objectID || running != wantRunning || swimming != 0 {
			t.Fatalf("ChangeMoveType fields = (%d,%d,%d), want (%d,%d,0)", id, running, swimming, objectID, wantRunning)
		}
	}
	if !bytes.Equal(ownMove, otherMove) {
		t.Fatal("stance broadcast differs between recipients")
	}
	otherCopy := bytes.Clone(otherMove)
	ownMove[1] ^= 0xff
	if !bytes.Equal(otherMove, otherCopy) {
		t.Fatal("mutating self stance frame changed observer frame")
	}
	frame := self.ReadWithTimeout(time.Second)
	if frame == nil {
		t.Fatal("stance change sent no UserInfo to self")
	}
	if frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("second self frame opcode = %#x, want UserInfo", frame[0])
	}
	frame = observer.ReadWithTimeout(time.Second)
	if frame == nil {
		t.Fatal("stance change sent no CharInfo to observer")
	}
	if frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("second observer frame opcode = %#x, want CharInfo", frame[0])
	}
}

func TestPlayerStanceChangeRefreshesSelfAndObserver(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1))
	self := srv.Client
	enterWorld(t, self)
	srv.SeedCharacterFor(t, "observer", "Observer", 1, 0)
	observer := srv.DialClient(t, "observer", 1)
	enterWorld(t, observer)
	drainQuiet(t, self)
	drainQuiet(t, observer)
	objectID := srv.SoleObjectID(t)

	self.Send(encodeRequestChangeMoveType(false))
	assertStanceRefresh(t, self, observer, objectID, false)
	self.Send(encodeRequestChangeMoveType(false))
	if frame := self.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("unchanged walk stance sent self opcode %#x", frame[0])
	}
	if frame := observer.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("unchanged walk stance sent observer opcode %#x", frame[0])
	}
	self.Send(encodeRequestChangeMoveType(true))
	assertStanceRefresh(t, self, observer, objectID, true)
}

func TestZeroSpeedStanceSkipsMoveTypeButRefreshesInfo(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithCharacter("Newbie", 1, 0), gameservertest.WithWantChars(1), gameservertest.WithWeightLimitMultiplier(1))
	self := srv.Client
	objectID := srv.SoleObjectID(t)
	srv.GiveItem(t, objectID, 9500, 100_000) // weight penalty 4, so move speed is zero
	self.Send(encodeRequestGameStart(0))
	self.Read() // SSQInfo
	self.Read() // CharSelected
	self.Send(encodeEnterWorld())
	// The overload StatusUpdate and band refresh ride inside the world-entry burst.
	drainQuiet(t, self)
	srv.SeedCharacterFor(t, "observer", "Observer", 1, 0)
	observer := srv.DialClient(t, "observer", 1)
	enterWorld(t, observer)
	drainQuiet(t, self)
	drainQuiet(t, observer)
	player, ok := srv.State.Player(objectID)
	if !ok {
		t.Fatal("overloaded player missing from world")
	}
	loaded := player.(interface {
		CurrentWeight() int
		WeightLimit() int
		WeightPenalty() int
		Running() bool
	})
	if loaded.CurrentWeight() <= loaded.WeightLimit() {
		t.Fatalf("restored weight %d does not exceed limit %d", loaded.CurrentWeight(), loaded.WeightLimit())
	}
	if got := loaded.WeightPenalty(); got != 4 {
		t.Fatalf("weight penalty = %d, want 4 (weight %d, limit %d)", got, loaded.CurrentWeight(), loaded.WeightLimit())
	}
	drainQuiet(t, self)
	drainQuiet(t, observer)

	self.Send(encodeRequestChangeMoveType(false))
	frame := self.ReadWithTimeout(time.Second)
	if frame == nil || frame[0] != serverpackets.OpcodeUserInfo {
		t.Fatalf("zero-speed stance self frame = %x, want UserInfo first", frame)
	}
	frame = observer.ReadWithTimeout(time.Second)
	if frame == nil || frame[0] != serverpackets.OpcodeCharInfo {
		t.Fatalf("zero-speed stance observer frame = %x, want CharInfo first", frame)
	}
	if frame := self.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("zero-speed stance sent extra self opcode %#x", frame[0])
	}
	if frame := observer.ReadWithTimeout(rejectSilenceWindow); frame != nil {
		t.Fatalf("zero-speed stance sent extra observer opcode %#x", frame[0])
	}
	if loaded.Running() {
		t.Fatal("zero-speed stance left the player running")
	}
}
