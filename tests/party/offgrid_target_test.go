package party

import (
	"bytes"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// TestSelectOffGridPlayerOnlyInParty pins Player.setTarget's visibility gate
// (Player.java:2443-2447): a player taken off the grid cannot be selected
// unless it is in the selector's party. A refused click answers ActionFailed
// alone with nothing held, ActionFailed then TargetUnselected while a target
// is held, and the selection ends empty; the party mate is selected as
// usual.
func TestSelectOffGridPlayerOnlyInParty(t *testing.T) {
	t.Parallel()
	g := bootGroup(t, []seat{{"Leader", 10}, {"Mate", 10}, {"Stranger", 10}})
	g.invite(t, 0, 1, 0)
	selector, mate, stranger := g.players[0], g.players[1], g.players[2]
	for _, p := range []player{mate, stranger} {
		obj, ok := g.srv.State.Player(p.id)
		if !ok {
			t.Fatalf("%s not online", p.name)
		}
		g.srv.State.Leave(obj)
	}
	g.quiet(t)

	selector.c.Send(encodeAction(stranger.id, 0, 0, 0))
	if got := opcodes(drainFrames(t, selector.c)); !bytes.Equal(got, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("off-grid stranger click with no target = % x, want ActionFailed", got)
	}

	selector.c.Send(encodeAction(mate.id, 0, 0, 0))
	frames := drainFrames(t, selector.c)
	selected := false
	for _, f := range frames {
		switch f[0] {
		case serverpackets.OpcodeActionFailed:
			t.Fatalf("off-grid party mate click = % x, want no ActionFailed", opcodes(frames))
		case serverpackets.OpcodeMyTargetSelected:
			selected = wire.NewReader(f[1:]).ReadInt32() == mate.id
		}
	}
	if !selected {
		t.Fatalf("off-grid party mate click = % x, want MyTargetSelected(%d)", opcodes(frames), mate.id)
	}

	selector.c.Send(encodeAction(stranger.id, 0, 0, 0))
	want := []byte{serverpackets.OpcodeActionFailed, serverpackets.OpcodeTargetUnselected}
	if got := opcodes(drainFrames(t, selector.c)); !bytes.Equal(got, want) {
		t.Fatalf("off-grid stranger click holding a target = % x, want % x", got, want)
	}
	obj, ok := g.srv.State.Player(selector.id)
	if !ok {
		t.Fatal("selector not online")
	}
	ch, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("selector = %T is not an online character", obj)
	}
	if cur := ch.Target(); cur != nil {
		t.Fatalf("selector target = %v, want none", cur)
	}
}
