package olympiad

import (
	"fmt"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
)

// sendUnlanded has p send payload and returns what p was sent for it,
// leaving p's own teleports unanswered so a jump stays in flight; everyone
// else's are answered with Appearing as usual.
func (s *spectate) sendUnlanded(t *testing.T, p *competitor, payload []byte) [][]byte {
	t.Helper()
	p.c.Send(payload)
	s.srv.Advance(t, 50*time.Millisecond)
	var got [][]byte
	for _, o := range s.ones {
		if o == p {
			got = append(got, s.srv.ReadQueued(t, p.c)...)
			continue
		}
		for _, f := range s.srv.ReadQueued(t, o.c) {
			o.frames = append(o.frames, f)
			if f[0] == serverpackets.OpcodeTeleportToLocation && wire.NewReader(f[1:]).ReadInt32() == o.id {
				o.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
			}
		}
	}
	return got
}

// TestOlympiadSpectateWaitsForTheEntryJump pins the Go-only guard on a
// spectator whose jump into a stadium its client has not yet reported
// landed: RequestOlympiadObserverEnd and "arenachange" to another stadium
// are both ignored, as the jump they would start is refused while the
// first is in flight, and would otherwise leave a plain player inside the
// stadium or a hidden spectator of one stadium standing in another. Once
// it has appeared, leaving brings it back where it left.
func TestOlympiadSpectateWaitsForTheEntryJump(t *testing.T) {
	t.Parallel()
	s := bootSpectate(t)
	w := s.watcher
	s.atManager(t, w)
	c := s.char(t, w)
	left := c.CurrentLocation()

	entry := spectatorLines(w.id, s.sendUnlanded(t, w, encodeHeroBypass("arenachange 0")))
	if want := []string{fmt.Sprintf("Teleport %d %d", spectatorSpot1.X, spectatorSpot1.Y), "Mode 3"}; !subsequence(entry, want) {
		t.Fatalf("Charlie's entry = %q, want %q", entry, want)
	}
	if !c.Teleporting() {
		t.Fatal("entry jump not in flight before Appearing")
	}

	for _, early := range []struct {
		name    string
		payload []byte
	}{
		{"RequestOlympiadObserverEnd", encodeExRequest(clientpackets.OpcodeRequestOlympiadObserverEnd)},
		{"arenachange 1", encodeHeroBypass("arenachange 1")},
	} {
		if got := s.sendUnlanded(t, w, early.payload); len(got) != 0 {
			t.Fatalf("%s before Appearing answered %q, want silence", early.name, spectatorLines(w.id, got))
		}
		if saved, ok := c.SavedLocation(); !c.ObserverMode() || !c.Invisible() || !c.Invul() || c.OlympiadGameID() != 0 || !ok || saved != left {
			t.Fatalf("after early %s: observer=%v invisible=%v invul=%v game=%d saved=%+v/%v, want watching stadium 0 from %+v",
				early.name, c.ObserverMode(), c.Invisible(), c.Invul(), c.OlympiadGameID(), saved, ok, left)
		}
	}

	w.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	s.srv.Advance(t, 50*time.Millisecond)
	s.read(t)
	if c.Teleporting() {
		t.Fatal("entry jump still in flight after Appearing")
	}
	if at := c.CurrentLocation(); at.X != spectatorSpot1.X || at.Y != spectatorSpot1.Y {
		t.Fatalf("after Appearing Charlie stands at %+v, want on %+v", at, spectatorSpot1)
	}

	got := spectatorLines(w.id, s.send(t, w, encodeExRequest(clientpackets.OpcodeRequestOlympiadObserverEnd)))
	if leave := []string{"Mode 0", fmt.Sprintf("Teleport %d %d", left.X, left.Y)}; !subsequence(got, leave) {
		t.Fatalf("Charlie's leave after Appearing = %q, want %q", got, leave)
	}
	if _, ok := c.SavedLocation(); c.ObserverMode() || c.Invisible() || c.Invul() || c.OlympiadGameID() != -1 || ok {
		t.Fatalf("after leaving: observer=%v invisible=%v invul=%v game=%d saved=%v, want none",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.OlympiadGameID(), ok)
	}
	if at := c.CurrentLocation(); at.X != left.X || at.Y != left.Y {
		t.Fatalf("after leaving Charlie stands at %+v, want back at %+v", at, left)
	}
}
