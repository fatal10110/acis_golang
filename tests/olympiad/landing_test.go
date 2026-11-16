package olympiad

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	gamexml "github.com/fatal10110/acis_golang/internal/gameserver/data/xml"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport/datapack"
)

// withAdmin loads the shipped access levels and admin commands.
func withAdmin(t *testing.T) gameservertest.Option {
	t.Helper()
	data, err := gamexml.LoadAdminData(datapack.Path(t, "data", "xml"))
	if err != nil {
		t.Fatalf("load admin data: %v", err)
	}
	return gameservertest.WithAdmin(data)
}

// addGM seeds the GM "Gm", logs it in and has the match read its frames.
func (m *match) addGM(t *testing.T) *competitor {
	t.Helper()
	id := m.srv.SeedCharacterFor(t, "gm", "Gm", 40, 0).ID
	if _, err := m.srv.DB.ExecContext(context.Background(), "UPDATE characters SET accesslevel = 7 WHERE obj_Id = ?", id); err != nil {
		t.Fatalf("set access level: %v", err)
	}
	gm := &competitor{c: m.srv.DialClient(t, "gm", 1), id: id, name: "Gm"}
	startInWorld(t, gm.c)
	m.ones = append(m.ones, gm)
	m.read(t)
	return gm
}

// command has gm run the admin command line.
func (m *match) command(t *testing.T, gm *competitor, line string) {
	t.Helper()
	w := wire.NewPacketWriter(clientpackets.OpcodeSendBypassBuildCmd)
	w.WriteString(line)
	gm.c.Send(w.Bytes())
	m.srv.Advance(t, 50*time.Millisecond)
}

// readHolding is read leaving held's own teleports unanswered, so its jump
// stays in flight.
func (m *match) readHolding(t *testing.T, held *competitor) {
	t.Helper()
	for _, p := range m.ones {
		if p.c == nil {
			continue
		}
		for _, f := range m.srv.ReadQueued(t, p.c) {
			p.frames = append(p.frames, f)
			if p != held && f[0] == serverpackets.OpcodeTeleportToLocation && wire.NewReader(f[1:]).ReadInt32() == p.id {
				p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
			}
		}
	}
}

// land sends held's Appearing and reads what it brought.
func (m *match) land(t *testing.T, held *competitor) []string {
	t.Helper()
	from := len(held.frames)
	held.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)
	return spectatorLines(held.id, held.frames[from:])
}

// TestOlympiadEntryWaitsForTheLanding pins the owner-approved retry of a
// stadium entry refused while the competitor's own teleport lands (#5121):
// recalled by a GM as the match is drawn, Alpha's jump into the stadium is
// held while the recall is in flight, and made once it lands, so Alpha
// fights on its side's spawn; once the match is over it is sent back to
// where it entered from.
func TestOlympiadEntryWaitsForTheLanding(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 10_000 }, withAdmin(t))
	gm := m.addGM(t)
	a := m.ones[0]
	c := m.online(t, a)

	m.drawn(t)
	m.command(t, gm, "recall Alpha")
	m.readHolding(t, a)
	if !c.Teleporting() {
		t.Fatal("the recall is not in flight")
	}
	recalled := c.CurrentLocation()
	for passed := time.Duration(0); !c.OlympiadMode(); passed += time.Second {
		if passed > 2*time.Minute {
			t.Fatal("Alpha never entered the stadium")
		}
		m.srv.Advance(t, time.Second)
		m.readHolding(t, a)
	}
	if !c.Teleporting() || c.CurrentLocation() != recalled {
		t.Fatalf("at the entry: teleporting=%v at %+v, want still landing at %+v", c.Teleporting(), c.CurrentLocation(), recalled)
	}

	if got, want := m.land(t, a), fmt.Sprintf("Teleport %d %d", stadiumSide1.X, stadiumSide1.Y); !slices.Contains(got, want) {
		t.Fatalf("Alpha's landing = %q, want the held %q", got, want)
	}
	if at := c.CurrentLocation(); at.X != stadiumSide1.X || at.Y != stadiumSide1.Y || !c.OlympiadMode() || c.OlympiadGameID() != 0 {
		t.Fatalf("after the landing: at %+v mode=%v game=%d, want competing on %+v", at, c.OlympiadMode(), c.OlympiadGameID(), stadiumSide1)
	}

	m.fighting(t)
	if at := c.CurrentLocation(); at.X != stadiumSide1.X || at.Y != stadiumSide1.Y {
		t.Fatalf("fight started with Alpha at %+v, want on %+v", at, stadiumSide1)
	}
	m.over(t)
	if at := c.CurrentLocation(); c.OlympiadMode() || at.X != recalled.X || at.Y != recalled.Y {
		t.Fatalf("after the match: mode=%v at %+v, want sent back to %+v", c.OlympiadMode(), at, recalled)
	}
}

// TestJailEndsStadiumWatch pins the owner-approved jail of a stadium
// spectator (#4845): its watch ends where it stands, its client out of
// spectating before the jump, and it lands in the jail a plain player with
// no position left to return to, so a later RequestOlympiadObserverEnd does
// not move it.
func TestJailEndsStadiumWatch(t *testing.T) {
	t.Parallel()
	s := bootSpectate(t, withAdmin(t))
	gm := s.addGM(t)
	w := s.watcher
	s.atManager(t, w)
	s.send(t, w, encodeHeroBypass("arenachange 0"))
	c := s.char(t, w)
	if !c.ObserverMode() || c.OlympiadGameID() != 0 {
		t.Fatalf("Charlie does not watch stadium 0: observer=%v game=%d", c.ObserverMode(), c.OlympiadGameID())
	}

	from := len(w.frames)
	s.command(t, gm, "jail Charlie 30")
	s.read(t)
	s.srv.Advance(t, 50*time.Millisecond)
	s.read(t)
	got := spectatorLines(w.id, w.frames[from:])
	if want := []string{"Mode 0", fmt.Sprintf("Teleport %d %d", jailX, jailY)}; !subsequence(got, want) {
		t.Fatalf("Charlie's jail = %q, want %q", got, want)
	}
	if _, saved := c.SavedLocation(); c.ObserverMode() || c.Invisible() || c.Invul() || c.OlympiadGameID() != -1 || saved {
		t.Fatalf("jailed: observer=%v invisible=%v invul=%v game=%d saved=%v, want a plain player",
			c.ObserverMode(), c.Invisible(), c.Invul(), c.OlympiadGameID(), saved)
	}
	if got := s.send(t, w, encodeExRequest(clientpackets.OpcodeRequestOlympiadObserverEnd)); len(got) != 0 {
		t.Fatalf("RequestOlympiadObserverEnd from the jail answered %q, want nothing", spectatorLines(w.id, got))
	}
	if at := c.CurrentLocation(); at.X != jailX || at.Y != jailY {
		t.Fatalf("Charlie stands at %+v, want in the jail at %d %d", at, jailX, jailY)
	}
}

// The shipped jail's entry point.
const (
	jailX = -114356
	jailY = -249645
)
