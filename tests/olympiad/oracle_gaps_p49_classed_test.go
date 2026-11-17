package olympiad

import (
	"fmt"
	"slices"
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestOlympiadClassedMatchDrawnInEvenStadium has two nobles of one class
// wait on the class list and, with the class list holding the registrations
// a draw asks for (OLY_CLASSED), the draw puts them into stadium 0, an
// even one, which takes classed matches first (OlympiadGameManager.run,
// OlympiadGameManager.java:36-96; OlympiadGameClassed.createGame): the
// announcement names a class match, not a class-free one
// (OlympiadGameTask), and the fight is stored as classed. The draw-order
// rules of the queues themselves (an offline first or second pick, a queue
// that cannot give a pair) are pinned in TestOlympiadGameDraw
// (game_test.go).
func TestOlympiadClassedMatchDrawnInEvenStadium(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) {
		cfg.AnnounceGames = true
		cfg.ClassedParticipants = 2
	})
	for _, p := range m.ones {
		if got := m.srv.Olympiad.Unregister(p.id, true); got != olympiad.Unregistered {
			t.Fatalf("unregister %s = %v", p.name, got)
		}
		res := m.srv.Olympiad.Register(olympiad.Applicant{ObjectID: p.id, Name: p.name, BaseClass: m.character(t, p).BaseClassID(), Noble: true}, olympiad.Classed)
		if res != olympiad.Registered {
			t.Fatalf("register %s for a classed match = %v", p.name, res)
		}
	}
	id := m.srv.SeedCharacterFor(t, "charlie", "Charlie", 40, 0).ID
	watcher := &competitor{c: m.srv.DialClient(t, "charlie", 1), id: id, name: "Charlie"}
	startInWorld(t, watcher.c)
	at := m.start[0]
	manager := m.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("OlympiadManagerNpc", 31688), location.Location{X: at.X + 30, Y: at.Y, Z: at.Z})
	m.srv.ReadQueued(t, watcher.c)
	m.ones = append(m.ones, watcher)

	m.drawn(t)
	if got := m.stadium(t).Game().Type(); got != olympiad.Classed {
		t.Fatalf("drawn match = %v, want classed", got)
	}
	m.over(t)

	var shouts []string
	for _, f := range watcher.frames {
		if f[0] != serverpackets.OpcodeNpcSay {
			continue
		}
		r := wire.NewReader(f[1:])
		speaker, kind := r.ReadInt32(), r.ReadInt32()
		r.ReadInt32()
		shouts = append(shouts, fmt.Sprintf("%d %d %s", speaker, kind, r.ReadString()))
	}
	want := []string{fmt.Sprintf("%d %d Olympiad class individual match is going to begin in Arena 1 in a moment.", manager.ObjectID(), serverpackets.SayTypeShout)}
	if !slices.Equal(shouts, want) {
		t.Fatalf("shouts = %q, want %q", shouts, want)
	}
	if fights := matchFights(t, m); len(fights) != 1 || fights[0].classed != 1 {
		t.Fatalf("olympiad_fights = %+v, want one classed fight", fights)
	}
}
