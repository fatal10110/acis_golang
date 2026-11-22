package party

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// TestSelectUnknownObjectRefused pins #4993: a click on an object outside
// the clicker's known list (two regions away) discloses nothing. It answers
// ActionFailed alone with nothing held, ActionFailed then TargetUnselected
// while a target is held, sends no ValidateLocation, MyTargetSelected or
// StatusUpdate, and shows no TargetSelected to a bystander. A known monster
// and a far party mate stay selectable.
func TestSelectUnknownObjectRefused(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 10}, {"Mate", 10}, {"Stranger", 10}, {"Bystander", 10}})
	g.invite(t, 0, 1, 0)
	selector, mate, stranger, bystander := g.players[0], g.players[1], g.players[2], g.players[3]
	x, y, z := g.srv.PlayerPosition(t, selector.id)
	for _, p := range []player{mate, stranger} {
		obj, _ := g.srv.State.Player(p.id)
		c, ok := network.OnlineCharacter(obj)
		if !ok {
			t.Fatalf("%s is %T", p.name, obj)
		}
		c.TeleportTo(x+5000, y, z, 0)
		p.c.Send(encodeSingle(clientpackets.OpcodeAppearing))
	}
	far := g.srv.SpawnHostileNPCAt(t, location.Location{X: x + 5000, Y: y + 40, Z: z})
	near := g.srv.SpawnHostileNPCAt(t, location.Location{X: x + 60, Y: y + 20, Z: z})
	g.quiet(t)

	refused := func(objectID int32, want []byte, what string) {
		t.Helper()
		selector.c.Send(encodeAction(objectID, 0, 0, 0))
		if got := opcodes(drainFrames(t, selector.c)); !bytes.Equal(got, want) {
			t.Fatalf("%s = % x, want % x", what, got, want)
		}
		for _, f := range drainFrames(t, bystander.c) {
			if f[0] == serverpackets.OpcodeTargetSelected {
				t.Fatalf("%s: bystander received TargetSelected", what)
			}
		}
	}
	refused(stranger.id, []byte{serverpackets.OpcodeActionFailed}, "unknown player click with no target")
	refused(far.ObjectID(), []byte{serverpackets.OpcodeActionFailed}, "unknown monster click with no target")

	assertSelected(t, selector.c, near.ObjectID(), "known monster click")
	drainFrames(t, bystander.c)

	refused(far.ObjectID(), []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeTargetUnselected}, "unknown monster click holding a target")
	obj, _ := g.srv.State.Player(selector.id)
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("selector = %T is not an online character", obj)
	}
	if cur := ch.Target(); cur != nil {
		t.Fatalf("selector target = %v, want none", cur)
	}

	assertSelected(t, selector.c, mate.id, "far party mate click")
}

// assertSelected clicks objectID and asserts it is selected without an
// ActionFailed.
func assertSelected(t *testing.T, c *testsupport.ScriptedClient, objectID int32, what string) {
	t.Helper()
	c.Send(encodeAction(objectID, 0, 0, 0))
	frames := drainFrames(t, c)
	selected := false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed:
			t.Fatalf("%s = % x, want no ActionFailed", what, opcodes(frames))
		case serverpackets.OpcodeMyTargetSelected:
			selected = wire.NewReader(f[1:]).ReadInt32() == objectID
		}
	}
	if !selected {
		t.Fatalf("%s = % x, want MyTargetSelected(%d)", what, opcodes(frames), objectID)
	}
}
