package olympiad

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// The expected courses follow the stadium task of the reference
// (OlympiadGameTask.run and getDelay, OlympiadGameNormal,
// AbstractOlympiadGame): every countdown announces only the steps of its
// table below the configured start (30 to enter, 60 to fight, 40 to
// return), down to 1; the competitors are told the entry and return
// countdowns and their points, the stadium the fight countdown, the start,
// the result and its combat state.
var (
	enterCountdown  = countdown(serverpackets.SystemMessageEnterOlympiadStadiumInS1Seconds, 30, 15, 10, 5, 4, 3, 2, 1)
	battleCountdown = countdown(serverpackets.SystemMessageOlympiadGameStartsInS1Seconds, 50, 40, 30, 20, 10, 5, 4, 3, 2, 1)
	townCountdown   = countdown(serverpackets.SystemMessageMovedToTownInS1Seconds, 30, 20, 10, 5, 4, 3, 2, 1)
)

// TestOlympiadMatchTimeoutTie drives two nobles through a whole
// non-classed match no one wins: drawn, told the entry countdown, brought
// in on their sides, counted down to the fight, shown both gauges, fought
// until the fight's time runs out, judged a tie that costs each a fifth of
// its points, counted down and sent back where they stood. The fight row
// names no winner and the records count the match. The game manager draws
// without announcing here.
func TestOlympiadMatchTimeoutTie(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) {
		cfg.BattleMillis = 10_000
		cfg.AnnounceGames = false
	})
	a, b := m.ones[0], m.ones[1]

	m.drawn(t)
	if got := m.character(t, a).OlympiadGameID(); got != 0 {
		t.Fatalf("drawn competitor's game = %d, want 0", got)
	}
	if !m.srv.Olympiad.IsRegisteredInComp(a.id, m.character(t, a).BaseClassID()) {
		t.Fatal("a drawn competitor is not in competition")
	}
	m.fighting(t)
	for _, p := range m.ones {
		if c := m.character(t, p); !c.OlympiadMode() || !c.OlympiadStarted() {
			t.Fatalf("%s mode %v, started %v during the fight; want both", p.name, c.OlympiadMode(), c.OlympiadStarted())
		}
	}
	m.over(t)

	for i, p := range m.ones {
		side := i + 1
		spawn := stadiumSide1
		if side == 2 {
			spawn = stadiumSide2
		}
		want := lines(enterCountdown,
			fmt.Sprintf("Teleport %d %d", spawn.X, spawn.Y), fmt.Sprintf("Mode %d", side),
			battleCountdown,
			fmt.Sprintf("UserInfo 1 %d", a.id), fmt.Sprintf("UserInfo 2 %d", b.id),
			"SM 1496", "SM 283",
			"SM 1498", "SM 1658 Alpha 3", "SM 1658 Bravo 3", "SM 284", "MatchEnd",
			townCountdown,
			"Mode 0", fmt.Sprintf("Teleport %d %d", m.start[i].X, m.start[i].Y),
		)
		if got := p.course(); !slices.Equal(got, want) {
			t.Errorf("%s course:\n got %q\nwant %q", p.name, got, want)
		}
		c := m.character(t, p)
		if c.OlympiadMode() || c.OlympiadStarted() || c.OlympiadGameID() != -1 || c.OlympiadSide() != -1 {
			t.Errorf("%s standing after the match: mode %v started %v game %d side %d", p.name, c.OlympiadMode(), c.OlympiadStarted(), c.OlympiadGameID(), c.OlympiadSide())
		}
		n, _ := m.srv.Olympiad.Noble(p.id)
		if n.Points != 15 || n.CompDone != 1 || n.CompWon != 0 || n.CompLost != 0 || n.CompDrawn != 0 {
			t.Errorf("%s record = %+v, want 15 points and one match done", p.name, n)
		}
	}
	if got := matchFights(t, m); len(got) != 1 || got[0].winner != 0 || got[0].one != int(a.id) || got[0].two != int(b.id) || got[0].classed != 0 {
		t.Fatalf("olympiad_fights = %+v, want one non-classed tie of %d against %d", got, a.id, b.id)
	}
}

// matchFights reads the booted server's olympiad_fights rows.
func matchFights(t *testing.T, m *match) []fightRow {
	t.Helper()
	return fightRows(t, m.srv.DB)
}

// TestOlympiadMatchDefection pins a competitor gone before the stadium
// opens: the one still there is told its opponent ended the game, the
// defector loses a third of its points and the match counts for no one; the
// one left is still counted down and released, but never left its place.
func TestOlympiadMatchDefection(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, nil)
	a, b := m.ones[0], m.ones[1]

	m.drawn(t)
	m.leave(t, b)
	m.over(t)

	want := lines(enterCountdown,
		"SM 1493", "SM 1658 Bravo 6",
		townCountdown,
		"Mode 0",
	)
	if got := a.course(); !slices.Equal(got, want) {
		t.Errorf("Alpha course:\n got %q\nwant %q", got, want)
	}
	if c := m.character(t, a); c.OlympiadMode() || c.OlympiadGameID() != -1 || c.CurrentLocation() != m.start[0] {
		t.Errorf("Alpha after the match: mode %v game %d at %+v; want neither, at %+v", c.OlympiadMode(), c.OlympiadGameID(), c.CurrentLocation(), m.start[0])
	}
	if n, _ := m.srv.Olympiad.Noble(a.id); n.Points != 18 || n.CompDone != 0 {
		t.Errorf("Alpha record = %+v, want 18 points and no match", n)
	}
	if n, _ := m.srv.Olympiad.Noble(b.id); n.Points != 12 || n.CompDone != 0 {
		t.Errorf("Bravo record = %+v, want 12 points and no match", n)
	}
	if got := matchFights(t, m); len(got) != 0 {
		t.Fatalf("olympiad_fights = %+v, want none", got)
	}
}

// TestOlympiadMatchDisconnect pins a competitor leaving the world during
// the fight: the fight stops at once and the one still there wins, gaining
// what the other loses and the classed-less reward; no fight row is kept.
func TestOlympiadMatchDisconnect(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, nil)
	a, b := m.ones[0], m.ones[1]

	m.fighting(t)
	m.drop(t, b)
	m.over(t)

	want := lines(enterCountdown,
		"Teleport 5900 0", "Mode 1",
		battleCountdown,
		fmt.Sprintf("UserInfo 1 %d", a.id), fmt.Sprintf("UserInfo 2 %d", b.id), "SM 1496", "SM 283",
		"SM 1497 Alpha", "SM 1657 Alpha 3", "SM 1658 Bravo 3", "SM 53 6651 30", "SM 284", "MatchEnd",
		townCountdown,
		"Mode 0", fmt.Sprintf("Teleport %d %d", m.start[0].X, m.start[0].Y),
	)
	if got := a.course(); !slices.Equal(got, want) {
		t.Errorf("Alpha course:\n got %q\nwant %q", got, want)
	}
	if n, _ := m.srv.Olympiad.Noble(a.id); n.Points != 21 || n.CompDone != 1 || n.CompWon != 1 {
		t.Errorf("Alpha record = %+v, want 21 points, one match won", n)
	}
	if n, _ := m.srv.Olympiad.Noble(b.id); n.Points != 15 || n.CompDone != 1 || n.CompLost != 1 {
		t.Errorf("Bravo record = %+v, want 15 points, one match lost", n)
	}
	if got := matchFights(t, m); len(got) != 0 {
		t.Fatalf("olympiad_fights = %+v, want none", got)
	}
}

// TestOlympiadMatchWonOnDamage pins a fight its time runs out on with both
// competitors standing: the one that dealt more damage, counted from its
// attacks on its opponent, wins and is rewarded, and the fight row names
// side 1 the winner.
func TestOlympiadMatchWonOnDamage(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 6_000 })
	a, b := m.ones[0], m.ones[1]

	m.fighting(t)
	// The first request selects Bravo, the second attacks it; once a hit
	// landed Alpha walks away, leaving Bravo standing.
	a.c.Send(encodeAttackRequest(b.id))
	m.read(t)
	a.c.Send(encodeAttackRequest(b.id))
	m.runEvery(t, "Alpha's first hit", 50*time.Millisecond, 10*time.Second, a.dealt)
	at := m.character(t, a).CurrentLocation()
	a.c.Send(encodeMove(at.X-100, at.Y, at.Z, at.X, at.Y, at.Z))
	m.over(t)
	if m.character(t, b).(interface{ Dead() bool }).Dead() {
		t.Fatal("Bravo died; the fight should end on time")
	}

	got := a.course()
	result := []string{"SM 1497 Alpha", "SM 1657 Alpha 3", "SM 1658 Bravo 3", "SM 53 6651 30", "SM 284", "MatchEnd"}
	i := slices.Index(got, "SM 1497 Alpha")
	if i < 0 || i+len(result) > len(got) || !slices.Equal(got[i:i+len(result)], result) {
		t.Fatalf("Alpha course %q does not hold the result %q", got, result)
	}
	if got := matchFights(t, m); len(got) != 1 || got[0].winner != 1 {
		t.Fatalf("olympiad_fights = %+v, want one won by side 1", got)
	}
	if n, _ := m.srv.Olympiad.Noble(a.id); n.Points != 21 || n.CompWon != 1 {
		t.Errorf("Alpha record = %+v, want 21 points, one match won", n)
	}
}

// encodeMove is a MoveBackwardToLocation clicked from origin to target.
func encodeMove(tx, ty, tz, ox, oy, oz int) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeMoveBackwardToLocation)
	for _, v := range []int{tx, ty, tz, ox, oy, oz, 1} {
		w.WriteInt32(int32(v))
	}
	return w.Bytes()
}

// encodeAttackRequest is an AttackRequest on objectID.
func encodeAttackRequest(objectID int32) []byte {
	w := wire.NewPacketWriter(clientpackets.OpcodeAttackRequest)
	w.WriteInt32(objectID)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteInt32(0)
	w.WriteUint8(0)
	return w.Bytes()
}

// TestOlympiadMatchAnnounced pins the announcing game manager: once a
// match brought its competitors in, every Grand Olympiad Manager shouts it
// once, naming its kind and its arena.
func TestOlympiadMatchAnnounced(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.AnnounceGames = true })
	id := m.srv.SeedCharacterFor(t, "charlie", "Charlie", 40, 0).ID
	watcher := &competitor{c: m.srv.DialClient(t, "charlie", 1), id: id, name: "Charlie"}
	startInWorld(t, watcher.c)
	at := m.start[0]
	manager := m.srv.SpawnFolkNPCAt(t, gameservertest.FolkTemplate("OlympiadManagerNpc", 31688), location.Location{X: at.X + 30, Y: at.Y, Z: at.Z})
	m.srv.ReadQueued(t, watcher.c)
	m.ones = append(m.ones, watcher)

	m.drawn(t)
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
	want := []string{fmt.Sprintf("%d %d Olympiad class-free individual match is going to begin in Arena 1 in a moment.", manager.ObjectID(), serverpackets.SayTypeShout)}
	if !slices.Equal(shouts, want) {
		t.Fatalf("shouts = %q, want %q", shouts, want)
	}
}
