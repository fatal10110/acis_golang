package olympiad

import (
	"bytes"
	"context"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// assertArenaChangeRefused has p send "arenachange 0" and requires the
// lone ActionFailed back, p still a plain player standing where it stood.
func (s *spectate) assertArenaChangeRefused(t *testing.T, p *competitor, why string) {
	t.Helper()
	c := s.char(t, p)
	at := c.CurrentLocation()
	got := s.send(t, p, encodeHeroBypass("arenachange 0"))
	if ops := testsupport.FrameOpcodes(got); !bytes.Equal(ops, []byte{serverpackets.OpcodeActionFailed}) {
		t.Fatalf("%s: arenachange answered %x (%q), want only ActionFailed", why, ops, spectatorLines(p.id, got))
	}
	if _, ok := c.SavedLocation(); c.ObserverMode() || c.Invisible() || c.Invul() || c.OlympiadGameID() != -1 || ok {
		t.Fatalf("%s: observer=%v invisible=%v invul=%v game=%d saved=%v, want not watching",
			why, c.ObserverMode(), c.Invisible(), c.Invul(), c.OlympiadGameID(), ok)
	}
	if now := c.CurrentLocation(); now != at {
		t.Fatalf("%s: stands at %+v, want still at %+v", why, now, at)
	}
}

// TestOlympiadSpectateGate pins the owner-approved gate on "arenachange"
// (#5152): the stadium is watched only from within reach of the Olympiad
// manager the player has selected now, and only by a player whose state
// allows it. A manager out of reach, a manager selected and then traded
// for another target, a player in combat, running a store, with karma or
// dead are each refused with ActionFailed and left where they stand; once
// the bar is lifted the same player enters as before.
func TestOlympiadSpectateGate(t *testing.T) {
	t.Parallel()
	s := bootSpectate(t)
	a, w := s.ones[0], s.watcher
	home := s.char(t, w).CurrentLocation()

	far := s.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("OlympiadManagerNpc", 31688),
		location.Location{X: home.X + 600, Y: home.Y, Z: home.Z})
	s.read(t)
	s.send(t, w, encodeSelect(far.ObjectID(), s.char(t, w).CurrentLocation()))
	s.assertArenaChangeRefused(t, w, "manager out of reach")

	s.atManager(t, w)
	s.send(t, w, encodeSelect(a.id, s.char(t, w).CurrentLocation()))
	s.assertArenaChangeRefused(t, w, "another target selected after the manager")

	s.atManager(t, w)
	s.srv.SetPlayerInCombat(t, w.id, true)
	s.assertArenaChangeRefused(t, w, "in combat")
	s.srv.SetPlayerInCombat(t, w.id, false)

	s.srv.SetPlayerOperating(t, w.id, true)
	s.assertArenaChangeRefused(t, w, "running a store")
	s.srv.SetPlayerOperating(t, w.id, false)

	// With nothing barring it any more, the same click watches stadium 0.
	s.send(t, w, encodeHeroBypass("arenachange 0"))
	if c := s.char(t, w); !c.ObserverMode() || c.OlympiadGameID() != 0 || !c.Invisible() || !c.Invul() {
		t.Fatalf("allowed entry: observer=%v game=%d invisible=%v invul=%v, want watching stadium 0",
			c.ObserverMode(), c.OlympiadGameID(), c.Invisible(), c.Invul())
	}

	ch := s.srv.SeedCharacterFor(t, "delta", "Delta", 40, 0)
	if _, err := s.srv.DB.ExecContext(context.Background(), "UPDATE characters SET karma = 100 WHERE obj_Id = ?", ch.ID); err != nil {
		t.Fatalf("seed karma: %v", err)
	}
	d := &competitor{c: s.srv.DialClient(t, "delta", 1), id: ch.ID, name: "Delta"}
	startInWorld(t, d.c)
	s.ones = append(s.ones, d)
	s.read(t)
	s.atManager(t, d)
	s.assertArenaChangeRefused(t, d, "with karma")

	ch = s.srv.SeedCharacterFor(t, "echo", "Echo", 40, 0)
	e := &competitor{c: s.srv.DialClient(t, "echo", 1), id: ch.ID, name: "Echo"}
	startInWorld(t, e.c)
	s.ones = append(s.ones, e)
	s.read(t)
	s.atManager(t, e)
	s.srv.MarkPlayerDead(t, e.id)
	s.assertArenaChangeRefused(t, e, "dead")
}
