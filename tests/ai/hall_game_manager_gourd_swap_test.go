package ai

import (
	"slices"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	modelskill "github.com/fatal10110/acis_golang/internal/gameserver/model/skill"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
)

// hotSpringSwap is the hot springs water that has the manager's gourd and
// the gourd of another circle trade their hit points.
const hotSpringSwap = modelskill.ID(2242)

// tellGourd sends the manager the script event eventID naming gourd, the
// way a circle names its gourd to the others.
func (w *warriorWorld) tellGourd(t *testing.T, manager *npc.Folk, eventID int32, gourd *npc.Folk) {
	t.Helper()
	if !w.srv.RunScript(t, w.player.ObjectID(), "GourdTeller", func(s *script.Script, _ *script.Player) {
		s.SendScriptEvent(script.NPCOf(manager), eventID, gourd.ObjectID(), 0)
	}) {
		t.Fatal("the script event panicked")
	}
	w.srv.Settle(t)
}

// setGourdHP sets the gourd's hit points.
func (w *warriorWorld) setGourdHP(t *testing.T, gourd *npc.Folk, hp float64) {
	t.Helper()
	done := make(chan struct{})
	if !gourd.Queue().Post(func() { defer close(done); gourd.SetHP(hp) }) {
		t.Fatal("the gourd's queue is closed")
	}
	<-done
	w.srv.Settle(t)
}

// TestGameManagerSwapsGourdHitPoints ports the manager's spelled hook for
// the water 2242: it shouts the poured party's leader (1010641), draws one
// of the other circles' gourds, and sets its own gourd's hit points to that
// gourd's, then that gourd's to the manager's gourd's, which are then the
// same; a gourd left with 1 or less breaks. The draw by thirds picks the
// first, second or third circle other than the manager's own.
func TestGameManagerSwapsGourdHitPoints(t *testing.T) {
	t.Parallel()
	for _, row := range []struct {
		name    string
		draw    int
		partner int // index in the others, 0..2, of the gourd drawn
		hp      float64
		broken  bool
	}{
		{"first circle", 0, 0, 50, false},
		{"second circle", 50, 1, 50, false},
		{"third circle", 99, 2, 50, false},
		{"a gourd left at 1 breaks", 50, 1, 1, true},
	} {
		t.Run(row.name, func(t *testing.T) {
			t.Parallel()
			won := make(chan string, 4)
			w := bootGameManager(t, func() gameManagerSetup {
				s := rainbowSprings(t, won, hotSpringSwap)
				s.draw = row.draw
				s.scripts["test.GourdTeller"] = func() script.Script { return script.Script{} }
				return s
			}())
			manager := w.spawnFolk(t, gameManagerID, 100, 0)
			own := w.spawnFolk(t, gourdID, 150, 0)
			w.joinMate(t)
			w.srv.Advance(t, 10*time.Second)
			w.srv.ReadQueued(t, w.srv.Client)
			// The other circles' gourds, told to the manager as events 10002 to 10004.
			var others []*npc.Folk
			for i := range 3 {
				g := w.spawnFolk(t, gourdID, 150, 40*(i+1))
				others = append(others, g)
				w.tellGourd(t, manager, int32(10002+i), g)
				w.setGourdHP(t, g, 100+float64(i))
			}
			w.setGourdHP(t, own, 200)
			w.setGourdHP(t, others[row.partner], row.hp)

			w.managerTalk(t, manager) // selects it
			frames := w.castFrames(t, hotSpringSwap)

			if got, want := says(frames, manager.ObjectID()), []string{script.NpcString(1010641, "Hunter")}; !slices.Equal(got, want) {
				t.Fatalf("manager said %q, want %q", got, want)
			}
			for i, g := range others {
				switch {
				case i == row.partner && row.broken:
					if !g.Dead() {
						t.Errorf("drawn gourd with %v HP is alive, want broken", row.hp)
					}
				case i == row.partner:
					if got := g.CurrentHP(); float64(got) != row.hp {
						t.Errorf("drawn gourd HP = %d, want %v", got, row.hp)
					}
				default:
					if got, want := g.CurrentHP(), 100+i; got != want {
						t.Errorf("gourd %d not drawn has %d HP, want it untouched at %d", i, got, want)
					}
				}
			}
			if row.broken != own.Dead() {
				t.Errorf("manager's gourd broken = %v, want %v", own.Dead(), row.broken)
			}
			if !row.broken {
				if got := own.CurrentHP(); float64(got) != row.hp {
					t.Errorf("manager's gourd HP = %d, want the drawn gourd's %v", got, row.hp)
				}
			}
		})
	}
}
