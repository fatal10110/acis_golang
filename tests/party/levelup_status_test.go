package party

import (
	"testing"

	playermodel "github.com/fatal10110/acis_golang/internal/gameserver/model/actor/player"
	"github.com/fatal10110/acis_golang/internal/gameserver/network/serverpackets"
	"github.com/fatal10110/acis_golang/internal/gameserver/sim"
	"github.com/fatal10110/acis_golang/internal/gameservertest"
)

// TestLevelUpUpdatesPartyWindow gains a level on a party member whose
// maxima are below the party bar's resolution, so every one of its status
// reports moves its bar: the other member's party window updates once per
// report — three for the values before the refill, one for the HP and MP
// refill and one for the CP refill.
func TestLevelUpUpdatesPartyWindow(t *testing.T) {
	t.Parallel()
	tmpl := gameservertest.ClassTemplate()
	tmpl.HPTable, tmpl.MPTable, tmpl.CPTable = make([]float64, 85), make([]float64, 85), make([]float64, 85)
	for i := range 85 {
		tmpl.HPTable[i] = float64(80 + 2*i)
		tmpl.MPTable[i] = float64(30 + 2*i)
		tmpl.CPTable[i] = float64(32 + 2*i)
	}
	levels := make(map[int]playermodel.Level, 85)
	for lvl := 1; lvl <= 85; lvl++ {
		levels[lvl] = playermodel.Level{RequiredExpToLevelUp: int64(lvl-1) * 1000}
	}
	table, err := playermodel.NewLevelTable(levels)
	if err != nil {
		t.Fatal(err)
	}
	g := bootGroup(t, []seat{{"Leader", 1}, {"Member", 1}},
		gameservertest.WithClassTemplate(tmpl), gameservertest.WithLevels(table))
	g.invite(t, 0, 1, 0)

	obj, ok := g.srv.State.Player(g.players[0].id)
	if !ok {
		t.Fatal("Leader missing from the world")
	}
	leader, ok := obj.(interface {
		AddExpAndSp(exp int64, sp int)
		Queue() *sim.Queue
	})
	if !ok {
		t.Fatalf("Leader is %T, which cannot gain experience", obj)
	}
	done := make(chan struct{})
	if !leader.Queue().Post(func() { leader.AddExpAndSp(1000, 0); close(done) }) {
		t.Fatal("post the experience: queue closed")
	}
	<-done

	updates := 0
	for _, f := range drainFrames(t, g.players[1].c) {
		if f[0] == serverpackets.OpcodePartySmallWindowUpdate {
			updates++
		}
	}
	if updates != 5 {
		t.Fatalf("Member got %d PartySmallWindowUpdate frames, want 5", updates)
	}
}
