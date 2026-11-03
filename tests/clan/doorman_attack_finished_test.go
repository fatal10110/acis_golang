package clan

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/attackable"
	"github.com/fatal10110/acis_golang/internal/gameserver/model/actor/npc"
	"github.com/fatal10110/acis_golang/internal/gameserver/script"
	"github.com/fatal10110/acis_golang/internal/gameserver/script/ai/wizard"
)

// finishedLog records each attack-finished hook the gatekeeper runs, once
// DoormanOfDrain's own hook returned.
type finishedLog struct {
	mu    sync.Mutex
	lines []string
}

func (l *finishedLog) add(e script.AttackFinished) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf("target=%d dead=%v", e.Target.ObjectID(), e.Target.Dead()))
}

func (l *finishedLog) snapshot() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.lines)
}

// drainSwingWorld is drainSummonWorld with the gatekeeper swinging at atkSpd
// from a reach of 300, its attack-finished hooks logged, and its only desire
// an attack on the founder's golem, left with 1 HP. It returns once the
// golem is dead.
func drainSwingWorld(t *testing.T, atkSpd float64) (*clanWorld, *npc.Hostile, *finishedLog, int32) {
	t.Helper()
	log := &finishedLog{}
	behavior := func() script.Script {
		s := wizard.DoormanOfDrain()
		base := s.Hooks
		s.Hooks = base.With(script.Hooks{OnAttackFinished: func(sc *script.Script, e script.AttackFinished) {
			base.AttackFinished(sc, e)
			log.add(e)
		}})
		return s
	}
	w, h, _, golem := drainSummonWorldWith(t, func(tmpl *npc.Template) {
		tmpl.AtkSpd, tmpl.BaseAttackRange = atkSpd, 300
	}, behavior)
	golem.Queue().Post(func() { golem.SetHP(1) })
	onHostileQueue(t, w, h, func() {
		h.AI().Desires().Clear()
		h.AI().AddAttackDesire(golem, 1e9)
	})
	untilTicking(t, w, "the gatekeeper's swing killing the golem", golem.Dead)
	return w, h, log, golem.ObjectID()
}

// TestDoormanOfDrainTurnsOnTheOwnerOfASummonItsSwingKills runs
// DoormanOfDrain's attack-finished hook on the swing that kills the
// founder's Siege Golem. The swing's finish comes half an attack time after
// its hit, ahead of the AI's re-run 300ms after the hit for a gatekeeper
// swinging at 1250 (an attack time of 400ms): the finish is reported, so
// the hook runs once on the dead golem, after one run on the live golem per
// earlier swing, and turns the gatekeeper on the founder. A re-run that
// comes first and finds nothing left to do idles the gatekeeper, which
// aborts the swing and its finish.
func TestDoormanOfDrainTurnsOnTheOwnerOfASummonItsSwingKills(t *testing.T) {
	t.Parallel()
	w, h, log, golemID := drainSwingWorld(t, 1250)
	live, killed := fmt.Sprintf("target=%d dead=false", golemID), fmt.Sprintf("target=%d dead=true", golemID)
	untilTicking(t, w, "the attack-finished hook on the dead golem", func() bool {
		return slices.Contains(log.snapshot(), killed)
	})
	lines := log.snapshot()
	at := slices.Index(lines, killed)
	for i, l := range lines {
		if (i < at && l != live) || (i > at && strings.HasPrefix(l, fmt.Sprintf("target=%d ", golemID))) {
			t.Fatalf("attack-finished hooks = %q, want one per swing on the live golem, then one on the dead golem", lines)
		}
	}
	obj, ok := w.srv.State.Object(w.leaderID)
	if !ok {
		t.Fatal("the founder is not in the world")
	}
	wantsOwner(t, h, obj.(attackable.Combatant))
}
