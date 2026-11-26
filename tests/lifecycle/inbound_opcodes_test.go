package lifecycle

import (
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
	"github.com/fatal10110/acis_golang/internal/testsupport/clientwire"
)

// manorListFence is a request every in-world client is answered at once:
// the answer arriving first proves the frames sent before it were read and
// answered nothing.
func manorListFence(t *testing.T, c *testsupport.ScriptedClient, what string) {
	t.Helper()
	c.Send([]byte{clientpackets.OpcodeExtended, byte(clientpackets.OpcodeRequestManorList), 0})
	assertFrameOpcode(t, c.Read(), serverpackets.OpcodeExtended, what)
}

// TestFillerOpcodesAnswerNothingInGame sends the client's filler packets and
// its game-guard reply in the world, many times over: none is answered, none
// counts toward the unknown-packet disconnect, and the session stays up.
func TestFillerOpcodesAnswerNothingInGame(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithAccount("filler"), gameservertest.WithCharacter("Filler", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	for _, op := range []byte{
		clientpackets.OpcodeDummy1A, clientpackets.OpcodeDummy23, clientpackets.OpcodeDummy2E,
		clientpackets.OpcodeDummy34, clientpackets.OpcodeDummy3E, clientpackets.OpcodeGameGuardReply,
	} {
		for range 20 {
			c.Send([]byte{op, 1, 2, 3, 4})
		}
	}
	manorListFence(t, c, "manor list after the filler packets")
}

// TestStartRotatingIsBroadcastToSelf turns the player: StartRotation with the
// player's object id, the client's degree and side, and a zero speed goes to
// the player too.
func TestStartRotatingIsBroadcastToSelf(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithAccount("rotator"), gameservertest.WithCharacter("Rotator", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)
	objID := srv.SoleObjectID(t)

	w := wire.NewPacketWriter(clientpackets.OpcodeStartRotating)
	w.WriteInt32(12345)
	w.WriteInt32(1)
	c.Send(w.Bytes())
	frame := c.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeStartRotation, "StartRotating answer")
	r := wire.NewReader(frame[1:])
	if id, degree, side, speed := r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32(); id != objID || degree != 12345 || side != 1 || speed != 0 {
		t.Fatalf("StartRotation = object %d degree %d side %d speed %d, want %d 12345 1 0", id, degree, side, speed, objID)
	}
}

// TestRequestNewCharacterAnswersTemplates asks, at character selection, for
// the creatable classes: the answer lists the ten entries of the creation
// screen (the human fighter twice), each as race, class id and six stat rows
// of three ints.
func TestRequestNewCharacterAnswersTemplates(t *testing.T) {
	t.Parallel()
	var roots []*player.Template
	for _, id := range []int{10, 18, 25, 31, 38, 44, 49, 53} {
		tmpl := gameservertest.ClassTemplate()
		tmpl.ID = id
		roots = append(roots, tmpl)
	}
	srv := gameservertest.Boot(t, gameservertest.WithAccount("creator"), gameservertest.WithCharacter("Creator", 5, 0), gameservertest.WithWantChars(1),
		gameservertest.WithClassTemplates(roots...))
	srv.Client.Send(clientwire.SingleOpcode(clientpackets.OpcodeRequestNewCharacter))
	frame := srv.Client.Read()
	assertFrameOpcode(t, frame, serverpackets.OpcodeNewCharacterSuccess, "RequestNewCharacter answer")
	r := wire.NewReader(frame[1:])
	if count := r.ReadInt32(); count != 10 {
		t.Fatalf("entries = %d, want 10", count)
	}
	var classes []int32
	for range 10 {
		r.ReadInt32() // race
		classes = append(classes, r.ReadInt32())
		for range 6 * 3 {
			r.ReadInt32()
		}
	}
	if want := []int32{0, 0, 10, 18, 25, 31, 38, 44, 49, 53}; !slices.Equal(classes, want) {
		t.Fatalf("classes = %v, want %v", classes, want)
	}
}

// TestUnknownCrestRequestsAreSilent asks the world for an alliance crest and
// a large pledge crest nobody uploaded: nothing is answered.
func TestUnknownCrestRequestsAreSilent(t *testing.T) {
	t.Parallel()
	srv := gameservertest.Boot(t, gameservertest.WithAccount("crests"), gameservertest.WithCharacter("Crests", 5, 0), gameservertest.WithWantChars(1))
	c := srv.Client
	startInWorld(t, c)

	ally := wire.NewPacketWriter(clientpackets.OpcodeRequestAllyCrest)
	ally.WriteInt32(987654)
	c.Send(ally.Bytes())
	large := wire.NewPacketWriter(clientpackets.OpcodeExtended)
	large.WriteUint16(clientpackets.OpcodeRequestExPledgeCrestLarge)
	large.WriteInt32(987654)
	c.Send(large.Bytes())
	manorListFence(t, c, "manor list after the crest requests")
}
