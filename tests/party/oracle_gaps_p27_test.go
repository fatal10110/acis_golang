package party

import (
	"testing"

	"github.com/fatal10110/acis_golang/internal/commons/wire"
	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// progressionWorld is two players whose class has rising per-level maxima
// below the party bar's resolution and a level every 1000 experience.
type progressionWorld struct {
	g      *group
	levels *playermodel.LevelTable
	tmpl   *playermodel.Template
	leader *playermodel.Character
}

func bootProgression(t *testing.T) *progressionWorld {
	t.Helper()
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPTable, tmpl.MPTable, tmpl.CPTable = make([]float64, 85), make([]float64, 85), make([]float64, 85)
	for i := range 85 {
		tmpl.HPTable[i] = float64(80 + 2*i)
		tmpl.MPTable[i] = float64(30 + 2*i)
		tmpl.CPTable[i] = float64(32 + 2*i)
	}
	rows := make(map[int]playermodel.Level, 85)
	for lvl := 1; lvl <= 85; lvl++ {
		rows[lvl] = playermodel.Level{RequiredExpToLevelUp: int64(lvl-1) * 1000}
	}
	levels, err := playermodel.NewLevelTable(rows)
	if err != nil {
		t.Fatal(err)
	}
	g := bootGroup(t, []seat{{"Leader", 1}, {"Member", 1}},
		gameservertest.WithClassTemplate(tmpl), gameservertest.WithLevels(levels))
	obj, ok := g.srv.State.Player(g.players[0].id)
	if !ok {
		t.Fatal("Leader missing from the world")
	}
	leader, ok := network.OnlineCharacter(obj)
	if !ok {
		t.Fatalf("%T is not an online character", obj)
	}
	return &progressionWorld{g: g, levels: levels, tmpl: tmpl, leader: leader}
}

// onLeader runs fn on Leader's queue and waits for it.
func (w *progressionWorld) onLeader(t *testing.T, fn func()) {
	t.Helper()
	done := make(chan struct{})
	if !w.leader.Queue().Post(func() { defer close(done); fn() }) {
		t.Fatal("post: queue closed")
	}
	<-done
}

// countOpcode counts the frames of opcode.
func countOpcode(frames [][]byte, opcode byte) int {
	n := 0
	for _, f := range frames {
		if f[0] == opcode {
			n++
		}
	}
	return n
}

// TestLevelUpSocialActionReachesObservers pins the level-up announcement
// (PlayerStatus.addLevel, PlayerStatus.java:623-624): the level-up social
// action, action 15, is a broadcast, so a player watching the one who levels
// sees it, while the level-up system message stays with the one who levels.
func TestLevelUpSocialActionReachesObservers(t *testing.T) {
	t.Parallel()
	w := bootProgression(t)
	leader, member := w.g.players[0], w.g.players[1]

	w.onLeader(t, func() { w.leader.AddExpAndSp(w.levels.RequiredExpForLevel(2), 0) })
	w.g.srv.Settle(t)

	for _, tc := range []struct {
		who         player
		wantMessage int
	}{{leader, 1}, {member, 0}} {
		frames := drainFrames(t, tc.who.c)
		var actions [][2]int32
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeSocialAction {
				r := wire.NewReader(f[1:])
				actions = append(actions, [2]int32{r.ReadInt32(), r.ReadInt32()})
			}
		}
		if len(actions) != 1 || actions[0] != [2]int32{leader.id, 15} {
			t.Fatalf("%s saw social actions %v, want one (%d, 15)", tc.who.name, actions, leader.id)
		}
		messages := 0
		for _, f := range frames {
			if f[0] == serverpackets.OpcodeSystemMessage && int(wire.NewReader(f[1:]).ReadInt32()) == serverpackets.SystemMessageYouIncreasedYourLevel {
				messages++
			}
		}
		if messages != tc.wantMessage {
			t.Fatalf("%s saw %d level-up messages, want %d", tc.who.name, messages, tc.wantMessage)
		}
	}
}

// TestLevelDropReportsStatusToParty pins PlayerStatus.removeExpAndSp
// (PlayerStatus.java:583-603): a loss that drops a level reports the new
// status once more after its messages, which reaches the other party member's
// window; a loss inside the level does not.
func TestLevelDropReportsStatusToParty(t *testing.T) {
	t.Parallel()
	w := bootProgression(t)
	w.g.invite(t, 0, 1, 0)
	leader, member := w.g.players[0], w.g.players[1]

	w.onLeader(t, func() { w.leader.AddExpAndSp(w.levels.RequiredExpForLevel(6)+500, 0) })
	w.g.srv.Settle(t)
	w.g.quiet(t)

	// Inside the level: the loss message, no status report for the party.
	w.onLeader(t, func() { w.leader.RemoveExpAndSp(w.levels, w.tmpl, 100, 0) })
	w.g.srv.Settle(t)
	if got := w.leader.Level(); got != 6 {
		t.Fatalf("level after a small loss = %d, want 6", got)
	}
	if n := countOpcode(drainFrames(t, member.c), serverpackets.OpcodePartySmallWindowUpdate); n != 0 {
		t.Fatalf("Member got %d party window updates for a loss inside the level, want 0", n)
	}
	drainFrames(t, leader.c)

	// Across the level: one report after the loss.
	w.onLeader(t, func() { w.leader.RemoveExpAndSp(w.levels, w.tmpl, 1000, 0) })
	w.g.srv.Settle(t)
	if got := w.leader.Level(); got != 5 {
		t.Fatalf("level after the deep loss = %d, want 5", got)
	}
	if n := countOpcode(drainFrames(t, member.c), serverpackets.OpcodePartySmallWindowUpdate); n != 1 {
		t.Fatalf("Member got %d party window updates for the level drop, want 1", n)
	}
}
