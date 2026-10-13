package olympiad

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// gauge is one ExOlympiadUserInfo a client received, with its place among
// the client's frames.
type gauge struct {
	at                   int
	side                 int
	objectID             int32
	hp, maxHP, cp, maxCP int32
}

// gauges are the ExOlympiadUserInfo frames among frames.
func gauges(frames [][]byte) []gauge {
	var out []gauge
	for i, f := range frames {
		if f[0] != serverpackets.OpcodeExtended {
			continue
		}
		r := wire.NewReader(f[1:])
		if r.ReadUint16() != serverpackets.OpcodeExOlympiadUserInfo {
			continue
		}
		g := gauge{at: i, side: int(r.ReadUint8()), objectID: r.ReadInt32()}
		r.ReadString()
		r.ReadInt32() // class
		g.hp, g.maxHP, g.cp, g.maxCP = r.ReadInt32(), r.ReadInt32(), r.ReadInt32(), r.ReadInt32()
		out = append(out, g)
	}
	return out
}

// sysMsgAt is the index of the first system message id among frames from
// index from on, -1 when none.
func sysMsgAt(frames [][]byte, from int, id int32) int {
	for i := from; i < len(frames); i++ {
		if frames[i][0] == serverpackets.OpcodeSystemMessage && wire.NewReader(frames[i][1:]).ReadInt32() == id {
			return i
		}
	}
	return -1
}

// onCharacter runs fn on p's queue and waits for it, staging state no client
// packet reaches.
func (m *match) onCharacter(t *testing.T, p *competitor, fn func(*player.Character)) {
	t.Helper()
	obj, ok := m.srv.State.Player(p.id)
	if !ok {
		t.Fatalf("%s not in world", p.name)
	}
	pc, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%s is not an online character", p.name)
	}
	done := make(chan struct{})
	if !pc.Queue().Post(func() { fn(pc); close(done) }) {
		t.Fatalf("%s's queue is closed", p.name)
	}
	<-done
}

// shown waits until each competitor was shown both battle-start gauges.
func (m *match) shown(t *testing.T) {
	t.Helper()
	m.runUntil(t, "battle-start gauges", 5*time.Minute, func() bool {
		return len(gauges(m.ones[0].frames)) >= 2 && len(gauges(m.ones[1].frames)) >= 2
	})
}

// TestOlympiadMatchBattleStartShowsHealedGauges pins the battle start
// (OlympiadGameTask BATTLE_STARTED, OlympiadGameNormal.healPlayers then
// broadcastOlympiadInfo): a competitor hurt after the countdown's heal is
// healed as the fight starts, and every window shows it at full CP and HP.
func TestOlympiadMatchBattleStartShowsHealedGauges(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 60_000 })
	a := m.ones[0]

	m.runUntil(t, "battle countdown at 10", 5*time.Minute, func() bool { return slices.Contains(a.course(), "SM 1495 10") })
	var maxHP, maxCP int32
	m.onCharacter(t, a, func(pc *player.Character) {
		maxHP, maxCP = int32(pc.MaxHPValue()), int32(pc.MaxCPValue())
		pc.SetHP(pc.MaxHPValue() / 2)
		pc.SetCP(0)
	})
	m.shown(t)

	for _, p := range m.ones {
		var seen bool
		for _, g := range gauges(p.frames)[:2] {
			if g.objectID != a.id {
				continue
			}
			seen = true
			if g.hp != maxHP || g.maxHP != maxHP || g.cp != maxCP || g.maxCP != maxCP {
				t.Errorf("%s shown Alpha at HP %d/%d CP %d/%d, want full %d and %d", p.name, g.hp, g.maxHP, g.cp, g.maxCP, maxHP, maxCP)
			}
		}
		if !seen {
			t.Errorf("%s was not shown Alpha's gauges", p.name)
		}
	}
}

// TestOlympiadMatchVitalsReachTheOtherSide pins the gauge updates of a
// fight (PlayerStatus.broadcastStatusUpdate, OlympiadStadiumZone
// .broadcastStatusUpdate): a hit competitor's new CP and HP reach its
// opponent's window, and not its own.
func TestOlympiadMatchVitalsReachTheOtherSide(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 60_000 })
	a, b := m.ones[0], m.ones[1]

	m.shown(t)
	startA, startB := len(gauges(a.frames)), len(gauges(b.frames))
	a.c.Send(encodeAttackRequest(b.id))
	m.read(t)
	a.c.Send(encodeAttackRequest(b.id))
	m.runEvery(t, "Alpha's first hit", 50*time.Millisecond, 10*time.Second, a.dealt)
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)

	var updated bool
	for _, g := range gauges(a.frames)[startA:] {
		if g.objectID != b.id || g.side != 2 {
			t.Fatalf("Alpha shown gauges of %d on side %d during the fight, want only Bravo's on side 2", g.objectID, g.side)
		}
		if g.cp < g.maxCP || g.hp < g.maxHP {
			updated = true
		}
	}
	if !updated {
		t.Fatalf("Alpha was not shown Bravo's gauges after the hit: %+v", gauges(a.frames)[startA:])
	}
	if got := gauges(b.frames)[startB:]; len(got) != 0 {
		t.Fatalf("Bravo shown its own side's gauges during the fight: %+v", got)
	}
}

// TestOlympiadMatchShownToWhoWalksIn pins a player entering a fighting
// stadium (OlympiadStadiumZone.onEnter, OlympiadGameNormal.sendOlympiadInfo):
// it is told it entered a combat zone, then shown both competitors' gauges,
// side 1 first.
func TestOlympiadMatchShownToWhoWalksIn(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, func(cfg *olympiad.Config) { cfg.BattleMillis = 60_000 })
	a, b := m.ones[0], m.ones[1]
	id := m.srv.SeedCharacterFor(t, "charlie", "Charlie", 40, 0).ID
	if _, err := m.srv.DB.Exec(`UPDATE characters SET x = ?, y = ?, z = ? WHERE obj_Id = ?`, 5930, 0, gameservertest.SpawnZ, id); err != nil {
		t.Fatalf("place Charlie: %v", err)
	}

	m.shown(t)
	walker := &competitor{c: m.srv.DialClient(t, "charlie", 1), id: id, name: "Charlie"}
	walker.frames = startInWorld(t, walker.c)
	walker.frames = append(walker.frames, m.srv.ReadQueued(t, walker.c)...)

	at := sysMsgAt(walker.frames, 0, serverpackets.SystemMessageEnteredCombatZone)
	if at < 0 {
		t.Fatal("Charlie was not told it entered a combat zone")
	}
	got := gauges(walker.frames)
	if len(got) != 2 || got[0].at < at || got[0].side != 1 || got[0].objectID != a.id || got[1].side != 2 || got[1].objectID != b.id {
		t.Fatalf("Charlie shown gauges %+v after the combat notice at %d, want Alpha's on side 1 then Bravo's on side 2", got, at)
	}
}

// TestOlympiadMatchRefusesPrivateStore pins a competitor's store
// (Player.canOpenPrivateStore): one in Olympiad mode is told no private
// store opens here, and no manage window opens.
func TestOlympiadMatchRefusesPrivateStore(t *testing.T) {
	t.Parallel()
	m := bootMatch(t, nil)
	a := m.ones[0]

	m.runUntil(t, "battle countdown at 50", 5*time.Minute, func() bool { return slices.Contains(a.course(), "SM 1495 50") })
	from := len(a.frames)
	a.c.Send(wire.NewPacketWriter(clientpackets.OpcodeRequestPrivateStoreManageSell).Bytes())
	m.srv.Advance(t, 50*time.Millisecond)
	m.read(t)

	if sysMsgAt(a.frames, from, serverpackets.SystemMessageNoPrivateStoreHere) < 0 {
		t.Fatal("a competitor's store was not refused as no store opens here")
	}
	for _, f := range a.frames[from:] {
		if f[0] == serverpackets.OpcodePrivateStoreManageListSell {
			t.Fatal("a competitor's sell manage window opened")
		}
	}
}
