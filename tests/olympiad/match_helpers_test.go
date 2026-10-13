package olympiad

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/item"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/location"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/zone"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/clientpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/olympiad"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
	"github.com/fatal10110/acis_golang/internal/testsupport"
)

// The match scenarios fight in one stadium east of the class spawn, its
// sides' spawns 60 apart.
var (
	stadiumSide1 = location.Location{X: 5900, Y: 0, Z: gameservertest.SpawnZ}
	stadiumSide2 = location.Location{X: 5960, Y: 0, Z: gameservertest.SpawnZ}
)

// matchStadium is a zone index holding the one stadium.
func matchStadium(t *testing.T) *zone.Index {
	t.Helper()
	form, err := zone.NewCuboid(5000, 7000, -1000, 1000, -1000, 1000)
	if err != nil {
		t.Fatalf("stadium form: %v", err)
	}
	stadium := zone.NewOlympiad(1, form)
	stadium.AddSpawn(zone.SpawnNormal, stadiumSide1)
	stadium.AddSpawn(zone.SpawnNormal, stadiumSide2)
	stadium.AddSpawn(zone.SpawnNormal, location.Location{X: 5930, Y: 0, Z: gameservertest.SpawnZ})
	ix := zone.NewIndex()
	ix.Add(stadium)
	return ix
}

// competitor is one dialed client and its character.
type competitor struct {
	c      *testsupport.ScriptedClient
	id     int32
	name   string
	frames [][]byte
}

// match is a booted server inside a competition window, two nobles online
// and registered for a non-classed match the next draw pairs, the first
// one on side 1.
type match struct {
	srv  *gameservertest.Server
	ones []*competitor
	// start is where each competitor stood before the match.
	start []location.Location
}

// bootMatch boots the stadium and the two competitors, then registers
// them. adjust tunes the Olympiad configuration on top of a draw needing
// two non-classed registrations.
func bootMatch(t *testing.T, adjust func(*olympiad.Config), opts ...gameservertest.Option) *match {
	t.Helper()
	srv := gameservertest.Boot(t, append([]gameservertest.Option{
		gameservertest.WithCharacter("Alpha", 40, 0),
		gameservertest.WithWantChars(1),
		gameservertest.WithZones(matchStadium(t)),
		gameservertest.WithOlympiadCompetition(90 * time.Minute),
		gameservertest.WithOlympiadDraw(func(int) int { return 0 }),
		gameservertest.WithItemTemplates(item.NewTable(append(gameservertest.ItemTemplates().All(), &item.Template{
			ID: olympiad.NoblesseGatePass, Name: "Noblesse Gate Pass", Kind: item.KindEtcItem, Duration: -1, Stackable: true,
			Dropable: true, Tradable: true, Destroyable: true, Depositable: true, EtcItem: &item.EtcItemDetail{},
		}))),
		gameservertest.WithOlympiadConfig(func(cfg *olympiad.Config) {
			cfg.NonClassedParticipants = 2
			if adjust != nil {
				adjust(cfg)
			}
		}),
	}, opts...)...)
	m := &match{srv: srv, ones: []*competitor{{c: srv.Client, id: srv.SoleObjectID(t), name: "Alpha"}}}
	id := srv.SeedCharacterFor(t, "bravo", "Bravo", 40, 0).ID
	m.ones = append(m.ones, &competitor{c: srv.DialClient(t, "bravo", 1), id: id, name: "Bravo"})
	for _, p := range m.ones {
		startInWorld(t, p.c)
	}
	for _, p := range m.ones {
		m.start = append(m.start, m.character(t, p).CurrentLocation())
		res := srv.Olympiad.Register(olympiad.Applicant{ObjectID: p.id, Name: p.name, BaseClass: m.character(t, p).BaseClassID(), Noble: true}, olympiad.NonClassed)
		if res != olympiad.Registered {
			t.Fatalf("register %s = %v", p.name, res)
		}
	}
	m.read(t)
	for _, p := range m.ones {
		p.frames = nil
	}
	return m
}

// standing is a live player's Olympiad standing and position.
type standing interface {
	OlympiadMode() bool
	OlympiadStarted() bool
	OlympiadGameID() int
	OlympiadSide() int
	BaseClassID() int
	CurrentLocation() location.Location
}

func (m *match) character(t *testing.T, p *competitor) standing {
	t.Helper()
	obj, ok := m.srv.State.Player(p.id)
	if !ok {
		t.Fatalf("%s not in world", p.name)
	}
	return obj.(standing)
}

// stadium is the match's stadium.
func (m *match) stadium(t *testing.T) *olympiad.Stadium {
	t.Helper()
	s, ok := m.srv.Olympiad.Stadium(0)
	if !ok {
		t.Fatal("no stadium 0")
	}
	return s
}

// read collects the frames queued to each competitor still online,
// answering each teleport of its own with Appearing.
func (m *match) read(t *testing.T) {
	t.Helper()
	for _, p := range m.ones {
		if p.c == nil {
			continue
		}
		for _, f := range m.srv.ReadQueued(t, p.c) {
			p.frames = append(p.frames, f)
			if f[0] == serverpackets.OpcodeTeleportToLocation && wire.NewReader(f[1:]).ReadInt32() == p.id {
				p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeAppearing).Bytes())
			}
		}
	}
}

// runUntil lets the clock run a second at a time, reading every frame,
// until done holds; it gives up after limit.
func (m *match) runUntil(t *testing.T, what string, limit time.Duration, done func() bool) {
	t.Helper()
	m.runEvery(t, what, time.Second, limit, done)
}

// runEvery is runUntil moving the clock step at a time.
func (m *match) runEvery(t *testing.T, what string, step, limit time.Duration, done func() bool) {
	t.Helper()
	for passed := time.Duration(0); !done(); passed += step {
		if passed > limit {
			t.Fatalf("%s not reached within %v", what, limit)
		}
		m.srv.Advance(t, step)
		m.read(t)
	}
}

// dealt reports whether p was told it dealt damage.
func (p *competitor) dealt() bool {
	for _, f := range p.frames {
		if f[0] == serverpackets.OpcodeSystemMessage && wire.NewReader(f[1:]).ReadInt32() == serverpackets.SystemMessageYouDidS1Dmg {
			return true
		}
	}
	return false
}

// drawn waits for the draw to start the match.
func (m *match) drawn(t *testing.T) {
	t.Helper()
	s := m.stadium(t)
	m.runUntil(t, "match drawn", 2*time.Minute, s.Running)
}

// over waits for the stadium to fall idle, then reads what the cleanup
// sent.
func (m *match) over(t *testing.T) {
	t.Helper()
	s := m.stadium(t)
	m.runUntil(t, "match over", 15*time.Minute, func() bool { return !s.Running() })
	m.srv.Advance(t, time.Second)
	m.read(t)
}

// fighting waits for the fight to start.
func (m *match) fighting(t *testing.T) {
	t.Helper()
	s := m.stadium(t)
	m.runUntil(t, "fight started", 5*time.Minute, s.BattleStarted)
}

// leave logs p out and waits until it left the world.
func (m *match) leave(t *testing.T, p *competitor) {
	t.Helper()
	p.c.Send(wire.NewPacketWriter(clientpackets.OpcodeLogout).Bytes())
	m.gone(t, p)
}

// drop closes p's connection and waits until it left the world.
func (m *match) drop(t *testing.T, p *competitor) {
	t.Helper()
	if err := p.c.Close(); err != nil {
		t.Fatalf("close %s: %v", p.name, err)
	}
	m.gone(t, p)
}

// gone waits until p left the world.
func (m *match) gone(t *testing.T, p *competitor) {
	t.Helper()
	m.srv.AdvanceUntil(t, p.name+" leaves the world", func() bool {
		_, ok := m.srv.State.Player(p.id)
		return !ok
	})
	p.c = nil
}

// matchMessages are the system messages of a match's course.
var matchMessages = map[int32]bool{
	serverpackets.SystemMessageEnterOlympiadStadiumInS1Seconds: true,
	serverpackets.SystemMessageOlympiadOpponentEndedTheGame:    true,
	serverpackets.SystemMessageOlympiadOpponentUnfit:           true,
	serverpackets.SystemMessageOlympiadGameStartsInS1Seconds:   true,
	serverpackets.SystemMessageOlympiadStartsTheGame:           true,
	serverpackets.SystemMessageS1HasWonTheGame:                 true,
	serverpackets.SystemMessageOlympiadGameEndedInATie:         true,
	serverpackets.SystemMessageMovedToTownInS1Seconds:          true,
	serverpackets.SystemMessageS1HasGainedS2OlympiadPoints:     true,
	serverpackets.SystemMessageS1HasLostS2OlympiadPoints:       true,
	serverpackets.SystemMessageEarnedS2S1S:                     true,
	283:                                                        true, // entered a combat zone
	284:                                                        true, // left a combat zone
}

// course is p's frames reduced to the match's course: its system
// messages, its Olympiad packets and its own teleports, one line each.
func (p *competitor) course() []string {
	var out []string
	for _, f := range p.frames {
		r := wire.NewReader(f[1:])
		switch f[0] {
		case serverpackets.OpcodeSystemMessage:
			id := r.ReadInt32()
			if !matchMessages[id] {
				continue
			}
			line := "SM " + strconv.Itoa(int(id))
			for range int(r.ReadInt32()) {
				if r.ReadInt32() == serverpackets.SystemMessageParamText {
					line += " " + r.ReadString()
				} else {
					line += " " + strconv.Itoa(int(r.ReadInt32()))
				}
			}
			out = append(out, line)
		case serverpackets.OpcodeTeleportToLocation:
			if r.ReadInt32() == p.id {
				out = append(out, fmt.Sprintf("Teleport %d %d", r.ReadInt32(), r.ReadInt32()))
			}
		case serverpackets.OpcodeExtended:
			switch r.ReadUint16() {
			case serverpackets.OpcodeExOlympiadMode:
				out = append(out, fmt.Sprintf("Mode %d", r.ReadUint8()))
			case serverpackets.OpcodeExOlympiadMatchEnd:
				out = append(out, "MatchEnd")
			case serverpackets.OpcodeExOlympiadUserInfo:
				side := r.ReadUint8()
				out = append(out, fmt.Sprintf("UserInfo %d %d", side, r.ReadInt32()))
			}
		}
	}
	return out
}

// countdown is the lines of a countdown message id announcing each of
// seconds.
func countdown(id int, seconds ...int) []string {
	out := make([]string, len(seconds))
	for i, s := range seconds {
		out[i] = fmt.Sprintf("SM %d %d", id, s)
	}
	return out
}

// lines joins parts of a course.
func lines(parts ...any) []string {
	var out []string
	for _, p := range parts {
		switch v := p.(type) {
		case string:
			out = append(out, v)
		case []string:
			out = append(out, v...)
		}
	}
	return out
}
